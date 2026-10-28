// Package xshop holds the cash shop of the emulator (CCustomXShop, client
// head FA): its limits and result codes, the purchase record, the option
// check, the client packets and the GM recharge image (the PIX QR record,
// its BMP encoding and the EMV amount). The game package runs the shop
// on the store.
package xshop

import (
	"time"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/packet"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/frame"
)

// Limits of the CCustomXShop handlers.
const (
	ListInterval  = 120 * time.Millisecond   // 0x77 < tick difference
	BuyInterval   = 500 * time.Millisecond   // 499 < tick difference
	PendingWindow = 15000 * time.Millisecond // 0x3a98
	GMInterval    = 300 * time.Millisecond
	MaxPrice      = 2000000000
	PageSize      = 8
)

// Results of FA 04 (FUN_00404720).
const (
	ResultOK         = 0
	ResultNoCredit   = 1
	ResultFailed     = 2 // no room, or the item could not be given (refunded)
	ResultNoItem     = 3 // unknown item or price 0
	ResultBusy       = 5
	ResultBadOptions = 6
	ResultBadPrice   = 7
)

// Purchase is a pending purchase (the 0x2c-byte record of +0x84).
type Purchase struct {
	Account string
	Item    int
	Price   uint32
	At      time.Time
	Seq     uint32
	Name    string
	Opts    [5]byte // level, skill, luck, option, excellent
}

// OptionsValid is FUN_00401000: level < 16, skill and luck < 2, option
// < 8, excellent < 0x40; without flag bit 0 everything must be 0, without
// bit 1 the skill and without bit 2 the excellent byte.
func OptionsValid(o [5]byte, flags byte) bool {
	if o[0] > 15 || o[1] > 1 || o[2] > 1 || o[3] > 7 || o[4] > 0x3f {
		return false
	}
	if flags&1 == 0 && o != ([5]byte{}) {
		return false
	}
	if flags&2 == 0 && o[1] != 0 {
		return false
	}
	return flags&4 != 0 || o[4] == 0
}

// PacketCredits is C1 08 FA 03 credits(u32) (FUN_00404350).
func PacketCredits(credits uint32) []byte {
	p := packet.New(frame.C1, 0xFA, 0x03, 8)
	packet.PutUint32(p[4:], credits)
	return p
}

// PacketResult is C1 10 FA 04 result pad[3] credits(u32) item(u16)
// pad[2] (FUN_00404720; credits in ECX, item in DX, the pads are stack
// bytes the original never writes).
func PacketResult(result byte, credits uint32, item int) []byte {
	p := packet.New(frame.C1, 0xFA, 0x04, 0x10)
	p[4] = result
	packet.PutUint32(p[8:], credits)
	packet.PutUint16(p[12:], item)
	return p
}

// PacketGMResult is C1 0C FA 06 result (FUN_00403860; bytes 5..11
// are not written by the original).
func PacketGMResult(result byte) []byte {
	p := packet.New(frame.C1, 0xFA, 0x06, 0x0C)
	p[4] = result
	return p
}

// Entry is one item of an FA 09 page.
type Entry struct {
	Item  int
	Price uint32
	Flags byte
}

// PacketPage is C1 68 FA 09 category page pages count + 8 entries of
// 12 bytes: item(u16) pad[2] price(u32) flags pad[3] (FUN_00402ed0).
func PacketPage(cat, page, pages int, list []Entry) []byte {
	p := packet.New(frame.C1, 0xFA, 0x09, 0x68)
	p[4], p[5] = byte(cat), byte(page)
	p[6] = 0xFF
	if pages < 0x100 {
		p[6] = byte(pages)
	}
	p[7] = byte(len(list))
	for i, e := range list {
		b := p[8+12*i:]
		packet.PutUint16(b, e.Item)
		packet.PutUint32(b[4:], e.Price)
		b[8] = e.Flags
	}
	return p
}
