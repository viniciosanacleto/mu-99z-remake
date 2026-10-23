package game

import (
	"encoding/binary"
	"time"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/frame"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/store"
)

// Castle Siege packet builders (heads B2, B3, B4, B5, B6, B7, B9, BB).
// Multi-byte values are little-endian unless noted.

func timeSeconds(s int64) time.Duration { return time.Duration(s) * time.Second }

func put8(dst []byte, s string) { copy(dst[:8], s) }

// putCSDate writes year (big-endian u16), month, day, hour, minute.
func putCSDate(dst []byte, t time.Time) {
	if t.IsZero() {
		return
	}
	binary.BigEndian.PutUint16(dst, uint16(t.Year()))
	dst[2], dst[3], dst[4], dst[5] = byte(t.Month()), byte(t.Day()), byte(t.Hour()), byte(t.Minute())
}

// csStatePacket holds the fields of B2 00.
type csStatePacket struct {
	Result            byte
	State             byte
	Start, End, Siege time.Time
	Owner, Master     string
	Remain            int64
}

// packetCSState is C1 2E B2 00 (FUN_00423280): [4] result (1 owner,
// 2 no owner, 3 empty owner, 4 owner guild gone, 0 no data) [5] state
// (0xFF unknown) [6..11] state start [12..17] state end [18..23] next
// battle (year BE, month, day, hour, minute) [24..31] owner guild
// [32..41] its master [42..45] seconds left (big-endian).
func packetCSState(s csStatePacket) []byte {
	p := newPacket(frame.C1, 0xB2, 0x00, 0x2E)
	p[4], p[5] = s.Result, s.State
	putCSDate(p[6:], s.Start)
	putCSDate(p[12:], s.End)
	putCSDate(p[18:], s.Siege)
	put8(p[24:], s.Owner)
	putName(p[32:], s.Master)
	binary.BigEndian.PutUint32(p[42:], uint32(int32(s.Remain)))
	return p
}

// packetCSRegister is C1 0D B2 01 result guild[8] (FUN_004223b0).
func packetCSRegister(result byte, guild string) []byte {
	p := newPacket(frame.C1, 0xB2, 0x01, 0x0D)
	p[4] = result
	put8(p[5:], guild)
	return p
}

// packetCSGiveUp is C1 0E B2 02 result giveUp guild[8] (FUN_00422490).
func packetCSGiveUp(result, giveUp byte, guild string) []byte {
	p := newPacket(frame.C1, 0xB2, 0x02, 0x0E)
	p[4], p[5] = result, giveUp
	put8(p[6:], guild)
	return p
}

// packetCSRegInfo is C1 13 B2 03 result guild[8] marks(BE u32) giveUp seq
// (FUN_00424020).
func packetCSRegInfo(result byte, guild string, marks int, giveUp bool, seq int) []byte {
	p := newPacket(frame.C1, 0xB2, 0x03, 0x13)
	p[4] = result
	put8(p[5:], guild)
	binary.BigEndian.PutUint32(p[13:], uint32(marks))
	if giveUp {
		p[17] = 1
	}
	p[18] = byte(seq)
	return p
}

// packetCSRegMark is C1 11 B2 04 result guild[8] marks(BE u32)
// (FUN_004243a0; 3 without a Sign of Lord).
func packetCSRegMark(result byte, guild string, marks int) []byte {
	p := newPacket(frame.C1, 0xB2, 0x04, 0x11)
	p[4] = result
	put8(p[5:], guild)
	binary.BigEndian.PutUint32(p[13:], uint32(marks))
	return p
}

// packetCSNPCBuy is C1 10 B2 05 result pad[3] class index (FUN_00422580).
func packetCSNPCBuy(result byte, class, index int) []byte {
	p := newPacket(frame.C1, 0xB2, 0x05, 0x10)
	p[4] = result
	u32le(p[8:], uint32(class))
	u32le(p[12:], uint32(index))
	return p
}

// packetCSNPCRepair is C1 18 B2 06 result pad[3] class index hp maxHP
// (FUN_00422640).
func packetCSNPCRepair(result byte, class, index, hp, maxHP int) []byte {
	p := newPacket(frame.C1, 0xB2, 0x06, 0x18)
	p[4] = result
	u32le(p[8:], uint32(class))
	u32le(p[12:], uint32(index))
	u32le(p[16:], uint32(hp))
	u32le(p[20:], uint32(maxHP))
	return p
}

