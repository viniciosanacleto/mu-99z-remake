package gamedata

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/config/ini"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/config/mutext"
)

// The remaining emulator world customs of Data/Custom (block 12, doc
// 10f): CustomArena, CustomEventDrop, CustomLukeHelper, CustomHealthBar,
// CustomMonsterKillCount, SummonScroll, CreationGold, DailyReward and
// CustomPatente. The sectioned files are read by the standard reader
// (FUN_004e0cb0 / FUN_004e0d70: numbered sections closed by "end",
// "*" = -1).

func customSections(dataDir, name string) (map[int][]mutext.Line, error) {
	lines, err := mutext.LoadFile(filepath.Join(dataDir, "Custom", name))
	if err != nil {
		return nil, err
	}
	secs, err := mutext.Sections(lines)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	out := map[int][]mutext.Line{}
	for _, s := range secs {
		out[s.Index] = append(out[s.Index], s.Rows...)
	}
	return out, nil
}

func scheduleOf(l mutext.Line, first int) (Schedule, error) {
	v, err := lineInts(l, []int{first, first + 1, first + 2, first + 3, first + 4, first + 5, first + 6})
	if err != nil {
		return Schedule{}, err
	}
	return Schedule{v[0], v[1], v[2], v[3], v[4], v[5], v[6]}, nil
}

// --- CustomArena --------------------------------------------------------------------------

// CustomArenaCount is the number of arenas (CCustomArena, 30 entries of
// 0xb9c bytes).
const CustomArenaCount = 30

// CustomArenaRule is a section-1 row of CustomArena.txt (FUN_00443040).
// MinMasterLevel / MaxMasterLevel (entry +0x6c / +0x70) are not in the
// file: the loader sets them to 0.
type CustomArenaRule struct {
	Name                            string // 32 bytes
	AlarmTime, StandTime, EventTime int    // minutes
	CloseTime                       int    // minutes
	StartGate, FinalGate            int
	MinUser, MaxUser, MaxDeath      int
	MinLevel, MaxLevel              int
	MinMasterLevel, MaxMasterLevel  int
	MinReset, MaxReset              int
	MinMasterReset, MaxMasterReset  int
	RewardBag                       int
	Class                           [5]int // DW DK FE MG DL: 0 = refused, n = allowed from change-up n-1
	Loaded                          bool
}

// CustomArena holds CustomArena.txt: the start times and the rule of each
// arena index.
type CustomArena struct {
	Schedule [CustomArenaCount][]Schedule
	Rules    [CustomArenaCount]CustomArenaRule
}

// LoadCustomArena reads Custom/CustomArena.txt. Rows with an index out of
// 0..29 are ignored.
func LoadCustomArena(dataDir string) (*CustomArena, error) {
	secs, err := customSections(dataDir, "CustomArena.txt")
	if err != nil {
		return nil, err
	}
	a := &CustomArena{}
	for _, l := range secs[0] {
		idx, err := l.Int(0)
		if err != nil {
			return nil, err
		}
		s, err := scheduleOf(l, 1)
		if err != nil {
			return nil, err
		}
		if idx >= 0 && idx < CustomArenaCount {
			a.Schedule[idx] = append(a.Schedule[idx], s)
		}
	}
	for _, l := range secs[1] {
		if len(l.Tokens) < 23 {
			return nil, fmt.Errorf("CustomArena.txt line %d: %d columns", l.No, len(l.Tokens))
		}
		v, err := lineInts(l, []int{0, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22})
		if err != nil {
			return nil, err
		}
		if v[0] < 0 || v[0] >= CustomArenaCount {
			continue
		}
		name := l.Tokens[1]
		if len(name) > 0x1F {
			name = name[:0x1F]
		}
		a.Rules[v[0]] = CustomArenaRule{Name: name, AlarmTime: v[1], StandTime: v[2], EventTime: v[3],
			CloseTime: v[4], StartGate: v[5], FinalGate: v[6], MinUser: v[7], MaxUser: v[8], MaxDeath: v[9],
			MinLevel: v[10], MaxLevel: v[11], MinReset: v[12], MaxReset: v[13], MinMasterReset: v[14],
			MaxMasterReset: v[15], RewardBag: v[16], Class: [5]int{v[17], v[18], v[19], v[20], v[21]}, Loaded: true}
	}
	return a, nil
}

// --- CustomEventDrop ----------------------------------------------------------------------

// CustomEventDropCount is the number of drop events (CCustomEventDrop, 10
// entries of 0x74 bytes).
const CustomEventDropCount = 10

