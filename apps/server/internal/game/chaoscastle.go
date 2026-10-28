package game

import (
	"math"
	"time"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/bloodcastle"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/chaoscastle"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/devilsquare"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/item"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/packet"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/store"
)

// Chaos Castle (CChaosCastle, 0x4301a0..0x434080; Event/ChaosCastle.dat).
// Six castles on maps 18..23 (record of 0x748 bytes from this+4) cycle
// EMPTY -> READY -> STAND -> START -> CLEAN like the Devil Square, driven
// by the 1-second timer FUN_00430670. During START the floor around the
// castle turns into holes in three steps as users and monsters die; who
// stands on a hole falls. The survivor with the most kills wins when no
// monster is left.

type ccCastle struct {
	Index, State, Map int
	Remain            int       // r[3]
	Target            time.Time // r[4]
	Tick              time.Time // r[5]
	Open              bool      // r[6]
	LastMin           int       // r[7]
	Warned            bool      // r[8]
	Step              int       // r[9]: hole step 0..3
	Total             int       // r[10]: users at START + monsters spawned
	Winner            int       // r[11]
	Users             [chaoscastle.MaxUsers]chaoscastle.User
	Mobs              [chaoscastle.MaxMobs]int
	Delayed           []bcDelayed // record+0x738
}

type chaosCastle struct {
	data *gamedata.ChaosCastle
	c    [chaoscastle.Castles]ccCastle
}

func registerChaoscastle(h *hooks) {
	h.start = append(h.start, func(w *World) error {
		cc := &chaosCastle{data: &gamedata.ChaosCastle{}}
		for i := range cc.c {
			c := &cc.c[i]
			c.Index, c.Map, c.Tick, c.LastMin, c.Winner = i, 18+i, w.now(), -1, -1
			for k := range c.Users {
				c.Users[k].Index = -1
			}
			for k := range c.Mobs {
				c.Mobs[k] = -1
			}
		}
		w.ev().cc = cc
		if w.configDir != "" {
			d, err := gamedata.LoadChaosCastle(w.configDir)
			if err != nil {
				return err
			}
			cc.data = d
		}
		// FUN_004302a0.
		for i := range cc.c {
			if w.ccOn() {
				cc.c[i].State = devilsquare.StateEmpty
				w.ccEnterEmpty(&cc.c[i])
			}
		}
		return nil
	})
	h.tick1000 = append(h.tick1000, (*World).ccTick)
	h.worldHandlers[0xAF] = (*World).handleCCPacket
	h.eventTimeHandlers[4] = (*World).ccRemainTime
	h.chaosCastleState = func(w *World, room int) int { // FUN_00431420
		if w.cc() == nil || room < 0 || room >= chaoscastle.Castles {
			return 0
		}
		return w.cc().c[room].State
	}
	h.death = append(h.death, func(w *World, killer, victim *Object) {
		if victim.Map < 18 || victim.Map > 23 {
			return
		}
		if victim.Type == TypePlayer {
			w.ccPlayerKilled(killer, victim)
		} else {
			w.ccMonsterKilled(killer, victim)
		}
	})
}

func (w *World) cc() *chaosCastle { return w.ev().cc }

func (w *World) ccOn() bool { return w.cfg.Int("Event", "ChaosCastleEvent", 0) != 0 }

// --- packets -----------------------------------------------------------

// packetCCStatus is C1 0E 9B 06 remain total current 00 00 FF 00
// (FUN_00430ab0): the remaining seconds, users + monsters at START and
// the current users + monsters.
func packetCCStatus(remain, total, current int) []byte {
	p := bloodcastle.PacketState(6)
	packet.PutUint16(p[4:], remain)
	packet.PutUint16(p[6:], total)
	packet.PutUint16(p[8:], current)
	return p
}

// --- users ---------------------------------------------------------------

func ccIndex(i int) bool { return i >= 0 && i < MaxObjects }

// user is FUN_004317f0.
func (c *ccCastle) user(index int) *chaoscastle.User {
	if !ccIndex(index) {
		return nil
	}
	for i := range c.Users {
		if c.Users[i].Index == index {
			return &c.Users[i]
		}
	}
	return nil
}

// userCount is FUN_004318d0.
func (c *ccCastle) userCount() int {
	n := 0
	for _, u := range c.Users {
		if ccIndex(u.Index) {
			n++
		}
	}
	return n
}

// mobCount is FUN_004322a0.
func (c *ccCastle) mobCount() int {
	n := 0
	for _, m := range c.Mobs {
		if ccIndex(m) {
			n++
		}
	}
	return n
}

