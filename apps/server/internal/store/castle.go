package store

import (
	"context"
	"sort"
	"strings"
	"time"
)

// Castle Siege persistence: the MuCastle_* tables and the WZ_CS_* stored
// procedures the original DataServer ran for the CastleSiege GameServer
// (protocol 0x80..0x89, docs/server-plan/10d-castle-siege.md). Every call
// takes the MAP_SVR_GROUP.

// CastleData is a MuCastle_DATA row.
type CastleData struct {
	StartDate    time.Time // SIEGE_START_DATE (00:00 of the day)
	EndDate      time.Time // SIEGE_END_DATE
	GuildListSet bool      // SIEGE_GUILDLIST_SETTED
	SiegeEnded   bool      // SIEGE_ENDED
	Occupied     bool      // CASTLE_OCCUPY
	Owner        string    // OWNER_GUILD
	Money        int64     // MONEY
	TaxChaos     int       // TAX_RATE_CHAOS
	TaxStore     int       // TAX_RATE_STORE
	TaxHunt      int       // TAX_HUNT_ZONE
}

// CastleNPCData is a MuCastle_NPC row.
type CastleNPCData struct {
	Number, Index    int
	DFLevel, RGLevel int
	MaxHP, HP        int
	X, Y, Dir        int
}

// CastleRegGuild is a MuCastle_REG_SIEGE row.
type CastleRegGuild struct {
	Guild  string
	Marks  int
	GiveUp bool
	Seq    int
}

// CastleSiegeGuild is a MuCastle_SIEGE_GUILDLIST row (GUILD_ID holds the
// side: 1 defender, 2.. attackers).
type CastleSiegeGuild struct {
	Guild    string
	Side     int
	Involved bool
	Score    int
}

// CastleGuildCheck carries the Guild/GuildMember/Character lookups of
// WZ_CS_ReqRegAttackGuild (the guilds live in the guild store).
type CastleGuildCheck struct {
	Exists      bool
	MasterLevel int
	Members     int
}

// Results of WZ_CS_ReqRegAttackGuild.
const (
	CastleRegOK          = 1
	CastleRegAlready     = 2
	CastleRegOwner       = 3
	CastleRegNoGuild     = 4
	CastleRegMasterLevel = 5
	CastleRegFull        = 6
	CastleRegNoMembers   = 8
	castleRegMax         = 100 // @iMaxRegGuildCount
)

