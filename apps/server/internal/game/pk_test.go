package game

import (
	"testing"
	"time"
)

// PKDownRequirePoint1..4 = 21600, 43200, 86400, 3600 (Common.dat).
func TestPlayerKillLevels(t *testing.T) {
	b := newBareWorld(t)
	k := statsPlayer(b, PlayerStart, 1, 100)
	v := statsPlayer(b, PlayerStart+1, 1, 100)
	k.PKLevel, v.PKLevel = PKCommoner, PKCommoner
	b.see(k, v)
	steps := []struct{ level, count, time int }{
		{PKWarning, 1, 21600}, // first murder: PKTime = Req1 -> 4
		{PKOutlaw1, 2, 43200}, // warning: Req2 -> 5
		{PKOutlaw2, 3, 86400}, // outlaw: Req3 -> 6
		{PKOutlaw2, 4, 90000}, // +Req4
	}
	for i, s := range steps {
		b.hooks.playerKill(b.World, k, v)
		c := k.P.Char
		if k.PKLevel != s.level || c.PKCount != s.count || c.PKTime != s.time || c.PKLevel != s.level {
			t.Fatalf("kill %d: level %d count %d time %d", i+1, k.PKLevel, c.PKCount, c.PKTime)
		}
		p := find(b.drain(k), 0xF3, 0x08)
		if p == nil || len(p) != 7 || int(p[4])<<8|int(p[5]) != k.Index || int(p[6]) != s.level {
			t.Fatalf("F3 08 % X", p)
		}
		if find(b.drain(v), 0xF3, 0x08) == nil {
			t.Fatal("viewport not told")
		}
	}
	// A murderer killing a murderer changes nothing.
	v.PKLevel = PKOutlaw1
	b.hooks.playerKill(b.World, k, v)
	if k.P.Char.PKCount != 4 {
		t.Fatal("outlaw vs outlaw counted")
	}
}

func TestPlayerKillHero(t *testing.T) {
	b := newBareWorld(t)
	k := statsPlayer(b, PlayerStart, 1, 100)
	v := statsPlayer(b, PlayerStart+1, 1, 100)
	k.PKLevel, v.PKLevel = PKCommoner, PKOutlaw1
	b.hooks.playerKill(b.World, k, v)
	if k.PKLevel != PKHero0 || k.P.Char.PKTime != 21600 || k.P.Char.PKCount != 1 {
		t.Fatalf("hero level %d time %d", k.PKLevel, k.P.Char.PKTime)
	}
	b.hooks.playerKill(b.World, k, v) // hero0: Req2 -> not above Req2 -> hero1
	if k.PKLevel != PKHero1 || k.P.Char.PKTime != 43200 {
		t.Fatalf("hero level %d time %d", k.PKLevel, k.P.Char.PKTime)
	}
}

func TestPlayerKillExceptions(t *testing.T) {
	b := newBareWorld(t)
	k := statsPlayer(b, PlayerStart, 1, 100)
	v := statsPlayer(b, PlayerStart+1, 1, 100)
	k.PKLevel, v.PKLevel = PKCommoner, PKCommoner
	// Chaos Castle (both on maps 18..23).
	k.Map, v.Map = 18, 18
	b.hooks.playerKill(b.World, k, v)
	if k.PKLevel != PKCommoner {
		t.Fatal("PK in Chaos Castle")
	}
	// Self defense: the victim attacked first (SelfDefenseTime 60 s).
	k.Map, v.Map = 0, 0
	b.addSelfDefense(v, k)
	b.hooks.playerKill(b.World, k, v)
	if k.PKLevel != PKCommoner {
		t.Fatal("self defense counted as PK")
	}
	b.advance(61 * time.Second)
	b.hooks.playerKill(b.World, k, v)
	if k.PKLevel != PKWarning {
		t.Fatal("expired self defense")
	}
}

// FUN_0050e750: PKTime decreases by PKDownPlusTimePoint (1) per second;
// the level follows the thresholds.
func TestPKDownPerSecond(t *testing.T) {
	b := newBareWorld(t)
	o := statsPlayer(b, PlayerStart, 1, 100)
	o.PKLevel, o.P.Char.PKTime = PKOutlaw1, 21601
	b.pkSecond()
	if o.PKLevel != PKWarning || o.P.Char.PKTime != 21600 {
		t.Fatalf("level %d time %d", o.PKLevel, o.P.Char.PKTime)
	}
	if p := find(b.drain(o), 0xF3, 0x08); p == nil || p[6] != PKWarning {
		t.Fatalf("F3 08 % X", p)
	}
	o.P.Char.PKTime = 1
	b.pkSecond()
	if o.PKLevel != PKCommoner {
		t.Fatalf("level %d", o.PKLevel)
	}
	// Commoners are untouched.
	b.pkSecond()
	if o.P.Char.PKTime != 0 {
		t.Fatalf("commoner time %d", o.P.Char.PKTime)
	}
}