// ccAddUser is FUN_004316e0 (counts the daily entry, obj+0x209a).
func (w *World) ccAddUser(c *ccCastle, o *Object) bool {
	if c.user(o.Index) != nil {
		return false
	}
	for i := range c.Users {
		if !ccIndex(c.Users[i].Index) {
			c.Users[i] = chaoscastle.User{Index: o.Index}
			o.P.entries.CC++
			w.saveCharacter(o, nil)
			return true
		}
	}
	return false
}

// removeUser is FUN_00431790.
func (c *ccCastle) removeUser(index int) {
	if u := c.user(index); u != nil {
		*u = chaoscastle.User{Index: -1}
	}
}

// ccCleanUsers is FUN_00431870: users that left the game or the map.
func (w *World) ccCleanUsers(c *ccCastle) {
	for i := range c.Users {
		u := c.Users[i].Index
		if !ccIndex(u) {
			continue
		}
		if o := w.objects[u]; o.State != StatePlaying || o.Map != c.Map {
			c.Users[i] = chaoscastle.User{Index: -1}
		}
	}
}

// ccUsersSend is FUN_00433810.
func (w *World) ccUsersSend(c *ccCastle, p []byte) {
	for _, u := range c.Users {
		if ccIndex(u.Index) {
			w.send(w.objects[u.Index], p)
		}
	}
}

// ccUsersMsg is FUN_00433630: each user gets the text in its own
// language (unlike the Devil Square / Blood Castle versions).
func (w *World) ccUsersMsg(c *ccCastle, typ byte, n int, args ...any) {
	for _, u := range c.Users {
		if ccIndex(u.Index) {
			o := w.objects[u.Index]
			w.send(o, packetNotice(typ, 0, 0, 0, 0, 0, w.messageFor(o, n, args...)))
		}
	}
}

// --- terrain -------------------------------------------------------------

func (w *World) ccAttrRects(c *ccCastle, attr byte, set bool, rects [][4]byte) {
	t := w.terrain(c.Map)
	if t == nil {
		return
	}
	for _, r := range rects {
		for x := int(r[0]); x <= int(r[2]); x++ {
			for y := int(r[1]); y <= int(r[3]); y++ {
				chaoscastle.SetAttr(t, x, y, attr, set)
			}
		}
	}
}

// ccSafeZone is FUN_00432570 (set) / FUN_00432640 (clear): attribute 1 on
// x 23..44 and y 75..108 (the loop runs y = 75, 77, .. 107 and also
// writes y + 1), then 46 with the rectangle DAT_00547928 to every user.
func (w *World) ccSafeZone(c *ccCastle, set bool) {
	if t := w.terrain(c.Map); t != nil {
		for x := 23; x <= 44; x++ {
			for y := 75; y <= 108; y += 2 {
				chaoscastle.SetAttr(t, x, y, gamedata.AttrSafeZone, set)
				chaoscastle.SetAttr(t, x, y+1, gamedata.AttrSafeZone, set)
			}
		}
	}
	w.ccUsersSend(c, packetMapAttr(gamedata.AttrSafeZone, !set, chaoscastle.SafeRect))
}

// ccHoles is FUN_00432710 (set) / FUN_004327a0 (clear): attribute 8 on the
// four rectangles of a hole set. No packet: the client opens the holes
// itself on 9B 08/09/0A.
func (w *World) ccHoles(c *ccCastle, set int, on bool) {
	w.ccAttrRects(c, gamedata.AttrNoGround, on, chaoscastle.HoleRects[set][:])
}

// ccCenterHoles is FUN_00432830.
func (w *World) ccCenterHoles(c *ccCastle) {
	w.ccAttrRects(c, gamedata.AttrNoGround, true, chaoscastle.CenterRects[:])
}

func (w *World) ccAttrAt(c *ccCastle, x, y int) byte {
	t := w.terrain(c.Map)
	if t == nil || x < 0 || y < 0 || x >= t.Width || y >= t.Height {
		return 0
	}
	return t.Get(x, y)
}

// --- state machine -------------------------------------------------------

// ccTick is FUN_00430670.
func (w *World) ccTick() {
	cc := w.cc()
	if cc == nil || !w.ccOn() {
		return
	}
	now := w.now()
	for i := range cc.c {
		c := &cc.c[i]
		if now.Sub(c.Tick) < time.Second {
			continue
		}
		c.Tick = now
		c.Remain = int(math.Floor(c.Target.Sub(now).Seconds()))
		switch c.State {
		case devilsquare.StateEmpty:
			w.ccEmptyProc(c)
		case devilsquare.StateReady:
			w.ccReadyProc(c)
		case devilsquare.StateStand:
			w.ccStandProc(c)
		case devilsquare.StateStart:
			w.ccStartProc(c)
		case devilsquare.StateClean:
			w.ccCleanProc(c)
		}
		w.ccDelayedProc(c, now)
	}
}

