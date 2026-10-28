package game

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/packet"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/crypto"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/frame"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/store"
)

// Server moves between instances (map server change): the source sends
// C3 2C B1 00 with the destination address and four authentication codes
// (FUN_004a1c10); the client reconnects and sends B1 01 (FUN_004d89e0),
// which the destination validates (FUN_004a1e70) before loading the
// character on the new map.

// packetServerMove is C3 2C B1 00 ip[16] port u16 serverCode u16 codes[4].
func packetServerMove(ip string, port, serverCode int, codes [4]uint32) []byte {
	p := packet.New(frame.C3, 0xB1, 0x00, 0x2C)
	copy(p[4:19], ip)
	packet.PutUint16(p[20:], port)
	packet.PutUint16(p[22:], serverCode)
	for i, c := range codes {
		packet.PutUint32(p[24+4*i:], c)
	}
	return p
}

// packetMoveAuthResult is C1 05 B1 01 result (FUN_004dadc0).
func packetMoveAuthResult(result byte) []byte {
	p := packet.New(frame.C1, 0xB1, 0x01, 5)
	p[4] = result
	return p
}

// startServerMove saves the character and hands the client over to the
// instance that hosts mp.
func (w *World) startServerMove(o *Object, serverCode, mp, x, y int) {
	var dest *struct {
		addr string
		port int
	}
	for _, s := range w.data.MapServer.Servers {
		if s.Code == serverCode {
			dest = &struct {
				addr string
				port int
			}{s.Address, s.Port}
		}
	}
	if dest == nil || o.P.Char == nil {
		w.systemMessage(o, w.messageFor(o, 0x103))
		return
	}
	t, err := w.auth.BeginServerMove(o.P.Account.Login, o.P.Char.Name, serverCode, mp, x, y)
	if err != nil {
		w.closeClient(o, "server move")
		return
	}
	o.Map, o.X, o.Y = mp, x, y
	w.auth.SetMoving(o.P.Account.Login, true)
	o.P.Moving = true
	w.send(o, packetServerMove(w.publicAddress(dest.addr), dest.port, serverCode, t.Codes))
	if o.State == StatePlaying {
		w.leaveWorld(o)
		w.saveCharacter(o, nil)
		o.State = StateLogged
	}
}

// publicAddress lets the operator override the address sent to clients.
func (w *World) publicAddress(fromFile string) string {
	if w.publicIP != "" {
		return w.publicIP
	}
	return fromFile
}

// handleMoveAuth is B1 01 on the destination instance.
func (w *World) handleMoveAuth(o *Object, p []byte) {
	if o.State != StateConnected {
		w.closeClient(o, "move auth in wrong state")
		return
	}
	if len(p) < 0x45 {
		return
	}
	var serial [16]byte
	copy(serial[:], w.cfg.ServerSerial)
	if !bytes.Equal(p[0x30:0x35], w.cfg.ServerVersion[:]) || !bytes.Equal(p[0x35:0x45], serial[:]) {
		w.send(o, packetMoveAuthResult(0))
		return
	}
	if o.P.LoginPending {
		return
	}
	o.P.LoginAttempts++
	o.P.LoginPending = true
	o.P.ClientTick = binary.LittleEndian.Uint32(p[0x2C:])
	acc := make([]byte, 10)
	copy(acc, p[4:14])
	crypto.Bux(acc)
	account := cString(acc)
	name := cString(p[16:26])
	var codes [4]uint32
	for i := range codes {
		codes[i] = binary.LittleEndian.Uint32(p[0x1C+4*i:])
	}
	c := o.conn
	w.async(func(ctx context.Context) func() {
		t, ok := w.auth.CompleteServerMove(account, name, codes)
		var ch *store.Character
		var err error
		sess := w.auth.Session(account)
		if ok && sess != nil {
			ch, err = w.store.LoadCharacter(ctx, sess.Account.ID, name)
		}
		return func() {
			o := w.objectOf(c)
			if o == nil {
				return
			}
			o.P.LoginPending = false
			if !ok || sess == nil || err != nil {
				if err != nil && !errors.Is(err, store.ErrNotFound) {
					w.log.Error("move auth load", "err", err)
				}
				w.send(o, packetMoveAuthResult(0))
				w.closeClient(o, "MoveAuth failed")
				return
			}
			if !w.connectMemberAllowed(account) && w.cfg.ConnectMemberCheck {
				w.send(o, packetMoveAuthResult(0))
				w.closeClient(o, "MoveAuth ConnectMemberCheck")
				return
			}
			if sess.Account.BlockCode != 0 || sess.Account.Level < w.cfg.ServerLock {
				w.send(o, packetMoveAuthResult(0))
				w.closeClient(o, "MoveAuth blocked")
				return
			}
			w.auth.SetMoving(account, false)
			w.auth.SetSessionServer(account, w.cfg.ServerCode, w)
			o.State = StateLogged
			o.P.Session = sess
			o.P.Account = sess.Account
			o.P.CharListLoaded = false
			w.send(o, packetMoveAuthResult(1))
			ch.Map, ch.X, ch.Y = t.Map, t.X, t.Y
			w.enterWorld(o, ch)
		}
	})
}
