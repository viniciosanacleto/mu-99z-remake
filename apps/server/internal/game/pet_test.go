package game

import (
	"bytes"
	"context"
	"testing"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/item"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/store"
)

func TestPetExpTable(t *testing.T) {
	tb := petExpTable(1)
	// FUN_0050ad00: levels below 3 are 0, then (L+10)*L^3.
	want := map[int]uint32{0: 0, 1: 0, 2: 0, 3: 351, 4: 896, 10: 20000, 50: 7500000}
	for l, v := range want {
		if tb[l] != v {
			t.Errorf("pet exp[%d] = %d, want %d", l, tb[l], v)
		}
	}
	if t2 := petExpTable(2); t2[10] != 40000 {
		t.Errorf("ConstA 2: %d", t2[10])
	}
}

func TestPetInitAndExperience(t *testing.T) {
	b := newBareWorld(t)
	horse := item.Item{Index: idxDarkHorse, Durability: 255}
	// FUN_0048d010: level capped by MaxPetLevel (50), experience clamped.
	b.petInit(&horse, 70, 0)
	if horse.PetLevel != 50 || horse.PetExp != 7500000 {
		t.Fatalf("init 70 = %d/%d", horse.PetLevel, horse.PetExp)
	}
	b.petInit(&horse, 3, 5000)
	if horse.PetLevel != 3 || horse.PetExp != 351 {
		t.Fatalf("init exp above the level = %d/%d", horse.PetLevel, horse.PetExp)
	}
	// FUN_0048db00 with PetExperienceRateDivisor 10: 10000 -> 1000.
	if !b.petAddExp(&horse, 10000) || horse.PetLevel != 4 || horse.PetExp != 896 {
		t.Fatalf("level up = %d/%d", horse.PetLevel, horse.PetExp)
	}
	if b.petAddExp(&horse, 100) || horse.PetExp != 906 {
		t.Fatalf("plain gain = %d", horse.PetExp)
	}
	// FUN_0048dc80: losing below the floor goes one level down.
	if !b.petSubExp(&horse, 200) || horse.PetLevel != 3 || horse.PetExp != 895 {
		t.Fatalf("level down = %d/%d", horse.PetLevel, horse.PetExp)
	}
	// Broken pets and other items gain nothing; uninitialized pets become 1/0.
	broken := item.Item{Index: idxDarkRaven}
	if b.petAddExp(&broken, 1000) {
		t.Fatal("broken pet gained")
	}
	fresh := item.Item{Index: idxDarkRaven, Durability: 1}
	if !b.petAddExp(&fresh, 1000) || fresh.PetLevel != 1 || fresh.PetExp != 0 {
		t.Fatalf("fresh = %+v", fresh)
	}
	// FUN_0048cf60 prices.
	if p, ok := petPrice(item.Item{Index: idxDarkHorse, PetLevel: 3}); !ok || p.Buy != 6000000 || p.Sell != 2000000 {
		t.Fatalf("horse price = %+v", p)
	}
	if p, _ := petPrice(item.Item{Index: idxDarkRaven, PetLevel: 1}); p.Buy != 1000000 || p.Sell != 333300 {
		t.Fatalf("raven price = %+v", p)
	}
}

func TestPetKillExperienceAndInfo(t *testing.T) {
	b := newBareWorld(t)
	o := statsPlayer(b, PlayerStart, 4, 400)
	o.P.Inventory[1] = item.Item{Index: idxDarkRaven, Durability: 255, PetLevel: 3, PetExp: 351}
	if b.hooks.petExperience == nil {
		t.Fatal("hooks.petExperience not set")
	}
	b.hooks.petExperience(b.World, o, 100)
	a9 := find(b.drain(o), 0xA9, -1)
	if !bytes.Equal(a9, []byte{0xC1, 0x0C, 0xA9, 0, 0, 1, 3, 0, 0x69, 0x01, 0, 0}) {
		t.Fatalf("A9 = % X", a9)
	}
	// A level up the owner cannot equip (raven +4 needs 245 leadership)
	// is undone: level 3, experience just below level 4, flag 0.
	o.P.Char.Leadership = 100
	b.hooks.petExperience(b.World, o, 6000)
	a9 = find(b.drain(o), 0xA9, -1)
	if a9 == nil || a9[4] != 0 || a9[6] != 3 || o.P.Inventory[1].PetExp != 895 {
		t.Fatalf("A9 undone level up = % X", a9)
	}
	// With enough leadership the level up stays and is flagged 0xFE.
	o.P.Char.Leadership = 300
	b.hooks.petExperience(b.World, o, 10)
	a9 = find(b.drain(o), 0xA9, -1)
	if a9 == nil || a9[4] != 0xFE || a9[6] != 4 {
		t.Fatalf("A9 level up = % X (req %+v)", a9, b.requirements(o.P.Inventory[1]))
	}
	// A9 request: inventory slot 1.
	b.handlePetInfo(o, []byte{0xC1, 6, 0xA9, 0, 0, 1})
	if r := find(b.drain(o), 0xA9, -1); r == nil || r[5] != 1 || r[6] != 4 {
		t.Fatalf("A9 answer = % X", r)
	}
	// Chaos box (4) and bad positions.
	chaos(o).Box[2] = item.Item{Index: idxDarkHorse, PetLevel: 9}
	b.handlePetInfo(o, []byte{0xC1, 6, 0xA9, 1, 4, 2})
	if r := find(b.drain(o), 0xA9, -1); r == nil || r[3] != 1 || r[4] != 4 || r[6] != 9 {
		t.Fatalf("A9 chaos box = % X", r)
	}
	b.handlePetInfo(o, []byte{0xC1, 6, 0xA9, 0, 0, 108})
	if r := find(b.drain(o), 0xA9, -1); r != nil {
		t.Fatal("A9 for slot 108 answered")
	}
}

