// Package effect is the effect (buff and debuff) data of CEffectManager /
// CEffect: the EffectList.txt table, the 16 per-object slots, the option
// accumulators, the state bits and the C1 0C 07 state packet. The game
// package applies, ticks and removes effects on its objects.
package effect

import (
	"fmt"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/packet"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/frame"
)

// Slots is the number of CEffect slots per object (obj+0x5fc).
const Slots = 16

// Effect indexes used by the 97D/99z skills (EffectList.txt "Index").
const (
	GreaterDamage    = 1
	GreaterDefense   = 2
	ElfBuffer        = 3
	ManaShield       = 4
	CriticalDamage   = 5
	InfinityArrow    = 6
	GreaterLife      = 8
	GreaterMana      = 9
	BlessPotion      = 10
	SoulPotion       = 11
	Invisibility     = 18
	Brand            = 19
	Poison           = 55
	Ice              = 56
	IceArrow         = 57
	FireSlash        = 58
	PhysiImmunity    = 59
	MagicImmunity    = 60
	Stern            = 61
	MonPhysiImmunity = 63
	MonMagicImmunity = 64
	DamageReflect    = 71
	Immunity34       = 0x22 // FUN_0040b110: full immunity (every hit misses)
	RemovedOnHit     = 0x48 // FUN_0040b110: removed from the target of a damaging hit
)

// Info is an EffectList.txt row.
type Info struct {
	Index, Group, Item int
	Name               string
	Save, Type, Flag   int
	Duration           int // -1 = "*" (the caller's duration)
	Power              [4]int
}

// Slot is one CEffect (Index -1 = empty slot).
type Slot struct {
	Index, Group     int
	Save, Type, Flag int
	Count            int   // seconds left (+0x10); 0 never expires
	Expire           int64 // +0xc: Unix time the effect ends (Type 2 only)
	Value            [4]int
}

// Options are the CEffect option fields of the object (obj+0x6e8..)
// read by CAttack. Player-only fields that CalcCharacter reads (speeds,
// life/mana rates) live in statsEffects (stats.go).
type Options struct {
	AddPhysiDamage        int // obj+0x6e8
	AddMagicDamage        int // obj+0x6ec
	AddCurseDamage        int // obj+0x6f0
	AddAttackSuccessRate  int // obj+0x70c
	AddDefense            int // obj+0x718
	AddDefenseSuccessRate int // obj+0x71c
	DamageReduction       int // obj+0x744 (Mana Shield)
	DamageReflect         int // obj+0x748
	AddCriticalDamage     int // obj+0x750
	SoulExtra             int // obj+0x780 (Soul Potion value 2, not read here)
}

// State is the per-object effect list.
type State struct {
	Slots [Slots]Slot
	Opt   Options
}

func NewState() *State {
	e := &State{}
	for i := range e.Slots {
		e.Slots[i].Index = -1
	}
	return e
}

// LoadList reads the EffectList table of config/data/skill.toml
// (FUN_004786c0): indexes 0..0x7f; an item outside 0..0x1fff is none.
func LoadList(configDir string) (map[int]Info, error) {
	f, err := gamedata.LoadSkillConfig(configDir)
	if err != nil {
		return nil, err
	}
	out := map[int]Info{}
	for i, r := range f.EffectList.Rows {
		e := Info{Index: r.Index, Group: r.Group, Item: -1, Name: r.Name, Save: r.Save, Type: r.Type, Flag: r.Flag,
			Duration: r.Duration, Power: r.Power}
		if r.ItemIndex != "*" {
			cat, idx, err := gamedata.SplitItemRef(r.ItemIndex)
			if err != nil {
				return nil, fmt.Errorf("EffectList row %d: %w", i+1, err)
			}
			if it := cat*0x200 + idx; cat != -1 && it >= 0 && it < 0x2000 {
				e.Item = it
			}
		}
		if e.Index >= 0 && e.Index < 0x80 {
			out[e.Index] = e
		}
	}
	return out, nil
}

// StateBit is FUN_0047a9e0: the ViewSkillState bit of effects
// 1..70 (byte table at 0x47ab10 into the switch at 0x47aa9c); 0 = none.
func StateBit(index int) uint32 {
	if index < 1 || index > 70 {
		return 0
	}
	return Bits[index-1]
}

var Bits = func() [70]uint32 {
	// Bytes 0x47ab10..0x47ab55 read from the binary.
	sel := [70]byte{
		0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09,
		0x0a, 0x1c, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10, 0x11, 0x0b,
		0x0a, 0x0b, 0x0c, 0x0d, 0x1c, 0x1c, 0x12, 0x13, 0x14, 0x14,
		0x14, 0x1c, 0x1c, 0x0c, 0x1c, 0x1c, 0x1c, 0x1c, 0x1c, 0x1c,
		0x1c, 0x1c, 0x1c, 0x1c, 0x1c, 0x1c, 0x1c, 0x1c, 0x1c, 0x14,
		0x14, 0x14, 0x14, 0x14, 0x15, 0x16, 0x17, 0x18, 0x03, 0x01,
		0x19, 0x1c, 0x1a, 0x1b, 0x0d, 0x15, 0x16, 0x00, 0x01, 0x03,
	}
	cases := [...]uint32{4, 8, 0x2000000, 0x100, 0x800, 0x4000000, 0x400, 0x10, 0x2000, 0x8000,
		0x10000, 0x20000, 0x40000, 0x80000, 0x400000, 0x800000, 0x4000, 0x200, 0x10000000,
		0x1000000, 0x8000000, 1, 2, 0x20, 0x40, 0x1000, 0x200000, 0x80, 0}
	var t [70]uint32
	for i, s := range sel {
		t[i] = cases[s]
	}
	return t
}()

// PacketState is C1 0C 07 (FUN_0047ab60): [3] 1 = added / 0 =
// removed, [4..7] ViewSkillState bit (LE), [8..9] object BE.
func PacketState(add bool, bit uint32, index int) []byte {
	p := packet.New(frame.C1, 0x07, -1, 0x0C)
	if add {
		p[3] = 1
	}
	packet.PutUint32(p[4:], bit)
	p[8], p[9] = byte(index>>8), byte(index)
	return p
}
