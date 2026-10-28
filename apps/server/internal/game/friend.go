package game

import (
	"context"
	"encoding/binary"
	"strings"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/frame"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/store"
)

// Friend list and mail (CFriend). The client heads C0..CB go to the
// DataServer (B0 00..08) whose answers the GameServer turns into client
// packets (FUN_004723a0 case 0xB0). Here the DataServer half runs on the
// store and the Hub.

const (
	maxFriends  = 50   // FUN_00460b20: count > 0x31 => result 3
	mailCost    = 1000 // FUN_00460ee0 / FUN_00460670
	maxMailMemo = 1000
)

// Friend packets (GameServer -> client).

// packetFriendList is C2 size C0 memoTotal 0x32 count + {name[10], server}
// (FUN_00460350).
func packetFriendList(memoTotal int, friends []string, server func(string) byte) []byte {
	p := newPacket(frame.C2, 0xC0, -1, 7+11*len(friends))
	p[4], p[5], p[6] = byte(memoTotal), 0x32, byte(len(friends))
	for i, n := range friends {
		putName(p[7+11*i:], n)
		p[7+11*i+10] = server(n)
	}
	return p
}

// packetFriendRequest is C1 0D C2 name: someone asks to add the player
// (FUN_004604c0).
func packetFriendRequest(name string) []byte {
	p := newPacket(frame.C1, 0xC2, -1, 0x0D)
	putName(p[3:], name)
	return p
}

// packetFriendState is C1 0E C4 name server (FUN_004609b0); 0xFF = offline.
func packetFriendState(name string, server byte) []byte {
	p := newPacket(frame.C1, 0xC4, -1, 0x0E)
	putName(p[3:], name)
	p[13] = server
	return p
}

// packetFriendAddResult is C1 0F C1 result name server (FUN_004614a0).
func packetFriendAddResult(result byte, name string, server byte) []byte {
	p := newPacket(frame.C1, 0xC1, -1, 0x0F)
	p[3] = result
	putName(p[4:], name)
	p[14] = server
	return p
}

// packetFriendDelResult is C1 0E C3 result name (FUN_00460590).
func packetFriendDelResult(result byte, name string) []byte {
	p := newPacket(frame.C1, 0xC3, -1, 0x0E)
	p[3] = result
	putName(p[4:], name)
	return p
}

// packetMailResult is C1 08 C5 result (FUN_00461570).
func packetMailResult(result byte) []byte {
	p := newPacket(frame.C1, 0xC5, -1, 8)
	p[3] = result
	return p
}

// packetMailHeader is C3 50 C6 index(u16) from[10] date[30] subject[32]
// read (FUN_004608a0).
func packetMailHeader(m store.Mail) []byte {
	p := newPacket(frame.C3, 0xC6, -1, 0x50)
	binary.LittleEndian.PutUint16(p[3:], uint16(m.Index))
	putName(p[5:], m.From)
	copy(p[15:44], store.MailDate(m.Date))
	copy(p[45:76], m.Subject)
	if m.Read {
		p[77] = 1
	}
	return p
}

// packetMailRead is C4 size C7 index len photo[18] act dir memo
// (FUN_004606e0).
func packetMailRead(m *store.Mail) []byte {
	memo := m.Memo
	if len(memo) > maxMailMemo {
		memo = memo[:maxMailMemo]
	}
	if i := strings.IndexByte(string(memo), 0); i >= 0 {
		memo = memo[:i]
	}
	p := newPacket(frame.C4, 0xC7, -1, 0x1C+len(memo))
	binary.LittleEndian.PutUint16(p[4:], uint16(m.Index))
	binary.LittleEndian.PutUint16(p[6:], uint16(len(memo)))
	copy(p[8:26], m.Photo)
	p[26], p[27] = m.Act, m.Dir
	copy(p[28:], memo)
	return p
}

// packetMailDelResult is C1 06 C8 result index (FUN_004607e0).
func packetMailDelResult(result byte, index int) []byte {
	p := newPacket(frame.C1, 0xC8, -1, 6)
	p[3] = result
	binary.LittleEndian.PutUint16(p[4:], uint16(index))
	return p
}

func (w *World) friendStore() store.FriendStore {
	fs, _ := w.store.(store.FriendStore)
	return fs
}

func charName(o *Object) string {
	if o.P != nil && o.P.Char != nil {
		return o.P.Char.Name
	}
	return ""
}

