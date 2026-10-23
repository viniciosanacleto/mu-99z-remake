package game

import (
	"context"
	"testing"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/item"
)

func TestCAtoi(t *testing.T) {
	for s, want := range map[string]int{"12": 12, " 7x": 7, "-3": -3, "+4": 4, "abc": 0, "": 0} {
		if got := cAtoi(s); got != want {
			t.Fatalf("cAtoi(%q) = %d", s, got)
		}
	}
	if cmdInt("auto 25", 1) != 25 || cmdInt("auto 25", 0) != 0 {
		t.Fatal("cmdInt")
	}
}

func TestCommandAddPoint(t *testing.T) {
	b := newBareWorld(t)
	o := b.player(PlayerStart, "Adder", 0, 130, 130)
	ch := o.P.Char
	ch.Strength, ch.LevelUpPoints = 20, 50
	b.runCommand(o, "/f 30")
	if ch.Strength != 50 || ch.LevelUpPoints != 20 {
		t.Fatalf("str %d points %d", ch.Strength, ch.LevelUpPoints)
	}
	pk := b.drain(o)
	if find(pk, 0xF3, 0xE0) == nil {
		t.Fatal("no F3 E0 refresh")
	}
	if _, ok := mv(o).CommandTicks[5]; !ok {
		t.Fatal("command not marked done")
	}
	// More than the free points: message 0x145, nothing changes.
	b.runCommand(o, "/a 21")
	if ch.Dexterity != 0 || ch.LevelUpPoints != 20 {
		t.Fatalf("dex %d points %d", ch.Dexterity, ch.LevelUpPoints)
	}
	// /c is for the Dark Lord only.
	b.runCommand(o, "/c 5")
	if ch.Leadership != 0 {
		t.Fatal("leadership for a non-DL")
	}
	o.Class = 4
	b.runCommand(o, "/c 5")
	if ch.Leadership != 5 || ch.LevelUpPoints != 15 {
		t.Fatalf("lead %d points %d", ch.Leadership, ch.LevelUpPoints)
	}
}

func TestCommandAddPointAuto(t *testing.T) {
	b := newBareWorld(t)
	o := b.player(PlayerStart, "Auto", 0, 130, 130)
	ch := o.P.Char
	ch.LevelUpPoints = 0
	b.runCommand(o, "/f auto 7")
	b.runCommand(o, "/v auto 100")
	a := o.P.autoPoints
	if a == nil || a.Count != 2 || a.Amount[0] != 7 || a.Amount[2] != 100 {
		t.Fatalf("auto %+v", a)
	}
	ch.Strength, ch.Vitality = 10, 10
	ch.LevelUpPoints = 15
	b.autoPointSecond()
	// 15 points over 2 types: strength takes min(ceil(15/2), 7) = 7 and is
	// done; vitality takes ceil(8/1) = 8.
	if ch.Strength != 17 || ch.Vitality != 18 || ch.LevelUpPoints != 0 || a.Amount[0] != 0 || a.Amount[2] != 92 || a.Count != 1 {
		t.Fatalf("str %d vit %d points %d auto %+v", ch.Strength, ch.Vitality, ch.LevelUpPoints, a)
	}
	b.runCommand(o, "/v auto 0")
	if a.Count != 0 || a.Amount[2] != 0 {
		t.Fatalf("off %+v", a)
	}

	// The cap: MaxStatPoint_AL stops the stat and clears its amount.
	maxStat := b.cfg.ALInt("Common", "MaxStatPoint", 0, 0)
	a.Amount, a.Count = [5]int{0, 0, 0, 50, 0}, 1
	ch.Energy, ch.LevelUpPoints = maxStat-3, 40
	b.autoPointSecond()
	if ch.Energy != maxStat || ch.LevelUpPoints != 37 || a.Amount[3] != 0 || a.Count != 0 {
		t.Fatalf("ene %d points %d auto %+v", ch.Energy, ch.LevelUpPoints, a)
	}
}

func TestCommandPKClear(t *testing.T) {
	b := newBareWorld(t)
	o := b.player(PlayerStart, "Killer", 0, 130, 130)
	ch := o.P.Char
	o.PKLevel = 3
	b.runCommand(o, "/pk")
	if find(b.drain(o), 0xF3, 0x08) != nil {
		t.Fatal("cleared a commoner")
	}
	o.PKLevel, ch.PKLevel, ch.PKCount, ch.PKTime = 5, 5, 4, 1000
	b.runCommand(o, "/pk")
	if o.PKLevel != 3 || ch.PKCount != 0 || ch.PKTime != 0 {
		t.Fatalf("pk %d count %d time %d", o.PKLevel, ch.PKCount, ch.PKTime)
	}
	if p := find(b.drain(o), 0xF3, 0x08); p == nil || p[6] != 3 {
		t.Fatalf("F3 08 %x", p)
	}
}

