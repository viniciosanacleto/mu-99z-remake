package game

import (
	"time"
)

// Offline modes of the original emulator (docs/server-plan/
// 10e-customs.md, "Modo offline"). Three flags keep a player
// object in the world after its socket is closed:
//
//   - obj+0x2110 CHelperOffline (/offline and index 21, FUN_004888e0);
//   - obj+0x20d9 the offline auto attack (/offattack, FUN_00445d90);
//   - obj+0x249e the offline personal shop (/offstore, FUN_00469030).
//
// The original closes the socket (closesocket, obj+0x14 = -1) and leaves
// the object in state 3: every send to it is dropped because the socket is
// gone. Here the object gets a detached conn whose send is a no-op, so the
// rest of the game keeps treating it as a playing character. The object is
// removed (gObjDel, FUN_0050c4a0, with the character save) when the
// account logs in again (JoinServer "already connected", FUN_004a2490), by
// /dcfriend (FUN_005102a0) or when the offline shop is closed.

// offlineState is obj+0x2100..0x2110 and 0x22b4.
type offlineState struct {
	Helper bool      // obj+0x2110
	X, Y   int       // obj+0x210c / 0x210e: position when it went offline
	Since  time.Time // obj+0x2100 (tick) and 0x22b4 (time64)
}

func init() {
	hardCodedHandlers["/offline"] = func(w *World, o *Object, arg string) { w.helperOffline(o) }
	commandHandlers[21] = func(w *World, o *Object, c command, arg string) { w.helperOffline(o) }
	tick1000Hooks = append(tick1000Hooks, (*World).offlineSecond)
}

// isOffline reports whether o is kept in the world without a client (one
// of obj+0x20d9, 0x2110, 0x249e).
func isOffline(o *Object) bool {
	if o.P == nil || o.P.cust == nil {
		return false
	}
	c := o.P.cust
	return c.Offline.Helper || c.Attack.Offline || c.Store.Offline != 0
}

// detachedConn is the socket of an offline object: closed, every send is
// dropped (the original sends to socket -1 fail the same way).
func detachedConn(index int, ip string) *conn {
	c := &conn{index: index, ip: ip, done: make(chan struct{})}
	c.once.Do(func() { close(c.done) })
	return c
}

// detached reports whether o lost its socket to an offline mode.
func detached(o *Object) bool { return o.conn != nil && o.conn.nc == nil }

// detachClient closes the client socket of o and keeps the object: the
// reader goroutine's disconnect is ignored because o.conn no longer is
// that connection (World.disconnected compares them).
func (w *World) detachClient(o *Object) {
	old := o.conn
	ip := ""
	if old != nil {
		ip = old.ip
	}
	o.conn = detachedConn(o.Index, ip)
	if o.P != nil {
		o.P.LastPacket = w.now() // obj+0x5c = GetTickCount()
	}
	if old != nil {
		old.close()
	}
}

// helperOffline is CHelperOffline::Enable (FUN_004888e0): a playing
// character with its socket, not already offline, with no interface open,
// not in live state 0x20, not dying nor teleporting, outside the event maps
// 11..23, stays on the map without its client. No delay, level or charge.
func (w *World) helperOffline(o *Object) {
	if o.Type != TypePlayer || o.State != StatePlaying || o.P == nil || o.P.Char == nil ||
		o.conn == nil || detached(o) || isOffline(o) {
		return
	}
	if ifBusy(o) || o.Live == LiveTeleport || w.dying(o) || o.Teleporting {
		w.customNotice(o, 1, 2, "Feche as interfaces antes de usar /offline.")
		return
	}
	if o.Map >= 11 && o.Map <= 23 {
		w.customNotice(o, 1, 2, "Nao e permitido usar /offline dentro de eventos.")
		return
	}
	c := custom(o)
	c.Offline = offlineState{Helper: true, X: o.X, Y: o.Y, Since: w.now()}
	w.reconnectSave(o) // FUN_004e1350 (reconnect.go)
	w.detachClient(o)
	w.log.Info("[OfflineMode] Offline enabled", "index", o.Index, "account", o.P.Account.Login,
		"name", o.P.Char.Name, "map", o.Map, "x", o.X, "y", o.Y)
}

// offlineSecond is the offline part of the 1-second client pass
// (FUN_00510810): FUN_00488890 (helper), FUN_00468820 (shop) and the
// offline attack branch refresh obj+0x5c so the 60-second timeout never
// fires; a closing offline shop (state 2) counts its 5 seconds down.
func (w *World) offlineSecond() {
	now := w.now()
	for i := PlayerStart; i < MaxObjects; i++ {
		o := w.objects[i]
		if o.Type != TypePlayer || o.State == StateEmpty || !isOffline(o) {
			continue
		}
		o.P.LastPacket = now
		s := &o.P.cust.Store
		if s.Offline == 2 {
			s.Countdown--
			if s.Countdown <= 0 {
				w.removeOffline(o)
			}
		}
	}
}

// removeOffline ends every offline mode of o and deletes the object
// (FUN_00488aa0 / FUN_00444d90 / FUN_00468880 followed by gObjDel
// FUN_0050c4a0, which saves the character and logs the account out).
// Each first deletes the DataServer reconnect row (C1 12 C0 01 name
// server); gObjDel then stores a new record (FUN_004c4b40 ->
// FUN_004e1350). Only /offline clears its flag before gObjDel: the
// offline attack (obj+0x20d8) and shop (obj+0x249e) flags are cleared
// after it, so that record keeps them (reconnect.go).
func (w *World) removeOffline(o *Object) {
	if !isOffline(o) {
		return
	}
	c := o.P.cust
	w.reconnectForget(o.P.Char.Name)
	c.Offline.Helper = false
	w.log.Info("[OfflineMode] Offline removed", "index", o.Index, "name", o.P.Char.Name)
	w.removePlayer(o)
	c.Attack.Offline = false
	c.Attack.Active = false
	c.Store.Offline = 0
}

// offlineKick is the offline part of JGAccountAlreadyConnectedRecv
// (FUN_004a2490) and of FUN_005102a0: an offline object of the account is
// removed at once. It returns false for a normal client.
func (w *World) offlineKick(o *Object) bool {
	if !isOffline(o) {
		return false
	}
	w.removeOffline(o)
	return true
}
