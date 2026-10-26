package legacy

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/config/mutext"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/config/tomlconf"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"
)

func init() {
	register("monster", monsterGroup)
	register("spawn", spawnGroup)
}

// monsterGroup converts Data/Monster/*.txt and Data/Monster/Skill/*.txt.
func monsterGroup(dataDir string) ([]byte, error) {
	var c gamedata.MonsterConfig
	steps := []step{
		{"Monster/MonsterList.txt", func(l []mutext.Line) (err error) {
			c.MonsterList.Rows, err = Table[gamedata.MonsterListRow](l, true)
			return err
		}},
		{"Monster/MonsterMove.txt", func(l []mutext.Line) error {
			recs, comments := streamComments(mutext.Table(l), 6)
			for i, v := range recs {
				c.MonsterMove.Rows = append(c.MonsterMove.Rows, gamedata.MonsterMoveConfigRow{Index: v[0], MapNumber: v[1],
					MapPosX: v[2], MapPosY: v[3], Direction: v[4], KeepTime: v[5], Comment: comments[i]})
			}
			return nil
		}},
		{"Monster/MonsterSchedule.txt", func(l []mutext.Line) error {
			secs, err := mutext.Sections(l)
			if err != nil {
				return err
			}
			t := &c.MonsterSchedule
			for _, s := range secs {
				switch s.Index {
				case 0:
					recs, comments := streamComments(s.Rows, 8)
					for i, v := range recs {
						t.Schedule = append(t.Schedule, gamedata.MonsterScheduleTimeRow{Group: v[0], StartYear: v[1],
							StartMonth: v[2], StartDay: v[3], StartDoW: v[4], StartHour: v[5], StartMin: v[6], StartSec: v[7],
							Comment: comments[i]})
					}
				case 1:
					recs, comments := streamComments(s.Rows, 5)
					for i, v := range recs {
						t.Message = append(t.Message, gamedata.MonsterScheduleMessageRow{Group: v[0], EnableMessage: v[1],
							DisableMessage: v[2], ItsNotTimeMessage: v[3], EnableNpcTime: v[4], Comment: comments[i]})
					}
				case 2:
					recs, comments := streamComments(s.Rows, 5)
					for i, v := range recs {
						t.Monster = append(t.Monster, gamedata.MonsterScheduleMonsterRow{Group: v[0], MonsterIndex: v[1],
							MapNumber: v[2], LocationX: v[3], LocationY: v[4], Comment: comments[i]})
					}
				}
			}
			return nil
		}},
		{"Monster/NpcTalk.txt", func(l []mutext.Line) (err error) {
			c.NpcTalk.Rows, err = Table[gamedata.NpcTalkRow](l, true)
			return err
		}},
		{"Monster/Skill/MonsterSkill.txt", func(l []mutext.Line) error {
			rows, err := section0(l)
			for _, r := range rows {
				row := gamedata.MonsterSkillRow{Monster: zeroNum(r, 0), Name: tok(r, 1), Comment: lineComment(r)}
				for i := range row.Skill {
					row.Skill[i] = [2]int{zeroNum(r, 2+2*i), zeroNum(r, 3+2*i)}
				}
				c.MonsterSkill.Rows = append(c.MonsterSkill.Rows, row)
			}
			return err
		}},
		{"Monster/Skill/MonsterSkillElement.txt", func(l []mutext.Line) error {
			rows, err := section0(l)
			for _, r := range rows {
				c.MonsterSkillElement.Rows = append(c.MonsterSkillElement.Rows, gamedata.MonsterSkillElementRow{
					Index: zeroNum(r, 0), Name: tok(r, 1), Type: zeroNum(r, 2), SuccessRate: zeroNum(r, 3),
					ContinuanceTime: zeroNum(r, 4), IncAndDecType: zeroNum(r, 5), IncAndDecValue: zeroNum(r, 6),
					NullifiedMagic: zeroNum(r, 7), CharacterClass: zeroNum(r, 8), CharacterLevel: zeroNum(r, 9),
					Comment: lineComment(r)})
			}
			return err
		}},
		{"Monster/Skill/MonsterSkillUnit.txt", func(l []mutext.Line) error {
			rows, err := section0(l)
			for _, r := range rows {
				row := gamedata.MonsterSkillUnitRow{Index: zeroNum(r, 0), Name: tok(r, 1), TargetType: zeroNum(r, 2),
					ScopeType: zeroNum(r, 3), ScopeValue: zeroNum(r, 4), Delay: zeroNum(r, 5), Comment: lineComment(r)}
				for i := range row.Slot {
					row.Slot[i] = zeroNum(r, 6+i)
				}
				c.MonsterSkillUnit.Rows = append(c.MonsterSkillUnit.Rows, row)
			}
			return err
		}},
	}
	srcs, err := runSteps(dataDir, steps)
	if err != nil {
		return nil, err
	}
	return marshal(&c, srcs...)
}