// ccDelayedProc is FUN_00433500: results queued 8 s earlier.
func (w *World) ccDelayedProc(c *ccCastle, now time.Time) {
	keep := c.Delayed[:0]
	for _, d := range c.Delayed {
		if now.Sub(d.At) < 8*time.Second {
			keep = append(keep, d)
			continue
		}
		if ccIndex(d.Index) && w.objects[d.Index].State == StatePlaying {
			w.send(w.objects[d.Index], d.Pkt)
		}
	}
	c.Delayed = keep
}

// ccEmptyProc is FUN_004307d0.
func (w *World) ccEmptyProc(c *ccCastle) {
	d := w.cc().data
	if c.Remain > 0 && c.Remain <= d.ReadyTime*60 {
		if m := minuteOf(c.Remain); c.LastMin != m {
			c.LastMin = m
			if c.Index == 0 {
				w.noticeAllMsg(NoticeGold, 0x2D8, m+1)
			}
		}
	}
	if c.Remain < 1 && w.ccOn() {
		c.State = devilsquare.StateReady
		w.ccEnterReady(c)
	}
}

// ccReadyProc is FUN_00430860.
func (w *World) ccReadyProc(c *ccCastle) {
	if c.Remain > 0 {
		w.ccCleanUsers(c)
		if m := minuteOf(c.Remain); c.LastMin != m {
			c.LastMin = m
			if c.Index == 0 {
				w.noticeAllMsg(NoticeGold, 0x10, m+1)
			}
		}
	}
	if c.Remain > 0 && c.Remain < 31 && !c.Warned {
		w.sendAll(devilsquare.PacketState(0x0A))
		c.Warned = true
	}
	if c.Remain > 0 {
		return
	}
	if c.Index == 0 {
		w.noticeAllMsg(NoticeGold, 0x11)
	}
	w.ccUsersMsg(c, NoticeBlue, 0x12, c.Index+1, w.cc().data.StandTime)
	if w.ccOn() {
		c.State = devilsquare.StateStand
		w.ccEnterStand(c)
	}
}

// ccStandProc is FUN_004309a0: fewer users than ChaosCastleMinUser
// cancels the castle (msg 0x15).
func (w *World) ccStandProc(c *ccCastle) {
	w.ccCleanUsers(c)
	if c.userCount() < w.cfg.Int("Event", "ChaosCastleMinUser", 0) {
		w.ccUsersMsg(c, NoticeGold, 0x15)
		if w.ccOn() {
			c.State = devilsquare.StateEmpty
			w.ccEnterEmpty(c)
		}
		return
	}
	if c.Remain > 0 && c.Remain < 31 && !c.Warned {
		w.ccUsersSend(c, devilsquare.PacketState(0x0B))
		c.Warned = true
	}
	if c.Remain < 1 {
		w.ccUsersMsg(c, NoticeGold, 0x13)
		if w.ccOn() {
			c.State = devilsquare.StateStart
			w.ccEnterStart(c)
		}
	}
}

// ccStartProc is FUN_00430ab0: the hole steps (FUN_004310d0 /
// FUN_00431150 / FUN_004311d0 at fewer than 41 / 31 / 21 users plus
// monsters), falls, monsters and items on holes, the 9B 06 status, and the
// end when one user is left without monsters or the time is over.
func (w *World) ccStartProc(c *ccCastle) {
	w.ccCleanUsers(c)
	if c.userCount() == 0 {
		if w.ccOn() {
			c.State = devilsquare.StateEmpty
			w.ccEnterEmpty(c)
		}
		return
	}
	limits := [3]int{41, 31, 21}
	if c.Step < 3 && c.userCount()+c.mobCount() < limits[c.Step] {
		c.Step++
		w.ccUsersSend(c, bloodcastle.PacketState(byte(7+c.Step))) // 9B 08 / 09 / 0A
		w.ccHoles(c, c.Step, true)
	}
	w.ccFallProc(c)
	w.ccMoveMobsProc(c)
	w.ccItemsProc(c)
	w.ccUsersSend(c, packetCCStatus(c.Remain, c.Total, c.userCount()+c.mobCount()))
	if c.userCount() != 1 || c.mobCount() != 0 {
		if c.Remain > 0 && c.Remain < 31 && !c.Warned {
			w.ccUsersSend(c, devilsquare.PacketState(0x0C))
			c.Warned = true
		}
		if c.Remain > 0 {
			return
		}
		w.ccUsersMsg(c, NoticeGold, 0x14)
	}
	if w.ccOn() {
		c.State = devilsquare.StateClean
		w.ccEnterClean(c)
	}
}

