package game

import (
	"context"
	"encoding/binary"
	"sync"
	"time"
)

// Taxes and money of the castle: CCastleSiegeSync (DAT_010d8d48: the
// state, the tax rates and the owner relayed by the DataServer 0x80 0x17
// to every game server, and the tax collected locally), the Senior NPC,
// B2 08/09/10/1F, the Land of Trial (B9 03/05) and the owner's mark B9 02.

const (
	csTaxMax       = 3      // B2 09 kinds 1/2 (FUN_00421310)
	csHuntTaxMax   = 300000 // B2 09 kind 3
	csHuntZoneGate = 0x5F   // gate 95, Land of Trial
	csZenMax       = 2000000000
)

// castleSync is CCastleSiegeSync, shared by the instances of a hub.
type castleSync struct {
	mu                          sync.Mutex
	state                       int    // +0x04
	taxChaos, taxStore, taxHunt int    // +0x08 / +0x0c / +0x10
	owner                       string // +0x1c
	acc                         map[*World]int64
	flushed                     map[*World]time.Time
}

func newCastleSync() *castleSync {
	return &castleSync{acc: map[*World]int64{}, flushed: map[*World]time.Time{}}
}

// csSync returns the hub's castle sync (a World without hub gets its own).
func (w *World) csSync() *castleSync {
	if w.hub != nil {
		return w.hub.castle
	}
	if w.csLocal == nil {
		w.csLocal = newCastleSync()
	}
	return w.csLocal
}

// csPublishSync is FUN_00425c90 -> DataServer 0x80 0x17 -> FUN_00424640.
func (w *World) csPublishSync() {
	c := w.cs()
	s := w.csSync()
	s.mu.Lock()
	s.state = c.state
	s.taxChaos, s.taxStore, s.taxHunt = c.taxChaos, c.taxStore, c.taxHunt
	s.owner = c.owner
	s.mu.Unlock()
}

func (w *World) csSyncState() int {
	s := w.csSync()
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state
}

func (w *World) csSyncValues() (chaos, store, hunt int, owner string) {
	s := w.csSync()
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.taxChaos, s.taxStore, s.taxHunt, s.owner
}

// csTaxFree is FUN_00429b00 / FUN_00429bc0: the members of the owner
// guild and of its alliance pay no tax.
func (w *World) csTaxFree(o *Object) bool {
	if !o.IsPlayer() || o.P.LogoutCountdown > 0 || o.P.Moving {
		return false
	}
	_, _, _, owner := w.csSyncValues()
	if owner == "" {
		return false
	}
	if g := csGuildName(o); g != "" && g == owner {
		return true
	}
	return csGuildName(o) != "" && csUnionMaster(w, o) == owner
}

// castleChaosTax is FUN_00429aa0 (Sync+8, the chaos machine rate).
func (w *World) castleChaosTax(o *Object) int {
	if w.csTaxFree(o) {
		return 0
	}
	chaos, _, _, _ := w.csSyncValues()
	return chaos
}

// castleStoreRate is FUN_00429ad0 (Sync+0xc).
func (w *World) castleStoreRate(o *Object) int {
	if w.csTaxFree(o) {
		return 0
	}
	_, store, _, _ := w.csSyncValues()
	return store
}

// castleStoreTax is the tax of a shop purchase (FUN_0049a910:
// rate * price / 100).
func (w *World) castleStoreTax(o *Object, price int64) int64 {
	return int64(uint32(int64(w.castleStoreRate(o)) * price / 100))
}

// castleAddTax adds collected tax to Sync+0x14 (only 1..2e9).
func (w *World) castleAddTax(tax int64) {
	if tax < 1 || tax > csZenMax {
		return
	}
	s := w.csSync()
	s.mu.Lock()
	s.acc[w] += tax
	s.mu.Unlock()
}

// csFlushTax is FUN_00429a00 (every minute): the collected tax goes to
// the castle money (WZ_CS_ModifyMoney through 0x80 0x18; the answer
// FUN_00424700 updates the castle money on the CastleSiege server).
func (w *World) csFlushTax() {
	s := w.csSync()
	now := w.now()
	s.mu.Lock()
	amount := s.acc[w]
	last := s.flushed[w]
	if amount <= 0 || (!last.IsZero() && now.Sub(last) < 60*time.Second) {
		s.mu.Unlock()
		return
	}
	s.flushed[w] = now
	s.acc[w] = 0
	s.mu.Unlock()
	st := w.csStore()
	if st == nil || amount > csZenMax {
		return
	}
	group := w.cfg.ServerCode / 20
	w.async(func(ctx context.Context) func() {
		r, money, err := st.ModifyCastleMoney(ctx, group, amount)
		if err != nil || r != 1 {
			return nil
		}
		return func() {
			if c := w.cs(); c != nil {
				c.money = money
			}
		}
	})
}

