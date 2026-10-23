package game

import (
	"testing"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/item"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"
)

func TestDropEventDrop(t *testing.T) {
	b := newBareWorld(t)
	p := b.player(PlayerStart, "Looter", 0, 130, 130)
	m := b.monsterNear(0, p, 1)
	running := &timedEventState{data: &gamedata.TimedEvent{}, State: invStart}
	idle := &timedEventState{data: &gamedata.TimedEvent{Index: 1}, State: invEmpty}
	b.ev().drop = &dropEventManager{events: []*timedEventState{running, idle}, items: []gamedata.DropEventItem{
		{Event: 1, Index: 14*512 + 13, Option: [6]int{-1, -1, -1, -1, -1, -1}, Map: -1, Monster: -1, MinLevel: -1, MaxLevel: -1, AccountLevel: -1, Rate: -1},
		{Event: 0, Index: 14*512 + 14, Option: [6]int{-1, -1, -1, -1, -1, -1}, Map: 0, Monster: -1, MinLevel: -1, MaxLevel: -1, AccountLevel: 0, Rate: 5000},
	}}
	var dropped item.Item
	old := dropMapItemHook
	dropMapItemHook = func(w *World, mp, x, y int, it item.Item, owner int) bool { dropped = it; return owner == p.Index }
	t.Cleanup(func() { dropMapItemHook = old })
	fixRand(t, 5000) // rate 5000 <= 5000: refused
	if b.dropEventDrop(p, m) {
		t.Fatal("dropped although the roll failed")
	}
	fixRand(t, 4999)
	if !b.dropEventDrop(p, m) || dropped.Index != 14*512+14 {
		t.Fatalf("dropped %+v", dropped)
	}
	p.P.Account.Level = 1 // AccountLevel column 0 only
	if b.dropEventDrop(p, m) {
		t.Fatal("dropped for another account level")
	}
}
