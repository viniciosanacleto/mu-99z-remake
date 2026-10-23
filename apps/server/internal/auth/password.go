package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Password hashes are stored with a scheme prefix:
//
//	plain:<password>   migrated from MEMB_INFO with MD5Encryption=0
//	argon2id:<salt>:<key>  everything written by this server
//
// The original compares the password with strcmp (case-sensitive), which
// both schemes preserve.
const (
	schemePlain  = "plain:"
	schemeArgon2 = "argon2id:"
)

// argon2id parameters (RFC 9106 second recommended option, reduced memory).
const (
	argonTime    = 2
	argonMemory  = 64 * 1024
	argonThreads = 2
	argonKeyLen  = 32
)

// HashPassword returns an argon2id hash.
func HashPassword(pw string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key := argon2.IDKey([]byte(pw), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	enc := base64.RawStdEncoding
	return schemeArgon2 + enc.EncodeToString(salt) + ":" + enc.EncodeToString(key), nil
}

// CheckPassword verifies pw against a stored hash. needsRehash is true for
// legacy schemes that should be upgraded after a successful login.
func CheckPassword(stored, pw string) (ok, needsRehash bool, err error) {
	switch {
	case strings.HasPrefix(stored, schemePlain):
		want := stored[len(schemePlain):]
		return subtle.ConstantTimeCompare([]byte(want), []byte(pw)) == 1, true, nil
	case strings.HasPrefix(stored, schemeArgon2):
		parts := strings.Split(stored[len(schemeArgon2):], ":")
		if len(parts) != 2 {
			return false, false, fmt.Errorf("auth: malformed argon2id hash")
		}
		enc := base64.RawStdEncoding
		salt, err := enc.DecodeString(parts[0])
		if err != nil {
			return false, false, err
		}
		want, err := enc.DecodeString(parts[1])
		if err != nil {
			return false, false, err
		}
		got := argon2.IDKey([]byte(pw), salt, argonTime, argonMemory, argonThreads, uint32(len(want)))
		return subtle.ConstantTimeCompare(got, want) == 1, false, nil
	}
	return false, false, fmt.Errorf("auth: unknown password scheme")
}
