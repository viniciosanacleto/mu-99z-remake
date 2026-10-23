// Package gamedata loads the original game data files under Data/
// (docs/server-plan/11-arquivos-de-configuracao.md §4).
package gamedata

import (
	"fmt"
	"os"
	"path/filepath"
)

// MaxMaps is the size of the original map table (0x87d1c8 .. 0xfee628,
// 0x130d8 bytes per map).
const MaxMaps = 100

// Terrain attribute bits (CMap, FUN_004ad4d0 tests them).
const (
	AttrSafeZone  byte = 0x01
	AttrCharacter byte = 0x02 // set while a character stands on the cell
	AttrBlock     byte = 0x04
	AttrNoGround  byte = 0x08
	AttrWall      byte = 0x10 // castle gates (map 30)
)

// Terrain is one map's attribute grid.
type Terrain struct {
	File          string
	Width, Height int
	Attr          []byte
}

// Get returns the attribute of a cell; out of range cells read as blocked.
func (t *Terrain) Get(x, y int) byte {
	if t == nil || x < 0 || y < 0 || x >= t.Width || y >= t.Height {
		return AttrBlock | AttrNoGround
	}
	return t.Attr[y*t.Height+x]
}

// Set overwrites a cell attribute (used for the dynamic 0x02 bit).
func (t *Terrain) Set(x, y int, v byte) {
	if t == nil || x < 0 || y < 0 || x >= t.Width || y >= t.Height {
		return
	}
	t.Attr[y*t.Height+x] = v
}

// Clone returns an independent copy (each game instance owns its maps).
func (t *Terrain) Clone() *Terrain {
	if t == nil {
		return nil
	}
	c := *t
	c.Attr = append([]byte(nil), t.Attr...)
	return &c
}

// ParseTerrain decodes a .att file: version, width-1, height-1, cells.
func ParseTerrain(name string, data []byte) (*Terrain, error) {
	if len(data) < 3 {
		return nil, fmt.Errorf("%s: error reading file header", name)
	}
	w, h := int(data[1])+1, int(data[2])+1
	if len(data) < 3+w*h {
		return nil, fmt.Errorf("%s: error reading map data", name)
	}
	return &Terrain{File: name, Width: w, Height: h, Attr: append([]byte(nil), data[3:3+w*h]...)}, nil
}

// LoadTerrains reproduces FUN_004eb3a0: map i uses terrain<i+1>.att; when the
// file does not exist the last file found is used again (so Blood Castle 2-7
// reuse terrain12.att, Chaos Castle 2-6 reuse terrain19.att, ...).
func LoadTerrains(dataDir string) ([MaxMaps]*Terrain, error) {
	var out [MaxMaps]*Terrain
	dir := filepath.Join(dataDir, "Terrain")
	last := ""
	var cache = map[string]*Terrain{}
	for i := 0; i < MaxMaps; i++ {
		name := fmt.Sprintf("Terrain%d.att", i+1)
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			last = name
		}
		if last == "" {
			return out, fmt.Errorf("%s: error opening file", filepath.Join(dir, name))
		}
		t := cache[last]
		if t == nil {
			data, err := os.ReadFile(filepath.Join(dir, last))
			if err != nil {
				return out, err
			}
			if t, err = ParseTerrain(last, data); err != nil {
				return out, err
			}
			cache[last] = t
		}
		out[i] = t.Clone()
	}
	return out, nil
}