// packetCSNPCUpgrade is C1 18 B2 07 result pad[3] class index type value
// (FUN_00422700).
func packetCSNPCUpgrade(result byte, class, index, typ, value int) []byte {
	p := newPacket(frame.C1, 0xB2, 0x07, 0x18)
	p[4] = result
	u32le(p[8:], uint32(class))
	u32le(p[12:], uint32(index))
	u32le(p[16:], uint32(typ))
	u32le(p[20:], uint32(value))
	return p
}

// packetCSTaxInfo is C1 0F B2 08 result chaos store money(BE u64)
// (FUN_00423c10).
func packetCSTaxInfo(result, chaos, store byte, money int64) []byte {
	p := newPacket(frame.C1, 0xB2, 0x08, 0x0F)
	p[4], p[5], p[6] = result, chaos, store
	binary.BigEndian.PutUint64(p[7:], uint64(money))
	return p
}

// packetCSTaxRate is C1 0A B2 09 result kind rate(BE u32) (FUN_004227c0).
func packetCSTaxRate(result, kind byte, rate int) []byte {
	p := newPacket(frame.C1, 0xB2, 0x09, 0x0A)
	p[4], p[5] = result, kind
	binary.BigEndian.PutUint32(p[6:], uint32(rate))
	return p
}

// packetCSMoneyOut is C1 0D B2 10 result money(BE u64) (FUN_00422890).
func packetCSMoneyOut(result byte, money int64) []byte {
	p := newPacket(frame.C1, 0xB2, 0x10, 0x0D)
	p[4] = result
	binary.BigEndian.PutUint64(p[5:], uint64(money))
	return p
}

// packetCSLever is C1 07 B2 11 result gate(BE) (FUN_00422990).
func packetCSLever(result byte, gate int) []byte {
	p := newPacket(frame.C1, 0xB2, 0x11, 0x07)
	p[4], p[5], p[6] = result, byte(gate>>8), byte(gate)
	return p
}

// packetCSGateOperate is C1 08 B2 12 result op gate(BE) (FUN_00422a50).
func packetCSGateOperate(result, op byte, gate int) []byte {
	p := newPacket(frame.C1, 0xB2, 0x12, 0x08)
	p[4], p[5], p[6], p[7] = result, op, byte(gate>>8), byte(gate)
	return p
}

// packetCSGateState is C1 07 B2 13 op gate(BE) to the gate's viewers.
func packetCSGateState(op byte, gate int) []byte {
	p := newPacket(frame.C1, 0xB2, 0x13, 0x07)
	p[4], p[5], p[6] = op, byte(gate>>8), byte(gate)
	return p
}

// packetCSSwitchUser is C1 09 B2 14 switch(BE) user(BE) state
// (FUN_00422b20: 1 taken, 2 held by user, 0 released).
func packetCSSwitchUser(sw, user int, state byte) []byte {
	p := newPacket(frame.C1, 0xB2, 0x14, 0x09)
	p[4], p[5] = byte(sw>>8), byte(sw)
	p[6], p[7] = byte(user>>8), byte(user)
	p[8] = state
	return p
}

// packetCSCrownAccess is C1 0C B2 15 state pad[3] accumulated(ms)
// (FUN_00422bf0).
func packetCSCrownAccess(state byte, acc int) []byte {
	p := newPacket(frame.C1, 0xB2, 0x15, 0x0C)
	p[4] = state
	u32le(p[8:], uint32(acc))
	return p
}

// packetCSCrownState is C1 05 B2 16 v (FUN_00422d10: 0 open, 1 closed,
// 2 taken).
func packetCSCrownState(v byte) []byte {
	p := newPacket(frame.C1, 0xB2, 0x16, 0x05)
	p[4] = v
	return p
}

// packetCSCrownGuild is C1 0D B2 18 v guild[8] to map 30 (FUN_00422db0:
// 0 registration started, 1 castle taken).
func packetCSCrownGuild(v byte, guild string) []byte {
	p := newPacket(frame.C1, 0xB2, 0x18, 0x0D)
	p[4] = v
	put8(p[5:], guild)
	return p
}

