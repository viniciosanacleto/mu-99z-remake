package gamedata

import (
	"fmt"
)

// Event data (Event/*.dat) shared by the event managers.

// Schedule is a start time row: Year Month Day DayOfWeek Hour Minute
// Second, "*" = -1 (CScheduleManager, FUN_004e53c0).
type Schedule struct {
	Year, Month, Day, DayOfWeek, Hour, Minute, Second int
}

// DevilSquare is Event/DevilSquare.dat (CDevilSquare::Load, FUN_0046deb0):
// section 0 the state times in minutes (WarningTime/ready, StandTime,
// EventTime, CloseTime), 1 the start schedule, 2 and 3 the experience and
// zen rewards per square and rank (10 ranks).
type DevilSquare struct {
	ReadyTime, StandTime, EventTime, CloseTime int
	Schedule                                   []Schedule
	Experience, Money                          [6][10]int
}

// LoadDevilSquare reads the DevilSquare table of config/data/event.toml;
// reward rows outside squares 0..5 are skipped.
func LoadDevilSquare(configDir string) (*DevilSquare, error) {
	f, err := LoadEventConfig(configDir)
	if err != nil {
		return nil, err
	}
	t := f.DevilSquare
	d := &DevilSquare{}
	for _, r := range t.Time {
		d.ReadyTime, d.StandTime, d.EventTime, d.CloseTime = r.WarningTime, r.NotifyTime, r.EventTime, r.CloseTime
	}
	for _, r := range t.Schedule {
		d.Schedule = append(d.Schedule, r.Schedule())
	}
	for i, rows := range [2][]DevilRewardRow{t.Experience, t.Money} {
		for _, r := range rows {
			if r.EventIndex < 0 || r.EventIndex >= 6 {
				continue
			}
			if i == 0 {
				d.Experience[r.EventIndex] = r.Reward
			} else {
				d.Money[r.EventIndex] = r.Reward
			}
		}
	}
	return d, nil
}

// EntryLevel is one row of Event/EventEntryLevel.dat: Level MinLevel
// MaxLevel MinLevelMG MaxLevelMG ("*" = -1).
type EntryLevel struct {
	Level, Min, Max, MinMG, MaxMG int
}

// EventEntryLevel holds the rows per section (0 Blood Castle, 1 Devil
// Square, 2 Chaos Castle, 3 Kalima, ...).
type EventEntryLevel map[int][]EntryLevel

// LoadEventEntryLevel reads the EventEntryLevel table of
// config/data/event.toml (CEventEntryLevel, FUN_0047aec0).
func LoadEventEntryLevel(configDir string) (EventEntryLevel, error) {
	f, err := LoadEventConfig(configDir)
	if err != nil {
		return nil, err
	}
	out := EventEntryLevel{}
	for _, s := range f.EventEntryLevel.Sections {
		for _, r := range s.Rows {
			out[s.Section] = append(out[s.Section], EntryLevel{r.Index, r.CommonMinLevel, r.CommonMaxLevel,
				r.SpecialMinLevel, r.SpecialMaxLevel})
		}
	}
	return out, nil
}

// Level is FUN_0047b370 for one section: the last row whose range holds
// the character level (classes MG, DL and 6 use the MG columns), -1 when
// none does.
func (e EventEntryLevel) Level(section, class, level int) int {
	mg := class == 3 || class == 4 || class == 6
	out := -1
	for i, r := range e[section] {
		lo, hi := r.Min, r.Max
		if mg {
			lo, hi = r.MinMG, r.MaxMG
		}
		if (lo == -1 || lo <= level) && (hi == -1 || level <= hi) {
			out = i
		}
	}
	return out
}

// StageSpawn is a row of Event/EventStageSpawn.dat: the event (section:
// 0 Blood Castle, 1 Chaos Castle, 2 Devil Square), its level, the stage
// and the monster class spawned from the map's section-4 spawn records
// with MaxRegenTime (ms, -1 = "*").
type StageSpawn struct {
	Event, Level, Stage, Monster, RegenTime int
}

