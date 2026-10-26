package gamedata

import ()

// Emulator world customs (config/data/custom.toml).

// CustomMoveRow is a CustomMove.txt row (CCustomMove, FUN_0044f210, 0x50
// bytes kept in a std::map by Id: the first row of an Id wins).
type CustomMoveRow struct {
	Index                int
	Command              string
	Map, X, Y            int
	MinLevel, MaxLevel   int
	MinReset, MaxReset   int
	MinMReset, MaxMReset int
	AccountLevel         int
	PKMove               int
}

// LoadCustomMove reads the CustomMove table of config/data/custom.toml,
// sorted by Id.
func LoadCustomMove(configDir string) ([]CustomMoveRow, error) {
	f, err := LoadCustomConfig(configDir)
	if err != nil {
		return nil, err
	}
	seen := map[int]bool{}
	var out []CustomMoveRow
	for _, r := range f.CustomMove.Rows {
		if seen[r.Id] {
			continue
		}
		seen[r.Id] = true
		out = append(out, CustomMoveRow{Index: r.Id, Command: cutString(r.Command, 0x1F), Map: r.Map, X: r.X, Y: r.Y,
			MinLevel: r.MinLevel, MaxLevel: r.MaxLevel, MinReset: r.MinReset, MaxReset: r.MaxReset, MinMReset: r.MinMReset,
			MaxMReset: r.MaxMReset, AccountLevel: r.AccountLevel, PKMove: r.PKMove})
	}
	sortByIndex(out, func(r CustomMoveRow) int { return r.Index })
	return out, nil
}

// CustomGateRow is a CustomGate.txt row (CCustomGate, FUN_0044b650, a
// std::map by GateIndex): a player standing on (Map, X, Y) is moved to
// (ToMap, ToX, ToY).
type CustomGateRow struct {
	Index              int
	Map, X, Y          int
	ToMap, ToX, ToY    int
	MinLevel, MaxLevel int
	MinReset, MaxReset int
	AccountLevel       int
}

// LoadCustomGate reads the CustomGate table of config/data/custom.toml,
// sorted by GateIndex.
func LoadCustomGate(configDir string) ([]CustomGateRow, error) {
	f, err := LoadCustomConfig(configDir)
	if err != nil {
		return nil, err
	}
	seen := map[int]bool{}
	var out []CustomGateRow
	for _, r := range f.CustomGate.Rows {
		if seen[r.GateIndex] {
			continue
		}
		seen[r.GateIndex] = true
		out = append(out, CustomGateRow{Index: r.GateIndex, Map: r.MapStart, X: r.RangeStartX, Y: r.RangeStartY,
			ToMap: r.MapEnd, ToX: r.RangeEndX, ToY: r.RangeEndY, MinLevel: r.MinLevel, MaxLevel: r.MaxLevel,
			MinReset: r.MinReset, MaxReset: r.MaxReset, AccountLevel: r.AccountLevel})
	}
	sortByIndex(out, func(r CustomGateRow) int { return r.Index })
	return out, nil
}

// CustomRespawnRow is a CustomRespawn.txt row (CCustomRespawn,
// FUN_004585f0): the respawn of a map by account level: -1 the death
// cell, 0 the death cell moved 0..2 down, a gate number, or -2 nothing.
type CustomRespawnRow struct {
	Map  int
	Gate [4]int
}

// LoadCustomRespawn reads the CustomRespawn table of config/data/custom.toml.
func LoadCustomRespawn(configDir string) ([]CustomRespawnRow, error) {
	f, err := LoadCustomConfig(configDir)
	if err != nil {
		return nil, err
	}
	var out []CustomRespawnRow
	for _, r := range f.CustomRespawn.Rows {
		out = append(out, CustomRespawnRow{Map: r.Mapa, Gate: [4]int{r.AccLevelGate_AL0, r.AccLevelGate_AL1,
			r.AccLevelGate_AL2, r.AccLevelGate_AL3}})
	}
	return out, nil
}

// CustomNPCCommandRow is a CustomNPCCommand.txt row (CCustomNpcCommand,
// FUN_004503a0): talking to the NPC class on that exact cell runs the
// command.
type CustomNPCCommandRow struct {
	Class, Map, X, Y int
	Command          string
}

// LoadCustomNPCCommand reads the CustomNPCCommand table of
// config/data/custom.toml.
func LoadCustomNPCCommand(configDir string) ([]CustomNPCCommandRow, error) {
	f, err := LoadCustomConfig(configDir)
	if err != nil {
		return nil, err
	}
	var out []CustomNPCCommandRow
	for _, r := range f.CustomNPCCommand.Rows {
		out = append(out, CustomNPCCommandRow{Class: r.Npc, Map: r.Map, X: r.X, Y: r.Y, Command: r.Command})
	}
	return out, nil
}

// LoadCustomDeathMessage reads the CustomDeathMessage table of
// config/data/custom.toml (CustomDeathMessage, FUN_0044aac0): monster class
// -> the sentence the monster says after a kill (std::map: the first row of
// a class wins).
func LoadCustomDeathMessage(configDir string) (map[int]string, error) {
	f, err := LoadCustomConfig(configDir)
	if err != nil {
		return nil, err
	}
	out := map[int]string{}
	for _, r := range f.CustomDeathMessage.Rows {
		if _, ok := out[r.MobIndex]; !ok {
			out[r.MobIndex] = r.Message
		}
	}
	return out, nil
}

func sortByIndex[T any](s []T, key func(T) int) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && key(s[j]) < key(s[j-1]); j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}
