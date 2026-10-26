package game

import (
	"context"
	"encoding/binary"
	"errors"
	"sort"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/auth"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/store"
)

// CReconnect of the original GameServer (singleton FUN_004e1000 at
// DAT_01241a08; docs/server-plan/10e-customs.md, "Cache de
// reconexão"). It is a std::map name -> 0x13C-byte record:
//
//   - FUN_004e1350 fills the record of a character that leaves the game
//     (gObjCharacterLeave FUN_004c4b40, so every logout, character select,
//     disconnect and gObjDel) and of a character entering an offline mode
//     (/offline FUN_004888e0, /offattack FUN_00445d90, /offstore
//     FUN_00469030), then sends it to the DataServer (C2 01A8 C0 00 name
//     ServerCode ReconnectTime data[400]), which keeps it in ReconnectData;
//   - FUN_004e1260 (every second, FUN_0050a140) counts the record time
//     (ReconnectTime) down; below 0 the record is erased and the
//     DataServer row deleted (C1 12 C0 01 name ServerCode);
//   - FUN_004e1970 (character load, FUN_00473a10) gives the record of the
//     name back: party, auto points, auto reset, party flags and password,
//     the auto attack, the auto pick and, for an object without client,
//     the offline shop; then the record is erased (C0 01 too);
//   - when a GameServer connects, the DataServer replays the rows of its
//     ServerCode (FUN_0041a7f0); FUN_004e22e0 stores each record and
//     FUN_004e2020 re-creates the offline characters ("Offline Reconnect",
//     ReconnectOfflineSwitch).
//
// The record never moves between servers: a character that logs into
// another server does not get it. Only ReconnectTime and
// ReconnectOfflineSwitch are read; the other Reconnect* keys of
// GameServerInfo - Common.dat are never loaded by this build.

// reconnectRecordSize is the record copied into the 400-byte Data.
const reconnectRecordSize = 0x13C

// reconnectRecord is the 0x13C-byte record (offsets of FUN_004e1350).
type reconnectRecord struct {
	Account string // +0x00 obj+0x6f
	// +0x0b obj+0x85 (password): not kept by the rewrite, see LoginOffline.
	Name string // +0x16 obj+0x7a
	IP   string // +0x21 obj+0x19 (16)
	HWID string // +0x31 obj+0x29 (45)

	Time  int // +0x60 ReconnectTime countdown (CServerInfo+0x1bc)
	Party int // +0x64 obj+0x2f0

	AutoCount     int    // +0x68 obj+0x63c
	AutoAmount    [5]int // +0x6c obj+0x640..0x650
	AutoReset     bool   // +0x80 obj+0x654
	AutoPoints    [5]int // +0x84 obj+0x658..0x668
	PartyFlags    int    // +0x98 obj+0x434
	PartyPassword string // +0x9c obj+0x630 (11)

	AttackActive  bool // +0xa7 obj+0x20d8
	AttackOffline bool // +0xa8 obj+0x20d9
	AttackBuff    int  // +0xac obj+0x20dc
	AttackSkill   int  // +0xb0 obj+0x20e0
	AttackMap     int  // +0xb2 obj+0x20e2
	AttackX       int  // +0xb3 obj+0x20e3
	AttackY       int  // +0xb4 obj+0x20e4
	PickOn        bool // +0xb5 obj+0x22bc
	// +0xb6 obj+0x20f8 is never set (U-CUST-17): always 0.
	Helper       bool      // +0xb7 obj+0x2110
	PShopOpen    bool      // +0xb8 obj+0x2468
	StoreOffline byte      // +0xb9 obj+0x249e
	StoreType    int       // +0xba obj+0x249d
	Prices       [32]int32 // +0xbc the CItem+0x84 price of the shop slots 76..107
}

func putCStr(b []byte, s string) {
	n := copy(b[:len(b)-1], s)
	clear(b[n:])
}

func boolByte(v bool) byte {
	if v {
		return 1
	}
	return 0
}

