package game

import (
	"context"
	"sort"
	"time"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/castlesiege"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/guild"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/store"
)

// Registration (B2 00..04, B4), the choice of the siege guilds at
// NOTIFY/READY and the sides of the players (obj+0x97e).

// Results of B2 01 that the GameServer decides before asking the
// DataServer (FUN_004204a0); 1..6 and 8 come from WZ_CS_ReqRegAttackGuild.
const (
	csRegNotMaster = 0
	csRegOwner     = 3
	csRegLevel     = 5
	csRegNoGuild   = 6
	csRegNotTime   = 7
	csRegMembers   = 8
	csMarkRefund   = 3000 // zen per mark returned by B2 02 (FUN_00424510)
)

// csGuildMaster is FUN_0041f370: guild master (0x80) of a guild that is
// not in an alliance or is the alliance master.
func (w *World) csGuildMaster(o *Object) bool {
	if !o.IsPlayer() || csGuildStatus(o) != 0x80 {
		return false
	}
	n := guildNumberOf(o)
	g, ok := w.guildMgr().ByNumber(n)
	if !ok {
		return false
	}
	return g.Union == n || g.Union == 0
}

// csGuildRecord returns the guild of the player (the GameServer CGuild).
func (w *World) csGuildRecord(o *Object) (guild.Record, bool) {
	n := guildNumberOf(o)
	if n == 0 {
		return guild.Record{}, false
	}
	return w.guildMgr().ByNumber(n)
}

// handleCSState is B2 00 (FUN_004203b0 / answer FUN_00423280):
// WZ_CS_GetOwnerGuildMaster and the current state and dates.
func (w *World) handleCSState(o *Object, _ []byte) {
	if !w.csEnabled() || !o.IsPlayer() {
		return
	}
	c := w.cs()
	result, owner, master := byte(0), "", ""
	switch {
	case !c.dbOK:
		result = 0
	case !c.occupied:
		result = 2
	case c.owner == "":
		result = 3
	default:
		if g, ok := w.guildMgr().Find(c.owner); ok {
			result, owner, master = 1, g.Name, g.Master
		} else {
			result = 4
		}
	}
	s := castlesiege.StateInfo{Result: result, Owner: owner, Master: master, State: 0xFF}
	if c.state >= 0 && c.state < 10 && c.running {
		s.State = byte(c.state)
		s.Start = c.stateStart
		s.End = w.now().Add(time.Duration(c.remain/1000) * time.Second)
		s.Siege = c.leftSiege
		s.Remain = c.remain / 1000
	}
	w.send(o, castlesiege.PacketState(s))
}

// handleCSRegister is B2 01 (FUN_004204a0).
func (w *World) handleCSRegister(o *Object, _ []byte) {
	if !w.csEnabled() || !o.IsPlayer() {
		return
	}
	c := w.cs()
	guildName := csGuildName(o)
	fail := func(r byte) { w.send(o, castlesiege.PacketRegister(r, guildName)) }
	switch {
	case c.state != castlesiege.StateRegSiege || w.cfg.Int("Event", "CastleSiegeEvent", 0) == 0:
		fail(csRegNotTime)
		return
	case guildName == "":
		fail(csRegNoGuild)
		return
	case !w.csGuildMaster(o):
		fail(csRegNotMaster)
		return
	case w.csOwnerGuild(o) || w.csOwnerAlliance(o):
		fail(csRegOwner)
		return
	case o.P.Char.Level < w.cfg.Int("Event", "CastleSiegeRegisterMinMasterLevel", 0):
		fail(csRegLevel)
		return
	}
	g, ok := w.csGuildRecord(o)
	if !ok || g.Count() < w.cfg.Int("Event", "CastleSiegeRegisterMinGuildMember", 0) {
		fail(csRegMembers)
		return
	}
	st := w.csStore()
	if st == nil {
		return
	}
	chk := store.CastleGuildCheck{Exists: true, MasterLevel: o.P.Char.Level, Members: g.Count()}
	group, idx := c.group, o.Index
	w.async(func(ctx context.Context) func() {
		r, err := st.RegisterSiegeGuild(ctx, group, guildName, chk)
		if err != nil {
			r = 0
		}
		return func() { w.csReply(idx, castlesiege.PacketRegister(byte(r), guildName)) } // FUN_00424180
	})
}

