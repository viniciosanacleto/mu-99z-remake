package game

import (
	"context"
	"math"
	"time"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/item"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/store"
)

// Blood Castle (CBloodCastle, 0x4137b0..0x417b20; Event/BloodCastle.dat).
// Seven castles on maps 11..17 cycle EMPTY -> READY -> STAND -> START ->
// CLEAN like the Devil Square. START has five steps (obj r[9]): kill
// users*40 monsters, the bridge opens (3 s), break the castle gate, kill
// users*2 magic skeletons, break the crystal statue; whoever brings the
// Archangel weapon it drops to the Archangel (NPC 0xE8) wins for the party.

const (
	bcCastles    = 7
	bcMaxUsers   = 50
	bcEntrySec   = 0 // EventEntryLevel.dat / EventStageSpawn.dat section
	bcKickGate   = 0x16
	bcArchangel  = 0xE8
	bcMessenger  = 0xE9
	bcGateClass  = 0x83
	bcStatueMin  = 0x84
	itemBCCloak  = 0x1A12 // Invisibility Cloak (13,18), level = castle + 1
	itemBCFree   = 0x1A2F // free entrance (13,47)
	itemBCWeapon = 0x1A13 // Archangel weapon (13,19), level 0..2
)

var bcGates = [bcCastles]int{0x42, 0x43, 0x44, 0x45, 0x46, 0x47, 0x50}

// Rectangles (x1, y1, x2, y2) of the attribute changes (0x54792c,
// 0x5479b0, 0x5479b4).
var (
	bcEntranceRect = [4]byte{13, 15, 15, 23}
	bcBridgeRect   = [4]byte{13, 70, 15, 75}
	bcStatueRects  = [3][4]byte{{13, 76, 15, 79}, {11, 80, 25, 89}, {8, 80, 10, 83}}
)

// bcMagicSkeleton are the classes counted in step 3 (the others in step 0).
func bcMagicSkeleton(class int) bool {
	switch class {
	case 0x59, 0x5F, 0x70, 0x76, 0x7C, 0x82, 0x8F, 0x1B1:
		return true
	}
	return false
}

type bcUser struct {
	Index, Score, Exp, Money int
}

type bcDelayed struct {
	At    time.Time
	Index int
	Pkt   []byte
}

type bcCastle struct {
	Index, State, Map int
	Remain            int
	Target, Tick      time.Time
	Open              bool // r[6]
	LastMin           int  // r[7]
	Warned            bool // r[8]
	Step              int  // r[9]
	Kills, Required   int  // r[10], r[11]
	StepTick          time.Time
	WeaponSerial      int64 // r[13], -1 = none
	WeaponItem        int   // r[14] map item
	Carrier           int   // r[15]
	WeaponLevel       int   // r[16]
	Winner            int   // r[17]
	Users             [bcMaxUsers]bcUser
	Gate, Statue      int // r[0xda], r[0xdb]
	Mobs              []int
	Delayed           []bcDelayed
}

type bloodCastle struct {
	data *gamedata.BloodCastle
	c    [bcCastles]bcCastle
}

func init() {
	startHooks = append(startHooks, func(w *World) error {
		bc := &bloodCastle{data: &gamedata.BloodCastle{}}
		for i := range bc.c {
			c := &bc.c[i]
			c.Index, c.Map, c.Tick = i, 11+i, w.now()
			c.resetFight()
			for k := range c.Users {
				c.Users[k].Index = -1
			}
		}
		w.ev().bc = bc
		if w.dataDir != "" {
			d, err := gamedata.LoadBloodCastle(w.dataDir)
			if err != nil {
				return err
			}
			bc.data = d
		}
		// FUN_00413880.
		for i := range bc.c {
			if w.bcOn() {
				bc.c[i].State = dsEmpty
				w.bcEnterEmpty(&bc.c[i])
			}
		}
		return nil
	})
	tick1000Hooks = append(tick1000Hooks, (*World).bcTick)
	worldHandlers[0x9A] = (*World).handleBCEnter
	eventTimeHandlers[2] = (*World).bcRemainTime
	npcTalkHooks[bcArchangel] = func(w *World, o, npc *Object) bool { w.bcArchangelTalk(o); return true }
	npcTalkHooks[bcMessenger] = func(w *World, o, npc *Object) bool { w.bcMessengerTalk(o); return true }
	deathHooks = append(deathHooks, func(w *World, killer, victim *Object) {
		if victim.Map < 11 || victim.Map > 17 {
			return
		}
		if victim.Type == TypePlayer {
			w.bcPlayerDied(victim)
		} else {
			w.bcMonsterKilled(victim)
		}
	})
	itemPickedHooks = append(itemPickedHooks, (*World).bcWeaponPicked)
	itemDroppedHooks = append(itemDroppedHooks, (*World).bcWeaponDropped)
}

func (w *World) bc() *bloodCastle { return w.ev().bc }

