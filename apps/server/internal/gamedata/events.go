package gamedata

import (
	"fmt"
	"path/filepath"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/config/mutext"
)

// Event data files (Data/Event/*.dat) shared by the event managers.

// Schedule is a start time row: Year Month Day DayOfWeek Hour Minute
// Second, "*" = -1 (CScheduleManager, FUN_004e53c0).
type Schedule struct {
	Year, Month, Day, DayOfWeek, Hour, Minute, Second int
}

func scheduleRow(l mutext.Line) (Schedule, error) {
	var v [7]int
	for i := range v {
		n, err := l.Int(i)
		if err != nil {
			return Schedule{}, err
		}
		v[i] = n
	}
	return Schedule{v[0], v[1], v[2], v[3], v[4], v[5], v[6]}, nil
}

func loadSections(path string) ([]mutext.Section, error) {
	lines, err := mutext.LoadFile(path)
	if err != nil {
		return nil, err
	}
	return mutext.Sections(lines)
}

func rowInts(l mutext.Line, n int) ([]int, error) {
	if len(l.Tokens) < n {
		return nil, fmt.Errorf("line %d: %d columns, want %d", l.No, len(l.Tokens), n)
	}
	v := make([]int, n)
	for i := range v {
		x, err := l.Int(i)
		if err != nil {
			return nil, err
		}
		v[i] = x
	}
	return v, nil
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

// LoadDevilSquare reads Data/Event/DevilSquare.dat.
func LoadDevilSquare(dataDir string) (*DevilSquare, error) {
	secs, err := loadSections(filepath.Join(dataDir, "Event", "DevilSquare.dat"))
	if err != nil {
		return nil, err
	}
	d := &DevilSquare{}
	for _, s := range secs {
		for _, l := range s.Rows {
			switch s.Index {
			case 0:
				v, err := rowInts(l, 4)
				if err != nil {
					return nil, err
				}
				d.ReadyTime, d.StandTime, d.EventTime, d.CloseTime = v[0], v[1], v[2], v[3]
			case 1:
				sc, err := scheduleRow(l)
				if err != nil {
					return nil, err
				}
				d.Schedule = append(d.Schedule, sc)
			case 2, 3:
				v, err := rowInts(l, 11)
				if err != nil {
					return nil, err
				}
				if v[0] < 0 || v[0] >= 6 {
					continue
				}
				dst := &d.Experience[v[0]]
				if s.Index == 3 {
					dst = &d.Money[v[0]]
				}
				copy(dst[:], v[1:])
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

// LoadEventEntryLevel reads Data/Event/EventEntryLevel.dat (CEventEntryLevel,
// FUN_0047aec0).
func LoadEventEntryLevel(dataDir string) (EventEntryLevel, error) {
	secs, err := loadSections(filepath.Join(dataDir, "Event", "EventEntryLevel.dat"))
	if err != nil {
		return nil, err
	}
	out := EventEntryLevel{}
	for _, s := range secs {
		for _, l := range s.Rows {
			v, err := rowInts(l, 5)
			if err != nil {
				return nil, err
			}
			out[s.Index] = append(out[s.Index], EntryLevel{v[0], v[1], v[2], v[3], v[4]})
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

// LoadEventStageSpawn reads Data/Event/EventStageSpawn.dat (CEventSpawnStage).
func LoadEventStageSpawn(dataDir string) ([]StageSpawn, error) {
	secs, err := loadSections(filepath.Join(dataDir, "Event", "EventStageSpawn.dat"))
	if err != nil {
		return nil, err
	}
	var out []StageSpawn
	for _, s := range secs {
		for _, l := range s.Rows {
			v, err := rowInts(l, 4)
			if err != nil {
				return nil, err
			}
			out = append(out, StageSpawn{s.Index, v[0], v[1], v[2], v[3]})
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

// LoadBloodCastle reads Data/Event/BloodCastle.dat.
func LoadBloodCastle(dataDir string) (*BloodCastle, error) {
	secs, err := loadSections(filepath.Join(dataDir, "Event", "BloodCastle.dat"))
	if err != nil {
		return nil, err
	}
	d := &BloodCastle{}
	for _, s := range secs {
		for _, l := range s.Rows {
			switch s.Index {
			case 0:
				v, err := rowInts(l, 4)
				if err != nil {
					return nil, err
				}
				d.ReadyTime, d.StandTime, d.EventTime, d.CloseTime = v[0], v[1], v[2], v[3]
			case 1:
				sc, err := scheduleRow(l)
				if err != nil {
					return nil, err
				}
				d.Schedule = append(d.Schedule, sc)
			case 2, 3, 4, 5:
				v, err := rowInts(l, 3)
				if err != nil {
					return nil, err
				}
				i := v[0]
				if i < 0 || i >= 7 {
					continue
				}
				switch s.Index {
				case 2:
					d.Experience[i] = [2]int{v[1], v[2]}
				case 3:
					d.Money[i] = [2]int{v[1], v[2]}
				case 4:
					d.GateLife[i], d.StatueLife[i] = v[1], v[2]
				case 5:
					d.StatueX[i], d.StatueY[i] = v[1], v[2]
				}
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

// LoadInvasions reads Data/Event/InvasionManager.dat (20 events at most).
func LoadInvasions(dataDir string) ([]*Invasion, error) {
	secs, err := loadSections(filepath.Join(dataDir, "Event", "InvasionManager.dat"))
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
	for _, s := range secs {
		for _, l := range s.Rows {
			idx, err := l.Int(0)
			if err != nil {
				return nil, err
			}
			inv := get(idx)
			if inv == nil {
				continue
			}
			rest := mutext.Line{No: l.No, Tokens: l.Tokens[1:]}
			switch s.Index {
			case 0:
				sc, err := scheduleRow(rest)
				if err != nil {
					return nil, err
				}
				inv.Schedule = append(inv.Schedule, sc)
			case 1:
				v, err := rowInts(rest, 6)
				if err != nil {
					return nil, err
				}
				inv.RespawnMsg, inv.DespawnMsg, inv.Boss, inv.BossKilledMsg, inv.Time, inv.EffectType =
					v[0], v[1], v[2], v[3], v[4], v[5]
				if len(rest.Tokens) > 6 {
					inv.Name = rest.Tokens[6]
				}
			case 2:
				v, err := rowInts(rest, 3)
				if err != nil {
					return nil, err
				}
				inv.Groups[v[0]] = append(inv.Groups[v[0]], InvasionGroup{v[0], v[1], v[2]})
			case 3:
				v, err := rowInts(rest, 13)
				if err != nil {
					return nil, err
				}
				inv.Spawns = append(inv.Spawns, InvasionSpawn{v[0], v[1], v[2], v[3], v[4], v[5], v[6], v[7],
					v[8], v[9], v[10], v[11], v[12]})
			}
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

// loadTimedEvents reads sections 0 and 1 of an event file into 20 slots.
func loadTimedEvents(secs []mutext.Section) ([]*TimedEvent, error) {
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
	for _, s := range secs {
		if s.Index > 1 {
			continue
		}
		for _, l := range s.Rows {
			idx, err := l.Int(0)
			if err != nil {
				return nil, err
			}
			ev := get(idx)
			if ev == nil {
				continue
			}
			rest := mutext.Line{No: l.No, Tokens: l.Tokens[1:]}
			if s.Index == 0 {
				sc, err := scheduleRow(rest)
				if err != nil {
					return nil, err
				}
				ev.Schedule = append(ev.Schedule, sc)
				continue
			}
			v, err := rowInts(rest, 3)
			if err != nil {
				return nil, err
			}
			ev.StartMsg, ev.FinalMsg, ev.Time = v[0], v[1], v[2]
			if len(rest.Tokens) > 3 {
				ev.Name = rest.Tokens[3]
			}
		}
	}
	return out, nil
}

// LoadBonusManager reads Data/Event/BonusManager.dat (CBonusManager).
func LoadBonusManager(dataDir string) ([]*TimedEvent, []Bonus, error) {
	secs, err := loadSections(filepath.Join(dataDir, "Event", "BonusManager.dat"))
	if err != nil {
		return nil, nil, err
	}
	events, err := loadTimedEvents(secs)
	if err != nil {
		return nil, nil, err
	}
	var bonuses []Bonus
	for _, s := range secs {
		if s.Index != 2 {
			continue
		}
		for _, l := range s.Rows {
			v, err := rowInts(l, 7)
			if err != nil {
				return nil, nil, err
			}
			bonuses = append(bonuses, Bonus{v[0], v[1], [4]int{v[2], v[3], v[4], v[5]}, v[6]})
		}
	}
	return events, bonuses, nil
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

// LoadChaosCastle reads Data/Event/ChaosCastle.dat. Rows of section 2
// outside 0..5 are skipped ("Invalid experience level" in the original).
func LoadChaosCastle(dataDir string) (*ChaosCastle, error) {
	secs, err := loadSections(filepath.Join(dataDir, "Event", "ChaosCastle.dat"))
	if err != nil {
		return nil, err
	}
	d := &ChaosCastle{}
	for _, s := range secs {
		for _, l := range s.Rows {
			switch s.Index {
			case 0:
				v, err := rowInts(l, 4)
				if err != nil {
					return nil, err
				}
				d.ReadyTime, d.StandTime, d.EventTime, d.CloseTime = v[0], v[1], v[2], v[3]
			case 1:
				sc, err := scheduleRow(l)
				if err != nil {
					return nil, err
				}
				d.Schedule = append(d.Schedule, sc)
			case 2:
				v, err := rowInts(l, 3)
				if err != nil {
					return nil, err
				}
				if v[0] < 0 || v[0] > 5 {
					continue
				}
				d.Experience[v[0]] = [2]int{v[1], v[2]}
			}
		}
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

// LoadDropEvent reads Data/Event/DropEvent.dat (CDropEvent).
func LoadDropEvent(dataDir string) ([]*TimedEvent, []DropEventItem, error) {
	secs, err := loadSections(filepath.Join(dataDir, "Event", "DropEvent.dat"))
	if err != nil {
		return nil, nil, err
	}
	events, err := loadTimedEvents(secs)
	if err != nil {
		return nil, nil, err
	}
	var items []DropEventItem
	for _, s := range secs {
		if s.Index != 2 {
			continue
		}
		for _, l := range s.Rows {
			v := numRow(numFields(l), 0, 18)
			r := DropEventItem{Event: v[0], Index: v[1]*512 + v[2], Level: v[3], Dur: v[4], Duration: v[11],
				Map: v[12], Monster: v[13], MinLevel: v[14], MaxLevel: v[15], AccountLevel: v[16], Rate: v[17]}
			copy(r.Option[:], v[5:11])
			items = append(items, r)
		}
	}
	return events, items, nil
}

// GoldenArcherReward is an Event/GoldenArcher.dat row: ItemIndex
// RewardGroup Option0..5 (ItemOptionRate rows) Duration DropRate (weight).
type GoldenArcherReward struct {
	Index, Group int
	Option       [6]int
	Duration     int
	Rate         int
}

// LoadGoldenArcher reads Data/Event/GoldenArcher.dat (CGoldenArcher,
// FUN_00461660).
func LoadGoldenArcher(dataDir string) ([]GoldenArcherReward, error) {
	lines, err := mutext.LoadFile(filepath.Join(dataDir, "Event", "GoldenArcher.dat"))
	if err != nil {
		return nil, err
	}
	var out []GoldenArcherReward
	for _, l := range mutext.Table(lines) {
		v := numRow(numFields(l), 0, 11)
		r := GoldenArcherReward{Index: v[0]*512 + v[1], Group: v[2], Duration: v[9], Rate: v[10]}
		copy(r.Option[:], v[3:9])
		out = append(out, r)
	}
	return out, nil
}