// encode is the record as FUN_004e1350 lays it out (the 400-byte Data of
// the DataServer packet; the bytes after 0x13C are 0 here).
func (r *reconnectRecord) encode() []byte {
	b := make([]byte, store.ReconnectDataSize)
	le := binary.LittleEndian
	putCStr(b[0x00:0x0b], r.Account)
	putCStr(b[0x16:0x21], r.Name)
	putCStr(b[0x21:0x31], r.IP)
	putCStr(b[0x31:0x5e], r.HWID)
	le.PutUint32(b[0x60:], uint32(int32(r.Time)))
	le.PutUint32(b[0x64:], uint32(int32(r.Party)))
	le.PutUint32(b[0x68:], uint32(int32(r.AutoCount)))
	for i := 0; i < 5; i++ {
		le.PutUint32(b[0x6c+4*i:], uint32(int32(r.AutoAmount[i])))
		le.PutUint32(b[0x84+4*i:], uint32(int32(r.AutoPoints[i])))
	}
	le.PutUint32(b[0x80:], uint32(boolByte(r.AutoReset)))
	le.PutUint32(b[0x98:], uint32(int32(r.PartyFlags)))
	putCStr(b[0x9c:0xa7], r.PartyPassword)
	b[0xa7] = boolByte(r.AttackActive)
	b[0xa8] = boolByte(r.AttackOffline)
	le.PutUint32(b[0xac:], uint32(int32(r.AttackBuff)))
	le.PutUint16(b[0xb0:], uint16(r.AttackSkill))
	b[0xb2], b[0xb3], b[0xb4] = byte(r.AttackMap), byte(r.AttackX), byte(r.AttackY)
	b[0xb5] = boolByte(r.PickOn)
	b[0xb7] = boolByte(r.Helper)
	b[0xb8] = boolByte(r.PShopOpen)
	b[0xb9] = r.StoreOffline
	b[0xba] = byte(r.StoreType)
	for i, p := range r.Prices {
		le.PutUint32(b[0xbc+4*i:], uint32(p))
	}
	return b
}

func cStrAt(b []byte) string {
	for i, c := range b {
		if c == 0 {
			return string(b[:i])
		}
	}
	return string(b)
}

// decodeReconnect reads a record from the Data column (FUN_004e22e0
// copies 0x13C bytes from packet+0x18).
func decodeReconnect(b []byte) (*reconnectRecord, bool) {
	if len(b) < reconnectRecordSize {
		return nil, false
	}
	le := binary.LittleEndian
	i32 := func(off int) int { return int(int32(le.Uint32(b[off:]))) }
	r := &reconnectRecord{
		Account: cStrAt(b[0x00:0x0b]), Name: cStrAt(b[0x16:0x21]), IP: cStrAt(b[0x21:0x31]), HWID: cStrAt(b[0x31:0x5e]),
		Time: i32(0x60), Party: i32(0x64), AutoCount: i32(0x68), AutoReset: b[0x80] != 0, PartyFlags: i32(0x98),
		PartyPassword: cStrAt(b[0x9c:0xa7]), AttackActive: b[0xa7] != 0, AttackOffline: b[0xa8] != 0,
		AttackBuff: i32(0xac), AttackSkill: int(le.Uint16(b[0xb0:])), AttackMap: int(b[0xb2]), AttackX: int(b[0xb3]),
		AttackY: int(b[0xb4]), PickOn: b[0xb5] != 0, Helper: b[0xb7] != 0, PShopOpen: b[0xb8] != 0,
		StoreOffline: b[0xb9], StoreType: int(b[0xba]),
	}
	for i := 0; i < 5; i++ {
		r.AutoAmount[i] = i32(0x6c + 4*i)
		r.AutoPoints[i] = i32(0x84 + 4*i)
	}
	for i := range r.Prices {
		r.Prices[i] = int32(le.Uint32(b[0xbc+4*i:]))
	}
	return r, true
}

// reconnectWorld is the CReconnect map of the instance.
type reconnectWorld struct {
	records  map[string]*reconnectRecord // std::map<std::string>: exact name
	last     chan struct{}               // the previous ReconnectData write (kept in order)
	replayed bool                        // the DataServer replay was requested
}

func (w *World) rc() *reconnectWorld {
	if w.rcn == nil {
		w.rcn = &reconnectWorld{records: map[string]*reconnectRecord{}}
	}
	return w.rcn
}

func init() {
	// FUN_004e1350 runs from FUN_004c4b40 before the party, guild and the
	// other leave steps, so the hook goes first.
	leaveWorldHooks = append([]func(*World, *Object){(*World).reconnectSave}, leaveWorldHooks...)
	enterWorldHooks = append(enterWorldHooks, (*World).reconnectRestore)
	tick1000Hooks = append(tick1000Hooks, (*World).reconnectSecond)
}

