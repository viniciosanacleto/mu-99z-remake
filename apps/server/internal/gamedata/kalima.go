package gamedata

import (
	"path/filepath"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/config/mutext"
)

// KalimaLevels is the number of rows of Event/Kalima.dat.
const KalimaLevels = 6

// LoadKalima reads Event/Kalima.dat (CKalima, FUN_004a2c20): rows
// "KALevel CreateNextPortal" up to "end"; levels outside 0..5 are ignored.
// Entry n enables the gate to Kalima n+2 when the Illusion of Kundun of
// Kalima n+1 dies.
func LoadKalima(dataDir string) ([KalimaLevels]int, error) {
	var out [KalimaLevels]int
	lines, err := mutext.LoadFile(filepath.Join(dataDir, "Event", "Kalima.dat"))
	if err != nil {
		return out, err
	}
	for _, l := range mutext.Table(lines) {
		v := numRow(numFields(l), 0, 2)
		if v[0] >= 0 && v[0] < KalimaLevels {
			out[v[0]] = v[1]
		}
	}
	return out, nil
}