func (w *World) bcOn() bool { return w.cfg.Int("Event", "BloodCastleEvent", 0) != 0 }

func (c *bcCastle) resetFight() {
	c.Open, c.LastMin, c.Warned = false, -1, false
	c.Step, c.Kills, c.Required = 0, 0, 0
	c.StepTick = time.Time{}
	c.WeaponSerial, c.WeaponItem, c.Carrier, c.WeaponLevel, c.Winner = -1, -1, -1, -1, -1
	c.Gate, c.Statue = -1, -1
}

// --- packets -----------------------------------------------------------

// packetBCResult is C1 1D 93 success FF name[10] and, after two bytes the
// original never writes, score exp zen at 17/21/25 (FUN_004178b0).
func packetBCResult(success byte, name string, u bcUser) []byte {
	p := make([]byte, 0x1D)
	p[0], p[1], p[2], p[3], p[4] = 0xC1, 0x1D, 0x93, success, 0xFF
	copy(p[5:15], name)
	u32le(p[17:], uint32(u.Score))
	u32le(p[21:], uint32(u.Exp))
	u32le(p[25:], uint32(u.Money))
	return p
}

// packetBCStatus is C1 0E 9B type remain required kills carrier level+1
// (FUN_00417ab0; 0xFF without a weapon level).
func packetBCStatus(typ byte, remain, required, kills, carrier, level int) []byte {
	p := make([]byte, 0x0E)
	p[0], p[1], p[2], p[3] = 0xC1, 0x0E, 0x9B, typ
	u16le(p[4:], remain)
	u16le(p[6:], required)
	u16le(p[8:], kills)
	u16le(p[10:], carrier)
	p[12] = 0xFF
	if level != 0xFF && level != -1 {
		p[12] = byte(level + 1)
	}
	return p
}

// packetBCState is C1 0E 9B type with zeros and 0xFF (the fixed status
// packets of FUN_00414420 / FUN_004145f0 / FUN_00414910 / FUN_00417b20).
func packetBCState(typ byte) []byte {
	p := make([]byte, 0x0E)
	p[0], p[1], p[2], p[3], p[12] = 0xC1, 0x0E, 0x9B, typ, 0xFF
	return p
}

// packetMapAttr is C1 size 46 00 attr remove count rects (FUN_004da600).
func packetMapAttr(attr byte, remove bool, rects ...[4]byte) []byte {
	p := []byte{0xC1, byte(7 + 4*len(rects)), 0x46, 0x00, attr, 0, byte(len(rects))}
	if remove {
		p[5] = 1
	}
	for _, r := range rects {
		p = append(p, r[:]...)
	}
	return p
}

// --- users ---------------------------------------------------------------

func (c *bcCastle) user(index int) *bcUser {
	for i := range c.Users {
		if c.Users[i].Index == index {
			return &c.Users[i]
		}
	}
	return nil
}

func (c *bcCastle) userCount() int {
	n := 0
	for _, u := range c.Users {
		if u.Index >= 0 && u.Index < MaxObjects {
			n++
		}
	}
	return n
}

// bcAddUser is FUN_00414ec0 (counts obj+0x2098).
func (w *World) bcAddUser(c *bcCastle, o *Object) bool {
	if c.user(o.Index) != nil {
		return false
	}
	for i := range c.Users {
		if u := c.Users[i]; u.Index < 0 || u.Index >= MaxObjects {
			c.Users[i] = bcUser{Index: o.Index}
			o.P.entries.BC++
			w.saveCharacter(o, nil)
			return true
		}
	}
	return false
}

// bcRemoveUser is FUN_00414f70: during START the weapon carrier drops the
// weapon (FUN_00415690) and gets the 9B 02 status; otherwise the weapon
// is deleted from the inventory (FUN_004158e0).
func (w *World) bcRemoveUser(c *bcCastle, index int) {
	u := c.user(index)
	if u == nil {
		return
	}
	if c.State == dsStart {
		w.bcDropCarriedWeapon(c, index)
		if index >= 0 && index < MaxObjects && w.objects[index].State == StatePlaying {
			w.send(w.objects[index], packetBCState(2))
		} else {
			w.bcUsersSend(c, packetBCState(2))
		}
	} else {
		w.bcDeleteCarriedWeapon(c, index)
	}
	*u = bcUser{Index: -1}
}

// bcCleanUsers is FUN_00415070.
func (w *World) bcCleanUsers(c *bcCastle) {
	for _, u := range c.Users {
		if u.Index < 0 || u.Index >= MaxObjects {
			continue
		}
		if o := w.objects[u.Index]; o.State != StatePlaying || o.Map != c.Map {
			w.bcRemoveUser(c, u.Index)
		}
	}
}

func (w *World) bcUsersSend(c *bcCastle, p []byte) {
	for _, u := range c.Users {
		if u.Index >= 0 && u.Index < MaxObjects {
			w.send(w.objects[u.Index], p)
		}
	}
}

