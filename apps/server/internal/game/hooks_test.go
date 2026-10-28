package game

import (
	"reflect"
	"testing"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/item"
)

func TestNewHooksBuildsIndependentTables(t *testing.T) {
	a, b := newHooks(), newHooks()
	if a.worldHandlers[0x22] == nil || a.commandHandlers[0] == nil {
		t.Fatal("core handlers not registered (22 pickup, command 0 /move)")
	}
	a.worldHandlers[0xFF] = func(*World, *Object, []byte) {}
	a.leaveWorld = append(a.leaveWorld, func(*World, *Object) {})
	if b.worldHandlers[0xFF] != nil {
		t.Fatal("worldHandlers shared between tables")
	}
	if len(b.leaveWorld) == len(a.leaveWorld) {
		t.Fatal("leaveWorld shared between tables")
	}
}

// FUN_004e1350 runs before every other leave step (reconnect.go prepends
// it after effect_save.go prepended its own), so the order is behavior.
func TestNewHooksLeaveWorldStartsWithReconnectSave(t *testing.T) {
	h := newHooks()
	if len(h.leaveWorld) == 0 {
		t.Fatal("no leaveWorld hooks")
	}
	got := reflect.ValueOf(h.leaveWorld[0]).Pointer()
	want := reflect.ValueOf((*World).reconnectSave).Pointer()
	if got != want {
		t.Fatal("leaveWorld[0] is not reconnectSave")
	}
}

func TestWorldsDoNotShareHooks(t *testing.T) {
	b1, b2 := newBareWorld(t), newBareWorld(t)
	b1.hooks.dropMapItem = func(*World, int, int, int, item.Item, int) bool { return false }
	if b1.hooks == b2.hooks {
		t.Fatal("worlds share one hooks table")
	}
	o := b2.player(PlayerStart, "Dropper", 0, 130, 130)
	if !b2.hooks.dropMapItem(b2.World, 0, o.X, o.Y, mkItem(14, 0, 0, 1), -1) {
		t.Fatal("stub on the first world reached the second")
	}
	// Shop NPC talks are registered on every world, not only the first one
	// that loaded ShopManager.txt.
	for i, b := range []*bareWorld{b1, b2} {
		if b.hooks.npcTalk[253] == nil {
			t.Fatalf("world %d: shop NPC 253 has no talk handler", i+1)
		}
	}
}