// ccCleanProc is FUN_00430c30.
func (w *World) ccCleanProc(c *ccCastle) {
	w.ccCleanUsers(c)
	if c.Remain > 0 && c.Remain < 31 && !c.Warned {
		w.ccUsersSend(c, devilsquare.PacketState(0x0D))
		c.Warned = true
	}
	if c.Remain < 1 && w.ccOn() {
		c.State = devilsquare.StateEmpty
		w.ccEnterEmpty(c)
	}
}

func (c *ccCastle) setRemain(now time.Time, sec int) {
	c.Remain = sec
	c.Target = now.Add(time.Duration(sec) * time.Second)
}

// ccEnterEmpty is FUN_00430d50: 9B 07 to the users, users sent to gate
// 22, monsters removed, safe-zone overlay set, holes of steps 1..3 closed,
// center holes opened, next start from the schedule (FUN_00431250; none
// = BLANK).
func (w *World) ccEnterEmpty(c *ccCastle) {
	c.LastMin, c.Winner = -1, -1
	c.Open, c.Warned, c.Step, c.Total = false, false, 0, 0
	w.ccUsersSend(c, bloodcastle.PacketState(7))
	for i := range c.Users { // FUN_00431820
		if u := c.Users[i].Index; ccIndex(u) {
			w.gateMove(w.objects[u], chaoscastle.KickGate)
			c.Users[i] = chaoscastle.User{Index: -1}
		}
	}
	w.ccClearMobs(c)
	w.ccSafeZone(c, true)
	for s := 1; s <= 3; s++ {
		w.ccHoles(c, s, false)
	}
	w.ccCenterHoles(c)
	now := w.now()
	next, ok := nextSchedule(w.cc().data.Schedule, now)
	if !ok {
		c.State = devilsquare.StateBlank
		return
	}
	c.Target = next
	c.Remain = int(math.Floor(next.Sub(now).Seconds()))
}

// ccEnterReady is FUN_00430e00.
func (w *World) ccEnterReady(c *ccCastle) {
	c.Open, c.LastMin, c.Warned = true, -1, false
	c.setRemain(w.now(), w.cc().data.ReadyTime*60)
}

// ccEnterStand is FUN_00430e60: the jackpot and the center holes.
func (w *World) ccEnterStand(c *ccCastle) {
	c.Open, c.LastMin, c.Warned = false, -1, false
	w.ccJackpot(c)
	w.ccCenterHoles(c)
	c.setRemain(w.now(), w.cc().data.StandTime*60)
}

// ccEnterStart is FUN_00430ee0: the safe zone goes away, the monsters
// appear and the users get 9B 05.
func (w *World) ccEnterStart(c *ccCastle) {
	c.Open, c.LastMin, c.Warned = false, -1, false
	c.Total = c.userCount()
	w.ccSafeZone(c, false)
	w.ccSpawn(c)
	w.ccUsersSend(c, bloodcastle.PacketState(5))
	c.setRemain(w.now(), w.cc().data.EventTime*60)
}

// ccEnterClean is FUN_00430fa0: without monsters left the user with the
// highest score wins (FUN_00431970, first slot on ties); every user gets
// its experience and the 93 result, the winner also the items.
func (w *World) ccEnterClean(c *ccCastle) {
	c.Open, c.LastMin, c.Warned = false, -1, false
	c.Winner = -1
	if c.mobCount() < 1 {
		c.Winner = c.bestUser()
	}
	w.ccClearMobs(c)
	for i := range c.Users {
		u := &c.Users[i]
		if !ccIndex(u.Index) {
			continue
		}
		o := w.objects[u.Index]
		w.ccExperience(c, u)
		if u.Index == c.Winner {
			w.ccItemReward(o)
			w.send(o, chaoscastle.PacketResult(1, bcName(o), *u))
		} else {
			w.send(o, chaoscastle.PacketResult(0, bcName(o), *u))
		}
		w.reportEventRanking(store.RankingChaosCastle, o, u.Score) // FUN_00476ce0
	}
	w.ccUsersSend(c, bloodcastle.PacketState(7))
	c.setRemain(w.now(), w.cc().data.CloseTime*60)
}

