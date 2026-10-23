package crypto

// BuxKey is the 3-byte cyclic XOR used for login credentials and client .bmd
// tables (GameServer FUN_00512390).
var BuxKey = [3]byte{0xFC, 0xCF, 0xAB}

// Bux XORs p in place with BuxKey. It is its own inverse.
func Bux(p []byte) {
	for i := range p {
		p[i] ^= BuxKey[i%3]
	}
}