// CustomEventDropRule is a section-1 row of CustomEventDrop.txt.
type CustomEventDropRule struct {
	Name                 string // 32 bytes
	Map, X, Y, Radius    int
	AlarmTime, EventTime int // minutes
}

// CustomEventDropItem is a section-2 row: Count items dropped DropDelay
// seconds after the start.
type CustomEventDropItem struct {
	Index, Level, Count, Delay int
}

// CustomEventDrop holds CustomEventDrop.txt (FUN_004623e0).
type CustomEventDrop struct {
	Schedule [CustomEventDropCount][]Schedule
	Rules    [CustomEventDropCount]CustomEventDropRule
	Items    [CustomEventDropCount][]CustomEventDropItem
}

// LoadCustomEventDrop reads Custom/CustomEventDrop.txt. An item reference
// outside 0..0x1FFF becomes -1 (the loader's range test).
func LoadCustomEventDrop(dataDir string) (*CustomEventDrop, error) {
	secs, err := customSections(dataDir, "CustomEventDrop.txt")
	if err != nil {
		return nil, err
	}
	d := &CustomEventDrop{}
	ok := func(i int) bool { return i >= 0 && i < CustomEventDropCount }
	for _, l := range secs[0] {
		idx, err := l.Int(0)
		if err != nil {
			return nil, err
		}
		s, err := scheduleOf(l, 1)
		if err != nil {
			return nil, err
		}
		if ok(idx) {
			d.Schedule[idx] = append(d.Schedule[idx], s)
		}
	}
	for _, l := range secs[1] {
		if len(l.Tokens) < 8 {
			return nil, fmt.Errorf("CustomEventDrop.txt line %d: %d columns", l.No, len(l.Tokens))
		}
		v, err := lineInts(l, []int{0, 2, 3, 4, 5, 6, 7})
		if err != nil {
			return nil, err
		}
		if !ok(v[0]) {
			continue
		}
		name := l.Tokens[1]
		if len(name) > 0x1F {
			name = name[:0x1F]
		}
		d.Rules[v[0]] = CustomEventDropRule{Name: name, Map: v[1], X: v[2], Y: v[3], Radius: v[4], AlarmTime: v[5],
			EventTime: v[6]}
	}
	for _, l := range secs[2] {
		if len(l.Tokens) < 5 {
			return nil, fmt.Errorf("CustomEventDrop.txt line %d: %d columns", l.No, len(l.Tokens))
		}
		v, err := lineInts(l, []int{0, 2, 3, 4})
		if err != nil {
			return nil, err
		}
		item, err := parseShopIndex(l.Tokens[1])
		if err != nil {
			return nil, err
		}
		if ok(v[0]) {
			d.Items[v[0]] = append(d.Items[v[0]], CustomEventDropItem{Index: item, Level: v[1], Count: v[2], Delay: v[3]})
		}
	}
	return d, nil
}

// --- CustomLukeHelper ---------------------------------------------------------------------

// LukeSpot is a section-1 row of CustomLukeHelper.txt: where the helper
// may appear and the bag of its reward. Key is the row number (the
// std::map key, counter +0x30).
type LukeSpot struct {
	Key                        int
	Map, Class                 int
	StartX, StartY, EndX, EndY int
	Bag                        int
}

// CustomLukeHelper holds CustomLukeHelper.txt (FUN_00467550).
type CustomLukeHelper struct {
	EventTime int // minutes (+0x10)
	Spots     []LukeSpot
	Schedule  []Schedule
}

// LoadCustomLukeHelper reads Custom/CustomLukeHelper.txt.
func LoadCustomLukeHelper(dataDir string) (*CustomLukeHelper, error) {
	secs, err := customSections(dataDir, "CustomLukeHelper.txt")
	if err != nil {
		return nil, err
	}
	h := &CustomLukeHelper{}
	for _, l := range secs[0] {
		if h.EventTime, err = l.Int(0); err != nil {
			return nil, err
		}
	}
	for _, l := range secs[1] {
		v, err := lineInts(l, []int{0, 1, 2, 3, 4, 5, 6})
		if err != nil {
			return nil, err
		}
		h.Spots = append(h.Spots, LukeSpot{Key: len(h.Spots), Map: v[0], Class: v[1], StartX: v[2], StartY: v[3],
			EndX: v[4], EndY: v[5], Bag: v[6]})
	}
	for _, l := range secs[2] {
		s, err := scheduleOf(l, 0)
		if err != nil {
			return nil, err
		}
		h.Schedule = append(h.Schedule, s)
	}
	return h, nil
}

