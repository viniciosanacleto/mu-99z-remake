package gamedata

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/config/mutext"
)

// MaxMonsterClass is the size of the CMonsterManager table (FUN_004ba3b0
// ignores indexes >= 1000).
const MaxMonsterClass = 1000

// MonsterInfo is one Data/Monster/MonsterList.txt row (CMonsterManager
// entry of 0x9c bytes, FUN_004ba0c0). Offsets are those of the original
// struct.
type MonsterInfo struct {
	Index        int    // +0x00
	Type         int    // +0x04: 0 monster (obj type 2), otherwise NPC (3)
	Name         string // +0x08
	Level        int    // +0x28
	MaxLife      int    // +0x2c (scaled by MonsterMaxLifeRate)
	Life         int    // +0x30 (= scaled MaxLife)
	MaxMana      int    // +0x34
	DamageMin    int    // +0x38 (scaled by MonsterPhysiDamageRate)
	DamageMax    int    // +0x3c (scaled by MonsterPhysiDamageRate)
	Defense      int    // +0x40 (scaled by MonsterDefenseRate)
	MagicDefense int    // +0x44
	AttackRate   int    // +0x48 (scaled by MonsterAttackSuccessRateRate)
	DefenseRate  int    // +0x4c (scaled by MonsterDefenseSuccessRateRate)
	MoveRange    int    // +0x50
	AttackRange  int    // +0x54
	AttackType   int    // +0x58
	ViewRange    int    // +0x5c
	MoveSpeed    int    // +0x60
	AttackSpeed  int    // +0x64
	RegenTime    int    // +0x68 (seconds)
	Attribute    int    // +0x6c
	ItemRate     int    // +0x70
	MoneyRate    int    // +0x74
	MaxItemLevel int    // +0x78
	Resistance   [7]int // +0x7c
	MonsterSkill int    // +0x98
}

// MonsterRates are the CServerInfo percentages FUN_004ba3b0 applies while
// loading (GameServerInfo - Common.dat, offsets 0x1a0..0x1b0).
type MonsterRates struct {
	MaxLife            int // MonsterMaxLifeRate
	Defense            int // MonsterDefenseRate
	DefenseSuccessRate int // MonsterDefenseSuccessRateRate
	PhysiDamage        int // MonsterPhysiDamageRate
	AttackSuccessRate  int // MonsterAttackSuccessRateRate
}

// DefaultMonsterRates leaves the file values unchanged.
var DefaultMonsterRates = MonsterRates{100, 100, 100, 100, 100}

// monsterRate is the FUN_004ba3b0 scaling: v*rate/100 in 64 bits, capped at
// 2100000000 when the product does not fit.
func monsterRate(v, rate int) int {
	r := int64(v) * int64(rate) / 100
	if r > 0x7d2b7500 || r < 0 {
		return 2100000000
	}
	return int(r)
}

// LoadMonsterList reads Data/Monster/MonsterList.txt (FUN_004ba0c0: 31
// columns, rows until "end"; index >= 1000 is ignored by FUN_004ba3b0).
func LoadMonsterList(dataDir string, rates MonsterRates) (map[int]*MonsterInfo, error) {
	lines, err := mutext.LoadFile(filepath.Join(dataDir, "Monster", "MonsterList.txt"))
	if err != nil {
		return nil, err
	}
	out := map[int]*MonsterInfo{}
	for _, l := range mutext.Table(lines) {
		if len(l.Tokens) < 31 {
			return nil, fmt.Errorf("MonsterList.txt line %d: %d columns", l.No, len(l.Tokens))
		}
		v := make([]int, 31)
		for i := range v {
			if i == 2 {
				continue
			}
			if v[i], err = l.Int(i); err != nil {
				return nil, fmt.Errorf("MonsterList.txt: %w", err)
			}
		}
		m := &MonsterInfo{Index: v[0], Type: v[1], Name: l.Tokens[2], Level: v[3], MaxLife: v[4],
			MaxMana: v[5], DamageMin: v[6], DamageMax: v[7], Defense: v[8], MagicDefense: v[9],
			AttackRate: v[10], DefenseRate: v[11], MoveRange: v[12], AttackType: v[13],
			AttackRange: v[14], ViewRange: v[15], MoveSpeed: v[16], AttackSpeed: v[17],
			RegenTime: v[18], Attribute: v[19], ItemRate: v[20], MoneyRate: v[21],
			MaxItemLevel: v[22], MonsterSkill: v[23]}
		copy(m.Resistance[:], v[24:31])
		if m.Index < 0 || m.Index >= MaxMonsterClass {
			continue
		}
		m.MaxLife = monsterRate(m.MaxLife, rates.MaxLife)
		m.Defense = monsterRate(m.Defense, rates.Defense)
		m.DefenseRate = monsterRate(m.DefenseRate, rates.DefenseSuccessRate)
		m.DamageMin = monsterRate(m.DamageMin, rates.PhysiDamage)
		m.DamageMax = monsterRate(m.DamageMax, rates.PhysiDamage)
		m.AttackRate = monsterRate(m.AttackRate, rates.AttackSuccessRate)
		m.Life = m.MaxLife
		out[m.Index] = m
	}
	return out, nil
}

