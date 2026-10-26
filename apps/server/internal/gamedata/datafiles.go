package gamedata

import (
	"fmt"
	"strings"
)

// Small data files that the original reads at start-up (loaders
// FUN_004e8520..FUN_004ec4c0, see docs/server-plan/16-cobertura-dados.md)
// and that had no Go parser yet. Most readers are the token-stream loop of
// the original tokenizer (FUN_004e0d70 reads a token, _ftol FUN_00534ff0
// converts it): a row is a fixed count of numbers, a comma ends a number
// ("12,015" is two numbers) and "*" reads as -1.

// itemRef joins a (category, index) pair read as two numbers; out of range
// gives -1 (the "-1 < v && v < 0x2000" check of the readers).
func itemRef(cat, idx int) int {
	v := cat*512 + idx
	if v < 0 || v >= 0x2000 {
		return -1
	}
	return v
}

// --- Character/DefaultClassFreebies.txt ---------------------------------------

// FreebieItem is a section 1 row: an item given to a new character.
type FreebieItem struct {
	Class, Index, Level, Durability int
	Skill, Luck, Option, Excellent  int
	SetOption, Duration             int
}

// FreebieEffect is a section 2 row: an effect given to a new character.
type FreebieEffect struct {
	Class, Effect, Type int
	Power               [4]int
	Duration            int
}

// ClassFreebies is Character/DefaultClassFreebies.txt (FUN_0046cb40, loaded
// by FUN_004e85f0): section 0 Class InitialMoney InitialPoint, section 1
// Class ItemIndex(cc,iii) Level Durability Skill Luck Option Excellent
// SetOption Duration (rows with Class outside 0..4 are dropped), section 2
// Class EffectIndex EffectType Power1..4 Duration.
type ClassFreebies struct {
	Money, Points map[int]int
	Items         []FreebieItem
	Effects       []FreebieEffect
}

// LoadDefaultClassFreebies reads the DefaultClassFreebies table of
// config/data/character.toml; item rows of a class outside 0..4 are dropped.
func LoadDefaultClassFreebies(configDir string) (*ClassFreebies, error) {
	f, err := LoadCharacterConfig(configDir)
	if err != nil {
		return nil, err
	}
	t := f.DefaultClassFreebies
	out := &ClassFreebies{Money: map[int]int{}, Points: map[int]int{}}
	for _, r := range t.Class {
		out.Money[r.Class], out.Points[r.Class] = r.InitialMoney, r.InitialPoint
	}
	for i, r := range t.Item {
		if r.Class < 0 || r.Class >= 5 {
			continue
		}
		cat, idx, err := SplitItemRef(r.ItemIndex)
		if err != nil {
			return nil, fmt.Errorf("DefaultClassFreebies.Item row %d: %w", i+1, err)
		}
		out.Items = append(out.Items, FreebieItem{Class: r.Class, Index: itemRef(cat, idx), Level: r.ItemLevel,
			Durability: r.Durability, Skill: r.SkillSwitch, Luck: r.LuckSwitch, Option: r.Aditional,
			Excellent: r.Excellent, SetOption: r.SetOption, Duration: r.Duration})
	}
	for _, r := range t.Effect {
		out.Effects = append(out.Effects, FreebieEffect{Class: r.Class, Effect: r.EffectIndex, Type: r.EffectType,
			Power: r.Power, Duration: r.Duration})
	}
	return out, nil
}

// --- CommandRequirement.txt ------------------------------------------------------

// CommandRequirement is a CommandRequirement.txt row (FUN_004402a0, 0x78
// bytes). ReqItem is -1 when the column is "*" (then no item index number
// follows).
type CommandRequirement struct {
	Index, AccountLevel                 int
	MinLevel, MaxLevel                  int
	MinReset, MaxReset                  int
	CheckItem, ReqItem                  int
	ReqItemCount, ReqItemLevel          int
	ReqEffect                           int
	ReqQuestIndex, ReqQuestState        int
	ReqPKLevel, ReqPKCount, ReqGuildRnk int
	Map, StartX, StartY, EndX, EndY     int
	DayOfWeek, MinHour, MaxHour         int
	FailMessage                         int
	Class                               [5]int
}

