package game

import (
	"context"
	"time"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/store"
)

// Unions (alliances, relation 1) and hostility (rivals, relation 2):
// client E5 / E6 / E9 / EB and DataServer A0 09..0F.

// E6 / E1 result codes seen in the original.
const (
	relResultCastle      = 0x10 // Castle Siege running (msg 742)
	relResultNotMaster   = 0x11 // both sides must be guild masters
	relResultRivalUnion  = 0x15 // hostility asked to an allied guild
	relResultRivalOfMine = 0x16 // union asked by a rival guild
	relResultInUnion     = 0x17 // already in a union (or castle owner)
	relResultHasRival    = 0x18 // a rival already exists
	relResultNoRelation  = 0x19 // nothing to break off
	relResultNotUnionMas = 0x1B // the target is a union member, not its master
	relResultNotRivals   = 0x1C // the rivals are not each other's
	relResultFewMembers  = 0x1D // fewer members than GuildAllianceMinUser
	relResultUnionFull   = 0x1E // GuildAllianceMaxCount reached
	relResultRefused     = 0x20 // the other master said no
	relResultSelf        = 0x21 // the union master kicking itself
)

// handleRelationRequest is E5 (FUN_00484da0): [3] relation (1 union, 2
// rival), [4] request (1 join, 2 break off), [5..6] the other master.
func (w *World) handleRelationRequest(o *Object, p []byte) {
	if len(p) < 7 {
		return
	}
	rel, req := p[3], p[4]
	ti := int(p[5])<<8 | int(p[6])
	fail := func(r byte) { w.send(o, packetRelationResult(rel, req, r, ti)) }
	if ti >= MaxObjects || !guildUsable(w.objects[ti]) {
		w.send(o, packetGuildResult(3))
		return
	}
	t := w.objects[ti]
	if !w.acceptsRequests(t) {
		return
	}
	if s := w.csSyncState(); s > csIdle3 && s < csEnd { // FUN_00484da0: 4 < Sync state < 8
		w.systemMessage(o, w.messageFor(o, 0x2E6))
		fail(relResultCastle)
		return
	}
	og, tg := guildOf(o), guildOf(t)
	mgr := w.guildMgr()
	mine, ok1 := mgr.byNumber(og.Number)
	theirs, ok2 := mgr.byNumber(tg.Number)
	switch {
	case !ok1 || !ok2:
		w.send(o, packetGuildResult(5))
		return
	case og.Status != store.GuildMasterStatus && tg.Status != store.GuildMasterStatus:
		w.send(o, packetGuildResult(4))
		return
	case ifBusy(o) || ifBusy(t):
		w.send(o, packetGuildResult(6))
		return
	case og.Status != store.GuildMasterStatus || tg.Status != store.GuildMasterStatus:
		fail(relResultNotMaster)
		return
	}
	switch req {
	case 1:
		switch rel {
		case 1:
			if mine.count() < w.cfg.Int("Common", "GuildAllianceMinUser", 0) {
				fail(relResultFewMembers)
				return
			}
			if theirs.Union != 0 && theirs.Union == theirs.Number {
				limit := min(w.cfg.Int("Common", "GuildAllianceMaxCount", 0), guildUnionMax)
				if len(mgr.unionList(theirs.Number)) >= limit {
					fail(relResultUnionFull)
					return
				}
			}
			if w.castleOwner(mine.Name) || mine.Union != 0 {
				fail(relResultInUnion)
				return
			}
			if mgr.relation(mine.Number, theirs.Number) == 2 {
				fail(relResultRivalOfMine)
				return
			}
			if theirs.Union != 0 && theirs.Number != theirs.Union {
				fail(relResultNotUnionMas)
				return
			}
		case 2:
			if mine.Rival != 0 || theirs.Rival != 0 {
				w.send(o, packetRelationResult(2, 1, relResultHasRival, ti))
				return
			}
			if mgr.relation(mine.Number, theirs.Number) == 1 {
				fail(relResultRivalUnion)
				return
			}
		}
	case 2:
		switch rel {
		case 1:
			if mine.Union == 0 {
				w.send(o, packetRelationResult(1, 2, relResultNoRelation, ti))
				return
			}
			// Leaving a union needs no answer: A0 0B at once.
			w.relationBreak(1, o.Index, -1, mine.Number)
			return
		case 2:
			if mine.Rival == 0 || theirs.Rival == 0 {
				w.send(o, packetRelationResult(2, 2, relResultNoRelation, ti))
				return
			}
			if mine.Rival != theirs.Number || theirs.Rival != mine.Number {
				w.send(o, packetRelationResult(2, 2, relResultNotRivals, ti))
				return
			}
		}
	}
	now := w.now()
	setIf(o, ifGuildUnion)
	setIf(t, ifGuildUnion)
	og.Partner, og.RequestAt = ti, now
	tg.Partner, tg.RequestAt = o.Index, now
	w.send(t, packetRelationRequest(rel, req, o.Index))
}