// friendReady is the precondition of every CFriend handler: playing player,
// not logging out, not moving between servers.
func friendReady(o *Object) bool {
	return o.State == StatePlaying && o.IsPlayer() && o.P.LogoutCountdown < 1 && !o.P.Moving && o.P.Char != nil
}

// friendNotify is FUN_00412fb0: tell every online friend of name its state
// (server code, or 0xFF when offline is true).
func (w *World) friendNotify(ctx context.Context, name string, offline bool) {
	fs := w.friendStore()
	guid, _, ok, err := fs.FriendMain(ctx, name)
	if err != nil || !ok {
		return
	}
	friends, err := fs.FriendNames(ctx, guid)
	if err != nil {
		return
	}
	state := byte(0xFF)
	if !offline {
		state = w.hub.ServerOf(name)
	}
	for _, f := range friends {
		w.hub.PostToChar(f, func(fw *World, fo *Object) {
			fw.send(fo, packetFriendState(name, state))
		})
	}
}

// friendListRequest is client C0 / world entry (FUN_00460a90 -> DS B0 00
// FUN_00410ed0): create the GUID row, send the list, the pending requests
// (FUN_00412d90) and the mail headers (FUN_004132f0).
func (w *World) friendListRequest(o *Object) {
	fs := w.friendStore()
	if fs == nil {
		return
	}
	name := charName(o)
	c := o.conn
	w.async(func(ctx context.Context) func() {
		if err := fs.FriendGuidCreate(ctx, name); err != nil {
			w.log.Error("friend guid", "err", err)
			return nil
		}
		guid, memoTotal, _, err := fs.FriendMain(ctx, name)
		if err != nil {
			return nil
		}
		friends, _ := fs.FriendNames(ctx, guid)
		waiting, _ := fs.WaitingFriends(ctx, guid)
		mails, _ := fs.MailList(ctx, guid)
		return func() {
			o := w.objectOf(c)
			if o == nil || o.State != StatePlaying {
				return
			}
			o.P.friendCount = len(friends)
			w.send(o, packetFriendList(memoTotal, friends, w.hub.ServerOf))
			for _, n := range waiting {
				w.send(o, packetFriendRequest(n))
			}
			for _, m := range mails {
				w.send(o, packetMailHeader(m))
			}
		}
	})
}

// handleFriendAdd is C1 (FUN_00460b20 -> DS B0 01 FUN_00411410).
func (w *World) handleFriendAdd(o *Object, p []byte) {
	if len(p) < 13 || w.friendStore() == nil {
		return
	}
	target := cString(p[3:13])
	switch {
	case o.P.friendCount >= maxFriends:
		w.send(o, packetFriendAddResult(3, target, 0xFF))
		return
	case o.P.Char.Level < 6:
		w.send(o, packetFriendAddResult(6, target, 0xFF))
		return
	case target == o.P.Char.Name:
		w.send(o, packetFriendAddResult(5, target, 0xFF))
		return
	}
	me := o.P.Char.Name
	fs := w.friendStore()
	c := o.conn
	w.async(func(ctx context.Context) func() {
		r, err := fs.WaitFriendAdd(ctx, me, target)
		if err != nil {
			w.log.Error("friend add", "err", err)
			return nil
		}
		if r == 8 {
			// The target already asked for us: make it mutual at once.
			r2, err := fs.FriendAdd(ctx, me, target)
			if err != nil {
				return nil
			}
			state := w.hub.ServerOf(target)
			if r2 == 1 {
				myState := w.hub.ServerOf(me)
				w.hub.PostToChar(target, func(tw *World, to *Object) {
					tw.send(to, packetFriendState(me, myState))
				})
			}
			return func() { w.friendReply(c, packetFriendAddResult(byte(r2), target, state), r2 == 1) }
		}
		if r == 1 {
			w.hub.PostToChar(target, func(tw *World, to *Object) {
				tw.send(to, packetFriendRequest(me))
			})
		}
		return func() { w.friendReply(c, packetFriendAddResult(byte(r), target, 0xFF), false) }
	})
}

func (w *World) friendReply(c *conn, p []byte, added bool) {
	o := w.objectOf(c)
	if o == nil || o.State != StatePlaying {
		return
	}
	if added {
		o.P.friendCount++
	}
	w.send(o, p)
}

