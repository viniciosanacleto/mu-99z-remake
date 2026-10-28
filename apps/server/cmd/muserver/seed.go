package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/auth"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/loadbot"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/store"
)

// Load test characters (loadbot.Account names them).
const (
	seedMap     = 0 // Lorencia
	seedLevel   = 50
	seedStatAdd = 150
)

// seedArea is the Spider / Budge Dragon field east of Lorencia
// (config/data/spawn.toml "000 - Lorencia", Section 1 rows of monsters 2 and 3).
var seedArea = struct{ x0, y0, x1, y1 int }{176, 108, 200, 132}

// seedCells lists the walkable cells of seedArea outside the safe zone.
func seedCells(t *gamedata.Terrain) [][2]int {
	var out [][2]int
	for y := seedArea.y0; y <= seedArea.y1; y++ {
		for x := seedArea.x0; x <= seedArea.x1; x++ {
			if t.Get(x, y)&(gamedata.AttrSafeZone|gamedata.AttrBlock|gamedata.AttrNoGround) == 0 {
				out = append(out, [2]int{x, y})
			}
		}
	}
	return out
}

// seedLoad creates (or resets) n load test accounts, each with one
// character placed on a free cell of seedArea. One argon2 hash is shared by
// every account so seeding stays fast; each login still verifies it.
func seedLoad(ctx context.Context, st store.Store, gameDataDir string, n int) error {
	b, err := os.ReadFile(filepath.Join(gameDataDir, "Terrain", fmt.Sprintf("Terrain%d.att", seedMap+1)))
	if err != nil {
		return err
	}
	t, err := gamedata.ParseTerrain("Terrain1.att", b)
	if err != nil {
		return err
	}
	cells := seedCells(t)
	if len(cells) == 0 {
		return fmt.Errorf("no free cell in the seed area")
	}
	hash, err := auth.HashPassword(loadbot.Password)
	if err != nil {
		return err
	}
	for i := 1; i <= n; i++ {
		login, name := loadbot.Account(i)
		acc, err := st.AccountByLogin(ctx, login)
		if err != nil || acc == nil {
			if acc, err = st.CreateAccount(ctx, login, hash, ""); err != nil {
				return fmt.Errorf("%s: %w", login, err)
			}
		}
		ch, err := st.LoadCharacter(ctx, acc.ID, name)
		if err != nil || ch == nil {
			r, _, err := st.CreateCharacter(ctx, acc.ID, name, loadbot.AccountClass(i))
			if err != nil || r != store.CreateOK {
				return fmt.Errorf("%s: create result %d: %v", name, r, err)
			}
			if ch, err = st.LoadCharacter(ctx, acc.ID, name); err != nil {
				return fmt.Errorf("%s: %w", name, err)
			}
		}
		c := cells[(i-1)*7%len(cells)] // stride spreads neighbours apart
		ch.Map, ch.X, ch.Y = seedMap, c[0], c[1]
		if ch.Level < seedLevel {
			ch.Level = seedLevel
			ch.Strength += seedStatAdd
			ch.Dexterity += seedStatAdd
			ch.Vitality += seedStatAdd
			ch.Energy += seedStatAdd
		}
		if err := st.SaveCharacter(ctx, ch); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
	}
	return nil
}