// LoadEventStageSpawn reads the EventStageSpawn table of
// config/data/event.toml (CEventSpawnStage).
func LoadEventStageSpawn(configDir string) ([]StageSpawn, error) {
	f, err := LoadEventConfig(configDir)
	if err != nil {
		return nil, err
	}
	var out []StageSpawn
	for _, s := range f.EventStageSpawn.Sections {
		for _, r := range s.Rows {
			out = append(out, StageSpawn{s.Section, r.Level, r.Stage, r.MonsterIndex, r.MaxRegenTime})
		}
	}
	return out, nil
}

// BloodCastle is Event/BloodCastle.dat (CBloodCastle::Load, FUN_00413900):
// section 0 the state times in minutes, 1 the schedule, then per castle
// (0..6) the experience (2) and zen (3) for the winning party and the
// others, the castle gate and statue life (4) and the statue position (5).
type BloodCastle struct {
	ReadyTime, StandTime, EventTime, CloseTime int
	Schedule                                   []Schedule
	Experience, Money                          [7][2]int
	GateLife, StatueLife                       [7]int
	StatueX, StatueY                           [7]int
}

// LoadBloodCastle reads the BloodCastle table of config/data/event.toml;
// rows outside castles 0..6 are skipped.
func LoadBloodCastle(configDir string) (*BloodCastle, error) {
	f, err := LoadEventConfig(configDir)
	if err != nil {
		return nil, err
	}
	t := f.BloodCastle
	d := &BloodCastle{}
	for _, r := range t.Time {
		d.ReadyTime, d.StandTime, d.EventTime, d.CloseTime = r.WarningTime, r.NotifyTime, r.EventTime, r.CloseTime
	}
	for _, r := range t.Schedule {
		d.Schedule = append(d.Schedule, r.Schedule())
	}
	for sec, rows := range [4][]EventIndexPairRow{t.Experience, t.Money, t.Life, t.Statue} {
		for _, r := range rows {
			i := r.EventIndex
			if i < 0 || i >= 7 {
				continue
			}
			switch sec {
			case 0:
				d.Experience[i] = [2]int{r.Value1, r.Value2}
			case 1:
				d.Money[i] = [2]int{r.Value1, r.Value2}
			case 2:
				d.GateLife[i], d.StatueLife[i] = r.Value1, r.Value2
			case 3:
				d.StatueX[i], d.StatueY[i] = r.Value1, r.Value2
			}
		}
	}
	return d, nil
}

// Invasion is one event of Event/InvasionManager.dat (CInvasionManager,
// FUN_00488f40): section 0 the schedule per event, 1 the event settings,
// 2 the map groups (one row of each group is drawn at start) and 3 the
// spawn rows.
type Invasion struct {
	Index                  int
	Schedule               []Schedule
	RespawnMsg, DespawnMsg int // -1 = none
	Boss, BossKilledMsg    int
	Time                   int // seconds
	EffectType             int
	Name                   string
	Groups                 map[int][]InvasionGroup
	Spawns                 []InvasionSpawn
}

// InvasionGroup is a section 2 row: Group MapNumber Stage.
type InvasionGroup struct{ Group, Map, Stage int }

// InvasionSpawn is a section 3 row: Group MonsterIndex SpawnRadius
// MapNumber RangeStartX RangeStartY RangeEndX RangeEndY MaxSpawnCount
// Stage RegenType RegenTime(ms) and the kill-message flag.
type InvasionSpawn struct {
	Group, Monster, Radius, Map    int
	X1, Y1, X2, Y2                 int
	Count, Stage, RegenType, Regen int
	KillMessage                    int
}

