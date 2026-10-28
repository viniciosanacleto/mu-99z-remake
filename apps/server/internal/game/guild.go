package game

import (
	"context"
	"encoding/binary"
	"strings"
	"time"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/character"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/guild"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/store"
)

// Guild system of the GameServer (CGuild/CGuildManager of GameServer.exe,
// client heads 50..66 and E1..EB, DataServer replies A0 00..10 handled by
// FUN_00481660..FUN_004831f0). The DataServer half is guild_ds.go and the
// guild war guild_war.go. docs/server-plan/09b-guild.md.

// Interface types of obj+0x3d4 used by the guild module.
const (
	ifGuildJoin   = 4  // 0x101: join request pending (50/51)
	ifGuildCreate = 5  // 0x140: guild creation window (54/55/57)
	ifGuildUnion  = 14 // 0x381: union/rival request pending (E5/E6)
)

// guildState is the guild part of a player: obj+0x2f8 (guild pointer, here
// the number), +0x2fc name, +0x308 number, +0x30c status, +0x318 the other
// side of a pending request and +0x2504 the 53 throttle.
type guildState struct {
	Number    int
	Name      string
	Status    int // -1 without guild
	Partner   int // obj+0x318
	RequestAt time.Time
	LastLeave time.Time
}

func guildOf(o *Object) *guildState {
	if o.P == nil {
		return &guildState{Status: -1, Partner: -1}
	}
	if o.P.Guild == nil {
		o.P.Guild = &guildState{Status: -1, Partner: -1}
	}
	return o.P.Guild
}

// guildNumberOf is obj+0x308 (0 without guild).
func guildNumberOf(o *Object) int {
	if o.P == nil || o.P.Guild == nil {
		return 0
	}
	return o.P.Guild.Number
}

// guildUsable is the common precondition of the guild handlers: playing
// player (obj+4 == 3, type 1), no logout countdown (obj+0xa <= 0), not
// moving between servers (obj+0x964).
func guildUsable(o *Object) bool {
	return o != nil && o.State == StatePlaying && o.IsPlayer() && o.P.Char != nil && o.P.LogoutCountdown < 1 && !o.P.Moving
}

func ifBusy(o *Object) bool { return items(o).If.Use }

func setIf(o *Object, typ int) { items(o).If = ifState{Use: true, Type: typ} }

func clearIf(o *Object) { items(o).If = ifState{} }

// clearIfType is the "(0x3d4 & 3) && (0x3d4 & 0xffc0) == type<<6" reset.
func clearIfType(o *Object, typ int) {
	if s := items(o).If; s.Use && s.Type == typ {
		items(o).If.Use = false
	}
}

// acceptsRequests is obj+0x434 bit 0 (party.go owns the flags).
func (w *World) acceptsRequests(o *Object) bool { return w.partyStateOf(o).Flags&1 != 0 }

// --- packets ---------------------------------------------------------------------

// --- views ------------------------------------------------------------------------

// guildEntry builds o's C2 65 entry as seen by viewer (nil = itself).
func (w *World) guildEntry(o, viewer *Object) (guild.ViewEntry, bool) {
	gs := o.P.Guild
	if gs == nil || gs.Number == 0 {
		return guild.ViewEntry{}, false
	}
	g, ok := w.guildMgr().ByNumber(gs.Number)
	if !ok {
		return guild.ViewEntry{}, false
	}
	e := guild.ViewEntry{Number: g.Number, Status: gs.Status, Type: g.Type, Index: o.Index}
	if viewer != nil && guildNumberOf(viewer) != 0 {
		e.Relation = w.guildMgr().Relation(g.Number, guildNumberOf(viewer))
	}
	return e, true
}

// unionEntry builds o's C2 67 entry as seen by viewer (nil = itself).
func (w *World) unionEntry(o, viewer *Object) (guild.UnionViewEntry, bool) {
	n := guildNumberOf(o)
	g, ok := w.guildMgr().ByNumber(n)
	if !ok {
		return guild.UnionViewEntry{}, false
	}
	e := guild.UnionViewEntry{Index: o.Index, Number: g.Number, Union: w.guildMgr().UnionName(g.Union)}
	if viewer != nil && guildNumberOf(viewer) != 0 {
		e.Relation = w.guildMgr().Relation(g.Number, guildNumberOf(viewer))
	}
	return e, true
}

