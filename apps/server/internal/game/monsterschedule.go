package game

import "github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"

// CMonsterSchedule (Monster/MonsterSchedule.txt, DAT_015b9a08, reader
// FUN_004bb0c0): despite the name it spawns nothing. Its 100 groups
// (0x44 bytes each: +4 state, +8 remaining seconds, +0xc target time,
// +0x10 tick, +0x14 EnableMessage, +0x18 DisableMessage, +0x1c
// ItsNotTimeMessage, +0x20 EnableNpcTime, +0x24 the start times, +0x34
// the NPCs) open and close NPCs on a schedule: while a group is closed
// its NPCs answer ItsNotTimeMessage and do not talk (FUN_004bb8d0, the
// first check of the NPC talk FUN_004c2eb0).

const (
	scheduleGroups = 100
	scheduleOff    = 0 // no start time
	scheduleClosed = 1 // waiting for the next start time
	scheduleOpen   = 2 // open for EnableNpcTime seconds
)

// scheduleGroup is one 0x44-byte group.
type scheduleGroup struct {
	State  int
	Remain int
	Target int64
	data   gamedata.MonsterScheduleGroup
}

// npcScheduleHook is FUN_004bb8d0; true ends the NPC talk.
var npcScheduleHook func(w *World, o, npc *Object) bool

func init() {
	npcScheduleHook = (*World).scheduleBlocksNpc
}

// setMonsterSchedule stores the groups (group numbers outside 0..99 are
// dropped: the original writes past its array) and runs the start-up
// part of FUN_004eb5f0: every group goes to state 1 (FUN_004bb7b0).
func (w *World) setMonsterSchedule(groups map[int]*gamedata.MonsterScheduleGroup) {
	d := w.dus()
	d.schedule = [scheduleGroups]scheduleGroup{}
	for i := range d.schedule {
		g := &d.schedule[i]
		g.data = gamedata.MonsterScheduleGroup{EnableMsg: -1, DisableMsg: -1, NotTimeMsg: -1}
		if src, ok := groups[i]; ok {
			g.data = *src
		}
		w.scheduleWait(g)
	}
}

// scheduleWait is FUN_004bb7b0: state 1 until the next start time
// (CScheduleManager, the earliest of the rows), or state 0 without one.
func (w *World) scheduleWait(g *scheduleGroup) {
	g.State = scheduleClosed
	now := w.now()
	next, ok := nextSchedule(g.data.Schedule, now)
	if !ok {
		g.State = scheduleOff
		return
	}
	g.Target = next.Unix()
	g.Remain = int(g.Target - now.Unix())
}

// monsterScheduleSecond is FUN_004bb560 (called every second by
// FUN_0050a140): Remain = Target - now, then state 1 with Remain < 1
// opens the group (FUN_004bb5f0: EnableMessage to everybody, open for
// EnableNpcTime seconds) and state 2 with Remain < 1 closes it
// (FUN_004bb6b0: DisableMessage, wait for the next start time). The
// messages are gold notices to all (FUN_004c0c70) and -1 sends none.
func (w *World) monsterScheduleSecond() {
	now := w.now().Unix()
	d := w.dus()
	for i := range d.schedule {
		g := &d.schedule[i]
		g.Remain = int(g.Target - now)
		switch g.State {
		case scheduleClosed:
			if g.Remain < 1 {
				if g.data.EnableMsg != -1 {
					w.noticeAllMsg(NoticeGold, g.data.EnableMsg)
				}
				g.State = scheduleOpen
				g.Remain = g.data.NPCSec
				g.Target = now + int64(g.Remain)
			}
		case scheduleOpen:
			if g.Remain < 1 {
				if g.data.DisableMsg != -1 {
					w.noticeAllMsg(NoticeGold, g.data.DisableMsg)
				}
				w.scheduleWait(g)
			}
		}
	}
}

// scheduleBlocksNpc is FUN_004bb8d0: the groups are scanned in order and
// the first NPC row matching the NPC (class, then map, X and Y with -1 =
// any) of a group in state 1 makes the NPC say ItsNotTimeMessage (C1 01,
// FUN_004d9c60, in the player's language) and blocks the talk; one of a
// group in state 2 lets the talk go on. Groups in state 0 are skipped.
func (w *World) scheduleBlocksNpc(o, npc *Object) bool {
	d := w.dus()
	for i := range d.schedule {
		g := &d.schedule[i]
		for _, m := range g.data.Monsters {
			if m.Monster != npc.Class || (m.Map != -1 && m.Map != npc.Map) ||
				(m.X != -1 && m.X != npc.X) || (m.Y != -1 && m.Y != npc.Y) {
				continue
			}
			switch g.State {
			case scheduleClosed:
				if g.data.NotTimeMsg != -1 {
					w.send(o, packetObjectChat(npc.Index, w.messageFor(o, g.data.NotTimeMsg)))
				}
				return true
			case scheduleOpen:
				return false
			}
		}
	}
	return false
}
