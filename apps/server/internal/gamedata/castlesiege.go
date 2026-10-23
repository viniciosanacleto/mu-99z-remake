package gamedata

import (
	"fmt"
	"path/filepath"
	"sort"
)

// CastleSiege is Event/CastleSiege.dat (CCastleSiege::LoadData,
// FUN_0041ab60):
//
//	0  CycleDays (CCastleSiege+0x18)
//	1  State StartDay StartHour StartMinute (schedule, 0x18-byte entries at +0xec)
//	2  Class Index DBSave Side DFLevel RGLevel MaxLife MapX MapY MapDir
//	   (NPC list, 0x54-byte entries at +0xdc)
//	3  gate defense upgrades     Index Value Count Money (DAT_0056989c)
//	4  statue defense upgrades   (DAT_005698e4)
//	5  gate life upgrades        (DAT_005698c0)
//	6  statue life upgrades      (DAT_00569908)
//	7  statue regen upgrades     (DAT_0056992c)
//	8  gate defense per DF level 0..3 (DAT_0056987c)
//	9  statue defense per DF level 0..3 (DAT_0056988c)
type CastleSiege struct {
	CycleDays int
	Schedule  []CastleSchedule
	NPCs      []CastleNPC

	GateDefenseUpgrade   [3]CastleUpgrade
	StatueDefenseUpgrade [3]CastleUpgrade
	GateLifeUpgrade      [3]CastleUpgrade
	StatueLifeUpgrade    [3]CastleUpgrade
	StatueRegenUpgrade   [3]CastleUpgrade

	GateDefense   [4]int
	StatueDefense [4]int
}

// CastleSchedule is one row of section 1. Duration is the number of
// seconds until the next row (0 for the last one), computed after sorting
// by state like the end of FUN_0041ab60.
type CastleSchedule struct {
	State, Day, Hour, Minute int
	Duration                 int
}

// CastleNPC is one row of section 2.
type CastleNPC struct {
	Class, Index int
	DBSave       bool
	Side         int
	DFLevel      int
	RGLevel      int
	MaxLife      int
	X, Y, Dir    int
}

// CastleUpgrade is one row of sections 3..7: the target value (level or
// life), the Jewel of Guardian count and the zen.
type CastleUpgrade struct {
	Value, Count, Money int
}

// LoadCastleSiege reads Data/Event/CastleSiege.dat.
func LoadCastleSiege(dataDir string) (*CastleSiege, error) {
	secs, err := loadSections(filepath.Join(dataDir, "Event", "CastleSiege.dat"))
	if err != nil {
		return nil, err
	}
	cs := &CastleSiege{CycleDays: 7}
	upg := map[int]*[3]CastleUpgrade{3: &cs.GateDefenseUpgrade, 4: &cs.StatueDefenseUpgrade,
		5: &cs.GateLifeUpgrade, 6: &cs.StatueLifeUpgrade, 7: &cs.StatueRegenUpgrade}
	for _, s := range secs {
		for _, l := range s.Rows {
			switch s.Index {
			case 0:
				v, err := rowInts(l, 1)
				if err != nil {
					return nil, err
				}
				cs.CycleDays = v[0]
			case 1:
				v, err := rowInts(l, 4)
				if err != nil {
					return nil, err
				}
				cs.Schedule = append(cs.Schedule, CastleSchedule{State: v[0], Day: v[1], Hour: v[2], Minute: v[3]})
			case 2:
				v, err := rowInts(l, 10)
				if err != nil {
					return nil, err
				}
				cs.NPCs = append(cs.NPCs, CastleNPC{Class: v[0], Index: v[1], DBSave: v[2] != 0, Side: v[3],
					DFLevel: v[4], RGLevel: v[5], MaxLife: v[6], X: v[7], Y: v[8], Dir: v[9]})
			case 3, 4, 5, 6, 7:
				v, err := rowInts(l, 4)
				if err != nil {
					return nil, err
				}
				if v[0] < 0 || v[0] > 2 {
					return nil, fmt.Errorf("CastleSiege.dat section %d line %d: index %d", s.Index, l.No, v[0])
				}
				upg[s.Index][v[0]] = CastleUpgrade{Value: v[1], Count: v[2], Money: v[3]}
			case 8, 9:
				v, err := rowInts(l, 4)
				if err != nil {
					return nil, err
				}
				dst := &cs.GateDefense
				if s.Index == 9 {
					dst = &cs.StatueDefense
				}
				copy(dst[:], v)
			}
		}
	}
	// FUN_00427da0 with FUN_004202b0: sort by state, then each entry lasts
	// until the next one: ((dDay*24 - h0 + h1)*60 - m0 + m1)*60 seconds; a
	// negative value is reported ("Date Order is wrong") and left at 0.
	sort.SliceStable(cs.Schedule, func(i, j int) bool { return cs.Schedule[i].State < cs.Schedule[j].State })
	for i := range cs.Schedule {
		if i+1 >= len(cs.Schedule) {
			cs.Schedule[i].Duration = 0
			continue
		}
		a, b := cs.Schedule[i], cs.Schedule[i+1]
		d := (((b.Day-a.Day)*24-a.Hour+b.Hour)*60 - a.Minute + b.Minute) * 60
		if d < 0 {
			// Logged as "Date Order is wrong (sort fail)"; the entry keeps 0.
			continue
		}
		cs.Schedule[i].Duration = d
	}
	return cs, nil
}

// ScheduleOf returns the schedule row of a state.
func (cs *CastleSiege) ScheduleOf(state int) (CastleSchedule, bool) {
	for _, s := range cs.Schedule {
		if s.State == state {
			return s, true
		}
	}
	return CastleSchedule{}, false
}
