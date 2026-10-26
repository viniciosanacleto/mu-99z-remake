package legacy

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/config/ini"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/config/mutext"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/config/tomlconf"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"
)

func init() {
	register("custom", customGroup)
	register("custom-event", customEventGroup)
	register("xshop", xshopGroup)
}

// customGroup converts Data/Custom (without CustomXShop*, see xshopGroup).
// Crywolf.dat, CustomDailyReward.txt and CustomMix.txt are not read by the
// original and are left out.
func customGroup(dataDir string) ([]byte, error) {
	var c gamedata.CustomConfig
	steps := []step{
		{"Custom/CustomArena.txt", mergedSections(map[int]func([]mutext.Line) error{
			0: rowsInto(&c.CustomArena.Schedule), 1: rowsInto(&c.CustomArena.Rule),
		})},
		{"Custom/CustomAttack.txt", func(l []mutext.Line) error {
			secs, err := mutext.Sections(l)
			if err != nil {
				return err
			}
			for _, s := range secs {
				for _, r := range s.Rows {
					// FUN_00445480 reads eight numbers from every row.
					var v struct{ A, B, C, D, E, F, G, H int }
					if err := Row(r, &v, true); err != nil {
						return err
					}
					switch s.Index {
					case 0:
						c.CustomAttack.Speed = append(c.CustomAttack.Speed, mustRow[gamedata.CustomAttackSpeedRow](r))
					case 1:
						c.CustomAttack.Skill = append(c.CustomAttack.Skill, mustRow[gamedata.CustomAttackSkillRow](r))
					}
				}
			}
			return nil
		}},
		{"Custom/CustomAttribute.txt", func(l []mutext.Line) error {
			for _, r := range mutext.Table(l) {
				row, err := rowOf[gamedata.CustomAttributeRow](r)
				if err != nil {
					return err
				}
				row.Comment = lineComment(r)
				c.CustomAttribute.Rows = append(c.CustomAttribute.Rows, row)
			}
			return nil
		}},
		{"Custom/CustomBuyVip.txt", func(l []mutext.Line) (err error) {
			c.CustomBuyVip.Rows, err = Table[gamedata.CustomBuyVipRow](l, true)
			return err
		}},
		{"Custom/CustomDeathMessage.txt", func(l []mutext.Line) (err error) {
			c.CustomDeathMessage.Rows, err = Table[gamedata.CustomDeathMessageRow](l, true)
			return err
		}},
		{"Custom/CustomEventDrop.txt", mergedSections(map[int]func([]mutext.Line) error{
			0: rowsInto(&c.CustomEventDrop.Schedule), 1: rowsInto(&c.CustomEventDrop.Rule), 2: rowsInto(&c.CustomEventDrop.Item),
		})},
		{"Custom/CustomEvolution.txt", func(l []mutext.Line) error {
			recs, comments := streamComments(mutext.Table(l), 5)
			for i, v := range recs {
				c.CustomEvolution.Rows = append(c.CustomEvolution.Rows, gamedata.CustomEvolutionRow{Index: v[0],
					ItemTarget: gamedata.FormatItemRef(v[1], v[2]), ItemResult: gamedata.FormatItemRef(v[3], v[4]),
					Comment: comments[i]})
			}
			return nil
		}},
		{"Custom/CustomGate.txt", func(l []mutext.Line) (err error) {
			c.CustomGate.Rows, err = Table[gamedata.CustomGateConfigRow](l, true)
			return err
		}},
		{"Custom/CustomGift.txt", func(l []mutext.Line) error {
			secs, err := mutext.Sections(l)
			if err != nil {
				return err
			}
			defined := map[int]bool{}
			for _, s := range secs {
				for _, r := range s.Rows {
					switch s.Index {
					case 0:
						row, err := rowOf[gamedata.CustomGiftRow](r)
						if err != nil {
							return err
						}
						row.Comment = lineComment(r)
						defined[row.GiftIndex] = true
						c.CustomGift.Gift = append(c.CustomGift.Gift, row)
					case 1:
						row, err := rowOf[gamedata.CustomGiftItemRow](r)
						if err != nil {
							return err
						}
						// Items of a gift not read yet are lost, like in FUN_00463150.
						if defined[row.GiftIndex] {
							c.CustomGift.Item = append(c.CustomGift.Item, row)
						}
					}
				}
			}
			return nil
		}},
		{"Custom/CustomGuard.txt", monsterClasses(&c.CustomGuard)},
		{"Custom/CustomHealthBar.txt", func(l []mutext.Line) (err error) {
			c.CustomHealthBar.Rows, err = Table[gamedata.CustomHealthBarRow](l, true)
			return err
		}},
		{"Custom/CustomJewel.txt", func(l []mutext.Line) error { return customJewel(l, &c.CustomJewel) }},
		{"Custom/CustomLuckyWheel.txt", func(l []mutext.Line) (err error) {
			c.CustomLuckyWheel.Rows, err = Table[gamedata.CustomLuckyWheelRow](l, true)
			return err
		}},
		{"Custom/CustomLukeHelper.txt", mergedSections(map[int]func([]mutext.Line) error{
			0: func(rows []mutext.Line) error {
				for _, r := range rows {
					v, err := r.Int(0)
					if err != nil {
						return err
					}
					c.CustomLukeHelper.EventTime = append(c.CustomLukeHelper.EventTime, v)
				}
				return nil
			},
			1: rowsInto(&c.CustomLukeHelper.Spot), 2: rowsInto(&c.CustomLukeHelper.Schedule),
		})},
		{"Custom/CustomMarriage.txt", sections(map[int]func([]mutext.Line) error{
			0: rowsInto(&c.CustomMarriage.Marry), 1: rowsInto(&c.CustomMarriage.Divorce), 2: rowsInto(&c.CustomMarriage.Reward),
		})},
		{"Custom/CustomMobCombo.txt", monsterClasses(&c.CustomMobCombo)},
		{"Custom/CustomMonster.txt", func(l []mutext.Line) error {
			for _, r := range mutext.Table(l) {
				v := mutext.NumRow(mutext.NumFields(r), 0, 11)
				c.CustomMonster.Rows = append(c.CustomMonster.Rows, gamedata.CustomMonsterRowConfig{MonsterIndex: v[0],
					MapNumber: v[1], MaxLife: v[2], DamageMin: v[3], DamageMax: v[4], Defense: v[5], AttackRate: v[6],
					DefenseRate: v[7], ExperienceRate: v[8], KillMessage: v[9], SpecialBag: v[10], Comment: lineComment(r)})
			}
			return nil
		}},
		{"Custom/CustomMonsterKillCount.txt", mergedSections(map[int]func([]mutext.Line) error{
			0: func(rows []mutext.Line) error {
				for _, r := range rows {
					var v struct{ A, B, C, D, E, F, G, H int }
					if err := Row(r, &v, true); err != nil {
						return err
					}
					c.CustomMonsterKillCount.Kill = append(c.CustomMonsterKillCount.Kill, gamedata.CustomKillCountRow{
						DBIndex: v.A, MonsterIndex: v.B, AL0: v.C, AL1: v.D, AL2: v.E, AL3: v.F, KillCount: v.G, Money: v.H,
						Message: tok(r, 8), Comment: lineComment(r)})
				}
				return nil
			},
			1: rowsInto(&c.CustomMonsterKillCount.Item),
		})},
		{"Custom/CustomMove.txt", func(l []mutext.Line) (err error) {
			c.CustomMove.Rows, err = Table[gamedata.CustomMoveConfigRow](l, true)
			return err
		}},
		{"Custom/CustomNPCCommand.txt", func(l []mutext.Line) (err error) {
			c.CustomNPCCommand.Rows, err = Table[gamedata.CustomNPCCommandConfigRow](l, true)
			return err
		}},
		{"Custom/CustomPet.txt", func(l []mutext.Line) error {
			for _, r := range mutext.Table(l) {
				v := func(i int) int { return zeroNum(r, i) }
				c.CustomPet.Rows = append(c.CustomPet.Rows, gamedata.CustomPetRow{Index: v(0), ItemIndex: tok(r, 1),
					AddHP: v(2), AddMP: v(3), DamageRate: v(4), AttackSpeed: v(5), DoubleDmgRate: v(6), TripleDmgRate: v(7),
					AddExp: v(8), ResistDoubleDmg: v(9), ResistIgnoreDefense: v(10), ResistIgnoreSD: v(11),
					ResistCriticalDMG: v(12), ResistExceDMG: v(13), BlockStuck: v(14), ReflectRate: v(15),
					PetRecovery: v(16), Comment: lineComment(r)})
			}
			return nil
		}},
		{"Custom/CustomPick.txt", func(l []mutext.Line) (err error) {
			c.CustomPick.Rows, err = Table[gamedata.CustomPickRow](l, true)
			return err
		}},
		{"Custom/CustomQuiz.txt", sections(map[int]func([]mutext.Line) error{
			0: rowsInto(&c.CustomQuiz.Schedule), 1: rowsInto(&c.CustomQuiz.Quiz), 2: rowsInto(&c.CustomQuiz.Question),
		})},
		{"Custom/CustomRespawn.txt", func(l []mutext.Line) (err error) {
			c.CustomRespawn.Rows, err = Table[gamedata.CustomRespawnConfigRow](l, true)
			return err
		}},
		{"Custom/CustomWing.txt", func(l []mutext.Line) error {
			for _, r := range mutext.Table(l) {
				if len(r.Tokens) < 21 {
					return fmt.Errorf("line %d: %d columns", r.No, len(r.Tokens))
				}
				head := mutext.Line{No: r.No, Tokens: r.Tokens[:21]}
				row, err := rowOf[gamedata.CustomWingRow](head)
				if err != nil {
					return err
				}
				row.ModelType = zeroNum(r, 21) // read only when present, a bad value reads 0
				row.Name = tok(r, 22)
				row.Comment = lineComment(r)
				c.CustomWing.Rows = append(c.CustomWing.Rows, row)
			}
			return nil
		}},
		{"Custom/DailyReward.txt", mergedSections(map[int]func([]mutext.Line) error{
			0: rowsInto(&c.DailyReward.Rows),
		})},
		{"Custom/SummonScroll.txt", mergedSections(map[int]func([]mutext.Line) error{
			0: rowsInto(&c.SummonScroll.Scroll), 1: rowsInto(&c.SummonScroll.Monster),
		})},
	}
	srcs, err := runSteps(dataDir, steps)
	if err != nil {
		return nil, err
	}

	// Profile files (GetPrivateProfileIntA).
	f, err := ini.Load(filepath.Join(dataDir, "Custom", "CreationGold.txt"))
	if err != nil {
		return nil, err
	}
	c.CreationGold.GoldValue = f.Int("CreationGold", "GoldValue", 0x32)
	if f, err = ini.Load(filepath.Join(dataDir, "Custom", "CustomGHRS.ini")); err != nil {
		return nil, err
	}
	w := &c.CustomGHRS.WEEK
	for d, dst := range map[time.Weekday]*int{time.Monday: &w.Monday, time.Tuesday: &w.Tuesday,
		time.Wednesday: &w.Wednesday, time.Thursday: &w.Thursday, time.Friday: &w.Friday,
		time.Saturday: &w.Saturday, time.Sunday: &w.Sunday} {
		*dst = f.Int("WEEK", d.String(), 0)
	}
	t := &c.CustomGHRS.TIMEUPDATE
	t.Hour, t.Min, t.Sec = f.Int("TIMEUPDATE", "Hour", 0), f.Int("TIMEUPDATE", "Min", 0), f.Int("TIMEUPDATE", "Sec", 0)
	srcs = append(srcs, "Data/Custom/CreationGold.txt", "Data/Custom/CustomGHRS.ini")

	// CustomPatente.txt is optional (FUN_00451f90 accepts a missing file).
	c.CustomPatente.Value = []int{}
	secs, err := LoadEventScript(filepath.Join(dataDir, "Custom", "CustomPatente.txt"))
	switch {
	case errors.Is(err, fs.ErrNotExist):
	case err != nil:
		return nil, err
	default:
		for _, s := range secs {
			if s.Index != 0 {
				continue
			}
			for _, r := range s.Rows {
				c.CustomPatente.Value = append(c.CustomPatente.Value, cAtoi(r[0]))
			}
		}
		srcs = append(srcs, "Data/Custom/CustomPatente.txt")
	}
	for _, l := range []*[]int{&c.CustomGuard.MonsterClass, &c.CustomMobCombo.MonsterClass, &c.CustomLukeHelper.EventTime} {
		if *l == nil {
			*l = []int{}
		}
	}
	return marshal(&c, srcs...)
}

