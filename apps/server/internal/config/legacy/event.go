package legacy

import (
	"fmt"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/config/mutext"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"
)

// eventGroup converts Data/Event/*.dat (Crywolf.dat, IllusionTemple.dat and
// Kanturu.dat are not read by the original and are left out).
func eventGroup(dataDir string) ([]byte, error) {
	var c gamedata.EventConfig
	steps := []step{
		{"Event/BloodCastle.dat", sections(map[int]func([]mutext.Line) error{
			0: rowsInto(&c.BloodCastle.Time), 1: rowsInto(&c.BloodCastle.Schedule),
			2: rowsInto(&c.BloodCastle.Experience), 3: rowsInto(&c.BloodCastle.Money),
			4: rowsInto(&c.BloodCastle.Life), 5: rowsInto(&c.BloodCastle.Statue),
		})},
		{"Event/BonusManager.dat", sections(map[int]func([]mutext.Line) error{
			0: eventRows(&c.BonusManager.Schedule, eventSchedule),
			1: eventRows(&c.BonusManager.Settings, timedSettings),
			2: rowsInto(&c.BonusManager.Bonus),
		})},
		{"Event/CastleDeepEvent.dat", sections(map[int]func([]mutext.Line) error{
			0: rowsInto(&c.CastleDeepEvent.Schedule), 1: rowsInto(&c.CastleDeepEvent.Group),
			2: rowsInto(&c.CastleDeepEvent.Stage), 3: rowsInto(&c.CastleDeepEvent.Monster),
		})},
		{"Event/CastleSiege.dat", sections(map[int]func([]mutext.Line) error{
			0: rowsInto(&c.CastleSiege.Cycle), 1: rowsInto(&c.CastleSiege.Schedule), 2: rowsInto(&c.CastleSiege.NPC),
			3: upgradeRows(&c.CastleSiege.GateDefenseUpgrade), 4: upgradeRows(&c.CastleSiege.StatueDefenseUpgrade),
			5: upgradeRows(&c.CastleSiege.GateLifeUpgrade), 6: upgradeRows(&c.CastleSiege.StatueLifeUpgrade),
			7: upgradeRows(&c.CastleSiege.StatueRegenUpgrade),
			8: rowsInto(&c.CastleSiege.GateDefense), 9: rowsInto(&c.CastleSiege.StatueDefense),
		})},
		{"Event/ChaosCastle.dat", sections(map[int]func([]mutext.Line) error{
			0: rowsInto(&c.ChaosCastle.Time), 1: rowsInto(&c.ChaosCastle.Schedule), 2: rowsInto(&c.ChaosCastle.Experience),
		})},
		{"Event/DevilSquare.dat", sections(map[int]func([]mutext.Line) error{
			0: rowsInto(&c.DevilSquare.Time), 1: rowsInto(&c.DevilSquare.Schedule),
			2: rowsInto(&c.DevilSquare.Experience), 3: rowsInto(&c.DevilSquare.Money),
		})},
		{"Event/DropEvent.dat", sections(map[int]func([]mutext.Line) error{
			0: eventRows(&c.DropEvent.Schedule, eventSchedule),
			1: eventRows(&c.DropEvent.Settings, timedSettings),
			2: func(rows []mutext.Line) error {
				for _, r := range rows {
					v := mutext.NumRow(mutext.NumFields(r), 0, 18)
					c.DropEvent.Item = append(c.DropEvent.Item, gamedata.DropEventItemRow{EventIndex: v[0],
						ItemIndex: gamedata.FormatItemRef(v[1], v[2]), ItemLevel: v[3], Durability: v[4],
						Option: [6]int(v[5:11]), Duration: v[11], MapNumber: v[12], MonsterIndex: v[13],
						MonsterMinLevel: v[14], MonsterMaxLevel: v[15], AccountLevel: v[16], ItemDropRate: v[17],
						Comment: lineComment(r)})
				}
				return nil
			},
		})},
		{"Event/EventEntryLevel.dat", func(l []mutext.Line) error {
			secs, err := mutext.Sections(l)
			if err != nil {
				return err
			}
			for _, s := range secs {
				rows, err := Rows[gamedata.EntryLevelRow](s.Rows, true)
				if err != nil {
					return err
				}
				c.EventEntryLevel.Sections = append(c.EventEntryLevel.Sections, gamedata.EntryLevelSection{Section: s.Index, Rows: rows})
			}
			return nil
		}},
		{"Event/EventStageSpawn.dat", func(l []mutext.Line) error {
			secs, err := mutext.Sections(l)
			if err != nil {
				return err
			}
			for _, s := range secs {
				rows, err := Rows[gamedata.StageSpawnRow](s.Rows, true)
				if err != nil {
					return err
				}
				c.EventStageSpawn.Sections = append(c.EventStageSpawn.Sections, gamedata.StageSpawnSection{Section: s.Index, Rows: rows})
			}
			return nil
		}},
		{"Event/GoldenArcher.dat", func(l []mutext.Line) error {
			for _, r := range mutext.Table(l) {
				v := mutext.NumRow(mutext.NumFields(r), 0, 11)
				c.GoldenArcher.Rows = append(c.GoldenArcher.Rows, gamedata.GoldenArcherRow{
					ItemIndex: gamedata.FormatItemRef(v[0], v[1]), RewardGroup: v[2], Option: [6]int(v[3:9]),
					Duration: v[9], DropRate: v[10], Comment: lineComment(r)})
			}
			return nil
		}},
		{"Event/InvasionManager.dat", sections(map[int]func([]mutext.Line) error{
			0: eventRows(&c.InvasionManager.Schedule, eventSchedule),
			1: eventRows(&c.InvasionManager.Settings, func(l mutext.Line) (gamedata.InvasionSettingRow, error) {
				var r gamedata.InvasionSettingRow
				var v struct{ A, B, C, D, E, F, G int }
				if err := Row(l, &v, true); err != nil {
					return r, err
				}
				r = gamedata.InvasionSettingRow{EventIndex: v.A, RespawnMessage: v.B, DespawnMessage: v.C,
					MonsterBossIndex: v.D, BossKilledMessage: v.E, InvasionTime: v.F, EffectType: v.G,
					EventName: tok(l, 7), Comment: lineComment(l)}
				return r, nil
			}),
			2: eventRows(&c.InvasionManager.Group, rowOf[gamedata.InvasionGroupRow]),
			3: eventRows(&c.InvasionManager.Spawn, rowOf[gamedata.InvasionSpawnRow]),
		})},
		{"Event/Kalima.dat", func(l []mutext.Line) error {
			for _, r := range mutext.Table(l) {
				v := mutext.NumRow(mutext.NumFields(r), 0, 2)
				c.Kalima.Rows = append(c.Kalima.Rows, gamedata.KalimaRow{KALevel: v[0], CreateNextPortal: v[1],
					Comment: lineComment(r)})
			}
			return nil
		}},
	}
	srcs, err := runSteps(dataDir, steps)
	if err != nil {
		return nil, err
	}
	return marshal(&c, srcs...)
}

