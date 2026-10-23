package gamedata

import "path/filepath"

// CastleDeep is Event/CastleDeepEvent.dat (CCastleDeep::Load, FUN_00419320):
// section 0 the start schedule (this+0x20), 1 the group draw rates
// (this+0x30), 2 the minute each stage of a group spawns (this+0x40) and
// 3 the monsters of each group and stage (this+0x50).
type CastleDeep struct {
	Schedule []Schedule
	Groups   []CastleDeepGroup
	Stages   []CastleDeepStage
	Monsters []CastleDeepMonster
}

// CastleDeepGroup is a section 1 row: Group SpawnRate (FUN_0041a1c0).
type CastleDeepGroup struct{ Group, Rate int }

// CastleDeepStage is a section 2 row: Group Stage StageTime (minutes after
// the start, FUN_004933d0).
type CastleDeepStage struct{ Group, Stage, Time int }

// CastleDeepMonster is a section 3 row (FUN_0041a220): Group Stage
// MonsterIndex MaxSpawnCount RangeStartX RangeStartY RangeEndX RangeEndY.
type CastleDeepMonster struct {
	Group, Stage, Class, Count int
	X1, Y1, X2, Y2             int
}

// LoadCastleDeep reads Data/Event/CastleDeepEvent.dat. Sections other than
// 0..3 are skipped, like the token loop of the original.
func LoadCastleDeep(dataDir string) (*CastleDeep, error) {
	secs, err := loadSections(filepath.Join(dataDir, "Event", "CastleDeepEvent.dat"))
	if err != nil {
		return nil, err
	}
	d := &CastleDeep{}
	for _, s := range secs {
		for _, l := range s.Rows {
			switch s.Index {
			case 0:
				sc, err := scheduleRow(l)
				if err != nil {
					return nil, err
				}
				d.Schedule = append(d.Schedule, sc)
			case 1:
				v, err := rowInts(l, 2)
				if err != nil {
					return nil, err
				}
				d.Groups = append(d.Groups, CastleDeepGroup{v[0], v[1]})
			case 2:
				v, err := rowInts(l, 3)
				if err != nil {
					return nil, err
				}
				d.Stages = append(d.Stages, CastleDeepStage{v[0], v[1], v[2]})
			case 3:
				v, err := rowInts(l, 8)
				if err != nil {
					return nil, err
				}
				d.Monsters = append(d.Monsters, CastleDeepMonster{v[0], v[1], v[2], v[3], v[4], v[5], v[6], v[7]})
			}
		}
	}
	return d, nil
}