// bestUser is FUN_00431970.
func (c *ccCastle) bestUser() int {
	best, score := -1, -1
	for _, u := range c.Users {
		if ccIndex(u.Index) && score < u.Score {
			best, score = u.Index, u.Score
		}
	}
	return best
}

// ccJackpot is FUN_00431a40: with ChaosCastleMoneyRate set, a random user
// gets rate * fee * users / 100 zen (capped at 2,000,000,000) and every
// user reads msg 0x2D4.
func (w *World) ccJackpot(c *ccCastle) {
	rate := w.cfg.Int("Event", "ChaosCastleMoneyRate", 0)
	users := c.userCount()
	if rate == 0 || users == 0 {
		return
	}
	var list []int
	for _, u := range c.Users {
		if ccIndex(u.Index) {
			list = append(list, u.Index)
		}
	}
	o := w.objects[list[rnd(len(list))]]
	amount := int64(uint32(rate * chaoscastle.Fee[c.Index] * users / 100))
	if v := o.P.Char.Money + amount; v <= maxZen {
		o.P.Char.Money = v
	} else {
		o.P.Char.Money = maxZen
	}
	w.sendMoney(o)
	w.ccUsersMsg(c, NoticeBlue, 0x2D4, bcName(o), amount)
}

// ccExperience is FUN_00431c30: experience = table1 * users killed +
// table2 * monsters killed (capped at 0x7FFFFFFF), score = experience / 5,
// then times AddEventExperienceRate_AL; nothing under effects 31/42;
// MaxLevelUpEvent levels at most.
func (w *World) ccExperience(c *ccCastle, u *chaoscastle.User) {
	o := w.objects[u.Index]
	if o.State != StatePlaying || o.P == nil || o.P.Char == nil {
		return
	}
	t := w.cc().data.Experience[c.Index]
	capped := func(v int64) int {
		if v < 0 || v > math.MaxInt32 {
			return math.MaxInt32
		}
		return int(v)
	}
	exp := capped(int64(t[0])*int64(u.UserKills) + int64(t[1])*int64(u.MonsterKills))
	u.Exp, u.Score = exp, exp/5
	if exp <= 0 {
		return
	}
	u.Exp = capped(int64(w.cfg.ALInt("Common", "AddEventExperienceRate", accountLevel(o), 0)) * int64(exp))
	if w.hooks.moveEffect != nil && (w.hooks.moveEffect(w, o, 31) || w.hooks.moveEffect(w, o, 42)) {
		return
	}
	if !w.levelUp(o, int64(u.Exp), w.cfg.Int("Common", "MaxLevelUpEvent", 0)) {
		ch := o.P.Char
		w.send(o, packetKillExp(0xFFFF, uint32(u.Exp), 0, uint32(ch.Experience), o.P.NextExp))
	}
}

// ccItemReward is FUN_00431de0: five Boxes of Luck +12 (14,11), a Jewel of
// Bless and a Jewel of Chaos on the winner's cell (FUN_004753e0).
func (w *World) ccItemReward(o *Object) {
	drop := func(index, level int) {
		it := item.Item{Index: index, Level: level, Serial: math.MaxUint32}
		it.Durability = float32(w.itemMaxDurability(it))
		if w.hooks.dropMapItem != nil {
			w.hooks.dropMapItem(w, o.Map, o.X, o.Y, it, o.Index)
		}
	}
	for i := 0; i < 5; i++ {
		drop(0x1C0B, 12)
	}
	drop(0x1C0E, 0)
	drop(0x180F, 0)
}

// queueResult is FUN_00433fb0: the 93 result with success 0, sent 8 s
// later by FUN_00433500.
func (w *World) ccQueueResult(c *ccCastle, u chaoscastle.User) {
	c.Delayed = append(c.Delayed, bcDelayed{At: w.now(), Index: u.Index, Pkt: chaoscastle.PacketResult(0, bcName(w.objects[u.Index]), u)})
}

// --- monsters ------------------------------------------------------------

// ccSpawn is FUN_00432340 / FUN_004323f0: the EventStageSpawn section 1
// rows of the castle (stage 0) become monsters from the map's section-4
// spawn records while users + monsters stay at most 100 (r[10]).
func (w *World) ccSpawn(c *ccCastle) {
	for _, r := range w.ev().stage {
		if r.Event != chaoscastle.StageSection || r.Level != c.Index || r.Stage != 0 {
			continue
		}
		for i, s := range w.mw().spawns {
			if c.Total > 99 {
				return
			}
			if s.Section != gamedata.SpawnEvent || s.Class != r.Monster || s.Map != c.Map {
				continue
			}
			idx := w.allocMonsterIndex()
			if idx < 0 {
				continue
			}
			o := w.objects[idx]
			o.reset()
			o.State = StateConnected
			if !w.setMonsterPosition(o, i) || !w.setMonster(o, r.Monster) || !c.addMob(idx) {
				w.deleteMonster(o)
				continue
			}
			c.Total++
		}
	}
}

