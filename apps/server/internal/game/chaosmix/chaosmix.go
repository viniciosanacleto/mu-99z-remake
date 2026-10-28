// Package chaosmix holds the Chaos Machine (C1 86) data that does not need
// the world actor: the recipe item indexes, the mix results, the mix
// record, the wing and weapon choices, the set option roll and the client
// packets. The game package runs the mixes on the player's chaos box.
package chaosmix

import (
	"encoding/binary"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/item"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/packet"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/frame"
)

// Chaos box size and the Chaos Goblin NPC class.
const (
	BoxSlots = 32
	Goblin   = 0xEE
)

// Item indexes used by the recipes.
const (
	ItemChaos      = 0x180F // 12,15 Jewel of Chaos
	ItemBless      = 0x1C0D // 14,13
	ItemSoul       = 0x1C0E // 14,14
	ItemLife       = 0x1C10 // 14,16
	ItemDSEye      = 0x1C11 // 14,17 Devil's Eye
	ItemDSKey      = 0x1C12 // 14,18 Devil's Key
	ItemDSInvite   = 0x1C13 // 14,19 Devil's Invitation
	ItemCreation   = 0x1C16 // 14,22
	ItemGuardian   = 0x1C1F // 14,31
	ItemCharmLuck  = 0x1C35 // 14,53 Charm of Luck
	ItemUniria     = 0x1A02 // 13,2
	ItemDinorant   = 0x1A03 // 13,3
	ItemDarkHorse  = 0x1A04 // 13,4
	ItemDarkRaven  = 0x1A05 // 13,5
	ItemLifeStone  = 0x1A0B // 13,11
	ItemFeather    = 0x1A0E // 13,14 (level 1 = Crest of Monarch)
	ItemFruit      = 0x1A0F // 13,15
	ItemArchangel  = 0x1A10 // 13,16 Scroll of Archangel
	ItemBloodBone  = 0x1A11 // 13,17
	ItemCloak      = 0x1A12 // 13,18 Invisibility Cloak
	ItemSpirit     = 0x1A1F // 13,31 Spirit (level 0 horse, 1 raven)
	ItemSiegePot   = 0x1C07 // 14,7
	ItemFenrir     = 0x1A25 // 13,37
	ItemNoUpgrade1 = 0x1C60 // 14,96
	ItemNoUpgrade2 = 0x1CBE // 14,190
)

// Mix results of C1 0B 86.
const (
	ResultFail          = 0
	ResultSuccess       = 1
	ResultNoMoney       = 2
	ResultWrongItems    = 7
	ResultWrongLevel    = 8
	ResultTooManyCharms = 0xF0
)

// Data holds the tables the mixes read (loaded by a start hook).
type Data struct {
	Bags  []gamedata.BagEntry
	Rates *gamedata.ItemOptionRate
	Exc   []gamedata.ExcellentOptionRate
	Sets  *gamedata.SetItemTypes
}

// PacketResult is C1 0B 86 result item[7] (FUN_0042ff70); a nil item
// is sent as FF bytes.
func PacketResult(result byte, it *item.Item) []byte {
	p := packet.New(frame.C1, 0x86, int(result), 0x0B)
	if it == nil {
		for i := 4; i < 11; i++ {
			p[i] = 0xFF
		}
		return p
	}
	it.EncodeWire(p[4:11], false)
	return p
}

// PacketList is C2 size 31 type count {slot, item[7]} (FUN_0042fd60):
// type 3 for the chaos box, 5 for interface 13.
func PacketList(typ byte, box []item.Item) []byte {
	var body []byte
	n := 0
	for slot, it := range box {
		if it.IsEmpty() {
			continue
		}
		var e [8]byte
		e[0] = byte(slot)
		it.EncodeWire(e[1:], false)
		body = append(body, e[:]...)
		n++
	}
	p := packet.New(frame.C2, 0x31, -1, 6+len(body))
	p[4] = typ
	p[5] = byte(n)
	copy(p[6:], body)
	return p
}

// PacketRate is C1 10 88 00 rate(u32) money(u32) tax(u32)
// (FUN_0042ee40).
func PacketRate(rate, money, tax uint32) []byte {
	p := packet.New(frame.C1, 0x88, -1, 0x10)
	binary.LittleEndian.PutUint32(p[4:], rate)
	binary.LittleEndian.PutUint32(p[8:], money)
	binary.LittleEndian.PutUint32(p[12:], tax)
	return p
}

// IsFirstWing is the 0x1800..0x1802 / 0x1829 test of the second wing mix.
func IsFirstWing(idx int) bool { return (idx >= 0x1800 && idx <= 0x1802) || idx == 0x1829 }

// Wing2Choice maps the selection items 13,88..13,92 to the wing they force.
var Wing2Choice = [6]int{0, 0x1805, 0x1804, 0x1803, 0x182A, 0x1806}

// Wing1Choice maps the selection items 13,83..13,86 to the wing they force.
var Wing1Choice = [5]int{0, 0x1802, 0x1801, 0x1800, 0x1829}

// IsChaosWeapon lists the chaos weapons of the first wing mix.
func IsChaosWeapon(idx int) bool { return idx == 0x406 || idx == 0x806 || idx == 0xA07 }

// Item is the item a recipe asks the DataServer to create
// (FUN_004753e0 arguments).
type Item struct {
	Index, Level int
	Durability   byte // 0 = computed (FUN_00494500) unless listed in FixedDurability
	Skill, Luck  bool
	Option       int
	Excellent    byte
	SetOption    byte
}

// FixedDurability lists the items whose created durability is the one the
// mix passes (FUN_004742b0); the others get FUN_00494500.
func FixedDurability(idx int) bool {
	switch idx {
	case ItemCloak, ItemFenrir, 0x1A33, ItemSiegePot, ItemDSInvite, 0x1C6D:
		return true
	}
	return false
}

// MakeSet is FUN_0049f270: the ancient byte for a set option value.
// 1/2: random among the tiers with an option row (tier 1 gives 5 or 9,
// tier 2 gives 10 or 6 for values 1/2); 3/4: tier 1 (5/9); 5/6: tier 2
// (6/10).
func MakeSet(sets *gamedata.SetItemTypes, index, value int, rnd func() int) byte {
	if sets == nil || value == 0 {
		return 0
	}
	r, ok := sets.Get(index)
	if !ok {
		return 0
	}
	t1, t2 := byte(9), byte(10)
	if value%2 == 1 {
		t1 = 5
	}
	if value == 2 || value == 5 {
		t2 = 6
	}
	switch value {
	case 1, 2:
		var cand []byte
		if r.Option[0] > 0 {
			cand = append(cand, t1)
		}
		if r.Option[1] > 0 {
			cand = append(cand, t2)
		}
		if len(cand) == 0 {
			return 0
		}
		return cand[rnd()%len(cand)]
	case 3, 4:
		if r.Option[0] > 0 {
			return t1
		}
	case 5, 6:
		if r.Option[1] > 0 {
			return t2
		}
	}
	return 0
}