// LoadCommandRequirement reads the CommandRequirement table of
// config/data/common.toml.
func LoadCommandRequirement(configDir string) ([]CommandRequirement, error) {
	f, err := LoadCommonConfig(configDir)
	if err != nil {
		return nil, err
	}
	var out []CommandRequirement
	for i, r := range f.CommandRequirement.Rows {
		req := CommandRequirement{Index: r.Index, AccountLevel: r.AccountLevel, MinLevel: r.MinLevel, MaxLevel: r.MaxLevel,
			MinReset: r.MinReset, MaxReset: r.MaxReset, CheckItem: r.CheckItem, ReqItem: -1, ReqItemCount: r.ReqItemCount,
			ReqItemLevel: r.ReqItemLevel, ReqEffect: r.ReqEffectState, ReqQuestIndex: r.ReqQuestIndex,
			ReqQuestState: r.ReqQuestState, ReqPKLevel: r.ReqPKLevel, ReqPKCount: r.ReqPKCount, ReqGuildRnk: r.ReqGuildRank,
			Map: r.MapIndex, StartX: r.MapStartX, StartY: r.MapStartY, EndX: r.MapEndX, EndY: r.MapEndY,
			DayOfWeek: r.DayOfWeek, MinHour: r.MinHour, MaxHour: r.MaxHour, FailMessage: r.FailMessage,
			Class: [5]int{r.DW, r.DK, r.FE, r.MG, r.DL}}
		if r.ReqItemIndex != "*" {
			cat, idx, err := SplitItemRef(r.ReqItemIndex)
			if err != nil {
				return nil, fmt.Errorf("CommandRequirement row %d: %w", i+1, err)
			}
			req.ReqItem = itemRef(cat, idx)
		}
		out = append(out, req)
	}
	return out, nil
}

// --- Custom/CustomEvolution.txt ------------------------------------------------

// EvolutionRow is a CustomEvolution.txt row (FUN_0044aff0): Index
// ItemTarget(cc,iii) ItemResult(cc,iii), kept in a map by Index.
type EvolutionRow struct {
	Index, Target, Result int
}

// LoadCustomEvolution reads the CustomEvolution table of
// config/data/custom.toml (the first row of an Index wins).
func LoadCustomEvolution(configDir string) (map[int]EvolutionRow, error) {
	f, err := LoadCustomConfig(configDir)
	if err != nil {
		return nil, err
	}
	out := map[int]EvolutionRow{}
	for i, r := range f.CustomEvolution.Rows {
		if _, ok := out[r.Index]; ok {
			continue // std::map insert keeps the first row
		}
		var ref [2]int
		for k, s := range [2]string{r.ItemTarget, r.ItemResult} {
			cat, idx, err := SplitItemRef(s)
			if err != nil {
				return nil, fmt.Errorf("CustomEvolution row %d: %w", i+1, err)
			}
			ref[k] = itemRef(cat, idx)
		}
		out[r.Index] = EvolutionRow{Index: r.Index, Target: ref[0], Result: ref[1]}
	}
	return out, nil
}

// --- Custom/CustomMobCombo.txt -------------------------------------------------

// monsterClassList is FUN_0044ef50, shared by CustomGuard.txt and
// CustomMobCombo.txt: up to 100 monster classes.
func monsterClassList(f MonsterClassListFile) []int {
	out := f.MonsterClass
	if len(out) > 100 {
		out = out[:100]
	}
	return append([]int(nil), out...)
}

// LoadCustomMobCombo reads the CustomMobCombo table of
// config/data/custom.toml (same reader as CustomGuard.txt, FUN_0044ef50 on
// DAT_0173d940).
func LoadCustomMobCombo(configDir string) ([]int, error) {
	f, err := LoadCustomConfig(configDir)
	if err != nil {
		return nil, err
	}
	return monsterClassList(f.CustomMobCombo), nil
}