// CastleStore replaces the castle queries of the DataServer.
type CastleStore interface {
	// CastleTotalInfo is WZ_CS_GetCastleTotalInfo: the row is created on
	// first use (start = today, end = today + cycleDays); the bool is
	// FIRST_CREATE.
	CastleTotalInfo(ctx context.Context, group, cycleDays int) (CastleData, bool, error)
	// CastleNPCs is WZ_CS_GetCastleNpcInfo.
	CastleNPCs(ctx context.Context, group int) ([]CastleNPCData, error)
	// ReplaceCastleNPCs is "DELETE MuCastle_NPC WHERE MAP_SVR_GROUP" plus
	// one INSERT per NPC (DataServer 0x84 and 0x89).
	ReplaceCastleNPCs(ctx context.Context, group int, npcs []CastleNPCData) error
	// BuyCastleNPC is WZ_CS_ReqNpcBuy: 4 when the NPC exists, 1 inserted.
	BuyCastleNPC(ctx context.Context, group int, n CastleNPCData) (int, error)
	// RepairCastleNPC is WZ_CS_ReqNpcRepair: 1 with HP = MAXHP, 2 missing.
	RepairCastleNPC(ctx context.Context, group, number, index int) (result, hp, maxHP int, err error)
	// UpgradeCastleNPC is WZ_CS_ReqNpcUpgrade (type 1 DF, 2 RG, 3 MAXHP and
	// HP): 1 ok, 2 bad type, 0 missing.
	UpgradeCastleNPC(ctx context.Context, group, number, index, typ, value int) (int, error)
	// RemoveCastleNPC is the single-row DELETE of DataServer 0x80 0x16.
	RemoveCastleNPC(ctx context.Context, group, number, index int) error
	// CastleTaxInfo is WZ_CS_GetCastleTaxInfo (false without a row).
	CastleTaxInfo(ctx context.Context, group int) (CastleData, bool, error)
	// ModifyCastleTaxRate is WZ_CS_ModifyTaxRate (kind 1 chaos, 2 store,
	// 3 hunt zone): result 1 and the stored rate, otherwise 0.
	ModifyCastleTaxRate(ctx context.Context, group, kind, rate int) (result, stored int, err error)
	// ModifyCastleMoney is WZ_CS_ModifyMoney: 2 when the total would go
	// below 0 (unchanged), 1 applied; both log MuCastle_MONEY_STATISTICS.
	// 0 without a row.
	ModifyCastleMoney(ctx context.Context, group int, delta int64) (result int, money int64, err error)
	// SetCastleSchedule is WZ_CS_ModifyCastleSchedule.
	SetCastleSchedule(ctx context.Context, group int, start, end time.Time) error
	// SetCastleSiegeEnded is WZ_CS_ModifySiegeEnd.
	SetCastleSiegeEnded(ctx context.Context, group int, ended bool) error
	// SetCastleOwner is WZ_CS_ModifyCastleOwnerInfo.
	SetCastleOwner(ctx context.Context, group int, occupied bool, owner string) error
	// ResetCastleTaxInfo is WZ_CS_ResetCastleTaxInfo (money and the three
	// rates to 0): 1 ok, 0 without a row.
	ResetCastleTaxInfo(ctx context.Context, group int) (int, error)
	// ResetCastleSiege is WZ_CS_ResetCastleSiege (SIEGE_GUILDLIST_SETTED,
	// SIEGE_ENDED = 0 and MuCastle_SIEGE_GUILDLIST emptied).
	ResetCastleSiege(ctx context.Context, group int) (int, error)
	// ResetCastleRegSiege is WZ_CS_ResetRegSiegeInfo.
	ResetCastleRegSiege(ctx context.Context, group int) error
	// ResetCastleSiegeGuilds is WZ_CS_ResetSiegeGuildInfo.
	ResetCastleSiegeGuilds(ctx context.Context, group int) error
	// RegisterSiegeGuild is WZ_CS_ReqRegAttackGuild.
	RegisterSiegeGuild(ctx context.Context, group int, guild string, chk CastleGuildCheck) (int, error)
	// RegisterGuildMark is WZ_CS_ReqRegGuildMark: 1 and the new count, or
	// 0 when not registered or given up.
	RegisterGuildMark(ctx context.Context, group int, guild string) (result, marks int, err error)
	// GiveUpSiegeGuild is WZ_CS_ModifyGuildGiveUp: 1 and the deleted marks
	// (the marks are zeroed), 2 when not registered.
	GiveUpSiegeGuild(ctx context.Context, group int, guild string, giveUp bool) (result, delMarks int, err error)
	// GuildMarkRegInfo is WZ_CS_GetGuildMarkRegInfo.
	GuildMarkRegInfo(ctx context.Context, group int, guild string) (CastleRegGuild, bool, error)
	// RegisteredSiegeGuilds is "SELECT TOP 100 * FROM MuCastle_REG_SIEGE
	// ... ORDER BY SEQ_NUM ASC" (DataServer 0x83).
	RegisteredSiegeGuilds(ctx context.Context, group int) ([]CastleRegGuild, error)
	// CalcRegGuildList is the cursor of WZ_CS_GetCalcRegGuildList: the
	// TOP 100 not given up rows ORDER BY SEQ_NUM DESC (the caller adds the
	// member count and the master level).
	CalcRegGuildList(ctx context.Context, group int) ([]CastleRegGuild, error)
	// SetSiegeGuildList is DataServer 0x87: DELETE and one
	// WZ_CS_SetSiegeGuildInfo per guild. The procedure returns no row, so
	// the reply result is always 0.
	SetSiegeGuildList(ctx context.Context, group int, list []CastleSiegeGuild) (int, error)
	// SiegeGuildList is WZ_CS_GetSiegeGuildInfo.
	SiegeGuildList(ctx context.Context, group int) ([]CastleSiegeGuild, error)
	// CheckSiegeGuildList is WZ_CS_CheckSiegeGuildList: true when the guild
	// takes part in an unfinished siege (siege list, or registered and not
	// given up).
	CheckSiegeGuildList(ctx context.Context, guild string) (bool, error)
}

// castleDate truncates to the day, like the 'yyyy-mm-dd 00:00:00' strings
// of the procedures.
func castleDate(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}

// castleNow is GetDate() for the memory store (tests replace it).
var castleNow = time.Now

type memCastle struct {
	data  CastleData
	npcs  []CastleNPCData
	reg   []CastleRegGuild
	siege []CastleSiegeGuild
	stats []castleMoneyLog
}

type castleMoneyLog struct {
	At    time.Time
	Delta int64
}

func (m *Memory) castle(group int) *memCastle {
	if m.castles == nil {
		m.castles = map[int]*memCastle{}
	}
	return m.castles[group]
}

func (m *Memory) CastleTotalInfo(_ context.Context, group, cycleDays int) (CastleData, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c := m.castleRow(group)
	first := false
	if !c.hasData() {
		now := castleNow()
		c.data = CastleData{StartDate: castleDate(now), EndDate: castleDate(now.AddDate(0, 0, cycleDays))}
		first = true
	}
	return c.data, first, nil
}

