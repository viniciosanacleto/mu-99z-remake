package game

import (
	"testing"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/item"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/frame"
)

// A monster with the Golden Goblin class drops its EventItemBag item
// (Box of Kundun 1, 14,11 +8) 500 ms after death; the client sees it in
// C2 20.
func TestEndToEndBagDropSeenByClient(t *testing.T) {
	env := newTestEnv(t)
	var m *Object
	var x, y int
	env.onWorld(func() {
		w := env.world
		tr := w.terrain(0)
	search:
		for yy := 60; yy < 230; yy++ {
			for xx := 60; xx < 220; xx++ {
				ok := true
				for k := 0; k <= 2 && ok; k++ {
					ok = tr.Get(xx+k, yy)&(gamedata.AttrBlock|gamedata.AttrNoGround|gamedata.AttrCharacter|gamedata.AttrSafeZone) == 0
				}
				if ok {
					x, y = xx, yy
					break search
				}
			}
		}
		m = w.monsterByClass(0, 0)
		w.monsterOccupy(m, false)
		m.X, m.Y, m.TX, m.TY = x+1, y, x+1, y
		m.Move = nil
		w.setMoveSpeed(m, m.Mon.Info.MoveSpeed)
		w.monsterOccupy(m, true)
		m.Mon.Attribute = 0
		m.Mon.DefenseRate, m.Mon.Defense = 0, 0
		m.Mon.Life = 1
	})
	c := enterAt(t, env, "tester", "Hero", 0x10, 0, x, y, 1)
	waitMonster(t, c, m.Index)
	env.onWorld(func() { m.Class = 78 }) // Golden Goblin bag
	c.send(frame.Build(frame.C1, 0xD7, byte(m.Index>>8), byte(m.Index), 0x78, 3))
	if die := c.next(0x17, -1); int(die[3])<<8|int(die[4]) != m.Index {
		t.Fatalf("die % X", die)
	}
	p := c.next(0x20, -1)
	if p[0] != 0xC2 || p[4] < 1 {
		t.Fatalf("C2 20 % X", p)
	}
	e := p[5:16]
	it := item.DecodeWire(e[4:11])
	if it.Index != 14*512+11 || it.Level != 8 || int(e[2]) != x+1 || int(e[3]) != y {
		t.Fatalf("item entry % X = %+v", e, it)
	}
}