// addMob is FUN_00431ef0.
func (c *ccCastle) addMob(idx int) bool {
	for _, m := range c.Mobs {
		if m == idx {
			return false
		}
	}
	for i, m := range c.Mobs {
		if !ccIndex(m) {
			c.Mobs[i] = idx
			return true
		}
	}
	return false
}

// mobSlot is FUN_00431fc0.
func (c *ccCastle) mobSlot(idx int) int {
	if !ccIndex(idx) {
		return -1
	}
	for i, m := range c.Mobs {
		if m == idx {
			return i
		}
	}
	return -1
}

// ccClearMobs is FUN_00432000.
func (w *World) ccClearMobs(c *ccCastle) {
	for i, idx := range c.Mobs {
		if ccIndex(idx) {
			if o := w.objects[idx]; o.State != StateEmpty && o.Mon != nil {
				w.deleteMonster(o)
			}
		}
		c.Mobs[i] = -1
	}
}

// ccFallProc is FUN_004328b0: a user standing on a hole dies (C1 08 17
// with skill 0 and killer 0), gets its experience and its result 8 s later
// and leaves the castle.
func (w *World) ccFallProc(c *ccCastle) {
	for i := range c.Users {
		u := c.Users[i]
		if !ccIndex(u.Index) {
			continue
		}
		o := w.objects[u.Index]
		if w.ccAttrAt(c, o.X, o.Y)&gamedata.AttrNoGround == 0 {
			continue
		}
		if o.P != nil && o.P.Char != nil {
			o.P.Char.Life = 0
		}
		o.Alive = false
		o.Live = LiveDying
		w.mw().deaths[o.Index] = &deadState{Die: 1, Time: w.now(), Respawn: 1}
		m := mv(o)
		m.clearPath()
		if m.TeleportState == 1 {
			m.TeleportState = 0
		}
		p := packetDie(o.Index, 0, 0)
		w.send(o, p)
		w.broadcast(o, p)
		w.ccExperience(c, &c.Users[i])
		w.ccQueueResult(c, c.Users[i])
		c.Users[i] = chaoscastle.User{Index: -1}
	}
}

// ccFreeCell is FUN_004330e0: the first cell of x-8..x+7 (outer) and
// y-8..y+7 (inner) without a character, a block or a hole (outside the
// map counts as blocked).
func (w *World) ccFreeCell(c *ccCastle, x, y int) (int, int, bool) {
	t := w.terrain(c.Map)
	if t == nil {
		return 0, 0, false
	}
	for nx := x - 8; nx < x+8; nx++ {
		for ny := y - 8; ny < y+8; ny++ {
			a := gamedata.AttrBlock
			if nx >= 0 && ny >= 0 && nx < t.Width && ny < t.Height {
				a = t.Get(nx, ny)
			}
			if a&(gamedata.AttrCharacter|gamedata.AttrBlock|gamedata.AttrNoGround) == 0 {
				return nx, ny, true
			}
		}
	}
	return 0, 0, false
}

// ccMoveMobsProc is FUN_00432a10: a monster on a hole moves to the
// nearest free cell (FUN_00511cb0).
func (w *World) ccMoveMobsProc(c *ccCastle) {
	for _, idx := range c.Mobs {
		if !ccIndex(idx) {
			continue
		}
		o := w.objects[idx]
		if w.ccAttrAt(c, o.X, o.Y)&gamedata.AttrNoGround == 0 {
			continue
		}
		if x, y, ok := w.ccFreeCell(c, o.X, o.Y); ok {
			w.resendPosition(o, x, y)
			mv(o).CCPushTick = w.now()
		}
	}
}

// ccItemsProc is FUN_00432b10: map items lying on a hole are removed.
func (w *World) ccItemsProc(c *ccCastle) {
	t := w.mapItems(c.Map)
	if t == nil {
		return
	}
	for i := range t.Items {
		m := &t.Items[i]
		if m.Valid() && w.ccAttrAt(c, m.X, m.Y)&gamedata.AttrNoGround != 0 {
			w.removeMapItem(c.Map, i)
		}
	}
}

