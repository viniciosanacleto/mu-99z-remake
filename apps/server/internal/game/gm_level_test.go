package game

import (
	"testing"
)

// TestGMLevelFromCharacter: the GM level comes from the character
// (characters.gm_level) and entering the world copies it to Authority, so
// a level changed in the database applies on the next login.
func TestGMLevelFromCharacter(t *testing.T) {
	b := newBareWorld(t)
	o := b.player(PlayerStart, "Admin", 0, 130, 130)
	if b.gmLevel(o) != 0 {
		t.Fatalf("plain player level %d", b.gmLevel(o))
	}
	o.P.Char.GMLevel = 32
	for _, h := range b.hooks.enterWorld {
		h(b.World, o)
	}
	b.wg.Wait()
	for len(b.events) > 0 {
		(<-b.events)()
	}
	if b.gmLevel(o) != 32 || mv(o).Authority != 32 {
		t.Fatalf("level %d authority %d", b.gmLevel(o), mv(o).Authority)
	}
}

// TestGMFull: the original's Authority == 8 privileges go to every level
// from 8 up (32 is a superset of 8); lower levels keep none.
func TestGMFull(t *testing.T) {
	b := newBareWorld(t)
	o := b.player(PlayerStart, "Gm", 0, 130, 130)
	if gmFull(&Object{}) {
		t.Fatal("object without movement state")
	}
	for lv, want := range map[int]bool{0: false, 1: false, 2: false, 4: false, 8: true, 32: true} {
		mv(o).Authority = lv
		if gmFull(o) != want {
			t.Fatalf("level %d gmFull %v", lv, !want)
		}
	}
}

// TestLevel32HasLevel8Privileges goes through the call sites: PvP target
// immunity, no pushback, /superdrop and /hide.
func TestLevel32HasLevel8Privileges(t *testing.T) {
	for _, lv := range []int{8, 32} {
		b := newBareWorld(t)
		a, v := pvpPair(t, b, 0, 1)
		mv(v).Authority = lv
		if b.pvpAllowed(a, v) {
			t.Fatalf("level %d: PvP target allowed", lv)
		}
		if b.pushAllowed(v, false) {
			t.Fatalf("level %d: pushed", lv)
		}
		v.P.Char.GMLevel = lv
		b.runCommand(v, "/superdrop")
		if !b.superDropOn() {
			t.Fatalf("level %d: /superdrop refused", lv)
		}
		b.runCommand(v, "/hide")
		if !gmHidden(v) {
			t.Fatalf("level %d: /hide did not hide", lv)
		}
		mv(v).Authority = 0
		if !b.pvpAllowed(a, v) || !b.pushAllowed(v, false) || gmHidden(v) {
			t.Fatalf("level %d: privileges kept at level 0", lv)
		}
	}
}
