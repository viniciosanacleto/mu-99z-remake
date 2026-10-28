package game

import (
	"time"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"
)

// Periodic notices (CNotice, Lang/<lang>/Notice.txt in config/data/lang.toml): one rotating list per
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

// loadNotices reads the Notice tables of config/data/lang.toml.
func loadNotices(configDir string, now time.Time) ([3]noticeList, error) {
	var out [3]noticeList
	f, err := gamedata.LoadLangConfig(configDir)
	if err != nil {
		return out, err
	}
	for i, lang := range f.Languages() {
		out[i].last = now
		for _, r := range lang.Notice.Rows {
			out[i].items = append(out[i].items, notice{
				text: r.Message, typ: byte(r.Type), count: byte(r.Count), opacity: byte(r.Opacity),
				delay: uint16(r.Delay),
				color: uint32(r.ColorR) | uint32(r.ColorG)<<8 | uint32(r.ColorB)<<16 | uint32(r.Opacity)<<24,
				speed: byte(r.Speed), repeat: time.Duration(r.RepeatTime) * time.Second,
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

func registerNotice(h *hooks) {
	h.start = append(h.start, func(w *World) error {
		var err error
		w.notices, err = loadNotices(w.configDir, w.now())
		return err
	})
	h.tick1000 = append(h.tick1000, (*World).noticeTick)
}