// guildHidden is the skip test of the guild viewport builders: Authority
// 2 (obj+0x250) and the invisibility effect 0x12 (FUN_00478ec0).
func (w *World) guildHidden(o *Object) bool {
	return mv(o).Authority == 2 || w.hasEffect(o, 0x12)
}

// showGuild is FUN_00516f60: the player's own C2 65 entry to itself and to
// everyone who sees it.
func (w *World) showGuild(o *Object) {
	if w.guildHidden(o) {
		return
	}
	e, ok := w.guildEntry(o, nil)
	if !ok {
		return
	}
	p := guild.PacketViewport([]guild.ViewEntry{e})
	w.send(o, p)
	w.broadcast(o, p)
}

// showUnion is FUN_00517080: the own C2 67 entry (relation 0) and, to each
// player that sees o, the entry with that player's relation.
func (w *World) showUnion(o *Object) {
	if w.guildHidden(o) {
		return
	}
	e, ok := w.unionEntry(o, nil)
	if !ok {
		return
	}
	w.send(o, guild.PacketUnionViewport([]guild.UnionViewEntry{e}))
	for _, v := range o.VP2 {
		if v.State == VPEmpty || v.Type != byte(TypePlayer) || v.Index < 0 || v.Index >= MaxObjects {
			continue
		}
		t := w.objects[v.Index]
		if !t.IsPlayer() {
			continue
		}
		e2, _ := w.unionEntry(o, t)
		w.send(t, guild.PacketUnionViewport([]guild.UnionViewEntry{e2}))
	}
}

// guildViewportSend is the guild part of FUN_0050dbf0: C2 65 for the new
// players in the viewport (FUN_005160a0) and C2 67 for the visible ones
// (FUN_005162f0).
func (w *World) guildViewportSend(o *Object) {
	var news []guild.ViewEntry
	var unions []guild.UnionViewEntry
	for _, e := range o.VP {
		if e.Type != byte(TypePlayer) || e.Index < 0 || e.Index >= MaxObjects {
			continue
		}
		t := w.objects[e.Index]
		if !t.IsPlayer() || guildNumberOf(t) == 0 || w.guildHidden(t) {
			continue
		}
		switch e.State {
		case VPNew:
			if g, ok := w.guildEntry(t, o); ok {
				news = append(news, g)
			}
		case VPVisible:
			if u, ok := w.unionEntry(t, o); ok {
				unions = append(unions, u)
			}
		}
	}
	if len(news) > 0 {
		w.send(o, guild.PacketViewport(news))
	}
	if len(unions) > 0 {
		w.send(o, guild.PacketUnionViewport(unions))
	}
}

// --- shared helpers ---------------------------------------------------------------

// guildAll runs f on every instance of the process: this one at once (it
// must be called on the world goroutine) and the others through their
// event queues. It is the DataServer broadcast FUN_00415760.
func (w *World) guildAll(f func(*World)) {
	w.hub.mu.Lock()
	ws := make([]*World, 0, len(w.hub.worlds))
	for _, x := range w.hub.worlds {
		if x != w {
			ws = append(ws, x)
		}
	}
	w.hub.mu.Unlock()
	f(w)
	for _, x := range ws {
		x.post(func() { f(x) })
	}
}

// guildMembersHere returns the online members of guild n on this instance
// (FUN_0050cc80 over the member slots).
func (w *World) guildMembersHere(n int) []*Object {
	if n == 0 {
		return nil
	}
	var out []*Object
	for i := PlayerStart; i < MaxObjects; i++ {
		if o := w.objects[i]; o.State == StatePlaying && o.IsPlayer() && guildNumberOf(o) == n {
			out = append(out, o)
		}
	}
	return out
}

// playerHere finds a playing character of this instance by exact name
// (FUN_0050c6d0).
func (w *World) playerHere(name string) *Object {
	if name == "" {
		return nil
	}
	for i := PlayerStart; i < MaxObjects; i++ {
		if o := w.objects[i]; o.State == StatePlaying && o.IsPlayer() && o.P.Char != nil && o.P.Char.Name == name {
			return o
		}
	}
	return nil
}