// --- NPCs -------------------------------------------------------------------

// csSeniorTalk is FUN_004c1cf0 (NPC 0xDF): owner guild master/assistant
// only, not during the battle; opens window 0x0C on the chaos box.
func (w *World) csSeniorTalk(o *Object) {
	if c := chaos(o); c.Trans == 1 {
		return
	}
	cs := w.cs()
	if w.csSyncState() == csStart {
		w.systemMessage(o, w.messageFor(o, 241))
		return
	}
	if cs == nil || csGuildName(o) != cs.owner || cs.owner == "" {
		w.systemMessage(o, w.messageFor(o, 242))
		return
	}
	if !csMasterOrAssistant(o) {
		w.systemMessage(o, w.messageFor(o, 243))
		return
	}
	items(o).If = ifState{Use: true, Type: ifChaosBox}
	w.send(o, packetNpcWindow(0x0C))
	w.send(o, packetTaxInfo(1, byte(w.castleChaosTax(o))))
	csOf(o).Involved = false // obj+0x980 = 0
	chaos(o).Mixed = false
	w.chaosBegin(o)
}

// csGuardTalk is FUN_004c1ec0 (Guardsman 0xE0): window 0x0D for guild
// members out of the battle.
func (w *World) csGuardTalk(o, npc *Object) {
	switch {
	case w.csSyncState() == csStart:
		w.send(o, packetObjectChat(npc.Index, w.messageFor(o, 244)))
	case csGuildName(o) == "":
		w.send(o, packetObjectChat(npc.Index, w.messageFor(o, 248)))
	default:
		items(o).If = ifState{Use: true, Type: csIfLever}
		w.send(o, packetNpcWindow(0x0D))
	}
}

// csKeeperTalk is FUN_00426d70 (Gatekeeper 0xDC, Land of Trial): C1 14
// B9 03 with the player's kind (1 other, 2 owner alliance/guild member,
// 3 owner master), the hunt zone state, its tax and the fixed 300000 /
// 10000.
func (w *World) csKeeperTalk(o *Object) {
	c := w.cs()
	kind, open := byte(1), byte(0)
	if c != nil && c.huntOpen {
		open = 1
	}
	if csGuildName(o) != "" {
		switch {
		case w.csOwnerGuild(o) && csGuildStatus(o) == 0x80:
			kind = 3
		case w.csOwnerGuild(o) || w.csOwnerAlliance(o):
			kind, open = 2, 0
		}
	}
	_, _, hunt, _ := w.csSyncValues()
	w.send(o, packetCSHuntInfo(kind, open, hunt))
}

// handleCSHuntEnter is B9 05 (FUN_004220b0): into the Land of Trial for
// the hunt zone tax (free for the owner guild and alliance); closed hunt
// zones only admit them.
func (w *World) handleCSHuntEnter(o *Object, _ []byte) {
	c := w.cs()
	if !w.csEnabled() || !o.IsPlayer() {
		return
	}
	if !c.huntOpen && !w.csOwnerGuild(o) && !w.csOwnerAlliance(o) {
		w.send(o, packetCSHuntEnter(0))
		return
	}
	_, _, tax, _ := w.csSyncValues()
	if w.csTaxFree(o) {
		tax = 0
	}
	if int64(tax) > o.P.Char.Money {
		w.send(o, packetCSHuntEnter(0))
		return
	}
	o.P.Char.Money -= int64(tax)
	w.sendMoney(o)
	w.castleAddTax(int64(tax))
	w.gateMove(o, csHuntZoneGate)
	w.send(o, packetCSHuntEnter(1))
}

// handleCSOwnerMark is B9 02 (FUN_00421fb0): the owner guild's mark.
func (w *World) handleCSOwnerMark(o *Object, _ []byte) {
	if !w.csEnabled() || !o.IsPlayer() {
		return
	}
	if g, ok := w.guildMgr().find(w.cs().owner); ok {
		w.send(o, packetCSOwnerMark(g.Mark))
	}
}

// --- owner management -------------------------------------------------------------

// csOwnerMaster is the "owner guild and obj+0x30c == 0x80" test.
func (w *World) csOwnerMaster(o *Object) bool {
	return w.csEnabled() && o.IsPlayer() && w.csOwnerGuild(o) && csGuildStatus(o) == 0x80
}