// ccKnockBack is FUN_00432eb0: users alive, not teleporting and within 3
// cells of (x, y) are pushed away (FUN_00432f40).
func (w *World) ccKnockBack(c *ccCastle, x, y int) {
	for _, u := range c.Users {
		if !ccIndex(u.Index) {
			continue
		}
		o := w.objects[u.Index]
		if d := w.mw().deaths[o.Index]; d != nil && d.Die != 0 {
			continue
		}
		if mv(o).TeleportState != 0 {
			continue
		}
		if o.X >= x-3 && o.X <= x+3 && o.Y >= y-3 && o.Y <= y+3 {
			w.ccPush(o, x, y)
		}
	}
}

// ccPush is FUN_00432f40: direction away from (x, y) per axis (random
// +1/-1 on the same line: odd rand = -1), distance (rand%2) - dist + 4
// with dist = (int)sqrt(dx*dx + dy*dy); then FUN_00511cb0 and obj+0x438.
func (w *World) ccPush(o *Object, x, y int) {
	dx, dy := o.X-x, o.Y-y
	sign := func(d int) int {
		switch {
		case d < 0:
			return -1
		case d > 0:
			return 1
		case monsterRand()%2 != 0:
			return -1
		}
		return 1
	}
	sx := sign(dx)
	sy := sign(dy)
	dist := int(float32(math.Sqrt(float64(float32(dx*dx + dy*dy)))))
	nx := (monsterRand()%2-dist+4)*sx + o.X
	ny := (monsterRand()%2-dist+4)*sy + o.Y
	w.resendPosition(o, nx, ny)
	mv(o).CCPushTick = w.now()
}

func (w *World) ccCastleOf(o *Object) *ccCastle {
	if o.Map < 18 || o.Map > 23 || w.cc() == nil {
		return nil
	}
	return &w.cc().c[o.Map-18]
}

// ccMonsterKilled is FUN_00433330: a castle monster dying in START may
// knock back the users near it (ChaosCastleBlowUserRate %); the top
// hitter (or the killer) scores 2 and one monster kill.
func (w *World) ccMonsterKilled(killer, mon *Object) {
	c := w.ccCastleOf(mon)
	if c == nil || c.State != devilsquare.StateStart {
		return
	}
	if mon.Mon != nil {
		if top := w.topHitter(mon); ccIndex(top) {
			killer = w.objects[top]
		}
	}
	slot := c.mobSlot(mon.Index)
	if slot < 0 {
		return
	}
	if rnd(100) < w.cfg.Int("Event", "ChaosCastleBlowUserRate", 0) {
		w.ccKnockBack(c, mon.X, mon.Y)
	}
	c.Mobs[slot] = -1
	if killer != nil && killer.Type == TypePlayer {
		if u := c.user(killer.Index); u != nil {
			u.Score += 2
			u.MonsterKills++
		}
	}
}

// ccPlayerKilled is FUN_004331d0: a user dying in START gets its
// experience, its result 8 s later and leaves; a killing user scores 1 and
// one user kill.
func (w *World) ccPlayerKilled(killer, victim *Object) {
	c := w.ccCastleOf(victim)
	if c == nil || c.State != devilsquare.StateStart {
		return
	}
	u := c.user(victim.Index)
	if u == nil {
		return
	}
	w.ccExperience(c, u)
	w.ccQueueResult(c, *u)
	c.removeUser(victim.Index)
	if killer != nil && killer.Type == TypePlayer {
		if k := c.user(killer.Index); k != nil {
			k.Score++
			k.UserKills++
		}
	}
}

// --- client requests -----------------------------------------------------

// handleCCPacket is AF: sub 01 entry, sub 02 move inside the castle.
func (w *World) handleCCPacket(o *Object, p []byte) {
	if len(p) < 4 {
		return
	}
	switch p[3] {
	case 1:
		w.handleCCEnter(o, p)
	case 2:
		w.handleCCMove(o, p)
	}
}

// ccTransformed is obj+0x314 != -1 (FUN_004fa1a0): a transformation ring
// with durability in slot 10 or 11.
func ccTransformed(o *Object) bool {
	for _, s := range []int{10, 11} {
		it := o.P.Inventory[s]
		if it.IsEmpty() || it.Durability == 0 {
			continue
		}
		switch it.Index {
		case 0x1A0A, 0x1A27, 0x1A28, 0x1A29, 0x1A2A, 0x1A44, 0x1A4C, 0x1A4D, 0x1A4E, 0x1A7A, 0x1AA3, 0x1AA4, 0x1AA5:
			return true
		}
	}
	return false
}