// reconnectWrite runs one DataServer ReconnectData request off the world
// goroutine, after the previous one finished.
func (w *World) reconnectWrite(f func(ctx context.Context, rs store.ReconnectStore) error) {
	rs, ok := w.store.(store.ReconnectStore)
	if !ok {
		return
	}
	rc := w.rc()
	prev, done := rc.last, make(chan struct{})
	rc.last = done
	w.async(func(ctx context.Context) func() {
		defer close(done)
		if prev != nil {
			<-prev
		}
		if err := f(ctx, rs); err != nil {
			w.log.Error("reconnect data", "err", err)
		}
		return nil
	})
}

// reconnectForget is C1 12 C0 01 name ServerCode (DataServer FUN_0041a760).
func (w *World) reconnectForget(name string) {
	code := w.cfg.ServerCode
	w.reconnectWrite(func(ctx context.Context, rs store.ReconnectStore) error {
		return rs.DeleteReconnect(ctx, name, code)
	})
}

// reconnectCapture builds the record of o.
func (w *World) reconnectCapture(o *Object) *reconnectRecord {
	p := o.P
	r := &reconnectRecord{Account: p.Account.Login, Name: p.Char.Name, IP: p.IP, HWID: p.HWID,
		Time: w.cfg.Int("Common", "ReconnectTime", 0), Party: w.partyNumber(o)}
	if a := p.autoPoints; a != nil {
		r.AutoCount, r.AutoAmount = a.Count, a.Amount
	}
	if c := p.cmd; c != nil {
		r.AutoReset, r.AutoPoints = c.AutoReset, c.AutoPoints
	}
	ps := w.partyStateOf(o)
	r.PartyFlags, r.PartyPassword = ps.Flags, ps.Password
	if c := p.cust; c != nil {
		a := &c.Attack
		r.AttackActive, r.AttackOffline, r.AttackBuff, r.AttackSkill = a.Active, a.Offline, a.Buff, a.Skill
		r.AttackMap, r.AttackX, r.AttackY = a.Map, a.X, a.Y
		r.PickOn = c.Pick.On
		r.Helper = c.Offline.Helper
		r.StoreOffline, r.StoreType = c.Store.Offline, c.Store.Type
	}
	r.PShopOpen = pshopOpen(o)
	for i := range r.Prices {
		if it := p.Inventory[ShopStart+i]; !it.IsEmpty() {
			r.Prices[i] = it.PShopPrice
		}
	}
	return r
}

// reconnectSave is FUN_004e1350. With ReconnectOfflineSwitch, an object in
// the offline attack or /offline mode inside a safe zone gets no record
// at all, otherwise it is saved (DS 0x30); an offline shop is saved too.
// The record replaces the one of the same name and goes to the
// DataServer.
func (w *World) reconnectSave(o *Object) {
	if o.Type != TypePlayer || o.P == nil || o.P.Char == nil {
		return
	}
	if w.cfg.Int("Common", "ReconnectOfflineSwitch", 0) != 0 && o.P.cust != nil {
		c := o.P.cust
		if c.Attack.Offline || c.Offline.Helper {
			if w.inSafeZone(o) {
				return
			}
			w.saveCharacter(o, nil)
		}
		if c.Store.Offline != 0 {
			w.saveCharacter(o, nil)
		}
	}
	r := w.reconnectCapture(o)
	w.rc().records[r.Name] = r
	row := store.ReconnectRow{Name: r.Name, ServerCode: w.cfg.ServerCode, Data: r.encode(), Time: r.Time}
	w.reconnectWrite(func(ctx context.Context, rs store.ReconnectStore) error {
		return rs.SaveReconnect(ctx, row)
	})
}

// reconnectSecond is FUN_004e1260 (map order: by name), plus the end of
// a re-created object that got no offline mode back (see
// reconnectOffline). The first pass also asks for the DataServer replay
// (the instance "connects" once its loop runs).
func (w *World) reconnectSecond() {
	rc := w.rc()
	if !rc.replayed {
		rc.replayed = true
		w.reconnectReplay()
	}
	if len(rc.records) > 0 {
		names := make([]string, 0, len(rc.records))
		for n := range rc.records {
			names = append(names, n)
		}
		sort.Strings(names)
		for _, n := range names {
			r := rc.records[n]
			if r.Time--; r.Time < 0 {
				w.reconnectForget(n)
				delete(rc.records, n)
			}
		}
	}
	now := w.now()
	for i := PlayerStart; i < MaxObjects; i++ {
		o := w.objects[i]
		if o.State == StatePlaying && o.Type == TypePlayer && o.P != nil && detached(o) && !isOffline(o) &&
			now.Sub(o.P.LastPacket) > clientIdleTimeout {
			w.removePlayer(o)
		}
	}
}

