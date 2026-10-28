package game

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/store"
)

// CCustomMonsterKillCount (Custom/CustomMonsterKillCount.txt,
// FUN_004b8920): kills of a monster class are counted in the
// MonsterKillCount table; reaching a row's KillCount gives its money,
// items, a firework and its message. The original file has no rows.
//
//   - monster death (FUN_004b7290 -> FUN_004b8f90): the first row of the
//     class with the killer's account-level column on sends DS C1 1E 50
//     {index u16, account[11], name[11], class u16};
//   - DataServer FUN_0040e6d0 adds one and answers C1 20 50 with the
//     count (u16) at [0x1e];
//   - the answer (FUN_004b9070) walks every row of the class: below its
//     KillCount "%s %d/%d" (monster name, count, KillCount, blue notice);
//     exactly at it the reward, then the walk stops.

type killCount struct {
	rows []gamedata.KillCountRow
}

func (w *World) kills() *killCount {
	x := w.wcx()
	if x.kills == nil {
		x.kills = &killCount{}
	}
	return x.kills
}

func registerKillcount(h *hooks) {
	h.start = append(h.start, func(w *World) error {
		w.wcx().kills = nil
		if w.configDir == "" {
			return nil
		}
		rows, err := gamedata.LoadCustomMonsterKillCount(w.configDir)
		w.kills().rows = rows
		return err
	})
	h.monsterKill = (*World).killCountKill
}

// killCountKill is FUN_004b8f90.
func (w *World) killCountKill(mon, p *Object) {
	if !p.IsPlayer() || p.P.Char == nil {
		return
	}
	al := accountLevelOf(p)
	found := false
	for _, r := range w.kills().rows {
		if r.Monster == mon.Class && al >= 0 && al < 4 && r.AL[al] != 0 {
			found = true
			break
		}
	}
	if !found {
		return
	}
	cs, _ := w.store.(store.CustomWorldStore)
	if cs == nil {
		return
	}
	name, class, idx := p.P.Char.Name, mon.Class, p.Index
	w.async(func(ctx context.Context) func() {
		n, err := cs.AddMonsterKill(ctx, name, class)
		if err != nil {
			slog.Warn("monster kill count", "name", name, "class", class, "err", err)
			return nil
		}
		return func() {
			if o := w.objects[idx]; o.State == StatePlaying && o.IsPlayer() && o.P.Char != nil && o.P.Char.Name == name {
				w.killCountAnswer(o, class, n&0xFFFF)
			}
		}
	})
}

// killCountAnswer is FUN_004b9070.
func (w *World) killCountAnswer(o *Object, class, count int) {
	for i := range w.kills().rows {
		r := &w.kills().rows[i]
		if r.Monster != class || count > r.KillCount {
			continue
		}
		if count < r.KillCount {
			name := "Unknown"
			if info := w.mw().info[class]; info != nil {
				name = info.Name
			}
			w.eventText(o, NoticeBlue, fmt.Sprintf("%s %d/%d", name, count, r.KillCount))
			continue
		}
		if r.Money > 0 {
			addMoney(o, int64(r.Money)) // FUN_0050cf10: capped at 2.000.000.000
			w.sendMoney(o)
		}
		x, y := o.X, o.Y
		for _, it := range r.Items {
			if nx, ny, ok := w.randomNearCell(o.Map, x, y, 2, 2, 0x32); ok {
				x, y = nx, ny
			} else {
				x, y = o.X, o.Y
			}
			w.createItem(o, o.Map, x, y, dropSpec{Index: it.Index, Level: it.Level, Dur: it.Durability,
				Skill: it.Skill, Luck: it.Luck, Option: it.Option, Exc: byte(it.Excellent), Set: byte(it.SetOption),
				Duration: it.Duration})
		}
		pk := packetFirework(0, byte(o.X), byte(o.Y))
		w.send(o, pk)
		w.broadcast(o, pk)
		w.eventText(o, NoticeBlue, r.Message)
		return
	}
}
