package game

import (
	"context"
	"strings"
	"sync"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/store"
)

// The DataServer half of the guild system (CGuildManager of DataServer.exe,
// 0x4158a0..0x417a70, reached by the GameServer with C1 A0 xx). The
// original DataServer kept every guild in memory, wrote through to SQL and
// broadcast the results to every GameServer (FUN_00415760); here one
// guildManager per Hub plays that role for every instance of the process
// and store.GuildStore is the SQL. docs/server-plan/09b-guild.md.

const (
	guildMaxMembers = 80 // 0x50 member slots, slot 0 is the master
	guildUnionMax   = 5  // FUN_00416d70: union list (master included) < 5
)

// guildMember is one DataServer member slot (0x14 bytes: name[11],
// +0xc server code (-1 offline), +0x10 G_Status).
type guildMember struct {
	Name   string
	Status int
	Server int
}

// guildRecord is the DataServer guild (0x6c4 bytes): +0 Number, +4 Score,
// +8 Name[9], +0x11 Master[11], +0x1c Notice[60], +0x58 Mark[32], +0x78
// members, +0x6b8 Type, +0x6bc Union, +0x6c0 Rival.
type guildRecord struct {
	Number  int
	Score   int
	Name    string
	Master  string
	Notice  string
	Mark    [32]byte
	Members [guildMaxMembers]guildMember
	Type    int
	Union   int
	Rival   int
}

// count is FUN_0041a440 / FUN_00413ed0: used member slots.
func (g *guildRecord) count() int {
	n := 0
	for _, m := range g.Members {
		if m.Name != "" {
			n++
		}
	}
	return n
}

// member returns the slot of name (exact match like strcmp), or -1.
func (g *guildRecord) member(name string) int {
	for i, m := range g.Members {
		if m.Name != "" && m.Name == name {
			return i
		}
	}
	return -1
}

// statusCount is FUN_004863f0 (0x20) / FUN_00486450 (0x40).
func (g *guildRecord) statusCount(status int) int {
	n := 0
	for _, m := range g.Members {
		if m.Name != "" && m.Status == status {
			n++
		}
	}
	return n
}

type guildManager struct {
	mu     sync.Mutex
	st     store.GuildStore
	loaded bool
	byNum  map[int]*guildRecord
	byName map[string]*guildRecord // lower(name)
	owner  map[string]*guildRecord // lower(member name)
}

func newGuildManager() *guildManager {
	return &guildManager{byNum: map[int]*guildRecord{}, byName: map[string]*guildRecord{}, owner: map[string]*guildRecord{}}
}

// noHubGuilds is the guild manager shared by Worlds built without a hub.
var noHubGuilds = newGuildManager()

// guildMgr returns the process-wide guild manager of the hub.
func (w *World) guildMgr() *guildManager {
	if w.hub == nil {
		return noHubGuilds
	}
	return w.hub.guilds
}

// load is the DataServer start-up (FUN_00415b90): every guild and member in
// memory, members offline (server -1).
func (m *guildManager) load(ctx context.Context, st store.GuildStore) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.loaded {
		return nil
	}
	m.st = st
	m.loaded = true
	if st == nil {
		return nil
	}
	gs, err := st.LoadGuilds(ctx)
	if err != nil {
		m.loaded = false
		return err
	}
	for _, g := range gs {
		r := &guildRecord{Number: g.Number, Score: g.Score, Name: g.Name, Master: g.Master, Notice: g.Notice,
			Type: g.Type, Union: g.Union, Rival: g.Rival}
		copy(r.Mark[:], g.Mark)
		for i := range r.Members {
			r.Members[i] = guildMember{Status: -1, Server: -1}
		}
		r.Members[0] = guildMember{Name: g.Master, Status: store.GuildMasterStatus, Server: -1}
		slot := 1
		for _, mb := range g.Members {
			if strings.EqualFold(mb.Name, g.Master) {
				r.Members[0].Status = mb.Status
				continue
			}
			if slot < guildMaxMembers {
				r.Members[slot] = guildMember{Name: mb.Name, Status: mb.Status, Server: -1}
				slot++
			}
		}
		m.add(r)
	}
	return nil
}

func (m *guildManager) add(r *guildRecord) {
	m.byNum[r.Number] = r
	m.byName[strings.ToLower(r.Name)] = r
	for _, mb := range r.Members {
		if mb.Name != "" {
			m.owner[strings.ToLower(mb.Name)] = r
		}
	}
}