// bcUsersMsg is FUN_00417210 (language of objects[slot], as in the
// original).
func (w *World) bcUsersMsg(c *bcCastle, typ byte, n int, args ...any) {
	for i, u := range c.Users {
		if u.Index >= 0 && u.Index < MaxObjects {
			w.send(w.objects[u.Index], packetNotice(typ, 0, 0, 0, 0, 0, w.messageFor(w.objects[i], n, args...)))
		}
	}
}

func bcName(o *Object) string {
	if o != nil && o.P != nil && o.P.Char != nil {
		return o.P.Char.Name
	}
	return ""
}

// --- terrain -------------------------------------------------------------

func (w *World) bcAttr(c *bcCastle, attr byte, set bool, rects [][4]byte) {
	t := w.terrain(c.Map)
	if t == nil {
		return
	}
	for _, r := range rects {
		for x := int(r[0]); x <= int(r[2]); x++ {
			for y := int(r[1]); y <= int(r[3]); y++ {
				if x < 0 || y < 0 || x >= t.Width || y >= t.Height {
					continue
				}
				if set {
					t.Set(x, y, t.Get(x, y)|attr)
				} else {
					t.Set(x, y, t.Get(x, y)&^attr)
				}
			}
		}
	}
}

func (w *World) bcAttrSend(c *bcCastle, attr byte, set bool, rects ...[4]byte) {
	w.bcAttr(c, attr, set, rects)
	w.bcUsersSend(c, packetMapAttr(attr, !set, rects...))
}

// --- state machine -------------------------------------------------------

// bcTick is FUN_00413dd0.
func (w *World) bcTick() {
	bc := w.bc()
	if bc == nil || !w.bcOn() {
		return
	}
	now := w.now()
	for i := range bc.c {
		c := &bc.c[i]
		if now.Sub(c.Tick) < time.Second {
			continue
		}
		c.Tick = now
		c.Remain = int(math.Floor(c.Target.Sub(now).Seconds()))
		switch c.State {
		case dsEmpty:
			w.bcEmptyProc(c)
		case dsReady:
			w.bcReadyProc(c)
		case dsStand:
			w.bcStandProc(c)
		case dsStart:
			w.bcStartProc(c)
		case dsClean:
			w.bcCleanProc(c)
		}
		w.bcDelayedProc(c, now)
	}
}

// bcDelayedProc is FUN_004170e0: packets queued 8 s earlier.
func (w *World) bcDelayedProc(c *bcCastle, now time.Time) {
	keep := c.Delayed[:0]
	for _, d := range c.Delayed {
		if now.Sub(d.At) < 8*time.Second {
			keep = append(keep, d)
			continue
		}
		if d.Index >= 0 && d.Index < MaxObjects && w.objects[d.Index].State == StatePlaying {
			w.send(w.objects[d.Index], d.Pkt)
		}
	}
	c.Delayed = keep
}

// bcEmptyProc is FUN_00413ed0.
func (w *World) bcEmptyProc(c *bcCastle) {
	d := w.bc().data
	if c.Remain > 0 && c.Remain <= d.ReadyTime*60 {
		if m := minuteOf(c.Remain); c.LastMin != m {
			c.LastMin = m
			if c.Index == 0 {
				w.noticeAllMsg(NoticeGold, 727, m+1)
			}
		}
	}
	if c.Remain < 1 && w.bcOn() {
		c.State = dsReady
		w.bcEnterReady(c)
	}
}

// bcReadyProc is FUN_00413f60.
func (w *World) bcReadyProc(c *bcCastle) {
	d := w.bc().data
	if c.Remain > 0 && c.Remain <= d.ReadyTime*60 {
		w.bcCleanUsers(c)
		if m := minuteOf(c.Remain); c.LastMin != m {
			c.LastMin = m
			if c.Index == 0 {
				w.noticeAllMsg(NoticeGold, 0, m+1)
			}
		}
	}
	if c.Remain > 0 && c.Remain <= 30 && !c.Warned {
		w.sendAll(packetDSState(3))
		c.Warned = true
	}
	if c.Remain > 0 {
		return
	}
	if c.Index == 0 {
		w.noticeAllMsg(NoticeGold, 1)
	}
	w.bcUsersMsg(c, NoticeBlue, 2, c.Index+1, d.StandTime)
	if w.bcOn() {
		c.State = dsStand
		w.bcEnterStand(c)
	}
}

// bcStandProc is FUN_004140c0.
func (w *World) bcStandProc(c *bcCastle) {
	w.bcCleanUsers(c)
	if c.userCount() == 0 {
		if w.bcOn() {
			c.State = dsEmpty
			w.bcEnterEmpty(c)
		}
		return
	}
	if c.Remain > 0 && c.Remain <= 30 && !c.Warned {
		w.bcUsersSend(c, packetDSState(4))
		c.Warned = true
	}
	if c.Remain > 0 {
		return
	}
	w.bcUsersMsg(c, NoticeGold, 3)
	if w.bcOn() {
		c.State = dsStart
		w.bcEnterStart(c)
	}
}

