package gamedata

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/config/jsonconf"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/config/mutext"
)

// Small data files that the original reads at start-up (loaders
// FUN_004e8520..FUN_004ec4c0, see docs/server-plan/16-cobertura-dados.md)
// and that had no Go parser yet. Most readers are the token-stream loop of
// the original tokenizer (FUN_004e0d70 reads a token, _ftol FUN_00534ff0
// converts it): a row is a fixed count of numbers, a comma ends a number
// ("12,015" is two numbers) and "*" reads as -1.

// streamRows flattens the numeric fields of rows into one token stream and
// cuts it into records of n numbers, like the original reader, which does
// not care about line breaks. A short last record repeats its last value
// (numRow), as the original keeps the previous number.
func streamRows(rows []mutext.Line, n int) [][]int {
	var f []string
	for _, l := range rows {
		f = append(f, numFields(l)...)
	}
	var out [][]int
	for i := 0; i < len(f); i += n {
		out = append(out, numRow(f, i, n))
	}
	return out
}

func loadTable(path string) ([]mutext.Line, error) {
	lines, err := jsonconf.LoadLines(path)
	if err != nil {
		return nil, err
	}
	return mutext.Table(lines), nil
}

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

// LoadDefaultClassFreebies reads Data/Character/DefaultClassFreebies.txt.
func LoadDefaultClassFreebies(dataDir string) (*ClassFreebies, error) {
	secs, err := dropSections(filepath.Join(dataDir, "Character", "DefaultClassFreebies.json"))
	if err != nil {
		return nil, err
	}
	out := &ClassFreebies{Money: map[int]int{}, Points: map[int]int{}}
	for _, s := range secs {
		switch s.Index {
		case 0:
			for _, v := range streamRows(s.Rows, 3) {
				out.Money[v[0]], out.Points[v[0]] = v[1], v[2]
			}
		case 1:
			for _, v := range streamRows(s.Rows, 11) {
				if v[0] < 0 || v[0] >= 5 {
					continue
				}
				out.Items = append(out.Items, FreebieItem{Class: v[0], Index: itemRef(v[1], v[2]), Level: v[3],
					Durability: v[4], Skill: v[5], Luck: v[6], Option: v[7], Excellent: v[8], SetOption: v[9],
					Duration: v[10]})
			}
		case 2:
			for _, v := range streamRows(s.Rows, 8) {
				out.Effects = append(out.Effects, FreebieEffect{Class: v[0], Effect: v[1], Type: v[2],
					Power: [4]int{v[3], v[4], v[5], v[6]}, Duration: v[7]})
			}
		}
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

// LoadCommandRequirement reads Data/CommandRequirement.txt.
func LoadCommandRequirement(dataDir string) ([]CommandRequirement, error) {
	rows, err := loadTable(filepath.Join(dataDir, "CommandRequirement.json"))
	if err != nil {
		return nil, err
	}
	var f []string
	for _, l := range rows {
		f = append(f, numFields(l)...)
	}
	var out []CommandRequirement
	pos, prev := 0, 0
	next := func() int {
		tok := ""
		if pos < len(f) {
			tok = f[pos]
		}
		pos++
		prev = numTok(tok, prev)
		return prev
	}
	for pos < len(f) {
		var r CommandRequirement
		r.Index, r.AccountLevel, r.MinLevel, r.MaxLevel = next(), next(), next(), next()
		r.MinReset, r.MaxReset, r.CheckItem = next(), next(), next()
		r.ReqItem = next()
		if r.ReqItem != -1 {
			r.ReqItem = itemRef(r.ReqItem, next())
		}
		r.ReqItemCount, r.ReqItemLevel, r.ReqEffect = next(), next(), next()
		r.ReqQuestIndex, r.ReqQuestState = next(), next()
		r.ReqPKLevel, r.ReqPKCount, r.ReqGuildRnk = next(), next(), next()
		r.Map, r.StartX, r.StartY, r.EndX, r.EndY = next(), next(), next(), next(), next()
		r.DayOfWeek, r.MinHour, r.MaxHour, r.FailMessage = next(), next(), next(), next()
		for i := range r.Class {
			r.Class[i] = next()
		}
		out = append(out, r)
	}
	return out, nil
}

// --- Custom/CustomEvolution.txt ------------------------------------------------

// EvolutionRow is a CustomEvolution.txt row (FUN_0044aff0): Index
// ItemTarget(cc,iii) ItemResult(cc,iii), kept in a map by Index.
type EvolutionRow struct {
	Index, Target, Result int
}

// LoadCustomEvolution reads Data/Custom/CustomEvolution.txt.
func LoadCustomEvolution(dataDir string) (map[int]EvolutionRow, error) {
	rows, err := loadTable(filepath.Join(dataDir, "Custom", "CustomEvolution.json"))
	if err != nil {
		return nil, err
	}
	out := map[int]EvolutionRow{}
	for _, v := range streamRows(rows, 5) {
		if _, ok := out[v[0]]; ok {
			continue // std::map insert keeps the first row
		}
		out[v[0]] = EvolutionRow{Index: v[0], Target: itemRef(v[1], v[2]), Result: itemRef(v[3], v[4])}
	}
	return out, nil
}

// --- Custom/CustomMobCombo.txt -------------------------------------------------

// monsterClassList is FUN_0044ef50, shared by CustomGuard.txt and
// CustomMobCombo.txt: section 0 holds up to 100 monster classes.
func monsterClassList(path string) ([]int, error) {
	secs, err := dropSections(path)
	if err != nil {
		return nil, err
	}
	var out []int
	for _, s := range secs {
		if s.Index != 0 {
			continue
		}
		for _, l := range s.Rows {
			if len(out) < 100 {
				out = append(out, numRow(numFields(l), 0, 1)[0])
			}
		}
	}
	return out, nil
}

// LoadCustomMobCombo reads Data/Custom/CustomMobCombo.txt (same reader as
// CustomGuard.txt, FUN_0044ef50 on DAT_0173d940).
func LoadCustomMobCombo(dataDir string) ([]int, error) {
	return monsterClassList(filepath.Join(dataDir, "Custom", "CustomMobCombo.json"))
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

// LoadCustomGHRS reads Data/Custom/CustomGHRS.ini.
func LoadCustomGHRS(dataDir string) (*CustomGHRS, error) {
	f, err := jsonconf.LoadINI(filepath.Join(dataDir, "Custom", "CustomGHRS.json"))
	if err != nil {
		return nil, err
	}
	g := &CustomGHRS{}
	for d := time.Sunday; d <= time.Saturday; d++ {
		g.Week[d] = f.Int("WEEK", d.String(), 0)
	}
	g.Hour = f.Int("TIMEUPDATE", "Hour", 0)
	g.Min = f.Int("TIMEUPDATE", "Min", 0)
	g.Sec = f.Int("TIMEUPDATE", "Sec", 0)
	return g, nil
}

// --- Item/ItemLevel.txt ----------------------------------------------------------

// ItemLevelName is an Item/ItemLevel.txt row (FUN_00493010): the name of
// an item at one level (ItemIndex cc,iii, ItemLevel, quoted name up to 32
// characters).
type ItemLevelName struct {
	Index, Level int
	Name         string
}

// LoadItemLevel reads Data/Item/ItemLevel.txt.
func LoadItemLevel(dataDir string) ([]ItemLevelName, error) {
	rows, err := loadTable(filepath.Join(dataDir, "Item", "ItemLevel.json"))
	if err != nil {
		return nil, err
	}
	var out []ItemLevelName
	for _, l := range rows {
		if len(l.Tokens) < 3 {
			return nil, fmt.Errorf("ItemLevel.txt line %d: %d columns", l.No, len(l.Tokens))
		}
		v := numRow(numFields(mutext.Line{Tokens: l.Tokens[:2]}), 0, 3)
		name := l.Tokens[2]
		if len(name) > 0x31 {
			name = name[:0x31] // strcpy_s into a 0x32 buffer
		}
		out = append(out, ItemLevelName{Index: itemRef(v[0], v[1]), Level: v[2], Name: name})
	}
	return out, nil
}

// --- Monster/MonsterMove.txt -----------------------------------------------------

// MonsterMoveRow is a Monster/MonsterMove.txt row (FUN_004b1110 ->
// FUN_004b12b0): Index Map X Y Direction KeepTime.
type MonsterMoveRow struct {
	Monster, Map, X, Y, Dir, KeepTime int
}

// LoadMonsterMove reads Data/Monster/MonsterMove.txt.
func LoadMonsterMove(dataDir string) ([]MonsterMoveRow, error) {
	rows, err := loadTable(filepath.Join(dataDir, "Monster", "MonsterMove.json"))
	if err != nil {
		return nil, err
	}
	var out []MonsterMoveRow
	for _, v := range streamRows(rows, 6) {
		out = append(out, MonsterMoveRow{v[0], v[1], v[2], v[3], v[4], v[5]})
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

// LoadMonsterSchedule reads Data/Monster/MonsterSchedule.txt by group.
func LoadMonsterSchedule(dataDir string) (map[int]*MonsterScheduleGroup, error) {
	secs, err := dropSections(filepath.Join(dataDir, "Monster", "MonsterSchedule.json"))
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
	for _, s := range secs {
		switch s.Index {
		case 0:
			for _, v := range streamRows(s.Rows, 8) {
				g := group(v[0])
				g.Schedule = append(g.Schedule, Schedule{v[1], v[2], v[3], v[4], v[5], v[6], v[7]})
			}
		case 1:
			for _, v := range streamRows(s.Rows, 5) {
				g := group(v[0])
				g.EnableMsg, g.DisableMsg, g.NotTimeMsg, g.NPCSec = v[1], v[2], v[3], v[4]
			}
		case 2:
			for _, v := range streamRows(s.Rows, 5) {
				g := group(v[0])
				g.Monsters = append(g.Monsters, ScheduledMonster{v[1], v[2], v[3], v[4]})
			}
		}
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

// LoadMoveSummon reads Data/Move/MoveSummon.txt.
func LoadMoveSummon(dataDir string) ([]MoveSummonRow, error) {
	rows, err := loadTable(filepath.Join(dataDir, "Move", "MoveSummon.json"))
	if err != nil {
		return nil, err
	}
	var out []MoveSummonRow
	for _, v := range streamRows(rows, 13) {
		out = append(out, MoveSummonRow{Map: v[0], StartX: v[1], StartY: v[2], EndX: v[3], EndY: v[4],
			MinLevel: v[5], MaxLevel: v[6], MinReset: v[7], MaxReset: v[8], AL: [4]int{v[9], v[10], v[11], v[12]}})
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

// LoadHackSkillSpeedCheck reads Data/Hack/HackSkillSpeedCheck.txt.
func LoadHackSkillSpeedCheck(dataDir string) ([]SkillSpeedRule, error) {
	rows, err := loadTable(filepath.Join(dataDir, "Hack", "HackSkillSpeedCheck.json"))
	if err != nil {
		return nil, err
	}
	var out []SkillSpeedRule
	for _, v := range streamRows(rows, 12) {
		out = append(out, SkillSpeedRule{v[0], v[1], v[2], v[3], v[4], v[5], v[6], v[7], v[8], v[9], v[10], v[11]})
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

// LoadExperienceTable reads Data/Util/ExperienceTable.txt.
func LoadExperienceTable(dataDir string) (ExperienceTable, error) {
	rows, err := loadTable(filepath.Join(dataDir, "Util", "ExperienceTable.json"))
	if err != nil {
		return nil, err
	}
	var out ExperienceTable
	for _, v := range streamRows(rows, 7) {
		out = append(out, ExperienceRateRow{v[0], v[1], v[2], v[3], v[4], v[5], v[6]})
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

// LoadPetDamageTable reads Data/Util/PetDamageTable.txt.
func LoadPetDamageTable(dataDir string) ([]PetDamageRow, error) {
	rows, err := loadTable(filepath.Join(dataDir, "Util", "PetDamageTable.json"))
	if err != nil {
		return nil, err
	}
	var out []PetDamageRow
	for _, v := range streamRows(rows, 6) {
		out = append(out, PetDamageRow{v[0], v[1], v[2], v[3], v[4], v[5]})
	}
	return out, nil
}

// --- BlackList.txt (GameServer) ---------------------------------------------------------

// BlackList is Data/BlackList.txt of the GameServer (FUN_00411ae0, loaded
// by FUN_004e8520): section 0 IP addresses (16 characters), section 1
// hardware ids (44 characters).
type BlackList struct {
	IPs, HWIDs []string
}

// LoadBlackList reads Data/BlackList.txt.
func LoadBlackList(dataDir string) (*BlackList, error) {
	secs, err := dropSections(filepath.Join(dataDir, "BlackList.json"))
	if err != nil {
		return nil, err
	}
	b := &BlackList{}
	for _, s := range secs {
		for _, l := range s.Rows {
			v := l.Tokens[0]
			switch s.Index {
			case 0:
				b.IPs = append(b.IPs, truncate(v, 0x10))
			case 1:
				b.HWIDs = append(b.HWIDs, truncate(v, 0x2c))
			}
		}
	}
	return b, nil
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return strings.Clone(s)
}
