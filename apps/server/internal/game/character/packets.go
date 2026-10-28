package character

import (
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/packet"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/frame"
)

// PacketJoinResult is F1 00 (FUN_004db050): result 1, index BE, version,
// ServerCode u16.
func PacketJoinResult(index int, version [5]byte, serverCode int) []byte {
	p := packet.New(frame.C1, 0xF1, 0x00, 14)
	p[4] = 1
	p[5], p[6] = byte(index>>8), byte(index)
	copy(p[7:12], version[:])
	packet.PutUint16(p[12:], serverCode)
	return p
}

// PacketLoginResult is C1 05 F1 01 result (FUN_004db220).
func PacketLoginResult(result byte) []byte {
	p := packet.New(frame.C1, 0xF1, 0x01, 5)
	p[4] = result
	return p
}

// PacketLogoutResult is C3 05 F1 02 type (FUN_004db2b0).
func PacketLogoutResult(t byte) []byte {
	p := packet.New(frame.C3, 0xF1, 0x02, 5)
	p[4] = t
	return p
}

// ListEntry is one 34-byte F3 00 entry (FUN_00472d00).
type ListEntry struct {
	Slot        byte
	Name        string
	Level       int
	CtlCode     byte
	CharSet     [18]byte
	GuildStatus byte
}

// PacketList is C3 size F3 00 classFlag moveCnt count + entries.
func PacketList(classFlag, moveCnt byte, entries []ListEntry) []byte {
	p := packet.New(frame.C3, 0xF3, 0x00, 7+34*len(entries))
	p[4], p[5], p[6] = classFlag, moveCnt, byte(len(entries))
	for i, e := range entries {
		b := p[7+34*i:]
		b[0] = e.Slot
		packet.PutName(b[1:], e.Name)
		packet.PutUint16(b[12:], e.Level)
		b[14] = e.CtlCode
		copy(b[15:33], e.CharSet[:])
		b[33] = e.GuildStatus
	}
	return p
}

// PacketEquipment is the custom C1 32 F3 F9 slot + 45 bytes (the
// first nine 5-byte equipment records sent by the DataServer).
func PacketEquipment(slot byte, summary []byte) []byte {
	p := packet.New(frame.C1, 0xF3, 0xF9, 0x32)
	p[4] = slot
	copy(p[5:], summary)
	return p
}

// PacketListInfo is C1 0E DE 00 (FUN_004daef0).
func PacketListInfo(classFlag byte, expA, expB, maxLevel, deleteMaxLevel int) []byte {
	p := packet.New(frame.C1, 0xDE, 0x00, 14)
	p[4] = classFlag
	packet.PutUint16(p[6:], expA)
	packet.PutUint16(p[8:], expB)
	packet.PutUint16(p[10:], maxLevel)
	packet.PutUint16(p[12:], deleteMaxLevel)
	return p
}

// PacketCreateResult is C1 2C F3 01 (FUN_00473770); on failure only result
// and name are set.
func PacketCreateResult(result byte, name string, slot byte, level int, classByte byte) []byte {
	p := packet.New(frame.C1, 0xF3, 0x01, 0x2C)
	p[4] = result
	packet.PutName(p[5:], name)
	if result != 1 {
		return p
	}
	p[15] = slot
	packet.PutUint16(p[16:], level)
	p[18] = classByte
	for i := 19; i < 43; i++ {
		p[i] = 0xFF
	}
	return p
}

// PacketDeleteResult is C1 05 F3 02 result.
func PacketDeleteResult(result byte) []byte {
	p := packet.New(frame.C1, 0xF3, 0x02, 5)
	p[4] = result
	return p
}

// Info is the data of C3 74 F3 03 (FUN_00473a10).
type Info struct {
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

// PacketInfo is C3 B0 F3 03. Bytes 0..55 are the 97D structure read by
// main.exe; the rest is the layout the original client's Main.dll reads in its
// F3 03 handler (0x1001d640): class u16 at 56, resets 60, master resets 64,
// account level 68, 30 custom bytes at 72 and 5 custom u32 at 102 (the
// same unidentified fields F3 E0 carries, sent as 0), GM authority 124 and
// the 12 u32 values from 128. The server builder (0x473a10) was read as a
// 0x74-byte packet with the u32 block at 60 and the level at 108; the
// client reads past that, so this follows the client (U-PROTO-09). The
// level and experience of the Main.dll character window are not in F3 03:
// enterWorld sends F3 E0 after it.
func PacketInfo(c Info) []byte {
	p := packet.New(frame.C3, 0xF3, 0x03, 0xB0)
	p[4], p[5], p[6], p[7] = byte(c.X), byte(c.Y), byte(c.Map), byte(c.Dir)
	packet.PutUint32(p[8:], c.Exp)
	packet.PutUint32(p[12:], c.NextExp)
	for i, v := range []int{c.LevelUpPoints, c.Str, c.Agi, c.Vit, c.Ene, c.Life, c.MaxLife,
		c.Mana, c.MaxMana, c.BP, c.MaxBP} {
		packet.PutUint16(p[16+2*i:], packet.Clamp65000(v))
	}
	packet.PutUint32(p[40:], c.Money)
	p[44] = byte(c.PKLevel)
	p[45] = c.CtlCode
	packet.PutUint16(p[46:], c.FruitAdd)
	packet.PutUint16(p[48:], c.MaxFruit)
	packet.PutUint16(p[50:], packet.Clamp65000(c.Leadership))
	packet.PutUint16(p[52:], c.FruitSub)
	packet.PutUint16(p[54:], c.MaxFruitS)
	packet.PutUint16(p[56:], c.Class)
	packet.PutUint32(p[60:], c.Resets)
	packet.PutUint32(p[64:], c.MasterResets)
	packet.PutUint32(p[68:], c.AccountLevel)
	packet.PutUint32(p[124:], c.Authority)
	for i, v := range []int{c.LevelUpPoints, c.Life, c.MaxLife, c.Mana, c.MaxMana, c.BP, c.MaxBP,
		c.Str, c.Agi, c.Vit, c.Ene, c.Leadership} {
		packet.PutUint32(p[128+4*i:], uint32(v))
	}
	return p
}