// bcStartProc is FUN_00414170 with the steps FUN_00414910 (kills reached:
// the bridge opens), FUN_004149a0 (3 s later the users get the bridge and
// the gate becomes attackable), FUN_00414a30 (gate broken), FUN_00414a80
// (skeletons killed: the statue appears).
func (w *World) bcStartProc(c *bcCastle) {
	w.bcCleanUsers(c)
	if c.userCount() == 0 {
		if w.bcOn() {
			c.State = dsEmpty
			w.bcEnterEmpty(c)
		}
		return
	}
	now := w.now()
	switch c.Step {
	case 0:
		if c.Kills >= c.Required {
			c.Step, c.StepTick = 1, now
			w.bcUsersSend(c, packetBCState(3))
			w.bcAttr(c, 8, false, [][4]byte{bcBridgeRect})
			w.bcSpawnStage(c, 1)
			w.bcUsersMsg(c, NoticeGold, 4)
		}
	case 1:
		if now.Sub(c.StepTick) > 2999*time.Millisecond {
			c.Step, c.StepTick = 2, time.Time{}
			w.bcUsersSend(c, packetMapAttr(8, true, bcBridgeRect))
			w.bcImmune(c.Gate, false)
			w.bcSpawnStage(c, 2)
		}
	case 2:
		if c.Gate < 0 || c.Gate >= MaxObjects {
			c.Step, c.Kills = 3, 0
			c.Required = c.userCount() * 2
			w.bcAttrSend(c, 4, false, bcStatueRects[:]...)
			w.bcSpawnStage(c, 3)
		}
	case 3:
		if c.Kills >= c.Required {
			c.Step = 4
			w.bcSpawnStatue(c)
			w.bcUsersMsg(c, NoticeGold, 5)
			w.bcImmune(c.Statue, false)
			w.bcSpawnStage(c, 4)
		}
	}
	typ := byte(4)
	if c.Step <= 2 {
		typ = 1
	}
	w.bcUsersSend(c, packetBCStatus(typ, c.Remain, c.Required, c.Kills, c.Carrier, c.WeaponLevel))
	if c.Step == 4 && c.Winner >= 0 && c.Winner < MaxObjects {
		w.bcUsersMsg(c, NoticeGold, 12, bcName(w.objects[c.Winner]))
		if w.bcOn() {
			c.State = dsClean
			w.bcEnterClean(c)
		}
		return
	}
	if c.Remain > 0 && c.Remain <= 30 && !c.Warned {
		w.bcUsersSend(c, packetDSState(5))
		c.Warned = true
	}
	if c.Remain > 0 {
		return
	}
	w.bcUsersMsg(c, NoticeGold, 6)
	if w.bcOn() {
		c.State = dsClean
		w.bcEnterClean(c)
	}
}

// bcCleanProc is FUN_004142f0.
func (w *World) bcCleanProc(c *bcCastle) {
	w.bcCleanUsers(c)
	if c.Remain > 0 && c.Remain <= 30 && !c.Warned {
		w.bcUsersSend(c, packetDSState(6))
		c.Warned = true
	}
	if c.Remain < 1 && w.bcOn() {
		c.State = dsEmpty
		w.bcEnterEmpty(c)
	}
}

// bcEnterEmpty is FUN_00414420.
func (w *World) bcEnterEmpty(c *bcCastle) {
	gate, statue := c.Gate, c.Statue
	c.resetFight()
	w.bcUsersSend(c, packetBCState(2))
	for i := range c.Users { // FUN_00415020: gate 22
		if u := c.Users[i].Index; u >= 0 && u < MaxObjects {
			c.Users[i] = bcUser{Index: -1}
			w.gateMove(w.objects[u], bcKickGate)
		}
	}
	w.bcDeleteObject(gate)
	w.bcDeleteObject(statue)
	w.bcClearMobs(c)
	w.bcAttrSend(c, 4, true, bcEntranceRect)      // FUN_004167b0
	w.bcAttr(c, 8, true, [][4]byte{bcBridgeRect}) // FUN_004169d0
	w.bcAttrSend(c, 4, true, bcStatueRects[:]...) // FUN_00416c80
	now := w.now()
	next, ok := nextSchedule(w.bc().data.Schedule, now)
	if !ok {
		c.State = dsBlank
		return
	}
	c.Target = next
	c.Remain = int(math.Floor(next.Sub(now).Seconds()))
}

// bcEnterReady is FUN_00414530.
func (w *World) bcEnterReady(c *bcCastle) {
	c.Open, c.LastMin, c.Warned = true, -1, false
	c.Remain = w.bc().data.ReadyTime * 60
	c.Target = w.now().Add(time.Duration(c.Remain) * time.Second)
}

