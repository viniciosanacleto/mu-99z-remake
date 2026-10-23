package game

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Push-back of skill hits (docs/server-plan/08c-skills-efeitos-pvp.md,
// "Push-back"):
//
//   - FUN_0040e300: Lightning (3) and the DK skills 19..23 queue message 2
//     on the target, 150 ms later, sent by the attacker (FUN_00510550);
//   - monsters run it through FUN_004b48f0 case 2 and players through
//     FUN_004c4120 case 2; both call FUN_005110e0 while the attacker is
//     alive: one cell in the attacker's direction (reversed one time in
//     three) when the cell is free, then FUN_00511cb0 (C1 07 DF to the
//     object and its viewport);
//   - FUN_005114f0 is the multi-step knock-back of the messages 6 / 7
//     (2 / 3 cells away from the sender, the third cell only for
//     classes 1, 3 and 4); CAttack sends message 6 to the battle soccer
//     ball.

// Object message codes of the push-back (FUN_004b48f0 jump table: 2 -> 2,
// 6 -> 5, 7 -> 6; FUN_004c4120 case 2 for players).
const (
	msgPushBack   = 2
	msgKnockBack2 = 6
	msgKnockBack3 = 7
)

// pushTo moves o to (x, y) with FUN_00511cb0 (a monster that never walked
// gets its old cell first so the occupancy bit follows it).
func (w *World) pushTo(o *Object, x, y int) {
	if o.Move == nil {
		m := mv(o)
		m.OldX, m.OldY = o.X, o.Y
	}
	w.resendPosition(o, x, y)
}

// Classes the push-back never moves (FUN_005110e0 / FUN_005114f0): the
// event and castle objects; the knock-back also spares the mercenaries.
func pushImmuneClass(c int, knock bool) bool {
	switch {
	case c >= 0x20B && c <= 0x210, c >= 0x256 && c <= 0x25A, c >= 0x273 && c <= 0x277, c >= 0x83 && c <= 0x86,
		c == 0x115, c == 0x11B, c == 0x120, c == 0x116, c >= 0xD7 && c <= 0xDB:
		return true
	case c == 0x15C, c == 0x113, c == 0x291, c == 0x2A1, c >= 0x1CB && c <= 0x1CE:
		return true
	case knock && (c == 0x11E || c == 0x11F):
		return true
	}
	return false
}

// pushAllowed is the common gate of FUN_005110e0 / FUN_005114f0: a map of
// MapManager.txt, a player neither teleporting nor with authority 8, a
// class outside the lists and outside Custom/CustomMobCombo.txt
// (FUN_004e8260 / FUN_0044f040), and not the Castle Siege crown user
// riding a Dark Horse (CCastleSiege+0x80, slot 8 = 13,4).
func (w *World) pushAllowed(o *Object, knock bool) bool {
	if _, ok := w.data.Maps[o.Map]; !ok {
		return false
	}
	if o.Type == TypePlayer && (o.Teleporting || (o.Move != nil && o.Move.Authority == 8)) {
		return false
	}
	if pushImmuneClass(o.Class, knock) {
		return false
	}
	for _, c := range w.q().noPush {
		if c == o.Class {
			return false
		}
	}
	if c := w.cs(); c != nil && c.crownUser == o.Index && o.P != nil && o.P.Inventory[8].Index == idxDarkHorse {
		return false
	}
	return true
}

// pushBack is FUN_005110e0 (EDI = o, param = the attacker a).
func (w *World) pushBack(o, a *Object) bool {
	if !w.pushAllowed(o, false) {
		return false
	}
	d := a.Dir & 7
	if rnd(3) == 0 {
		if d < 4 {
			d += 4
		} else {
			d -= 4
		}
	}
	x, y := o.X+pathDirs[d][0], o.Y+pathDirs[d][1]
	attr := w.terrain(o.Map).Get(x, y)
	if attr&0x0F != 0 || (o.Map == csMap && attr&0x10 != 0) {
		return false
	}
	w.pushTo(o, x, y)
	if o.Type == TypePlayer {
		mv(o).PathCount = 0 // obj+0x1d8
	}
	return true
}

// knockBack is FUN_005114f0: steps cells away from a (direction
// FUN_005122d0 of the offset a -> o); a blocked cell (attributes 0x1d)
// turns the direction around for the next step. Steps after the second
// only move classes 1, 3 and 4. The position is always re-sent.
func (w *World) knockBack(o, a *Object, steps int) bool {
	if !w.pushAllowed(o, true) {
		return false
	}
	d := dirTo(o.X-a.X, o.Y-a.Y)
	x, y := o.X, o.Y
	t := w.terrain(o.Map)
	for i := 0; i < steps; i++ {
		if i >= 2 && o.Class != 1 && o.Class != 3 && o.Class != 4 {
			continue
		}
		nx, ny := x+pathDirs[d][0], y+pathDirs[d][1]
		if t.Get(nx, ny)&0x1D == 0 {
			x, y = nx, ny
			continue
		}
		if d+4 > 7 {
			d -= 4
		} else {
			d += 4
		}
	}
	w.pushTo(o, x, y)
	return true
}

// queuePushBack is FUN_00510550(2, attacker, delay, 0) on t.
func (w *World) queuePushBack(t, a *Object, delayMs int) {
	if t.Type != TypePlayer {
		w.sendMsg(t, msgPushBack, a.Index, delayMs, 0)
		return
	}
	q := w.q()
	n := 0
	for _, m := range q.pushes {
		if m.target == t.Index {
			n++
		}
	}
	if n >= 20 { // 20 message slots per object (DAT_01090774)
		return
	}
	q.pushes = append(q.pushes, pushMsg{target: t.Index, sender: a.Index, due: w.now().Add(time.Duration(delayMs) * time.Millisecond)})
}

// pushTick runs the due player push-backs (FUN_004c3700 -> FUN_004c4120
// case 2: only while the sender is alive).
func (w *World) pushTick() {
	q := w.q()
	if len(q.pushes) == 0 {
		return
	}
	now := w.now()
	keep := q.pushes[:0]
	var due []pushMsg
	for _, m := range q.pushes {
		if now.After(m.due) {
			due = append(due, m)
		} else {
			keep = append(keep, m)
		}
	}
	q.pushes = keep
	for _, m := range due {
		o := w.objects[m.target]
		if o.State != StatePlaying || m.sender < 0 || m.sender >= MaxObjects {
			continue
		}
		if a := w.objects[m.sender]; a.Alive {
			w.pushBack(o, a)
		}
	}
}

// monsterPushMessage handles the monster messages 2, 6 and 7 of
// FUN_004b48f0 (cases 2, 5 and 6): sender alive -> push-back / knock-back.
func (w *World) monsterPushMessage(o *Object, code, sender int) {
	if sender < 0 || sender >= MaxObjects || !w.objects[sender].Alive {
		return
	}
	a := w.objects[sender]
	switch code {
	case msgPushBack:
		w.pushBack(o, a)
	case msgKnockBack2:
		w.knockBack(o, a, 2)
	case msgKnockBack3:
		w.knockBack(o, a, 3)
	}
}

// loadMobCombo reads Custom/CustomMobCombo.txt section 0 (FUN_0044ef50,
// at most 100 classes).
func loadMobCombo(dataDir string) ([]int, error) {
	path := filepath.Join(dataDir, "Custom", "CustomMobCombo.txt")
	if _, err := os.Stat(path); err != nil {
		return nil, nil
	}
	secs, err := loadSections(path)
	if err != nil {
		return nil, err
	}
	var out []int
	for _, s := range secs {
		if s.Index != 0 {
			continue
		}
		for _, l := range s.Rows {
			v, err := l.Int(0)
			if err != nil {
				return nil, fmt.Errorf("CustomMobCombo.txt: %w", err)
			}
			if len(out) < 100 {
				out = append(out, v)
			}
		}
	}
	return out, nil
}

func init() {
	startHooks = append(startHooks, func(w *World) error {
		if w.dataDir == "" {
			return nil
		}
		l, err := loadMobCombo(w.dataDir)
		if err != nil {
			return err
		}
		w.q().noPush = l
		return nil
	})
	tick100Hooks = append(tick100Hooks, (*World).pushTick)
}
