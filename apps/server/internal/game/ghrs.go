package game

import (
	"context"
	"time"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/store"
)

// GHRS (Custom/CustomGHRS.ini), only in CastleSiege.exe (the instance with
// ServerType 1): a daily reset cap. Its log text is Vietnamese ("[GHRS]
// Cap nhat GHRS hom nay: %d (Top 1 00h duoc reset +%d)", status line
// "GHRS:TOP %d + %d(Ngay)"). In CastleSiege.exe:
//   - FUN_00442c10 reads [WEEK] Monday..Sunday and [TIMEUPDATE]
//     Hour/Min/Sec (GetPrivateProfileIntA, default 0) at start-up
//     (FUN_004e2250);
//   - the thread FUN_00442ed0 waits for the DataServer link (0x8eb6a4),
//     then every 10 s sends C1 08 F8 YYYYMMDD (FUN_00442cf0) for the
//     "GHRS day" (UTC+7 time, the previous day before TIMEUPDATE) while
//     the stored date (0x8eb6a0) is older;
//   - the DataServer reply (head F8, FUN_00442dc0 from the dispatcher at
//     0x46aa36) carries the date and the top-1 reset count: the cap
//     (0x8eb698) becomes top + Week[weekday of the GHRS day];
//   - /reset (0x42f79d, before the busy and level checks), the auto reset
//     (0x434470, after its busy/level/zen checks) and the reset reply
//     (0x4357a3, after the save) use the cap.
//
// The DataServer.exe of this installation has no F8 handler (dispatcher
// FUN_0040a5a0 in .re/ds.c), so in the original the reply never comes, the
// cap stays at its .bss value 0 and every /reset on the CastleSiege server
// is refused (U-GHRS-01). Fixed here: the service answers F8 itself
// (ghrsSecond, every 10 s while the stored date is older, with the top-1
// ResetCount from store.GHRSStore), and until the first answer no cap is
// applied. Messages 0x3DF and the "after reset" line (0x5F, whose shipped
// text is about connection time) use texts added to the data/ copy of
// Message.txt (991 and 992).

func init() {
	tick1000Hooks = append(tick1000Hooks, (*World).ghrsSecond)
}

// ghrsState is 0x8eb698..0x8eb6b4.
type ghrsState struct {
	cfg     gamedata.CustomGHRS
	next    time.Time // the 10 s period of the thread FUN_00442ed0
	pending bool
	Limit   int // 0x8eb698
	Date    int // 0x8eb6a0 (YYYYMMDD of the last reply)
	Top     int // 0x8eb6b0
	Bonus   int // 0x8eb6b4
}

const (
	msgGHRSLimit = 0x3DF // CastleSiege.exe: resets, cap
	msgGHRSDone  = 992   // CastleSiege.exe uses 0x5F (another text in Message.txt)
	ghrsZone     = 0x6270
	ghrsBack     = 0xef10
)

// ghrsDay is the date FUN_00442cf0 / FUN_00442dc0 compute: gmtime(now +
// 7 h); before Hour:Min:Sec of [TIMEUPDATE] it is gmtime(now - 17 h).
func ghrsDay(g gamedata.CustomGHRS, now time.Time) time.Time {
	t := now.Add(ghrsZone * time.Second).UTC()
	if t.Hour()*10000+t.Minute()*100+t.Second() < g.Hour*10000+g.Min*100+g.Sec {
		t = now.Add(-ghrsBack * time.Second).UTC()
	}
	return t
}

func ghrsDate(t time.Time) int { return t.Year()*10000 + int(t.Month())*100 + t.Day() }

// ghrsReply is FUN_00442dc0: cap = top + Week[weekday of the GHRS day].
func (w *World) ghrsReply(date, top int) {
	g := w.dus().ghrs
	if g == nil {
		return
	}
	bonus := g.cfg.Week[ghrsDay(g.cfg, w.now()).Weekday()]
	g.Limit, g.Date, g.Top, g.Bonus = top+bonus, date, top, bonus
	w.log.Info("[GHRS] daily reset cap", "cap", g.Limit, "top", top, "bonus", bonus)
}

// ghrsSecond is the request thread FUN_00442ed0 with the F8 answer the
// shipped DataServer lacks: every 10 s, while the stored date is older
// than the GHRS day, the top-1 reset count is read and ghrsReply runs.
func (w *World) ghrsSecond() {
	if w.du == nil || w.du.ghrs == nil {
		return
	}
	g, now := w.du.ghrs, w.now()
	if g.pending || now.Before(g.next) {
		return
	}
	g.next = now.Add(10 * time.Second)
	day := ghrsDate(ghrsDay(g.cfg, now))
	gs, _ := w.store.(store.GHRSStore)
	if g.Date >= day || gs == nil {
		return
	}
	g.pending = true
	w.async(func(ctx context.Context) func() {
		top, err := gs.TopResets(ctx)
		return func() {
			g.pending = false
			if err != nil {
				w.log.Warn("[GHRS] top reset", "err", err)
				return
			}
			w.ghrsReply(day, top)
		}
	})
}

// ghrsBlocks is the cap test of 0x42f79d / 0x434470: resets (obj+0x680)
// >= cap sends message 0x3DF (resets, cap) as a gold notice
// (FUN_004b9160 type 0) and refuses the reset.
func (w *World) ghrsBlocks(o *Object) bool {
	g := w.dus().ghrs
	if g == nil || g.Date == 0 || o.P.Char.Resets < g.Limit {
		return false // no answer yet: no cap (the original used 0)
	}
	w.goldMessage(o, w.messageFor(o, msgGHRSLimit, o.P.Char.Resets, g.Limit))
	return true
}

// ghrsResetDone is 0x4357a3: after a reset, message 0x5F (resets, cap) as
// a blue notice.
func (w *World) ghrsResetDone(o *Object) {
	if g := w.dus().ghrs; g != nil && g.Date != 0 {
		w.systemMessage(o, w.messageFor(o, msgGHRSDone, o.P.Char.Resets, g.Limit))
	}
}
