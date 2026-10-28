// Package monsterskill holds the monster skills: the MagicList of a
// monster class, the MonsterSkill / MonsterSkillUnit / MonsterSkillElement
// tables of config/data/monster.toml, the element arithmetic and the
// client packets (C3 1E, C1 69). The game package chooses and applies the
// monster attacks.
package monsterskill

import (
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/packet"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/frame"
)

// List is the MagicList setMonster builds: slot 0 is the
// AttackType (150 -> 50) and some classes get more skills; nothing when
// AttackType is 0.
func List(class, attackType int) []int {
	if attackType == 0 {
		return nil
	}
	s := []int{attackType}
	if attackType == 150 {
		s[0] = 50
	}
	in := func(v ...int) bool {
		for _, c := range v {
			if c == class {
				return true
			}
		}
		return false
	}
	switch {
	case in(0x42, 0x49):
		s = append(s, 0x11)
	case in(0x4d):
		s = append(s, 3)
	case in(0x59, 0x5f, 0x70, 0x76, 0x7c, 0x82, 0x8f, 0x1b1):
		s = append(s, 3)
	case in(0x90, 0xae, 0xb6, 0xbe, 0x104, 0x91, 0xaf, 0xb7, 0xbf, 0x105, 0x92, 0xb0, 0xb8, 0xc0, 0x106,
		0x93, 0xb1, 0xb9, 0xc1, 0x107, 0x94, 0xb2, 0xba, 0xc2, 0x108, 0xa0, 0xb4, 0xbc, 0xc4, 0x10a,
		0xa3, 0xa5, 0xa7, 0xa9, 0xab, 0xad, 0x1ab):
		s = append(s, 0x11)
	case in(0x95, 0xb3, 0xbb, 0xc3, 0x109):
		s = append(s, 1, 0x11)
	case in(0xa1, 0xb5, 0xbd, 0xc5, 0x10b):
		s = append(s, 1, 0x11, 0x37, 200, 0xc9, 0xca)
	case in(0x211, 0x212):
		s = append(s, 0x32)
	case in(0x215):
		s = append(s, 0xef)
	case in(0x216):
		s = append(s, 0xeb)
	case in(0x217):
		s = append(s, 0x29)
	case in(0x218):
		s = append(s, 0x27)
	case in(0x219):
		s = append(s, 0x38)
	case in(0x21a):
		s = append(s, 0x3e)
	case in(0x21b):
		s = append(s, 0xe6)
	case in(0x231):
		s = append(s, 9, 0x26, 0xee)
	}
	return s
}

// PacketArea is C3 09 1E skill user(BE) X Y dir (FUN_00501740).
func PacketArea(skill, user, x, y, dir int) []byte {
	p := packet.New(frame.C3, 0x1E, -1, 9)
	p[3] = byte(skill)
	p[4], p[5] = byte(user>>8), byte(user)
	p[6], p[7], p[8] = byte(x), byte(y), byte(dir)
	return p
}

// AreaSkills are the skill numbers FUN_00500b70 accepts for a
// multi-target attack.
var AreaSkills = map[int]bool{5: true, 8: true, 9: true, 10: true, 12: true, 13: true, 14: true, 24: true,
	47: true, 50: true, 52: true, 55: true, 78: true}

// Element is a MonsterSkillElement.txt row.
type Element struct {
	Index       int
	Name        string
	Type        int // +0x3c
	SuccessRate int // +0x40
	Time        int // +0x44 ContinuanceTime (s)
	IncDecType  int // +0x48
	IncDecValue int // +0x4c
	Nullified   int // +0x50
}

// Unit is a MonsterSkillUnit.txt row (200 slots of 100 bytes).
type Unit struct {
	Index      int // +0x38 (sent in C1 08 69)
	TargetType int // +0x3c: 1 target, 2 self
	ScopeType  int // +0x40: -1 single, 0 distance, 1 area table
	ScopeValue int // +0x44
	Delay      int // +0x48
	Elements   [5]*Element
}

// Info is a MonsterSkill.txt row (0x58 bytes).
type Info struct {
	Monster int
	Types   [10]int
	Units   [10]*Unit
}

// Data is the MonsterSkill.txt table by monster class.
type Data struct {
	byClass map[int]*Info
}

// ForClass returns the MonsterSkill.txt entry of a monster class, or nil.
func (d *Data) ForClass(class int) *Info { return d.byClass[class] }

// Load reads the Monster/Skill tables of config/data/monster.toml.
func Load(configDir string) (*Data, error) {
	f, err := gamedata.LoadMonsterConfig(configDir)
	if err != nil {
		return nil, err
	}
	elems := map[int]*Element{}
	for _, r := range f.MonsterSkillElement.Rows {
		e := &Element{Index: r.Index, Name: r.Name, Type: r.Type, SuccessRate: r.SuccessRate,
			Time: r.ContinuanceTime, IncDecType: r.IncAndDecType, IncDecValue: r.IncAndDecValue, Nullified: r.NullifiedMagic}
		elems[e.Index] = e
	}
	units := map[int]*Unit{}
	for _, r := range f.MonsterSkillUnit.Rows {
		u := &Unit{Index: r.Index, TargetType: r.TargetType, ScopeType: r.ScopeType, ScopeValue: r.ScopeValue,
			Delay: r.Delay}
		for i, slot := range r.Slot {
			u.Elements[i] = elems[slot]
		}
		if u.Index >= 0 && u.Index < 200 {
			units[u.Index] = u
		}
	}
	d := &Data{byClass: map[int]*Info{}}
	for _, r := range f.MonsterSkill.Rows {
		info := &Info{Monster: r.Monster}
		for i, p := range r.Skill {
			info.Types[i] = p[0]
			info.Units[i] = units[p[1]]
		}
		if info.Monster >= 0 && info.Monster < 1000 {
			d.byClass[info.Monster] = info
		}
	}
	return d, nil
}

// UnitCount is the number of skill units the monster has.
func (info *Info) UnitCount() int {
	n := 0
	for _, u := range info.Units {
		if u != nil {
			n++
		}
	}
	return n
}

// PacketUse is C1 08 69 unit monster(LE) target(LE)
// (FUN_004daa40).
func PacketUse(unit, monster, target int) []byte {
	p := packet.New(frame.C1, 0x69, -1, 8)
	p[3] = byte(unit)
	packet.PutUint16(p[4:], monster)
	packet.PutUint16(p[6:], target)
	return p
}

// ElementDelta is the IncAndDecType arithmetic of the life / mana /
// durability elements: 1/101 add value% of the current amount, 2/102
// subtract it, 11/111 add value, 12/112 subtract value (truncated).
func ElementDelta(e *Element, cur float32) int {
	d := 0
	switch e.IncDecType {
	case 1, 0x65:
		d = int(float64(cur) * float64(e.IncDecValue) / 100)
	case 2, 0x66:
		d += int(float64(cur) * float64(e.IncDecValue) / -100)
	}
	switch e.IncDecType {
	case 0xb, 0x6f:
		d += e.IncDecValue
	case 0xc, 0x70:
		d -= e.IncDecValue
	}
	return d
}
