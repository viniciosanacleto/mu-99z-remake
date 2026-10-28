package game

import (
	"math"
	"time"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/store"
)

// Devil Square (CDevilSquare, 0x46dcf0..0x4705d0; Event/DevilSquare.dat).
// Six squares (maps 9 for 0..3, 32 for 4..5) cycle through
// EMPTY -> READY (entry open) -> STAND -> START -> CLEAN -> EMPTY, driven
// by the 1-second timer FUN_0046e260.

const (
	dsBlank = iota
	dsEmpty
	dsReady
	dsStand
	dsStart
	dsClean
)

const (
	dsSquares   = 6
	dsMaxUsers  = 50
	dsMaxMobs   = 200
	dsEntrySec  = 1 // EventEntryLevel.dat section
	dsStageSec  = 2 // EventStageSpawn.dat section
	dsKickGate  = 0x1B
	dsCharon    = 0xED
	itemDSInvit = 0x1C13 // Devil's Invitation (14,19), level = square + 1
	itemDSFree  = 0x1A2E // free entrance (13,46)
)

// dsGates are the entry gates of FUN_004701f0 per square.
var dsGates = [dsSquares]int{0x3A, 0x3B, 0x3C, 0x3D, 0x6F, 0x70}

type dsUser struct {
	Index, Score, Rank, Exp, Money int
}

type dsSquare struct {
	Index, State, Map int
	Remain            int       // r[3], seconds
	Target            time.Time // r[4]
	Tick              time.Time // r[5]
	Open              bool      // r[6]
	LastMin           int       // r[7]
	Warned            bool      // r[8]
	Stage             int       // r[9]
	Users             [dsMaxUsers]dsUser
	Mobs              []int // r[0x104], at most 200
}

type devilSquare struct {
	data *gamedata.DevilSquare
	sq   [dsSquares]dsSquare
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
				ds.sq[i].State = dsEmpty
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
	h.npcTalk[dsCharon] = func(w *World, o, npc *Object) bool { w.dsCharonTalk(o); return true }
}

func (w *World) ds() *devilSquare { return w.ev().ds }

func (w *World) dsOn() bool { return w.cfg.Int("Event", "DevilSquareEvent", 0) != 0 }

// --- packets -----------------------------------------------------------

// packetDSResult is C1 04 90 result (FUN_004701f0).
func packetDSResult(r byte) []byte { return []byte{0xC1, 0x04, 0x90, r} }

// packetDSState is C1 04 92 state: 0 entry closing, 1 starting, 2 ending.
func packetDSState(s byte) []byte { return []byte{0xC1, 0x04, 0x92, s} }

// packetDSRanking is FUN_004705d0: C1 size 93 myRank+1 count, then
// 24-byte entries (name[10] + 2 padding, score, experience, zen) — the
// player's own first, then ranks 1..9.
func packetDSRanking(w *World, s *dsSquare, self dsUser) []byte {
	entry := func(u dsUser) []byte {
		b := make([]byte, 24)
		if o := w.objects[u.Index]; o.P != nil && o.P.Char != nil {
			copy(b[:10], o.P.Char.Name)
		}
		u32le(b[12:], uint32(u.Score))
		u32le(b[16:], uint32(u.Exp))
		u32le(b[20:], uint32(u.Money))
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

func (w *World) dsUserCount(s *dsSquare) int {
	n := 0
	for _, u := range s.Users {
		if u.Index >= 0 && u.Index < MaxObjects {
			n++
		}
	}
	return n
}

func (s *dsSquare) user(index int) *dsUser {
	for i := range s.Users {
		if s.Users[i].Index == index {
			return &s.Users[i]
		}
	}
	return nil
}

// dsAddUser is FUN_0046eea0 (also counts the entry, obj+0x209c).
func (w *World) dsAddUser(s *dsSquare, o *Object) bool {
	if s.user(o.Index) != nil {
		return false
	}
	for i := range s.Users {
		if u := s.Users[i]; u.Index < 0 || u.Index >= MaxObjects {
			s.Users[i] = dsUser{Index: o.Index}
			o.P.entries.DS++
			w.saveCharacter(o, nil)
			return true
		}
	}
	return false
}

// dsCleanUsers is FUN_0046f040: users that left the game or the map.
func (w *World) dsCleanUsers(s *dsSquare) {
	for i := range s.Users {
		u := s.Users[i].Index
		if u < 0 || u >= MaxObjects {
			continue
		}
		if o := w.objects[u]; o.State != StatePlaying || o.Map != s.Map {
			s.Users[i] = dsUser{Index: -1}
		}
	}
}

// dsUsersSend is FUN_00470140.
func (w *World) dsUsersSend(s *dsSquare, p []byte) {
	for _, u := range s.Users {
		if u.Index >= 0 && u.Index < MaxObjects {
			w.send(w.objects[u.Index], p)
		}
	}
}

// dsUsersMsg is FUN_0046ff50. The original picks the language of
// objects[slot] (the slot number, not the user), kept as is.
func (w *World) dsUsersMsg(s *dsSquare, typ byte, n int, args ...any) {
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
		case dsEmpty:
			w.dsEmptyProc(s)
		case dsReady:
			w.dsReadyProc(s)
		case dsStand:
			w.dsStandProc(s)
		case dsStart:
			w.dsStartProc(s)
		case dsClean:
			w.dsCleanUsers(s)
			if s.Remain < 1 && w.dsOn() {
				s.State = dsEmpty
				w.dsEnterEmpty(s)
			}
		}
	}
}