// bcEnterStand is FUN_00414590.
func (w *World) bcEnterStand(c *bcCastle) {
	c.Open, c.LastMin, c.Warned = false, -1, false
	c.Remain = w.bc().data.StandTime * 60
	c.Target = w.now().Add(time.Duration(c.Remain) * time.Second)
}

// bcEnterStart is FUN_004145f0.
func (w *World) bcEnterStart(c *bcCastle) {
	c.Open, c.LastMin, c.Warned = false, -1, false
	c.Kills = 0
	c.Required = c.userCount() * 40
	w.bcAttrSend(c, 4, false, bcEntranceRect) // FUN_004168c0
	w.bcSpawnGate(c)
	w.bcSpawnStage(c, 0)
	w.bcUsersSend(c, packetBCState(0))
	c.Remain = w.bc().data.EventTime * 60
	c.Target = w.now().Add(time.Duration(c.Remain) * time.Second)
}

// bcEnterClean is FUN_004146c0: the winner's party gets the full rewards
// and the item rewards, the others the second column.
func (w *World) bcEnterClean(c *bcCastle) {
	c.Open, c.LastMin, c.Warned = false, -1, false
	if c.Carrier >= 0 && c.Carrier < MaxObjects {
		w.bcDeleteCarriedWeapon(c, c.Carrier)
	}
	w.bcDeleteObject(c.Gate)
	w.bcDeleteObject(c.Statue)
	c.Gate, c.Statue = -1, -1
	w.bcClearMobs(c)
	for i := range c.Users {
		u := &c.Users[i]
		if u.Index < 0 || u.Index >= MaxObjects {
			continue
		}
		o := w.objects[u.Index]
		win := w.bcWinnerSide(c, o)
		w.bcReward(c, u, win)
		if win {
			w.bcItemReward(c, o)
			w.send(o, packetBCResult(1, bcName(o), *u))
		} else {
			w.send(o, packetBCResult(0, bcName(o), *u))
		}
		w.reportEventRanking(store.RankingBloodCastle, o, u.Score) // FUN_00476c50
	}
	w.bcUsersSend(c, packetBCState(2))
	c.Remain = w.bc().data.CloseTime * 60
	c.Target = w.now().Add(time.Duration(c.Remain) * time.Second)
}

// bcWinnerSide: the deliverer or a member of its party (FUN_004d2380).
func (w *World) bcWinnerSide(c *bcCastle, o *Object) bool {
	if c.Winner < 0 || c.Winner >= MaxObjects {
		return false
	}
	return o.Index == c.Winner || (samePartyHook != nil && samePartyHook(w, o, w.objects[c.Winner]))
}

// bcReward is FUN_00415180 (experience, score = exp/5) and FUN_00415300
// (zen).
func (w *World) bcReward(c *bcCastle, u *bcUser, win bool) {
	d := w.bc().data
	o := w.objects[u.Index]
	col := 1
	if win {
		col = 0
	}
	exp := d.Experience[c.Index][col]
	u.Score = exp / 5
	u.Exp = exp * w.cfg.ALInt("Common", "AddEventExperienceRate", accountLevel(o), 0)
	if moveEffectHook == nil || !(moveEffectHook(w, o, 31) || moveEffectHook(w, o, 42)) {
		if !w.levelUp(o, int64(u.Exp), w.cfg.Int("Common", "MaxLevelUpEvent", 0)) {
			ch := o.P.Char
			w.send(o, packetKillExp(0xFFFF, uint32(u.Exp), 0, uint32(ch.Experience), o.P.NextExp))
		}
	}
	u.Money = d.Money[c.Index][col]
	if v := o.P.Char.Money + int64(u.Money); v <= maxZen {
		o.P.Char.Money = v
	} else {
		o.P.Char.Money = maxZen
	}
	w.sendMoney(o)
}

// bcItemReward is FUN_00415480: castles 1..3 drop three Boxes of Luck +11
// (14,11), a Jewel of Life and a Jewel of Chaos; castles 4..7 four boxes
// +12, a Jewel of Bless, a Jewel of Soul and a Jewel of Chaos, on the
// player's cell.
func (w *World) bcItemReward(c *bcCastle, o *Object) {
	drop := func(index, level int) {
		it := item.Item{Index: index, Level: level, Serial: math.MaxUint32}
		it.Durability = float32(w.itemMaxDurability(it))
		if dropMapItemHook != nil {
			dropMapItemHook(w, o.Map, o.X, o.Y, it, o.Index)
		}
	}
	switch {
	case c.Index <= 2:
		for i := 0; i < 3; i++ {
			drop(0x1C0B, 11)
		}
		drop(0x1C10, 0)
	default:
		for i := 0; i < 4; i++ {
			drop(0x1C0B, 12)
		}
		drop(0x1C0E, 0)
		drop(0x1C0D, 0)
	}
	drop(0x180F, 0)
}

// --- monsters ------------------------------------------------------------