// handleFriendAnswer is C2 (FUN_00460ca0 -> DS B0 02 FUN_004118a0):
// [3] accept, [4..13] requester.
func (w *World) handleFriendAnswer(o *Object, p []byte) {
	if len(p) < 14 || w.friendStore() == nil {
		return
	}
	accept := p[3] != 0
	requester := cString(p[4:14])
	me := o.P.Char.Name
	fs := w.friendStore()
	c := o.conn
	w.async(func(ctx context.Context) func() {
		if !accept {
			r, err := fs.WaitFriendDel(ctx, me, requester)
			if err != nil {
				return nil
			}
			return func() { w.friendReply(c, packetFriendAddResult(byte(r), requester, w.hub.ServerOf(requester)), false) }
		}
		r, err := fs.FriendAdd(ctx, me, requester)
		if err != nil {
			return nil
		}
		if r == 1 {
			myState := w.hub.ServerOf(me)
			w.hub.PostToChar(requester, func(tw *World, to *Object) {
				tw.send(to, packetFriendState(me, myState))
			})
		}
		state := w.hub.ServerOf(requester)
		return func() { w.friendReply(c, packetFriendAddResult(byte(r), requester, state), r == 1) }
	})
}

// handleFriendDelete is C3 (FUN_00460d70 -> DS B0 03 FUN_00411c20).
func (w *World) handleFriendDelete(o *Object, p []byte) {
	if len(p) < 13 || w.friendStore() == nil {
		return
	}
	target := cString(p[3:13])
	me := o.P.Char.Name
	fs := w.friendStore()
	c := o.conn
	w.async(func(ctx context.Context) func() {
		r, err := fs.FriendDel(ctx, me, target)
		if err != nil {
			return nil
		}
		if r == 1 {
			w.hub.PostToChar(target, func(tw *World, to *Object) {
				tw.send(to, packetFriendState(me, 0xFF))
			})
		}
		return func() {
			o := w.objectOf(c)
			if o == nil || o.State != StatePlaying {
				return
			}
			if r == 1 && o.P.friendCount > 0 {
				o.P.friendCount--
			}
			w.send(o, packetFriendDelResult(byte(r), target))
		}
	})
}

// handleFriendState is C4 (FUN_00460e40 -> DS B0 08 FUN_00412bc0): [3]
// state; 0 appears offline to friends.
func (w *World) handleFriendState(o *Object, p []byte) {
	if len(p) < 4 || w.friendStore() == nil {
		return
	}
	state := int(p[3])
	me := o.P.Char.Name
	fs := w.friendStore()
	w.async(func(ctx context.Context) func() {
		if err := fs.SetFriendDel(ctx, me, state); err != nil {
			return nil
		}
		w.friendNotify(ctx, me, state == 0)
		return nil
	})
}

// handleMailSend is C5 (FUN_00460ee0 -> DS B0 04 FUN_00411e80):
// [4..7] id, [8..17] to, [18..49] subject, [50] dir, [51] act,
// [52..53] memo length, [54..] memo.
func (w *World) handleMailSend(o *Object, p []byte) {
	if len(p) < 0x36 || w.friendStore() == nil {
		return
	}
	if o.P.Char.Money < mailCost {
		w.send(o, packetMailResult(7))
		return
	}
	to := cString(p[8:18])
	n := int(binary.LittleEndian.Uint16(p[0x34:]))
	if n > maxMailMemo || to == "" || 0x36+n > len(p) {
		w.send(o, packetMailResult(0))
		return
	}
	m := store.Mail{From: o.P.Char.Name, To: to, Subject: cString(p[18:50]),
		Dir: p[0x32], Act: p[0x33], Memo: append([]byte(nil), p[0x36:0x36+n]...),
		Photo: append([]byte(nil), o.CharSet[:]...)}
	fs := w.friendStore()
	c := o.conn
	w.async(func(ctx context.Context) func() {
		r, _, err := fs.WriteMail(ctx, m)
		if err != nil {
			w.log.Error("write mail", "err", err)
			return nil
		}
		result := 1
		if r < 10 {
			result = r
		} else {
			m.Index = r
			m.Date = w.now()
			w.hub.PostToChar(to, func(tw *World, to *Object) {
				tw.send(to, packetMailHeader(m))
			})
		}
		return func() {
			o := w.objectOf(c)
			if o == nil || o.State != StatePlaying {
				return
			}
			w.send(o, packetMailResult(byte(result)))
			if result == 1 && o.P.Char.Money >= mailCost {
				o.P.Char.Money -= mailCost
				w.sendMoney(o)
			}
		}
	})
}

