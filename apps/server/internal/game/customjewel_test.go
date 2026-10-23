package game

import (
	"testing"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/item"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"
)

const testJewel = 14*512 + 17 // Devil's Eye used as a custom jewel

// customJewelWorld installs one CustomJewel.txt row on a bare world.
func customJewelWorld(t *testing.T) (*bareWorld, *gamedata.CustomJewel) {
	t.Helper()
	b := newBareWorld(t)
	j := &b.cw2().jewels.Slots[0]
	*j = gamedata.CustomJewel{Index: 0, ItemIndex: testJewel, Weapon: 1, Armor: 0, Wing: 0,
		Rate: [4]int{100, 100, 100, 100}, SalePrice: 1234,
		MinLevel: -1, MaxLevel: 12, Skill: -1, Luck: -1, MinOption: -1, MaxOption: -1, MinExc: -1, MaxExc: -1,
		MaxSetOption: -1,
		Success:      gamedata.JewelChange{Level: 2, Skill: 1, Luck: 1, Option: 3, Excellent: 2},
		Failure:      gamedata.JewelChange{Level: 1, Skill: 1, Luck: 1, Option: 1, Excellent: 6, SetOption: 1}}
	return b, j
}

// useJewelOn puts the jewel in the first bag slot, the target in the
// second, and sends C1 06 26 src dst 00.
func useJewelOn(b *bareWorld, o *Object, target item.Item) (src, dst int) {
	src, dst = BagStart, BagStart+2
	o.P.Inventory[src] = newItem(testJewel, 0)
	o.P.Inventory[dst] = target
	b.handleUseItem(o, []byte{0xC1, 6, 0x26, byte(src), byte(dst), 0})
	return src, dst
}

func TestCustomJewelTables(t *testing.T) {
	b, j := customJewelWorld(t)
	if !b.jewelItem(testJewel) || !b.jewelItem(14*512+13) || b.jewelItem(14*512+18) {
		t.Fatal("jewel test")
	}
	if p := b.itemPrice(newItem(testJewel, 0)); p.Buy != roundPrice(1234, 99) || p.Sell != roundPrice(1234/3, 99) {
		t.Fatalf("price %+v", p)
	}
	// FUN_004650b0 needs Index == slot position.
	j.Index = 1
	if b.jewelItem(testJewel) {
		t.Fatal("slot index mismatch accepted")
	}
}

func TestCustomJewelSuccess(t *testing.T) {
	b, _ := customJewelWorld(t)
	o := b.player(PlayerStart, "Smith", 0, 130, 130)
	kris := newItem(0, 0)
	kris.Durability = 20
	src, dst := useJewelOn(b, o, kris)
	pk := b.drain(o)
	if !o.P.Inventory[src].IsEmpty() || find(pk, 0x28, -1) == nil || find(pk, 0xF3, 0x14) == nil {
		t.Fatal("jewel not consumed / target not sent")
	}
	got := o.P.Inventory[dst]
	if got.Level != 2 || !got.Skill || !got.Luck || got.Option != 3 || countExcellent(got.Excellent) != 2 {
		t.Fatalf("target %+v", got)
	}
	// The option is capped by MaxItemOption (7), the level by MaxItemLevel (13).
	o.P.Inventory[dst].Option, o.P.Inventory[dst].Level = 6, 12
	o.P.Inventory[src] = newItem(testJewel, 0)
	b.handleUseItem(o, []byte{0xC1, 6, 0x26, byte(src), byte(dst), 0})
	if got := o.P.Inventory[dst]; got.Level != 13 || got.Option != 7 || countExcellent(got.Excellent) != 4 {
		t.Fatalf("capped %+v", got)
	}
}