func (w *World) bcDeleteObject(idx int) {
	if idx >= 0 && idx < MaxObjects && w.objects[idx].State != StateEmpty && w.objects[idx].Mon != nil {
		w.deleteMonster(w.objects[idx])
	}
}

// bcClearMobs is FUN_00415aa0.
func (w *World) bcClearMobs(c *bcCastle) {
	for _, idx := range c.Mobs {
		w.bcDeleteObject(idx)
	}
	c.Mobs = c.Mobs[:0]
}

// bcSpawnStage is FUN_00416070: EventStageSpawn section 0 rows of the
// castle and step.
func (w *World) bcSpawnStage(c *bcCastle, stage int) {
	for _, r := range w.ev().stage {
		if r.Event != bcEntrySec || r.Level != c.Index || r.Stage != stage {
			continue
		}
		for _, idx := range w.spawnStage(c.Map, r.Monster, r.RegenTime) {
			if len(c.Mobs) >= 200 {
				w.bcDeleteObject(idx)
				continue
			}
			c.Mobs = append(c.Mobs, idx)
		}
	}
}

// bcSpawnFixed places the gate or the statue (FUN_00415d40 /
// FUN_00415eb0): no regen (obj+0x294 = 0xFFFF), life from BloodCastle.dat
// section 4, effects 0x3F and 0x40 (immune until the step opens it).
func (w *World) bcSpawnFixed(c *bcCastle, class, x, y, life int) int {
	idx := w.allocMonsterIndex()
	if idx < 0 {
		return -1
	}
	o := w.objects[idx]
	o.reset()
	o.State = StateConnected
	o.Map, o.X, o.Y, o.TX, o.TY, o.Dir = c.Map, x, y, x, y, 1
	if !w.setMonster(o, class) {
		w.deleteMonster(o)
		return -1
	}
	o.Mon.Life, o.Mon.MaxLife = float32(life), float32(life)
	o.Mon.Attribute = 3 // removed on death instead of regenerating
	w.bcImmune(idx, true)
	return idx
}

func (w *World) bcSpawnGate(c *bcCastle) {
	if c.Gate >= 0 && c.Gate < MaxObjects {
		return
	}
	c.Gate = w.bcSpawnFixed(c, bcGateClass, 14, 75, w.bc().data.GateLife[c.Index])
}

func (w *World) bcSpawnStatue(c *bcCastle) {
	if c.Statue >= 0 && c.Statue < MaxObjects {
		return
	}
	d := w.bc().data
	c.Statue = w.bcSpawnFixed(c, bcStatueMin+rnd(3), d.StatueX[c.Index], d.StatueY[c.Index], d.StatueLife[c.Index])
}

// bcImmune applies / removes the effects 0x3F and 0x40 (FUN_00478ae0 /
// FUN_00478d30) through the effect module.
func (w *World) bcImmune(idx int, on bool) {
	if idx < 0 || idx >= MaxObjects || eventEffectHook == nil {
		return
	}
	o := w.objects[idx]
	eventEffectHook(w, o, 0x3F, on)
	eventEffectHook(w, o, 0x40, on)
}

// bcMonsterKilled is FUN_00416ec0.
func (w *World) bcMonsterKilled(mon *Object) {
	c := &w.bc().c[mon.Map-11]
	if c.State != dsStart {
		return
	}
	var hitter *Object
	if top := w.topHitter(mon); top >= 0 && top < MaxObjects {
		hitter = w.objects[top]
	}
	switch {
	case mon.Class == bcGateClass && mon.Index == c.Gate && c.Step == 2:
		c.Gate = -1
		w.bcUsersMsg(c, NoticeGold, 10, bcName(hitter))
		return
	case mon.Class >= bcStatueMin && mon.Class <= bcStatueMin+2 && mon.Index == c.Statue && c.Step == 4:
		c.Statue = -1
		w.bcDropWeapon(c, mon, hitter, mon.Class-bcStatueMin)
		w.bcUsersMsg(c, NoticeGold, 11, bcName(hitter))
		w.bcSpawnStage(c, 5)
		return
	}
	owned := false
	for _, idx := range c.Mobs {
		owned = owned || idx == mon.Index
	}
	if !owned || hitter == nil || hitter.Type != TypePlayer || c.user(hitter.Index) == nil {
		return
	}
	counting := c.Step == 0
	if bcMagicSkeleton(mon.Class) {
		counting = c.Step == 3
	}
	if counting && c.Kills < c.Required {
		c.Kills++
	}
}

