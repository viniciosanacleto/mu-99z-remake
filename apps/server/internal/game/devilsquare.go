package game

import (
	"math"
	"time"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/devilsquare"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/packet"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/store"
)

// Devil Square (CDevilSquare, 0x46dcf0..0x4705d0; Event/DevilSquare.dat).
// Six squares (maps 9 for 0..3, 32 for 4..5) cycle through
// EMPTY -> READY (entry open) -> STAND -> START -> CLEAN -> EMPTY, driven
// by the 1-second timer FUN_0046e260.

type devilSquare struct {
	data *gamedata.DevilSquare
	sq   [devilsquare.Squares]devilsquare.Square
}

func registerDevilsquare(h *hooks) {
	h.start = append(h.start, func(w *World) error {
		ds := &devilSquare{data: &gamedata.DevilSquare{}}
		for i := range ds.sq {
			s := &ds.sq[i]
			s.Index, s.Map, s.LastMin = i, 9, -1
			if i > 3 {
				s.Map = 32
			}
			s.Tick = w.now()
			for k := range s.Users {
				s.Users[k].Index = -1
			}
		}
		w.ev().ds = ds
		if w.configDir != "" {
			d, err := gamedata.LoadDevilSquare(w.configDir)
			if err != nil {
				return err
			}
			ds.data = d
		}
		// FUN_0046de30: start every square when DevilSquareEvent is on.
		for i := range ds.sq {
			if w.dsOn() {
				ds.sq[i].State = devilsquare.StateEmpty
				w.dsEnterEmpty(&ds.sq[i])
			}
		}
		return nil
	})
	h.tick1000 = append(h.tick1000, (*World).dsTick)
	h.worldHandlers[0x90] = (*World).handleDSEnter
	h.eventTimeHandlers[1] = (*World).dsRemainTime
	h.death = append(h.death, func(w *World, killer, victim *Object) {
		if victim.Map != 9 && victim.Map != 32 {
			return
		}
		if victim.Type == TypePlayer {
			w.dsPlayerDied(victim)
		} else {
			w.dsMonsterKilled(victim)
		}
	})
	h.npcTalk[devilsquare.Charon] = func(w *World, o, npc *Object) bool { w.dsCharonTalk(o); return true }
}

func (w *World) ds() *devilSquare { return w.ev().ds }

func (w *World) dsOn() bool { return w.cfg.Int("Event", "DevilSquareEvent", 0) != 0 }

// --- packets -----------------------------------------------------------

// packetDSRanking is FUN_004705d0: C1 size 93 myRank+1 count, then
// 24-byte entries (name[10] + 2 padding, score, experience, zen) — the
// player's own first, then ranks 1..9.
func packetDSRanking(w *World, s *devilsquare.Square, self devilsquare.User) []byte {
	entry := func(u devilsquare.User) []byte {
		b := make([]byte, 24)
		if o := w.objects[u.Index]; o.P != nil && o.P.Char != nil {
			copy(b[:10], o.P.Char.Name)
		}
		packet.PutUint32(b[12:], uint32(u.Score))
		packet.PutUint32(b[16:], uint32(u.Exp))
		packet.PutUint32(b[20:], uint32(u.Money))
		return b
	}
	body := entry(self)
	count := 1
	total := w.dsUserCount(s) + 1
	for count != total && count <= 9 {
		found := false
		for _, u := range s.Users {
			if u.Index >= 0 && u.Index < MaxObjects && u.Rank+1 == count {
				body = append(body, entry(u)...)
				count++
				found = true
				break
			}
		}
		if !found {
			break
		}
	}
	p := []byte{0xC1, byte(5 + len(body)), 0x93, byte(self.Rank + 1), byte(count)}
	return append(p, body...)
}

// --- users ---------------------------------------------------------------

func (w *World) dsUserCount(s *devilsquare.Square) int {
	n := 0
	for _, u := range s.Users {
		if u.Index >= 0 && u.Index < MaxObjects {
			n++
		}
	}
	return n
}

