package gamedata

import (
	"fmt"
	"sort"
	"strconv"
)

// MaxMonsterClass is the size of the CMonsterManager table (FUN_004ba3b0
// ignores indexes >= 1000).
const MaxMonsterClass = 1000

// MonsterInfo is one MonsterList.txt row (CMonsterManager
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

// LoadMonsterList reads the MonsterList table of config/data/monster.toml
// (FUN_004ba0c0; an index >= 1000 is ignored by FUN_004ba3b0) and applies
// the CServerInfo rates.
func LoadMonsterList(configDir string, rates MonsterRates) (map[int]*MonsterInfo, error) {
	f, err := LoadMonsterConfig(configDir)
	if err != nil {
		return nil, err
	}
	out := map[int]*MonsterInfo{}
	for _, r := range f.MonsterList.Rows {
		m := &MonsterInfo{Index: r.Index, Type: r.Type, Name: r.Name, Level: r.Level, MaxLife: r.MaxLife,
			MaxMana: r.MaxMana, DamageMin: r.DamageMin, DamageMax: r.DamageMax, Defense: r.Defense,
			MagicDefense: r.MagicDefense, AttackRate: r.AttackRate, DefenseRate: r.DefenseRate,
			MoveRange: r.MoveRange, AttackType: r.AttackType, AttackRange: r.AttackRange, ViewRange: r.ViewRange,
			MoveSpeed: r.MoveSpeed, AttackSpeed: r.AttackSpeed, RegenTime: r.RegenTime, Attribute: r.Attribute,
			ItemRate: r.ItemRate, MoneyRate: r.MoneyRate, MaxItemLevel: r.MaxItemLevel, MonsterSkill: r.MonsterSkill,
			Resistance: [7]int{r.Resistance1, r.Resistance2, r.Resistance3, r.Resistance4, r.Resistance5,
				r.Resistance6, r.Resistance7}}
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

// Spawn sections of the Monster/Spawn files (CMonsterSetBase).
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

// LoadSpawns reads config/data/spawn.toml: the spawn files ("NNN - Name",
// NNN the map) whose map is accepted, in file-name order (FUN_004bbaa0).
// rnd returns a non-negative random int (the original Mersenne generator
// FUN_005138d0); it decides random directions (-1 => rand&7, FUN_004bbf10)
// and the +-3 offset of section 2 (FUN_004bbc70).
func LoadSpawns(configDir string, mapOK func(int) bool, rnd func() int) ([]SpawnRecord, error) {
	f, err := loadSpawnConfig(configDir)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(f.Spawn))
	for n := range f.Spawn {
		if !IsSpawnFileName(n) {
			return nil, fmt.Errorf("Spawn.%q: want \"NNN - Name\" (NNN = map)", n)
		}
		names = append(names, n)
	}
	// The original sorts the file names, ".txt" included.
	sort.Slice(names, func(i, j int) bool { return names[i]+".txt" < names[j]+".txt" })
	var out []SpawnRecord
	for _, n := range names {
		mp, _ := strconv.Atoi(n[:3])
		if mapOK != nil && !mapOK(mp) {
			continue
		}
		out = buildSpawns(f.Spawn[n].Rows, mp, rnd, out)
	}
	return out, nil
}

// IsSpawnFileName is the FUN_004bbaa0 file name test: three digits and " - ".
func IsSpawnFileName(n string) bool {
	return len(n) >= 6 && isDigit(n[0]) && isDigit(n[1]) && isDigit(n[2]) && n[3] == ' ' && n[4] == '-' && n[5] == ' '
}

func isDigit(b byte) bool { return b >= '0' && b <= '9' }

// buildSpawns turns the rows of one file into records like FUN_004bbc70:
// section 1 rows expand to Quantity records, section 2 positions move by
// +-3 and Direction -1 is random; section 5 rows are not records.
func buildSpawns(rows []SpawnRow, mp int, rnd func() int, out []SpawnRecord) []SpawnRecord {
	for _, row := range rows {
		if row.Section == spawnSkipped {
			continue
		}
		r := SpawnRecord{Section: row.Section, Class: row.MonsterIndex, Map: mp, Dis: row.SpawnRadius,
			X: row.LocationX, Y: row.LocationY, Dir: row.Direction}
		count := 1
		switch row.Section {
		case SpawnArea:
			r.TX, r.TY = row.LocationTX, row.LocationTY
			count = row.Quantity
		case SpawnSingle:
			r.X += -3 + rnd()%7
			r.Y += -3 + rnd()%7
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
	return out
}

// NpcTalkEntry is one NpcTalk.txt row (FUN_004c12e0):
// NPCIndex Map CordX CordY Message ("*" = any, stored as -1).
type NpcTalkEntry struct {
	Class   int
	Map     int
	X, Y    int
	Message string // at most 59 bytes are sent (FUN_004d9c60)
}

// LoadNpcTalk reads the NpcTalk table of config/data/monster.toml.
func LoadNpcTalk(configDir string) ([]NpcTalkEntry, error) {
	f, err := LoadMonsterConfig(configDir)
	if err != nil {
		return nil, err
	}
	var out []NpcTalkEntry
	for _, r := range f.NpcTalk.Rows {
		out = append(out, NpcTalkEntry{Class: r.NPCIndex, Map: r.Map, X: r.CordX, Y: r.CordY, Message: r.Message})
	}
	return out, nil
}
