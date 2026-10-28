package game

import (
	"testing"
)

func TestSummonCreateViewportDismiss(t *testing.T) {
	b := newBareWorld(t)
	x, y := b.openRow(0, 8)
	p := b.player(PlayerStart, "Elfa", 0, x+2, y)
	if !b.summonMonster(p, 30) {
		t.Fatal("summon failed")
	}
	s := b.summonOf(p)
	class := b.cfg.Int("Skill", "SummonMonster1", 0)
	if s == nil || s.Index < SummonStart || s.Index >= PlayerStart || s.Class != class || s.Mon.Owner != p.Index ||
		s.Mon.Attribute != 4 || s.Mon.MoveRange != summonRange || s.X != p.X-1 || s.Y != p.Y+1 || s.Dir != 2 {
		t.Fatalf("summon = %+v", s)
	}
	pk := find(b.drain(p), 0xF3, 0x20)
	if pk == nil || len(pk) != 5 || pk[4] != 100 {
		t.Fatalf("F3 20 = % X", pk)
	}
	// C2 1F to a viewer: owner name at [13..22] of the 24-byte entry.
	vpAdd(&p.VP, s.Index, byte(s.Type))
	b.sendSummonViewport(p)
	v := find(b.drain(p), 0x1F, -1)
	if v == nil || v[0] != 0xC2 || int(v[1])<<8|int(v[2]) != 5+24 || v[4] != 1 {
		t.Fatalf("1F = % X", v)
	}
	e := v[5:]
	if int(e[0]&0x7F)<<8|int(e[1]) != s.Index || int(e[2])<<8|int(e[3]) != class || int(e[8]) != s.X ||
		string(e[13:17]) != "Elfa" {
		t.Fatalf("entry % X", e)
	}
	// The ordinary C2 13 skips the summon.
	b.sendMonsterViewport(p)
	if find(b.drain(p), 0x13, -1) != nil {
		t.Fatal("summon in C2 13")
	}
	// Casting again dismisses it (F3 20 00).
	if b.summonMonster(p, 30) {
		t.Fatal("second summon")
	}
	if b.summonOf(p) != nil || s.State != StateEmpty {
		t.Fatal("summon kept")
	}
	if pk := find(b.drain(p), 0xF3, 0x20); pk == nil || pk[4] != 0 {
		t.Fatalf("dismiss F3 20 = % X", pk)
	}
	// Not on Icarus.
	p.Map = 10
	if b.summonMonster(p, 30) {
		t.Fatal("summon on Icarus")
	}
}

func TestSummonAssistFollowKeepUpAndDeath(t *testing.T) {
	b := newBareWorld(t)
	x, y := b.openRow(0, 8)
	p := b.player(PlayerStart, "Elfa", 0, x+2, y)
	b.summonMonster(p, 30)
	s := b.summonOf(p)
	mob := b.monsterNear(0, p, 4)
	// The owner's attack gives the idle summon a target (FUN_004b58f0).
	b.summonAssist(p, mob)
	if s.Mon.Target != mob.Index || s.Mon.Emotion != emotionAttack || s.Mon.EmotionCount != 30 {
		t.Fatalf("assist: %+v", *s.Mon)
	}
	other := b.player(PlayerStart+1, "Other", 0, x+5, y)
	b.summonAssist(p, other)
	if s.Mon.Target != mob.Index {
		t.Fatal("busy summon retargeted")
	}
	// Follow: next to the owner.
	s.Mon.Target = -1
	b.summonFollow(s)
	if !s.Mon.ActMove || abs(s.Mon.MTX-p.X) > s.Mon.attackReach() || abs(s.Mon.MTY-p.Y) > s.Mon.attackReach() {
		t.Fatalf("follow to %d,%d owner %d,%d", s.Mon.MTX, s.Mon.MTY, p.X, p.Y)
	}
	// Far from the owner: teleported to it.
	var tp []int
	b.hooks.teleport = func(w *World, o *Object, mp, x, y, dir int) { tp = []int{o.Index, mp, x, y} }
	if b.summonKeepUp(s) {
		t.Fatal("teleport while close")
	}
	s.X += summonRange + 1
	if !b.summonKeepUp(s) || len(tp) != 4 || tp[0] != s.Index || tp[2] != p.X || tp[3] != p.Y {
		t.Fatalf("keep up = %v", tp)
	}
	// Damage on the summon updates the owner's bar.
	b.drain(p)
	s.Mon.Life = s.Mon.MaxLife / 2
	b.summonDamaged(s, 10)
	if pk := find(b.drain(p), 0xF3, 0x20); pk == nil || pk[4] != 50 {
		t.Fatalf("F3 20 = % X", pk)
	}
	// Summon death (msgDie): no drop, removed from the owner.
	b.monsterDie(s, mob.Index)
	if b.summonOf(p) != nil || s.State != StateEmpty {
		t.Fatal("dead summon kept")
	}
	// Owner death removes a new summon.
	b.summonMonster(p, 31)
	for _, h := range b.hooks.death {
		h(b.World, mob, p)
	}
	if b.summonOf(p) != nil {
		t.Fatal("summon kept after the owner's death")
	}
}

func TestSummonIsProcessedByMonsterTick(t *testing.T) {
	b := newBareWorld(t)
	x, y := b.openRow(0, 8)
	p := b.player(PlayerStart, "Elfa", 0, x+2, y)
	b.summonMonster(p, 30)
	s := b.summonOf(p)
	s.Live = LiveNormal
	s.Mon.Delay = 0
	before := s.Mon.LastAct
	b.advance(1)
	b.monsterTick()
	if s.Mon.LastAct == before {
		t.Fatal("summon not processed by the AI tick")
	}
}
