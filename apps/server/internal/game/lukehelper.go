package game

import "github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"

// CCustomLukeHelper (Custom/CustomLukeHelper.txt, CustomLukeHelperSwitch
// of GameServerInfo - Custom.dat): at each start time an NPC (class 258)
// appears on a random spot for EventTime minutes; the first player to talk
// to it gets fireworks and the bag of the spot. Object (DAT_012524d0): +4
// state, +8 remain, +0xc target, +0x10 EventTime, +0x14 last minute,
// +0x18 tick, +0x1c chosen spot, +0x20 NPC index, +0x24 switch, +0x28 the
// spots (std::map by row number), +0x38 the start times.

const (
	lukeWait        = 1
	lukeOut         = 2
	lukeAlarmSecs   = 300 // the announcements start below 5 minutes
	lukeDefaultBag  = 0x15
	msgLukeMinutes  = 0x212
	msgLukeAppeared = 0x213
	msgLukeFindIt   = 0x214
	msgLukeLeft     = 0x215
	msgLukeReward   = 0x216
	msgLukeFound    = 0x217
)

type lukeHelper struct {
	on      bool
	data    *gamedata.CustomLukeHelper
	State   int
	Remain  int
	Target  int64
	LastMin int
	Key     int
	Monster int
}

func (w *World) luke() *lukeHelper {
	x := w.wcx()
	if x.luke == nil {
		x.luke = &lukeHelper{data: &gamedata.CustomLukeHelper{}, LastMin: -1, Monster: -1}
	}
	return x.luke
}

func init() {
	startHooks = append(startHooks, loadLukeHelper)
	tick1000Hooks = append(tick1000Hooks, (*World).lukeSecond)
	npcPreTalkHook = (*World).lukeTalk
}

// npcPreTalkHook runs after the quest talk and before the NpcTalk lines
// (FUN_004c2eb0); true ends the talk (npc.go).
var npcPreTalkHook func(w *World, o, npc *Object) bool

// loadLukeHelper is FUN_00467550 and the start-up part of FUN_004e8a50
// (state 1, NPC removed, FUN_00467d30).
func loadLukeHelper(w *World) error {
	w.wcx().luke = nil
	l := w.luke()
	l.on = w.cfg.Int("Custom", "CustomLukeHelperSwitch", 0) != 0
	if w.dataDir != "" {
		d, err := gamedata.LoadCustomLukeHelper(w.dataDir)
		if err != nil {
			return err
		}
		l.data = d
	}
	if l.on {
		l.State = lukeWait
		w.lukeRemove()
		w.lukeSchedule()
	}
	return nil
}

// lukeSchedule is FUN_00467d30.
func (w *World) lukeSchedule() {
	l := w.luke()
	now := w.now()
	next, ok := nextSchedule(l.data.Schedule, now)
	if !ok {
		if l.on {
			l.State = 0
		}
		return
	}
	l.Target = next.Unix()
	l.Remain = int(l.Target - now.Unix())
}

// lukeRemove deletes the NPC (FUN_0050c4a0) and resets the minute.
func (w *World) lukeRemove() {
	l := w.luke()
	if validIdx(l.Monster) {
		if m := w.objects[l.Monster]; m.State != StateEmpty {
			w.deleteMonster(m)
		}
		l.Monster, l.LastMin = -1, -1
	}
}

// lukeBack is the "back to state 1" sequence (FUN_00467cf0).
func (w *World) lukeBack() {
	l := w.luke()
	if l.on {
		l.State = lukeWait
		w.lukeRemove()
		w.lukeSchedule()
	}
}

// lukeSecond is FUN_00467870.
func (w *World) lukeSecond() {
	l := w.luke()
	if !l.on {
		return
	}
	l.Remain = int(l.Target - w.now().Unix())
	switch l.State {
	case lukeWait:
		w.lukeWaitSecond()
	case lukeOut:
		if l.Remain < 1 { // FUN_00467c20
			w.noticeAllMsg(NoticeGold, msgLukeLeft)
			w.lukeBack()
		}
	}
}

