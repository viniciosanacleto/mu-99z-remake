package crypto

import (
	"errors"
	"strings"
)

// StreamKeys is the byte-wise cipher applied by the GameServer to every byte
// of its client sockets, on top of framing and the other ciphers:
//
//	send    (FUN_00487a30, FUN_005126b0): b = (b + Add) ^ Xor
//	receive (FUN_005051d0):               b = (b ^ Xor) - Add
//
// Add is DAT_00770d7c and Xor is DAT_00770d7d.
type StreamKeys struct {
	Add byte
	Xor byte
}

// ErrFormatName is returned for a CustomerName containing '%': the original
// passes it to wsprintfA as a format string, whose expansion is not reproduced.
var ErrFormatName = errors.New("crypto: CustomerName with '%' is not supported")

// DeriveStreamKeys reproduces FUN_00487a60. customerName is the CustomerName
// key (read into a 32-byte buffer) and serial is ServerSerial (17-byte buffer),
// both from "GameServerInfo - Common.dat".
func DeriveStreamKeys(customerName, serial string) (StreamKeys, error) {
	if strings.ContainsRune(customerName, '%') {
		return StreamKeys{}, ErrFormatName
	}
	var name [32]byte
	copy(name[:31], customerName) // GetPrivateProfileStringA size 0x20 keeps 31 chars + NUL
	var ser [17]byte
	copy(ser[:16], serial) // size 0x11 keeps 16 chars + NUL

	var sum uint16
	for i := 0; i < 32; i++ {
		sum += uint16(ser[i%17] ^ name[i])
	}
	return StreamKeys{
		Xor: byte(sum) - 0x1E,
		Add: byte(sum>>8) + 0x76,
	}, nil
}

// Encode applies the send direction in place.
func (k StreamKeys) Encode(p []byte) {
	for i := range p {
		p[i] = (p[i] + k.Add) ^ k.Xor
	}
}

// Decode applies the receive direction in place.
func (k StreamKeys) Decode(p []byte) {
	for i := range p {
		p[i] = (p[i] ^ k.Xor) - k.Add
	}
}
