package game

import "github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"

// Drop event (CDropEvent, 0x4708f0..0x471690; Event/DropEvent.dat, empty in
// this installation): timed events (same cycle as the bonus manager) that
// add items to monster drops while they run.

type dropEventManager struct {
	events []*timedEventState
	items  []gamedata.DropEventItem
}

func registerDropevent(h *hooks) {
	h.start = append(h.start, func(w *World) error {
		de := &dropEventManager{}
		w.ev().drop = de
		if w.configDir == "" {
			return nil
		}
		events, items, err := gamedata.LoadDropEvent(w.configDir)
		if err != nil {
			return err
		}
		de.items = items
		for _, d := range events {
			if d == nil {
				de.events = append(de.events, nil)
				continue
			}
			s := &timedEventState{data: d, Tick: w.now()}
			de.events = append(de.events, s)
			if w.dropEventOn() {
				s.State = invEmpty
				w.timedEnterEmpty(s)
			}
		}
		return nil
	})
	h.tick1000 = append(h.tick1000, func(w *World) {
		de := w.ev().drop
		if de == nil || !w.dropEventOn() {
			return
		}
		for _, s := range de.events {
			w.timedTick(s, true)
		}
	})
}

func (w *World) dropEventOn() bool { return w.cfg.Int("Event", "DropEventSwitch", 0) != 0 }

// dropEventDrop is FUN_00471690, called by the monster drop after the
// quest drop: the rows of the running events that match the monster's
// map, class and level and the owner's account level, each passing its
// rate (of 1,000,000; -1 = always), are drawn with weight 1e6/rate. The
// item gets ItemOptionRate options and drops at the monster, owned by p.
func (w *World) dropEventDrop(p, mon *Object) bool {
	de := w.ev().drop
	if de == nil || mon.Mon == nil {
		return false
	}
	al := accountLevel(p)
	var cand []gamedata.DropEventItem
	var weights []int
	for _, r := range de.items {
		if r.Event < 0 || r.Event >= len(de.events) || de.events[r.Event] == nil || de.events[r.Event].State != invStart {
			continue
		}
		if _, ok := w.data.Items[r.Index]; !ok {
			continue
		}
		if (r.Map != -1 && r.Map != mon.Map) || (r.Monster != -1 && r.Monster != mon.Class) ||
			(r.MinLevel != -1 && mon.Mon.Level < r.MinLevel) || (r.MaxLevel != -1 && mon.Mon.Level > r.MaxLevel) ||
			(r.AccountLevel != -1 && r.AccountLevel != al) {
			continue
		}
		rate := r.Rate
		if rate == -1 {
			rate = 1000000
		} else if rate <= rnd(1000000) {
			continue
		}
		if wt := 1000000 / max(rate, 1); wt != 0 {
			cand = append(cand, r)
			weights = append(weights, wt)
		}
	}
	total := 0
	for _, v := range weights {
		total += v
	}
	if total == 0 {
		return false
	}
	x := monsterRand() % total
	for i, v := range weights {
		if x >= v {
			x -= v
			continue
		}
		r := cand[i]
		opt := w.optionsFrom(r.Option, r.Level)
		s := dropSpec{Index: r.Index, Level: opt[0], Dur: r.Dur, Skill: opt[1], Luck: opt[2], Option: opt[3],
			Duration: r.Duration}
		s.Exc = w.makeNewOption(r.Index, 1, opt[4])
		s.Set = w.makeSetOption(r.Index, opt[5])
		w.createItem(p, mon.Map, mon.X, mon.Y, s)
		return true
	}
	return false
}