// csReply sends to an object index if it is still a playing player.
func (w *World) csReply(idx int, p []byte) {
	if o := w.objects[idx]; o.State == StatePlaying && o.IsPlayer() {
		w.send(o, p)
	}
}

// handleCSGiveUp is B2 02 (FUN_00420620): [4] 1 = give up. Only during
// the registration states 1..3.
func (w *World) handleCSGiveUp(o *Object, p []byte) {
	if !w.csEnabled() || !o.IsPlayer() || len(p) < 5 {
		return
	}
	c := w.cs()
	if c.state < castlesiege.StateRegSiege || c.state > castlesiege.StateRegMark {
		w.send(o, castlesiege.PacketGiveUp(3, 0, ""))
		return
	}
	guildName := csGuildName(o)
	if guildName == "" || !w.csGuildMaster(o) {
		return
	}
	st := w.csStore()
	if st == nil {
		return
	}
	giveUp := p[4] != 0
	group, idx := c.group, o.Index
	w.async(func(ctx context.Context) func() {
		r, del, err := st.GiveUpSiegeGuild(ctx, group, guildName, giveUp)
		if err != nil {
			r, del = 0, 0
		}
		return func() { w.csGiveUpDone(idx, r, del, giveUp, guildName) }
	})
}

// csGiveUpDone is FUN_00424510: 3000 zen per deleted mark.
func (w *World) csGiveUpDone(idx, result, del int, giveUp bool, guildName string) {
	o := w.objects[idx]
	if o.State != StatePlaying || !o.IsPlayer() {
		return
	}
	if result == 1 && del > 0 {
		o.P.Char.Money = min(o.P.Char.Money+int64(del)*csMarkRefund, 2000000000)
		w.sendMoney(o)
	}
	g := byte(0)
	if giveUp {
		g = 1
	}
	w.send(o, castlesiege.PacketGiveUp(byte(result), g, guildName))
}

// handleCSRegInfo is B2 03 (FUN_00420730 / FUN_00424020):
// WZ_CS_GetGuildMarkRegInfo (1 found, 2 not registered).
func (w *World) handleCSRegInfo(o *Object, _ []byte) {
	if !w.csEnabled() || !o.IsPlayer() {
		return
	}
	guildName := csGuildName(o)
	st := w.csStore()
	if guildName == "" || st == nil {
		return
	}
	group, idx := w.cs().group, o.Index
	w.async(func(ctx context.Context) func() {
		r, ok, err := st.GuildMarkRegInfo(ctx, group, guildName)
		result := byte(1)
		if err != nil || !ok {
			result, r = 2, store.CastleRegGuild{}
		}
		return func() { w.csReply(idx, castlesiege.PacketRegInfo(result, guildName, r.Marks, r.GiveUp, r.Seq)) }
	})
}

// handleCSRegMark is B2 04 (FUN_00420800): one Sign of Lord +3 per mark
// during the mark registration (state 3).
func (w *World) handleCSRegMark(o *Object, _ []byte) {
	if !w.csEnabled() || !o.IsPlayer() {
		return
	}
	c := w.cs()
	guildName := csGuildName(o)
	if guildName == "" || c.state != castlesiege.StateRegMark {
		return
	}
	if w.countItems(o, csSignOfLord, 3) == 0 {
		w.send(o, castlesiege.PacketRegMark(3, "", 0))
		return
	}
	cs := csOf(o)
	if cs.MarkReq {
		return
	}
	st := w.csStore()
	if st == nil {
		return
	}
	cs.MarkReq = true
	group, idx := c.group, o.Index
	w.async(func(ctx context.Context) func() {
		r, marks, err := st.RegisterGuildMark(ctx, group, guildName)
		if err != nil {
			r, marks = 0, 0
		}
		return func() { w.csRegMarkDone(idx, r, marks, guildName) }
	})
}