// handleRelationAnswer is E6 (FUN_00485280): [3] relation, [4] request,
// [5] result (1 yes), [6..7] the requesting master.
func (w *World) handleRelationAnswer(o *Object, p []byte) {
	if len(p) < 8 {
		return
	}
	rel, req := p[3], p[4]
	ri := int(p[6])<<8 | int(p[7])
	if ri >= MaxObjects || !guildUsable(w.objects[ri]) {
		w.send(o, packetGuildResult(3))
		return
	}
	r := w.objects[ri]
	defer func() {
		clearIf(o)
		clearIf(r)
		guildOf(o).Partner, guildOf(r).Partner = -1, -1
		guildOf(o).RequestAt, guildOf(r).RequestAt = time.Time{}, time.Time{}
	}()
	if p[5] != 1 {
		w.send(r, packetRelationResult(rel, req, relResultRefused, o.Index))
		return
	}
	rn, an := guildNumberOf(r), guildNumberOf(o)
	if rn == 0 || an == 0 {
		return
	}
	switch req {
	case 1:
		w.relationJoin(int(rel), ri, o.Index, rn, an)
	case 2:
		w.relationBreak(int(rel), o.Index, ri, an)
	}
}

// relationJoin is DataServer A0 0A (FUN_00414b60) and the reply A0 0E
// (FUN_00482db0): E6 to both masters, then the union / rival messages.
func (w *World) relationJoin(rel, ri, ai, rn, an int) {
	mgr := w.guildMgr()
	w.async(func(ctx context.Context) func() {
		result := mgr.join(ctx, rel, rn, an)
		return func() {
			if result != 1 {
				w.sendIndex(ri, packetRelationResult(byte(rel), 1, 0, ai))
				w.sendIndex(ai, packetRelationResult(byte(rel), 1, 0, ri))
				return
			}
			rg, _ := mgr.byNumber(rn)
			ag, _ := mgr.byNumber(an)
			w.sendIndex(ri, packetRelationResult(byte(rel), 1, result, ai))
			w.sendIndex(ai, packetRelationResult(byte(rel), 1, result, ri))
			w.guildAll(func(tw *World) {
				if rel == 1 {
					tw.guildMessage(rn, NoticeBlue, 0xC2, ag.Name)
					tw.guildMessage(an, NoticeBlue, 0xC4, rg.Name, ag.Name)
				} else {
					tw.guildMessage(rn, NoticeBlue, 0xC3, ag.Name)
					tw.guildMessage(an, NoticeBlue, 0xC3, rg.Name)
				}
				tw.guildRefresh(rn)
				tw.guildRefresh(an)
			})
		}
	})
}

