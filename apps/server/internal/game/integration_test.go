package game

import (
	"testing"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/frame"
)

// Cross-module wiring done by the coordinator after the four modules.

func TestShopNPCsSpawnFromShopManager(t *testing.T) {
	b := newBareWorld(t)
	b.tick1000()
	var amy *Object
	for _, s := range b.itemData.shops {
		if s.info.NPCClass != 253 || s.info.Map != 0 {
			continue
		}
		for i := 0; i < SummonStart; i++ {
			o := b.objects[i]
			if o.State == StatePlaying && o.Class == 253 && o.Map == 0 && o.X == s.info.X && o.Y == s.info.Y {
				amy = o
			}
		}
	}
	if amy == nil || amy.Type != TypeNPC || amy.Dir != b.shopRow(253).Dir {
		t.Fatalf("Potion Girl Amy not spawned: %+v", amy)
	}
	o := b.player(PlayerStart, "Buyer", 0, amy.X, amy.Y+2)
	if h := npcTalkHooks[253]; h == nil || !h(b.World, o, amy) || find(b.drain(o), 0x31, -1) == nil {
		t.Fatal("spawned shop NPC does not open the shop")
	}
	// Once only.
	n := 0
	b.tick1000()
	for i := 0; i < SummonStart; i++ {
		if b.objects[i].State == StatePlaying && b.objects[i].Class == 253 && b.objects[i].Map == 0 {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("%d Amys", n)
	}
}

func (b *bareWorld) shopRow(class int) (r struct{ Dir int }) {
	for _, s := range b.itemData.shops {
		if s.info.NPCClass == class {
			r.Dir = s.info.Dir
			return
		}
	}
	return
}

func killWith(t *testing.T, b *bareWorld, p, m *Object) {
	t.Helper()
	old := combatStatsHook
	combatStatsHook = func(w *World, o *Object) CombatStats {
		return CombatStats{DamageMinLeft: 40, DamageMaxLeft: 50, AttackSuccessRate: 1000}
	}
	t.Cleanup(func() { combatStatsHook = old })
	m.Mon.Life = 1
	b.handleAttack(p, frame.Build(frame.C1, 0xD7, byte(m.Index>>8), byte(m.Index), 0x78, 3))
}

func TestKillInPartySharesExperience(t *testing.T) {
	b := newBareWorld(t)
	a := statsPlayer(b, PlayerStart, 1, 50)
	c := statsPlayer(b, PlayerStart+1, 0, 50)
	c.X = a.X + 2
	if !b.partyCreate(a) || !b.partyAdd(b.partyOf(a), c) {
		t.Fatal("party")
	}
	m := b.monsterNear(0, a, 1)
	b.see(a, m)
	b.drain(a)
	b.drain(c)
	before := c.P.Char.Experience
	killWith(t, b, a, m)
	if find(b.drain(a), 0x9C, -1) == nil || find(b.drain(c), 0x9C, -1) == nil || c.P.Char.Experience <= before {
		t.Fatal("party member got no experience: the kill did not use FUN_004c5ea0")
	}
	if !samePartyHook(b.World, a, c) || samePartyHook(b.World, a, b.player(PlayerStart+2, "Out", 0, a.X, a.Y+3)) {
		t.Fatal("samePartyHook")
	}
}

func TestSoloKillShowsZeroExpOnLevelUp(t *testing.T) {
	b := newBareWorld(t)
	p := statsPlayer(b, PlayerStart, 1, 1)
	m := b.monsterNear(0, p, 1)
	b.see(p, m)
	b.drain(p)
	killWith(t, b, p, m)
	ex := find(b.drain(p), 0x9C, -1)
	if p.P.Char.Level < 2 || ex == nil || ex[6]|ex[7]|ex[8]|ex[9] != 0 {
		t.Fatalf("level %d 9C % X", p.P.Char.Level, ex)
	}
}

func TestSummonKillGivesNoExperience(t *testing.T) {
	b := newBareWorld(t)
	p := statsPlayer(b, PlayerStart, 1, 50)
	m := b.monsterNear(0, p, 1)
	m.Mon.Owner = PlayerStart + 5
	b.see(p, m)
	b.drain(p)
	killWith(t, b, p, m)
	if pk := b.drain(p); find(pk, 0x17, -1) == nil || find(pk, 0x9C, -1) != nil {
		t.Fatal("summon kill gave experience")
	}
}

func TestMonsterSkillEffectHooksUseEffectSystem(t *testing.T) {
	b := newBareWorld(t)
	o := b.player(PlayerStart, "Target", 0, 130, 130)
	if !addEffectHook(b.World, o, 0x3F, 0, 10) || !moveEffectHook(b.World, o, 0x3F) {
		t.Fatal("effect not applied")
	}
	removeEffectHook(b.World, o, 0x3F)
	if moveEffectHook(b.World, o, 0x3F) {
		t.Fatal("effect not removed")
	}
}

func TestSummonSkillsDispatchToSummonModule(t *testing.T) {
	b := newBareWorld(t)
	o := b.player(PlayerStart, "Elf", 0, 130, 130)
	var got int
	old := summonSkillHook
	summonSkillHook = func(w *World, p *Object, skill int) bool { got = skill; return true }
	t.Cleanup(func() { summonSkillHook = old })
	if !b.dispatchSkill(o, &skillRef{Num: 32}, -1, o.X, o.Y, 0, false) || got != 32 {
		t.Fatalf("summon skill dispatched to %d", got)
	}
}