// guildMessage is FUN_00486d60: a Lang message to every member of guild n
// on this instance (notice type typ: 0 gold, 1 blue).
func (w *World) guildMessage(n int, typ byte, id int, args ...any) {
	for _, o := range w.guildMembersHere(n) {
		w.send(o, packetNotice(typ, 0, 0, 0, 0, 0, w.messageFor(o, id, args...)))
	}
}

// guildSendAll is FUN_00486ef0: a packet to every member of guild n here.
func (w *World) guildSendAll(n int, p []byte) {
	for _, o := range w.guildMembersHere(n) {
		w.send(o, p)
	}
}

// setGuild fills the guild fields of a player (obj+0x2f8..0x30c).
func (w *World) setGuild(o *Object, g *guild.Record, status int) {
	gs := guildOf(o)
	gs.Number, gs.Name, gs.Status = g.Number, g.Name, status
}

// clearGuild empties them (number 0, status -1).
func clearGuild(o *Object) {
	gs := guildOf(o)
	gs.Number, gs.Name, gs.Status = 0, "", -1
}

// sendGuildList is FUN_00483a30 (52): the member list of the player's
// guild, with the server of each member.
func (w *World) sendGuildList(o *Object) {
	n := guildNumberOf(o)
	g, ok := w.guildMgr().ByNumber(n)
	if !ok {
		return
	}
	var ms []guild.ListMember
	for _, m := range g.Members {
		if m.Name != "" {
			ms = append(ms, guild.ListMember{Name: m.Name, Server: m.Server, Status: m.Status})
		}
	}
	rival := ""
	if g.Rival != 0 {
		if r, ok := w.guildMgr().ByNumber(g.Rival); ok {
			rival = r.Name
		}
	}
	if len(ms) > 0 {
		w.send(o, guild.PacketList(g.Score, w.warScore(n), rival, ms))
	}
}

// guild.MaxMembers is FUN_00486200: the members a master can have. Dark
// Lords: (level + master level) / 10 + leadership / 10; others: (level +
// master level) / 10; GuildInsertMaxUser2 / GuildInsertMaxUser1 when the
// master has resets; capped by those values and by 80.
func (w *World) guildCapacity(master *Object) int {
	c := master.P.Char
	var n, limit int
	if master.Class == 4 {
		limit = w.cfg.Int("Common", "GuildInsertMaxUser2", 0)
		n = c.Level/10 + c.Leadership/10
	} else {
		limit = w.cfg.Int("Common", "GuildInsertMaxUser1", 0)
		n = c.Level / 10
	}
	if c.Resets != 0 {
		n = limit
	}
	n = min(n, limit)
	return min(n, guild.MaxMembers)
}

func (w *World) castleBattle() bool { return w.hooks.castleBattle != nil && w.hooks.castleBattle(w) }

// --- world entry / exit --------------------------------------------------------------

// guildEnter is the guild part of DataServer 0x70 (FUN_00415620 with the
// server code): the member goes online, every instance refreshes the guild
// (A0 0A, FUN_004826b0: C2 65 / C2 67 of each online member) and the
// member's instance applies A0 04 (FUN_00481d50).
func (w *World) guildEnter(o *Object) {
	name := charName(o)
	g, slot, ok := w.guildMgr().SetServer(name, w.cfg.ServerCode)
	if !ok {
		return
	}
	n := g.Number
	w.guildAll(func(tw *World) {
		for _, m := range tw.guildMembersHere(n) {
			if m != o {
				tw.showGuild(m)
				tw.showUnion(m)
			}
		}
	})
	w.setGuild(o, &g, g.Members[slot].Status)
	if g.Notice != "" {
		w.send(o, packetNotice(NoticeGuild, 0, 0, 0, 0, 0, g.Notice))
	}
	w.showGuild(o)
	w.showUnion(o)
	if war := w.guildWarOf(n); war != nil && war.active {
		if enemy, ok := w.guildMgr().ByNumber(war.enemy); ok {
			w.send(o, w.packetWarScoreFor(n))
			w.send(o, guild.PacketWarStart(enemy.Name, war.typ, war.team))
		}
	}
}

// guildLeave is DataServer 0x71 (FUN_00415620(-1)): the member is offline.
func (w *World) guildLeave(o *Object) {
	w.guildMgr().SetServer(charName(o), -1)
	if o.P != nil && o.P.Guild != nil {
		if s := o.P.Guild; s.Partner >= 0 {
			s.Partner = -1
		}
		clearGuild(o)
	}
}