// LoadInvasions reads the InvasionManager table of config/data/event.toml
// (20 events at most; rows of other event indexes are ignored).
func LoadInvasions(configDir string) ([]*Invasion, error) {
	f, err := LoadEventConfig(configDir)
	if err != nil {
		return nil, err
	}
	out := make([]*Invasion, 20)
	get := func(i int) *Invasion {
		if i < 0 || i >= len(out) {
			return nil
		}
		if out[i] == nil {
			out[i] = &Invasion{Index: i, RespawnMsg: -1, DespawnMsg: -1, BossKilledMsg: -1, Boss: -1,
				Groups: map[int][]InvasionGroup{}}
		}
		return out[i]
	}
	t := f.InvasionManager
	for _, r := range t.Schedule {
		if inv := get(r.EventIndex); inv != nil {
			inv.Schedule = append(inv.Schedule, r.Schedule())
		}
	}
	for _, r := range t.Settings {
		if inv := get(r.EventIndex); inv != nil {
			inv.RespawnMsg, inv.DespawnMsg, inv.Boss, inv.BossKilledMsg, inv.Time, inv.EffectType =
				r.RespawnMessage, r.DespawnMessage, r.MonsterBossIndex, r.BossKilledMessage, r.InvasionTime, r.EffectType
			if r.EventName != "" {
				inv.Name = r.EventName
			}
		}
	}
	for _, r := range t.Group {
		if inv := get(r.EventIndex); inv != nil {
			inv.Groups[r.Group] = append(inv.Groups[r.Group], InvasionGroup{r.Group, r.MapNumber, r.Stage})
		}
	}
	for _, r := range t.Spawn {
		if inv := get(r.EventIndex); inv != nil {
			inv.Spawns = append(inv.Spawns, InvasionSpawn{r.Group, r.MonsterIndex, r.SpawnRadius, r.MapNumber,
				r.RangeStartX, r.RangeStartY, r.RangeEndX, r.RangeEndY, r.MaxSpawnCount, r.Stage, r.RegenType,
				r.RegenTime, r.KillMessage})
		}
	}
	return out, nil
}

// TimedEvent is one entry of BonusManager.dat / DropEvent.dat: schedule
// (section 0), StartMessage FinalMessage Duration(s) Name (section 1).
type TimedEvent struct {
	Index                    int
	Schedule                 []Schedule
	StartMsg, FinalMsg, Time int // messages -1 = none
	Name                     string
}

// Bonus is a BonusManager.dat section 2 row: BonusIndex (0 experience,
// 1 item drop, 2 chaos mix, 3 excellent drop, 4 set drop), the value per
// AccountLevel and the map (-1 = any).
type Bonus struct {
	Event, Index int
	Value        [4]int
	Map          int
}

// buildTimedEvents fills 20 event slots from the schedule (section 0) and
// settings (section 1) rows; other event indexes are ignored.
func buildTimedEvents(schedule []EventScheduleRow, settings []TimedEventSettingsRow) []*TimedEvent {
	out := make([]*TimedEvent, 20)
	get := func(i int) *TimedEvent {
		if i < 0 || i >= len(out) {
			return nil
		}
		if out[i] == nil {
			out[i] = &TimedEvent{Index: i, StartMsg: -1, FinalMsg: -1}
		}
		return out[i]
	}
	for _, r := range schedule {
		if ev := get(r.EventIndex); ev != nil {
			ev.Schedule = append(ev.Schedule, r.Schedule())
		}
	}
	for _, r := range settings {
		if ev := get(r.EventIndex); ev != nil {
			ev.StartMsg, ev.FinalMsg, ev.Time = r.StartMessage, r.FinalMessage, r.EventTime
			if r.EventName != "" {
				ev.Name = r.EventName
			}
		}
	}
	return out
}

// LoadBonusManager reads the BonusManager table of config/data/event.toml
// (CBonusManager).
func LoadBonusManager(configDir string) ([]*TimedEvent, []Bonus, error) {
	f, err := LoadEventConfig(configDir)
	if err != nil {
		return nil, nil, err
	}
	t := f.BonusManager
	var bonuses []Bonus
	for _, r := range t.Bonus {
		bonuses = append(bonuses, Bonus{r.EventIndex, r.BonusIndex, r.BonusValue, r.BonusMapNumber})
	}
	return buildTimedEvents(t.Schedule, t.Settings), bonuses, nil
}

