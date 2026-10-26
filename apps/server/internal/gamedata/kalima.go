package gamedata

import ()

// KalimaLevels is the number of rows of Event/Kalima.dat.
const KalimaLevels = 6

// LoadKalima reads the Kalima table of config/data/event.toml (CKalima,
// FUN_004a2c20): KALevel CreateNextPortal; levels outside 0..5 are ignored.
// Entry n enables the gate to Kalima n+2 when the Illusion of Kundun of
// Kalima n+1 dies.
func LoadKalima(configDir string) ([KalimaLevels]int, error) {
	var out [KalimaLevels]int
	f, err := LoadEventConfig(configDir)
	if err != nil {
		return out, err
	}
	for _, r := range f.Kalima.Rows {
		if r.KALevel >= 0 && r.KALevel < KalimaLevels {
			out[r.KALevel] = r.CreateNextPortal
		}
	}
	return out, nil
}
