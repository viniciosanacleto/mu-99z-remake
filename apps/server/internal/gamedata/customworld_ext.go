package gamedata

import (
	"fmt"
	"sort"
)

// The remaining emulator world customs of Data/Custom (block 12, doc
// 10f, now in config/data/custom.toml): CustomArena, CustomEventDrop,
// CustomLukeHelper, CustomHealthBar, CustomMonsterKillCount, SummonScroll,
// CreationGold, DailyReward and CustomPatente.

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

// LoadCustomArena reads the CustomArena table of config/data/custom.toml.
// Rows with an index out of 0..29 are ignored.
func LoadCustomArena(configDir string) (*CustomArena, error) {
	f, err := LoadCustomConfig(configDir)
	if err != nil {
		return nil, err
	}
	a := &CustomArena{}
	for _, r := range f.CustomArena.Schedule {
		if r.EventIndex >= 0 && r.EventIndex < CustomArenaCount {
			a.Schedule[r.EventIndex] = append(a.Schedule[r.EventIndex], r.Schedule())
		}
	}
	for _, r := range f.CustomArena.Rule {
		if r.EventIndex < 0 || r.EventIndex >= CustomArenaCount {
			continue
		}
		a.Rules[r.EventIndex] = CustomArenaRule{Name: cutString(r.Name, 0x1F), AlarmTime: r.AlarmTime,
			StandTime: r.StandTime, EventTime: r.EventTime, CloseTime: r.CloseTime, StartGate: r.StartGate,
			FinalGate: r.FinalGate, MinUser: r.MinUser, MaxUser: r.MaxUser, MaxDeath: r.MaxDeath, MinLevel: r.MinLevel,
			MaxLevel: r.MaxLevel, MinReset: r.MinReset, MaxReset: r.MaxReset, MinMasterReset: r.MinMasterReset,
			MaxMasterReset: r.MaxMasterReset, RewardBag: r.RewardBagIndex, Class: [5]int{r.DW, r.DK, r.FE, r.MG, r.DL},
			Loaded: true}
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

// LoadCustomEventDrop reads the CustomEventDrop table of
// config/data/custom.toml. An item reference outside 0..0x1FFF becomes -1
// (the loader's range test).
func LoadCustomEventDrop(configDir string) (*CustomEventDrop, error) {
	f, err := LoadCustomConfig(configDir)
	if err != nil {
		return nil, err
	}
	d := &CustomEventDrop{}
	ok := func(i int) bool { return i >= 0 && i < CustomEventDropCount }
	t := f.CustomEventDrop
	for _, r := range t.Schedule {
		if ok(r.EventIndex) {
			d.Schedule[r.EventIndex] = append(d.Schedule[r.EventIndex], r.Schedule())
		}
	}
	for _, r := range t.Rule {
		if ok(r.EventIndex) {
			d.Rules[r.EventIndex] = CustomEventDropRule{Name: cutString(r.Name, 0x1F), Map: r.MapNumber, X: r.LocationX,
				Y: r.LocationY, Radius: r.DropRadius, AlarmTime: r.AlarmTime, EventTime: r.EventTime}
		}
	}
	for i, r := range t.Item {
		item, err := parseShopIndex(r.ItemIndex)
		if err != nil {
			return nil, fmt.Errorf("CustomEventDrop.Item row %d: %w", i+1, err)
		}
		if ok(r.EventIndex) {
			d.Items[r.EventIndex] = append(d.Items[r.EventIndex], CustomEventDropItem{Index: item, Level: r.ItemLevel,
				Count: r.DropCount, Delay: r.DropDelay})
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

// LoadCustomLukeHelper reads the CustomLukeHelper table of
// config/data/custom.toml.
func LoadCustomLukeHelper(configDir string) (*CustomLukeHelper, error) {
	f, err := LoadCustomConfig(configDir)
	if err != nil {
		return nil, err
	}
	t := f.CustomLukeHelper
	h := &CustomLukeHelper{}
	for _, v := range t.EventTime {
		h.EventTime = v
	}
	for _, r := range t.Spot {
		h.Spots = append(h.Spots, LukeSpot{Key: len(h.Spots), Map: r.MapNumber, Class: r.MonsterClass,
			StartX: r.RangeStartX, StartY: r.RangeStartY, EndX: r.RangeEndX, EndY: r.RangeEndY, Bag: r.BagSpecial})
	}
	for _, r := range t.Schedule {
		h.Schedule = append(h.Schedule, r.Schedule())
	}
	return h, nil
}

// --- CustomHealthBar ----------------------------------------------------------------------

// HealthBarRow is a CustomHealthBar.txt row (FUN_0044c3b0): -1 matches any
// monster class / map.
type HealthBarRow struct {
	Class, Map, Switch int
}

// LoadCustomHealthBar reads the CustomHealthBar table of
// config/data/custom.toml.
func LoadCustomHealthBar(configDir string) ([]HealthBarRow, error) {
	f, err := LoadCustomConfig(configDir)
	if err != nil {
		return nil, err
	}
	out := make([]HealthBarRow, 0, len(f.CustomHealthBar.Rows))
	for _, r := range f.CustomHealthBar.Rows {
		out = append(out, HealthBarRow{Class: r.MonsterIndex, Map: r.MapNumber, Switch: r.BarSwitch})
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

// LoadCustomMonsterKillCount reads the CustomMonsterKillCount table of
// config/data/custom.toml (FUN_004b8920), sorted by DBIndex. Item rows of an
// unknown DBIndex are dropped.
func LoadCustomMonsterKillCount(configDir string) ([]KillCountRow, error) {
	f, err := LoadCustomConfig(configDir)
	if err != nil {
		return nil, err
	}
	byKey := map[int]*KillCountRow{}
	var out []*KillCountRow
	for _, r := range f.CustomMonsterKillCount.Kill {
		if byKey[r.DBIndex] != nil {
			continue
		}
		k := &KillCountRow{DBIndex: r.DBIndex, Monster: r.MonsterIndex, AL: [4]int{r.AL0, r.AL1, r.AL2, r.AL3},
			KillCount: r.KillCount, Money: r.Money, Message: cutString(r.Message, 0x7F)}
		byKey[r.DBIndex] = k
		out = append(out, k)
	}
	for i, r := range f.CustomMonsterKillCount.Item {
		item, err := parseShopIndex(r.ItemIndex)
		if err != nil {
			return nil, fmt.Errorf("CustomMonsterKillCount.Item row %d: %w", i+1, err)
		}
		if k := byKey[r.DBIndex]; k != nil {
			k.Items = append(k.Items, KillCountItem{Index: item, Level: r.ItemLevel, Durability: r.Durability,
				Skill: r.SkillSwitch, Luck: r.LuckSwitch, Option: r.Aditional, Excellent: r.Excellent,
				SetOption: r.SetOption, Duration: r.Duration})
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

// LoadSummonScroll reads the SummonScroll table of config/data/custom.toml.
func LoadSummonScroll(configDir string) (*SummonScroll, error) {
	f, err := LoadCustomConfig(configDir)
	if err != nil {
		return nil, err
	}
	s := &SummonScroll{}
	for i, r := range f.SummonScroll.Scroll {
		item, err := parseShopIndex(r.ItemIndex)
		if err != nil {
			return nil, fmt.Errorf("SummonScroll.Scroll row %d: %w", i+1, err)
		}
		s.Scrolls = append(s.Scrolls, SummonScrollItem{Index: r.Index, Item: item})
	}
	for _, r := range f.SummonScroll.Monster {
		s.Monsters = append(s.Monsters, SummonScrollMonster{Index: r.Index, Group: r.Group, Class: r.MonsterClass,
			Rate: r.CreateRate})
	}
	return s, nil
}

// --- CreationGold -------------------------------------------------------------------------

// CreationGoldValue is GetPrivateProfileInt("CreationGold", "GoldValue",
// 50, "Custom\CreationGold.txt"), read on every use (FUN_0049a690): the
// CreationGold table of config/data/custom.toml, 50 when it cannot be read.
func CreationGoldValue(configDir string) int {
	f, err := LoadCustomConfig(configDir)
	if err != nil {
		return 0x32
	}
	return f.CreationGold.GoldValue
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

// LoadDailyReward reads the DailyReward table of config/data/custom.toml by
// day (the first row of a day wins).
func LoadDailyReward(configDir string) (map[int]DailyRewardRow, error) {
	f, err := LoadCustomConfig(configDir)
	if err != nil {
		return nil, err
	}
	out := map[int]DailyRewardRow{}
	for i, r := range f.DailyReward.Rows {
		item, err := parseShopIndex(r.Item)
		if err != nil {
			return nil, fmt.Errorf("DailyReward row %d: %w", i+1, err)
		}
		if item < 0 {
			item = 0xFFFF
		}
		if _, dup := out[r.Day]; dup {
			continue
		}
		out[r.Day] = DailyRewardRow{Day: r.Day, Index: item, Level: r.Level, Skill: r.Skill, Luck: r.Luck,
			Option: r.AddOption, Durability: r.Dur, Excellent: r.Exc, Ancient: r.Anc, Duration: r.Duration,
			Info1: cutString(r.Info1, 99), Info2: cutString(r.Info2, 99)}
	}
	return out, nil
}

// --- CustomPatente ------------------------------------------------------------------------

// LoadCustomPatente reads the CustomPatente table of config/data/custom.toml
// (FUN_00451f90): the position (1-based) of a value is the rank shown by
// C1 0C F3 E5. At most 50 values.
func LoadCustomPatente(configDir string) ([]int, error) {
	f, err := LoadCustomConfig(configDir)
	if err != nil {
		return nil, err
	}
	out := f.CustomPatente.Value
	if len(out) > 50 {
		out = out[:50]
	}
	if len(out) == 0 {
		return nil, nil
	}
	return append([]int(nil), out...), nil
}