// --- Custom/CustomGHRS.ini (CastleSiege.exe only) ------------------------------

// CustomGHRS is Custom/CustomGHRS.ini, read only by CastleSiege.exe
// (FUN_00442c10 there, GetPrivateProfileIntA default 0): [WEEK] Monday ..
// Sunday and [TIMEUPDATE] Hour, Min, Sec. Week is indexed by time.Weekday
// (the original keeps Sunday first, at 0x8eb6bc, then Monday..Saturday at
// 0x8eb6c0..0x8eb6d4).
type CustomGHRS struct {
	Week           [7]int
	Hour, Min, Sec int
}

// LoadCustomGHRS reads the CustomGHRS table of config/data/custom.toml.
func LoadCustomGHRS(configDir string) (*CustomGHRS, error) {
	f, err := LoadCustomConfig(configDir)
	if err != nil {
		return nil, err
	}
	w, t := f.CustomGHRS.WEEK, f.CustomGHRS.TIMEUPDATE
	return &CustomGHRS{Week: [7]int{w.Sunday, w.Monday, w.Tuesday, w.Wednesday, w.Thursday, w.Friday, w.Saturday},
		Hour: t.Hour, Min: t.Min, Sec: t.Sec}, nil
}

// --- Item/ItemLevel.txt ----------------------------------------------------------

// ItemLevelName is an Item/ItemLevel.txt row (FUN_00493010): the name of
// an item at one level (ItemIndex cc,iii, ItemLevel, quoted name up to 32
// characters).
type ItemLevelName struct {
	Index, Level int
	Name         string
}

// LoadItemLevel reads the ItemLevel table of config/data/item.toml.
func LoadItemLevel(configDir string) ([]ItemLevelName, error) {
	f, err := loadItemConfig(configDir)
	if err != nil {
		return nil, err
	}
	var out []ItemLevelName
	for i, r := range f.ItemLevel.Rows {
		cat, idx, err := SplitItemRef(r.ItemIndex)
		if err != nil {
			return nil, fmt.Errorf("ItemLevel row %d: %w", i+1, err)
		}
		name := r.Name
		if len(name) > 0x31 {
			name = name[:0x31] // strcpy_s into a 0x32 buffer
		}
		out = append(out, ItemLevelName{Index: itemRef(cat, idx), Level: r.ItemLevel, Name: name})
	}
	return out, nil
}

// --- Monster/MonsterMove.txt -----------------------------------------------------

// MonsterMoveRow is a Monster/MonsterMove.txt row (FUN_004b1110 ->
// FUN_004b12b0): Index Map X Y Direction KeepTime.
type MonsterMoveRow struct {
	Monster, Map, X, Y, Dir, KeepTime int
}

// LoadMonsterMove reads the MonsterMove table of config/data/monster.toml.
func LoadMonsterMove(configDir string) ([]MonsterMoveRow, error) {
	f, err := LoadMonsterConfig(configDir)
	if err != nil {
		return nil, err
	}
	var out []MonsterMoveRow
	for _, r := range f.MonsterMove.Rows {
		out = append(out, MonsterMoveRow{r.Index, r.MapNumber, r.MapPosX, r.MapPosY, r.Direction, r.KeepTime})
	}
	return out, nil
}

// --- Monster/MonsterSchedule.txt -------------------------------------------------

// ScheduledMonster is a section 2 row: MonsterIndex Map X Y.
type ScheduledMonster struct {
	Monster, Map, X, Y int
}

// MonsterScheduleGroup is one group of Monster/MonsterSchedule.txt
// (FUN_004bb0c0, 0x44 bytes per group): section 0 start times, section 1
// EnableMessage DisableMessage ItsNotTimeMessage EnableNpcTime (seconds),
// section 2 the monsters.
type MonsterScheduleGroup struct {
	Schedule                                  []Schedule
	EnableMsg, DisableMsg, NotTimeMsg, NPCSec int
	Monsters                                  []ScheduledMonster
}