// --- CustomHealthBar ----------------------------------------------------------------------

// HealthBarRow is a CustomHealthBar.txt row (FUN_0044c3b0): -1 matches any
// monster class / map.
type HealthBarRow struct {
	Class, Map, Switch int
}

// LoadCustomHealthBar reads Custom/CustomHealthBar.txt (one table).
func LoadCustomHealthBar(dataDir string) ([]HealthBarRow, error) {
	rows, err := customTable(dataDir, "CustomHealthBar.txt")
	if err != nil {
		return nil, err
	}
	out := make([]HealthBarRow, 0, len(rows))
	for _, l := range rows {
		v, err := lineInts(l, []int{0, 1, 2})
		if err != nil {
			return nil, err
		}
		out = append(out, HealthBarRow{Class: v[0], Map: v[1], Switch: v[2]})
	}
	return out, nil
}

// HealthBarSwitch is FUN_0044c500: the switch of the first row matching
// the class and the map, 0 when none does.
func HealthBarSwitch(rows []HealthBarRow, class, mp int) int {
	for _, r := range rows {
		if (r.Class == -1 || r.Class == class) && (r.Map == -1 || r.Map == mp) {
			return r.Switch
		}
	}
	return 0
}

// --- CustomMonsterKillCount ---------------------------------------------------------------

// KillCountItem is a section-1 row of CustomMonsterKillCount.txt.
type KillCountItem struct {
	Index, Level, Durability int
	Skill, Luck, Option      int
	Excellent, SetOption     int
	Duration                 int // seconds
}

// KillCountRow is a section-0 row (a std::map keyed by DBIndex, the first
// row of a key wins). The three coin fields of the entry (+0x30..+0x38)
// are not read by the loader and stay 0.
type KillCountRow struct {
	DBIndex, Monster int
	AL               [4]int
	KillCount        int
	Money            int
	Message          string // 128 bytes
	Items            []KillCountItem
}

// LoadCustomMonsterKillCount reads Custom/CustomMonsterKillCount.txt
// (FUN_004b8920), sorted by DBIndex. Item rows of an unknown DBIndex are
// dropped.
func LoadCustomMonsterKillCount(dataDir string) ([]KillCountRow, error) {
	secs, err := customSections(dataDir, "CustomMonsterKillCount.txt")
	if err != nil {
		return nil, err
	}
	byKey := map[int]*KillCountRow{}
	var out []*KillCountRow
	for _, l := range secs[0] {
		v, err := lineInts(l, []int{0, 1, 2, 3, 4, 5, 6, 7})
		if err != nil {
			return nil, err
		}
		if byKey[v[0]] != nil {
			continue
		}
		msg, _ := l.String(8)
		if len(msg) > 0x7F {
			msg = msg[:0x7F]
		}
		r := &KillCountRow{DBIndex: v[0], Monster: v[1], AL: [4]int{v[2], v[3], v[4], v[5]}, KillCount: v[6],
			Money: v[7], Message: msg}
		byKey[v[0]] = r
		out = append(out, r)
	}
	for _, l := range secs[1] {
		v, err := lineInts(l, []int{0, 2, 3, 4, 5, 6, 7, 8, 9})
		if err != nil {
			return nil, err
		}
		item, err := parseShopIndex(l.Tokens[1])
		if err != nil {
			return nil, err
		}
		if r := byKey[v[0]]; r != nil {
			r.Items = append(r.Items, KillCountItem{Index: item, Level: v[1], Durability: v[2], Skill: v[3], Luck: v[4],
				Option: v[5], Excellent: v[6], SetOption: v[7], Duration: v[8]})
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].DBIndex < out[j].DBIndex })
	rows := make([]KillCountRow, len(out))
	for i, r := range out {
		rows[i] = *r
	}
	return rows, nil
}

// --- SummonScroll -------------------------------------------------------------------------

// SummonScrollItem is a section-0 row: the item that is a scroll.
type SummonScrollItem struct{ Index, Item int }

// SummonScrollMonster is a section-1 row: CreateRate is the weight of the
// row inside its group (0 = never).
type SummonScrollMonster struct{ Index, Group, Class, Rate int }

// SummonScroll holds SummonScroll.txt (FUN_00505d20).
type SummonScroll struct {
	Scrolls  []SummonScrollItem
	Monsters []SummonScrollMonster
}