// section0 returns the rows of every section numbered 0 (the monster skill
// readers skip the other sections).
func section0(lines []mutext.Line) ([]mutext.Line, error) {
	secs, err := mutext.Sections(lines)
	if err != nil {
		return nil, err
	}
	var out []mutext.Line
	for _, s := range secs {
		if s.Index == 0 {
			out = append(out, s.Rows...)
		}
	}
	return out, nil
}

// zeroNum is token i as a number ("*" = -1), 0 when missing or not a number.
func zeroNum(l mutext.Line, i int) int {
	if i >= len(l.Tokens) {
		return 0
	}
	v, err := l.Int(i)
	if err != nil {
		return 0
	}
	return v
}

// tok is token i in UTF-8, "" when missing.
func tok(l mutext.Line, i int) string {
	if i >= len(l.Tokens) {
		return ""
	}
	return tomlconf.UTF8(l.Tokens[i])
}

// streamComments is mutext.StreamRows with the comment of the line each
// record starts on.
func streamComments(rows []mutext.Line, n int) ([][]int, []string) {
	var f []string
	var owner []string
	for _, l := range rows {
		fs := mutext.NumFields(l)
		for range fs {
			owner = append(owner, lineComment(l))
		}
		f = append(f, fs...)
	}
	var recs [][]int
	var comments []string
	for i := 0; i < len(f); i += n {
		recs = append(recs, mutext.NumRow(f, i, n))
		comments = append(comments, owner[i])
	}
	return recs, comments
}

// spawnGroup converts the Data/Monster/Spawn/"NNN - Name.txt" files.
func spawnGroup(dataDir string) ([]byte, error) {
	dir := filepath.Join(dataDir, "Monster", "Spawn")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	c := gamedata.SpawnConfig{Spawn: map[string]gamedata.SpawnFile{}}
	var srcs []string
	for _, e := range entries {
		name := strings.TrimSuffix(e.Name(), ".txt")
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".txt") || !gamedata.IsSpawnFileName(name) {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		rows, err := spawnRows(b)
		if err != nil {
			return nil, fmt.Errorf("Spawn/%s: %w", e.Name(), err)
		}
		c.Spawn[tomlconf.UTF8(name)] = gamedata.SpawnFile{Rows: rows}
		srcs = append(srcs, "Data/Monster/Spawn/"+e.Name())
	}
	return marshal(&c, srcs...)
}

// spawnRows is the token-stream reader of FUN_004bbc70 without its random
// parts: each section number is followed by records until "end"; a record
// is five numbers (Class Range X Y Dir), eight in section 1 (Class Range X
// Y TX TY Dir Quantity). Section 5 is read and discarded. Rows written with
// more or fewer columns are re-cut by the stream, like the original.
func spawnRows(data []byte) ([]gamedata.SpawnRow, error) {
	lines, err := mutext.Tokenize(data)
	if err != nil {
		return nil, err
	}
	var toks, owner []string
	for _, l := range lines {
		for range l.Tokens {
			owner = append(owner, lineComment(l))
		}
		toks = append(toks, l.Tokens...)
	}
	pos := 0
	next := func() (string, bool) {
		if pos >= len(toks) {
			return "", false
		}
		pos++
		return toks[pos-1], true
	}
	num := func() int {
		t, _ := next()
		return tokenNumber(t)
	}
	var out []gamedata.SpawnRow
	for {
		t, ok := next()
		if !ok {
			return out, nil
		}
		section := tokenNumber(t)
		for {
			start := pos
			t, ok = next()
			if !ok {
				return out, nil
			}
			if strings.EqualFold(t, "end") {
				break
			}
			if section == 5 {
				continue
			}
			r := gamedata.SpawnRow{Section: section, MonsterIndex: tokenNumber(t), Comment: owner[start]}
			r.SpawnRadius = num()
			r.LocationX = num()
			r.LocationY = num()
			if section == gamedata.SpawnArea {
				r.LocationTX = num()
				r.LocationTY = num()
			}
			r.Direction = num()
			if section == gamedata.SpawnArea {
				r.Quantity = num()
			}
			out = append(out, r)
		}
	}
}

// tokenNumber converts a token the way the spawn reader does (numbers are
// read as float and truncated, FUN_00534ff0); non-numbers read as 0.
func tokenNumber(t string) int {
	f, err := strconv.ParseFloat(t, 64)
	if err != nil {
		return 0
	}
	return int(f)
}
