package game

import (
	"encoding/binary"
	"testing"
	"time"
)

// Experience table with ExperienceMultiplierConstA = 1: exp[L] = (L+9)*L*L
// (10, 44, 108, 208 ...).
func TestLevelUpSingleLevel(t *testing.T) {
	b := newBareWorld(t)
	o := statsPlayer(b, PlayerStart, 1, 1)
	b.calcCharacter(o)
	o.P.Char.Life = 1
	b.drain(o)
	if o.P.NextExp != 10 || o.P.Char.Experience != 0 {
		t.Fatalf("next %d exp %d", o.P.NextExp, o.P.Char.Experience)
	}
	// Below the next level: only added.
	if b.levelUp(o, 7, 1) || o.P.Char.Experience != 7 || o.P.Char.Level != 1 {
		t.Fatalf("exp %d level %d", o.P.Char.Experience, o.P.Char.Level)
	}
	// MaxLevelUp = 1: the excess is dropped and Experience = exp[1].
	if !b.levelUp(o, 30, 1) {
		t.Fatal("no level up")
	}
	c := o.P.Char
	if c.Level != 2 || c.Experience != 10 || o.P.NextExp != 44 || c.LevelUpPoints != 1 {
		t.Fatalf("level %d exp %d next %d points %d", c.Level, c.Experience, o.P.NextExp, c.LevelUpPoints)
	}
	if c.MaxLife != 112 || c.Life != 112 || c.Mana != c.MaxMana {
		t.Fatalf("life %v/%v mana %v/%v", c.Life, c.MaxLife, c.Mana, c.MaxMana)
	}
	p := find(b.drain(o), 0xF3, 0x05)
	if len(p) != 0x30 {
		t.Fatalf("F3 05 % X", p)
	}
	u16 := func(off int) int { return int(binary.LittleEndian.Uint16(p[off:])) }
	u32 := func(off int) int { return int(binary.LittleEndian.Uint32(p[off:])) }
	if u16(4) != 2 || u16(6) != 1 || u16(8) != 112 || u16(10) != 20 || u32(24) != 1 || u32(28) != 112 ||
		u32(40) != 10 || u32(44) != 44 || u16(12) != u32(36) {
		t.Fatalf("F3 05 % X", p)
	}
}

func TestLevelUpMultipleLevels(t *testing.T) {
	b := newBareWorld(t)
	o := statsPlayer(b, PlayerStart, 0, 1)
	b.calcCharacter(o)
	// 200 exp with up to 3 levels: 0->10 (L2), 10->44 (L3), then L4 takes
	// the rest and Experience = exp[3] = 108 (next 208).
	b.levelUp(o, 200, 3)
	c := o.P.Char
	if c.Level != 4 || c.Experience != 108 || o.P.NextExp != 208 || c.LevelUpPoints != 3 {
		t.Fatalf("level %d exp %d next %d points %d", c.Level, c.Experience, o.P.NextExp, c.LevelUpPoints)
	}
}

func TestLevelUpPlusStatAndMaxLevel(t *testing.T) {
	b := newBareWorld(t)
	o := statsPlayer(b, PlayerStart, 1, 220)
	b.calcCharacter(o)
	// Marlon quest done (quest[0] bits 4-5 = 2): +PlusStatPoint above
	// PlusStatMinLevel 220.
	o.P.Char.Quest = []byte{0x20}
	b.levelUp(o, int64(o.P.NextExp), 1)
	if o.P.Char.Level != 221 || o.P.Char.LevelUpPoints != 2 {
		t.Fatalf("level %d points %d", o.P.Char.Level, o.P.Char.LevelUpPoints)
	}
	// MaxLevel 400: message 0x10a and no change.
	o.P.Char.Level = 400
	b.drain(o)
	if !b.levelUp(o, 1000, 1) || o.P.Char.Level != 400 {
		t.Fatal("max level")
	}
	if find(b.drain(o), 0x0D, -1) == nil {
		t.Fatal("no max level notice")
	}
}