// ChaosCastle is Event/ChaosCastle.dat (CChaosCastle::Load, FUN_00430330):
// section 0 the state times in minutes (WarningTime = ready, NotifyTime =
// stand, EventTime, CloseTime; this+0x2bd0..0x2bdc), 1 the schedule
// (this+0x2be0), 2 per castle (0..5) the experience per user killed and
// per monster killed (this+0x2bf0 + castle*8).
type ChaosCastle struct {
	ReadyTime, StandTime, EventTime, CloseTime int
	Schedule                                   []Schedule
	Experience                                 [6][2]int
}

// LoadChaosCastle reads the ChaosCastle table of config/data/event.toml.
// Experience rows outside 0..5 are skipped ("Invalid experience level" in
// the original).
func LoadChaosCastle(configDir string) (*ChaosCastle, error) {
	f, err := LoadEventConfig(configDir)
	if err != nil {
		return nil, err
	}
	t := f.ChaosCastle
	d := &ChaosCastle{}
	for _, r := range t.Time {
		d.ReadyTime, d.StandTime, d.EventTime, d.CloseTime = r.WarningTime, r.NotifyTime, r.EventTime, r.CloseTime
	}
	for _, r := range t.Schedule {
		d.Schedule = append(d.Schedule, r.Schedule())
	}
	for _, r := range t.Experience {
		if r.EventIndex < 0 || r.EventIndex > 5 {
			continue
		}
		d.Experience[r.EventIndex] = [2]int{r.Value1, r.Value2}
	}
	return d, nil
}

// DropEventItem is a DropEvent.dat section 2 row: EventIndex ItemIndex
// ItemLevel Durability Option0..5 Duration MapNumber MonsterIndex
// MonsterMinLevel MonsterMaxLevel AccountLevel ItemDropRate ("*" = -1).
type DropEventItem struct {
	Event, Index, Level, Dur int
	Option                   [6]int
	Duration                 int
	Map, Monster             int
	MinLevel, MaxLevel       int
	AccountLevel, Rate       int
}

// LoadDropEvent reads the DropEvent table of config/data/event.toml
// (CDropEvent).
func LoadDropEvent(configDir string) ([]*TimedEvent, []DropEventItem, error) {
	f, err := LoadEventConfig(configDir)
	if err != nil {
		return nil, nil, err
	}
	t := f.DropEvent
	var items []DropEventItem
	for i, r := range t.Item {
		cat, idx, err := SplitItemRef(r.ItemIndex)
		if err != nil {
			return nil, nil, fmt.Errorf("DropEvent.Item row %d: %w", i+1, err)
		}
		items = append(items, DropEventItem{Event: r.EventIndex, Index: cat*512 + idx, Level: r.ItemLevel,
			Dur: r.Durability, Option: r.Option, Duration: r.Duration, Map: r.MapNumber, Monster: r.MonsterIndex,
			MinLevel: r.MonsterMinLevel, MaxLevel: r.MonsterMaxLevel, AccountLevel: r.AccountLevel, Rate: r.ItemDropRate})
	}
	return buildTimedEvents(t.Schedule, t.Settings), items, nil
}

// GoldenArcherReward is an Event/GoldenArcher.dat row: ItemIndex
// RewardGroup Option0..5 (ItemOptionRate rows) Duration DropRate (weight).
type GoldenArcherReward struct {
	Index, Group int
	Option       [6]int
	Duration     int
	Rate         int
}

// LoadGoldenArcher reads the GoldenArcher table of config/data/event.toml
// (CGoldenArcher, FUN_00461660).
func LoadGoldenArcher(configDir string) ([]GoldenArcherReward, error) {
	f, err := LoadEventConfig(configDir)
	if err != nil {
		return nil, err
	}
	var out []GoldenArcherReward
	for i, r := range f.GoldenArcher.Rows {
		cat, idx, err := SplitItemRef(r.ItemIndex)
		if err != nil {
			return nil, fmt.Errorf("GoldenArcher row %d: %w", i+1, err)
		}
		out = append(out, GoldenArcherReward{Index: cat*512 + idx, Group: r.RewardGroup, Option: r.Option,
			Duration: r.Duration, Rate: r.DropRate})
	}
	return out, nil
}