// mergedSections is sections for the readers that gather the rows of every
// section with the same number before reading them (customSections of the
// gamedata loaders).
func mergedSections(handlers map[int]func([]mutext.Line) error) func([]mutext.Line) error {
	return func(l []mutext.Line) error {
		secs, err := mutext.Sections(l)
		if err != nil {
			return err
		}
		merged := map[int][]mutext.Line{}
		for _, s := range secs {
			merged[s.Index] = append(merged[s.Index], s.Rows...)
		}
		for idx, h := range handlers {
			if err := h(merged[idx]); err != nil {
				return fmt.Errorf("section %d: %w", idx, err)
			}
		}
		return nil
	}
}

// mustRow decodes a row already checked with Row.
func mustRow[T any](l mutext.Line) T {
	r, _ := rowOf[T](l)
	return r
}

// monsterClasses reads FUN_0044ef50 lists (CustomGuard, CustomMobCombo):
// the first number of each row of section 0.
func monsterClasses(dst *gamedata.MonsterClassListFile) func([]mutext.Line) error {
	return func(l []mutext.Line) error {
		secs, err := mutext.Sections(l)
		if err != nil {
			return err
		}
		for _, s := range secs {
			if s.Index != 0 {
				continue
			}
			for _, r := range s.Rows {
				dst.MonsterClass = append(dst.MonsterClass, mutext.NumRow(mutext.NumFields(r), 0, 1)[0])
			}
		}
		return nil
	}
}