func TestGainExperienceHookSavesAtMostEveryMinute(t *testing.T) {
	b := newBareWorld(t)
	o := statsPlayer(b, PlayerStart, 1, 1)
	b.calcCharacter(o)
	b.hooks.gainExperience(b.World, o, 10)
	first := b.statsOf(o).lastLevelSave
	if first.IsZero() || o.P.Char.Level != 2 {
		t.Fatalf("level %d save %v", o.P.Char.Level, first)
	}
	b.advance(30 * time.Second)
	b.hooks.gainExperience(b.World, o, 100)
	if b.statsOf(o).lastLevelSave != first {
		t.Fatal("saved twice within 60 s")
	}
}

// Party experience (FUN_004c5ea0) for a level-10 monster killed by a DW 10
// and a DK 20 party (2 classes: PartyGeneralExperience2 = 80):
//
//	rate = 80*2 = 160; total level 30; base (35*10)/3 = 116, +116/4 = 145
//	DW: (10*145*160)/30/100 = 77 * AddExperienceRate_AL0 500 = 38500
//	DK: (20*145*160)/30/100 = 154 * 500 = 77000
func TestPartyExperienceSplit(t *testing.T) {
	b := newBareWorld(t)
	dw := statsPlayer(b, PlayerStart, 0, 10)
	dk := statsPlayer(b, PlayerStart+1, 1, 20)
	b.calcCharacter(dw)
	b.calcCharacter(dk)
	dk.X, dk.Y = dw.X+1, dw.Y
	if !b.partyCreate(dw) || !b.partyAdd(b.partyOf(dw), dk) {
		t.Fatal("party")
	}
	mon := b.objects[100]
	mon.Type, mon.Map, mon.X, mon.Y = TypeMonster, 0, dw.X, dw.Y+1
	mon.Mon = &monsterState{Level: 10, MaxLife: 100}
	b.drain(dw)
	b.drain(dk)
	dkExp := dk.P.Char.Experience
	if !b.hooks.partyExperience(b.World, dw, mon, 55, false) {
		t.Fatal("party hook")
	}
	// DW 10: 1458 + 38500 >= 1900 -> level 11 (MaxLevelUp 1), shown exp 0.
	if dw.P.Char.Level != 11 || dw.P.Char.Experience != 1900 {
		t.Fatalf("dw level %d exp %d", dw.P.Char.Level, dw.P.Char.Experience)
	}
	p := find(b.drain(dw), 0x9C, -1)
	if p == nil || p[0] != 0xC3 || int(p[3])<<8|int(p[4]) != 100 || p[6]|p[7]|p[8]|p[9] != 0 || p[11] != 55 {
		t.Fatalf("dw 9C % X", p)
	}
	// DK 20: exp[19] = 28*361 = 10108; +77000 >= exp[20] = 11600 -> level 21.
	if dk.P.Char.Level != 21 || dkExp != 10108 {
		t.Fatalf("dk level %d exp %d", dk.P.Char.Level, dkExp)
	}
	if mon.Mon.ExpGiven != 38500/2+77000/2 {
		t.Fatalf("monster exp given %d", mon.Mon.ExpGiven)
	}
	// Small amounts at levels 100/110: only added, shown in 9C.
	dw.P.Char.Level, dk.P.Char.Level = 100, 110
	dw.P.Char.Experience, dk.P.Char.Experience = 0, 0
	b.expBounds(dw)
	b.expBounds(dk)
	mon.Mon.Level = 15
	b.drain(dw)
	b.hooks.partyExperience(b.World, dw, mon, 3, true)
	p = find(b.drain(dw), 0x9C, -1)
	// base (40*15)/3 = 200, average level 105 > 25: 25*200/105 = 47 -> 58;
	// DW (100*58*160)/210/100 = 44 * 500 = 22000.
	if p == nil || p[3]&0x80 == 0 || int(p[6])<<24|int(p[7])<<16|int(p[9])<<8|int(p[8]) != 22000 ||
		dw.P.Char.Experience != int64(b.data.Experience.Floor(100))+22000 {
		t.Fatalf("small 9C % X", p)
	}
	// Out of range members get nothing; no party -> false.
	dk.X += 20
	before := dk.P.Char.Experience
	b.hooks.partyExperience(b.World, dw, mon, 3, false)
	if dk.P.Char.Experience != before {
		t.Fatal("distant member got exp")
	}
	solo := statsPlayer(b, PlayerStart+2, 0, 10)
	if b.hooks.partyExperience(b.World, solo, mon, 1, false) {
		t.Fatal("solo player handled as party")
	}
}