// dsAddUser is FUN_0046eea0 (also counts the entry, obj+0x209c).
func (w *World) dsAddUser(s *devilsquare.Square, o *Object) bool {
	if s.User(o.Index) != nil {
		return false
	}
	for i := range s.Users {
		if u := s.Users[i]; u.Index < 0 || u.Index >= MaxObjects {
			s.Users[i] = devilsquare.User{Index: o.Index}
			o.P.entries.DS++
			w.saveCharacter(o, nil)
			return true
		}
	}
	return false
}

// dsCleanUsers is FUN_0046f040: users that left the game or the map.
func (w *World) dsCleanUsers(s *devilsquare.Square) {
	for i := range s.Users {
		u := s.Users[i].Index
		if u < 0 || u >= MaxObjects {
			continue
		}
		if o := w.objects[u]; o.State != StatePlaying || o.Map != s.Map {
			s.Users[i] = devilsquare.User{Index: -1}
		}
	}
}

// dsUsersSend is FUN_00470140.
func (w *World) dsUsersSend(s *devilsquare.Square, p []byte) {
	for _, u := range s.Users {
		if u.Index >= 0 && u.Index < MaxObjects {
			w.send(w.objects[u.Index], p)
		}
	}
}

// dsUsersMsg is FUN_0046ff50. The original picks the language of
// objects[slot] (the slot number, not the user), kept as is.
func (w *World) dsUsersMsg(s *devilsquare.Square, typ byte, n int, args ...any) {
	for i, u := range s.Users {
		if u.Index >= 0 && u.Index < MaxObjects {
			text := w.messageFor(w.objects[i], n, args...)
			w.send(w.objects[u.Index], packetNotice(typ, 0, 0, 0, 0, 0, text))
		}
	}
}

// --- state machine -------------------------------------------------------

// dsTick is FUN_0046e260.
func (w *World) dsTick() {
	ds := w.ds()
	if ds == nil || !w.dsOn() {
		return
	}
	now := w.now()
	for i := range ds.sq {
		s := &ds.sq[i]
		if now.Sub(s.Tick) < time.Second {
			continue
		}
		s.Tick = now
		s.Remain = int(math.Floor(s.Target.Sub(now).Seconds()))
		switch s.State {
		case devilsquare.StateEmpty:
			w.dsEmptyProc(s)
		case devilsquare.StateReady:
			w.dsReadyProc(s)
		case devilsquare.StateStand:
			w.dsStandProc(s)
		case devilsquare.StateStart:
			w.dsStartProc(s)
		case devilsquare.StateClean:
			w.dsCleanUsers(s)
			if s.Remain < 1 && w.dsOn() {
				s.State = devilsquare.StateEmpty
				w.dsEnterEmpty(s)
			}
		}
	}
}

// minuteOf is the "remain/60, minus one on exact minutes" of the procs.
func minuteOf(remain int) int {
	m := remain / 60
	if remain%60 == 0 {
		m--
	}
	return m
}

// dsEmptyProc is FUN_0046e3a0.
func (w *World) dsEmptyProc(s *devilsquare.Square) {
	d := w.ds().data
	if s.Remain > 0 && s.Remain <= d.ReadyTime*60 {
		if m := minuteOf(s.Remain); s.LastMin != m {
			s.LastMin = m
			if s.Index == 0 {
				w.noticeAllMsg(NoticeGold, 725, m+1)
			}
		}
	}
	if s.Remain < 1 {
		if s.Index == 0 {
			w.noticeAllMsg(NoticeGold, 726)
		}
		if w.dsOn() {
			s.State = devilsquare.StateReady
			w.dsEnterReady(s)
		}
	}
}

// dsReadyProc is FUN_0046e4b0.
func (w *World) dsReadyProc(s *devilsquare.Square) {
	d := w.ds().data
	if s.Remain > 0 {
		w.dsCleanUsers(s)
		if m := minuteOf(s.Remain); s.LastMin != m {
			s.LastMin = m
			if s.Index == 0 && !(d.ReadyTime > 1 && m+1 == d.ReadyTime) {
				w.noticeAllMsg(NoticeGold, 30, m+1)
			}
		}
	}
	if s.Remain > 0 && s.Remain <= 30 && !s.Warned {
		w.sendAll(devilsquare.PacketState(0))
		s.Warned = true
	}
	if s.Remain > 0 {
		return
	}
	if s.Index == 0 {
		w.noticeAllMsg(NoticeGold, 31)
	}
	w.dsUsersMsg(s, NoticeBlue, 32, s.Index+1, d.StandTime)
	if w.dsOn() {
		s.State = devilsquare.StateStand
		w.dsEnterStand(s)
	}
}

