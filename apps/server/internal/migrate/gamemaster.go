package migrate

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/config/legacy"
)

// ImportGameMasters writes the levels of Util/GameMaster.txt into
// characters.gm_level, matching account login and character name like
// FUN_0047db10. It returns the number of characters set and the rows with
// no such account and character.
func ImportGameMasters(ctx context.Context, tx pgx.Tx, gms []legacy.GameMaster) (set int, missing []string, err error) {
	for _, g := range gms {
		tag, err := tx.Exec(ctx, `UPDATE characters c SET gm_level=$3 FROM accounts a
			WHERE c.account_id=a.id AND a.login=$1 AND c.name=$2`, g.Account, g.Name, int16(g.Level))
		if err != nil {
			return set, missing, err
		}
		if tag.RowsAffected() == 0 {
			missing = append(missing, g.Account+"/"+g.Name)
			continue
		}
		set++
	}
	return set, missing, nil
}