// LoadMonsterSchedule reads the MonsterSchedule table of
// config/data/monster.toml by group.
func LoadMonsterSchedule(configDir string) (map[int]*MonsterScheduleGroup, error) {
	f, err := LoadMonsterConfig(configDir)
	if err != nil {
		return nil, err
	}
	out := map[int]*MonsterScheduleGroup{}
	group := func(i int) *MonsterScheduleGroup {
		if out[i] == nil {
			out[i] = &MonsterScheduleGroup{}
		}
		return out[i]
	}
	t := f.MonsterSchedule
	for _, r := range t.Schedule {
		g := group(r.Group)
		g.Schedule = append(g.Schedule, Schedule{r.StartYear, r.StartMonth, r.StartDay, r.StartDoW, r.StartHour,
			r.StartMin, r.StartSec})
	}
	for _, r := range t.Message {
		g := group(r.Group)
		g.EnableMsg, g.DisableMsg, g.NotTimeMsg, g.NPCSec = r.EnableMessage, r.DisableMessage, r.ItsNotTimeMessage,
			r.EnableNpcTime
	}
	for _, r := range t.Monster {
		g := group(r.Group)
		g.Monsters = append(g.Monsters, ScheduledMonster{r.MonsterIndex, r.MapNumber, r.LocationX, r.LocationY})
	}
	return out, nil
}

// --- Move/MoveSummon.txt -----------------------------------------------------------

// MoveSummonRow is a Move/MoveSummon.txt row (FUN_004bfd00 ->
// FUN_004c0130): MapNumber RangeStartX RangeStartY RangeEndX RangeEndY
// MinLevel MaxLevel MinReset MaxReset AL0..AL3.
type MoveSummonRow struct {
	Map                        int
	StartX, StartY, EndX, EndY int
	MinLevel, MaxLevel         int
	MinReset, MaxReset         int
	AL                         [4]int
}

// LoadMoveSummon reads the MoveSummon table of config/data/move.toml.
func LoadMoveSummon(configDir string) ([]MoveSummonRow, error) {
	f, err := LoadMoveConfig(configDir)
	if err != nil {
		return nil, err
	}
	var out []MoveSummonRow
	for _, r := range f.MoveSummon.Rows {
		out = append(out, MoveSummonRow{Map: r.MapNumber, StartX: r.RangeStartX, StartY: r.RangeStartY, EndX: r.RangeEndX,
			EndY: r.RangeEndY, MinLevel: r.MinLevel, MaxLevel: r.MaxLevel, MinReset: r.MinReset, MaxReset: r.MaxReset,
			AL: [4]int{r.AL0, r.AL1, r.AL2, r.AL3}})
	}
	return out, nil
}

// --- Hack/HackSkillSpeedCheck.txt ----------------------------------------------------

// SkillSpeedRule is a Hack/HackSkillSpeedCheck.txt row (FUN_00488340 ->
// FUN_004887a0): Index Class MinLevel MaxLevel MinReset MaxReset MinSpeed
// MaxSpeed MaxDelay(ms) MinCount MaxCount Action(0..2).
type SkillSpeedRule struct {
	Skill, Class       int
	MinLevel, MaxLevel int
	MinReset, MaxReset int
	MinSpeed, MaxSpeed int
	MaxDelay           int
	MinCount, MaxCount int
	Action             int
}

// LoadHackSkillSpeedCheck reads the HackSkillSpeedCheck table of
// config/data/hack.toml.
func LoadHackSkillSpeedCheck(configDir string) ([]SkillSpeedRule, error) {
	f, err := LoadHackConfig(configDir)
	if err != nil {
		return nil, err
	}
	var out []SkillSpeedRule
	for _, r := range f.HackSkillSpeedCheck.Rows {
		out = append(out, SkillSpeedRule{r.Index, r.Class, r.MinLevel, r.MaxLevel, r.MinReset, r.MaxReset, r.MinSpeed,
			r.MaxSpeed, r.MaxDelay, r.MinCount, r.MaxCount, r.Action})
	}
	return out, nil
}