// Spawn sections of Data/Monster/Spawn/*.txt (CMonsterSetBase).
const (
	SpawnNPC      = 0 // fixed position
	SpawnArea     = 1 // random cell inside (X,Y)-(TX,TY), one record per Quantity
	SpawnSingle   = 2 // fixed position +-3 at load, random cell in a 6x6 box at spawn
	SpawnInvasion = 3 // not spawned at start-up (invasion manager)
	SpawnEvent    = 4 // not spawned at start-up (event managers)
	spawnSkipped  = 5 // section 5 is read and discarded by FUN_004bbc70
)

// MaxSpawnRecords is the CMonsterSetBase capacity (FUN_004bbf10).
const MaxSpawnRecords = 20000

// SpawnRecord is one 0x24-byte CMonsterSetBase entry.
type SpawnRecord struct {
	Section int
	Class   int
	Map     int
	Dis     int // SpawnRadius / Range column
	X, Y    int
	Dir     int
	TX, TY  int // section 1 only
}

// LoadSpawns reads every Data/Monster/Spawn/"NNN - Name.txt" whose map is
// accepted (FUN_004bbaa0: three digits, " - ", map known to MapManager),
// in file-name order. rnd returns a non-negative random int (the original
// Mersenne generator FUN_005138d0); it decides random directions (-1 =>
// rand&7, FUN_004bbf10) and the +-3 offset of section 2 (FUN_004bbc70).
func LoadSpawns(dataDir string, mapOK func(int) bool, rnd func() int) ([]SpawnRecord, error) {
	dir := filepath.Join(dataDir, "Monster", "Spawn")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		n := e.Name()
		if e.IsDir() || len(n) < 6 || !isDigit(n[0]) || !isDigit(n[1]) || !isDigit(n[2]) ||
			n[3] != ' ' || n[4] != '-' || n[5] != ' ' {
			continue
		}
		names = append(names, n)
	}
	sort.Strings(names)
	var out []SpawnRecord
	for _, n := range names {
		mp, _ := strconv.Atoi(n[:3])
		if mapOK != nil && !mapOK(mp) {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, n))
		if err != nil {
			return nil, err
		}
		if out, err = parseSpawnFile(data, mp, rnd, out); err != nil {
			return nil, fmt.Errorf("Spawn/%s: %w", n, err)
		}
	}
	return out, nil
}

func isDigit(b byte) bool { return b >= '0' && b <= '9' }

// parseSpawnFile reproduces the token-stream reader of FUN_004bbc70: every
// row of sections 0, 2, 3 and 4 is five numbers (section 3 rows written with
// more columns are therefore re-read as further records, exactly like the
// original), section 1 rows are eight numbers and expand to Quantity
// records, and section 5 is skipped.
func parseSpawnFile(data []byte, mp int, rnd func() int, out []SpawnRecord) ([]SpawnRecord, error) {
	lines, err := mutext.Tokenize(data)
	if err != nil {
		return nil, err
	}
	var toks []string
	for _, l := range lines {
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
	for {
		t, ok := next()
		if !ok {
			return out, nil
		}
		section := tokenNumber(t)
		for {
			t, ok = next()
			if !ok {
				return out, nil
			}
			if strings.EqualFold(t, "end") {
				break
			}
			if section == spawnSkipped {
				continue
			}
			r := SpawnRecord{Section: section, Class: tokenNumber(t), Map: mp}
			r.Dis = num()
			r.X = num()
			r.Y = num()
			switch section {
			case SpawnArea:
				r.TX = num()
				r.TY = num()
			case SpawnSingle:
				r.X += -3 + rnd()%7
				r.Y += -3 + rnd()%7
			}
			r.Dir = num()
			count := 1
			if section == SpawnArea {
				count = num()
			}
			for i := 0; i < count; i++ {
				if len(out) >= MaxSpawnRecords {
					break
				}
				rec := r
				if rec.Dir == -1 {
					rec.Dir = rnd() & 7
				}
				out = append(out, rec)
			}
		}
	}
}

// tokenNumber converts a token the way the original tokenizer does (numbers
// are read as float and truncated, FUN_00534ff0); non-numbers read as 0.
func tokenNumber(t string) int {
	f, err := strconv.ParseFloat(t, 64)
	if err != nil {
		return 0
	}
	return int(f)
}

// NpcTalkEntry is one Data/Monster/NpcTalk.txt row (FUN_004c12e0):
// NPCIndex Map CordX CordY Message ("*" = any, stored as -1).
type NpcTalkEntry struct {
	Class   int
	Map     int
	X, Y    int
	Message string // at most 59 bytes are sent (FUN_004d9c60)
}

// LoadNpcTalk reads Data/Monster/NpcTalk.txt.
func LoadNpcTalk(dataDir string) ([]NpcTalkEntry, error) {
	lines, err := mutext.LoadFile(filepath.Join(dataDir, "Monster", "NpcTalk.txt"))
	if err != nil {
		return nil, err
	}
	var out []NpcTalkEntry
	for _, l := range mutext.Table(lines) {
		if len(l.Tokens) < 5 {
			return nil, fmt.Errorf("NpcTalk.txt line %d: %d columns", l.No, len(l.Tokens))
		}
		var v [4]int
		for i := range v {
			if v[i], err = l.Int(i); err != nil {
				return nil, fmt.Errorf("NpcTalk.txt: %w", err)
			}
		}
		out = append(out, NpcTalkEntry{Class: v[0], Map: v[1], X: v[2], Y: v[3], Message: l.Tokens[4]})
	}
	return out, nil
}
