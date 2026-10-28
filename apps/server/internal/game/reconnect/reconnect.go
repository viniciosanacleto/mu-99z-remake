// Package reconnect is the 0x13C-byte reconnect record of CReconnect
// (FUN_004e1350 / FUN_004e22e0; docs/server-plan/10e-customs.md, "Cache de
// reconexão") and its encoding in the 400-byte ReconnectData column. The
// game package keeps the records and replays them.
package reconnect

import (
	"encoding/binary"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/store"
)

// RecordSize is the record copied into the 400-byte Data.
const RecordSize = 0x13C

// Record is the 0x13C-byte record (offsets of FUN_004e1350).
type Record struct {
	Account string // +0x00 obj+0x6f
	// +0x0b obj+0x85 (password): not kept by the rewrite, see LoginOffline.
	Name string // +0x16 obj+0x7a
	IP   string // +0x21 obj+0x19 (16)
	HWID string // +0x31 obj+0x29 (45)

	Time  int // +0x60 ReconnectTime countdown (CServerInfo+0x1bc)
	Party int // +0x64 obj+0x2f0

	AutoCount     int    // +0x68 obj+0x63c
	AutoAmount    [5]int // +0x6c obj+0x640..0x650
	AutoReset     bool   // +0x80 obj+0x654
	AutoPoints    [5]int // +0x84 obj+0x658..0x668
	PartyFlags    int    // +0x98 obj+0x434
	PartyPassword string // +0x9c obj+0x630 (11)

	AttackActive  bool // +0xa7 obj+0x20d8
	AttackOffline bool // +0xa8 obj+0x20d9
	AttackBuff    int  // +0xac obj+0x20dc
	AttackSkill   int  // +0xb0 obj+0x20e0
	AttackMap     int  // +0xb2 obj+0x20e2
	AttackX       int  // +0xb3 obj+0x20e3
	AttackY       int  // +0xb4 obj+0x20e4
	PickOn        bool // +0xb5 obj+0x22bc
	// +0xb6 obj+0x20f8 is never set (U-CUST-17): always 0.
	Helper       bool      // +0xb7 obj+0x2110
	PShopOpen    bool      // +0xb8 obj+0x2468
	StoreOffline byte      // +0xb9 obj+0x249e
	StoreType    int       // +0xba obj+0x249d
	Prices       [32]int32 // +0xbc the CItem+0x84 price of the shop slots 76..107
}

func putCStr(b []byte, s string) {
	n := copy(b[:len(b)-1], s)
	clear(b[n:])
}

func boolByte(v bool) byte {
	if v {
		return 1
	}
	return 0
}

// Encode is the record as FUN_004e1350 lays it out (the 400-byte Data of
// the DataServer packet; the bytes after 0x13C are 0 here).
func (r *Record) Encode() []byte {
	b := make([]byte, store.ReconnectDataSize)
	le := binary.LittleEndian
	putCStr(b[0x00:0x0b], r.Account)
	putCStr(b[0x16:0x21], r.Name)
	putCStr(b[0x21:0x31], r.IP)
	putCStr(b[0x31:0x5e], r.HWID)
	le.PutUint32(b[0x60:], uint32(int32(r.Time)))
	le.PutUint32(b[0x64:], uint32(int32(r.Party)))
	le.PutUint32(b[0x68:], uint32(int32(r.AutoCount)))
	for i := 0; i < 5; i++ {
		le.PutUint32(b[0x6c+4*i:], uint32(int32(r.AutoAmount[i])))
		le.PutUint32(b[0x84+4*i:], uint32(int32(r.AutoPoints[i])))
	}
	le.PutUint32(b[0x80:], uint32(boolByte(r.AutoReset)))
	le.PutUint32(b[0x98:], uint32(int32(r.PartyFlags)))
	putCStr(b[0x9c:0xa7], r.PartyPassword)
	b[0xa7] = boolByte(r.AttackActive)
	b[0xa8] = boolByte(r.AttackOffline)
	le.PutUint32(b[0xac:], uint32(int32(r.AttackBuff)))
	le.PutUint16(b[0xb0:], uint16(r.AttackSkill))
	b[0xb2], b[0xb3], b[0xb4] = byte(r.AttackMap), byte(r.AttackX), byte(r.AttackY)
	b[0xb5] = boolByte(r.PickOn)
	b[0xb7] = boolByte(r.Helper)
	b[0xb8] = boolByte(r.PShopOpen)
	b[0xb9] = r.StoreOffline
	b[0xba] = byte(r.StoreType)
	for i, p := range r.Prices {
		le.PutUint32(b[0xbc+4*i:], uint32(p))
	}
	return b
}

func cStrAt(b []byte) string {
	for i, c := range b {
		if c == 0 {
			return string(b[:i])
		}
	}
	return string(b)
}

// Decode reads a record from the Data column (FUN_004e22e0
// copies 0x13C bytes from packet+0x18).
func Decode(b []byte) (*Record, bool) {
	if len(b) < RecordSize {
		return nil, false
	}
	le := binary.LittleEndian
	i32 := func(off int) int { return int(int32(le.Uint32(b[off:]))) }
	r := &Record{
		Account: cStrAt(b[0x00:0x0b]), Name: cStrAt(b[0x16:0x21]), IP: cStrAt(b[0x21:0x31]), HWID: cStrAt(b[0x31:0x5e]),
		Time: i32(0x60), Party: i32(0x64), AutoCount: i32(0x68), AutoReset: b[0x80] != 0, PartyFlags: i32(0x98),
		PartyPassword: cStrAt(b[0x9c:0xa7]), AttackActive: b[0xa7] != 0, AttackOffline: b[0xa8] != 0,
		AttackBuff: i32(0xac), AttackSkill: int(le.Uint16(b[0xb0:])), AttackMap: int(b[0xb2]), AttackX: int(b[0xb3]),
		AttackY: int(b[0xb4]), PickOn: b[0xb5] != 0, Helper: b[0xb7] != 0, PShopOpen: b[0xb8] != 0,
		StoreOffline: b[0xb9], StoreType: int(b[0xba]),
	}
	for i := 0; i < 5; i++ {
		r.AutoAmount[i] = i32(0x6c + 4*i)
		r.AutoPoints[i] = i32(0x84 + 4*i)
	}
	for i := range r.Prices {
		r.Prices[i] = int32(le.Uint32(b[0xbc+4*i:]))
	}
	return r, true
}
