package game

import "github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"

// CCustomEventDrop (Custom/CustomEventDrop.txt, CustomEventDropSwitch of
// GameServerInfo - Custom.dat): ten scheduled events that rain items on a
// spot. Entry (0x74 bytes from DAT_015ae548): +0 index, +4 state, +8
// remain, +0xc target, +0x10 tick, +0x14 minute, +0x18 last minute, +0x1c
// name, +0x3c map, +0x40 x, +0x44 y, +0x48 radius, +0x4c AlarmTime, +0x50
// EventTime, +0x54 items {index, level, count, delay, dropped}, +0x64
// start times.

const (
	eventDropWait   = 1
	eventDropRun    = 2
	msgEventDropMin = 0x20D
	msgEventDropGo  = 0x20E
	msgEventDropEnd = 0x20F
	eventDropTries  = 0x32
)

type eventDropEntry struct {
	Index, State    int
	Remain          int
	Target          int64
	Minute, LastMin int
	Dropped         []bool
}

type customEventDrop struct {
	on   bool
	data *gamedata.CustomEventDrop
	e    [gamedata.CustomEventDropCount]eventDropEntry
}

func (w *World) edrop() *customEventDrop {
	x := w.wcx()
	if x.edrop == nil {
		x.edrop = &customEventDrop{data: &gamedata.CustomEventDrop{}}
		for i := range x.edrop.e {
			x.edrop.e[i] = eventDropEntry{Index: i, Minute: -1, LastMin: -1}
		}
	}
	return x.edrop
}

func registerCustomeventdrop(h *hooks) {
	h.start = append(h.start, loadCustomEventDrop)
	h.tick1000 = append(h.tick1000, (*World).eventDropSecond)
}

// loadCustomEventDrop is FUN_004623e0 and, at the end of the start-up,
// FUN_00462370.
func loadCustomEventDrop(w *World) error {
	w.wcx().edrop = nil
	d := w.edrop()
	d.on = w.cfg.Int("Custom", "CustomEventDropSwitch", 0) != 0
	if w.configDir != "" {
		data, err := gamedata.LoadCustomEventDrop(w.configDir)
		if err != nil {
			return err
		}
		d.data = data
	}
	w.eventDropInit()
	return nil
}

// eventDropInit is FUN_00462370.
func (w *World) eventDropInit() {
	d := w.edrop()
	if !d.on {
		return
	}
	for i := range d.e {
		w.eventDropRestart(&d.e[i])
	}
}

func (w *World) eventDropRestart(e *eventDropEntry) {
	e.Minute, e.LastMin, e.State = -1, -1, eventDropWait
	e.Dropped = make([]bool, len(w.edrop().data.Items[e.Index]))
	w.eventDropSchedule(e)
}

// eventDropSchedule is FUN_00462c20 (no start time: state 0).
func (w *World) eventDropSchedule(e *eventDropEntry) {
	d := w.edrop()
	now := w.now()
	next, ok := nextSchedule(d.data.Schedule[e.Index], now)
	if !ok {
		if d.on {
			e.State = 0
		}
		return
	}
	e.Target = next.Unix()
	e.Remain = int(e.Target - now.Unix())
}

// eventDropSecond is FUN_00462860 (1-second timer FUN_00509cc0).
func (w *World) eventDropSecond() {
	d := w.edrop()
	if !d.on {
		return
	}
	for i := range d.e {
		e := &d.e[i]
		e.Remain = int(e.Target - w.now().Unix())
		switch e.State {
		case eventDropWait:
			w.eventDropWait(e)
		case eventDropRun:
			w.eventDropRun(e)
		}
	}
}

// eventDropWait is FUN_004628f0: the minutes of the alarm window (0x20D to
// everybody), then 0x20E and EventTime minutes of drops (FUN_00462be0).
func (w *World) eventDropWait(e *eventDropEntry) {
	d := w.edrop()
	r := &d.data.Rules[e.Index]
	if e.Remain > 0 && e.Remain <= r.AlarmTime*60 {
		m := e.Remain / 60
		if e.Remain%60 == 0 {
			m--
		}
		e.Minute = m
		if m != e.LastMin {
			e.LastMin = m
			w.noticeAllMsg(NoticeGold, msgEventDropMin, r.Name, e.LastMin+1)
		}
	}
	if e.Remain < 1 {
		w.noticeAllMsg(NoticeGold, msgEventDropGo, r.Name)
		if d.on {
			e.State = eventDropRun
			e.Minute, e.LastMin = -1, -1
			e.Remain = r.EventTime * 60
			e.Target = w.now().Unix() + int64(e.Remain)
		}
	}
}

// eventDropRun is FUN_00462a00: each item row whose DropDelay has passed
// drops Count items, each on a random cell within DropRadius of the spot
// (FUN_0050c860, 50 tries, else the spot itself) with a firework shown to
// the players around (FUN_00462dd0); the items have no loot owner. At the
// end 0x20F and the next start time.
func (w *World) eventDropRun(e *eventDropEntry) {
	d := w.edrop()
	r := &d.data.Rules[e.Index]
	items := d.data.Items[e.Index]
	if len(e.Dropped) != len(items) {
		e.Dropped = make([]bool, len(items))
	}
	elapsed := r.EventTime*60 - e.Remain
	for i, it := range items {
		if e.Dropped[i] || it.Delay > elapsed {
			continue
		}
		for c := 0; c < it.Count; c++ {
			e.Dropped[i] = true
			x, y, ok := w.randomNearCell(r.Map, r.X, r.Y, r.Radius, r.Radius, eventDropTries)
			if !ok {
				x, y = r.X, r.Y
			}
			w.eventDropFirework(r.Map, x, y)
			w.createItem(nil, r.Map, x, y, dropSpec{Index: it.Index, Level: it.Level})
		}
	}
	if e.Remain < 1 {
		w.noticeAllMsg(NoticeGold, msgEventDropEnd, r.Name)
		if d.on {
			w.eventDropRestart(e)
		}
	}
}

// eventDropFirework is FUN_00462dd0: C1 07 F3 40 00 x y to every playing
// player of the map within the view range (MapManager, 12 by default).
func (w *World) eventDropFirework(mp, x, y int) {
	r := w.viewRange(mp)
	pk := packetFirework(0, byte(x), byte(y))
	for i := PlayerStart; i < MaxObjects; i++ {
		if o := w.objects[i]; noticePlayer(o) && inRange(o, mp, x, y, r) {
			w.send(o, pk)
		}
	}
}