func registerGuild(h *hooks) {
	h.start = append(h.start, func(w *World) error {
		gst, _ := w.store.(store.GuildStore)
		w.guilds = &guildWorld{wars: map[int]*guildWar{}}
		return w.guildMgr().Load(context.Background(), gst)
	})
	h.enterWorld = append(h.enterWorld, func(w *World, o *Object) {
		guildOf(o)
		w.guildEnter(o)
	})
	h.leaveWorld = append(h.leaveWorld, func(w *World, o *Object) { w.guildLeave(o) })
	h.guildViewport = func(w *World, o *Object) { w.guildViewportSend(o) }
	h.guildReshow = func(w *World, o *Object) {
		w.showGuild(o)
		w.showUnion(o)
	}
	h.guildNumber = func(w *World, o *Object) uint32 { return uint32(guildNumberOf(o)) }
	h.guildChat = func(w *World, o *Object, kind int, msg []byte) { w.guildChat(o, kind, msg) }

	reg := func(head byte, handler func(*World, *Object, []byte)) {
		h.worldHandlers[head] = func(w *World, o *Object, p []byte) {
			if guildUsable(o) {
				handler(w, o, p)
			}
		}
	}
	reg(0x50, (*World).handleGuildJoinRequest)
	reg(0x51, (*World).handleGuildJoinAnswer)
	reg(0x52, func(w *World, o *Object, _ []byte) { w.sendGuildList(o) })
	reg(0x53, (*World).handleGuildLeave)
	reg(0x54, (*World).handleGuildMasterAnswer)
	reg(0x55, (*World).handleGuildCreate)
	reg(0x57, func(w *World, o *Object, _ []byte) {
		// FUN_004844b0: close the creation window.
		if guildNumberOf(o) == 0 {
			clearIfType(o, ifGuildCreate)
		}
	})
	reg(0x61, (*World).handleGuildWarAnswer)
	reg(0x66, (*World).handleGuildInfo)
	reg(0xE1, (*World).handleGuildAssignStatus)
	reg(0xE2, (*World).handleGuildAssignType)
	reg(0xE5, (*World).handleRelationRequest)
	reg(0xE6, (*World).handleRelationAnswer)
	reg(0xE9, func(w *World, o *Object, _ []byte) { w.handleUnionList(o) })
	reg(0xEB, (*World).handleUnionKick)

	// NPC 241 (Guild Master): FUN_004c2520 from the NPC talk switch.
	h.npcTalk[241] = func(w *World, o, npc *Object) bool {
		w.guildMasterTalk(o)
		return true
	}
}

// guildMasterTalk is FUN_004c2520: already in a guild (msg 245), level
// below GuildCreateMinLevel_AL (246), resets below GuildCreateMinReset_AL
// (247), otherwise 54 when no interface is open.
func (w *World) guildMasterTalk(o *Object) {
	al := o.P.Account.Level
	switch {
	case guildNumberOf(o) != 0:
		w.systemMessage(o, w.messageFor(o, 0xF5))
	case o.P.Char.Level < w.cfg.ALInt("Common", "GuildCreateMinLevel", al, 0):
		w.systemMessage(o, w.messageFor(o, 0xF6))
	case o.P.Char.Resets < w.cfg.ALInt("Common", "GuildCreateMinReset", al, 0):
		w.systemMessage(o, w.messageFor(o, 0xF7))
	case !ifBusy(o):
		// The interface type 5 the 54/55/57 handlers reset is assumed to
		// be set here (the setter was not found, U-GUILD-03).
		setIf(o, ifGuildCreate)
		w.send(o, guild.PacketMasterQuestion())
	}
}

// --- 54 / 55: creation -------------------------------------------------------------

// handleGuildMasterAnswer is 54 (FUN_00484210): [3] 1 = "create": the
// level and reset limits are checked again and the window opens (55).
func (w *World) handleGuildMasterAnswer(o *Object, p []byte) {
	if len(p) < 4 {
		return
	}
	al := o.P.Account.Level
	if p[3] == 1 && o.P.Char.Level >= w.cfg.ALInt("Common", "GuildCreateMinLevel", al, 0) &&
		o.P.Char.Resets >= w.cfg.ALInt("Common", "GuildCreateMinReset", al, 0) {
		w.send(o, guild.PacketCreateWindow())
		return
	}
	clearIfType(o, ifGuildCreate)
}