func (m *Memory) CastleNPCs(_ context.Context, group int) ([]CastleNPCData, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if c := m.castle(group); c != nil {
		return append([]CastleNPCData(nil), c.npcs...), nil
	}
	return nil, nil
}

func (m *Memory) ReplaceCastleNPCs(_ context.Context, group int, npcs []CastleNPCData) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	c := m.castleRow(group)
	c.npcs = append([]CastleNPCData(nil), npcs...)
	return nil
}

// castleRow returns the group's state even before MuCastle_DATA exists
// (the NPC, registration and siege list tables do not depend on it).
func (m *Memory) castleRow(group int) *memCastle {
	c := m.castle(group)
	if c == nil {
		c = &memCastle{}
		m.castles[group] = c
	}
	return c
}

func (c *memCastle) npc(number, index int) *CastleNPCData {
	for i := range c.npcs {
		if c.npcs[i].Number == number && c.npcs[i].Index == index {
			return &c.npcs[i]
		}
	}
	return nil
}

func (m *Memory) BuyCastleNPC(_ context.Context, group int, n CastleNPCData) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c := m.castleRow(group)
	if c.npc(n.Number, n.Index) != nil {
		return 4, nil
	}
	c.npcs = append(c.npcs, n)
	return 1, nil
}

func (m *Memory) RepairCastleNPC(_ context.Context, group, number, index int) (int, int, int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := m.castleRow(group).npc(number, index)
	if n == nil {
		return 2, 0, 0, nil
	}
	n.HP = n.MaxHP
	return 1, n.HP, n.MaxHP, nil
}

func (m *Memory) UpgradeCastleNPC(_ context.Context, group, number, index, typ, value int) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := m.castleRow(group).npc(number, index)
	if n == nil {
		return 0, nil
	}
	switch typ {
	case 1:
		n.DFLevel = value
	case 2:
		n.RGLevel = value
	case 3:
		n.MaxHP, n.HP = value, value
	default:
		return 2, nil
	}
	return 1, nil
}

func (m *Memory) RemoveCastleNPC(_ context.Context, group, number, index int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	c := m.castleRow(group)
	for i := range c.npcs {
		if c.npcs[i].Number == number && c.npcs[i].Index == index {
			c.npcs = append(c.npcs[:i], c.npcs[i+1:]...)
			break
		}
	}
	return nil
}

func (m *Memory) CastleTaxInfo(_ context.Context, group int) (CastleData, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c := m.castle(group)
	if c == nil || c.data.StartDate.IsZero() {
		return CastleData{}, false, nil
	}
	return c.data, true, nil
}

// hasData is the IF EXISTS (MuCastle_DATA) of the procedures.
func (c *memCastle) hasData() bool { return c != nil && !c.data.StartDate.IsZero() }

func (m *Memory) ModifyCastleTaxRate(_ context.Context, group, kind, rate int) (int, int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c := m.castle(group)
	if !c.hasData() {
		return 0, 0, nil
	}
	switch kind {
	case 1:
		c.data.TaxChaos = rate
	case 2:
		c.data.TaxStore = rate
	case 3:
		c.data.TaxHunt = rate
	default:
		return 0, 0, nil
	}
	return 1, rate, nil
}

func (m *Memory) ModifyCastleMoney(_ context.Context, group int, delta int64) (int, int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c := m.castle(group)
	if !c.hasData() {
		return 0, 0, nil
	}
	result := 1
	if c.data.Money+delta < 0 {
		result = 2
	} else {
		c.data.Money += delta
	}
	c.stats = append(c.stats, castleMoneyLog{castleNow(), delta})
	return result, c.data.Money, nil
}

func (m *Memory) SetCastleSchedule(_ context.Context, group int, start, end time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if c := m.castle(group); c.hasData() {
		c.data.StartDate, c.data.EndDate = castleDate(start), castleDate(end)
	}
	return nil
}

func (m *Memory) SetCastleSiegeEnded(_ context.Context, group int, ended bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if c := m.castle(group); c.hasData() {
		c.data.SiegeEnded = ended
	}
	return nil
}

func (m *Memory) SetCastleOwner(_ context.Context, group int, occupied bool, owner string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if c := m.castle(group); c.hasData() {
		c.data.Occupied, c.data.Owner = occupied, owner
	}
	return nil
}

func (m *Memory) ResetCastleTaxInfo(_ context.Context, group int) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c := m.castle(group)
	if !c.hasData() {
		return 0, nil
	}
	c.data.Money, c.data.TaxChaos, c.data.TaxStore, c.data.TaxHunt = 0, 0, 0, 0
	return 1, nil
}

