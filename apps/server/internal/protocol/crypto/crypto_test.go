package crypto

import (
	"bytes"
	"errors"
	"path/filepath"
	"runtime"
	"testing"
)

// Key files: server side copied from MuServer/Data/Hack into apps/server/data,
// client side copied from Cliente/Data into testdata.
func keyPath(t *testing.T, parts ...string) string {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(append([]string{filepath.Dir(file)}, parts...)...)
}

func loadKeys(t *testing.T, parts ...string) ModulusKeys {
	t.Helper()
	k, err := LoadKeyFile(keyPath(t, parts...))
	if err != nil {
		t.Fatal(err)
	}
	return k
}

func serverKeys(t *testing.T) (enc, dec ModulusKeys) {
	return loadKeys(t, "..", "..", "..", "data", "Data", "Hack", "Enc2.dat"),
		loadKeys(t, "..", "..", "..", "data", "Data", "Hack", "Dec1.dat")
}

func clientKeys(t *testing.T) (enc, dec ModulusKeys) {
	return loadKeys(t, "testdata", "Enc1.dat"), loadKeys(t, "testdata", "Dec2.dat")
}

func TestParseKeyFile(t *testing.T) {
	// Enc2.dat header and first u32: 12 11 36 00 00 00 | f5 b9 09 3f
	enc, _ := serverKeys(t)
	if want := uint32(0x3F09B9F5 ^ 0x3F08A79B); enc.Modulus[0] != want {
		t.Fatalf("Modulus[0] = %#x, want %#x", enc.Modulus[0], want)
	}
	if _, err := ParseKeyFile([]byte{0x12, 0x11, 0x35, 0, 0, 0}); !errors.Is(err, ErrKeyFile) {
		t.Fatalf("bad header err = %v", err)
	}
	if _, err := ParseKeyFile(make([]byte, 54)); !errors.Is(err, ErrKeyFile) {
		t.Fatalf("bad magic err = %v", err)
	}
}

func TestSimpleModulusRoundTripBetweenClientAndServerKeys(t *testing.T) {
	sEnc, sDec := serverKeys(t)
	cEnc, cDec := clientKeys(t)
	for n := 1; n <= 64; n++ {
		src := make([]byte, n)
		for i := range src {
			src[i] = byte(i*37 + n*11)
		}
		// client -> server: Enc1.dat pairs with Dec1.dat
		enc := cEnc.Encrypt(src)
		if len(enc) != EncryptedLen(n) {
			t.Fatalf("n=%d: len(enc)=%d", n, len(enc))
		}
		got, err := sDec.Decrypt(enc)
		if err != nil || !bytes.Equal(got, src) {
			t.Fatalf("client->server n=%d: got % X err %v", n, got, err)
		}
		// server -> client: Enc2.dat pairs with Dec2.dat
		got, err = cDec.Decrypt(sEnc.Encrypt(src))
		if err != nil || !bytes.Equal(got, src) {
			t.Fatalf("server->client n=%d: got % X err %v", n, got, err)
		}
	}
}

func TestSimpleModulusRejectsCorruptBlocks(t *testing.T) {
	cEnc, _ := clientKeys(t)
	_, sDec := serverKeys(t)
	enc := cEnc.Encrypt([]byte("hello, world"))
	enc[10] ^= 0x01 // checksum byte of the first block
	if _, err := sDec.Decrypt(enc); !errors.Is(err, ErrBlock) {
		t.Fatalf("err = %v, want ErrBlock", err)
	}
	if _, err := sDec.Decrypt(enc[:12]); !errors.Is(err, ErrBlockSize) {
		t.Fatalf("err = %v, want ErrBlockSize", err)
	}
}

func TestXor32(t *testing.T) {
	plain := []byte{0xC1, 0x08, 0xD3, 0x10, 0x20, 0x30, 0x40, 0x50}
	p := bytes.Clone(plain)
	Xor32Encrypt(p)
	// header and head are never touched
	if !bytes.Equal(p[:3], plain[:3]) {
		t.Fatalf("header changed: % X", p[:3])
	}
	// first transformed byte: p[3] ^ p[2] ^ key[3]
	if want := plain[3] ^ plain[2] ^ Xor32Key[3]; p[3] != want {
		t.Fatalf("p[3] = %X, want %X", p[3], want)
	}
	// chained on the encrypted previous byte
	if want := plain[4] ^ p[3] ^ Xor32Key[4]; p[4] != want {
		t.Fatalf("p[4] = %X, want %X", p[4], want)
	}
	Xor32Decrypt(p)
	if !bytes.Equal(p, plain) {
		t.Fatalf("round trip = % X", p)
	}

	c2 := []byte{0xC2, 0x00, 0x07, 0xF3, 0x00, 0x11, 0x22}
	q := bytes.Clone(c2)
	Xor32Encrypt(q)
	if !bytes.Equal(q[:4], c2[:4]) || q[4] != c2[4]^c2[3]^Xor32Key[4] {
		t.Fatalf("C2 start index wrong: % X", q)
	}
	Xor32Decrypt(q)
	if !bytes.Equal(q, c2) {
		t.Fatalf("C2 round trip = % X", q)
	}
}

func TestBux(t *testing.T) {
	p := []byte("abcd")
	Bux(p)
	if want := []byte{'a' ^ 0xFC, 'b' ^ 0xCF, 'c' ^ 0xAB, 'd' ^ 0xFC}; !bytes.Equal(p, want) {
		t.Fatalf("Bux = % X", p)
	}
	Bux(p)
	if string(p) != "abcd" {
		t.Fatalf("Bux not involutive: %q", p)
	}
}

func TestDeriveStreamKeys(t *testing.T) {
	// Values from MuServer/4.GameServer/DATA/GameServerInfo - Common.dat and
	// PATCH CLIENTE/MainInfo.ini (CustomerName / ServerSerial = ClientSerial).
	k, err := DeriveStreamKeys("WizardTeam", "A9kT3wX5rBz7fQ1u")
	if err != nil {
		t.Fatal(err)
	}
	var name [32]byte
	copy(name[:], "WizardTeam")
	var ser [17]byte
	copy(ser[:], "A9kT3wX5rBz7fQ1u")
	var sum uint16
	for i := 0; i < 32; i++ {
		sum += uint16(ser[i%17] ^ name[i])
	}
	if k.Xor != byte(sum)-0x1E || k.Add != byte(sum>>8)+0x76 {
		t.Fatalf("keys = %+v (sum %#x)", k, sum)
	}
	if k != (StreamKeys{Add: wantStreamAdd, Xor: wantStreamXor}) {
		t.Fatalf("keys = %#v, want Add %#x Xor %#x", k, wantStreamAdd, wantStreamXor)
	}

	if _, err := DeriveStreamKeys("50%", "x"); !errors.Is(err, ErrFormatName) {
		t.Fatalf("err = %v", err)
	}
}

func TestStreamKeysRoundTrip(t *testing.T) {
	k := StreamKeys{Add: 0x77, Xor: 0x5A}
	src := []byte{0x00, 0x01, 0x7F, 0x80, 0xFF, 0xC1}
	p := bytes.Clone(src)
	k.Encode(p)
	if p[0] != (0x00+0x77)^0x5A {
		t.Fatalf("Encode[0] = %X", p[0])
	}
	k.Decode(p)
	if !bytes.Equal(p, src) {
		t.Fatalf("round trip = % X", p)
	}
}

// Independently computed for CustomerName "WizardTeam" and serial
// "A9kT3wX5rBz7fQ1u": sum 0x0869.
const (
	wantStreamAdd = 0x7E
	wantStreamXor = 0x4B
)
