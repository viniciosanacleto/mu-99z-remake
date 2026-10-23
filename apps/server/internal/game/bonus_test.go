package game

import (
	"testing"
	"time"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"
)

func TestBonusValueAndCycle(t *testing.T) {
	b := newBareWorld(t)
	b.clock = time.Date(2026, 10, 22, 19, 59, 0, 0, time.UTC)
	ev := &timedEventState{data: &gamedata.TimedEvent{Index: 0, StartMsg: -1, FinalMsg: -1, Time: 3600,
		Schedule: []gamedata.Schedule{{Year: -1, Month: -1, Day: -1, DayOfWeek: -1, Hour: 20}}}, Tick: b.clock}
	ev.State = invEmpty
	b.timedEnterEmpty(ev)
	b.ev().bonus = &bonusManager{events: []*timedEventState{ev}, bonuses: []gamedata.Bonus{
		{Event: 0, Index: BonusExperience, Value: [4]int{50, 60, 70, 80}, Map: -1},
		{Event: 0, Index: BonusChaosMix, Value: [4]int{5, 5, 5, 5}, Map: -1},
		{Event: 0, Index: BonusExcDrop, Value: [4]int{1000, 1000, 1000, 1000}, Map: 2},
	}}
	o := b.player(PlayerStart, "Lucky", 0, 130, 130)
	if v := b.bonusValue(o, BonusExperience, 100); v != 100 {
		t.Fatalf("before start %d", v)
	}
	b.advance(time.Minute)
	b.timedTick(ev, true)
	if ev.State != invStart {
		t.Fatalf("state %d", ev.State)
	}
	if v := b.bonusValue(o, BonusExperience, 1000); v != 1500 {
		t.Fatalf("experience %d", v)
	}
	o.P.Account.Level = 3
	if v := b.bonusValue(o, BonusExperience, 1000); v != 1800 {
		t.Fatalf("experience AL3 %d", v)
	}
	if v := b.bonusValue(o, BonusChaosMix, 60); v != 65 {
		t.Fatalf("chaos %d", v)
	}
	if v := b.bonusValue(o, BonusExcDrop, 2000); v != 2000 {
		t.Fatalf("exc on another map %d", v)
	}
	o.Map = 2
	if v := b.bonusValue(o, BonusExcDrop, 2000); v != 2000+1000*998000/1000000 {
		t.Fatalf("exc %d", v)
	}
	b.advance(time.Hour)
	b.timedTick(ev, true)
	if ev.State != invEmpty || !ev.Target.Equal(time.Date(2026, 10, 23, 20, 0, 0, 0, time.UTC)) {
		t.Fatalf("after: state %d next %v", ev.State, ev.Target)
	}
}

func TestBonusAppliedToKillExperience(t *testing.T) {
	b := newBareWorld(t)
	p := b.player(PlayerStart, "Hunter", 0, 130, 130)
	m := b.monsterNear(0, p, 1)
	base := b.killExperience(p, m, 10)
	b.ev().bonus = &bonusManager{
		events:  []*timedEventState{{data: &gamedata.TimedEvent{}, State: invStart}},
		bonuses: []gamedata.Bonus{{Event: 0, Index: BonusExperience, Value: [4]int{100, 100, 100, 100}, Map: -1}},
	}
	if got := b.killExperience(p, m, 10); got != base*2 {
		t.Fatalf("experience %d, base %d", got, base)
	}
	if got := b.bonusRate(p, 60); got != 60 {
		t.Fatalf("chaos without a chaos bonus %d", got)
	}
}