// reconnectRestore is FUN_004e1970.
func (w *World) reconnectRestore(o *Object) {
	if o.P == nil || o.P.Char == nil {
		return
	}
	rc := w.rc()
	r := rc.records[o.P.Char.Name]
	if r == nil {
		return
	}
	w.reconnectParty(o, r) // FUN_004e1bb0
	// FUN_004e1ca0.
	ap := autoPoints(o)
	ap.Count, ap.Amount = r.AutoCount, r.AutoAmount
	cs := cmdOf(o)
	cs.AutoReset, cs.AutoPoints = r.AutoReset, r.AutoPoints
	ps := w.partyStateOf(o)
	ps.Flags, ps.Password = r.PartyFlags, r.PartyPassword
	// FUN_004e1d60: the auto attack (offline only for an object without
	// client), then the F3 EB status (FUN_00445940).
	if r.AttackActive {
		a := attackOf(o)
		a.Active = true
		a.Offline = detached(o) && r.AttackOffline
		a.Buff, a.Skill, a.Map, a.X, a.Y = r.AttackBuff, r.AttackSkill, r.AttackMap, r.AttackX, r.AttackY
		now := w.now()
		a.NextCast, a.NextPotion, a.Start = now, now, now
	}
	w.sendAttackStatus(o)
	if r.PickOn {
		pk := pickOf(o)
		pk.On, pk.Start = true, w.now()
	}
	if detached(o) {
		w.reconnectStore(o, r) // FUN_004e1e70
	}
	w.reconnectForget(r.Name)
	delete(rc.records, r.Name)
}

// reconnectPartyOK is FUN_004e1ae0: a valid party number shared by at
// least two cached records, or a party that still exists.
func (w *World) reconnectPartyOK(r *reconnectRecord) bool {
	n := r.Party
	if n < 0 || n >= MaxObjects {
		return false
	}
	same := 0
	for _, x := range w.rc().records {
		if x.Party == n {
			same++
		}
	}
	if same > 1 {
		return true
	}
	return w.parties != nil && w.parties.parties[n] != nil
}

// reconnectParty is FUN_004e1bb0: when the party is gone it is created
// again under the same number with the character as leader (C1 42 list),
// otherwise the character is added to it (FUN_004d25b0).
func (w *World) reconnectParty(o *Object, r *reconnectRecord) {
	if w.parties == nil || !w.reconnectPartyOK(r) {
		return
	}
	if pt := w.parties.parties[r.Party]; pt != nil {
		w.partyAdd(pt, o)
		return
	}
	pt := &party{number: r.Party, count: 1, members: [partyMaxMembers]int{o.Index, -1, -1, -1, -1}}
	w.parties.parties[r.Party] = pt
	w.partyStateOf(o).Number = r.Party
	w.sendPartyList(pt)
}

// reconnectStore is FUN_004e1e70: an open personal shop in the offline
// mode gets the prices of its slots back; with at least one price the
// shop reopens as the custom shop of the saved type (FUN_00469140) and
// stays offline (obj+0x249e = 1).
func (w *World) reconnectStore(o *Object, r *reconnectRecord) {
	if !r.PShopOpen || r.StoreOffline != 1 {
		return
	}
	n := 0
	for i, price := range r.Prices {
		it := &o.P.Inventory[ShopStart+i]
		if !it.IsEmpty() && price > 0 {
			it.PShopPrice = price
			n++
		}
	}
	if n == 0 {
		return
	}
	pshop(o).Open = r.PShopOpen
	w.customStoreOpen(o, r.StoreType)
	custom(o).Store.Offline = 1
}

// --- replay and Offline Reconnect ----------------------------------------------

// reconnectReplay is the DataServer replay when the server connects
// (FUN_0041a7f0, C2 01A8 C0 00 per row) handled by FUN_004e22e0: each
// record replaces the cached one of its name and goes to FUN_004e2020.
func (w *World) reconnectReplay() {
	rs, ok := w.store.(store.ReconnectStore)
	if !ok {
		return
	}
	code := w.cfg.ServerCode
	w.async(func(ctx context.Context) func() {
		rows, err := rs.ReconnectRows(ctx, code)
		if err != nil {
			w.log.Error("reconnect data", "err", err)
			return nil
		}
		if len(rows) == 0 {
			return nil
		}
		return func() { w.reconnectLoaded(rows) }
	})
}

