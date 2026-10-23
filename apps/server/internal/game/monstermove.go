package game

import (
	"sort"
	"time"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"
)

// CMonsterMove (Monster/MonsterMove.txt, DAT_0173d90c, reader
// FUN_004b1110): the wandering NPCs. The rows are grouped in a std::map by
// monster class (FUN_004b12b0); each node keeps +0x14 the object index
// (-1 at load), +0x18 the remaining seconds, +0x1c the time the current
// spot ends (0 at load), +0x20 a GetTickCount stamp (taken at load) and
// the rows (Map X Y Direction KeepTime). The second timer FUN_0050a140
// calls FUN_004b1450.

// monsterMoveNode is one std::map node.
type monsterMoveNode struct {
	Class  int
	Rows   []gamedata.MonsterMoveRow
	Index  int       // +0x14
	Remain int       // +0x18
	Until  int64     // +0x1c (Unix time)
	Tick   time.Time // +0x20
}

// setMonsterMoves builds the nodes in class order (FUN_004b1110).
func (w *World) setMonsterMoves(rows []gamedata.MonsterMoveRow) {
	by := map[int]*monsterMoveNode{}
	d := w.dus()
	d.moves = nil
	for _, r := range rows {
		n := by[r.Monster]
		if n == nil {
			n = &monsterMoveNode{Class: r.Monster, Index: -1, Tick: w.now()}
			by[r.Monster] = n
			d.moves = append(d.moves, n)
		}
		n.Rows = append(n.Rows, r)
	}
	sort.Slice(d.moves, func(i, j int) bool { return d.moves[i].Class < d.moves[j].Class })
}

// monsterMoveSecond is FUN_004b1450. For each class, at most once per
// 1000 ms: Remain = Until - now; when it is below 1 the current NPC is
// deleted (FUN_0050c4a0), a row is drawn at random and the NPC appears
// there (index FUN_004b4b00, position and direction of the row,
// FUN_004b4fe0) for KeepTime seconds. A failed index allocation or
// SetMonster ends the whole pass, as in the original. A node without
// rows also ends it (the break at 0x4b1487), which cannot happen here.
func (w *World) monsterMoveSecond() {
	now := w.now()
	for _, n := range w.dus().moves {
		if len(n.Rows) == 0 {
			return
		}
		if now.Sub(n.Tick) < time.Second {
			continue
		}
		n.Tick = now
		n.Remain = int(n.Until - now.Unix())
		if n.Remain >= 1 {
			continue
		}
		if validIdx(n.Index) {
			if m := w.objects[n.Index]; m.State != StateEmpty {
				w.deleteMonster(m)
			}
		}
		r := n.Rows[monsterRand()%len(n.Rows)]
		idx := w.allocMonsterIndex()
		if idx < 0 {
			return
		}
		m := w.objects[idx]
		m.reset()
		m.State = StateConnected
		m.X, m.Y, m.TX, m.TY = r.X, r.Y, r.X, r.Y
		m.Dir, m.Map = r.Dir, r.Map
		if !w.setMonster(m, r.Monster) {
			w.deleteMonster(m)
			return
		}
		n.Index = idx
		n.Until = now.Unix() + int64(r.KeepTime)
		w.log.Info("[Monster Move] Spawn Monster", "class", r.Monster, "map", r.Map, "x", r.X, "y", r.Y)
	}
}