// dsStandProc is FUN_0046e610.
func (w *World) dsStandProc(s *devilsquare.Square) {
	w.dsCleanUsers(s)
	if w.dsUserCount(s) == 0 {
		if w.dsOn() {
			s.State = devilsquare.StateEmpty
			w.dsEnterEmpty(s)
		}
		return
	}
	if s.Remain > 0 && s.Remain <= 30 && !s.Warned {
		w.dsUsersSend(s, devilsquare.PacketState(1))
		s.Warned = true
	}
	if s.Remain > 0 {
		return
	}
	w.dsUsersMsg(s, NoticeGold, 33)
	if w.dsOn() {
		s.State = devilsquare.StateStart
		w.dsEnterStart(s)
	}
}

// dsStartProc is FUN_0046e6b0: stages 1..4 when the remaining time drops
// to 80/60/40/20 % of EventTime.
func (w *World) dsStartProc(s *devilsquare.Square) {
	w.dsCleanUsers(s)
	if w.dsUserCount(s) == 0 {
		if w.dsOn() {
			s.State = devilsquare.StateEmpty
			w.dsEnterEmpty(s)
		}
		return
	}
	d := w.ds().data
	if s.Stage < 4 && d.EventTime > 0 {
		limits := [4]int{80, 60, 40, 20}
		if s.Remain*100/(d.EventTime*60) <= limits[s.Stage] {
			s.Stage++
			w.dsSpawnStage(s, s.Stage)
		}
	}
	if s.Remain > 0 && s.Remain <= 30 && !s.Warned {
		w.dsUsersSend(s, devilsquare.PacketState(2))
		s.Warned = true
	}
	if s.Remain > 0 {
		return
	}
	w.dsUsersMsg(s, NoticeGold, 34)
	if w.dsOn() {
		s.State = devilsquare.StateClean
		w.dsEnterClean(s)
	}
}

// dsEnterEmpty is FUN_0046e8e0.
func (w *World) dsEnterEmpty(s *devilsquare.Square) {
	s.Open, s.LastMin, s.Warned, s.Stage = false, -1, false, 0
	w.dsKickUsers(s)
	w.dsClearMobs(s)
	w.dsSyncSchedule(s)
}

// dsSyncSchedule is FUN_0046eaf0: the next start of DevilSquare.dat
// section 1; without one the square goes BLANK.
func (w *World) dsSyncSchedule(s *devilsquare.Square) {
	now := w.now()
	next, ok := nextSchedule(w.ds().data.Schedule, now)
	if !ok {
		s.State = devilsquare.StateBlank
		return
	}
	s.Target = next
	s.Remain = int(math.Floor(next.Sub(now).Seconds()))
}

// dsEnterReady is FUN_0046e930.
func (w *World) dsEnterReady(s *devilsquare.Square) {
	s.Open, s.LastMin, s.Warned, s.Stage = true, -1, false, 0
	s.SetRemain(w.now(), w.ds().data.ReadyTime*60)
}

// dsEnterStand is FUN_0046e990.
func (w *World) dsEnterStand(s *devilsquare.Square) {
	s.Open, s.LastMin, s.Warned = false, -1, false
	s.SetRemain(w.now(), w.ds().data.StandTime*60)
}

// dsEnterStart is FUN_0046e9f0.
func (w *World) dsEnterStart(s *devilsquare.Square) {
	s.Open, s.LastMin, s.Warned = false, -1, false
	w.dsSpawnStage(s, 0)
	s.SetRemain(w.now(), w.ds().data.EventTime*60)
}

