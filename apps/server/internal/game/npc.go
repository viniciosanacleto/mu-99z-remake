package game

import (
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/packet"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/frame"
)

// NPC talk: 30 (FUN_004c2eb0) and the window close 31 (FUN_004c3170).

// packetObjectChat is C1 size 01 idxHi idxLo text\0 (FUN_004d9c60): an
// object (NPC) speaks; the text is cut at 59 bytes.
func packetObjectChat(index int, text string) []byte {
	if len(text) > 0x3B {
		text = text[:0x3B]
	}
	p := packet.New(frame.C1, 0x01, -1, len(text)+6)
	p[3], p[4] = byte(index>>8), byte(index)
	copy(p[5:], text)
	return p
}

// handleNpcTalk is 30: [3..4] NPC index BE. The player must be playing,
// alive, without logout countdown, not moving servers, and the NPC (type 3)
// on the same map less than 6 cells away.
func (w *World) handleNpcTalk(o *Object, p []byte) {
	if len(p) < 5 || o.State != StatePlaying || !o.IsPlayer() || !o.Alive ||
		o.P.LogoutCountdown > 0 || o.P.Moving {
		return
	}
	idx := int(p[3])<<8 | int(p[4])
	if idx >= MaxObjects {
		return
	}
	npc := w.objects[idx]
	if npc.Type != TypeNPC || npc.State != StatePlaying || npc.Map != o.Map || distance(o, npc) >= 6 {
		return
	}
	w.npcTalk(o, npc)
}

// npcTalk is the CNpcTalk part of FUN_004c1490: the first NpcTalk.txt row
// matching the NPC (class, map, X, Y; "*" = any) makes the NPC speak, then
// the class handler (shops, warehouse, ...) registered by the other
// modules runs.
func (w *World) npcTalk(o, npc *Object) bool {
	// CMonsterSchedule (FUN_004bb8d0, monsterschedule.go) is checked first
	// by FUN_004c2eb0: an NPC of a closed group answers NotTimeMessage.
	if w.hooks.npcSchedule != nil && w.hooks.npcSchedule(w, o, npc) {
		return true
	}
	// BridgeFunction_OnNpcTalk(npc, user) (FUN_004e6f10, lua.go) follows;
	// a non-zero result ends the talk.
	if w.scriptNpcTalk(npc, o) {
		return true
	}
	// CQuest NPC talk (FUN_004dd850) runs first and ends the talk when it
	// opens the quest window.
	if w.hooks.questTalk != nil && w.hooks.questTalk(w, o, npc) {
		return true
	}
	if w.hooks.npcPreTalk != nil && w.hooks.npcPreTalk(w, o, npc) { // Luke Helper (lukehelper.go)
		return true
	}
	for _, e := range w.mw().talk {
		if e.Class == npc.Class && (e.Map == -1 || e.Map == npc.Map) &&
			(e.X == -1 || e.X == npc.X) && (e.Y == -1 || e.Y == npc.Y) {
			w.send(o, packetObjectChat(npc.Index, e.Message))
			break
		}
	}
	if w.hooks.npcCommand != nil && w.hooks.npcCommand(w, o, npc) { // CustomNPCCommand (custom_world.go)
		return true
	}
	if h := w.hooks.npcTalk[npc.Class]; h != nil {
		return h(w, o, npc)
	}
	return false
}

// handleNpcClose is 31 (FUN_004c3170).
func (w *World) handleNpcClose(o *Object, p []byte) {
	if o.State != StatePlaying || !o.IsPlayer() || o.P.LogoutCountdown > 0 || o.P.Moving {
		return
	}
	if w.hooks.npcClose != nil {
		w.hooks.npcClose(w, o)
	}
}

func registerNpc(h *hooks) {
	h.worldHandlers[0x30] = (*World).handleNpcTalk
	h.worldHandlers[0x31] = (*World).handleNpcClose
}