// sections dispatches the rows of each numbered section; sections without
// a handler are skipped like the readers of the original.
func sections(handlers map[int]func([]mutext.Line) error) func([]mutext.Line) error {
	return func(l []mutext.Line) error {
		secs, err := mutext.Sections(l)
		if err != nil {
			return err
		}
		for _, s := range secs {
			if h := handlers[s.Index]; h != nil {
				if err := h(s.Rows); err != nil {
					return fmt.Errorf("section %d: %w", s.Index, err)
				}
			}
		}
		return nil
	}
}

// rowsInto appends the rows decoded as T (leading columns, extra ones
// ignored) to dst.
func rowsInto[T any](dst *[]T) func([]mutext.Line) error {
	return func(rows []mutext.Line) error {
		out, err := Rows[T](rows, true)
		*dst = append(*dst, out...)
		return err
	}
}

func rowOf[T any](l mutext.Line) (T, error) {
	var r T
	err := Row(l, &r, true)
	return r, err
}

// eventRows decodes the rows of an event file with 20 slots: the first
// column is the event index; rows of an index outside 0..19 are ignored
// before the rest of the row is read.
func eventRows[T any](dst *[]T, conv func(mutext.Line) (T, error)) func([]mutext.Line) error {
	return func(rows []mutext.Line) error {
		for _, l := range rows {
			idx, err := l.Int(0)
			if err != nil {
				return err
			}
			if idx < 0 || idx >= 20 {
				continue
			}
			r, err := conv(l)
			if err != nil {
				return err
			}
			*dst = append(*dst, r)
		}
		return nil
	}
}

func eventSchedule(l mutext.Line) (gamedata.EventScheduleRow, error) {
	return rowOf[gamedata.EventScheduleRow](l)
}

// timedSettings is section 1 of BonusManager.dat / DropEvent.dat:
// EventIndex StartMessage FinalMessage Time [Name].
func timedSettings(l mutext.Line) (gamedata.TimedEventSettingsRow, error) {
	var v struct{ A, B, C, D int }
	if err := Row(l, &v, true); err != nil {
		return gamedata.TimedEventSettingsRow{}, err
	}
	return gamedata.TimedEventSettingsRow{EventIndex: v.A, StartMessage: v.B, FinalMessage: v.C, EventTime: v.D,
		EventName: tok(l, 4), Comment: lineComment(l)}, nil
}

// upgradeRows reads a CastleSiege.dat upgrade section (index 0..2).
func upgradeRows(dst *[]gamedata.CastleUpgradeRow) func([]mutext.Line) error {
	return func(rows []mutext.Line) error {
		for _, l := range rows {
			r, err := rowOf[gamedata.CastleUpgradeRow](l)
			if err != nil {
				return err
			}
			if r.Index < 0 || r.Index > 2 {
				return fmt.Errorf("line %d: index %d", l.No, r.Index)
			}
			*dst = append(*dst, r)
		}
		return nil
	}
}
