package game

import (
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/item"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/packet"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/frame"
)

// Client packet builders. Every layout cites the original builder.

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
	p := packet.New(frame.C4, 0xF3, 0x10, 6+len(body))
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
	p := packet.New(frame.C1, 0xF3, 0x11, 6+len(body))
	packet.PutUint16(p[4:], n)
	copy(p[6:], body)
	return p
}

// packetSkillChange is C1 09 F3 11 FE/FF 00 slot idx byte (FUN_00501810):
// 0xFE adds, 0xFF removes.
func packetSkillChange(op byte, slot int, s Skill) []byte {
	p := packet.New(frame.C1, 0xF3, 0x11, 9)
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
	packet.PutUint32(dst[24:], o.ViewSkillState)
	if o.P != nil && o.P.Char != nil {
		packet.PutName(dst[28:], o.P.Char.Name)
	}
	dst[38], dst[39] = byte(o.TX), byte(o.TY)
	dst[40] = byte(o.Dir)<<4 | byte(o.PKLevel)&0xF
}

// packetViewportPlayers is C2 size 12 count + 44-byte entries.
func packetViewportPlayers(entries [][]byte) []byte {
	p := packet.New(frame.C2, 0x12, -1, 5+44*len(entries))
	p[4] = byte(len(entries))
	for i, e := range entries {
		copy(p[5+44*i:], e)
	}
	return p
}

// packetDestroy is C1 size 14 count + index BE (FUN_00514b80).
func packetDestroy(indexes []int) []byte {
	p := packet.New(frame.C1, 0x14, -1, 4+2*len(indexes))
	p[3] = byte(len(indexes))
	for i, idx := range indexes {
		p[4+2*i], p[5+2*i] = byte(idx>>8), byte(idx)
	}
	return p
}
