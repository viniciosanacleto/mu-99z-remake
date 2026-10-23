// Package crypto implements the packet ciphers used between the original MU
// client and servers (docs/server-plan/02-protocolo-de-rede.md §2).
package crypto

// Xor32Key is written by GameServer FUN_004ce420 (0x0173e350).
var Xor32Key = [32]byte{
	0xE7, 0x6D, 0x3A, 0x89, 0xBC, 0xB2, 0x9F, 0x73,
	0x23, 0xA8, 0xFE, 0xB6, 0x49, 0x5D, 0x39, 0x5D,
	0x8A, 0xCB, 0x63, 0x8D, 0xEA, 0x7D, 0x2B, 0x5F,
	0xC3, 0xB1, 0xE9, 0x83, 0x29, 0x51, 0xE8, 0x56,
}

// xor32Start is the index of the head byte: 2 for C1/C3, 3 for C2/C4
// (GameServer FUN_004ceb30 loads EDI with these values).
func xor32Start(p []byte) int {
	if p[0] == 0xC2 || p[0] == 0xC4 {
		return 3
	}
	return 2
}

// Xor32Decrypt undoes the client-side XOR32 in place, as GameServer
// FUN_004cebb0 does: from the last byte down to start+1, each byte is XORed
// with the previous (still encrypted) byte and the key.
func Xor32Decrypt(p []byte) {
	start := xor32Start(p)
	for n := len(p) - 1; n > start; n-- {
		p[n] ^= p[n-1] ^ Xor32Key[n%32]
	}
}

// Xor32Encrypt is the inverse of Xor32Decrypt (what the client does before sending).
func Xor32Encrypt(p []byte) {
	start := xor32Start(p)
	for n := start + 1; n < len(p); n++ {
		p[n] ^= p[n-1] ^ Xor32Key[n%32]
	}
}