// handleMailRead is C7 (FUN_00461120 -> DS B0 06 FUN_00412650).
func (w *World) handleMailRead(o *Object, p []byte) {
	w.mailIndexRequest(o, p, func(ctx context.Context, fs store.FriendStore, name string, idx int) []byte {
		m, err := fs.ReadMail(ctx, name, idx)
		if err != nil {
			return nil
		}
		return packetMailRead(m)
	})
}

// handleMailDelete is C8 (FUN_004611c0 -> DS B0 07 FUN_00412a30).
func (w *World) handleMailDelete(o *Object, p []byte) {
	w.mailIndexRequest(o, p, func(ctx context.Context, fs store.FriendStore, name string, idx int) []byte {
		r, err := fs.DelMail(ctx, name, idx)
		if err != nil {
			return nil
		}
		return packetMailDelResult(byte(r), idx)
	})
}

func (w *World) mailIndexRequest(o *Object, p []byte, f func(context.Context, store.FriendStore, string, int) []byte) {
	fs := w.friendStore()
	if len(p) < 6 || fs == nil {
		return
	}
	idx := int(binary.LittleEndian.Uint16(p[4:]))
	name := o.P.Char.Name
	c := o.conn
	w.async(func(ctx context.Context) func() {
		pkt := f(ctx, fs, name, idx)
		if pkt == nil {
			return nil
		}
		return func() {
			if o := w.objectOf(c); o != nil && o.State == StatePlaying {
				w.send(o, pkt)
			}
		}
	})
}

// handleMailList is C9 (FUN_00461400 -> DS B0 05 FUN_00412550): resend the
// mail headers.
func (w *World) handleMailList(o *Object) {
	fs := w.friendStore()
	if fs == nil {
		return
	}
	name := o.P.Char.Name
	c := o.conn
	w.async(func(ctx context.Context) func() {
		guid, _, ok, err := fs.FriendMain(ctx, name)
		if err != nil || !ok {
			return nil
		}
		mails, err := fs.MailList(ctx, guid)
		if err != nil {
			return nil
		}
		return func() {
			if o := w.objectOf(c); o != nil && o.State == StatePlaying {
				for _, m := range mails {
					w.send(o, packetMailHeader(m))
				}
			}
		}
	})
}

func registerFriend(h *hooks) {
	reg := func(head byte, handler func(*World, *Object, []byte)) {
		h.worldHandlers[head] = func(w *World, o *Object, p []byte) {
			if friendReady(o) {
				handler(w, o, p)
			}
		}
	}
	reg(0xC0, func(w *World, o *Object, _ []byte) { w.friendListRequest(o) })
	reg(0xC1, (*World).handleFriendAdd)
	reg(0xC2, (*World).handleFriendAnswer)
	reg(0xC3, (*World).handleFriendDelete)
	reg(0xC4, (*World).handleFriendState)
	reg(0xC5, (*World).handleMailSend)
	reg(0xC7, (*World).handleMailRead)
	reg(0xC8, (*World).handleMailDelete)
	reg(0xC9, func(w *World, o *Object, _ []byte) { w.handleMailList(o) })
	// CA / CB: chat rooms are disabled in this build (FUN_00461260 answers
	// C1 24 CA with byte 35 = 2, FUN_00461340 answers C1 06 CB zeros).
	reg(0xCA, func(w *World, o *Object, _ []byte) {
		p := newPacket(frame.C1, 0xCA, -1, 0x24)
		p[0x23] = 2
		w.send(o, p)
	})
	reg(0xCB, func(w *World, o *Object, _ []byte) {
		w.send(o, newPacket(frame.C1, 0xCB, -1, 6))
	})

	// DataServer 0x70 / 0x71: online map and friend notifications.
	h.enterWorld = append(h.enterWorld, func(w *World, o *Object) {
		name := charName(o)
		w.hub.Enter(OnlineChar{Name: name, Account: o.P.Account.Login, ServerCode: w.cfg.ServerCode, Index: o.Index})
		if w.friendStore() == nil {
			return
		}
		w.friendListRequest(o)
		w.async(func(ctx context.Context) func() {
			w.friendNotify(ctx, name, false)
			return nil
		})
	})
	h.leaveWorld = append(h.leaveWorld, func(w *World, o *Object) {
		name := charName(o)
		if !w.hub.Leave(name, w.cfg.ServerCode, o.Index) || w.friendStore() == nil {
			return
		}
		w.async(func(ctx context.Context) func() {
			w.friendNotify(ctx, name, true)
			return nil
		})
	})
}
