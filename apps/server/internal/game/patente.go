package game

import (
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/frame"
)

// CustomPatente (Custom/CustomPatente.txt, FUN_00451f90; C1 0C F3 E5,
// FUN_00452250): the client (Main.dll) shows a rank badge next to the
// players whose patente (obj+0x2500, set by the custom events) is in the
// list. The file is missing in this installation, so the list is empty
// and every packet carries -1.

func registerPatente(h *hooks) {
	h.start = append(h.start, func(w *World) error {
		if w.configDir == "" {
			return nil
		}
		p, err := gamedata.LoadCustomPatente(w.configDir)
		w.wcx().patente = p
		return err
	})
}

// packetPatente is C1 0C F3 E5 [4] index (u32) [8] rank (i32).
func packetPatente(index, rank int) []byte {
	p := newPacket(frame.C1, 0xF3, 0xE5, 0x0C)
	u32le(p[4:], uint32(index))
	u32le(p[8:], uint32(int32(rank)))
	return p
}

// patenteRank is the rank of FUN_00452250: the 1-based position of the
// player's patente in the CustomPatente list, -1 when missing, for a GM
// (Authority > 1) or during the Castle Siege battle (state 7).
func (w *World) patenteRank(o *Object) int {
	rank := -1
	if o.P != nil && o.P.Char != nil {
		for i, v := range w.wcx().patente {
			if o.P.Char.Patente == v {
				rank = i + 1
				break
			}
		}
	}
	if o.Move != nil && o.Move.Authority > 1 {
		return -1
	}
	if w.castle != nil && w.castle.state == csStart { // CCastleSiege +0x1c
		return -1
	}
	return rank
}

// The original sends dst the patente of every playing player at the end
// of each viewport add (FUN_00514700, every second), for each new C2 12
// entry (FUN_00515160) and for the own entry (FUN_005167b0). That is N
// packets per event per player: with ~50 players arriving in the same area
// a player gets 50 x 50 packets at once, overflows the send queue and is
// disconnected (docs/server-plan/18-teste-de-carga.md, U-LOAD-01). Fixed
// here: the client only draws the badge of players it sees, so each
// player gets the patente of its own viewport.

// patenteViewport is the end of FUN_00514700 (every viewport cycle): the
// patente of o and of every player in its viewport.
func (w *World) patenteViewport(o *Object) {
	if !o.IsPlayer() || o.State != StatePlaying {
		return
	}
	w.send(o, packetPatente(o.Index, w.patenteRank(o)))
	for _, e := range o.VP {
		if e.Type != byte(TypePlayer) || (e.State != VPNew && e.State != VPVisible) || !validIdx(e.Index) || e.Index == o.Index {
			continue
		}
		if t := w.objects[e.Index]; t.State == StatePlaying && t.IsPlayer() {
			w.send(o, packetPatente(t.Index, w.patenteRank(t)))
		}
	}
}

// patenteEntry sends viewer the patente of the player of a new C2 12
// entry (the original sent every player's, see above).
func (w *World) patenteEntry(viewer, t *Object) {
	if viewer.IsPlayer() && viewer.State == StatePlaying && t.IsPlayer() && t.State == StatePlaying {
		w.send(viewer, packetPatente(t.Index, w.patenteRank(t)))
	}
}