// --- Util/ExperienceTable.txt ------------------------------------------------------

// ExperienceRateRow is an Util/ExperienceTable.txt record (FUN_0047be80,
// 0x24 bytes: +0 MinLevel, +4 MaxLevel, +0x10 MinReset, +0x14 MaxReset,
// +0x18 MinMasterReset, +0x1c MaxMasterReset, +0x20 rate). The reader takes
// seven numbers per record although the file header documents four rate
// columns (ExperienceRate_AL0..AL3): only one rate is read, and further
// columns would start the next record.
type ExperienceRateRow struct {
	MinLevel, MaxLevel             int
	MinReset, MaxReset             int
	MinMasterReset, MaxMasterReset int
	Rate                           int
}

// ExperienceTable is the list of ExperienceRateRow.
type ExperienceTable []ExperienceRateRow

// LoadExperienceTable reads the ExperienceTable table of
// config/data/util.toml.
func LoadExperienceTable(configDir string) (ExperienceTable, error) {
	f, err := LoadUtilConfig(configDir)
	if err != nil {
		return nil, err
	}
	var out ExperienceTable
	for _, r := range f.ExperienceTable.Rows {
		out = append(out, ExperienceRateRow{r.MinLevel, r.MaxLevel, r.MinReset, r.MaxReset, r.MinMasterReset,
			r.MaxMasterReset, r.ExperienceRate})
	}
	return out, nil
}

func inRange(lo, hi, v int) bool { return (lo == -1 || lo <= v) && (hi == -1 || v <= hi) }

// Rate is FUN_0047c040: the rate of the first record matching level,
// resets and master resets (-1 = any), 100 when none matches.
func (t ExperienceTable) Rate(level, resets, masterResets int) int {
	for _, r := range t {
		if inRange(r.MinLevel, r.MaxLevel, level) && inRange(r.MinReset, r.MaxReset, resets) &&
			inRange(r.MinMasterReset, r.MaxMasterReset, masterResets) {
			return r.Rate
		}
	}
	return 100
}

// --- Util/PetDamageTable.txt ---------------------------------------------------------

// PetDamageRow is an Util/PetDamageTable.txt row (FUN_004d61d0 ->
// FUN_00418b20): Index MinLevel MaxLevel MinReset MaxReset DamageRate.
type PetDamageRow struct {
	Pet                int
	MinLevel, MaxLevel int
	MinReset, MaxReset int
	DamageRate         int
}

// LoadPetDamageTable reads the PetDamageTable table of
// config/data/util.toml.
func LoadPetDamageTable(configDir string) ([]PetDamageRow, error) {
	f, err := LoadUtilConfig(configDir)
	if err != nil {
		return nil, err
	}
	var out []PetDamageRow
	for _, r := range f.PetDamageTable.Rows {
		out = append(out, PetDamageRow{r.Index, r.MinLevel, r.MaxLevel, r.MinReset, r.MaxReset, r.DamageRate})
	}
	return out, nil
}

// --- BlackList.txt (GameServer) ---------------------------------------------------------

// BlackList is BlackList.txt of the GameServer (FUN_00411ae0, loaded
// by FUN_004e8520): section 0 IP addresses (16 characters), section 1
// hardware ids (44 characters).
type BlackList struct {
	IPs, HWIDs []string
}

// LoadBlackList reads the BlackList table of config/data/common.toml.
func LoadBlackList(configDir string) (*BlackList, error) {
	f, err := LoadCommonConfig(configDir)
	if err != nil {
		return nil, err
	}
	b := &BlackList{}
	for _, v := range f.BlackList.IpAddress {
		b.IPs = append(b.IPs, truncate(v, 0x10))
	}
	for _, v := range f.BlackList.HardwareId {
		b.HWIDs = append(b.HWIDs, truncate(v, 0x2c))
	}
	return b, nil
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return strings.Clone(s)
}