// customJewel reads CustomJewel.txt (FUN_00464b70): every row starts with
// JewelIndex; the item of section 0 is "cc,iii" in one token or two numbers.
func customJewel(lines []mutext.Line, j *gamedata.CustomJewelFile) error {
	secs, err := mutext.Sections(lines)
	if err != nil {
		return err
	}
	for _, s := range secs {
		for _, l := range s.Rows {
			idx, err := l.Int(0)
			if err != nil {
				return err
			}
			switch s.Index {
			case 0:
				item, n, err := jewelItem(l)
				if err != nil {
					return err
				}
				var v [8]int
				for i := range v {
					if v[i], err = l.Int(1 + n + i); err != nil {
						return err
					}
				}
				j.Jewel = append(j.Jewel, gamedata.CustomJewelRow{JewelIndex: idx, ItemIndex: item,
					EnableSlotWeapon: v[0], EnableSlotArmor: v[1], EnableSlotWing: v[2], Rate_AL0: v[3], Rate_AL1: v[4],
					Rate_AL2: v[5], Rate_AL3: v[6], SalePrice: v[7], Comment: lineComment(l)})
			case 1:
				row, err := rowOf[gamedata.CustomJewelReqRow](l)
				if err != nil {
					return err
				}
				j.Requirement = append(j.Requirement, row)
			case 2, 3:
				row, err := rowOf[gamedata.CustomJewelChangeRow](l)
				if err != nil {
					return err
				}
				if s.Index == 2 {
					j.Success = append(j.Success, row)
				} else {
					j.Failure = append(j.Failure, row)
				}
			}
		}
	}
	return nil
}