func (w *World) reconnectLoaded(rows []store.ReconnectRow) {
	for _, row := range rows {
		r, ok := decodeReconnect(row.Data)
		if !ok {
			continue
		}
		w.rc().records[row.Name] = r
		w.reconnectOffline(r)
	}
}

// reconnectOfflineAllowed is the first part of FUN_004e2020:
// ReconnectOfflineSwitch, then CustomAttackOfflineSwitch,
// CustomStoreOfflineSwitch or CServerInfo+0x540 (no key loads it: 0); the
// record must be in an offline mode with the shop not closing, and have
// its account, name, IP and hardware id (the password test of the
// original is dropped with the password).
func (w *World) reconnectOfflineAllowed(r *reconnectRecord) bool {
	if w.cfg.Int("Common", "ReconnectOfflineSwitch", 0) == 0 {
		return false
	}
	if w.cfg.Int("Custom", "CustomAttackOfflineSwitch", 0) == 0 && w.cfg.Int("Custom", "CustomStoreOfflineSwitch", 0) == 0 {
		return false
	}
	if !(r.AttackOffline || r.StoreOffline != 0 || r.Helper) || r.StoreOffline >= 2 {
		return false
	}
	return r.Account != "" && r.Name != "" && r.IP != "" && r.HWID != ""
}

// reconnectOffline is FUN_004e2020 ("Offline Reconnect"): a new object
// without socket (FUN_0050c260 / FUN_0050c3c0 with socket -1, verified)
// logs the account in (JS 0x01, FUN_004a26e0; LoginOffline here) and loads
// the character (DS 0x04, FUN_00475340). Its entry runs FUN_004e1970,
// which gives the offline attack and the offline shop back because the
// object has no client. The /offline flag (obj+0x2110) is never restored:
// such an object keeps no mode and is removed once its client timeout
// passes (U-CUST-29).
func (w *World) reconnectOffline(r *reconnectRecord) {
	if !w.reconnectOfflineAllowed(r) || w.closing || w.users >= min(w.cfg.MaxUsers, 1000) {
		return
	}
	idx := -1
	for n := 0; n < MaxObjects-PlayerStart; n++ {
		i := w.nextUser
		w.nextUser++
		if w.nextUser >= MaxObjects {
			w.nextUser = PlayerStart
		}
		if w.objects[i].State == StateEmpty {
			idx = i
			break
		}
	}
	if idx < 0 {
		return
	}
	o := w.objects[idx]
	o.reset()
	now := w.now()
	c := detachedConn(idx, r.IP)
	o.State, o.Type, o.conn = StateConnected, TypePlayer, c
	o.P = &Player{IP: r.IP, HWID: r.HWID, ConnectedAt: now, LastPacket: now, Verified: true, recvSerial: 0xFF}
	w.users++
	w.report()
	w.log.Info("[ObjectManager] Offline Reconnect", "index", idx, "account", r.Account, "name", r.Name,
		"ip", r.IP, "hwid", r.HWID)
	req := auth.LoginRequest{Account: r.Account, IP: r.IP, HWID: r.HWID, ServerCode: w.cfg.ServerCode,
		ServerName: w.cfg.ServerName, Kicker: w}
	name := r.Name
	w.async(func(ctx context.Context) func() {
		result, sess, err := w.auth.LoginOffline(ctx, req)
		var ch *store.Character
		if err == nil && result == auth.ResultOK {
			ch, err = w.store.LoadCharacter(ctx, sess.Account.ID, name)
			if err != nil {
				w.auth.Logout(ctx, req.Account)
			}
		}
		return func() { w.reconnectOfflineLoaded(c, sess, ch, result, err) }
	})
}

func (w *World) reconnectOfflineLoaded(c *conn, sess *auth.Session, ch *store.Character, result byte, err error) {
	o := w.objectOf(c)
	if o == nil || o.State != StateConnected {
		if sess != nil && ch != nil {
			w.async(func(ctx context.Context) func() { w.auth.Logout(ctx, sess.Account.Login); return nil })
		}
		return
	}
	if err != nil || result != auth.ResultOK || ch == nil || ch.CtlCode&1 != 0 {
		if err != nil && !errors.Is(err, store.ErrNotFound) {
			w.log.Error("offline reconnect", "err", err)
		}
		if sess != nil && ch != nil {
			w.async(func(ctx context.Context) func() { w.auth.Logout(ctx, sess.Account.Login); return nil })
		}
		o.reset()
		w.users--
		w.report()
		return
	}
	o.State = StateLogged
	o.P.Session = sess
	o.P.Account = sess.Account
	w.enterWorld(o, ch)
}