func (s *dsSquare) setRemain(now time.Time, sec int) {
	s.Remain = sec
	s.Target = now.Add(time.Duration(sec) * time.Second)
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
func (w *World) dsEmptyProc(s *dsSquare) {
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
			s.State = dsReady
			w.dsEnterReady(s)
		}
	}
}

// dsReadyProc is FUN_0046e4b0.
func (w *World) dsReadyProc(s *dsSquare) {
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
		w.sendAll(packetDSState(0))
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
		s.State = dsStand
		w.dsEnterStand(s)
	}
}

// dsStandProc is FUN_0046e610.
func (w *World) dsStandProc(s *dsSquare) {
	w.dsCleanUsers(s)
	if w.dsUserCount(s) == 0 {
		if w.dsOn() {
			s.State = dsEmpty
			w.dsEnterEmpty(s)
		}
		return
	}
	if s.Remain > 0 && s.Remain <= 30 && !s.Warned {
		w.dsUsersSend(s, packetDSState(1))
		s.Warned = true
	}
	if s.Remain > 0 {
		return
	}
	w.dsUsersMsg(s, NoticeGold, 33)
	if w.dsOn() {
		s.State = dsStart
		w.dsEnterStart(s)
	}
}

// dsStartProc is FUN_0046e6b0: stages 1..4 when the remaining time drops
// to 80/60/40/20 % of EventTime.
func (w *World) dsStartProc(s *dsSquare) {
	w.dsCleanUsers(s)
	if w.dsUserCount(s) == 0 {
		if w.dsOn() {
			s.State = dsEmpty
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
		w.dsUsersSend(s, packetDSState(2))
		s.Warned = true
	}
	if s.Remain > 0 {
		return
	}
	w.dsUsersMsg(s, NoticeGold, 34)
	if w.dsOn() {
		s.State = dsClean
		w.dsEnterClean(s)
	}
}

// dsEnterEmpty is FUN_0046e8e0.
func (w *World) dsEnterEmpty(s *dsSquare) {
	s.Open, s.LastMin, s.Warned, s.Stage = false, -1, false, 0
	w.dsKickUsers(s)
	w.dsClearMobs(s)
	w.dsSyncSchedule(s)
}

// dsSyncSchedule is FUN_0046eaf0: the next start of DevilSquare.dat
// section 1; without one the square goes BLANK.
func (w *World) dsSyncSchedule(s *dsSquare) {
	now := w.now()
	next, ok := nextSchedule(w.ds().data.Schedule, now)
	if !ok {
		s.State = dsBlank
		return
	}
	s.Target = next
	s.Remain = int(math.Floor(next.Sub(now).Seconds()))
}

// dsEnterReady is FUN_0046e930.
func (w *World) dsEnterReady(s *dsSquare) {
	s.Open, s.LastMin, s.Warned, s.Stage = true, -1, false, 0
	s.setRemain(w.now(), w.ds().data.ReadyTime*60)
}

// dsEnterStand is FUN_0046e990.
func (w *World) dsEnterStand(s *dsSquare) {
	s.Open, s.LastMin, s.Warned = false, -1, false
	s.setRemain(w.now(), w.ds().data.StandTime*60)
}

// dsEnterStart is FUN_0046e9f0.
func (w *World) dsEnterStart(s *dsSquare) {
	s.Open, s.LastMin, s.Warned = false, -1, false
	w.dsSpawnStage(s, 0)
	s.setRemain(w.now(), w.ds().data.EventTime*60)
}

// dsEnterClean is FUN_0046ea60: monsters removed, ranking, experience,
// zen, item bags and the ranking window.
func (w *World) dsEnterClean(s *dsSquare) {
	s.Open, s.LastMin, s.Warned = false, -1, false
	w.dsClearMobs(s)
	w.dsRank(s)
	w.dsGiveExperience(s)
	w.dsGiveMoney(s)
	w.dsGiveItemBags(s)
	w.dsSendRanking(s)
	s.setRemain(w.now(), w.ds().data.CloseTime*60)
}

// dsKickUsers is FUN_0046eff0: gate 27 for every user.
func (w *World) dsKickUsers(s *dsSquare) {
	for i := range s.Users {
		u := s.Users[i].Index
		if u >= 0 && u < MaxObjects {
			w.gateMove(w.objects[u], dsKickGate)
			s.Users[i] = dsUser{Index: -1}
		}
	}
}

// dsSpawnStage is FUN_0046f970: the EventStageSpawn section 2 rows of the
// square and stage.
func (w *World) dsSpawnStage(s *dsSquare, stage int) {
	for _, r := range w.ev().stage {
		if r.Event != dsStageSec || r.Level != s.Index || r.Stage != stage {
			continue
		}
		for _, idx := range w.spawnStage(s.Map, r.Monster, r.RegenTime) {
			if len(s.Mobs) >= dsMaxMobs {
				w.deleteMonster(w.objects[idx])
				continue
			}
			s.Mobs = append(s.Mobs, idx)
		}
	}
}

// dsClearMobs is FUN_0046f6d0.
func (w *World) dsClearMobs(s *dsSquare) {
	for _, idx := range s.Mobs {
		if o := w.objects[idx]; o.State != StateEmpty {
			w.deleteMonster(o)
		}
	}
	s.Mobs = s.Mobs[:0]
}

// dsRank is FUN_0046f140: rank = users with a higher score, or the same
// score in an earlier slot.
func (w *World) dsRank(s *dsSquare) {
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
func (w *World) dsGiveExperience(s *dsSquare) {
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
func (w *World) dsGiveMoney(s *dsSquare) {
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
func (w *World) dsGiveItemBags(s *dsSquare) {
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
func (w *World) dsSendRanking(s *dsSquare) {
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
func (w *World) dsSquareOf(o *Object) *dsSquare {
	lv := w.ev().entry.Level(dsEntrySec, o.Class, o.P.Char.Level)
	if lv < 0 || lv >= dsSquares || w.ds() == nil {
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
	if s == nil || s.State != dsStart {
		return
	}
	owned := false
	for _, idx := range s.Mobs {
		owned = owned || idx == mon.Index
	}
	if !owned {
		return
	}
	if u := s.user(p.Index); u != nil {
		u.Score += mon.Mon.Level * (s.Index + 1)
	}
}

// dsPlayerDied is FUN_0046fdb0 (from FUN_0050e8a0): a dead user leaves
// the ranking while the square is running.
func (w *World) dsPlayerDied(o *Object) {
	if o.P == nil || o.P.Char == nil {
		return
	}
	if s := w.dsSquareOf(o); s != nil && s.State == dsStart {
		if u := s.user(o.Index); u != nil {
			*u = dsUser{Index: -1}
		}
	}
}

// --- client requests -----------------------------------------------------

// dsCharonTalk is FUN_0046fbc0 (NPC 0xED).
func (w *World) dsCharonTalk(o *Object) {
	if !w.dsOn() {
		return
	}
	lv := w.ev().entry.Level(dsEntrySec, o.Class, o.P.Char.Level)
	if lv < 0 || lv > 5 {
		w.send(o, packetNotice(NoticeBlue, 0, 0, 0, 0, 0, w.messageFor(o, 35)))
		return
	}
	if w.countItems(o, itemDSInvit, lv+1) == 0 && w.countItems(o, itemDSFree, 0) == 0 &&
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
		w.send(o, packetDSResult(1))
		return
	}
	al := accountLevel(o)
	if max := w.cfg.ALInt("Event", "DevilSquareMaxEntryCount", al, 0); max != 0 && max <= o.P.entries.DS {
		w.send(o, packetCustomNotice(0, 0, w.messageFor(o, 715, max)))
		return
	}
	if slot >= BagStart+64 {
		w.send(o, packetDSResult(1))
		return
	}
	it := o.P.Inventory[slot]
	if it.IsEmpty() || !(it.Index == itemDSInvit && it.Level == sq+1 || it.Index == itemDSFree) {
		w.send(o, packetDSResult(1))
		return
	}
	s := &w.ds().sq[sq]
	if !s.Open {
		w.send(o, packetDSResult(2))
		return
	}
	lv := w.ev().entry.Level(dsEntrySec, o.Class, o.P.Char.Level)
	switch {
	case sq < lv:
		w.send(o, packetDSResult(3))
		return
	case sq > lv:
		w.send(o, packetDSResult(4))
		return
	}
	if w.dsUserCount(s) >= w.cfg.Int("Event", "DevilSquareMaxUser", 0) {
		w.send(o, packetDSResult(5))
		return
	}
	if w.cfg.Int("Common", "PKLimitFree", 0) == 0 && int(o.PKLevel) > 4 && w.cfg.Int("Common", "PKLimitEventEntry", 0) != 0 {
		w.send(o, packetDSResult(6))
		return
	}
	if !w.dsAddUser(s, o) {
		w.send(o, packetDSResult(5))
		return
	}
	w.consumeEntryItem(o, slot)
	w.send(o, packetDSResult(0))
	items(o).If = ifState{}
	w.gateMove(o, dsGates[sq])
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
	lv := w.ev().entry.Level(dsEntrySec, o.Class, o.P.Char.Level)
	if lv < 0 || lv > 5 {
		w.send(o, packetCustomNotice(0, 0, w.messageFor(o, 35)))
		return nil
	}
	out := packetEventTime(1)
	if s := &w.ds().sq[lv]; s.State == dsReady {
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