// LoadSummonScroll reads Custom/SummonScroll.txt.
func LoadSummonScroll(dataDir string) (*SummonScroll, error) {
	secs, err := customSections(dataDir, "SummonScroll.txt")
	if err != nil {
		return nil, err
	}
	s := &SummonScroll{}
	for _, l := range secs[0] {
		idx, err := l.Int(0)
		if err != nil {
			return nil, err
		}
		tok, err := l.String(1)
		if err != nil {
			return nil, err
		}
		item, err := parseShopIndex(tok)
		if err != nil {
			return nil, err
		}
		s.Scrolls = append(s.Scrolls, SummonScrollItem{Index: idx, Item: item})
	}
	for _, l := range secs[1] {
		v, err := lineInts(l, []int{0, 1, 2, 3})
		if err != nil {
			return nil, err
		}
		s.Monsters = append(s.Monsters, SummonScrollMonster{Index: v[0], Group: v[1], Class: v[2], Rate: v[3]})
	}
	return s, nil
}

// --- CreationGold -------------------------------------------------------------------------

// CreationGoldValue is GetPrivateProfileInt("CreationGold", "GoldValue",
// 50, "Custom\CreationGold.txt"), read on every use (FUN_0049a690).
func CreationGoldValue(dataDir string) int {
	f, err := ini.Load(filepath.Join(dataDir, "Custom", "CreationGold.txt"))
	if err != nil {
		return 0x32
	}
	return f.Int("CreationGold", "GoldValue", 0x32)
}

// --- DailyReward --------------------------------------------------------------------------

// DailyRewardRow is a DailyReward.txt row (FUN_0045a200), kept in a
// std::map by day. Index is 0xFFFF when the reference is out of range.
type DailyRewardRow struct {
	Day, Index                 int
	Level, Skill, Luck, Option int
	Durability                 int
	Excellent, Ancient         int
	Duration                   int
	Info1, Info2               string // 100 bytes each
}

// LoadDailyReward reads Custom/DailyReward.txt by day (the first row of a
// day wins).
func LoadDailyReward(dataDir string) (map[int]DailyRewardRow, error) {
	secs, err := customSections(dataDir, "DailyReward.txt")
	if err != nil {
		return nil, err
	}
	out := map[int]DailyRewardRow{}
	for _, l := range secs[0] {
		if len(l.Tokens) < 12 {
			return nil, fmt.Errorf("DailyReward.txt line %d: %d columns", l.No, len(l.Tokens))
		}
		v, err := lineInts(l, []int{0, 2, 3, 4, 5, 6, 7, 8, 9})
		if err != nil {
			return nil, err
		}
		item, err := parseShopIndex(l.Tokens[1])
		if err != nil {
			return nil, err
		}
		if item < 0 {
			item = 0xFFFF
		}
		if _, dup := out[v[0]]; dup {
			continue
		}
		cut := func(s string) string {
			if len(s) > 99 {
				return s[:99]
			}
			return s
		}
		out[v[0]] = DailyRewardRow{Day: v[0], Index: item, Level: v[1], Skill: v[2], Luck: v[3], Option: v[4],
			Durability: v[5], Excellent: v[6], Ancient: v[7], Duration: v[8], Info1: cut(l.Tokens[10]),
			Info2: cut(l.Tokens[11])}
	}
	return out, nil
}

// --- CustomPatente ------------------------------------------------------------------------

// LoadCustomPatente reads Custom/CustomPatente.txt with the script reader
// (FUN_00451f90 -> FUN_00409600 / FUN_00451390): the first cell of each
// section-0 row is a patente value, its position (1-based) is the rank
// shown by C1 0C F3 E5. At most 50 values. A missing file gives an empty
// list, as in this installation.
func LoadCustomPatente(dataDir string) ([]int, error) {
	secs, err := LoadEventScript(filepath.Join(dataDir, "Custom", "CustomPatente.txt"))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []int
	for _, s := range secs {
		if s.Index != 0 {
			continue
		}
		for _, r := range s.Rows {
			if len(out) == 50 {
				break
			}
			out = append(out, atoiC(r[0]))
		}
	}
	return out, nil
}

// atoiC is the C atoi: leading blanks, sign, digits; anything else stops.
func atoiC(s string) int {
	i, n, neg := 0, 0, false
	for i < len(s) && (s[i] == ' ' || s[i] == '\t') {
		i++
	}
	if i < len(s) && (s[i] == '-' || s[i] == '+') {
		neg = s[i] == '-'
		i++
	}
	for ; i < len(s) && s[i] >= '0' && s[i] <= '9'; i++ {
		n = n*10 + int(s[i]-'0')
	}
	if neg {
		return -n
	}
	return n
}