// dsEnterClean is FUN_0046ea60: monsters removed, ranking, experience,
// zen, item bags and the ranking window.
func (w *World) dsEnterClean(s *devilsquare.Square) {
	s.Open, s.LastMin, s.Warned = false, -1, false
	w.dsClearMobs(s)
	w.dsRank(s)
	w.dsGiveExperience(s)
	w.dsGiveMoney(s)
	w.dsGiveItemBags(s)
	w.dsSendRanking(s)
	s.SetRemain(w.now(), w.ds().data.CloseTime*60)
}

// dsKickUsers is FUN_0046eff0: gate 27 for every user.
func (w *World) dsKickUsers(s *devilsquare.Square) {
	for i := range s.Users {
		u := s.Users[i].Index
		if u >= 0 && u < MaxObjects {
			w.gateMove(w.objects[u], devilsquare.KickGate)
			s.Users[i] = devilsquare.User{Index: -1}
		}
	}
}

// dsSpawnStage is FUN_0046f970: the EventStageSpawn section 2 rows of the
// square and stage.
func (w *World) dsSpawnStage(s *devilsquare.Square, stage int) {
	for _, r := range w.ev().stage {
		if r.Event != devilsquare.StageSection || r.Level != s.Index || r.Stage != stage {
			continue
		}
		for _, idx := range w.spawnStage(s.Map, r.Monster, r.RegenTime) {
			if len(s.Mobs) >= devilsquare.MaxMobs {
				w.deleteMonster(w.objects[idx])
				continue
			}
			s.Mobs = append(s.Mobs, idx)
		}
	}
}

// dsClearMobs is FUN_0046f6d0.
func (w *World) dsClearMobs(s *devilsquare.Square) {
	for _, idx := range s.Mobs {
		if o := w.objects[idx]; o.State != StateEmpty {
			w.deleteMonster(o)
		}
	}
	s.Mobs = s.Mobs[:0]
}

// dsRank is FUN_0046f140: rank = users with a higher score, or the same
// score in an earlier slot.
func (w *World) dsRank(s *devilsquare.Square) {
	for i := range s.Users {
		me := &s.Users[i]
		if me.Index < 0 || me.Index >= MaxObjects {
			continue
		}
		r := 0
		for j, o := range s.Users {
			if j == i || o.Index < 0 || o.Index >= MaxObjects {
				continue
			}
			if o.Score > me.Score || (o.Score == me.Score && j < i) {
				r++
			}
		}
		me.Rank = r
	}
}

// dsGiveExperience is FUN_0046f2a0: ranks 0..9 get the section 2 value
// times AddEventExperienceRate_AL (no experience under effects 31/42),
// MaxLevelUpEvent levels at most.
func (w *World) dsGiveExperience(s *devilsquare.Square) {
	d := w.ds().data
	for i := range s.Users {
		u := &s.Users[i]
		if u.Index < 0 || u.Index >= MaxObjects || u.Rank >= 10 {
			continue
		}
		o := w.objects[u.Index]
		u.Exp = d.Experience[s.Index][u.Rank] * w.cfg.ALInt("Common", "AddEventExperienceRate", accountLevel(o), 0)
		if w.hooks.moveEffect != nil && (w.hooks.moveEffect(w, o, 31) || w.hooks.moveEffect(w, o, 42)) {
			continue
		}
		if !w.levelUp(o, int64(u.Exp), w.cfg.Int("Common", "MaxLevelUpEvent", 0)) {
			c := o.P.Char
			w.send(o, packetKillExp(0xFFFF, uint32(u.Exp), 0, uint32(c.Experience), o.P.NextExp))
		}
	}
}

// dsGiveMoney is FUN_0046f3f0 (zen capped at 2,000,000,000).
func (w *World) dsGiveMoney(s *devilsquare.Square) {
	d := w.ds().data
	for i := range s.Users {
		u := &s.Users[i]
		if u.Index < 0 || u.Index >= MaxObjects || u.Rank >= 10 {
			continue
		}
		o := w.objects[u.Index]
		u.Money = d.Money[s.Index][u.Rank]
		if v := o.P.Char.Money + int64(u.Money); v <= maxZen {
			o.P.Char.Money = v
		} else {
			o.P.Char.Money = maxZen
		}
		w.sendMoney(o)
	}
}

