package game

import (
	"context"
	"log/slog"
	"time"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/frame"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/store"
)

// DailyReward (Custom/DailyReward.txt, FUN_0045a200; client window of
// Main.dll): one reward per day of the month, claimed once per account.
//
//   - with the character the DataServer returns the days of the current
//     month the account claimed ("SELECT * FROM DailyReward WHERE Acc AND
//     month = MONTH(GETDATE())", FUN_0040c4c0); the answer (DS 0x68,
//     FUN_0045a620) keeps today's day and the flags and sends C1 2C FB 17
//     (the month) and C1 E0 FB 18 (today's reward);
//   - every second (FUN_0045a150 from FUN_00510810) a new day sends both
//     again;
//   - C1 FB 02 (FUN_0045a7c0): [4] 0 = refresh, 1 = the reward of day [8],
//     2 = claim day [8] (FUN_0045abd0);
//   - a claim saves the character (FUN_00475c30), whose DS 0x68 inserts
//     the claimed days (FUN_00410980).

const dailyDays = 31

type dailyState struct {
	mday  int // +4 of the per-player record
	flags [dailyDays]bool
}

type dailyReward struct {
	rows    map[int]gamedata.DailyRewardRow
	players map[int]*dailyState
}

func (w *World) daily() *dailyReward {
	x := w.wcx()
	if x.daily == nil {
		x.daily = &dailyReward{rows: map[int]gamedata.DailyRewardRow{}, players: map[int]*dailyState{}}
	}
	return x.daily
}

func registerDailyreward(h *hooks) {
	h.start = append(h.start, func(w *World) error {
		w.wcx().daily = nil
		if w.configDir == "" {
			return nil
		}
		rows, err := gamedata.LoadDailyReward(w.configDir)
		w.daily().rows = rows
		return err
	})
	h.enterWorld = append(h.enterWorld, (*World).dailyLoad)
	h.leaveWorld = append(h.leaveWorld, func(w *World, o *Object) { delete(w.daily().players, o.Index) })
	h.tick1000 = append(h.tick1000, (*World).dailySecond)
	h.fbHandlers[0x02] = func(w *World, o *Object, p []byte) { w.handleDailyReward(o, p) }
}

// daysInMonth is the [4] of FB 17: February 28/29 by year%4, otherwise
// (m>>3 ^ m) | 30.
func daysInMonth(t time.Time) int {
	m := int(t.Month())
	if m == 2 {
		if t.Year()%4 == 0 {
			return 29
		}
		return 28
	}
	return (m>>3 ^ m) | 0x1E
}

// packetDailyMonth is C1 2C FB 17 (FUN_0045a810): [4] days in the month,
// [5] day, [6] weekday of the 1st, [7..37] claimed flags, [40] time (u32).
func packetDailyMonth(now time.Time, flags [dailyDays]bool) []byte {
	p := newPacket(frame.C1, 0xFB, 0x17, 0x2C)
	first := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	p[4] = byte(daysInMonth(now))
	p[5] = byte(now.Day())
	p[6] = byte(first.Weekday())
	for i, f := range flags {
		if f {
			p[7+i] = 1
		}
	}
	u32le(p[40:], uint32(now.Unix()))
	return p
}

// dailyItemBytes is FUN_0045ab20: the 5-byte item of FB 18.
func dailyItemBytes(r gamedata.DailyRewardRow) [5]byte {
	var b [5]byte
	b[0] = byte(r.Index)
	b[1] = byte(r.Level<<3) | byte(r.Skill<<7) | byte(r.Luck<<2) | byte(r.Option&3)
	b[2] = byte(r.Durability)
	b[3] = byte(int16(r.Index)>>1)&0x80 | byte(r.Excellent)
	if r.Option > 3 {
		b[3] |= 0x40
	}
	b[4] = byte(r.Ancient)
	return b
}

// packetDailyItem is C1 E0 FB 18 (FUN_0045a9d0): [4] item index u16, [6]
// durability, [7..11] the item, [24] Info1[100], [124] Info2[100].
func packetDailyItem(r gamedata.DailyRewardRow) []byte {
	p := newPacket(frame.C1, 0xFB, 0x18, 0xE0)
	u16le(p[4:], r.Index)
	p[6] = byte(r.Durability)
	ib := dailyItemBytes(r)
	copy(p[7:12], ib[:])
	copy(p[24:123], r.Info1)
	copy(p[124:223], r.Info2)
	return p
}