// handleGuildCreate is 55 (FUN_00484310): [3] type, [4..11] name, [12..43]
// mark. Name with a forbidden character -> 56 result 5; not 3..8
// characters -> 56 result 2; otherwise DataServer A0 00.
func (w *World) handleGuildCreate(o *Object, p []byte) {
	if len(p) < 44 {
		return
	}
	typ := p[3]
	name := cString(p[4:12])
	if !character.ValidName(name) {
		w.log.Info("[CheckNameSyntax] Guild Create error", "account", o.P.Account.Login, "name", o.P.Char.Name, "guild", name)
		w.send(o, guild.PacketCreateResult(5, 0))
		return
	}
	if len(name) < 3 || len(name) > 9 {
		w.send(o, guild.PacketCreateResult(2, 0))
		return
	}
	var mark [32]byte
	copy(mark[:], p[12:44])
	clearIfType(o, ifGuildCreate)
	master, c, server := o.P.Char.Name, o.conn, w.cfg.ServerCode
	mgr := w.guildMgr()
	w.async(func(ctx context.Context) func() {
		result, _ := mgr.Create(ctx, name, master, mark, int(typ), server)
		return func() {
			// FUN_00481660: the result goes to the master; on success the
			// guild is set and shown (A0 00, then A0 04 of FUN_00415620).
			o := w.objectOf(c)
			if o == nil || o.State != StatePlaying {
				return
			}
			w.send(o, guild.PacketCreateResult(result, typ))
			if result != 1 {
				return
			}
			clearIfType(o, ifGuildCreate)
			w.guildEnter(o)
		}
	})
}

// --- 50 / 51: joining -------------------------------------------------------------

// handleGuildJoinRequest is 50 (FUN_00483580): [3..4] the master's index.
func (w *World) handleGuildJoinRequest(o *Object, p []byte) {
	if len(p) < 5 {
		return
	}
	ti := int(p[3])<<8 | int(p[4])
	if ti >= MaxObjects {
		return
	}
	t := w.objects[ti]
	if !guildUsable(t) {
		return
	}
	var result byte
	switch {
	case !w.acceptsRequests(t):
		result = 0
	case guildNumberOf(t) == 0 || guildOf(t).Status != store.GuildMasterStatus:
		result = 4
	case guildNumberOf(o) != 0:
		result = 5
	case ifBusy(o) || ifBusy(t) || w.guildWarActive(guildNumberOf(t)):
		result = 6
	case w.castleBattle():
		w.systemMessage(o, w.messageFor(o, 0x2E4))
		return
	case o.P.Char.Level < 6:
		result = 7
	default:
		g, _ := w.guildMgr().ByNumber(guildNumberOf(t))
		if g.Count() >= w.guildCapacity(t) {
			result = 2
			break
		}
		now := w.now()
		setIf(o, ifGuildJoin)
		setIf(t, ifGuildJoin)
		guildOf(o).Partner, guildOf(o).RequestAt = ti, now
		guildOf(t).Partner, guildOf(t).RequestAt = o.Index, now
		w.send(t, guild.PacketJoinRequest(o.Index))
		return
	}
	w.send(o, guild.PacketResult(result))
}

// handleGuildJoinAnswer is 51 (FUN_00483810): [3] accept, [4..5] the
// requester's index. Both sides must be in the join interface.
func (w *World) handleGuildJoinAnswer(o *Object, p []byte) {
	if len(p) < 6 {
		return
	}
	ri := int(p[4])<<8 | int(p[5])
	if ri >= MaxObjects {
		return
	}
	r := w.objects[ri]
	if !guildUsable(r) {
		return
	}
	in := func(x *Object) bool { s := items(x).If; return s.Use && s.Type == ifGuildJoin && s.State == 0 }
	if !in(o) || !in(r) {
		return
	}
	defer func() {
		clearIf(o)
		clearIf(r)
		guildOf(o).Partner, guildOf(r).Partner = -1, -1
	}()
	switch {
	case p[3] == 0 || !w.acceptsRequests(o):
		w.send(r, guild.PacketResult(0))
	case guildNumberOf(o) == 0 || guildOf(o).Status != store.GuildMasterStatus:
		w.send(r, guild.PacketResult(4))
	case guildNumberOf(r) != 0:
		w.send(r, guild.PacketResult(5))
	case w.guildWarActive(guildNumberOf(o)):
		w.send(r, guild.PacketResult(6))
	case w.castleBattle():
		w.systemMessage(o, w.messageFor(o, 0x2E4))
	case r.P.Char.Level < 6:
		w.send(r, guild.PacketResult(7))
	default:
		g, _ := w.guildMgr().ByNumber(guildNumberOf(o))
		if g.Count() >= w.guildCapacity(o) {
			w.send(r, guild.PacketResult(2))
			return
		}
		w.guildAddMember(g.Name, r.P.Char.Name)
	}
}