// packetCSMinimapResult is C1 05 B2 1B result (FUN_00422f80).
func packetCSMinimapResult(result byte) []byte {
	p := newPacket(frame.C1, 0xB2, 0x1B, 0x05)
	p[4] = result
	return p
}

// packetCSCommand is C1 08 B2 1D team x y command (FUN_00421790).
func packetCSCommand(a, b, c, d byte) []byte {
	p := newPacket(frame.C1, 0xB2, 0x1D, 0x08)
	p[4], p[5], p[6], p[7] = a, b, c, d
	return p
}

// packetCSRemainTime is C1 06 B2 1E hours minutes (FUN_00423080).
func packetCSRemainTime(remainMS int64) []byte {
	p := newPacket(frame.C1, 0xB2, 0x1E, 0x06)
	s := remainMS / 1000
	p[4], p[5] = byte(s/3600), byte(s%3600/60)
	return p
}

// packetCSHuntZone is C1 06 B2 1F result open (FUN_004218a0).
func packetCSHuntZone(result, open byte) []byte {
	p := newPacket(frame.C1, 0xB2, 0x1F, 0x06)
	p[4], p[5] = result, open
	return p
}

// packetCSSwitchInfo is C1 1A B2 20 switch(BE) valid side guild[8]
// name[10] (FUN_004230e0).
func packetCSSwitchInfo(sw int, valid bool, side byte, guild, name string) []byte {
	p := newPacket(frame.C1, 0xB2, 0x20, 0x1A)
	p[4], p[5] = byte(sw>>8), byte(sw)
	if valid {
		p[6] = 1
	}
	p[7] = side
	put8(p[8:], guild)
	putName(p[16:], name)
	return p
}

// csNPCListEntry is one 28-byte B3 entry.
type csNPCListEntry struct {
	Class, Index, DF, RG, MaxLife, Life, X, Y int
	Live                                      bool
}

// packetCSNPCList is C2 size B3 result pad[3] count(u32) + 28 bytes per
// NPC: class, index, DF, RG, max life, life (u32 each), X, Y, live, pad
// (FUN_004219b0).
func packetCSNPCList(result byte, list []csNPCListEntry) []byte {
	if result != 1 {
		p := newPacket(frame.C2, 0xB3, -1, 0x0C)
		p[4] = result
		return p
	}
	p := newPacket(frame.C2, 0xB3, -1, 0x0C+0x1C*len(list))
	p[4] = result
	u32le(p[8:], uint32(len(list)))
	for i, e := range list {
		b := p[0x0C+0x1C*i:]
		u32le(b[0:], uint32(e.Class))
		u32le(b[4:], uint32(e.Index))
		u32le(b[8:], uint32(e.DF))
		u32le(b[12:], uint32(e.RG))
		u32le(b[16:], uint32(e.MaxLife))
		u32le(b[20:], uint32(e.Life))
		b[24], b[25] = byte(e.X), byte(e.Y)
		if e.Live {
			b[26] = 1
		}
	}
	return p
}

// packetCSRegList is C2 size B4 result pad[3] count(u32) + 14 bytes per
// guild: name[8], marks (BE u32), giveUp, seq (FUN_00424d50).
func packetCSRegList(result byte, list []store.CastleRegGuild) []byte {
	if result != 1 {
		list = nil
	}
	p := newPacket(frame.C2, 0xB4, -1, 0x0C+0x0E*len(list))
	p[4] = result
	u32le(p[8:], uint32(len(list)))
	for i, r := range list {
		b := p[0x0C+0x0E*i:]
		put8(b, r.Guild)
		binary.BigEndian.PutUint32(b[8:], uint32(r.Marks))
		if r.GiveUp {
			b[12] = 1
		}
		b[13] = byte(r.Seq)
	}
	return p
}

// csSiegeEntry is one 10-byte B5 entry.
type csSiegeEntry struct {
	Side     int
	Involved bool
	Guild    string
}