func TestPetPersistence(t *testing.T) {
	b := newBareWorld(t)
	ps := b.store.(store.PetStore)
	ctx := context.Background()
	if err := ps.SavePetInfo(ctx, []store.PetInfo{{Serial: 900, Level: 12, Exp: 30000}}); err != nil {
		t.Fatal(err)
	}
	o := statsPlayer(b, PlayerStart, 4, 400)
	o.P.Inventory[8] = item.Item{Index: idxDarkHorse, Serial: 900, Durability: 255}
	o.P.Inventory[BagStart] = item.Item{Index: idxDarkRaven, Serial: 901, Durability: 255}
	b.requestPetInfo(o, 0)
	b.pump()
	tb := b.petTable()
	if h := o.P.Inventory[8]; h.PetLevel != 12 || h.PetExp != tb[12] {
		t.Fatalf("horse = %d/%d (floor %d)", h.PetLevel, h.PetExp, tb[12])
	}
	if r := o.P.Inventory[BagStart]; r.PetLevel != 1 || r.PetExp != 0 {
		t.Fatalf("new raven = %d/%d", r.PetLevel, r.PetExp)
	}
	// The character save also saves the pets (DS 0x34).
	o.P.Inventory[BagStart].PetLevel = 5
	o.P.Inventory[BagStart].PetExp = tb[5]
	b.saveCharacter(o, nil)
	b.pump()
	got, _ := ps.LoadPetInfo(ctx, []uint32{901})
	if got[0].Level != 5 || got[0].Exp != tb[5] {
		t.Fatalf("saved = %+v", got)
	}
}

func TestDarkRavenCommand(t *testing.T) {
	b := newBareWorld(t)
	o := statsPlayer(b, PlayerStart, 4, 400)
	// Without an active raven (FUN_0046b0c0 never set +0x14) A7 is ignored.
	b.handlePetCommand(o, []byte{0xC1, 7, 0xA7, 0, 1, 0, 0})
	if r := find(b.drain(o), 0xA7, -1); r != nil {
		t.Fatal("A7 answered without a raven")
	}
	o.P.Char.Leadership = 2000
	o.P.Inventory[1] = item.Item{Index: idxDarkRaven, Durability: 255, PetLevel: 10}
	b.calcCharacter(o)
	b.drain(o)
	b.handlePetCommand(o, []byte{0xC1, 7, 0xA7, 0, 1, 0, 0})
	if r := find(b.drain(o), 0xA7, -1); !bytes.Equal(r, []byte{0xC1, 7, 0xA7, 0, 1, 0, 0}) {
		t.Fatalf("A7 mode 1 = % X", r)
	}
	b.handlePetCommand(o, []byte{0xC1, 7, 0xA7, 0, 1, 0, 0})
	if r := find(b.drain(o), 0xA7, -1); r != nil {
		t.Fatal("unchanged mode answered")
	}
	b.handlePetCommand(o, []byte{0xC1, 7, 0xA7, 0, 3, 0x12, 0x34})
	if r := find(b.drain(o), 0xA7, -1); !bytes.Equal(r, []byte{0xC1, 7, 0xA7, 0, 3, 0x12, 0x34}) || pet(o).Target != 0x1234 {
		t.Fatalf("A7 mode 3 = % X", r)
	}
	// Other classes are ignored.
	k := statsPlayer(b, PlayerStart+1, 1, 400)
	b.handlePetCommand(k, []byte{0xC1, 7, 0xA7, 0, 2, 0, 0})
	if r := find(b.drain(k), 0xA7, -1); r != nil {
		t.Fatal("DK commanded a raven")
	}
	// FUN_0046b0c0 stats for a level 10 raven (Character.dat constants),
	// copied from CalcCharacter.
	lead := o.P.Char.Leadership + b.statsOf(o).AddLead
	ps := pet(o)
	if !ps.Owner || ps.DamageMin != lead/8+15*10+180 || ps.DamageMax != lead/4+15*10+200 ||
		ps.AttackSpeed != 4*10/5+20+lead/50 || ps.SuccessRate != 1000+16*10/1 {
		t.Fatalf("raven stats = %+v (lead %d)", ps, lead)
	}
}