// --- reads (any goroutine) -----------------------------------------------------

// byNumber returns a copy of a guild.
func (m *guildManager) byNumber(n int) (guildRecord, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if r := m.byNum[n]; r != nil && n != 0 {
		return *r, true
	}
	return guildRecord{}, false
}

// find returns a copy of a guild by name (FUN_00487350, strcmp).
func (m *guildManager) find(name string) (guildRecord, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if r := m.byName[strings.ToLower(name)]; r != nil && r.Name == name {
		return *r, true
	}
	return guildRecord{}, false
}

// of returns the guild of a member and the slot.
func (m *guildManager) of(name string) (guildRecord, int, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if r := m.owner[strings.ToLower(name)]; r != nil {
		if s := r.member(name); s >= 0 {
			return *r, s, true
		}
	}
	return guildRecord{}, -1, false
}

// relation is FUN_00486c20 -> FUN_005091a0: the union key of the viewer's
// guild is its G_Union, or its own number; 1 when target is in that union
// list (FUN_00417370), 2 when it is in its rival list (FUN_00417470).
func (m *guildManager) relation(target, viewer int) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.relationLocked(target, viewer)
}

func (m *guildManager) relationLocked(target, viewer int) int {
	v, t := m.byNum[viewer], m.byNum[target]
	if v == nil || t == nil {
		return 0
	}
	key := v.Union
	if key == 0 {
		key = v.Number
	}
	kg := m.byNum[key]
	if kg == nil {
		return 0
	}
	if kg.Union != 0 && t.Union == key {
		return 1
	}
	if kg.Rival != 0 && (t.Number == kg.Rival || t.Union == kg.Rival) {
		return 2
	}
	return 0
}

// unionList is FUN_00417370: the guild and every guild whose G_Union is its
// number (empty when its own G_Union is 0).
func (m *guildManager) unionList(n int) []guildRecord {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []guildRecord
	for _, n := range m.unionNumbers(n) {
		out = append(out, *m.byNum[n])
	}
	return out
}

func (m *guildManager) unionNumbers(n int) []int {
	g := m.byNum[n]
	if g == nil || g.Union == 0 {
		return nil
	}
	out := []int{n}
	for _, x := range m.sorted() {
		if x.Number != n && x.Union == n {
			out = append(out, x.Number)
		}
	}
	return out
}

// rivalNumbers is FUN_00417470: the rival and every guild of its union.
func (m *guildManager) rivalNumbers(n int) []int {
	g := m.byNum[n]
	if g == nil || g.Rival == 0 {
		return nil
	}
	out := []int{g.Rival}
	for _, x := range m.sorted() {
		if x.Number != n && x.Union == g.Rival && x.Number != g.Rival {
			out = append(out, x.Number)
		}
	}
	return out
}

func (m *guildManager) sorted() []*guildRecord {
	out := make([]*guildRecord, 0, len(m.byNum))
	for _, g := range m.byNum {
		out = append(out, g)
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j].Number < out[j-1].Number; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

// unionName is FUN_00486b70: the name of the union master guild.
func (m *guildManager) unionName(union int) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if g := m.byNum[union]; g != nil && union != 0 {
		return g.Name
	}
	return ""
}

// --- writes (DataServer requests, called from w.async) -----------------------

// create is DataServer A0 00 (FUN_00413f40 -> FUN_004166b0): 3 the name
// exists, 2 WZ_GuildCreate failed, 0 database error, 1 created.
func (m *guildManager) create(ctx context.Context, name, master string, mark [32]byte, typ, server int) (byte, int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.byName[strings.ToLower(name)] != nil {
		return 3, 0
	}
	number := 0
	if m.st != nil {
		n, code, err := m.st.GuildCreate(ctx, name, master, mark[:], store.GuildMasterStatus)
		if err != nil {
			return 0, 0
		}
		if code != 0 {
			return 2, 0
		}
		// The type only lives in memory: FUN_004166b0 writes G_Mark and
		// G_Status, not G_Type.
		number = n
	} else {
		if m.owner[strings.ToLower(master)] != nil {
			return 2, 0
		}
		for n := range m.byNum {
			number = max(number, n)
		}
		number++
	}
	r := &guildRecord{Number: number, Name: name, Master: master, Mark: mark, Type: typ}
	for i := range r.Members {
		r.Members[i] = guildMember{Status: -1, Server: -1}
	}
	r.Members[0] = guildMember{Name: master, Status: store.GuildMasterStatus, Server: server}
	m.add(r)
	return 1, number
}

