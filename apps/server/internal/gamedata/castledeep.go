package gamedata

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

// LoadCastleDeep reads the CastleDeepEvent table of config/data/event.toml.
func LoadCastleDeep(configDir string) (*CastleDeep, error) {
	f, err := LoadEventConfig(configDir)
	if err != nil {
		return nil, err
	}
	t := f.CastleDeepEvent
	d := &CastleDeep{}
	for _, r := range t.Schedule {
		d.Schedule = append(d.Schedule, r.Schedule())
	}
	for _, r := range t.Group {
		d.Groups = append(d.Groups, CastleDeepGroup{r.Group, r.SpawnRate})
	}
	for _, r := range t.Stage {
		d.Stages = append(d.Stages, CastleDeepStage{r.Group, r.Stage, r.StageTime})
	}
	for _, r := range t.Monster {
		d.Monsters = append(d.Monsters, CastleDeepMonster{r.Group, r.Stage, r.MonsterIndex, r.MaxSpawnCount,
			r.RangeStartX, r.RangeStartY, r.RangeEndX, r.RangeEndY})
	}
	return d, nil
}