// csRegMarkDone is FUN_004243a0.
func (w *World) csRegMarkDone(idx, result, marks int, guildName string) {
	o := w.objects[idx]
	if o.State != StatePlaying || !o.IsPlayer() {
		return
	}
	csOf(o).MarkReq = false
	if result != 0 {
		w.deleteItems(o, csSignOfLord, 3, 1)
	}
	w.send(o, castlesiege.PacketRegMark(byte(result), guildName, marks))
}

// handleCSRegList is B4 (FUN_00421ce0 / FUN_00424d50): the TOP 100
// registrations in registration order.
func (w *World) handleCSRegList(o *Object, _ []byte) {
	if !w.csEnabled() || !o.IsPlayer() {
		return
	}
	st := w.csStore()
	if st == nil {
		return
	}
	group, idx := w.cs().group, o.Index
	w.async(func(ctx context.Context) func() {
		list, err := st.RegisteredSiegeGuilds(ctx, group)
		result := byte(1)
		if err != nil {
			result, list = 0, nil
		}
		return func() { w.csReply(idx, castlesiege.PacketRegList(result, list)) }
	})
}

// --- siege guild list ------------------------------------------------------

// csLoadSiegeGuildList is FUN_0041e670 (entering NOTIFY and READY): the
// stored list (DataServer 0x88) when SIEGE_GUILDLIST_SETTED, otherwise
// a new calculation (0x85).
func (w *World) csLoadSiegeGuildList() {
	c, st := w.cs(), w.csStore()
	c.listLoaded, c.unionLoaded = false, false
	c.basic, c.all = map[string]*csGuild{}, map[string]*csGuild{}
	if st == nil {
		return
	}
	group := c.group
	if c.listSet {
		w.async(func(ctx context.Context) func() {
			list, err := st.SiegeGuildList(ctx, group)
			if err != nil || len(list) == 0 {
				return nil
			}
			return func() { w.csSiegeListLoaded(list) }
		})
		return
	}
	w.async(func(ctx context.Context) func() {
		list, err := st.CalcRegGuildList(ctx, group)
		if err != nil || len(list) == 0 {
			return nil
		}
		return func() { w.csCalcSiegeList(list) }
	})
}

// csSiegeListLoaded is FUN_00425700 (DataServer 0x88 answer).
func (w *World) csSiegeListLoaded(list []store.CastleSiegeGuild) {
	c := w.cs()
	for _, g := range list {
		c.all[g.Guild] = &csGuild{Side: g.Side, Involved: g.Involved, Score: g.Score}
		if g.Involved {
			c.basic[g.Guild] = &csGuild{Side: g.Side, Involved: true, Score: g.Score}
		}
	}
	c.listLoaded, c.unionLoaded = true, true
	w.csRefreshSides()
}

// csCalcSiegeList is FUN_00425020 (DataServer 0x85 answer): the three
// rows with the highest SEQ_NUM (sort predicate FUN_004202d0 on +0x1c,
// which holds SEQ_NUM; the marks are not used) become sides 2, 3 and 4;
// the owner joins as side 1. Then the alliances are asked (0x86).
func (w *World) csCalcSiegeList(rows []store.CastleRegGuild) {
	c := w.cs()
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].Seq > rows[j].Seq })
	for i, r := range rows {
		if i >= 3 {
			break
		}
		c.basic[r.Guild] = &csGuild{Side: i + 2, Involved: true}
	}
	if c.occupied && c.owner != "" {
		c.basic[c.owner] = &csGuild{Side: 1, Involved: true}
	}
	c.listLoaded = true
	w.csAddAlliances() // FUN_00426300
}

// csAddAlliances is FUN_00426300 + FUN_00425410: every siege guild is
// copied to the full list, then the guilds of their alliances
// (WZ_CS_GetCsGuildUnionInfo) join with the same side, not involved.
// The list is saved (0x87) and the players get their sides.
func (w *World) csAddAlliances() {
	c := w.cs()
	if !c.listLoaded {
		return
	}
	mgr := w.guildMgr()
	for _, name := range csSortedNames(c.basic) {
		g := c.basic[name]
		c.all[name] = &csGuild{Side: g.Side, Involved: g.Involved, Score: g.Score}
	}
	for _, name := range csSortedNames(c.basic) {
		side := c.basic[name].Side
		rec, ok := mgr.Find(name)
		if !ok || rec.Union == 0 {
			continue
		}
		for _, member := range mgr.UnionList(rec.Union) {
			if _, ok := c.all[member.Name]; !ok {
				c.all[member.Name] = &csGuild{Side: side}
			}
		}
	}
	c.unionLoaded = true
	w.csSaveSiegeList() // FUN_004265a0 (0x87)
	w.csRefreshSides()  // FUN_0041e860
}

