// Package friend holds the friend list and mail (CFriend) limits and
// client packets (C0..C8). The game package answers the client heads on
// the store and the Hub, in place of the DataServer B0 requests.
package friend

import (
	"encoding/binary"
	"strings"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/packet"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/frame"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/store"
)

// Friend list and mail limits.
const (
	MaxFriends  = 50   // FUN_00460b20: count > 0x31 => result 3
	MailCost    = 1000 // FUN_00460ee0 / FUN_00460670
	MaxMailMemo = 1000
)

// PacketList is C2 size C0 memoTotal 0x32 count + {name[10], server}
// (FUN_00460350).
func PacketList(memoTotal int, friends []string, server func(string) byte) []byte {
	p := packet.New(frame.C2, 0xC0, -1, 7+11*len(friends))
	p[4], p[5], p[6] = byte(memoTotal), 0x32, byte(len(friends))
	for i, n := range friends {
		packet.PutName(p[7+11*i:], n)
		p[7+11*i+10] = server(n)
	}
	return p
}

// PacketRequest is C1 0D C2 name: someone asks to add the player
// (FUN_004604c0).
func PacketRequest(name string) []byte {
	p := packet.New(frame.C1, 0xC2, -1, 0x0D)
	packet.PutName(p[3:], name)
	return p
}

// PacketState is C1 0E C4 name server (FUN_004609b0); 0xFF = offline.
func PacketState(name string, server byte) []byte {
	p := packet.New(frame.C1, 0xC4, -1, 0x0E)
	packet.PutName(p[3:], name)
	p[13] = server
	return p
}

// PacketAddResult is C1 0F C1 result name server (FUN_004614a0).
func PacketAddResult(result byte, name string, server byte) []byte {
	p := packet.New(frame.C1, 0xC1, -1, 0x0F)
	p[3] = result
	packet.PutName(p[4:], name)
	p[14] = server
	return p
}

// PacketDelResult is C1 0E C3 result name (FUN_00460590).
func PacketDelResult(result byte, name string) []byte {
	p := packet.New(frame.C1, 0xC3, -1, 0x0E)
	p[3] = result
	packet.PutName(p[4:], name)
	return p
}

// PacketMailResult is C1 08 C5 result (FUN_00461570).
func PacketMailResult(result byte) []byte {
	p := packet.New(frame.C1, 0xC5, -1, 8)
	p[3] = result
	return p
}

// PacketMailHeader is C3 50 C6 index(u16) from[10] date[30] subject[32]
// read (FUN_004608a0).
func PacketMailHeader(m store.Mail) []byte {
	p := packet.New(frame.C3, 0xC6, -1, 0x50)
	binary.LittleEndian.PutUint16(p[3:], uint16(m.Index))
	packet.PutName(p[5:], m.From)
	copy(p[15:44], store.MailDate(m.Date))
	copy(p[45:76], m.Subject)
	if m.Read {
		p[77] = 1
	}
	return p
}

// PacketMailRead is C4 size C7 index len photo[18] act dir memo
// (FUN_004606e0).
func PacketMailRead(m *store.Mail) []byte {
	memo := m.Memo
	if len(memo) > MaxMailMemo {
		memo = memo[:MaxMailMemo]
	}
	if i := strings.IndexByte(string(memo), 0); i >= 0 {
		memo = memo[:i]
	}
	p := packet.New(frame.C4, 0xC7, -1, 0x1C+len(memo))
	binary.LittleEndian.PutUint16(p[4:], uint16(m.Index))
	binary.LittleEndian.PutUint16(p[6:], uint16(len(memo)))
	copy(p[8:26], m.Photo)
	p[26], p[27] = m.Act, m.Dir
	copy(p[28:], memo)
	return p
}

// PacketMailDelResult is C1 06 C8 result index (FUN_004607e0).
func PacketMailDelResult(result byte, index int) []byte {
	p := packet.New(frame.C1, 0xC8, -1, 6)
	p[3] = result
	binary.LittleEndian.PutUint16(p[4:], uint16(index))
	return p
}
