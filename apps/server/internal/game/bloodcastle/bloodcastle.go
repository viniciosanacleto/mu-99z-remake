// Package bloodcastle holds the Blood Castle event (CBloodCastle,
// 0x4137b0..0x417b20; Event/BloodCastle.dat) data that does not need the
// world actor: castle limits and classes, entry items, gates and attribute
// rectangles, the participant record and the client packets. The game
// package runs the castles.
package bloodcastle

import "github.com/viniciosanacleto/mu-remake/apps/server/internal/game/packet"

// Castles (maps 11..17), limits, NPC classes and entry items.
const (
	Castles        = 7
	MaxUsers       = 50
	EntrySection   = 0 // EventEntryLevel.dat / EventStageSpawn.dat section
	KickGate       = 0x16
	Archangel      = 0xE8
	Messenger      = 0xE9
	GateClass      = 0x83
	StatueMin      = 0x84
	ItemCloak      = 0x1A12 // Invisibility Cloak (13,18), level = castle + 1
	ItemFreeTicket = 0x1A2F // free entrance (13,47)
	ItemWeapon     = 0x1A13 // Archangel weapon (13,19), level 0..2
)

// Gates are the entry gates per castle.
var Gates = [Castles]int{0x42, 0x43, 0x44, 0x45, 0x46, 0x47, 0x50}

// Rectangles (x1, y1, x2, y2) of the attribute changes (0x54792c,
// 0x5479b0, 0x5479b4).
var (
	EntranceRect = [4]byte{13, 15, 15, 23}
	BridgeRect   = [4]byte{13, 70, 15, 75}
	StatueRects  = [3][4]byte{{13, 76, 15, 79}, {11, 80, 25, 89}, {8, 80, 10, 83}}
)

// MagicSkeleton are the classes counted in step 3 (the others in step 0).
func MagicSkeleton(class int) bool {
	switch class {
	case 0x59, 0x5F, 0x70, 0x76, 0x7C, 0x82, 0x8F, 0x1B1:
		return true
	}
	return false
}

// User is one participant slot of a castle (Index -1 = empty).
type User struct {
	Index, Score, Exp, Money int
}

// PacketResult is C1 1D 93 success FF name[10] and, after two bytes the
// original never writes, score exp zen at 17/21/25 (FUN_004178b0).
func PacketResult(success byte, name string, u User) []byte {
	p := make([]byte, 0x1D)
	p[0], p[1], p[2], p[3], p[4] = 0xC1, 0x1D, 0x93, success, 0xFF
	copy(p[5:15], name)
	packet.PutUint32(p[17:], uint32(u.Score))
	packet.PutUint32(p[21:], uint32(u.Exp))
	packet.PutUint32(p[25:], uint32(u.Money))
	return p
}

// PacketStatus is C1 0E 9B type remain required kills carrier level+1
// (FUN_00417ab0; 0xFF without a weapon level).
func PacketStatus(typ byte, remain, required, kills, carrier, level int) []byte {
	p := make([]byte, 0x0E)
	p[0], p[1], p[2], p[3] = 0xC1, 0x0E, 0x9B, typ
	packet.PutUint16(p[4:], remain)
	packet.PutUint16(p[6:], required)
	packet.PutUint16(p[8:], kills)
	packet.PutUint16(p[10:], carrier)
	p[12] = 0xFF
	if level != 0xFF && level != -1 {
		p[12] = byte(level + 1)
	}
	return p
}

// PacketState is C1 0E 9B type with zeros and 0xFF (the fixed status
// packets of FUN_00414420 / FUN_004145f0 / FUN_00414910 / FUN_00417b20).
func PacketState(typ byte) []byte {
	p := make([]byte, 0x0E)
	p[0], p[1], p[2], p[3], p[12] = 0xC1, 0x0E, 0x9B, typ, 0xFF
	return p
}

// PacketEnterResult is C1 04 9A result (FUN_004174b0).
func PacketEnterResult(r byte) []byte { return []byte{0xC1, 0x04, 0x9A, r} }