// guildAddMember is DataServer A0 02 (FUN_00483380 -> FUN_004141f0) and
// its reply FUN_00481a60: 51 result to the new member; on success the
// member is set, shown (C2 65 / C2 67) and every instance learns it.
func (w *World) guildAddMember(guildName, name string) {
	mgr, server := w.guildMgr(), w.cfg.ServerCode
	w.async(func(ctx context.Context) func() {
		result := mgr.AddMember(ctx, guildName, name, server)
		return func() {
			if result != 1 {
				if t := w.playerHere(name); t != nil {
					w.send(t, guild.PacketResult(result))
				}
				return
			}
			w.guildAll(func(tw *World) {
				t := tw.playerHere(name)
				if t == nil {
					return
				}
				tw.send(t, guild.PacketResult(1))
				g, slot, ok := tw.guildMgr().Of(name)
				if !ok {
					return
				}
				tw.setGuild(t, &g, g.Members[slot].Status)
				tw.showGuild(t)
				tw.showUnion(t)
			})
		}
	})
}

// --- 53: leave / kick / disband ------------------------------------------------------

// handleGuildLeave is 53 (FUN_00483ed0): [3..12] name, [13..22] personal
// code. The master naming itself disbands the guild (A0 01), the master
// naming a member kicks it and a member naming itself leaves (A0 03).
func (w *World) handleGuildLeave(o *Object, p []byte) {
	if len(p) < 23 {
		return
	}
	gs := guildOf(o)
	now := w.now()
	if now.Sub(gs.LastLeave) < 300*time.Millisecond {
		return
	}
	gs.LastLeave = now
	if w.cfg.Int("Common", "GuildDeleteSwitch", 0) == 0 || gs.Number == 0 {
		w.send(o, guild.PacketLeaveResult(0))
		return
	}
	if o.P.Account.Lock > 0 { // obj+0x24fc: message 0x6F2 and the failure reply
		w.systemMessage(o, w.messageFor(o, 0x6f2))
		w.send(o, guild.PacketLeaveResult(0))
		return
	}
	if w.castleBattle() {
		w.systemMessage(o, w.messageFor(o, 0x2E5))
		return
	}
	name := cString(p[3:13])
	code := p[13:23]
	self := o.P.Char.Name
	g, ok := w.guildMgr().ByNumber(gs.Number)
	if !ok {
		w.send(o, guild.PacketLeaveResult(0))
		return
	}
	if gs.Status == store.GuildMasterStatus && name == self {
		if !w.personalCodeOK(o, code) || (w.cfg.Int("Common", "GuildOwnerDestroyLimit", 0) != 0 && w.castleOwner(g.Name)) {
			w.send(o, guild.PacketLeaveResult(0))
			return
		}
		w.guildWarAbandon(o)
		w.guildDelete(g.Name, self)
		return
	}
	if gs.Status != store.GuildMasterStatus && name != self {
		w.send(o, guild.PacketLeaveResult(0))
		return
	}
	if !w.personalCodeOK(o, code) {
		w.send(o, guild.PacketLeaveResult(0))
		return
	}
	w.guildDelMember(g.Name, name)
}

func (w *World) castleOwner(guildName string) bool {
	return w.hooks.castleOwner != nil && w.hooks.castleOwner(w, guildName)
}