func TestCustomJewelFailure(t *testing.T) {
	b, j := customJewelWorld(t)
	j.Rate = [4]int{}
	o := b.player(PlayerStart, "Smith", 0, 130, 130)
	kris := newItem(0, 5)
	kris.Skill, kris.Luck, kris.Option, kris.Excellent = true, true, 3, 0x21
	src, dst := useJewelOn(b, o, kris)
	got := o.P.Inventory[dst]
	if !o.P.Inventory[src].IsEmpty() || got.Level != 4 || got.Skill || got.Luck || got.Option != 2 || got.Excellent != 0 {
		t.Fatalf("failure %+v", got)
	}
}

func TestCustomJewelRequirements(t *testing.T) {
	b, j := customJewelWorld(t)
	o := b.player(PlayerStart, "Smith", 0, 130, 130)
	refused := func(target item.Item) {
		t.Helper()
		src, dst := useJewelOn(b, o, target)
		if o.P.Inventory[src].IsEmpty() || o.P.Inventory[dst] != target {
			t.Fatalf("accepted %+v", target)
		}
		b.drain(o)
	}
	refused(newItem(0, 13))     // MaxLevel 12
	refused(newItem(8*512, 0))  // armour without EnableSlotArmor
	refused(newItem(12*512, 0)) // wings without EnableSlotWing
	refused(newItem(13*512, 0)) // a pet is never a target
	refused(newItem(0x807, 0))  // bolts
	periodic := newItem(0, 0)
	periodic.Flag1 = true
	refused(periodic)
	j.MaxExc = 10 + 0x20 // the bit 0x20 is forbidden
	exc := newItem(0, 0)
	exc.Excellent = 0x21
	refused(exc)
	j.MaxExc = 1 // at most one option
	exc.Excellent = 0x03
	refused(exc)
	j.MaxExc, j.MinExc = -1, 1
	refused(newItem(0, 0))
	j.MinExc, j.Skill = -1, 0 // must not have a skill
	sk := newItem(0, 0)
	sk.Skill = true
	refused(sk)
	j.Skill, j.MaxSetOption = -1, 0
	anc := newItem(0, 0)
	anc.SetOption = 5
	refused(anc)
}

func TestCustomJewelSetOption(t *testing.T) {
	b, j := customJewelWorld(t)
	idx, tier := -1, -1
	for i, r := range b.dt().setTypes.ByIndex {
		if i < 0xc00 && r.Option[0] > 0 && r.Option[1] < 1 {
			idx, tier = i, 0
			break
		}
	}
	if idx < 0 {
		t.Skip("no weapon with only a tier 1 set in SetItemType.txt")
	}
	j.Success = gamedata.JewelChange{SetOption: 1 | 2 | 4 | 8}
	j.MaxSetOption = 1
	o := b.player(PlayerStart, "Smith", 0, 130, 130)
	_, dst := useJewelOn(b, o, newItem(idx, 0))
	if got := o.P.Inventory[dst].SetOption; got != byte(tier+1)+8 {
		t.Fatalf("set option %d", got)
	}
	// Tier 2 is required but the item has none: refused.
	j.MaxSetOption = 2
	src, dst := useJewelOn(b, o, newItem(idx, 0))
	if o.P.Inventory[src].IsEmpty() || o.P.Inventory[dst].SetOption != 0 {
		t.Fatal("tier 2 requirement")
	}
	// Failure with SetOption > 0 removes it.
	j.MaxSetOption, j.Rate = -1, [4]int{}
	anc := newItem(idx, 0)
	anc.SetOption = 5
	_, dst = useJewelOn(b, o, anc)
	if o.P.Inventory[dst].SetOption != 0 {
		t.Fatal("set kept on failure")
	}
}

func TestCustomJewelInPick(t *testing.T) {
	b, _ := customJewelWorld(t)
	// The auto pick uses the same jewel test (U-CUST-22).
	if !b.jewelItem(testJewel) {
		t.Fatal("custom jewel not a jewel")
	}
	// Without the table entry the fixed list only.
	b.cw2().jewels.Slots[0].Index = -1
	if b.jewelItem(testJewel) || !b.jewelItem(14*512+161) {
		t.Fatal("fixed list")
	}
}
