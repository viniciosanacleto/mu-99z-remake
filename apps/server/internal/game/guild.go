package game

import (
	"context"
	"encoding/binary"
	"strings"
	"time"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/frame"
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

func putName8(dst []byte, s string) { copy(dst[:8], s) }

// packetGuildJoinRequest is C1 05 50 idxHi idxLo (FUN_00483580): someone
// asks the master to join.
func packetGuildJoinRequest(index int) []byte {
	p := newPacket(frame.C1, 0x50, -1, 5)
	p[3], p[4] = byte(index>>8), byte(index)
	return p
}

// packetGuildResult is C1 04 51 result (FUN_00485640).
func packetGuildResult(result byte) []byte {
	p := newPacket(frame.C1, 0x51, -1, 4)
	p[3] = result
	return p
}

// packetGuildLeaveResult is C1 04 53 result (FUN_004856e0).
func packetGuildLeaveResult(result byte) []byte {
	p := newPacket(frame.C1, 0x53, -1, 4)
	p[3] = result
	return p
}

// packetGuildMasterQuestion is C1 03 54 (FUN_004da9a0), sent by the Guild
// Master NPC.
func packetGuildMasterQuestion() []byte { return newPacket(frame.C1, 0x54, -1, 3) }

// packetGuildCreateWindow is C1 03 55 (FUN_00484210): open the creation window.
func packetGuildCreateWindow() []byte { return newPacket(frame.C1, 0x55, -1, 3) }

// packetGuildCreateResult is C1 05 56 result type (FUN_00485780).
func packetGuildCreateResult(result, typ byte) []byte {
	p := newPacket(frame.C1, 0x56, -1, 5)
	p[3], p[4] = result, typ
	return p
}

// packetGuildViewportDel is C1 05 5D idxHi idxLo (FUN_005145e0): the
// player's guild is gone.
func packetGuildViewportDel(index int) []byte {
	p := newPacket(frame.C1, 0x5D, -1, 5)
	p[3], p[4] = byte(index>>8), byte(index)
	return p
}

// guildListMember is one 13-byte entry of C2 52.
type guildListMember struct {
	Name   string
	Server int
	Status int
}

// packetGuildList is C2 size 52 result count pad[2] totalScore(u32)
// warScore rival[9] pad[2] + 13 bytes per member {name[10], server byte,
// 0x80|server&0x7f (online) or server&0x7f, status} (FUN_00483a30).
func packetGuildList(score int, warScore byte, rival string, members []guildListMember) []byte {
	p := newPacket(frame.C2, 0x52, -1, 24+13*len(members))
	p[4], p[5] = 1, byte(len(members))
	u32le(p[8:], uint32(score))
	p[12] = warScore
	copy(p[13:21], rival)
	for i, m := range members {
		e := p[24+13*i:]
		putName(e, m.Name)
		e[10] = byte(m.Server)
		e[11] = byte(m.Server) & 0x7F
		if m.Server != -1 {
			e[11] |= 0x80
		}
		e[12] = byte(m.Status)
	}
	return p
}

// guildViewEntry is one 12-byte C2 65 entry.
type guildViewEntry struct {
	Number   int
	Status   int
	Type     int
	Relation int
	Index    int
}

func (e guildViewEntry) put(b []byte) {
	u32le(b, uint32(e.Number))
	b[4], b[5], b[6] = byte(e.Status), byte(e.Type), byte(e.Relation)
	b[7], b[8] = byte(e.Index>>8), byte(e.Index)
	if e.Status == store.GuildMasterStatus {
		b[7] |= 0x80
	}
}

// packetGuildViewport is C2 size 65 count + 12 bytes per player {number u32,
// status, type, relation, idxHi | 0x80 for the master, idxLo, pad[3]}
// (FUN_005160a0; FUN_00516f60 sends one entry with relation 0).
func packetGuildViewport(entries []guildViewEntry) []byte {
	p := newPacket(frame.C2, 0x65, -1, 5+12*len(entries))
	p[4] = byte(len(entries))
	for i, e := range entries {
		e.put(p[5+12*i:])
	}
	return p
}

// unionViewEntry is one 20-byte C2 67 entry.
type unionViewEntry struct {
	Index    int
	Number   int
	Relation int
	Union    string
}

// packetUnionViewport is C2 size 67 count + 20 bytes per player {idxHi,
// idxLo, pad[2], guild number u32, relation u32 (low byte), union master
// guild name[8]} (FUN_005162f0 / FUN_00517080).
func packetUnionViewport(entries []unionViewEntry) []byte {
	p := newPacket(frame.C2, 0x67, -1, 5+20*len(entries))
	p[4] = byte(len(entries))
	for i, e := range entries {
		b := p[5+20*i:]
		b[0], b[1] = byte(e.Index>>8), byte(e.Index)
		u32le(b[4:], uint32(e.Number))
		b[8] = byte(e.Relation)
		putName8(b[9:], e.Union)
	}
	return p
}

// packetGuildInfo is C1 3C 66 pad number(u32) type union[8] name[8] mark[32]
// pad[3] (FUN_00484890).
func packetGuildInfo(g *guildRecord, union string) []byte {
	p := newPacket(frame.C1, 0x66, -1, 0x3C)
	u32le(p[4:], uint32(g.Number))
	p[8] = byte(g.Type)
	putName8(p[9:], union)
	putName8(p[17:], g.Name)
	copy(p[25:57], g.Mark[:])
	return p
}

// packetGuildAssignResult is C1 0F E1 type result name[10] (FUN_00485ba0).
func packetGuildAssignResult(typ, result byte, name string) []byte {
	p := newPacket(frame.C1, 0xE1, -1, 0x0F)
	p[3], p[4] = typ, result
	putName(p[5:], name)
	return p
}

// packetGuildTypeResult is C1 05 E2 type result (FUN_00485c70).
func packetGuildTypeResult(typ, result byte) []byte {
	p := newPacket(frame.C1, 0xE2, -1, 5)
	p[3], p[4] = typ, result
	return p
}

// packetRelationRequest is C1 07 E5 rel req idxHi idxLo (FUN_00485d10).
func packetRelationRequest(rel, req byte, index int) []byte {
	p := newPacket(frame.C1, 0xE5, -1, 7)
	p[3], p[4], p[5], p[6] = rel, req, byte(index>>8), byte(index)
	return p
}

// packetRelationResult is C1 08 E6 rel req result idxHi idxLo (FUN_00485de0).
func packetRelationResult(rel, req, result byte, index int) []byte {
	p := newPacket(frame.C1, 0xE6, -1, 8)
	p[3], p[4], p[5], p[6], p[7] = rel, req, result, byte(index>>8), byte(index)
	return p
}

// packetUnionList is C2 size E9 count result unionCount rivalCount + 41
// bytes per guild {members, mark[32], name[8]} (FUN_00482c40).
func packetUnionList(result, unions, rivals byte, guilds []guildRecord) []byte {
	p := newPacket(frame.C2, 0xE9, -1, 8+41*len(guilds))
	p[4], p[5], p[6], p[7] = byte(len(guilds)), result, unions, rivals
	for i := range guilds {
		e := p[8+41*i:]
		e[0] = byte(guilds[i].count())
		copy(e[1:33], guilds[i].Mark[:])
		putName8(e[33:], guilds[i].Name)
	}
	return p
}

// packetUnionKickResult is C1 07 EB 01 result 02 01 (FUN_004831f0).
func packetUnionKickResult(result byte) []byte {
	p := newPacket(frame.C1, 0xEB, 0x01, 7)
	p[4], p[5], p[6] = result, 2, 1
	return p
}

// --- views ------------------------------------------------------------------------

// guildEntry builds o's C2 65 entry as seen by viewer (nil = itself).
func (w *World) guildEntry(o, viewer *Object) (guildViewEntry, bool) {
	gs := o.P.Guild
	if gs == nil || gs.Number == 0 {
		return guildViewEntry{}, false
	}
	g, ok := w.guildMgr().byNumber(gs.Number)
	if !ok {
		return guildViewEntry{}, false
	}
	e := guildViewEntry{Number: g.Number, Status: gs.Status, Type: g.Type, Index: o.Index}
	if viewer != nil && guildNumberOf(viewer) != 0 {
		e.Relation = w.guildMgr().relation(g.Number, guildNumberOf(viewer))
	}
	return e, true
}

// unionEntry builds o's C2 67 entry as seen by viewer (nil = itself).
func (w *World) unionEntry(o, viewer *Object) (unionViewEntry, bool) {
	n := guildNumberOf(o)
	g, ok := w.guildMgr().byNumber(n)
	if !ok {
		return unionViewEntry{}, false
	}
	e := unionViewEntry{Index: o.Index, Number: g.Number, Union: w.guildMgr().unionName(g.Union)}
	if viewer != nil && guildNumberOf(viewer) != 0 {
		e.Relation = w.guildMgr().relation(g.Number, guildNumberOf(viewer))
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
	p := packetGuildViewport([]guildViewEntry{e})
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
	w.send(o, packetUnionViewport([]unionViewEntry{e}))
	for _, v := range o.VP2 {
		if v.State == VPEmpty || v.Type != byte(TypePlayer) || v.Index < 0 || v.Index >= MaxObjects {
			continue
		}
		t := w.objects[v.Index]
		if !t.IsPlayer() {
			continue
		}
		e2, _ := w.unionEntry(o, t)
		w.send(t, packetUnionViewport([]unionViewEntry{e2}))
	}
}

// guildViewportSend is the guild part of FUN_0050dbf0: C2 65 for the new
// players in the viewport (FUN_005160a0) and C2 67 for the visible ones
// (FUN_005162f0).
func (w *World) guildViewportSend(o *Object) {
	var news []guildViewEntry
	var unions []unionViewEntry
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
		w.send(o, packetGuildViewport(news))
	}
	if len(unions) > 0 {
		w.send(o, packetUnionViewport(unions))
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
func (w *World) setGuild(o *Object, g *guildRecord, status int) {
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
	g, ok := w.guildMgr().byNumber(n)
	if !ok {
		return
	}
	var ms []guildListMember
	for _, m := range g.Members {
		if m.Name != "" {
			ms = append(ms, guildListMember{Name: m.Name, Server: m.Server, Status: m.Status})
		}
	}
	rival := ""
	if g.Rival != 0 {
		if r, ok := w.guildMgr().byNumber(g.Rival); ok {
			rival = r.Name
		}
	}
	if len(ms) > 0 {
		w.send(o, packetGuildList(g.Score, w.warScore(n), rival, ms))
	}
}

// guildMaxMembers is FUN_00486200: the members a master can have. Dark
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
	return min(n, guildMaxMembers)
}

func (w *World) castleBattle() bool { return castleBattleHook != nil && castleBattleHook(w) }

// --- world entry / exit --------------------------------------------------------------

// guildEnter is the guild part of DataServer 0x70 (FUN_00415620 with the
// server code): the member goes online, every instance refreshes the guild
// (A0 0A, FUN_004826b0: C2 65 / C2 67 of each online member) and the
// member's instance applies A0 04 (FUN_00481d50).
func (w *World) guildEnter(o *Object) {
	name := charName(o)
	g, slot, ok := w.guildMgr().setServer(name, w.cfg.ServerCode)
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
		if enemy, ok := w.guildMgr().byNumber(war.enemy); ok {
			w.send(o, w.packetWarScoreFor(n))
			w.send(o, packetGuildWarStart(enemy.Name, war.typ, war.team))
		}
	}
}

// guildLeave is DataServer 0x71 (FUN_00415620(-1)): the member is offline.
func (w *World) guildLeave(o *Object) {
	w.guildMgr().setServer(charName(o), -1)
	if o.P != nil && o.P.Guild != nil {
		if s := o.P.Guild; s.Partner >= 0 {
			s.Partner = -1
		}
		clearGuild(o)
	}
}

func init() {
	startHooks = append(startHooks, func(w *World) error {
		gst, _ := w.store.(store.GuildStore)
		w.guilds = &guildWorld{wars: map[int]*guildWar{}}
		return w.guildMgr().load(context.Background(), gst)
	})
	enterWorldHooks = append(enterWorldHooks, func(w *World, o *Object) {
		guildOf(o)
		w.guildEnter(o)
	})
	leaveWorldHooks = append(leaveWorldHooks, func(w *World, o *Object) { w.guildLeave(o) })
	guildViewportHook = func(w *World, o *Object) { w.guildViewportSend(o) }
	guildReshowHook = func(w *World, o *Object) {
		w.showGuild(o)
		w.showUnion(o)
	}
	guildNumberHook = func(w *World, o *Object) uint32 { return uint32(guildNumberOf(o)) }
	guildChatHook = func(w *World, o *Object, kind int, msg []byte) { w.guildChat(o, kind, msg) }

	reg := func(head byte, h func(*World, *Object, []byte)) {
		worldHandlers[head] = func(w *World, o *Object, p []byte) {
			if guildUsable(o) {
				h(w, o, p)
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
	npcTalkHooks[241] = func(w *World, o, npc *Object) bool {
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
		w.send(o, packetGuildMasterQuestion())
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
		w.send(o, packetGuildCreateWindow())
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
	if !validName(name) {
		w.log.Info("[CheckNameSyntax] Guild Create error", "account", o.P.Account.Login, "name", o.P.Char.Name, "guild", name)
		w.send(o, packetGuildCreateResult(5, 0))
		return
	}
	if len(name) < 3 || len(name) > 9 {
		w.send(o, packetGuildCreateResult(2, 0))
		return
	}
	var mark [32]byte
	copy(mark[:], p[12:44])
	clearIfType(o, ifGuildCreate)
	master, c, server := o.P.Char.Name, o.conn, w.cfg.ServerCode
	mgr := w.guildMgr()
	w.async(func(ctx context.Context) func() {
		result, _ := mgr.create(ctx, name, master, mark, int(typ), server)
		return func() {
			// FUN_00481660: the result goes to the master; on success the
			// guild is set and shown (A0 00, then A0 04 of FUN_00415620).
			o := w.objectOf(c)
			if o == nil || o.State != StatePlaying {
				return
			}
			w.send(o, packetGuildCreateResult(result, typ))
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
		g, _ := w.guildMgr().byNumber(guildNumberOf(t))
		if g.count() >= w.guildCapacity(t) {
			result = 2
			break
		}
		now := w.now()
		setIf(o, ifGuildJoin)
		setIf(t, ifGuildJoin)
		guildOf(o).Partner, guildOf(o).RequestAt = ti, now
		guildOf(t).Partner, guildOf(t).RequestAt = o.Index, now
		w.send(t, packetGuildJoinRequest(o.Index))
		return
	}
	w.send(o, packetGuildResult(result))
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
		w.send(r, packetGuildResult(0))
	case guildNumberOf(o) == 0 || guildOf(o).Status != store.GuildMasterStatus:
		w.send(r, packetGuildResult(4))
	case guildNumberOf(r) != 0:
		w.send(r, packetGuildResult(5))
	case w.guildWarActive(guildNumberOf(o)):
		w.send(r, packetGuildResult(6))
	case w.castleBattle():
		w.systemMessage(o, w.messageFor(o, 0x2E4))
	case r.P.Char.Level < 6:
		w.send(r, packetGuildResult(7))
	default:
		g, _ := w.guildMgr().byNumber(guildNumberOf(o))
		if g.count() >= w.guildCapacity(o) {
			w.send(r, packetGuildResult(2))
			return
		}
		w.guildAddMember(g.Name, r.P.Char.Name)
	}
}

// guildAddMember is DataServer A0 02 (FUN_00483380 -> FUN_004141f0) and
// its reply FUN_00481a60: 51 result to the new member; on success the
// member is set, shown (C2 65 / C2 67) and every instance learns it.
func (w *World) guildAddMember(guild, name string) {
	mgr, server := w.guildMgr(), w.cfg.ServerCode
	w.async(func(ctx context.Context) func() {
		result := mgr.addMember(ctx, guild, name, server)
		return func() {
			if result != 1 {
				if t := w.playerHere(name); t != nil {
					w.send(t, packetGuildResult(result))
				}
				return
			}
			w.guildAll(func(tw *World) {
				t := tw.playerHere(name)
				if t == nil {
					return
				}
				tw.send(t, packetGuildResult(1))
				g, slot, ok := tw.guildMgr().of(name)
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
		w.send(o, packetGuildLeaveResult(0))
		return
	}
	if o.P.Account.Lock > 0 { // obj+0x24fc: message 0x6F2 and the failure reply
		w.systemMessage(o, w.messageFor(o, 0x6f2))
		w.send(o, packetGuildLeaveResult(0))
		return
	}
	if w.castleBattle() {
		w.systemMessage(o, w.messageFor(o, 0x2E5))
		return
	}
	name := cString(p[3:13])
	code := p[13:23]
	self := o.P.Char.Name
	g, ok := w.guildMgr().byNumber(gs.Number)
	if !ok {
		w.send(o, packetGuildLeaveResult(0))
		return
	}
	if gs.Status == store.GuildMasterStatus && name == self {
		if !w.personalCodeOK(o, code) || (w.cfg.Int("Common", "GuildOwnerDestroyLimit", 0) != 0 && w.castleOwner(g.Name)) {
			w.send(o, packetGuildLeaveResult(0))
			return
		}
		w.guildWarAbandon(o)
		w.guildDelete(g.Name, self)
		return
	}
	if gs.Status != store.GuildMasterStatus && name != self {
		w.send(o, packetGuildLeaveResult(0))
		return
	}
	if !w.personalCodeOK(o, code) {
		w.send(o, packetGuildLeaveResult(0))
		return
	}
	w.guildDelMember(g.Name, name)
}

// castleOwnerHook reports whether a guild owns the castle (FUN_00429b00,
// Castle Siege module).
var castleOwnerHook func(w *World, guild string) bool

func (w *World) castleOwner(guild string) bool {
	return castleOwnerHook != nil && castleOwnerHook(w, guild)
}

// guildDelete is DataServer A0 01 and its reply FUN_004817d0: every online
// member loses the guild (C1 5D) and gets 53 with the result.
func (w *World) guildDelete(guild, master string) {
	mgr := w.guildMgr()
	g, _ := mgr.find(guild)
	n := g.Number
	w.async(func(ctx context.Context) func() {
		result, changes := mgr.remove(ctx, guild)
		return func() {
			if result != 1 {
				if t := w.playerHere(master); t != nil {
					w.send(t, packetGuildLeaveResult(result))
				}
				return
			}
			w.guildAll(func(tw *World) {
				for _, m := range tw.guildMembersHere(n) {
					clearGuild(m)
					p := packetGuildViewportDel(m.Index)
					tw.send(m, p)
					tw.broadcast(m, p)
					tw.send(m, packetGuildLeaveResult(result))
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
func (w *World) guildDelMember(guild, name string) {
	mgr := w.guildMgr()
	w.async(func(ctx context.Context) func() {
		result := mgr.delMember(ctx, guild, name)
		return func() {
			if result != 1 {
				if t := w.playerHere(name); t != nil {
					w.send(t, packetGuildLeaveResult(result))
				}
				return
			}
			g, _ := mgr.find(guild)
			w.guildAll(func(tw *World) {
				if t := tw.playerHere(name); t != nil {
					tw.send(t, packetGuildLeaveResult(result))
					clearGuild(t)
					p := packetGuildViewportDel(t.Index)
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
	g, ok := w.guildMgr().byNumber(int(binary.LittleEndian.Uint32(p[4:])))
	if !ok {
		return
	}
	w.send(o, packetGuildInfo(&g, w.guildMgr().unionName(g.Union)))
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
		w.send(o, packetGuildAssignResult(typ, 0x10, name))
		return
	}
	g, _ := w.guildMgr().byNumber(gs.Number)
	if name == o.P.Char.Name || g.member(name) < 0 || gs.Status != store.GuildMasterStatus {
		w.send(o, packetGuildAssignResult(typ, 0x11, name))
		return
	}
	switch status {
	case store.GuildMemberStatus:
	case store.GuildBattleMasterStatus:
		if g.statusCount(store.GuildBattleMasterStatus) >= o.P.Char.Level/200+1 {
			w.send(o, packetGuildAssignResult(typ, 0x12, name))
			return
		}
	case store.GuildAssistantStatus:
		if g.statusCount(store.GuildAssistantStatus) > 0 {
			w.send(o, packetGuildAssignResult(typ, 0x12, name))
			return
		}
	default:
		w.send(o, packetGuildAssignResult(typ, 0x12, name))
		return
	}
	mgr := w.guildMgr()
	guild, n, master := g.Name, g.Number, o.P.Char.Name
	w.async(func(ctx context.Context) func() {
		result := mgr.setStatus(ctx, guild, name, status)
		return func() {
			// FUN_00481fe0 (A0 05, to every instance on success).
			if result != 1 {
				if m := w.playerHere(master); m != nil {
					w.send(m, packetGuildAssignResult(typ, result, name))
				}
				return
			}
			w.guildAll(func(tw *World) {
				if m := tw.playerHere(master); m != nil {
					tw.send(m, packetGuildAssignResult(typ, result, name))
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
		w.send(o, packetGuildTypeResult(typ, 0x10))
		return
	}
	if gs.Status != store.GuildMasterStatus {
		w.send(o, packetGuildTypeResult(typ, 0x11))
		return
	}
	mgr := w.guildMgr()
	g, _ := mgr.byNumber(gs.Number)
	guild, n, master := g.Name, g.Number, o.P.Char.Name
	w.async(func(ctx context.Context) func() {
		result := mgr.update(ctx, guild, func(s store.GuildStore) error { return s.SetGuildType(ctx, guild, int(typ)) },
			func(r *guildRecord) { r.Type = int(typ) })
		return func() {
			// FUN_00482140 (A0 06).
			if result != 1 {
				if m := w.playerHere(master); m != nil {
					w.send(m, packetGuildTypeResult(typ, result))
				}
				return
			}
			w.guildAll(func(tw *World) {
				if m := tw.playerHere(master); m != nil {
					tw.send(m, packetGuildTypeResult(typ, result))
				}
				tw.guildMessage(n, NoticeGold, 0xC1, guild, int(typ))
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
	g, ok := w.guildMgr().byNumber(gs.Number)
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
			if mgr.update(ctx, g.Name, func(s store.GuildStore) error { return s.SetGuildNotice(ctx, g.Name, text) },
				func(r *guildRecord) { r.Notice = text }) != 1 {
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
		for _, u := range w.guildMgr().unionList(g.Union) {
			targets = append(targets, u.Number)
		}
	}
	w.guildAll(func(tw *World) {
		for _, t := range targets {
			tw.guildSendAll(t, p)
		}
	})
}
