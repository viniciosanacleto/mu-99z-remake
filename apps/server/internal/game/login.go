package game

import (
	"bytes"
	"context"
	"encoding/binary"
	"strings"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/auth"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/crypto"
)

// cString returns the bytes up to the first NUL.
func cString(b []byte) string {
	if i := bytes.IndexByte(b, 0); i >= 0 {
		b = b[:i]
	}
	return string(b)
}

// handleVerify is F3 09 (FUN_00441730): the client sends its 44-character
// hardware id (Bux-encoded, '-' at 8/17/26/35) and a type byte at [0x31].
// Without it the 1-second tick closes the client after 5 seconds.
func (w *World) handleVerify(o *Object, p []byte) {
	if len(p) < 0x32 {
		return
	}
	hw := make([]byte, 45)
	copy(hw, p[4:49])
	crypto.Bux(hw)
	hwid := cString(hw)
	if len(hw) < 36 || (hw[8] != '-' && hw[17] != '-' && hw[26] != '-' && hw[35] != '-') {
		w.closeClient(o, "Invalid Hardware ID")
		return
	}
	if len(hwid) != 0x2C || w.blacklistedHWID(hwid) {
		w.closeClient(o, "Invalid Hardware ID")
		return
	}
	o.P.Verified = true
	t := p[0x31]
	if t > 2 {
		t = 0
	}
	o.P.ClientType = t
	o.P.HWID = hwid
	w.log.Debug("Verified client", "index", o.Index, "ip", o.P.IP, "hwid", hwid)
}

// handleLiveClient is 0E (FUN_004d7e30): tick and attack speeds reported by
// the client for the speed/latency hack checks (CheckSpeedHack*,
// CheckLatencyHack*, all disabled in this configuration).
func (w *World) handleLiveClient(o *Object, p []byte) {
	if len(p) >= 8 {
		o.P.ClientTick = binary.LittleEndian.Uint32(p[4:])
	}
}

// handleLogin is F1 01 (FUN_004d9360).
func (w *World) handleLogin(o *Object, p []byte) {
	if o.State != StateConnected {
		w.closeClient(o, "login in wrong state")
		return
	}
	if len(p) < 0x31 {
		return
	}
	var serial [16]byte
	copy(serial[:], w.cfg.ServerSerial)
	if !bytes.Equal(p[0x1c:0x21], w.cfg.ServerVersion[:]) || !bytes.Equal(p[0x21:0x31], serial[:]) {
		w.send(o, packetLoginResult(6))
		return
	}
	if o.P.LoginPending {
		return
	}
	o.P.LoginAttempts++
	o.P.LoginPending = true
	o.P.ClientTick = binary.LittleEndian.Uint32(p[0x18:])

	acc := make([]byte, 10)
	copy(acc, p[4:14])
	crypto.Bux(acc)
	pw := make([]byte, 10)
	copy(pw, p[14:24])
	crypto.Bux(pw)
	req := auth.LoginRequest{Account: cString(acc), Password: cString(pw), IP: o.P.IP,
		HWID: o.P.HWID, ServerCode: w.cfg.ServerCode, ServerName: w.cfg.ServerName, Kicker: w}
	c := o.conn
	w.async(func(ctx context.Context) func() {
		result, sess, err := w.auth.Login(ctx, req)
		return func() { w.loginReply(c, req, result, sess, err) }
	})
}

// loginReply reproduces the GameServer side of the JoinServer answer
// (FUN_004a18d0).
func (w *World) loginReply(c *conn, req auth.LoginRequest, result byte, sess *auth.Session, err error) {
	o := w.objectOf(c)
	if o == nil {
		if sess != nil {
			w.async(func(ctx context.Context) func() { w.auth.Logout(ctx, req.Account); return nil })
		}
		return
	}
	pending := o.P.LoginPending
	o.P.LoginPending = false
	logout := func() {
		if sess != nil {
			w.async(func(ctx context.Context) func() { w.auth.Logout(ctx, req.Account); return nil })
		}
	}
	if !pending {
		logout()
		w.closeClient(o, "login reply without request")
		return
	}
	if o.State != StateConnected {
		logout()
		return
	}
	if err != nil {
		w.log.Error("login", "err", err)
		result = auth.ResultInvalidAccount
	}
	if !w.connectMemberAllowed(req.Account) && w.cfg.ConnectMemberCheck {
		logout()
		w.send(o, packetLoginResult(2))
		return
	}
	if result != auth.ResultOK {
		code := result
		if o.P.LoginAttempts >= 3 {
			code = 8
		}
		w.send(o, packetLoginResult(code))
		return
	}
	if sess.Account.BlockCode != 0 {
		logout()
		w.send(o, packetLoginResult(5))
		return
	}
	if sess.Account.Level < w.cfg.ServerLock {
		logout()
		w.send(o, packetLoginResult(10))
		return
	}
	if !w.connectionAllowed(o, sess) {
		logout()
		w.closeClient(o, "connection limit")
		return
	}
	o.State = StateLogged
	o.P.Session = sess
	o.P.Account = sess.Account
	w.send(o, packetLoginResult(1))
	w.log.Info("AddAccountInfo", "index", o.Index, "account", sess.Account.Login, "ip", o.P.IP)
}

// connectMemberAllowed checks Data/ConnectMember.txt (ConnectMemberCheck).
func (w *World) connectMemberAllowed(account string) bool {
	for _, a := range w.connectMembers {
		if strings.EqualFold(a, account) {
			return true
		}
	}
	return false
}

// connectionAllowed is FUN_004413f0: MaxConnectionPerIP /
// MaxConnectionPerHID among logged players of this instance.
func (w *World) connectionAllowed(o *Object, _ *auth.Session) bool {
	ips, hids := 0, 0
	for i := PlayerStart; i < MaxObjects; i++ {
		x := w.objects[i]
		if x == o || x.P == nil || x.State < StateLogged {
			continue
		}
		if x.P.IP == o.P.IP {
			ips++
		}
		if o.P.HWID != "" && x.P.HWID == o.P.HWID {
			hids++
		}
	}
	if w.cfg.MaxConnectionPerIP > 0 && ips >= w.cfg.MaxConnectionPerIP {
		return false
	}
	if w.cfg.MaxConnectionPerHD > 0 && hids >= w.cfg.MaxConnectionPerHD {
		return false
	}
	return true
}

// handleLogout is F1 02 (FUN_004d9560 -> FUN_004c4f80): the reply is sent by
// the 1-second tick when the countdown expires.
func (w *World) handleLogout(o *Object, p []byte) {
	if o.State != StateLogged && o.State != StatePlaying {
		return
	}
	if len(p) < 5 || o.P.LogoutCountdown > 0 {
		return
	}
	t := p[4]
	if t > 2 {
		t = 0
	}
	o.P.LogoutCountdown = 1
	o.P.LogoutType = t
}