func (m *Memory) ResetCastleSiege(_ context.Context, group int) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c := m.castle(group)
	if !c.hasData() {
		return 0, nil
	}
	c.data.GuildListSet, c.data.SiegeEnded = false, false
	c.siege = nil
	return 1, nil
}

func (m *Memory) ResetCastleRegSiege(_ context.Context, group int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if c := m.castle(group); c.hasData() {
		c.reg = nil
	}
	return nil
}

func (m *Memory) ResetCastleSiegeGuilds(_ context.Context, group int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if c := m.castle(group); c.hasData() {
		c.siege = nil
	}
	return nil
}

func (c *memCastle) regIndex(guild string) int {
	for i, r := range c.reg {
		if strings.EqualFold(r.Guild, guild) {
			return i
		}
	}
	return -1
}

func (m *Memory) RegisterSiegeGuild(_ context.Context, group int, guild string, chk CastleGuildCheck) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c := m.castleRow(group)
	switch {
	case len(c.reg) >= castleRegMax:
		return CastleRegFull, nil
	case c.regIndex(guild) >= 0:
		return CastleRegAlready, nil
	case c.hasData() && strings.EqualFold(c.data.Owner, guild):
		return CastleRegOwner, nil
	case !chk.Exists:
		return CastleRegNoGuild, nil
	case chk.MasterLevel < 1:
		return CastleRegMasterLevel, nil
	case chk.Members < 1:
		return CastleRegNoMembers, nil
	}
	seq := 1
	for _, r := range c.reg {
		seq = max(seq, r.Seq+1)
	}
	c.reg = append(c.reg, CastleRegGuild{Guild: guild, Seq: seq})
	return CastleRegOK, nil
}

func (m *Memory) RegisterGuildMark(_ context.Context, group int, guild string) (int, int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c := m.castleRow(group)
	i := c.regIndex(guild)
	if i < 0 || c.reg[i].GiveUp {
		return 0, 0, nil
	}
	c.reg[i].Marks++
	return 1, c.reg[i].Marks, nil
}

func (m *Memory) GiveUpSiegeGuild(_ context.Context, group int, guild string, giveUp bool) (int, int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c := m.castleRow(group)
	i := c.regIndex(guild)
	if i < 0 {
		return 2, 0, nil
	}
	del := c.reg[i].Marks
	c.reg[i].GiveUp, c.reg[i].Marks = giveUp, 0
	return 1, del, nil
}

func (m *Memory) GuildMarkRegInfo(_ context.Context, group int, guild string) (CastleRegGuild, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c := m.castleRow(group)
	if i := c.regIndex(guild); i >= 0 {
		return c.reg[i], true, nil
	}
	return CastleRegGuild{}, false, nil
}

func (m *Memory) sortedReg(group int, desc, skipGiveUp bool) []CastleRegGuild {
	var out []CastleRegGuild
	for _, r := range m.castleRow(group).reg {
		if !skipGiveUp || !r.GiveUp {
			out = append(out, r)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if desc {
			return out[i].Seq > out[j].Seq
		}
		return out[i].Seq < out[j].Seq
	})
	if len(out) > castleRegMax {
		out = out[:castleRegMax]
	}
	return out
}

func (m *Memory) RegisteredSiegeGuilds(_ context.Context, group int) ([]CastleRegGuild, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.sortedReg(group, false, false), nil
}

func (m *Memory) CalcRegGuildList(_ context.Context, group int) ([]CastleRegGuild, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.sortedReg(group, true, true), nil
}

func (m *Memory) SetSiegeGuildList(_ context.Context, group int, list []CastleSiegeGuild) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.castleRow(group).siege = append([]CastleSiegeGuild(nil), list...)
	return 0, nil
}

func (m *Memory) SiegeGuildList(_ context.Context, group int) ([]CastleSiegeGuild, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]CastleSiegeGuild(nil), m.castleRow(group).siege...), nil
}

func (m *Memory) CheckSiegeGuildList(_ context.Context, guild string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	// SELECT @iEnd = SIEGE_ENDED FROM MuCastle_DATA (last row read wins).
	ended := false
	for _, c := range m.castles {
		if c.hasData() {
			ended = c.data.SiegeEnded
		}
	}
	if ended {
		return false, nil
	}
	for _, c := range m.castles {
		for _, s := range c.siege {
			if strings.EqualFold(s.Guild, guild) {
				return true, nil
			}
		}
		for _, r := range c.reg {
			if strings.EqualFold(r.Guild, guild) && !r.GiveUp {
				return true, nil
			}
		}
	}
	return false, nil
}