// dailySend is FUN_0045a810 + FUN_0045a9d0 for today (no row: no FB 18).
func (w *World) dailySend(o *Object, st *dailyState) {
	now := w.now()
	w.send(o, packetDailyMonth(now, st.flags))
	w.dailySendDay(o, now.Day())
}

func (w *World) dailySendDay(o *Object, day int) {
	if r, ok := w.daily().rows[day]; ok {
		w.send(o, packetDailyItem(r))
	}
}

// dailyLoad reads the claimed days with the character (DS answer 0x68,
// FUN_0045a620).
func (w *World) dailyLoad(o *Object) {
	cs, _ := w.store.(store.CustomWorldStore)
	if cs == nil || !o.IsPlayer() {
		return
	}
	login, idx, name := o.P.Account.Login, o.Index, charName(o)
	month := int(w.now().Month())
	w.async(func(ctx context.Context) func() {
		days, err := cs.DailyRewardDays(ctx, login, month)
		if err != nil {
			slog.Warn("daily reward", "account", login, "err", err)
			return nil
		}
		return func() {
			o := w.objects[idx]
			if o.State != StatePlaying || !o.IsPlayer() || charName(o) != name {
				return
			}
			st := &dailyState{mday: w.now().Day()}
			for _, d := range days {
				if d >= 1 && d <= dailyDays {
					st.flags[d-1] = true
				}
			}
			w.daily().players[idx] = st
			w.dailySend(o, st)
		}
	})
}

// dailySecond is FUN_0045a150: a new day sends the window again.
func (w *World) dailySecond() {
	today := w.now().Day()
	for idx, st := range w.daily().players {
		o := w.objects[idx]
		if o.State != StatePlaying || !o.IsPlayer() {
			continue
		}
		if st.mday != today {
			w.dailySend(o, st)
			st.mday = today
		}
	}
}

// handleDailyReward is C1 FB 02 (FUN_0045a7c0).
func (w *World) handleDailyReward(o *Object, p []byte) {
	st := w.daily().players[o.Index]
	if len(p) < 5 || st == nil {
		return
	}
	day := 0
	if len(p) >= 12 {
		day = int(int32(uint32(p[8]) | uint32(p[9])<<8 | uint32(p[10])<<16 | uint32(p[11])<<24))
	}
	switch p[4] {
	case 0:
		w.dailySend(o, st)
	case 1:
		w.dailySendDay(o, day)
	case 2:
		w.dailyClaim(o, st, day)
	}
}

// dailyClaim is FUN_0045abd0: "No Reward!" without a row, "You
// Complete!" for a claimed day, "Inventory full!" without a free 4x4
// area, "You Can't Receive Gifts!" for a day other than today; otherwise
// the item goes to the inventory, the day is marked and the window sent
// again.
func (w *World) dailyClaim(o *Object, st *dailyState, day int) {
	r, ok := w.daily().rows[day]
	if !ok {
		w.eventText(o, NoticeBlue, "No Reward!")
		return
	}
	if day < 1 || day > dailyDays {
		return
	}
	switch {
	case st.flags[day-1]:
		w.eventText(o, NoticeBlue, "You Complete!")
		return
	case !w.hasFreeArea(o, 4, 4):
		w.eventText(o, NoticeBlue, "Inventory full!")
		return
	case day != st.mday:
		w.eventText(o, NoticeBlue, "You Can't Receive Gifts!")
		return
	}
	if r.Index != 0xFFFF {
		w.createItem(o, 0xEB, o.X, o.Y, dropSpec{Index: r.Index, Level: r.Level, Dur: r.Durability, Skill: r.Skill,
			Luck: r.Luck, Option: r.Option, Exc: byte(r.Excellent), Set: byte(r.Ancient), ToInventory: true})
	}
	w.eventText(o, NoticeBlue, "You Received Success Gifts!")
	st.flags[day-1] = true
	w.dailySend(o, st)
	if cs, _ := w.store.(store.CustomWorldStore); cs != nil {
		login, name, month := o.P.Account.Login, charName(o), int(w.now().Month())
		w.async(func(ctx context.Context) func() {
			if err := cs.AddDailyReward(ctx, login, name, day, month); err != nil {
				slog.Warn("daily reward save", "account", login, "err", err)
			}
			return nil
		})
	}
}

// hasFreeArea is FUN_00495340: some free bag cell (rows 0..7) where a
// wd x ht item fits.
func (w *World) hasFreeArea(o *Object, wd, ht int) bool {
	g := w.occupancy(&o.P.Inventory, -1)
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			if !g[y][x] && fitsAt(&g, BagStart+y*8+x, wd, ht) {
				return true
			}
		}
	}
	return false
}
