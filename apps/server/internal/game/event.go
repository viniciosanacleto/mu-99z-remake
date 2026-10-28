package game

import (
	"time"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"
)

// Pieces shared by the event managers (Devil Square, Blood Castle, Chaos
// Castle, ...).

// eventData holds the files shared by the events.
type eventData struct {
	entry  gamedata.EventEntryLevel
	stage  []gamedata.StageSpawn
	ds     *devilSquare
	bc     *bloodCastle
	cc     *chaosCastle
	inv    []*invasionState
	bonus  *bonusManager
	drop   *dropEventManager
	archer []gamedata.GoldenArcherReward
	deep   *castleDeep // castledeep.go
	kalima *kalimaState
}

func (w *World) ev() *eventData {
	if w.eventData == nil {
		w.eventData = &eventData{entry: gamedata.EventEntryLevel{}}
	}
	return w.eventData
}

func registerEvent(h *hooks) {
	h.start = append(h.start, func(w *World) error {
		e := w.ev()
		if w.configDir == "" {
			return nil
		}
		var err error
		if e.entry, err = gamedata.LoadEventEntryLevel(w.configDir); err != nil {
			return err
		}
		e.stage, err = gamedata.LoadEventStageSpawn(w.configDir)
		return err
	})
}

// nextSchedule is CScheduleManager (FUN_004e52d0 / FUN_004e53c0): the
// earliest time >= now matching one of the rows ("*" fields match any
// value; DayOfWeek is tm_wday, 0 = Sunday).
func nextSchedule(rows []gamedata.Schedule, now time.Time) (time.Time, bool) {
	var best time.Time
	found := false
	for _, r := range rows {
		if t, ok := nextScheduleRow(r, now); ok && (!found || t.Before(best)) {
			best, found = t, true
		}
	}
	return best, found
}

func nextScheduleRow(r gamedata.Schedule, now time.Time) (time.Time, bool) {
	match := func(want, v int) bool { return want == -1 || want == v }
	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	for d := 0; d < 366*4+1; d++ {
		dt := day.AddDate(0, 0, d)
		if !match(r.Year, dt.Year()) || !match(r.Month, int(dt.Month())) || !match(r.Day, dt.Day()) ||
			!match(r.DayOfWeek, int(dt.Weekday())) {
			continue
		}
		for h := 0; h < 24; h++ {
			if !match(r.Hour, h) {
				continue
			}
			for m := 0; m < 60; m++ {
				if !match(r.Minute, m) {
					continue
				}
				for s := 0; s < 60; s++ {
					if !match(r.Second, s) {
						continue
					}
					t := dt.Add(time.Duration(h)*time.Hour + time.Duration(m)*time.Minute + time.Duration(s)*time.Second)
					if !t.Before(now) {
						return t, true
					}
				}
			}
		}
	}
	return time.Time{}, false
}

// noticePlayer is the per-player test of FUN_004c0c70: playing players
// without a pending logout or server move.
func noticePlayer(o *Object) bool {
	return o.State == StatePlaying && o.IsPlayer() && o.P != nil && o.P.LogoutCountdown < 1 && !o.P.Moving
}

// noticeAllMsg is FUN_004c0c70: a C1 0D notice of type typ with the
// Message.txt text n in each player's language.
func (w *World) noticeAllMsg(typ byte, n int, args ...any) {
	for i := PlayerStart; i < MaxObjects; i++ {
		if o := w.objects[i]; noticePlayer(o) {
			w.send(o, packetNotice(typ, 0, 0, 0, 0, 0, w.messageFor(o, n, args...)))
		}
	}
}

// sendAll is FUN_00512600: every playing player.
func (w *World) sendAll(p []byte) {
	for i := PlayerStart; i < MaxObjects; i++ {
		if o := w.objects[i]; o.State == StatePlaying {
			w.send(o, p)
		}
	}
}

// spawnStage is FUN_0046fa30 / FUN_0046f970: every section-4 spawn record
// of the class on the map becomes a monster (MaxRegenTime overrides the
// regen delay; -1 = obj+0x294 0xFFFF, kept as the MonsterList delay).
// Returns the indexes created.
func (w *World) spawnStage(mp, class, regen int) []int {
	var out []int
	for i, r := range w.mw().spawns {
		if r.Section != gamedata.SpawnEvent || r.Class != class || r.Map != mp {
			continue
		}
		idx := w.allocMonsterIndex()
		if idx < 0 {
			return out
		}
		o := w.objects[idx]
		o.reset()
		o.State = StateConnected
		if !w.setMonsterPosition(o, i) || !w.setMonster(o, class) {
			w.deleteMonster(o)
			continue
		}
		if regen != -1 {
			o.Mon.RegenTime = regen
		}
		out = append(out, idx)
	}
	return out
}
