package game

import (
	"bytes"
	"testing"
	"time"
)

func TestInvasionCycle(t *testing.T) {
	b := newBareWorld(t)
	b.clock = time.Date(2026, 10, 22, 8, 4, 0, 0, time.UTC) // Skeleton King at 08:05
	for _, s := range b.ev().inv {
		if s != nil {
			s.State = invEmpty
			b.invEnterEmpty(s)
		}
	}
	sk := b.ev().inv[0]
	o := b.player(PlayerStart, "Watcher", 0, 130, 130)
	b.advance(61 * time.Second)
	b.invTick()
	if sk.State != invStart || len(sk.Mobs) != 16 || sk.Remain != 900 {
		t.Fatalf("start: state %d mobs %d remain %d", sk.State, len(sk.Mobs), sk.Remain)
	}
	pk := b.drain(o)
	if n := find(pk, 0x0D, -1); n == nil {
		t.Fatal("no respawn notice (580)")
	}
	boss := -1
	for _, idx := range sk.Mobs {
		m := b.objects[idx]
		if m.Mon.Invasion != 0 || m.Mon.Attribute != invAttribute || m.Mon.RegenTime != 900000 {
			t.Fatalf("mob %+v", m.Mon)
		}
		if m.Class == 55 && boss < 0 {
			boss = idx
		}
	}
	if sk.BossOnMap[0]+sk.BossOnMap[3] != 6 {
		t.Fatalf("bosses %v", sk.BossOnMap[:4])
	}
	// Kill a Death King as the watcher: the boss-killed notice (590).
	m := b.objects[boss]
	o.Map, o.X, o.Y = m.Map, m.X, m.Y
	m.Mon.Hits[0] = damageEntry{Index: o.Index, Damage: 1, Time: b.now()}
	b.invMonsterKilled(m)
	if c := sk.Counters[0]; c.Class != 55 || c.Alive != 5 || c.Max != 6 {
		t.Fatalf("counter %+v", c)
	}
	if n := find(b.drain(o), 0x0D, -1); n == nil || !bytes.Contains(n, []byte("Watcher")) {
		t.Fatalf("boss notice % X", n)
	}
	b.advance(900 * time.Second)
	b.invTick()
	if sk.State != invEmpty || len(sk.Mobs) != 0 || b.objects[boss].State != StateEmpty {
		t.Fatalf("end: state %d mobs %d", sk.State, len(sk.Mobs))
	}
}

func TestInvasionMapState(t *testing.T) {
	b := newBareWorld(t)
	o := b.player(PlayerStart, "Seer", 0, 130, 130)
	rd := b.ev().inv[1] // Red Dragon, effect type 1
	if p := b.invMapState(o); !bytes.Equal(p, []byte{0xC1, 0x05, 0x0B, 0, 0}) {
		t.Fatalf("idle = % X", p)
	}
	rd.State = invStart
	rd.BossOnMap[0] = 2
	if p := b.invMapState(o); !bytes.Equal(p, []byte{0xC1, 0x05, 0x0B, 1, 1}) {
		t.Fatalf("dragon = % X", p)
	}
}
