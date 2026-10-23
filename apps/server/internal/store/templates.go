package store

import (
	"embed"
	"encoding/hex"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

// ClassTemplate is a DefaultClassType row, copied into new characters by
// WZ_CreateCharacter.
type ClassTemplate struct {
	Class                                             byte
	Level, LevelUpPoints                              int
	Strength, Dexterity, Vitality, Energy, Leadership int
	Inventory, MagicList, Quest, EffectList           []byte
	Life, MaxLife, Mana, MaxMana                      float32
	Map, X, Y                                         int
}

var templateRow = regexp.MustCompile(`INSERT INTO default_class_templates VALUES \((.*)\);`)
var hexValue = regexp.MustCompile(`^decode\('([0-9A-Fa-f]*)','hex'\)$`)

// ClassTemplates parses the seed migration so the in-memory store uses the
// same data as PostgreSQL.
func ClassTemplates() (map[byte]ClassTemplate, error) {
	data, err := migrationFS.ReadFile("migrations/0002_default_classes.sql")
	if err != nil {
		return nil, err
	}
	out := map[byte]ClassTemplate{}
	for _, m := range templateRow.FindAllStringSubmatch(string(data), -1) {
		f := splitTopLevel(m[1])
		if len(f) != 19 {
			return nil, fmt.Errorf("template row: %d columns", len(f))
		}
		ints := make([]int, len(f))
		blobs := make([][]byte, len(f))
		floats := make([]float32, len(f))
		for i, s := range f {
			s = strings.TrimSpace(s)
			if hm := hexValue.FindStringSubmatch(s); hm != nil {
				if blobs[i], err = hex.DecodeString(hm[1]); err != nil {
					return nil, err
				}
				continue
			}
			v, err := strconv.ParseFloat(s, 32)
			if err != nil {
				return nil, fmt.Errorf("template value %q: %w", s, err)
			}
			ints[i], floats[i] = int(v), float32(v)
		}
		t := ClassTemplate{
			Class: byte(ints[0]), Level: ints[1], LevelUpPoints: ints[2],
			Strength: ints[3], Dexterity: ints[4], Vitality: ints[5], Energy: ints[6], Leadership: ints[7],
			Inventory: blobs[8], MagicList: blobs[9],
			Life: floats[10], MaxLife: floats[11], Mana: floats[12], MaxMana: floats[13],
			Map: ints[14], X: ints[15], Y: ints[16], Quest: blobs[17], EffectList: blobs[18],
		}
		out[t.Class] = t
	}
	return out, nil
}

// splitTopLevel splits on commas that are not inside parentheses.
func splitTopLevel(s string) []string {
	var out []string
	depth, start := 0, 0
	for i, r := range s {
		switch r {
		case '(':
			depth++
		case ')':
			depth--
		case ',':
			if depth == 0 {
				out = append(out, s[start:i])
				start = i + 1
			}
		}
	}
	return append(out, s[start:])
}
