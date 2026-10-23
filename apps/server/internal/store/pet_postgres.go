package store

import (
	"context"

	"github.com/jackc/pgx/v5"
)

func (p *Postgres) LoadPetInfo(ctx context.Context, serials []uint32) ([]PetInfo, error) {
	out := make([]PetInfo, 0, len(serials))
	for _, s := range serials {
		info := DefaultPetInfo(s)
		var lvl int
		var exp int64
		err := p.pool.QueryRow(ctx, `SELECT pet_level, pet_exp FROM pet_item_info WHERE item_serial=$1`,
			int64(s)).Scan(&lvl, &exp)
		switch err {
		case nil:
			info.Level, info.Exp = lvl, uint32(exp)
		case pgx.ErrNoRows:
			// FUN_0040dfc0: insert the missing row and answer level 1, exp 0.
			if _, err := p.pool.Exec(ctx, `INSERT INTO pet_item_info(item_serial, pet_level, pet_exp)
				VALUES ($1,$2,$3) ON CONFLICT (item_serial) DO NOTHING`, int64(s), info.Level, int64(info.Exp)); err != nil {
				return nil, err
			}
		default:
			return nil, err
		}
		out = append(out, info)
	}
	return out, nil
}

func (p *Postgres) SavePetInfo(ctx context.Context, pets []PetInfo) error {
	for _, i := range pets {
		if _, err := p.pool.Exec(ctx, `INSERT INTO pet_item_info(item_serial, pet_level, pet_exp) VALUES ($1,$2,$3)
			ON CONFLICT (item_serial) DO UPDATE SET pet_level=EXCLUDED.pet_level, pet_exp=EXCLUDED.pet_exp`,
			int64(i.Serial), i.Level, int64(i.Exp)); err != nil {
			return err
		}
	}
	return nil
}

func (p *Postgres) CreatePetInfo(ctx context.Context, i PetInfo) error {
	_, err := p.pool.Exec(ctx, `INSERT INTO pet_item_info(item_serial, pet_level, pet_exp) VALUES ($1,$2,$3)
		ON CONFLICT (item_serial) DO NOTHING`, int64(i.Serial), i.Level, int64(i.Exp))
	return err
}