// packetCSSiegeList is C2 size B5 result pad[3] count(u32) + {side,
// involved, guild[8]} (FUN_00421dd0: 2 wrong state, 3 not ready).
func packetCSSiegeList(result byte, list []csSiegeEntry) []byte {
	p := newPacket(frame.C2, 0xB5, -1, 0x0C+0x0A*len(list))
	p[4] = result
	u32le(p[8:], uint32(len(list)))
	for i, e := range list {
		b := p[0x0C+0x0A*i:]
		b[0] = byte(e.Side)
		if e.Involved {
			b[1] = 1
		}
		put8(b[2:], e.Guild)
	}
	return p
}

// packetCSMinimapPlayers is C2 size B6 count(u32) + {x, y}
// (FUN_0041fa40).
func packetCSMinimapPlayers(cells [][2]byte) []byte {
	p := newPacket(frame.C2, 0xB6, -1, 8+2*len(cells))
	u32le(p[4:], uint32(len(cells)))
	for i, c := range cells {
		p[8+2*i], p[9+2*i] = c[0], c[1]
	}
	return p
}

// csMinimapNPC is one BB entry: type (0 gate, 1 statue), x, y.
type csMinimapNPC struct{ Type, X, Y byte }

// packetCSMinimapNPCs is C2 size BB count(u32) + {type, x, y}.
func packetCSMinimapNPCs(list []csMinimapNPC) []byte {
	p := newPacket(frame.C2, 0xBB, -1, 8+3*len(list))
	u32le(p[4:], uint32(len(list)))
	for i, n := range list {
		p[8+3*i], p[9+3*i], p[10+3*i] = n.Type, n.X, n.Y
	}
	return p
}

// packetCSWeaponWindow is C1 08 B7 00 1 type index(BE) (FUN_0042a2a0 /
// FUN_0042a380: type 1 attack, 2 defense slingshot).
func packetCSWeaponWindow(typ byte, npc int) []byte {
	p := newPacket(frame.C1, 0xB7, 0x00, 0x08)
	p[4], p[5], p[6], p[7] = 1, typ, byte(npc>>8), byte(npc)
	return p
}

// packetCSWeaponUse is C1 0A B7 01 v npc(BE) type x y (FUN_0042a460: v 1
// to the shooter, 2 to the viewers).
func packetCSWeaponUse(v byte, npc int, typ byte, x, y int) []byte {
	p := newPacket(frame.C1, 0xB7, 0x01, 0x0A)
	p[4], p[5], p[6], p[7], p[8], p[9] = v, byte(npc>>8), byte(npc), typ, byte(x), byte(y)
	return p
}

// packetCSWeaponShot is C1 07 B7 02 type x y (FUN_0042a750).
func packetCSWeaponShot(typ byte, x, y int) []byte {
	p := newPacket(frame.C1, 0xB7, 0x02, 0x07)
	p[4], p[5], p[6] = typ, byte(x), byte(y)
	return p
}

// packetCSStoneState is C1 07 B9 01 index(BE) level (FUN_004a38d0).
func packetCSStoneState(index int, level byte) []byte {
	p := newPacket(frame.C1, 0xB9, 0x01, 0x07)
	p[4], p[5], p[6] = byte(index>>8), byte(index), level
	return p
}

// packetCSOwnerMark is C1 24 B9 02 mark[32] (FUN_00421fb0).
func packetCSOwnerMark(mark [32]byte) []byte {
	p := newPacket(frame.C1, 0xB9, 0x02, 0x24)
	copy(p[4:], mark[:])
	return p
}

// packetCSHuntInfo is C1 14 B9 03 kind open pad[2] tax(u32) 300000 10000
// (FUN_00426d70).
func packetCSHuntInfo(kind, open byte, tax int) []byte {
	p := newPacket(frame.C1, 0xB9, 0x03, 0x14)
	p[4], p[5] = kind, open
	u32le(p[8:], uint32(tax))
	u32le(p[12:], 300000)
	u32le(p[16:], 10000)
	return p
}

// packetCSHuntEnter is C1 05 B9 05 result (FUN_004220b0).
func packetCSHuntEnter(result byte) []byte {
	p := newPacket(frame.C1, 0xB9, 0x05, 0x05)
	p[4] = result
	return p
}
