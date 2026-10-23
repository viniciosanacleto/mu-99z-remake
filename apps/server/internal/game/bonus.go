package game

import (
	"math"
	"time"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"
)

// Bonus manager (CBonusManager, 0x417c70..0x4188f0; Event/BonusManager.dat):
// timed events that raise experience, item drop, chaos mix, excellent and
// set drop rates. Same EMPTY/START cycle as the invasions.

const (
	BonusExperience = 0
	BonusItemDrop   = 1
	BonusChaosMix   = 2
	BonusExcDrop    = 3
	BonusSetDrop    = 4
)

type timedEventState struct {
	data         *gamedata.TimedEvent
	State        int // invBlank / invEmpty / invStart
	Remain       int
	Target, Tick time.Time
}

type bonusManager struct {
	events  []*timedEventState
	bonuses []gamedata.Bonus
}

func init() {
	startHooks = append(startHooks, func(w *World) error {
		bm := &bonusManager{}
		w.ev().bonus = bm
		if w.dataDir == "" {
			return nil
		}
		events, bonuses, err := gamedata.LoadBonusManager(w.dataDir)
		if err != nil {
			return err
		}
		bm.bonuses = bonuses
		for _, d := range events {
			if d == nil {
				bm.events = append(bm.events, nil)
				continue
			}
			s := &timedEventState{data: d, Tick: w.now()}
			bm.events = append(bm.events, s)
			if w.bonusOn() {
				s.State = invEmpty
				w.timedEnterEmpty(s)
			}
		}
		return nil
	})
	tick1000Hooks = append(tick1000Hooks, (*World).bonusTick)
}

func (w *World) bonusOn() bool { return w.cfg.Int("Event", "BonusManagerSwitch", 0) != 0 }

// timedEnterEmpty is FUN_00418730: the next schedule, or BLANK.
func (w *World) timedEnterEmpty(s *timedEventState) {
	now := w.now()
	next, ok := nextSchedule(s.data.Schedule, now)
	if !ok {
		s.State = invBlank
		return
	}
	s.Target = next
	s.Remain = int(math.Floor(next.Sub(now).Seconds()))
}

// timedTick is the 1-second step shared by the bonus manager (FUN_00418400,
// FUN_004184c0, FUN_004185c0) and the drop event: EMPTY -> START with the
// start message when the schedule is reached, START -> EMPTY with the final
// message after the event time.
func (w *World) timedTick(s *timedEventState, on bool) {
	now := w.now()
	if s == nil || now.Sub(s.Tick) < time.Second {
		return
	}
	s.Tick = now
	s.Remain = int(math.Floor(s.Target.Sub(now).Seconds()))
	if s.Remain >= 1 {
		return
	}
	switch s.State {
	case invEmpty:
		if s.data.StartMsg != -1 {
			w.noticeAllMsg(NoticeGold, s.data.StartMsg)
		}
		if on {
			s.State = invStart
			s.Remain = s.data.Time
			s.Target = now.Add(time.Duration(s.data.Time) * time.Second)
		}
	case invStart:
		if s.data.FinalMsg != -1 {
			w.noticeAllMsg(NoticeGold, s.data.FinalMsg)
		}
		if on {
			s.State = invEmpty
			w.timedEnterEmpty(s)
		}
	}
}

func (w *World) bonusTick() {
	bm := w.ev().bonus
	if bm == nil || !w.bonusOn() {
		return
	}
	for _, s := range bm.events {
		w.timedTick(s, w.bonusOn())
	}
}

// bonusValue is FUN_004188f0: v adjusted by every running bonus event row
// of the index for the player's map and account level.
func (w *World) bonusValue(o *Object, index, v int) int {
	bm := w.ev().bonus
	if bm == nil {
		return v
	}
	al := accountLevel(o)
	if al < 0 || al > 3 {
		al = 0
	}
	for _, b := range bm.bonuses {
		if b.Index != index || (b.Map != -1 && b.Map != o.Map) || b.Event < 0 || b.Event >= len(bm.events) {
			continue
		}
		if s := bm.events[b.Event]; s == nil || s.State != invStart {
			continue
		}
		rate := b.Value[al]
		switch index {
		case BonusExperience, BonusItemDrop:
			v += rate * v / 100
		case BonusChaosMix:
			v += rate
		case BonusExcDrop, BonusSetDrop:
			v += int(int64(rate) * int64(1000000-v) / 1000000)
		}
	}
	return v
}
