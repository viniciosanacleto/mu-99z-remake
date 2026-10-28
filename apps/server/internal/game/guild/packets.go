package guild

import (
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/packet"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/frame"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/store"
)

// PacketJoinRequest is C1 05 50 idxHi idxLo (FUN_00483580): someone
// asks the master to join.
func PacketJoinRequest(index int) []byte {
	p := packet.New(frame.C1, 0x50, -1, 5)
	p[3], p[4] = byte(index>>8), byte(index)
	return p
}

// PacketResult is C1 04 51 result (FUN_00485640).
func PacketResult(result byte) []byte {
	p := packet.New(frame.C1, 0x51, -1, 4)
	p[3] = result
	return p
}

// PacketLeaveResult is C1 04 53 result (FUN_004856e0).
func PacketLeaveResult(result byte) []byte {
	p := packet.New(frame.C1, 0x53, -1, 4)
	p[3] = result
	return p
}

// PacketMasterQuestion is C1 03 54 (FUN_004da9a0), sent by the Guild
// Master NPC.
func PacketMasterQuestion() []byte { return packet.New(frame.C1, 0x54, -1, 3) }

// PacketCreateWindow is C1 03 55 (FUN_00484210): open the creation window.
func PacketCreateWindow() []byte { return packet.New(frame.C1, 0x55, -1, 3) }

// PacketCreateResult is C1 05 56 result type (FUN_00485780).
func PacketCreateResult(result, typ byte) []byte {
	p := packet.New(frame.C1, 0x56, -1, 5)
	p[3], p[4] = result, typ
	return p
}

// PacketViewportDel is C1 05 5D idxHi idxLo (FUN_005145e0): the
// player's guild is gone.
func PacketViewportDel(index int) []byte {
	p := packet.New(frame.C1, 0x5D, -1, 5)
	p[3], p[4] = byte(index>>8), byte(index)
	return p
}

// ListMember is one 13-byte entry of C2 52.
type ListMember struct {
	Name   string
	Server int
	Status int
}

// PacketList is C2 size 52 result count pad[2] totalScore(u32)
// warScore rival[9] pad[2] + 13 bytes per member {name[10], server byte,
// 0x80|server&0x7f (online) or server&0x7f, status} (FUN_00483a30).
func PacketList(score int, warScore byte, rival string, members []ListMember) []byte {
	p := packet.New(frame.C2, 0x52, -1, 24+13*len(members))
	p[4], p[5] = 1, byte(len(members))
	packet.PutUint32(p[8:], uint32(score))
	p[12] = warScore
	copy(p[13:21], rival)
	for i, m := range members {
		e := p[24+13*i:]
		packet.PutName(e, m.Name)
		e[10] = byte(m.Server)
		e[11] = byte(m.Server) & 0x7F
		if m.Server != -1 {
			e[11] |= 0x80
		}
		e[12] = byte(m.Status)
	}
	return p
}

// ViewEntry is one 12-byte C2 65 entry.
type ViewEntry struct {
	Number   int
	Status   int
	Type     int
	Relation int
	Index    int
}

func (e ViewEntry) put(b []byte) {
	packet.PutUint32(b, uint32(e.Number))
	b[4], b[5], b[6] = byte(e.Status), byte(e.Type), byte(e.Relation)
	b[7], b[8] = byte(e.Index>>8), byte(e.Index)
	if e.Status == store.GuildMasterStatus {
		b[7] |= 0x80
	}
}

// PacketViewport is C2 size 65 count + 12 bytes per player {number u32,
// status, type, relation, idxHi | 0x80 for the master, idxLo, pad[3]}
// (FUN_005160a0; FUN_00516f60 sends one entry with relation 0).
func PacketViewport(entries []ViewEntry) []byte {
	p := packet.New(frame.C2, 0x65, -1, 5+12*len(entries))
	p[4] = byte(len(entries))
	for i, e := range entries {
		e.put(p[5+12*i:])
	}
	return p
}

// UnionViewEntry is one 20-byte C2 67 entry.
type UnionViewEntry struct {
	Index    int
	Number   int
	Relation int
	Union    string
}

// PacketUnionViewport is C2 size 67 count + 20 bytes per player {idxHi,
// idxLo, pad[2], guild number u32, relation u32 (low byte), union master
// guild name[8]} (FUN_005162f0 / FUN_00517080).
func PacketUnionViewport(entries []UnionViewEntry) []byte {
	p := packet.New(frame.C2, 0x67, -1, 5+20*len(entries))
	p[4] = byte(len(entries))
	for i, e := range entries {
		b := p[5+20*i:]
		b[0], b[1] = byte(e.Index>>8), byte(e.Index)
		packet.PutUint32(b[4:], uint32(e.Number))
		b[8] = byte(e.Relation)
		packet.PutGuildName(b[9:], e.Union)
	}
	return p
}

// PacketInfo is C1 3C 66 pad number(u32) type union[8] name[8] mark[32]
// pad[3] (FUN_00484890).
func PacketInfo(g *Record, union string) []byte {
	p := packet.New(frame.C1, 0x66, -1, 0x3C)
	packet.PutUint32(p[4:], uint32(g.Number))
	p[8] = byte(g.Type)
	packet.PutGuildName(p[9:], union)
	packet.PutGuildName(p[17:], g.Name)
	copy(p[25:57], g.Mark[:])
	return p
}

// PacketAssignResult is C1 0F E1 type result name[10] (FUN_00485ba0).
func PacketAssignResult(typ, result byte, name string) []byte {
	p := packet.New(frame.C1, 0xE1, -1, 0x0F)
	p[3], p[4] = typ, result
	packet.PutName(p[5:], name)
	return p
}

// PacketTypeResult is C1 05 E2 type result (FUN_00485c70).
func PacketTypeResult(typ, result byte) []byte {
	p := packet.New(frame.C1, 0xE2, -1, 5)
	p[3], p[4] = typ, result
	return p
}

// PacketRelationRequest is C1 07 E5 rel req idxHi idxLo (FUN_00485d10).
func PacketRelationRequest(rel, req byte, index int) []byte {
	p := packet.New(frame.C1, 0xE5, -1, 7)
	p[3], p[4], p[5], p[6] = rel, req, byte(index>>8), byte(index)
	return p
}

// PacketRelationResult is C1 08 E6 rel req result idxHi idxLo (FUN_00485de0).
func PacketRelationResult(rel, req, result byte, index int) []byte {
	p := packet.New(frame.C1, 0xE6, -1, 8)
	p[3], p[4], p[5], p[6], p[7] = rel, req, result, byte(index>>8), byte(index)
	return p
}

// PacketUnionList is C2 size E9 count result unionCount rivalCount + 41
// bytes per guild {members, mark[32], name[8]} (FUN_00482c40).
func PacketUnionList(result, unions, rivals byte, guilds []Record) []byte {
	p := packet.New(frame.C2, 0xE9, -1, 8+41*len(guilds))
	p[4], p[5], p[6], p[7] = byte(len(guilds)), result, unions, rivals
	for i := range guilds {
		e := p[8+41*i:]
		e[0] = byte(guilds[i].Count())
		copy(e[1:33], guilds[i].Mark[:])
		packet.PutGuildName(e[33:], guilds[i].Name)
	}
	return p
}

// PacketUnionKickResult is C1 07 EB 01 result 02 01 (FUN_004831f0).
func PacketUnionKickResult(result byte) []byte {
	p := packet.New(frame.C1, 0xEB, 0x01, 7)
	p[4], p[5], p[6] = result, 2, 1
	return p
}
