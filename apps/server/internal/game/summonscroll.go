package game

import (
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/item"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"
)

// CSummonScroll (Custom/SummonScroll.txt, FUN_00505d20): dropping a scroll
// item (23, FUN_004988f0, after the firecrackers) summons monsters around
// the drop cell. The rows of the scroll form up to 20 groups; in each
// group one row is drawn with CreateRate as its weight (rand() % sum), so
// a group with a single row always summons it.

const (
	scrollGroups = 20
	scrollRange  = 3
	scrollTries  = 0x32
)

type summonScroll struct {
	data *gamedata.SummonScroll
}

func (w *World) summon() *summonScroll {
	x := w.wcx()
	if x.summon == nil {
		x.summon = &summonScroll{data: &gamedata.SummonScroll{}}
	}
	return x.summon
}

func registerSummonscroll(h *hooks) {
	h.start = append(h.start, func(w *World) error {
		w.wcx().summon = nil
		if w.configDir == "" {
			return nil
		}
		d, err := gamedata.LoadSummonScroll(w.configDir)
		w.summon().data = d
		return err
	})
	h.summonScroll = (*World).summonScrollDrop
}

// summonEventMap: Devil Square, Blood Castle and Chaos Castle refuse the
// scroll.
func summonEventMap(mp int) bool {
	return mp == 9 || mp == 32 || (mp >= 11 && mp <= 17) || (mp >= 18 && mp <= 23)
}

// summonScrollDrop is the branch at joined_r0x0049909d: the player's cell
// and the drop cell must be outside the safe zone and the map not an
// event map (else the drop fails); each group summons its drawn monster on
// a cell within 3 of the drop cell (FUN_0050c860, 50 tries; a failure
// summons nothing) with direction 1.
func (w *World) summonScrollDrop(o *Object, it item.Item, x, y int) (bool, bool) {
	s := w.summon().data
	scroll := -1
	for _, r := range s.Scrolls {
		if r.Item == it.Index {
			scroll = r.Index
			break
		}
	}
	if scroll < 0 {
		return false, false
	}
	if w.attrAt(o.Map, o.X, o.Y)&1 != 0 || w.attrAt(o.Map, x, y)&1 != 0 || summonEventMap(o.Map) {
		return true, false
	}
	type pick struct{ class, lo, hi int }
	var groups [scrollGroups]struct {
		sum   int
		picks []pick
	}
	for _, m := range s.Monsters {
		if m.Index != scroll || m.Rate == 0 || m.Group < 0 || m.Group >= scrollGroups {
			continue
		}
		g := &groups[m.Group]
		g.picks = append(g.picks, pick{m.Class, g.sum, g.sum + m.Rate})
		g.sum += m.Rate
	}
	for gi := range groups {
		g := &groups[gi]
		if g.sum == 0 || len(g.picks) == 0 {
			continue
		}
		r := monsterRand() % g.sum
		class := -1
		for _, p := range g.picks {
			if p.lo <= r && r < p.hi {
				class = p.class
				break
			}
		}
		if class < 0 {
			continue
		}
		idx := w.allocMonsterIndex()
		if idx < 0 {
			continue
		}
		mx, my, ok := w.randomNearCell(o.Map, x, y, scrollRange, scrollRange, scrollTries)
		if !ok {
			continue
		}
		m := w.objects[idx]
		m.reset()
		m.State = StateConnected
		m.X, m.Y, m.TX, m.TY = mx, my, mx, my
		m.Dir = 1
		m.Map = o.Map
		if !w.setMonster(m, class) {
			w.deleteMonster(m)
		}
	}
	return true, true
}