// handleCCEnter is AF 01 (FUN_004338c0): [4] is overwritten with the
// castle of the character level, [5] the Armor of Guardsman slot.
func (w *World) handleCCEnter(o *Object, p []byte) {
	if len(p) < 6 || !w.ccOn() || !questPlayerOK(o) {
		return
	}
	lv := w.ev().entry.Level(chaoscastle.EntrySection, o.Class, o.P.Char.Level)
	if lv < 0 || lv > 5 {
		w.send(o, packetCustomNotice(0, 0, w.messageFor(o, 0x16)))
		return
	}
	if max := w.cfg.ALInt("Event", "ChaosCastleMaxEntryCount", accountLevel(o), 0); max != 0 && max <= o.P.entries.CC {
		w.send(o, packetCustomNotice(0, 0, w.messageFor(o, 0x2CA, max)))
		return
	}
	slot := int(p[5])
	refuse := func(r byte) { w.send(o, chaoscastle.PacketEnterResult(r)) }
	if slot > 0x4B || o.P.Inventory[slot].IsEmpty() || o.P.Inventory[slot].Index != chaoscastle.ItemArmor {
		refuse(1)
		return
	}
	if pet := o.P.Inventory[8]; !pet.IsEmpty() && (pet.Index == 0x1A02 || pet.Index == 0x1A03 || pet.Index == 0x1A25) {
		w.send(o, packetCustomNotice(0, 0, w.messageFor(o, 0x9F)))
		return
	}
	if w.partyOf(o) != nil {
		refuse(1)
		return
	}
	if n := guildNumberOf(o); n != 0 {
		if war := w.guildWarOf(n); war != nil && war.active {
			refuse(1)
			return
		}
	}
	if o.Duel != nil && ccIndex(o.Duel.Opponent) {
		refuse(1)
		return
	}
	if o.P.PShop != nil && o.P.PShop.Open {
		refuse(1)
		return
	}
	if ccTransformed(o) {
		w.send(o, packetCustomNotice(0, 0, w.messageFor(o, 0x9F)))
		return
	}
	c := &w.cc().c[lv]
	if !c.Open {
		refuse(2)
		return
	}
	// FUN_004b0540: the castle map must be served by this GameServer.
	if w.data.MapServer != nil && w.data.MapServer.ServerForMap(w.cfg.ServerCode, c.Map) != w.cfg.ServerCode {
		refuse(2)
		return
	}
	if o.P.Char.Money < int64(chaoscastle.Fee[lv]) {
		refuse(7)
		return
	}
	if w.cfg.Int("Common", "PKLimitFree", 0) == 0 && int(o.PKLevel) > 4 && w.cfg.Int("Common", "PKLimitEventEntry", 0) != 0 {
		refuse(8)
		return
	}
	if !w.ccAddUser(c, o) {
		refuse(5)
		return
	}
	o.P.Char.Money -= int64(chaoscastle.Fee[lv])
	w.sendMoney(o)
	w.deleteInventory(o, slot)
	w.send(o, packetItemDelete(slot, 1))
	w.send(o, chaoscastle.PacketEnterResult(0))
	if w.summonOf(o) != nil {
		w.removeSummon(o)
		w.send(o, packetSummonLife(0))
	}
	items(o).If = ifState{}
	w.gateMove(o, chaoscastle.Gates[lv])
}

// handleCCMove is AF 02 (FUN_00433d50): a user inside its castle is moved
// to ([4], [5]) whatever the state.
func (w *World) handleCCMove(o *Object, p []byte) {
	if len(p) < 6 || !questPlayerOK(o) {
		return
	}
	c := w.ccCastleOf(o)
	if c == nil || c.user(o.Index) == nil {
		return
	}
	w.resendPosition(o, int(p[4]), int(p[5]))
	mv(o).CCPushTick = w.now()
}

// ccRemainTime is the type 4 branch of 91 (FUN_004d8430): users in the
// castle while READY (FUN_00431690), otherwise the minutes to the next
// start (FUN_00431470).
func (w *World) ccRemainTime(o *Object, p []byte) []byte {
	lv := w.ev().entry.Level(chaoscastle.EntrySection, o.Class, o.P.Char.Level)
	if lv < 0 || lv > 5 {
		w.send(o, packetCustomNotice(0, 0, w.messageFor(o, 0x16)))
		return nil
	}
	out := packetEventTime(4)
	if c := &w.cc().c[lv]; c.State == devilsquare.StateReady {
		out[5] = byte(c.userCount())
	} else if next, ok := nextSchedule(w.cc().data.Schedule, w.now()); ok {
		sec := int(math.Floor(next.Sub(w.now()).Seconds()))
		m := sec / 60
		if sec != m*60 {
			m++
		}
		out[4] = byte(m)
	}
	return out
}
