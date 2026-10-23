package crypto

import (
	"encoding/binary"
	"errors"
	"fmt"
	"os"
)

// SimpleModulus block sizes: 8 plain bytes become 11 encrypted bytes.
const (
	smPlainBlock   = 8
	smEncodedBlock = 11
)

// keyFileXor is XORed into every u32 read from a key file (FUN_004ce420 /
// FUN_004ce4e0).
var keyFileXor = [4]uint32{0x3F08A79B, 0xE25CC287, 0x93D27AB9, 0x20DEA7BF}

var (
	ErrKeyFile   = errors.New("crypto: invalid SimpleModulus key file")
	ErrBlock     = errors.New("crypto: invalid SimpleModulus block")
	ErrBlockSize = errors.New("crypto: SimpleModulus input is not a multiple of 11 bytes")
)

// ModulusKeys is one direction (encrypt or decrypt) of a SimpleModulus context.
type ModulusKeys struct {
	Modulus [4]uint32
	Key     [4]uint32
	Xor     [4]uint32
}

// ParseKeyFile decodes an Enc*.dat / Dec*.dat file:
// u16 magic 0x1112, u32 0x36, then three blocks of 4 u32 (modulus, key, xor),
// each u32 XORed with keyFileXor.
func ParseKeyFile(data []byte) (ModulusKeys, error) {
	var k ModulusKeys
	if len(data) < 6+48 ||
		binary.LittleEndian.Uint16(data[0:]) != 0x1112 ||
		binary.LittleEndian.Uint32(data[2:]) != 0x36 {
		return k, ErrKeyFile
	}
	blocks := []*[4]uint32{&k.Modulus, &k.Key, &k.Xor}
	for b, dst := range blocks {
		for i := 0; i < 4; i++ {
			off := 6 + b*16 + i*4
			dst[i] = binary.LittleEndian.Uint32(data[off:]) ^ keyFileXor[i]
		}
	}
	return k, nil
}

// LoadKeyFile reads and parses a key file from disk.
func LoadKeyFile(path string) (ModulusKeys, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return ModulusKeys{}, err
	}
	k, err := ParseKeyFile(data)
	if err != nil {
		return k, fmt.Errorf("%s: %w", path, err)
	}
	return k, nil
}

// EncryptedLen returns the encrypted size of n plain bytes.
func EncryptedLen(n int) int {
	return (n + smPlainBlock - 1) / smPlainBlock * smEncodedBlock
}

// Encrypt implements FUN_004ce630 with the encryption keys.
func (k ModulusKeys) Encrypt(src []byte) []byte {
	out := make([]byte, EncryptedLen(len(src)))
	for off, o := 0, 0; off < len(src); off, o = off+smPlainBlock, o+smEncodedBlock {
		var block [smPlainBlock]byte
		n := copy(block[:], src[off:])
		k.encryptBlock(out[o:o+smEncodedBlock], &block, n)
	}
	return out
}

func (k ModulusKeys) encryptBlock(dst []byte, src *[smPlainBlock]byte, n int) {
	var s [4]uint32
	for i := range s {
		s[i] = uint32(binary.LittleEndian.Uint16(src[i*2:]))
	}
	var r [4]uint32
	r[0] = ((s[0] ^ k.Xor[0]) * k.Key[0]) % k.Modulus[0]
	for i := 1; i < 4; i++ {
		r[i] = ((s[i] ^ k.Xor[i] ^ (r[i-1] & 0xFFFF)) * k.Key[i]) % k.Modulus[i]
	}
	t := [4]uint32{
		k.Xor[0] ^ r[0] ^ (r[1] & 0xFFFF),
		k.Xor[1] ^ r[1] ^ (r[2] & 0xFFFF),
		k.Xor[2] ^ r[2] ^ (r[3] & 0xFFFF),
		r[3], // packed unmasked (local_c); only t2 uses r3 & 0xFFFF
	}
	pos := 0
	for _, v := range t {
		var b [4]byte
		binary.LittleEndian.PutUint32(b[:], v)
		pos = addBits(dst, pos, b[:], 0, 16)
		pos = addBits(dst, pos, b[:], 22, 2)
	}
	var sum byte
	for _, b := range src {
		sum ^= b
	}
	// (checksum<<8 | (checksum ^ 0xF8 ^ len)) ^ 0xF83D, as little-endian bytes.
	tail := [4]byte{sum ^ 0xF8 ^ byte(n) ^ 0x3D, sum ^ 0xF8}
	addBits(dst, pos, tail[:], 0, 16)
}

// Decrypt implements FUN_004ce7e0 with the decryption keys. It fails if any
// block checksum does not match, which makes the original drop the packet.
func (k ModulusKeys) Decrypt(src []byte) ([]byte, error) {
	if len(src)%smEncodedBlock != 0 {
		return nil, ErrBlockSize
	}
	out := make([]byte, 0, len(src)/smEncodedBlock*smPlainBlock)
	for off := 0; off < len(src); off += smEncodedBlock {
		block, n, err := k.decryptBlock(src[off : off+smEncodedBlock])
		if err != nil {
			return nil, err
		}
		out = append(out, block[:n]...)
	}
	return out, nil
}

func (k ModulusKeys) decryptBlock(src []byte) (out [smPlainBlock]byte, n int, err error) {
	var t [4]uint32
	pos := 0
	for i := range t {
		var b [4]byte
		addBits(b[:], 0, src, pos, 16)
		addBits(b[:], 22, src, pos+16, 2)
		t[i] = binary.LittleEndian.Uint32(b[:])
		pos += 18
	}
	t[2] = t[2] ^ (t[3] & 0xFFFF) ^ k.Xor[2]
	t[1] = t[1] ^ (t[2] & 0xFFFF) ^ k.Xor[1]
	t[0] = t[0] ^ (t[1] & 0xFFFF) ^ k.Xor[0]

	w := [4]uint16{
		uint16((k.Key[0]*t[0])%k.Modulus[0]) ^ uint16(k.Xor[0]),
		uint16((k.Key[1]*t[1])%k.Modulus[1]) ^ uint16(k.Xor[1]) ^ uint16(t[0]),
		uint16((k.Key[2]*t[2])%k.Modulus[2]) ^ uint16(k.Xor[2]) ^ uint16(t[1]),
		uint16((k.Key[3]*t[3])%k.Modulus[3]) ^ uint16(t[2]) ^ uint16(k.Xor[3]),
	}
	for i, v := range w {
		binary.LittleEndian.PutUint16(out[i*2:], v)
	}

	var tail [4]byte
	addBits(tail[:], 0, src, pos, 16)
	var sum byte
	for _, b := range out {
		sum ^= b
	}
	if sum^0xF8 != tail[1] {
		return out, 0, ErrBlock
	}
	n = int(tail[0] ^ tail[1] ^ 0x3D)
	if n > smPlainBlock {
		return out, 0, ErrBlock
	}
	return out, n, nil
}

// addBits ORs bitLen bits of src starting at bit srcPos into dst starting at
// bit dstPos, using MSB-first bit order within each byte. It is the semantic
// equivalent of FUN_004ce990 (which shifts whole buffers with FUN_004cea90)
// and returns dstPos + bitLen like the original.
func addBits(dst []byte, dstPos int, src []byte, srcPos, bitLen int) int {
	for i := 0; i < bitLen; i++ {
		s := srcPos + i
		if src[s/8]&(0x80>>(s%8)) != 0 {
			d := dstPos + i
			dst[d/8] |= 0x80 >> (d % 8)
		}
	}
	return dstPos + bitLen
}
