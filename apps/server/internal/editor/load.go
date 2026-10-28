package editor

import (
	"fmt"
	"path/filepath"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/config/gsconfig"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"
)

// Load builds the editor over st from the item data and the GameServer
// instance (<configDir>/gameserver/<instance>.toml) whose Common.dat
// limits apply.
func Load(configDir, instance string, st Store) (*Editor, error) {
	items, err := gamedata.LoadItems(configDir)
	if err != nil {
		return nil, err
	}
	opts, err := gamedata.LoadItemOptions(configDir)
	if err != nil {
		return nil, err
	}
	gcfg, err := gsconfig.Load(filepath.Join(configDir, "gameserver", instance+".toml"))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", instance, err)
	}
	lim := Limits{MaxLevel: gcfg.ServerMaxLevel, Exp: gamedata.NewExperience(
		int32(gcfg.Int("Common", "ExperienceMultiplierConstA", 0)),
		int32(gcfg.Int("Common", "ExperienceMultiplierConstB", 0)),
		int32(gcfg.Int("Common", "PetExperienceMultiplierConstA", 0)))}
	for al := range lim.MaxStat {
		lim.MaxStat[al] = gcfg.ALInt("Common", "MaxStatPoint", al, 0)
	}
	return New(st, NewCatalog(items, opts), lim), nil
}
