// Package packet holds the low-level helpers the game packet builders share:
// allocating a framed C1-C4 packet and writing its little-endian and
// fixed-width name fields.
package packet

import (
	"encoding/binary"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/frame"
)

// PutUint16 writes v as a little-endian u16 (truncated to 16 bits).
func PutUint16(b []byte, v int) { binary.LittleEndian.PutUint16(b, uint16(v)) }

// PutUint32 writes v as a little-endian u32.
func PutUint32(b []byte, v uint32) { binary.LittleEndian.PutUint32(b, v) }

// Clamp65000 mirrors the "< 0xfde9 ? v : 65000" pattern of F3 03.
func Clamp65000(v int) int {
	if v < 65001 {
		return v
	}
	return 65000
}

// New allocates a C1/C3 (1-byte size) or C2/C4 packet of total size n
// with head (and sub when sub >= 0).
func New(kind, head byte, sub int, n int) []byte {
	p := make([]byte, n)
	p[0] = kind
	frame.SetSize(p, n)
	hl := frame.HeaderLen(kind)
	p[hl] = head
	if sub >= 0 {
		p[hl+1] = byte(sub)
	}
	return p
}

// PutGuildName writes an 8-byte guild name.
func PutGuildName(dst []byte, name string) { copy(dst[:8], name) }

// PutName writes a 10-byte character/account name.
func PutName(dst []byte, name string) { copy(dst[:10], name) }
