package gamedata

import (
	"fmt"
	"path/filepath"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/config/mutext"
)

// WizardTeam world customs read with the standard tokenizer (one table
// closed by "end"; "*" = -1).

func customTable(dataDir, name string) ([]mutext.Line, error) {
	lines, err := mutext.LoadFile(filepath.Join(dataDir, "Custom", name))
	if err != nil {
		return nil, err
	}
	return mutext.Table(lines), nil
}

func lineInts(l mutext.Line, cols []int) ([]int, error) {
	out := make([]int, len(cols))
	for i, c := range cols {
		v, err := l.Int(c)
		if err != nil {
			return nil, err
		}
		out[i] = v
	}
	return out, nil
}

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

// LoadCustomMove reads Custom/CustomMove.txt, sorted by Id.
func LoadCustomMove(dataDir string) ([]CustomMoveRow, error) {
	rows, err := customTable(dataDir, "CustomMove.txt")
	if err != nil {
		return nil, err
	}
	seen := map[int]bool{}
	var out []CustomMoveRow
	for _, l := range rows {
		if len(l.Tokens) < 13 {
			return nil, fmt.Errorf("CustomMove.txt line %d: %d columns", l.No, len(l.Tokens))
		}
		v, err := lineInts(l, []int{0, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12})
		if err != nil {
			return nil, err
		}
		if seen[v[0]] {
			continue
		}
		seen[v[0]] = true
		cmd := l.Tokens[1]
		if len(cmd) > 0x1F {
			cmd = cmd[:0x1F]
		}
		out = append(out, CustomMoveRow{Index: v[0], Command: cmd, Map: v[1], X: v[2], Y: v[3], MinLevel: v[4],
			MaxLevel: v[5], MinReset: v[6], MaxReset: v[7], MinMReset: v[8], MaxMReset: v[9], AccountLevel: v[10],
			PKMove: v[11]})
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

// LoadCustomGate reads Custom/CustomGate.txt, sorted by GateIndex.
func LoadCustomGate(dataDir string) ([]CustomGateRow, error) {
	rows, err := customTable(dataDir, "CustomGate.txt")
	if err != nil {
		return nil, err
	}
	seen := map[int]bool{}
	var out []CustomGateRow
	for _, l := range rows {
		v, err := lineInts(l, []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11})
		if err != nil {
			return nil, fmt.Errorf("CustomGate.txt: %w", err)
		}
		if seen[v[0]] {
			continue
		}
		seen[v[0]] = true
		out = append(out, CustomGateRow{Index: v[0], Map: v[1], X: v[2], Y: v[3], ToMap: v[4], ToX: v[5], ToY: v[6],
			MinLevel: v[7], MaxLevel: v[8], MinReset: v[9], MaxReset: v[10], AccountLevel: v[11]})
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

// LoadCustomRespawn reads Custom/CustomRespawn.txt.
func LoadCustomRespawn(dataDir string) ([]CustomRespawnRow, error) {
	rows, err := customTable(dataDir, "CustomRespawn.txt")
	if err != nil {
		return nil, err
	}
	var out []CustomRespawnRow
	for _, l := range rows {
		v, err := lineInts(l, []int{0, 1, 2, 3, 4})
		if err != nil {
			return nil, fmt.Errorf("CustomRespawn.txt: %w", err)
		}
		out = append(out, CustomRespawnRow{Map: v[0], Gate: [4]int{v[1], v[2], v[3], v[4]}})
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

// LoadCustomNPCCommand reads Custom/CustomNPCCommand.txt.
func LoadCustomNPCCommand(dataDir string) ([]CustomNPCCommandRow, error) {
	rows, err := customTable(dataDir, "CustomNPCCommand.txt")
	if err != nil {
		return nil, err
	}
	var out []CustomNPCCommandRow
	for _, l := range rows {
		if len(l.Tokens) < 5 {
			return nil, fmt.Errorf("CustomNPCCommand.txt line %d: %d columns", l.No, len(l.Tokens))
		}
		v, err := lineInts(l, []int{0, 1, 2, 3})
		if err != nil {
			return nil, err
		}
		out = append(out, CustomNPCCommandRow{Class: v[0], Map: v[1], X: v[2], Y: v[3], Command: l.Tokens[4]})
	}
	return out, nil
}

// LoadCustomDeathMessage reads Custom/CustomDeathMessage.txt
// (CustomDeathMessage, FUN_0044aac0): monster class -> the sentence the
// monster says after a kill (std::map: the first row of a class wins).
func LoadCustomDeathMessage(dataDir string) (map[int]string, error) {
	rows, err := customTable(dataDir, "CustomDeathMessage.txt")
	if err != nil {
		return nil, err
	}
	out := map[int]string{}
	for _, l := range rows {
		if len(l.Tokens) < 2 {
			return nil, fmt.Errorf("CustomDeathMessage.txt line %d: %d columns", l.No, len(l.Tokens))
		}
		c, err := l.Int(0)
		if err != nil {
			return nil, err
		}
		if _, ok := out[c]; !ok {
			out[c] = l.Tokens[1]
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
