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

func init() {
	startHooks = append(startHooks, func(w *World) error {
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

// patenteAll sends to dst the patente of every playing player (the loop
// around FUN_00452250 in FUN_00514700, FUN_00515160 and FUN_005167b0).
func (w *World) patenteAll(dst *Object) {
	if !dst.IsPlayer() || dst.State != StatePlaying {
		return
	}
	for i := PlayerStart; i < MaxObjects; i++ {
		if t := w.objects[i]; t.State == StatePlaying && t.IsPlayer() {
			w.send(dst, packetPatente(t.Index, w.patenteRank(t)))
		}
	}
}

// patenteViewport is the end of FUN_00514700 (every viewport cycle).
func (w *World) patenteViewport(o *Object) { w.patenteAll(o) }