// guildChange describes the union/rival side effects of a write, for the
// game servers (A0 0B / A0 0C broadcasts).
type guildChange struct {
	dissolved int // union master guild whose union was dissolved (A0 0C)
}

// remove is A0 01 (FUN_004140d0 -> FUN_004169c0): 3 not found, 1 deleted.
// The union and rival links are broken first (FUN_00417010 for both).
func (m *guildManager) remove(ctx context.Context, name string) (byte, []guildChange) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r := m.byName[strings.ToLower(name)]
	if r == nil || r.Name != name {
		return 3, nil
	}
	var ch []guildChange
	if c, ok := m.breakLocked(ctx, 1, r.Number); ok {
		ch = append(ch, c)
	}
	if c, ok := m.breakLocked(ctx, 2, r.Number); ok {
		ch = append(ch, c)
	}
	if m.st != nil {
		if err := m.st.GuildDelete(ctx, r.Name); err != nil {
			return 0, nil
		}
	}
	delete(m.byNum, r.Number)
	delete(m.byName, strings.ToLower(r.Name))
	for _, mb := range r.Members {
		if mb.Name != "" {
			delete(m.owner, strings.ToLower(mb.Name))
		}
	}
	return 1, ch
}

// addMember is A0 02 (FUN_004141f0 -> FUN_00416af0): 0 no guild, 3 already
// in it, 4 already has a GuildMember row, 2 full, 1 added (status 0).
func (m *guildManager) addMember(ctx context.Context, guild, name string, server int) byte {
	m.mu.Lock()
	defer m.mu.Unlock()
	r := m.byName[strings.ToLower(guild)]
	if r == nil {
		return 0
	}
	if r.member(name) >= 0 {
		return 3
	}
	if m.owner[strings.ToLower(name)] != nil {
		return 4
	}
	slot := -1
	for i := 1; i < guildMaxMembers; i++ {
		if r.Members[i].Name == "" {
			slot = i
			break
		}
	}
	if slot < 0 {
		return 2
	}
	if m.st != nil {
		ok, err := m.st.GuildMemberAdd(ctx, r.Name, name, store.GuildMemberStatus)
		if err != nil {
			return 0
		}
		if !ok {
			return 4
		}
	}
	r.Members[slot] = guildMember{Name: name, Status: 0, Server: server}
	m.owner[strings.ToLower(name)] = r
	return 1
}

// delMember is A0 03 (FUN_00414320 -> FUN_00416cd0): 3 not a member, 1 ok.
func (m *guildManager) delMember(ctx context.Context, guild, name string) byte {
	m.mu.Lock()
	defer m.mu.Unlock()
	r := m.byName[strings.ToLower(guild)]
	if r == nil {
		return 3
	}
	s := r.member(name)
	if s < 0 {
		return 3
	}
	if m.st != nil {
		if err := m.st.GuildMemberDelete(ctx, name); err != nil {
			return 0
		}
	}
	r.Members[s] = guildMember{Status: -1, Server: -1}
	delete(m.owner, strings.ToLower(name))
	return 1
}

// setStatus is A0 04 (FUN_00414440 -> FUN_004179a0): 1 ok, 0 failed.
func (m *guildManager) setStatus(ctx context.Context, guild, name string, status int) byte {
	m.mu.Lock()
	defer m.mu.Unlock()
	r := m.byName[strings.ToLower(guild)]
	if r == nil {
		return 0
	}
	s := r.member(name)
	if s < 0 {
		return 0
	}
	if m.st != nil {
		if err := m.st.SetGuildMemberStatus(ctx, name, status); err != nil {
			return 0
		}
	}
	r.Members[s].Status = status
	return 1
}

// update runs one of the G_Type / G_Notice / G_Score writes (A0 05
// FUN_004178c0, A0 06 FUN_00417790, A0 08 FUN_004176b0): 1 ok, 0 failed.
func (m *guildManager) update(ctx context.Context, guild string, write func(store.GuildStore) error, apply func(*guildRecord)) byte {
	m.mu.Lock()
	defer m.mu.Unlock()
	r := m.byName[strings.ToLower(guild)]
	if r == nil {
		return 0
	}
	if m.st != nil {
		if err := write(m.st); err != nil {
			return 0
		}
	}
	apply(r)
	return 1
}

