package legacy

import (
	"fmt"
	"path/filepath"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/config/mutext"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/config/tomlconf"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"
)

// shopGroup converts Data/ShopManager.txt and the Data/Shop/<ShopPath>.txt
// files it names; the other files of Data/Shop are not read by the original.
func shopGroup(dataDir string) ([]byte, error) {
	c := gamedata.ShopConfig{Shop: map[string]gamedata.ShopFile{}}
	steps := []step{{"ShopManager.txt", func(l []mutext.Line) (err error) {
		c.ShopManager.Rows, err = Table[gamedata.ShopManagerRow](l, false)
		return err
	}}}
	srcs, err := runSteps(dataDir, steps)
	if err != nil {
		return nil, err
	}
	for _, r := range c.ShopManager.Rows {
		if _, done := c.Shop[r.ShopPath]; done {
			continue
		}
		rel := "Shop/" + r.ShopPath + ".txt"
		lines, err := File(filepath.Join(dataDir, filepath.FromSlash(tomlconf.ANSI(rel))))
		if err != nil {
			return nil, err
		}
		var shop gamedata.ShopFile
		// FUN_004f4aa0 needs eight columns; more are ignored.
		if shop.Rows, err = Table[gamedata.ShopRow](lines, true); err != nil {
			return nil, fmt.Errorf("%s: %w", rel, err)
		}
		c.Shop[r.ShopPath] = shop
		srcs = append(srcs, "Data/"+rel)
	}
	return marshal(&c, srcs...)
}