// lukeWaitSecond is FUN_004678e0: below 5 minutes the minutes are
// announced (0x212); at the start a spot is drawn (rand() % spots) and
// announced with its map name (0x213, 0x214), the NPC appears and stays
// EventTime minutes.
func (w *World) lukeWaitSecond() {
	l := w.luke()
	if l.Remain > 0 && l.Remain < lukeAlarmSecs {
		m := l.Remain / 60
		if l.Remain%60 == 0 {
			m--
		}
		if l.LastMin != m {
			l.LastMin = m
			w.noticeAllMsg(NoticeGold, msgLukeMinutes, l.LastMin+1)
		}
	}
	if l.Remain >= 1 {
		return
	}
	if n := len(l.data.Spots); n > 0 {
		l.Key = l.data.Spots[monsterRand()%n].Key
	}
	s := w.lukeSpot()
	if s == nil {
		w.lukeBack()
		return
	}
	name := "Unknown"
	if info, ok := w.data.Maps[s.Map]; ok {
		name = info.Name
	}
	w.noticeAllMsg(NoticeGold, msgLukeAppeared, name)
	w.noticeAllMsg(NoticeGold, msgLukeFindIt)
	if l.on {
		l.State = lukeOut
		w.lukeSpawn()
		l.Remain = l.data.EventTime * 60
		l.Target = w.now().Unix() + int64(l.Remain)
	}
}

func (w *World) lukeSpot() *gamedata.LukeSpot {
	l := w.luke()
	for i := range l.data.Spots {
		if l.data.Spots[i].Key == l.Key {
			return &l.data.Spots[i]
		}
	}
	return nil
}

// lukeSpawn is FUN_00467e70: the NPC appears on a random cell around
// (StartX, StartY) within (EndX-StartX, EndY-StartY) (FUN_0050c860, 10
// tries, else the start cell) with a random direction.
func (w *World) lukeSpawn() {
	l := w.luke()
	if validIdx(l.Monster) {
		return
	}
	s := w.lukeSpot()
	if s == nil {
		w.lukeBack()
		return
	}
	idx := w.allocMonsterIndex()
	if idx < 0 {
		return
	}
	x, y, ok := w.randomNearCell(s.Map, s.StartX, s.StartY, s.EndX-s.StartX, s.EndY-s.StartY, 10)
	if !ok {
		x, y = s.StartX, s.StartY
	}
	m := w.objects[idx]
	m.reset()
	m.State = StateConnected
	m.X, m.Y, m.TX, m.TY = x, y, x, y
	m.Dir = monsterRand() & 7
	m.Map = s.Map
	if !w.setMonster(m, s.Class) {
		w.deleteMonster(m)
		return
	}
	l.Monster = idx
	w.log.Info("[Luke Helper] SetMonster", "map", s.Map, "x", x, "y", y)
}

// lukeTalk is FUN_00468060: talking to the helper while it is out gives
// ten fireworks around the player (to it and its viewport), the helper
// says 0x216, everybody gets 0x217 (name), the helper leaves and the bag
// of the spot ("*" = 0x15) drops at the player.
func (w *World) lukeTalk(o, npc *Object) bool {
	l := w.luke()
	if l.State != lukeOut || npc.Index != l.Monster {
		return false
	}
	for i := 0; i < 10; i++ {
		ry := monsterRand()
		rx := monsterRand()
		pk := packetFirework(0, byte(o.X-4+(rx%5)*2), byte(o.Y-4+(ry%5)*2))
		w.send(o, pk)
		w.broadcast(o, pk)
	}
	w.send(o, packetObjectChat(npc.Index, w.messageFor(o, msgLukeReward)))
	w.noticeAllMsg(NoticeGold, msgLukeFound, charName(o))
	w.lukeBack()
	if s := w.lukeSpot(); s != nil {
		bag := lukeDefaultBag
		if s.Bag != -1 {
			bag = s.Bag
		}
		if itemBagHook != nil {
			itemBagHook(w, o, bag, -1, o.Map, o.X, o.Y)
		}
	}
	return true
}

// lukeWindow is the "[Luke Helper]" row of FUN_00458a70: closed in state
// 0, the remaining time in state 1.
func (w *World) lukeWindow(o *Object) []windowRow {
	l := w.luke()
	if !l.on {
		return nil
	}
	row := windowRow{Name: w.messageFor(o, 0x2F3), Open: true}
	switch l.State {
	case 0:
		row.Open = false
	case lukeWait:
		row.Remain = l.Remain
	}
	return []windowRow{row}
}
