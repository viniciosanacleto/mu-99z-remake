package game

import (
	"testing"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/frame"
)

// onWorld runs f on the world goroutine and waits for it.
func (e *testEnv) onWorld(f func()) {
	done := make(chan struct{})
	e.world.events <- func() { f(); close(done) } // not post: it reads w.ctx, set by Serve
	<-done
}

// waitMonster waits for the C2 13 entry of index and returns it.
func waitMonster(t *testing.T, c *testClient, index int) []byte {
	t.Helper()
	for {
		p := c.next(0x13, -1)
		for i := 0; i < int(p[4]); i++ {
			e := p[5+16*i : 5+16*i+16]
			if int(e[0]&0x3F)<<8|int(e[1]) == index {
				return e
			}
		}
	}
}

func TestEndToEndLorenciaViewportNPCs(t *testing.T) {
	env := newTestEnv(t)
	var npc *Object
	env.onWorld(func() {
		for i := 0; i < SummonStart; i++ {
			o := env.world.objects[i]
			if o.State == StatePlaying && o.Class == 236 && o.Map == 0 {
				npc = o
			}
		}
	})
	if npc == nil {
		t.Fatal("NPC 236 not spawned")
	}
	c := enterAt(t, env, "tester", "Hero", 0x10, 0, 182, 128, 1)
	e := waitMonster(t, c, npc.Index)
	if e[0]&0x80 == 0 || int(e[2])<<8|int(e[3]) != 236 || e[8] != 175 || e[9] != 120 || e[10] != 175 || e[11] != 120 ||
		e[12]>>4 != 3 {
		t.Fatalf("NPC entry % X", e)
	}
}

func TestEndToEndAttackAndKillMonster(t *testing.T) {
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
		m.Mon.Attribute = 0 // passive: the base AI does nothing
		m.Mon.DefenseRate = 0
		m.Mon.Defense = 0 // DK 4..13 minus 6 can floor at 1, which DKDamageRatePvM 95 truncates to 0
		m.Mon.Life = 1000
	})
	c := enterAt(t, env, "tester", "Hero", 0x10, 0, x, y, 1)
	waitMonster(t, c, m.Index)
	env.onWorld(func() {}) // the viewport pass that sent C2 13 also set live 2

	c.send(frame.Build(frame.C1, 0xD7, byte(m.Index>>8), byte(m.Index), 0x78, 3))
	d := c.next(0xD7, -1)
	life := int(d[8]) | int(d[9])<<8
	if int(d[3]&0x7F)<<8|int(d[4]) != m.Index || int(d[5])<<8|int(d[6]) == 0 || life >= 1000 {
		t.Fatalf("damage % X", d)
	}
	env.onWorld(func() { m.Mon.Life = 1 })
	c.send(frame.Build(frame.C1, 0xD7, byte(m.Index>>8), byte(m.Index), 0x78, 3))
	die := c.next(0x17, -1)
	if int(die[3])<<8|int(die[4]) != m.Index || int(die[6])<<8|int(die[7]) != PlayerStart {
		t.Fatalf("die % X", die)
	}
	if ex := c.next(0x9C, -1); int(ex[3]&0x7F)<<8|int(ex[4]) != m.Index {
		t.Fatalf("exp % X", ex)
	}
}

func TestEndToEndNpcTalk(t *testing.T) {
	env := newTestEnv(t)
	var npc *Object
	env.onWorld(func() { npc = env.world.monsterByClass(0, 240) })
	c := enterAt(t, env, "tester", "Hero", 0x10, 0, npc.X+1, npc.Y, 1)
	waitMonster(t, c, npc.Index)
	c.send(frame.Build(frame.C3, 0x30, byte(npc.Index>>8), byte(npc.Index)))
	p := c.next(0x01, -1)
	if int(p[3])<<8|int(p[4]) != npc.Index || string(p[5:15]) != "Seus Items" {
		t.Fatalf("talk % X", p)
	}
}