// dsGiveItemBags is FUN_0046f520: item bag type 1 (Devil Square), index
// = square, dropped at the user's position.
func (w *World) dsGiveItemBags(s *devilsquare.Square) {
	for _, u := range s.Users {
		if u.Index < 0 || u.Index >= MaxObjects || u.Rank >= 10 || w.hooks.itemBag == nil {
			continue
		}
		o := w.objects[u.Index]
		w.hooks.itemBag(w, o, 1, s.Index, o.Map, o.X, o.Y)
	}
}

// dsSendRanking is FUN_004705d0: the ranking window and the C1 20 3F
// score report to the DataServer.
func (w *World) dsSendRanking(s *devilsquare.Square) {
	for _, u := range s.Users {
		if u.Index >= 0 && u.Index < MaxObjects {
			o := w.objects[u.Index]
			w.send(o, packetDSRanking(w, s, u))
			w.reportEventRanking(store.RankingDevilSquare, o, u.Score)
		}
	}
}

// --- kills / deaths ------------------------------------------------------

// dsSquareOf is FUN_0047b370 on section 1 and the matching square.
func (w *World) dsSquareOf(o *Object) *devilsquare.Square {
	lv := w.ev().entry.Level(devilsquare.EntrySection, o.Class, o.P.Char.Level)
	if lv < 0 || lv >= devilsquare.Squares || w.ds() == nil {
		return nil
	}
	return &w.ds().sq[lv]
}

// dsMonsterKilled is FUN_0046fe60: the top hitter's square gets
// MonsterLevel * (square+1) points when the monster belongs to it.
func (w *World) dsMonsterKilled(mon *Object) {
	if mon.Mon == nil {
		return
	}
	top := w.topHitter(mon)
	if top < 0 || top >= MaxObjects {
		return
	}
	p := w.objects[top]
	if p.Type != TypePlayer || p.P == nil || p.P.Char == nil {
		return
	}
	s := w.dsSquareOf(p)
	if s == nil || s.State != devilsquare.StateStart {
		return
	}
	owned := false
	for _, idx := range s.Mobs {
		owned = owned || idx == mon.Index
	}
	if !owned {
		return
	}
	if u := s.User(p.Index); u != nil {
		u.Score += mon.Mon.Level * (s.Index + 1)
	}
}

// dsPlayerDied is FUN_0046fdb0 (from FUN_0050e8a0): a dead user leaves
// the ranking while the square is running.
func (w *World) dsPlayerDied(o *Object) {
	if o.P == nil || o.P.Char == nil {
		return
	}
	if s := w.dsSquareOf(o); s != nil && s.State == devilsquare.StateStart {
		if u := s.User(o.Index); u != nil {
			*u = devilsquare.User{Index: -1}
		}
	}
}

// --- client requests -----------------------------------------------------

// dsCharonTalk is FUN_0046fbc0 (NPC 0xED).
func (w *World) dsCharonTalk(o *Object) {
	if !w.dsOn() {
		return
	}
	lv := w.ev().entry.Level(devilsquare.EntrySection, o.Class, o.P.Char.Level)
	if lv < 0 || lv > 5 {
		w.send(o, packetNotice(NoticeBlue, 0, 0, 0, 0, 0, w.messageFor(o, 35)))
		return
	}
	if w.countItems(o, devilsquare.ItemInvitation, lv+1) == 0 && w.countItems(o, devilsquare.ItemFreeTicket, 0) == 0 &&
		w.cfg.Int("Event", "DevilSquareNPCWithoutEntrance", 0) == 0 {
		w.send(o, []byte{0xC1, 0x07, 0xF3, 0x40, 0x01, 0x02, 0x00})
		return
	}
	if !w.ds().sq[lv].Open {
		w.send(o, []byte{0xC1, 0x07, 0xF3, 0x40, 0x01, 0x03, 0x00})
		return
	}
	w.send(o, packetNpcWindow(4))
}

