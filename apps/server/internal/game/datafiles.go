package game

import "github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"

// Start-up loading of the small data files parsed in gamedata/datafiles.go
// that change game rules here. The others are parsed and checked by the
// data coverage test (datacoverage_test.go) until their systems exist.

func init() {
	startHooks = append(startHooks, loadDataFiles)
}

func loadDataFiles(w *World) error {
	if w.configDir == "" {
		return nil
	}
	var err error
	w.expTable, err = gamedata.LoadExperienceTable(w.configDir) // FUN_004ec4c0 -> FUN_0047be80
	return err
}