// bcDropWeapon creates the Archangel weapon at the statue (FUN_004753e0
// through the DataServer: the serial is registered by FUN_00416660).
func (w *World) bcDropWeapon(c *bcCastle, statue, owner *Object, level int) {
	mp, x, y := statue.Map, statue.X, statue.Y
	ownerIdx := -1
	if owner != nil {
		ownerIdx = owner.Index
	}
	w.async(func(ctx context.Context) func() {
		serial, err := w.store.NextItemSerial(ctx)
		return func() {
			if err != nil {
				return
			}
			if c.WeaponSerial == -1 {
				c.WeaponSerial = int64(serial)
			}
			it := item.Item{Index: itemBCWeapon, Level: level, Serial: serial}
			it.Durability = float32(w.itemMaxDurability(it))
			if dropMapItemHook != nil {
				dropMapItemHook(w, mp, x, y, it, ownerIdx)
			}
		}
	})
}

func (w *World) bcCastleOf(o *Object) *bcCastle {
	if o.Map < 11 || o.Map > 17 || w.bc() == nil {
		return nil
	}
	return &w.bc().c[o.Map-11]
}

func (c *bcCastle) isWeapon(it item.Item) bool {
	return it.Index == itemBCWeapon && c.WeaponSerial != -1 && int64(it.Serial) == c.WeaponSerial
}

// bcWeaponPicked is FUN_00416750.
func (w *World) bcWeaponPicked(o *Object, it item.Item) {
	c := w.bcCastleOf(o)
	if c == nil || !c.isWeapon(it) {
		return
	}
	c.Carrier, c.WeaponItem, c.WeaponLevel = o.Index, -1, it.Level
	w.bcUsersMsg(c, NoticeGold, 9, bcName(o))
}

// bcWeaponDropped is FUN_004166b0. The original sets the map item's life
// to "now + remaining seconds" in milliseconds; kept as is.
func (w *World) bcWeaponDropped(o *Object, it item.Item, num int) {
	c := w.bcCastleOf(o)
	if c == nil || !c.isWeapon(it) {
		return
	}
	if t := w.mapItems(o.Map); t != nil && num >= 0 && num < len(t.items) {
		now := w.now()
		t.items[num].Expire = now.Add(time.Duration(c.Remain) * time.Millisecond)
		t.items[num].Created, t.items[num].OwnerUntil = now, now
	}
	c.Carrier, c.WeaponLevel, c.WeaponItem = -1, -1, num
	w.bcUsersMsg(c, NoticeGold, 8, bcName(o))
}

func (w *World) bcWeaponSlot(c *bcCastle, o *Object) int {
	for s, it := range o.P.Inventory {
		if !it.IsEmpty() && c.isWeapon(it) {
			return s
		}
	}
	return -1
}

// bcDropCarriedWeapon is FUN_00415690: the carrier leaving during START
// drops the weapon on its cell (C1 05 23 result slot).
func (w *World) bcDropCarriedWeapon(c *bcCastle, index int) {
	if index != c.Carrier || index < 0 || index >= MaxObjects {
		return
	}
	o := w.objects[index]
	if o.P == nil {
		return
	}
	slot := w.bcWeaponSlot(c, o)
	if slot < 0 {
		return
	}
	it := o.P.Inventory[slot]
	r := byte(0)
	if num := w.addMapItem(o.Map, o.X, o.Y, it, o.Index); num >= 0 {
		r = 1
		w.deleteInventory(o, slot)
		w.equipmentChanged(o, slot)
	}
	w.send(o, packetDropResult(r, byte(slot)))
}

// bcDeleteCarriedWeapon is FUN_004158e0.
func (w *World) bcDeleteCarriedWeapon(c *bcCastle, index int) {
	if index != c.Carrier || index < 0 || index >= MaxObjects || w.objects[index].P == nil {
		return
	}
	o := w.objects[index]
	if slot := w.bcWeaponSlot(c, o); slot >= 0 {
		w.deleteInventory(o, slot)
		w.send(o, packetItemDelete(slot, 1))
		c.WeaponSerial, c.WeaponItem, c.Carrier, c.WeaponLevel = -1, -1, -1, -1
	}
}

// bcPlayerDied is FUN_00416e00: a user dying during START gets the
// losing rewards, its result 8 s later (FUN_004179e0) and leaves.
func (w *World) bcPlayerDied(o *Object) {
	c := w.bcCastleOf(o)
	if c == nil || c.State != dsStart || o.P == nil {
		return
	}
	u := c.user(o.Index)
	if u == nil {
		return
	}
	win := w.bcWinnerSide(c, o)
	w.bcReward(c, u, win)
	var s byte
	if win {
		s = 1
	}
	c.Delayed = append(c.Delayed, bcDelayed{At: w.now(), Index: o.Index, Pkt: packetBCResult(s, bcName(o), *u)})
	w.bcRemoveUser(c, o.Index)
}

// --- client requests -----------------------------------------------------

