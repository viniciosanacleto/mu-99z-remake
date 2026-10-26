package game

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/config/jsonconf"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/config/mutext"
)

// Periodic notices (CNotice, Lang/<lang>/Notice.txt): one rotating list per
// client language. FUN_004c0490 loads them; FUN_004c06d0 (called from the
// object timer) sends the current notice to every playing player of that
// language when its RepeatTime has elapsed and moves to the next one.

type notice struct {
	text                string
	typ, count, opacity byte
	delay               uint16
	color               uint32 // R | G<<8 | B<<16 | Opacity<<24
	speed               byte
	repeat              time.Duration
}

type noticeList struct {
	items []notice
	cur   int
	last  time.Time
}

func loadNotices(dataDir string, now time.Time) ([3]noticeList, error) {
	var out [3]noticeList
	for i, lang := range langDirs {
		lines, err := jsonconf.LoadLines(filepath.Join(dataDir, "Lang", lang, "Notice.json"))
		if err != nil {
			return out, err
		}
		out[i].last = now
		for _, l := range mutext.Table(lines) {
			if len(l.Tokens) != 10 {
				return out, fmt.Errorf("Lang/%s/Notice.txt line %d: %d columns", lang, l.No, len(l.Tokens))
			}
			v := make([]int, 10)
			for k := 1; k < 10; k++ {
				if v[k], err = l.Int(k); err != nil {
					return out, err
				}
			}
			out[i].items = append(out[i].items, notice{
				text: l.Tokens[0], typ: byte(v[1]), count: byte(v[2]), opacity: byte(v[3]),
				delay: uint16(v[4]),
				color: uint32(v[5]) | uint32(v[6])<<8 | uint32(v[7])<<16 | uint32(v[3])<<24,
				speed: byte(v[8]), repeat: time.Duration(v[9]) * time.Second,
			})
		}
	}
	return out, nil
}

// noticeTick is FUN_004c06d0.
func (w *World) noticeTick() {
	now := w.now()
	for lang := range w.notices {
		nl := &w.notices[lang]
		if len(nl.items) == 0 {
			continue
		}
		n := nl.items[nl.cur]
		if now.Sub(nl.last) < n.repeat {
			continue
		}
		nl.cur = (nl.cur + 1) % len(nl.items)
		nl.last = now
		p := packetNotice(n.typ, n.count, n.opacity, n.delay, n.color, n.speed, n.text)
		for i := PlayerStart; i < MaxObjects; i++ {
			o := w.objects[i]
			if o.State == StatePlaying && o.IsPlayer() && o.P.LogoutCountdown < 1 && !o.P.Moving &&
				int(o.P.ClientType) == lang {
				w.send(o, p)
			}
		}
	}
}

func init() {
	startHooks = append(startHooks, func(w *World) error {
		var err error
		w.notices, err = loadNotices(w.dataDir, w.now())
		return err
	})
	tick1000Hooks = append(tick1000Hooks, (*World).noticeTick)
}
