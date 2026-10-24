package game

import (
	"encoding/binary"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/item"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/frame"
)

// Client packet builders. Every layout cites the original builder.

func u16le(b []byte, v int)    { binary.LittleEndian.PutUint16(b, uint16(v)) }
func u32le(b []byte, v uint32) { binary.LittleEndian.PutUint32(b, v) }

// clamp65000 mirrors the "< 0xfde9 ? v : 65000" pattern of F3 03.
func clamp65000(v int) int {
	if v < 65001 {
		return v
	}
	return 65000
}

// newPacket allocates a C1/C3 (1-byte size) or C2/C4 packet of total size n
// with head (and sub when sub >= 0).
func newPacket(kind, head byte, sub int, n int) []byte {
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

// packetJoinResult is F1 00 (FUN_004db050): result 1, index BE, version,
// ServerCode u16.
func packetJoinResult(index int, version [5]byte, serverCode int) []byte {
	p := newPacket(frame.C1, 0xF1, 0x00, 14)
	p[4] = 1
	p[5], p[6] = byte(index>>8), byte(index)
	copy(p[7:12], version[:])
	u16le(p[12:], serverCode)
	return p
}

// packetLoginResult is C1 05 F1 01 result (FUN_004db220).
func packetLoginResult(result byte) []byte {
	p := newPacket(frame.C1, 0xF1, 0x01, 5)
	p[4] = result
	return p
}

// packetLogoutResult is C3 05 F1 02 type (FUN_004db2b0).
func packetLogoutResult(t byte) []byte {
	p := newPacket(frame.C3, 0xF1, 0x02, 5)
	p[4] = t
	return p
}

// putName writes a 10-byte character/account name.
func putName(dst []byte, name string) { copy(dst[:10], name) }

// charListEntry is one 34-byte F3 00 entry (FUN_00472d00).
type charListEntry struct {
	Slot        byte
	Name        string
	Level       int
	CtlCode     byte
	CharSet     [18]byte
	GuildStatus byte
}

// packetCharList is C3 size F3 00 classFlag moveCnt count + entries.
func packetCharList(classFlag, moveCnt byte, entries []charListEntry) []byte {
	p := newPacket(frame.C3, 0xF3, 0x00, 7+34*len(entries))
	p[4], p[5], p[6] = classFlag, moveCnt, byte(len(entries))
	for i, e := range entries {
		b := p[7+34*i:]
		b[0] = e.Slot
		putName(b[1:], e.Name)
		u16le(b[12:], e.Level)
		b[14] = e.CtlCode
		copy(b[15:33], e.CharSet[:])
		b[33] = e.GuildStatus
	}
	return p
}

// packetCharEquipment is the WizardTeam C1 32 F3 F9 slot + 45 bytes (the
// first nine 5-byte equipment records sent by the DataServer).
func packetCharEquipment(slot byte, summary []byte) []byte {
	p := newPacket(frame.C1, 0xF3, 0xF9, 0x32)
	p[4] = slot
	copy(p[5:], summary)
	return p
}

// packetCharListInfo is C1 0E DE 00 (FUN_004daef0).
func packetCharListInfo(classFlag byte, expA, expB, maxLevel, deleteMaxLevel int) []byte {
	p := newPacket(frame.C1, 0xDE, 0x00, 14)
	p[4] = classFlag
	u16le(p[6:], expA)
	u16le(p[8:], expB)
	u16le(p[10:], maxLevel)
	u16le(p[12:], deleteMaxLevel)
	return p
}

// packetCreateResult is C1 2C F3 01 (FUN_00473770); on failure only result
// and name are set.
func packetCreateResult(result byte, name string, slot byte, level int, classByte byte) []byte {
	p := newPacket(frame.C1, 0xF3, 0x01, 0x2C)
	p[4] = result
	putName(p[5:], name)
	if result != 1 {
		return p
	}
	p[15] = slot
	u16le(p[16:], level)
	p[18] = classByte
	for i := 19; i < 43; i++ {
		p[i] = 0xFF
	}
	return p
}

// packetDeleteResult is C1 05 F3 02 result.
func packetDeleteResult(result byte) []byte {
	p := newPacket(frame.C1, 0xF3, 0x02, 5)
	p[4] = result
	return p
}

// charInfo is the data of C3 74 F3 03 (FUN_00473a10).
type charInfo struct {
	X, Y, Map, Dir      int
	Exp, NextExp        uint32
	LevelUpPoints       int
	Str, Agi, Vit, Ene  int
	Life, MaxLife       int
	Mana, MaxMana       int
	BP, MaxBP           int
	Money               uint32
	PKLevel             int
	CtlCode             byte
	FruitAdd, MaxFruit  int
	Leadership          int
	FruitSub, MaxFruitS int
	Class               int
	Resets              uint32
	MasterResets        uint32
	AccountLevel        uint32
	Authority           uint32 // GameMaster.txt level
}

// packetCharInfo is C3 B0 F3 03. Bytes 0..55 are the 97D structure read by
// main.exe; the rest is the layout the WizardTeam Main.dll reads in its
// F3 03 handler (0x1001d640): class u16 at 56, resets 60, master resets 64,
// account level 68, 30 custom bytes at 72 and 5 custom u32 at 102 (the
// same unidentified fields F3 E0 carries, sent as 0), GM authority 124 and
// the 12 u32 values from 128. The server builder (0x473a10) was read as a
// 0x74-byte packet with the u32 block at 60 and the level at 108; the
// client reads past that, so this follows the client (U-PROTO-09). The
// level and experience of the Main.dll character window are not in F3 03:
// enterWorld sends F3 E0 after it.
func packetCharInfo(c charInfo) []byte {
	p := newPacket(frame.C3, 0xF3, 0x03, 0xB0)
	p[4], p[5], p[6], p[7] = byte(c.X), byte(c.Y), byte(c.Map), byte(c.Dir)
	u32le(p[8:], c.Exp)
	u32le(p[12:], c.NextExp)
	for i, v := range []int{c.LevelUpPoints, c.Str, c.Agi, c.Vit, c.Ene, c.Life, c.MaxLife,
		c.Mana, c.MaxMana, c.BP, c.MaxBP} {
		u16le(p[16+2*i:], clamp65000(v))
	}
	u32le(p[40:], c.Money)
	p[44] = byte(c.PKLevel)
	p[45] = c.CtlCode
	u16le(p[46:], c.FruitAdd)
	u16le(p[48:], c.MaxFruit)
	u16le(p[50:], clamp65000(c.Leadership))
	u16le(p[52:], c.FruitSub)
	u16le(p[54:], c.MaxFruitS)
	u16le(p[56:], c.Class)
	u32le(p[60:], c.Resets)
	u32le(p[64:], c.MasterResets)
	u32le(p[68:], c.AccountLevel)
	u32le(p[124:], c.Authority)
	for i, v := range []int{c.LevelUpPoints, c.Life, c.MaxLife, c.Mana, c.MaxMana, c.BP, c.MaxBP,
		c.Str, c.Agi, c.Vit, c.Ene, c.Leadership} {
		u32le(p[128+4*i:], uint32(v))
	}
	return p
}

// packetInventory is C4 size F3 10 count + {slot, item[7]} (FUN_0049b3f0).
func packetInventory(inv []item.Item, xshop func(int) bool) []byte {
	var body []byte
	n := 0
	for slot, it := range inv {
		if it.IsEmpty() {
			continue
		}
		var e [8]byte
		e[0] = byte(slot)
		it.EncodeWire(e[1:], xshop != nil && xshop(it.Index))
		body = append(body, e[:]...)
		n++
	}
	p := newPacket(frame.C4, 0xF3, 0x10, 6+len(body))
	p[5] = byte(n)
	copy(p[6:], body)
	return p
}

// packetSkills is C1 size F3 11 count(u16) + {slot, idx, (idx/255)&7 | lvl<<3}
// (FUN_005019d0).
func packetSkills(skills []Skill) []byte {
	var body []byte
	n := 0
	for slot, s := range skills {
		if s.Index < 0 {
			continue
		}
		body = append(body, byte(slot), byte(s.Index), byte(s.Index/0xFF)&7|s.Level<<3)
		n++
	}
	p := newPacket(frame.C1, 0xF3, 0x11, 6+len(body))
	u16le(p[4:], n)
	copy(p[6:], body)
	return p
}

// packetSkillChange is C1 09 F3 11 FE/FF 00 slot idx byte (FUN_00501810):
// 0xFE adds, 0xFF removes.
func packetSkillChange(op byte, slot int, s Skill) []byte {
	p := newPacket(frame.C1, 0xF3, 0x11, 9)
	p[4] = op
	p[6] = byte(slot)
	p[7] = byte(s.Index)
	p[8] = byte(s.Index/0xFF)&7 | s.Level<<3
	return p
}

// viewportPlayerEntry writes the 44-byte C2 12 entry (FUN_00515160).
func viewportPlayerEntry(dst []byte, o *Object, isNew bool) {
	dst[0], dst[1] = byte(o.Index>>8), byte(o.Index)
	if isNew {
		dst[0] |= 0x80
	}
	dst[2], dst[3] = byte(o.X), byte(o.Y)
	cs := o.CharSet
	cs[0] = cs[0]&^7 | o.ActionState&7
	copy(dst[4:22], cs[:])
	u32le(dst[24:], o.ViewSkillState)
	if o.P != nil && o.P.Char != nil {
		putName(dst[28:], o.P.Char.Name)
	}
	dst[38], dst[39] = byte(o.TX), byte(o.TY)
	dst[40] = byte(o.Dir)<<4 | byte(o.PKLevel)&0xF
}

// packetViewportPlayers is C2 size 12 count + 44-byte entries.
func packetViewportPlayers(entries [][]byte) []byte {
	p := newPacket(frame.C2, 0x12, -1, 5+44*len(entries))
	p[4] = byte(len(entries))
	for i, e := range entries {
		copy(p[5+44*i:], e)
	}
	return p
}

// packetDestroy is C1 size 14 count + index BE (FUN_00514b80).
func packetDestroy(indexes []int) []byte {
	p := newPacket(frame.C1, 0x14, -1, 4+2*len(indexes))
	p[3] = byte(len(indexes))
	for i, idx := range indexes {
		p[4+2*i], p[5+2*i] = byte(idx>>8), byte(idx)
	}
	return p
}