// setServer is the online-map part of DataServer 0x70 / 0x71 (FUN_00415620):
// the member's server code (-1 offline). ok is false for non-members.
func (m *guildManager) setServer(name string, server int) (guildRecord, int, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r := m.owner[strings.ToLower(name)]
	if r == nil {
		return guildRecord{}, -1, false
	}
	s := r.member(name)
	if s < 0 {
		return guildRecord{}, -1, false
	}
	r.Members[s].Server = server
	return *r, s, true
}

// join is A0 0A (FUN_00414b60 -> FUN_00416d70): requester joins the union
// of answerer (rel 1) or both become rivals (rel 2). 1 done, 0 refused.
func (m *guildManager) join(ctx context.Context, rel, requester, answerer int) byte {
	m.mu.Lock()
	defer m.mu.Unlock()
	a, b := m.byNum[requester], m.byNum[answerer]
	if a == nil || b == nil {
		return 0
	}
	switch rel {
	case 1:
		if a.Union != 0 || a.Rival != 0 || len(m.unionNumbers(b.Number)) >= guildUnionMax {
			return 0
		}
		if m.st != nil {
			if err := m.st.SetGuildUnion(ctx, b.Number, a.Number, b.Number); err != nil {
				return 0
			}
		}
		a.Union, b.Union = b.Number, b.Number
		return 1
	case 2:
		if a.Rival != 0 || b.Rival != 0 || (a.Union != 0 && a.Union != a.Number) ||
			(b.Union != 0 && b.Union != b.Number) {
			return 0
		}
		if m.st != nil {
			if err := m.st.SetGuildRival(ctx, b.Number, a.Number); err != nil {
				return 0
			}
			if err := m.st.SetGuildRival(ctx, a.Number, b.Number); err != nil {
				return 0
			}
		}
		a.Rival, b.Rival = b.Number, a.Number
		return 1
	}
	return 0
}

// breakOff is A0 0B (FUN_00414e00 -> FUN_00417010): guild n leaves its
// union (rel 1; a union master cannot) or ends its rivalry (rel 2).
func (m *guildManager) breakOff(ctx context.Context, rel, n int) (byte, guildChange) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.breakLocked(ctx, rel, n)
	if !ok {
		return 0, c
	}
	return 1, c
}

func (m *guildManager) breakLocked(ctx context.Context, rel, n int) (guildChange, bool) {
	g := m.byNum[n]
	if g == nil {
		return guildChange{}, false
	}
	switch rel {
	case 1:
		if g.Number == g.Union {
			return guildChange{}, false
		}
		master := m.byNum[g.Union]
		if master == nil {
			return guildChange{}, false
		}
		if len(m.unionNumbers(master.Number)) == 2 {
			// The last member leaves: the union is dissolved (A0 0C).
			if m.st != nil {
				if err := m.st.SetGuildUnion(ctx, 0, g.Number, master.Number); err != nil {
					return guildChange{}, false
				}
			}
			g.Union, master.Union = 0, 0
			return guildChange{dissolved: master.Number}, true
		}
		if m.st != nil {
			if err := m.st.SetGuildUnion(ctx, 0, g.Number); err != nil {
				return guildChange{}, false
			}
		}
		g.Union = 0
		return guildChange{}, true
	case 2:
		r := m.byNum[g.Rival]
		if r == nil {
			return guildChange{}, false
		}
		if m.st != nil {
			if err := m.st.SetGuildRival(ctx, 0, g.Number, r.Number); err != nil {
				return guildChange{}, false
			}
		}
		g.Rival, r.Rival = 0, 0
		return guildChange{}, true
	}
	return guildChange{}, false
}

// unionKick is A0 0C (FUN_00414f90 -> FUN_00417200): the union master guild
// removes target from its union.
func (m *guildManager) unionKick(ctx context.Context, master, target string) (byte, guildChange) {
	m.mu.Lock()
	defer m.mu.Unlock()
	mg, tg := m.byName[strings.ToLower(master)], m.byName[strings.ToLower(target)]
	if mg == nil || tg == nil || tg.Name != target || tg.Union != mg.Number || mg.Union != mg.Number || tg == mg {
		return 0, guildChange{}
	}
	c, ok := m.breakLocked(ctx, 1, tg.Number)
	if !ok {
		return 0, c
	}
	return 1, c
}