// handleDSEnter is 90 (FUN_004701f0): [3] square, [4] item slot + 12.
func (w *World) handleDSEnter(o *Object, p []byte) {
	if len(p) < 5 || !w.dsOn() || !questPlayerOK(o) {
		return
	}
	sq, slot := int(p[3]), int(byte(p[4]-12))
	if sq > 5 {
		w.send(o, devilsquare.PacketResult(1))
		return
	}
	al := accountLevel(o)
	if max := w.cfg.ALInt("Event", "DevilSquareMaxEntryCount", al, 0); max != 0 && max <= o.P.entries.DS {
		w.send(o, packetCustomNotice(0, 0, w.messageFor(o, 715, max)))
		return
	}
	if slot >= BagStart+64 {
		w.send(o, devilsquare.PacketResult(1))
		return
	}
	it := o.P.Inventory[slot]
	if it.IsEmpty() || !(it.Index == devilsquare.ItemInvitation && it.Level == sq+1 || it.Index == devilsquare.ItemFreeTicket) {
		w.send(o, devilsquare.PacketResult(1))
		return
	}
	s := &w.ds().sq[sq]
	if !s.Open {
		w.send(o, devilsquare.PacketResult(2))
		return
	}
	lv := w.ev().entry.Level(devilsquare.EntrySection, o.Class, o.P.Char.Level)
	switch {
	case sq < lv:
		w.send(o, devilsquare.PacketResult(3))
		return
	case sq > lv:
		w.send(o, devilsquare.PacketResult(4))
		return
	}
	if w.dsUserCount(s) >= w.cfg.Int("Event", "DevilSquareMaxUser", 0) {
		w.send(o, devilsquare.PacketResult(5))
		return
	}
	if w.cfg.Int("Common", "PKLimitFree", 0) == 0 && int(o.PKLevel) > 4 && w.cfg.Int("Common", "PKLimitEventEntry", 0) != 0 {
		w.send(o, devilsquare.PacketResult(6))
		return
	}
	if !w.dsAddUser(s, o) {
		w.send(o, devilsquare.PacketResult(5))
		return
	}
	w.consumeEntryItem(o, slot)
	w.send(o, devilsquare.PacketResult(0))
	items(o).If = ifState{}
	w.gateMove(o, devilsquare.Gates[sq])
}

// consumeEntryItem is FUN_004970f0: one durability of a stack, otherwise
// the item.
func (w *World) consumeEntryItem(o *Object, slot int) {
	it := o.P.Inventory[slot]
	if w.isStackItem(it) && it.Durability > 1 {
		o.P.Inventory[slot].Durability--
		w.send(o, packetItemDurability(slot, byte(o.P.Inventory[slot].Durability), 0))
		return
	}
	w.deleteInventory(o, slot)
	w.send(o, packetItemDelete(slot, 1))
}

// dsRemainTime is the type 1 branch of 91 (FUN_004d8430): users in the
// square while READY, otherwise the minutes to the next start.
func (w *World) dsRemainTime(o *Object, p []byte) []byte {
	lv := w.ev().entry.Level(devilsquare.EntrySection, o.Class, o.P.Char.Level)
	if lv < 0 || lv > 5 {
		w.send(o, packetCustomNotice(0, 0, w.messageFor(o, 35)))
		return nil
	}
	out := packetEventTime(1)
	if s := &w.ds().sq[lv]; s.State == devilsquare.StateReady {
		out[5] = byte(w.dsUserCount(s))
	} else if next, ok := nextSchedule(w.ds().data.Schedule, w.now()); ok {
		sec := int(math.Floor(next.Sub(w.now()).Seconds()))
		m := sec / 60
		if sec != m*60 {
			m++
		}
		out[4] = byte(m)
	}
	return out
}

// --- 91 (event remaining time) ------------------------------------------

// packetEventTime is C1 07 91 type minutes users extra.
func packetEventTime(typ byte) []byte { return []byte{0xC1, 0x07, 0x91, typ, 0, 0, 0} }

// handleEventTime is 91 (FUN_004d8430).
func (w *World) handleEventTime(o *Object, p []byte) {
	if len(p) < 4 || !questPlayerOK(o) {
		return
	}
	h := w.hooks.eventTimeHandlers[p[3]]
	if h == nil {
		w.send(o, packetEventTime(p[3]))
		return
	}
	if out := h(w, o, p); out != nil {
		w.send(o, out)
	}
}

func registerEventTimeHandler(h *hooks) {
	h.worldHandlers[0x91] = (*World).handleEventTime
}