// guildDelete is DataServer A0 01 and its reply FUN_004817d0: every online
// member loses the guild (C1 5D) and gets 53 with the result.
func (w *World) guildDelete(guildName, master string) {
	mgr := w.guildMgr()
	g, _ := mgr.Find(guildName)
	n := g.Number
	w.async(func(ctx context.Context) func() {
		result, changes := mgr.Remove(ctx, guildName)
		return func() {
			if result != 1 {
				if t := w.playerHere(master); t != nil {
					w.send(t, guild.PacketLeaveResult(result))
				}
				return
			}
			w.guildAll(func(tw *World) {
				for _, m := range tw.guildMembersHere(n) {
					clearGuild(m)
					p := guild.PacketViewportDel(m.Index)
					tw.send(m, p)
					tw.broadcast(m, p)
					tw.send(m, guild.PacketLeaveResult(result))
				}
				delete(tw.guilds.wars, n)
				for _, c := range changes {
					tw.unionDissolved(c)
				}
			})
		}
	})
}

// guildDelMember is DataServer A0 03 and its reply FUN_00481c20: the member
// gets 53 and loses the guild; the master gets the new list.
func (w *World) guildDelMember(guildName, name string) {
	mgr := w.guildMgr()
	w.async(func(ctx context.Context) func() {
		result := mgr.DelMember(ctx, guildName, name)
		return func() {
			if result != 1 {
				if t := w.playerHere(name); t != nil {
					w.send(t, guild.PacketLeaveResult(result))
				}
				return
			}
			g, _ := mgr.Find(guildName)
			w.guildAll(func(tw *World) {
				if t := tw.playerHere(name); t != nil {
					tw.send(t, guild.PacketLeaveResult(result))
					clearGuild(t)
					p := guild.PacketViewportDel(t.Index)
					tw.send(t, p)
					tw.broadcast(t, p)
				}
				if m := tw.playerHere(g.Master); m != nil && guildNumberOf(m) == g.Number {
					tw.sendGuildList(m)
				}
			})
		}
	})
}

// --- 66: guild information ------------------------------------------------------------

// handleGuildInfo is 66 (FUN_00484890): [4..7] guild number.
func (w *World) handleGuildInfo(o *Object, p []byte) {
	if len(p) < 8 {
		return
	}
	g, ok := w.guildMgr().ByNumber(int(binary.LittleEndian.Uint32(p[4:])))
	if !ok {
		return
	}
	w.send(o, guild.PacketInfo(&g, w.guildMgr().UnionName(g.Union)))
}

// --- E1 / E2: ranks and type ------------------------------------------------------------

// handleGuildAssignStatus is E1 (FUN_00484a40): [3] type (1..3), [4]
// status (0, 0x20 battle master, 0x40 assistant), [5..14] member name.
func (w *World) handleGuildAssignStatus(o *Object, p []byte) {
	if len(p) < 15 {
		return
	}
	typ, status := p[3], int(p[4])
	if typ < 1 || typ > 3 {
		return
	}
	name := cString(p[5:15])
	gs := guildOf(o)
	if gs.Number == 0 {
		w.send(o, guild.PacketAssignResult(typ, 0x10, name))
		return
	}
	g, _ := w.guildMgr().ByNumber(gs.Number)
	if name == o.P.Char.Name || g.Member(name) < 0 || gs.Status != store.GuildMasterStatus {
		w.send(o, guild.PacketAssignResult(typ, 0x11, name))
		return
	}
	switch status {
	case store.GuildMemberStatus:
	case store.GuildBattleMasterStatus:
		if g.StatusCount(store.GuildBattleMasterStatus) >= o.P.Char.Level/200+1 {
			w.send(o, guild.PacketAssignResult(typ, 0x12, name))
			return
		}
	case store.GuildAssistantStatus:
		if g.StatusCount(store.GuildAssistantStatus) > 0 {
			w.send(o, guild.PacketAssignResult(typ, 0x12, name))
			return
		}
	default:
		w.send(o, guild.PacketAssignResult(typ, 0x12, name))
		return
	}
	mgr := w.guildMgr()
	guildName, n, master := g.Name, g.Number, o.P.Char.Name
	w.async(func(ctx context.Context) func() {
		result := mgr.SetStatus(ctx, guildName, name, status)
		return func() {
			// FUN_00481fe0 (A0 05, to every instance on success).
			if result != 1 {
				if m := w.playerHere(master); m != nil {
					w.send(m, guild.PacketAssignResult(typ, result, name))
				}
				return
			}
			w.guildAll(func(tw *World) {
				if m := tw.playerHere(master); m != nil {
					tw.send(m, guild.PacketAssignResult(typ, result, name))
				}
				if t := tw.playerHere(name); t != nil && guildNumberOf(t) == n {
					guildOf(t).Status = status
					tw.showGuild(t)
				}
				id := 0xC0
				switch status {
				case store.GuildAssistantStatus:
					id = 0xBE
				case store.GuildBattleMasterStatus:
					id = 0xBF
				}
				tw.guildMessage(n, NoticeGold, id, name)
				if m := tw.playerHere(master); m != nil {
					tw.sendGuildList(m)
				}
			})
		}
	})
}