// csSaveSiegeList is FUN_004265a0 / answer FUN_00425650: the procedure
// returns no row, so SIEGE_GUILDLIST_SETTED stays as the answer (0).
func (w *World) csSaveSiegeList() {
	c, st := w.cs(), w.csStore()
	if st == nil {
		return
	}
	var list []store.CastleSiegeGuild
	for _, name := range csSortedNames(c.all) {
		g := c.all[name]
		list = append(list, store.CastleSiegeGuild{Guild: name, Side: g.Side, Involved: g.Involved, Score: g.Score})
	}
	group := c.group
	w.async(func(ctx context.Context) func() {
		r, err := st.SetSiegeGuildList(ctx, group, list)
		if err != nil {
			return nil
		}
		return func() {
			if c := w.cs(); c != nil {
				c.listSet = r == 1
			}
		}
	})
}

// csSortedNames is the std::map<std::string> order of the lists.
func csSortedNames(m map[string]*csGuild) []string {
	names := make([]string, 0, len(m))
	for n := range m {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// handleCSSiegeList is B5 (FUN_00421dd0): during NOTIFY and READY, the
// guilds of the siege with their sides.
func (w *World) handleCSSiegeList(o *Object, _ []byte) {
	if !w.csEnabled() || !o.IsPlayer() {
		return
	}
	c := w.cs()
	if c.state != castlesiege.StateNotify && c.state != castlesiege.StateReady {
		w.send(o, castlesiege.PacketSiegeList(2, nil))
		return
	}
	if !c.listLoaded || !c.unionLoaded {
		w.send(o, castlesiege.PacketSiegeList(3, nil))
		return
	}
	var list []castlesiege.SiegeEntry
	for _, name := range csSortedNames(c.all) {
		g := c.all[name]
		list = append(list, castlesiege.SiegeEntry{Side: g.Side, Involved: g.Involved, Guild: name})
	}
	w.send(o, castlesiege.PacketSiegeList(1, list))
}

// csRefreshSides is FUN_0041e860: every player of a listed guild gets
// its side; the side marks are refreshed.
func (w *World) csRefreshSides() {
	c := w.cs()
	if !c.unionLoaded {
		return
	}
	for i := PlayerStart; i < MaxObjects; i++ {
		o := w.objects[i]
		if o.State != StatePlaying || !o.IsPlayer() {
			continue
		}
		if g := csGuildName(o); g != "" {
			if e, ok := c.all[g]; ok {
				cs := csOf(o)
				cs.Side, cs.Involved = byte(e.Side), e.Involved
			}
		}
		w.csSideMark(o)
	}
}

// csResetPlayersSide is FUN_0041e7f0 (END).
func (w *World) csResetPlayersSide() {
	for i := PlayerStart; i < MaxObjects; i++ {
		o := w.objects[i]
		if o.State != StatePlaying || !o.IsPlayer() {
			continue
		}
		cs := csOf(o)
		cs.Side, cs.Involved, cs.Stones, cs.Crown = 0, false, 0, 0
		w.csSideMark(o)
	}
}

// csSideMark is FUN_00422e00: effects 14..17 ("Guild State 1..4") show
// the side on map 30 during START and END.
func (w *World) csSideMark(o *Object) {
	c := w.cs()
	side := csSide(o)
	if w.hooks.eventEffect == nil {
		return
	}
	if o.Map == castlesiege.Map && (c.state == castlesiege.StateStart || c.state == castlesiege.StateEnd) && side != 0 {
		if side >= 1 && side <= 4 {
			w.hooks.eventEffect(w, o, 13+int(side), true)
		}
		return
	}
	for e := 14; e <= 17; e++ {
		if hasEffectIndex(o, e) {
			w.hooks.eventEffect(w, o, e, false)
		}
	}
}
