// Package frame implements the C1/C2/C3/C4 packet framing shared by every
// original MU server process (docs/server-plan/02-protocolo-de-rede.md §1).
package frame

import (
	"errors"
	"fmt"
)

// Packet type markers (first byte of every packet).
const (
	C1 byte = 0xC1 // 1-byte size, plain
	C2 byte = 0xC2 // 2-byte big-endian size, plain
	C3 byte = 0xC3 // 1-byte size, SimpleModulus-encrypted body
	C4 byte = 0xC4 // 2-byte big-endian size, SimpleModulus-encrypted body
)

// Maximum packet sizes observed in the original receive buffers.
const (
	MaxSizeConnect = 0x800  // ConnectServer, JoinServer, DataServer
	MaxSizeGame    = 0x2000 // GameServer (FUN_005049c0)
)

var (
	ErrHeader = errors.New("frame: protocol header error")
	ErrSize   = errors.New("frame: protocol size error")
)

// HeaderLen returns the size of the type+size prefix for a packet type
// (2 for C1/C3, 3 for C2/C4).
func HeaderLen(kind byte) int {
	if kind == C2 || kind == C4 {
		return 3
	}
	return 2
}

// Next inspects the start of buf and returns the length of the first complete
// packet. It returns n == 0 with a nil error when more bytes are needed.
// Any header or size violation is fatal for the connection, exactly like the
// original parser, which disconnects the client.
func Next(buf []byte, maxSize int) (n int, err error) {
	if len(buf) < 3 {
		if len(buf) > 0 && !validKind(buf[0]) {
			return 0, fmt.Errorf("%w: 0x%02x", ErrHeader, buf[0])
		}
		return 0, nil
	}
	var size int
	switch buf[0] {
	case C1, C3:
		size = int(buf[1])
	case C2, C4:
		size = int(buf[1])<<8 | int(buf[2])
	default:
		return 0, fmt.Errorf("%w: 0x%02x", ErrHeader, buf[0])
	}
	if size < 3 || size > maxSize {
		return 0, fmt.Errorf("%w: %d", ErrSize, size)
	}
	if len(buf) < size {
		return 0, nil
	}
	return size, nil
}

func validKind(b byte) bool { return b >= C1 && b <= C4 }

// Head returns the head (opcode) byte of a plain C1/C2 packet.
func Head(p []byte) byte { return p[HeaderLen(p[0])] }

// Sub returns the sub-code byte that follows the head, as the GameServer
// reads it: buf[3] for C1, buf[4] for C2. ok is false if the packet is too short.
func Sub(p []byte) (sub byte, ok bool) {
	i := HeaderLen(p[0]) + 1
	if i >= len(p) {
		return 0, false
	}
	return p[i], true
}

// Payload returns the bytes after the head of a plain C1/C2 packet.
func Payload(p []byte) []byte { return p[HeaderLen(p[0])+1:] }

// SetSize writes size into the header of p according to its type.
func SetSize(p []byte, size int) {
	if p[0] == C2 || p[0] == C4 {
		p[1], p[2] = byte(size>>8), byte(size)
		return
	}
	p[1] = byte(size)
}

// Build creates a packet of the given type with head followed by body.
// It panics if a C1/C3 packet would exceed 255 bytes, which is a programming
// error in the caller.
func Build(kind, head byte, body ...byte) []byte {
	hl := HeaderLen(kind)
	size := hl + 1 + len(body)
	if hl == 2 && size > 0xFF {
		panic(fmt.Sprintf("frame: %d bytes do not fit a 0x%02X packet", size, kind))
	}
	p := make([]byte, size)
	p[0] = kind
	SetSize(p, size)
	p[hl] = head
	copy(p[hl+1:], body)
	return p
}