// handleGuildAssignType is E2 (FUN_00484ca0): [3] the new guild type.
func (w *World) handleGuildAssignType(o *Object, p []byte) {
	if len(p) < 4 {
		return
	}
	typ := p[3]
	gs := guildOf(o)
	if gs.Number == 0 {
		w.send(o, guild.PacketTypeResult(typ, 0x10))
		return
	}
	if gs.Status != store.GuildMasterStatus {
		w.send(o, guild.PacketTypeResult(typ, 0x11))
		return
	}
	mgr := w.guildMgr()
	g, _ := mgr.ByNumber(gs.Number)
	guildName, n, master := g.Name, g.Number, o.P.Char.Name
	w.async(func(ctx context.Context) func() {
		result := mgr.Update(ctx, guildName, func(s store.GuildStore) error { return s.SetGuildType(ctx, guildName, int(typ)) },
			func(r *guild.Record) { r.Type = int(typ) })
		return func() {
			// FUN_00482140 (A0 06).
			if result != 1 {
				if m := w.playerHere(master); m != nil {
					w.send(m, guild.PacketTypeResult(typ, result))
				}
				return
			}
			w.guildAll(func(tw *World) {
				if m := tw.playerHere(master); m != nil {
					tw.send(m, guild.PacketTypeResult(typ, result))
				}
				tw.guildMessage(n, NoticeGold, 0xC1, guildName, int(typ))
				for _, m := range tw.guildMembersHere(n) {
					tw.showGuild(m)
				}
				if m := tw.playerHere(master); m != nil {
					tw.sendGuildList(m)
				}
			})
		}
	})
}

// --- chat and notice ----------------------------------------------------------------------

// guildChat is the "@" branch of FUN_004d7720: kind 0 guild chat and 1
// union chat ("@@", only in a union) go through DataServer A0 07 / A0 08
// (FUN_00483500 -> FUN_00482430) as C1 49 00 name msg[60] with the prefix
// kept; kind 2 "@>" (master only) is the guild notice (A0 06 -> A0 07,
// FUN_00482320: C1 0D type 2 to every member).
func (w *World) guildChat(o *Object, kind int, msg []byte) {
	gs := guildOf(o)
	if gs.Number == 0 {
		return
	}
	g, ok := w.guildMgr().ByNumber(gs.Number)
	if !ok {
		return
	}
	n := g.Number
	switch kind {
	case 2:
		if gs.Status != store.GuildMasterStatus {
			return
		}
		text := strings.ReplaceAll(string(cstr(msg)), "'", "\"")
		if len(text) > 60 {
			text = text[:60]
		}
		mgr := w.guildMgr()
		w.async(func(ctx context.Context) func() {
			if mgr.Update(ctx, g.Name, func(s store.GuildStore) error { return s.SetGuildNotice(ctx, g.Name, text) },
				func(r *guild.Record) { r.Notice = text }) != 1 {
				return nil
			}
			return func() {
				w.guildAll(func(tw *World) {
					tw.guildSendAll(n, packetNotice(NoticeGuild, 0, 0, 0, 0, 0, text))
				})
			}
		})
		return
	case 1:
		if g.Union == 0 {
			return
		}
	}
	prefix := "@"
	if kind == 1 {
		prefix = "@@"
	}
	name := make([]byte, 10)
	copy(name, o.P.Char.Name)
	p := packetChat(name, append([]byte(prefix), cstr(msg)...))
	targets := []int{n}
	if kind == 1 {
		targets = nil
		for _, u := range w.guildMgr().UnionList(g.Union) {
			targets = append(targets, u.Number)
		}
	}
	w.guildAll(func(tw *World) {
		for _, t := range targets {
			tw.guildSendAll(t, p)
		}
	})
}