// jewelItem reads the item at token 1: "cc,iii" or "cc iii". It returns
// the reference and the number of tokens used.
func jewelItem(l mutext.Line) (string, int, error) {
	t, err := l.String(1)
	if err != nil {
		return "", 0, err
	}
	if c, x, ok := strings.Cut(t, ","); ok {
		cat, err1 := atoiStrict(c)
		idx, err2 := atoiStrict(x)
		if err1 != nil || err2 != nil {
			return "", 0, fmt.Errorf("line %d: item reference %q", l.No, t)
		}
		return gamedata.FormatItemRef(cat, idx), 1, nil
	}
	cat, err := l.Int(1)
	if err != nil {
		return "", 0, err
	}
	idx, err := l.Int(2)
	if err != nil {
		return "", 0, err
	}
	return gamedata.FormatItemRef(cat, idx), 2, nil
}

func atoiStrict(s string) (int, error) {
	l := mutext.Line{Tokens: []string{s}}
	if s == "*" {
		return 0, fmt.Errorf("not a number")
	}
	return l.Int(0)
}

// customEventGroup converts Data/Custom/Event/*.txt, filling the values the
// script reader gives to missing cells.
func customEventGroup(dataDir string) ([]byte, error) {
	var c gamedata.CustomEventConfig
	var srcs []string
	read := func(name string) (eventCells, error) {
		srcs = append(srcs, "Data/Custom/Event/"+name)
		secs, err := LoadEventScript(filepath.Join(dataDir, "Custom", "Event", name))
		return eventCells{secs}, err
	}
	starts := func(e eventCells, cols int) []gamedata.EventStartRow {
		out := []gamedata.EventStartRow{}
		for _, v := range e.rows(1, cols) {
			r := gamedata.EventStartRow{Day: v[0], Hour: v[1], Minute: v[2], Extra: -1}
			if cols > 3 {
				r.Time = v[3]
			}
			if cols > 4 {
				r.Extra = v[4]
			}
			out = append(out, r)
		}
		return out
	}
	str := func(e eventCells, sec, row, col int) string {
		s, _ := e.str(sec, row, col)
		return tomlconf.UTF8(s)
	}
	kill := func(name string) (gamedata.KillEventFile, error) {
		e, err := read(name)
		return gamedata.KillEventFile{Active: e.int(0, 0, 0), Syntax: str(e, 0, 0, 1), Patente: e.int(0, 0, 2),
			Level: e.int(0, 1, 0), Map: e.byte(0, 1, 1), X: e.byte(0, 1, 2), Y: e.byte(0, 1, 3), Start: starts(e, 5)}, err
	}
	fight := func(name string, cols int) (gamedata.FightEventFile, error) {
		e, err := read(name)
		return gamedata.FightEventFile{Active: e.int(0, 0, 0), Syntax: str(e, 0, 0, 1), Patente: e.int(0, 0, 2),
			Level: e.int(0, 1, 0), WaitMap: e.byte(0, 1, 1), WaitX: e.byte(0, 1, 2), WaitY: e.byte(0, 1, 3),
			FightMap: e.byte(0, 1, 4), FightX: e.byte(0, 1, 5), FightY: e.byte(0, 1, 6), Start: starts(e, cols)}, err
	}
	var err error
	if c.DeathEvent, err = kill("DeathEvent.txt"); err != nil {
		return nil, err
	}
	if c.DuelEvent, err = fight("DuelEvent.txt", 5); err != nil {
		return nil, err
	}
	if c.PairEvent, err = fight("PairEvent.txt", 4); err != nil {
		return nil, err
	}
	e, err := read("RaffleEvent.txt")
	if err != nil {
		return nil, err
	}
	c.RaffleEvent = gamedata.RaffleEventFile{Active: e.int(0, 0, 0), Start: starts(e, 3)}
	if e, err = read("RoletaEvent.txt"); err != nil {
		return nil, err
	}
	c.RoletaEvent = gamedata.RoletaEventFile{Active: e.int(0, 0, 0), Syntax: str(e, 0, 0, 1), Level: e.int(0, 1, 0),
		Map: e.byte(0, 1, 1), X: e.byte(0, 1, 2), Y: e.byte(0, 1, 3)}
	if e, err = read("SobreEvent.txt"); err != nil {
		return nil, err
	}
	c.SobreEvent = gamedata.SobreEventFile{Active: e.int(0, 0, 0), Syntax: str(e, 0, 0, 1), Patente: e.int(0, 0, 2),
		Level: e.int(0, 1, 0), Map: e.byte(0, 1, 1), X: e.byte(0, 1, 2), Y: e.byte(0, 1, 3), MinPlayers: e.byte(0, 1, 4),
		Start: starts(e, 4)}
	if c.TheftEvent, err = kill("TheftEvent.txt"); err != nil {
		return nil, err
	}
	return marshal(&c, srcs...)
}