// relationBreak is DataServer A0 0B (FUN_00414e00) and the reply A0 0F
// (FUN_00482fd0): guild n leaves its union (rel 1) or ends its rivalry.
// i1 is the master that gets the E6 (i2 the other one, -1 = none).
func (w *World) relationBreak(rel, i1, i2, n int) {
	mgr := w.guildMgr()
	before, _ := mgr.byNumber(n)
	other := before.Union
	if rel == 2 {
		other = before.Rival
	}
	og, _ := mgr.byNumber(other)
	w.async(func(ctx context.Context) func() {
		result, change := mgr.breakOff(ctx, rel, n)
		return func() {
			w.sendIndex(i1, packetRelationResult(byte(rel), 2, result, max(i2, 0)))
			if rel == 2 && i2 >= 0 {
				w.sendIndex(i2, packetRelationResult(byte(rel), 2, result, i1))
			}
			if result != 1 {
				return
			}
			w.guildAll(func(tw *World) {
				if rel == 1 {
					tw.guildMessage(n, NoticeBlue, 0xC5)
				} else {
					tw.guildMessage(n, NoticeBlue, 0xC8, og.Name)
					tw.guildMessage(other, NoticeBlue, 0xC8, before.Name)
				}
				tw.unionDissolved(change)
				tw.guildRefresh(n)
				tw.guildRefresh(other)
			})
		}
	})
}

// unionDissolved is the A0 0C reply (FUN_00482b80): the union master guild
// loses its union (message 201).
func (w *World) unionDissolved(c guildChange) {
	if c.dissolved == 0 {
		return
	}
	g, _ := w.guildMgr().byNumber(c.dissolved)
	w.guildMessage(c.dissolved, NoticeBlue, 0xC9, g.Name)
	w.guildRefresh(c.dissolved)
}

// guildRefresh resends C2 65 / C2 67 of the online members of a guild
// (A0 0B lists changed, FUN_004826b0 member loop).
func (w *World) guildRefresh(n int) {
	for _, m := range w.guildMembersHere(n) {
		w.showGuild(m)
		w.showUnion(m)
	}
}

// handleUnionList is E9 (FUN_00485410 -> DataServer A0 09 FUN_004147a0 ->
// A0 0D FUN_00482c40): the guilds of the player's union.
func (w *World) handleUnionList(o *Object) {
	n := guildNumberOf(o)
	mgr := w.guildMgr()
	g, ok := mgr.byNumber(n)
	if !ok || g.Union == 0 {
		w.send(o, packetGuildResult(3))
		return
	}
	list := mgr.unionList(g.Union)
	mgr.mu.Lock()
	rivals := len(mgr.rivalNumbers(g.Union))
	mgr.mu.Unlock()
	result := byte(1)
	if len(list) == 0 {
		result = 0
	}
	w.send(o, packetUnionList(result, byte(len(list)), byte(rivals), list))
}

// handleUnionKick is EB 01 (FUN_004854e0): [4..11] the guild the union
// master removes from its union (DataServer A0 0C -> A0 10, FUN_004831f0).
func (w *World) handleUnionKick(o *Object, p []byte) {
	if len(p) < 12 {
		return
	}
	n := guildNumberOf(o)
	mgr := w.guildMgr()
	g, ok := mgr.byNumber(n)
	if !ok {
		w.send(o, packetGuildResult(3))
		return
	}
	switch {
	case g.Union == 0:
		w.send(o, packetRelationResult(1, 2, relResultNoRelation, 0))
		return
	case guildOf(o).Status != store.GuildMasterStatus || g.Number != g.Union:
		w.send(o, packetRelationResult(1, 2, relResultNotMaster, 0))
		return
	}
	target := cString(p[4:12])
	if cut8(target) == cut8(g.Name) {
		w.send(o, packetRelationResult(1, 2, relResultSelf, 0))
		return
	}
	c := o.conn
	w.async(func(ctx context.Context) func() {
		result, change := mgr.unionKick(ctx, g.Name, target)
		return func() {
			if o := w.objectOf(c); o != nil && o.State == StatePlaying {
				w.send(o, packetUnionKickResult(result))
			}
			if result != 1 {
				return
			}
			tg, _ := mgr.find(target)
			w.guildAll(func(tw *World) {
				tw.guildMessage(g.Number, NoticeBlue, 0xC6, target, g.Name)
				tw.guildMessage(tg.Number, NoticeBlue, 0xC5)
				tw.unionDissolved(change)
				tw.guildRefresh(tg.Number)
				tw.guildRefresh(g.Number)
			})
		}
	})
}