// handleCSTaxInfo is B2 08 (FUN_00421240 / FUN_00423c10):
// WZ_CS_GetCastleTaxInfo.
func (w *World) handleCSTaxInfo(o *Object, _ []byte) {
	if !w.csOwnerMaster(o) {
		return
	}
	st := w.csStore()
	if st == nil {
		return
	}
	group, idx := w.cs().group, o.Index
	w.async(func(ctx context.Context) func() {
		d, ok, err := st.CastleTaxInfo(ctx, group)
		if err != nil || !ok {
			return nil
		}
		return func() {
			c := w.cs()
			c.money = d.Money
			w.csReply(idx, packetCSTaxInfo(1, byte(d.TaxChaos), byte(d.TaxStore), d.Money))
		}
	})
}

// handleCSTaxRate is B2 09 (FUN_00421310 / FUN_00423d90): [4] kind (1 chaos,
// 2 store: 0..3; 3 hunt zone: 0..300000) [5..8] rate big-endian.
func (w *World) handleCSTaxRate(o *Object, p []byte) {
	if len(p) < 9 || !w.csEnabled() || !o.IsPlayer() {
		return
	}
	if !w.csOwnerGuild(o) || csGuildStatus(o) != 0x80 {
		return
	}
	kind := int(p[4])
	limit := -1
	switch kind {
	case 1, 2:
		limit = csTaxMax
	case 3:
		limit = csHuntTaxMax
	}
	rate := int(int32(binary.BigEndian.Uint32(p[5:])))
	if int8(p[5]) < 0 || limit < 0 || rate > limit {
		w.send(o, packetCSTaxRate(0, 0, 0))
		return
	}
	st := w.csStore()
	if st == nil {
		return
	}
	group, idx := w.cs().group, o.Index
	w.async(func(ctx context.Context) func() {
		r, stored, err := st.ModifyCastleTaxRate(ctx, group, kind, rate)
		if err != nil {
			r = 0
		}
		return func() {
			if r == 1 && stored >= 0 && stored <= limit {
				c := w.cs()
				switch kind {
				case 1:
					c.taxChaos = stored
				case 2:
					c.taxStore = stored
				case 3:
					c.taxHunt = stored
				}
			}
			w.csReply(idx, packetCSTaxRate(byte(r), byte(kind), stored))
		}
	})
}

// handleCSMoneyOut is B2 10 (FUN_00421490 / FUN_00423ee0): [4..7] zen
// (little-endian) taken out of the castle by the owner guild master.
func (w *World) handleCSMoneyOut(o *Object, p []byte) {
	if len(p) < 8 || !w.csEnabled() || !o.IsPlayer() {
		return
	}
	if !w.csOwnerGuild(o) || csGuildStatus(o) != 0x80 {
		return
	}
	c := w.cs()
	amount := int64(binary.LittleEndian.Uint32(p[4:]))
	if amount == 0 || int8(p[7]) < 0 || amount > c.money || o.P.Char.Money+amount > csZenMax {
		w.send(o, packetCSMoneyOut(0, 0))
		return
	}
	st := w.csStore()
	if st == nil {
		return
	}
	group, idx := c.group, o.Index
	w.async(func(ctx context.Context) func() {
		r, money, err := st.ModifyCastleMoney(ctx, group, -amount)
		if err != nil {
			r = 0
		}
		return func() {
			if r == 1 {
				if t := w.objects[idx]; t.State == StatePlaying && t.IsPlayer() {
					t.P.Char.Money = min(t.P.Char.Money+amount, csZenMax)
					w.sendMoney(t)
				}
				w.cs().money = money
			}
			w.csReply(idx, packetCSMoneyOut(byte(r), money))
		}
	})
}

// handleCSHuntZone is B2 1F (FUN_004218a0): [4] open flag; owner guild
// master, not during the battle (0 then, 2 when not allowed).
func (w *World) handleCSHuntZone(o *Object, p []byte) {
	if len(p) < 5 || !w.csEnabled() || !o.IsPlayer() {
		return
	}
	c := w.cs()
	if !w.csOwnerGuild(o) || csGuildStatus(o) != 0x80 {
		w.send(o, packetCSHuntZone(2, p[4]))
		return
	}
	if c.state == csStart {
		w.send(o, packetCSHuntZone(0, p[4]))
		return
	}
	c.huntOpen = p[4] != 0
	w.send(o, packetCSHuntZone(1, p[4]))
}

func init() {
	tick1000Hooks = append(tick1000Hooks, (*World).csFlushTax)
}