// xshopGroup converts Data/Custom/CustomXShop.txt and the profile file
// CustomXShopRecharge.txt next to it.
func xshopGroup(dataDir string) ([]byte, error) {
	var c gamedata.XShopConfig
	steps := []step{{"Custom/CustomXShop.txt", sections(map[int]func([]mutext.Line) error{
		0: rowsInto(&c.CustomXShop.Category), 1: rowsInto(&c.CustomXShop.Item),
	})}}
	srcs, err := runSteps(dataDir, steps)
	if err != nil {
		return nil, err
	}
	path := filepath.Join(dataDir, "Custom", "CustomXShopRecharge.txt")
	if _, err := os.Stat(path); err == nil {
		f, err := ini.Load(path)
		if err != nil {
			return nil, err
		}
		s := func(k string) string { return tomlconf.UTF8(f.String("Recarga", k, "", 0)) }
		c.CustomXShopRecharge = gamedata.XShopRechargeFile{Ativo: f.Int("Recarga", "Ativo", 0), Titular: s("Titular"),
			ChavePix: s("ChavePix"), CopiaECola: s("CopiaECola"), ImagemURL: s("ImagemURL"), ImagemBMP: s("ImagemBMP"),
			Valor: s("Valor")}
		srcs = append(srcs, "Data/Custom/CustomXShopRecharge.txt")
	}
	return marshal(&c, srcs...)
}
