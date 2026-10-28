package legacy

import (
	"fmt"
	"path/filepath"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/config/mutext"
)

// GameMaster is one row of Util/GameMaster.txt (FUN_0047db10): the GM
// level of an account and character. The rewrite keeps the level in
// characters.gm_level, so the file is not converted to TOML; cmd/migrate
// imports it once (ReadGameMasters).
type GameMaster struct {
	Account, Name string
	Level         int
}

// ReadGameMasters reads <root>/Data/Util/GameMaster.txt (root is the
// MuServer folder). Rows without a level are skipped.
func ReadGameMasters(root string) ([]GameMaster, error) {
	lines, err := File(filepath.Join(root, "Data", "Util", "GameMaster.txt"))
	if err != nil {
		return nil, err
	}
	var out []GameMaster
	for _, r := range mutext.Table(lines) {
		if len(r.Tokens) < 3 {
			continue
		}
		lv, err := r.Int(2)
		if err != nil {
			return nil, fmt.Errorf("Util/GameMaster.txt: %w", err)
		}
		out = append(out, GameMaster{Account: tok(r, 0), Name: tok(r, 1), Level: lv})
	}
	return out, nil
}