func TestCommandMoney(t *testing.T) {
	b := newBareWorld(t)
	o := b.player(PlayerStart, "Rich", 0, 130, 130)
	ch := o.P.Char
	ch.Money = 100
	b.runCommand(o, "/zen 50")
	if ch.Money != 150 {
		t.Fatalf("money %d", ch.Money)
	}
	mv(o).CommandTicks = nil
	ch.Money = 1999999990
	b.runCommand(o, "/zen 50")
	if ch.Money != 2000000000 {
		t.Fatalf("cap %d", ch.Money)
	}
	mv(o).CommandTicks = nil
	b.runCommand(o, "/zen 0")
	if ch.Money != 0 {
		t.Fatalf("zero %d", ch.Money)
	}
	if p := find(b.drain(o), 0x22, -1); p == nil {
		t.Fatal("no money packet")
	}
}

func TestCommandClearInventory(t *testing.T) {
	b := newBareWorld(t)
	o := b.player(PlayerStart, "Messy", 0, 130, 130)
	o.P.Inventory[0] = item.Item{Index: 0, Durability: 10}
	o.P.Inventory[BagStart] = item.Item{Index: 0x1C0D, Durability: 1}
	o.P.Inventory[BagStart+1] = item.Item{Index: 0x1A13, Durability: 1}
	o.P.Inventory[BagEnd-1] = item.Item{Index: 0x1C40, Durability: 1}
	b.runCommand(o, "/clearinv")
	if !o.P.Inventory[BagStart].IsEmpty() || o.P.Inventory[BagStart+1].IsEmpty() ||
		o.P.Inventory[BagEnd-1].IsEmpty() || o.P.Inventory[0].IsEmpty() {
		t.Fatal("wrong slots cleared")
	}
	n := 0
	for _, p := range b.drain(o) {
		if len(p) > 2 && p[2] == 0x28 {
			n++
		}
	}
	if n != BagEnd-BagStart-2 {
		t.Fatalf("%d delete packets", n)
	}
}

func TestCommandRedistribute(t *testing.T) {
	b := newBareWorld(t)
	o := b.player(PlayerStart, "Readd", 0, 130, 130)
	o.Class = 4
	ch := o.P.Char
	ci := b.statsData.classes[4]
	ch.Strength, ch.Dexterity, ch.Vitality, ch.Energy, ch.Leadership = ci.Strength+10, ci.Dexterity+20, ci.Vitality+30, ci.Energy+40, ci.Lead+50
	ch.LevelUpPoints = 5
	b.runCommand(o, "/readd")
	if ch.Strength != ci.Strength || ch.Leadership != ci.Lead || ch.LevelUpPoints != 155 {
		t.Fatalf("str %d lead %d points %d", ch.Strength, ch.Leadership, ch.LevelUpPoints)
	}
	// Other classes keep their leadership.
	o.Class = 0
	ch.Leadership = 9
	mv(o).CommandTicks = nil
	b.runCommand(o, "/readd")
	if ch.Leadership != 9 {
		t.Fatal("leadership reset for a non-DL")
	}
}

// FUN_00475c30 (the DataServer 0x30 character save) follows /readd.
func TestCommandRedistributeSaves(t *testing.T) {
	b := newBareWorld(t)
	ctx := context.Background()
	acc, err := b.store.CreateAccount(ctx, "readdacc", "x", "1234567890123")
	if err != nil {
		t.Fatal(err)
	}
	b.store.CreateCharacter(ctx, acc.ID, "Saved", 0x00)
	ch, err := b.store.LoadCharacter(ctx, acc.ID, "Saved")
	if err != nil {
		t.Fatal(err)
	}
	o := b.player(PlayerStart, "Saved", 0, 130, 130)
	o.P.Char = ch
	ch.Strength += 25
	b.runCommand(o, "/readd")
	b.runAsync()
	got, _ := b.store.LoadCharacter(ctx, acc.ID, "Saved")
	if got.Strength != b.statsData.classes[0].Strength || got.LevelUpPoints != ch.LevelUpPoints || got.LevelUpPoints < 25 {
		t.Fatalf("saved str %d points %d", got.Strength, got.LevelUpPoints)
	}
}