// bcArchangelTalk is FUN_004162c0: delivering the weapon (C1 07 F3 40 01
// 17 00 accepted, 18 refused, 2E already delivered).
func (w *World) bcArchangelTalk(o *Object) {
	if !w.bcOn() {
		return
	}
	c := w.bcCastleOf(o)
	if c == nil {
		return
	}
	reply := func(r byte) { w.send(o, []byte{0xC1, 0x07, 0xF3, 0x40, 0x01, r, 0x00}) }
	if c.Winner >= 0 && c.Winner < MaxObjects {
		reply(0x2E)
		return
	}
	if c.State != dsStart || c.user(o.Index) == nil || o.Index != c.Carrier {
		reply(0x18)
		return
	}
	slot := w.bcWeaponSlot(c, o)
	if slot < 0 {
		reply(0x18)
		return
	}
	w.deleteInventory(o, slot)
	w.send(o, packetItemDelete(slot, 1))
	c.WeaponSerial, c.WeaponItem, c.Carrier, c.WeaponLevel = -1, -1, -1, -1
	c.Winner = o.Index
	reply(0x17)
}

// bcMessengerTalk is FUN_00416460 (NPC 0xE9).
func (w *World) bcMessengerTalk(o *Object) {
	if !w.bcOn() {
		return
	}
	lv := w.ev().entry.Level(bcEntrySec, o.Class, o.P.Char.Level)
	if lv < 0 || lv > 6 {
		w.send(o, packetCustomNotice(0, 0, w.messageFor(o, 7)))
		return
	}
	if w.countItems(o, itemBCCloak, lv+1) == 0 && w.countItems(o, itemBCFree, 0) == 0 &&
		w.cfg.Int("Event", "BloodCastleNPCWithoutEntrance", 0) == 0 {
		w.send(o, []byte{0xC1, 0x07, 0xF3, 0x40, 0x01, 0x15, 0x00})
		return
	}
	if !w.bc().c[lv].Open {
		w.send(o, []byte{0xC1, 0x07, 0xF3, 0x40, 0x01, 0x14, 0x00})
		return
	}
	w.send(o, packetNpcWindow(6))
}

// packetBCEnterResult is C1 04 9A result (FUN_004174b0).
func packetBCEnterResult(r byte) []byte { return []byte{0xC1, 0x04, 0x9A, r} }

// handleBCEnter is 9A (FUN_004174b0): [3] castle (1-based), [4] slot.
func (w *World) handleBCEnter(o *Object, p []byte) {
	if len(p) < 5 || !w.bcOn() || !questPlayerOK(o) {
		return
	}
	castle, slot := int(p[3]-1), int(p[4])
	if castle > 6 {
		w.send(o, packetBCEnterResult(1))
		return
	}
	if max := w.cfg.ALInt("Event", "BloodCastleMaxEntryCount", accountLevel(o), 0); max != 0 && max <= o.P.entries.BC {
		w.send(o, packetCustomNotice(0, 0, w.messageFor(o, 713, max)))
		return
	}
	if slot > 0x4B {
		w.send(o, packetBCEnterResult(1))
		return
	}
	it := o.P.Inventory[slot]
	if it.IsEmpty() || (it.Index != itemBCCloak && it.Index != itemBCFree) ||
		(it.Index == itemBCCloak && it.Level != castle+1) {
		w.send(o, packetBCEnterResult(1))
		return
	}
	c := &w.bc().c[castle]
	if !c.Open {
		w.send(o, packetBCEnterResult(2))
		return
	}
	lv := w.ev().entry.Level(bcEntrySec, o.Class, o.P.Char.Level)
	switch {
	case castle < lv:
		w.send(o, packetBCEnterResult(3))
		return
	case castle > lv:
		w.send(o, packetBCEnterResult(4))
		return
	}
	if c.userCount() >= w.cfg.Int("Event", "BloodCastleMaxUser", 0) {
		w.send(o, packetBCEnterResult(5))
		return
	}
	if w.cfg.Int("Common", "PKLimitFree", 0) == 0 && int(o.PKLevel) > 4 && w.cfg.Int("Common", "PKLimitEventEntry", 0) != 0 {
		w.send(o, packetBCEnterResult(7))
		return
	}
	if !w.bcAddUser(c, o) {
		w.send(o, packetBCEnterResult(5))
		return
	}
	w.consumeEntryItem(o, slot)
	w.send(o, packetBCEnterResult(0))
	items(o).If = ifState{}
	w.gateMove(o, bcGates[castle])
}

// bcRemainTime is the type 2 branch of 91 (FUN_004d8430).
func (w *World) bcRemainTime(o *Object, p []byte) []byte {
	lv := w.ev().entry.Level(bcEntrySec, o.Class, o.P.Char.Level)
	if lv < 0 || lv > 6 {
		w.send(o, packetCustomNotice(0, 0, w.messageFor(o, 7)))
		return nil
	}
	out := packetEventTime(2)
	if c := &w.bc().c[lv]; c.State == dsReady {
		out[5] = byte(c.userCount())
	} else if next, ok := nextSchedule(w.bc().data.Schedule, w.now()); ok {
		sec := int(math.Floor(next.Sub(w.now()).Seconds()))
		m := sec / 60
		if sec != m*60 {
			m++
		}
		out[4] = byte(m)
	}
	return out
}
