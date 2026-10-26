package gamedata

// EventConfig is config/data/event.toml: Data/Event/*.dat.
type EventConfig struct {
	BloodCastle     BloodCastleFile     `comment:"BloodCastle.dat: times in minutes; EventIndex = castle 0~6"`
	BonusManager    BonusManagerFile    `comment:"BonusManager.dat: BonusIndex 0 experience, 1 item drop, 2 chaos mix, 3 excellent drop, 4 set drop; BonusMapNumber -1 = any"`
	CastleDeepEvent CastleDeepFile      `comment:"CastleDeepEvent.dat: StageTime in minutes after the start"`
	CastleSiege     CastleSiegeFile     `comment:"CastleSiege.dat"`
	ChaosCastle     ChaosCastleFile     `comment:"ChaosCastle.dat: times in minutes; EventIndex = castle 0~5"`
	DevilSquare     DevilSquareFile     `comment:"DevilSquare.dat: times in minutes; EventIndex = square 0~5, rewards per rank 1~10"`
	DropEvent       DropEventFile       `comment:"DropEvent.dat: EventTime in seconds; -1 = any; ItemDropRate of 1,000,000"`
	EventEntryLevel EventEntryLevelFile `comment:"EventEntryLevel.dat: Section 0 Blood Castle, 1 Devil Square, 2 Chaos Castle, 3 Kalima; Special* = MG/DL"`
	EventStageSpawn EventStageSpawnFile `comment:"EventStageSpawn.dat: Section 0 Blood Castle, 1 Chaos Castle, 2 Devil Square; MaxRegenTime in ms, -1 = default"`
	GoldenArcher    GoldenArcherFile    `comment:"GoldenArcher.dat: Option = ItemOptionRate rows; DropRate is a weight"`
	InvasionManager InvasionFile        `comment:"InvasionManager.dat: EventIndex 0~19; messages -1 = none; InvasionTime in seconds; RegenTime in ms"`
	Kalima          KalimaFile          `comment:"Kalima.dat: KALevel 0~5"`
}

// EventTimeRow is the state times row (section 0) of the square and castle
// events, in minutes.
type EventTimeRow struct {
	WarningTime, NotifyTime, EventTime, CloseTime int
	Comment                                       string `toml:",omitempty" legacy:"comment"`
}

// ScheduleRow is a start time; -1 = any.
type ScheduleRow struct {
	StartYear, StartMonth, StartDay, StartDoW, StartHour, StartMin, StartSec int
	Comment                                                                  string `toml:",omitempty" legacy:"comment"`
}

func (r ScheduleRow) Schedule() Schedule {
	return Schedule{r.StartYear, r.StartMonth, r.StartDay, r.StartDoW, r.StartHour, r.StartMin, r.StartSec}
}

// EventScheduleRow is a ScheduleRow of one event of a multi-event file.
type EventScheduleRow struct {
	EventIndex                                                               int
	StartYear, StartMonth, StartDay, StartDoW, StartHour, StartMin, StartSec int
	Comment                                                                  string `toml:",omitempty" legacy:"comment"`
}

func (r EventScheduleRow) Schedule() Schedule {
	return Schedule{r.StartYear, r.StartMonth, r.StartDay, r.StartDoW, r.StartHour, r.StartMin, r.StartSec}
}

// EventIndexPairRow is EventIndex followed by two values.
type EventIndexPairRow struct {
	EventIndex     int
	Value1, Value2 int
	Comment        string `toml:",omitempty" legacy:"comment"`
}

type BloodCastleFile struct {
	Time       []EventTimeRow      `toml:",inline" comment:"section 0"`
	Schedule   []ScheduleRow       `toml:",inline" comment:"section 1"`
	Experience []EventIndexPairRow `toml:",inline" comment:"section 2: ExperienceTable1 (winning party), ExperienceTable2"`
	Money      []EventIndexPairRow `toml:",inline" comment:"section 3: MoneyTable1, MoneyTable2"`
	Life       []EventIndexPairRow `toml:",inline" comment:"section 4: GateNpcLife, StatueNpcLife"`
	Statue     []EventIndexPairRow `toml:",inline" comment:"section 5: SaintStatueX, SaintStatueY"`
}

// TimedEventSettingsRow is section 1 of BonusManager.dat and DropEvent.dat.
type TimedEventSettingsRow struct {
	EventIndex, StartMessage, FinalMessage, EventTime int
	EventName                                         string `toml:",omitempty"`
	Comment                                           string `toml:",omitempty"`
}

type BonusManagerFile struct {
	Schedule []EventScheduleRow      `toml:",inline" comment:"section 0"`
	Settings []TimedEventSettingsRow `toml:",inline" comment:"section 1: EventTime = BonusTime in seconds"`
	Bonus    []BonusRow              `toml:",inline" comment:"section 2: BonusValue per account level AL0~AL3"`
}

type BonusRow struct {
	EventIndex, BonusIndex int
	BonusValue             [4]int
	BonusMapNumber         int
	Comment                string `toml:",omitempty" legacy:"comment"`
}

type CastleDeepFile struct {
	Schedule []ScheduleRow          `toml:",inline" comment:"section 0"`
	Group    []CastleDeepGroupRow   `toml:",inline" comment:"section 1"`
	Stage    []CastleDeepStageRow   `toml:",inline" comment:"section 2"`
	Monster  []CastleDeepMonsterRow `toml:",inline" comment:"section 3"`
}

type CastleDeepGroupRow struct {
	Group, SpawnRate int
	Comment          string `toml:",omitempty" legacy:"comment"`
}

type CastleDeepStageRow struct {
	Group, Stage, StageTime int
	Comment                 string `toml:",omitempty" legacy:"comment"`
}

type CastleDeepMonsterRow struct {
	Group, Stage, MonsterIndex, MaxSpawnCount      int
	RangeStartX, RangeStartY, RangeEndX, RangeEndY int
	Comment                                        string `toml:",omitempty" legacy:"comment"`
}

type CastleSiegeFile struct {
	Cycle                []CastleCycleRow    `toml:",inline" comment:"section 0"`
	Schedule             []CastleScheduleRow `toml:",inline" comment:"section 1: the state machine schedule (day of the cycle, hour, minute)"`
	NPC                  []CastleNPCRow      `toml:",inline" comment:"section 2"`
	GateDefenseUpgrade   []CastleUpgradeRow  `toml:",inline" comment:"section 3: Index 0~2, Value = target level, Count = Jewels of Guardian"`
	StatueDefenseUpgrade []CastleUpgradeRow  `toml:",inline" comment:"section 4"`
	GateLifeUpgrade      []CastleUpgradeRow  `toml:",inline" comment:"section 5: Value = target life"`
	StatueLifeUpgrade    []CastleUpgradeRow  `toml:",inline" comment:"section 6"`
	StatueRegenUpgrade   []CastleUpgradeRow  `toml:",inline" comment:"section 7"`
	GateDefense          []CastleDefenseRow  `toml:",inline" comment:"section 8: gate defense per DF level 0~3"`
	StatueDefense        []CastleDefenseRow  `toml:",inline" comment:"section 9: statue defense per DF level 0~3"`
}

type CastleCycleRow struct {
	CycleDays int
	Comment   string `toml:",omitempty" legacy:"comment"`
}

type CastleScheduleRow struct {
	State, StartDay, StartHour, StartMinute int
	Comment                                 string `toml:",omitempty" legacy:"comment"`
}

type CastleNPCRow struct {
	Class, Index, DBSave, Side, DFLevel, RGLevel, MaxLife, MapX, MapY, MapDir int
	Comment                                                                   string `toml:",omitempty" legacy:"comment"`
}

type CastleUpgradeRow struct {
	Index, Value, Count, Money int
	Comment                    string `toml:",omitempty" legacy:"comment"`
}

type CastleDefenseRow struct {
	Level   [4]int
	Comment string `toml:",omitempty" legacy:"comment"`
}

type ChaosCastleFile struct {
	Time       []EventTimeRow      `toml:",inline" comment:"section 0"`
	Schedule   []ScheduleRow       `toml:",inline" comment:"section 1"`
	Experience []EventIndexPairRow `toml:",inline" comment:"section 2: ExperienceTable1 (per user killed), ExperienceTable2 (per monster)"`
}

type DevilSquareFile struct {
	Time       []EventTimeRow   `toml:",inline" comment:"section 0"`
	Schedule   []ScheduleRow    `toml:",inline" comment:"section 1"`
	Experience []DevilRewardRow `toml:",inline" comment:"section 2: RewardExperience1~10"`
	Money      []DevilRewardRow `toml:",inline" comment:"section 3: RewardMoney1~10"`
}

type DevilRewardRow struct {
	EventIndex int
	Reward     [10]int
	Comment    string `toml:",omitempty" legacy:"comment"`
}

type DropEventFile struct {
	Schedule []EventScheduleRow      `toml:",inline" comment:"section 0"`
	Settings []TimedEventSettingsRow `toml:",inline" comment:"section 1"`
	Item     []DropEventItemRow      `toml:",inline" comment:"section 2: Option = ItemOptionRate rows; Duration in seconds"`
}

type DropEventItemRow struct {
	EventIndex                                     int
	ItemIndex                                      string
	ItemLevel, Durability                          int
	Option                                         [6]int
	Duration, MapNumber, MonsterIndex              int
	MonsterMinLevel, MonsterMaxLevel, AccountLevel int
	ItemDropRate                                   int
	Comment                                        string `toml:",omitempty"`
}

// EventEntryLevelFile keeps the sections in file order.
type EventEntryLevelFile struct {
	Sections []EntryLevelSection
}

type EntryLevelSection struct {
	Section int
	Rows    []EntryLevelRow `toml:",inline"`
}

type EntryLevelRow struct {
	Index, CommonMinLevel, CommonMaxLevel, SpecialMinLevel, SpecialMaxLevel int
	Comment                                                                 string `toml:",omitempty" legacy:"comment"`
}

// EventStageSpawnFile keeps the sections in file order.
type EventStageSpawnFile struct {
	Sections []StageSpawnSection
}

type StageSpawnSection struct {
	Section int
	Rows    []StageSpawnRow `toml:",inline"`
}

type StageSpawnRow struct {
	Level, Stage, MonsterIndex, MaxRegenTime int
	Comment                                  string `toml:",omitempty" legacy:"comment"`
}

type GoldenArcherFile struct {
	Rows []GoldenArcherRow `toml:",inline"`
}

type GoldenArcherRow struct {
	ItemIndex          string
	RewardGroup        int
	Option             [6]int
	Duration, DropRate int
	Comment            string `toml:",omitempty"`
}

type InvasionFile struct {
	Schedule []EventScheduleRow   `toml:",inline" comment:"section 0"`
	Settings []InvasionSettingRow `toml:",inline" comment:"section 1"`
	Group    []InvasionGroupRow   `toml:",inline" comment:"section 2: one row of each group is drawn at the start"`
	Spawn    []InvasionSpawnRow   `toml:",inline" comment:"section 3"`
}

type InvasionSettingRow struct {
	EventIndex, RespawnMessage, DespawnMessage, MonsterBossIndex int
	BossKilledMessage, InvasionTime, EffectType                  int
	EventName                                                    string `toml:",omitempty"`
	Comment                                                      string `toml:",omitempty"`
}

type InvasionGroupRow struct {
	EventIndex, Group, MapNumber, Stage int
	Comment                             string `toml:",omitempty" legacy:"comment"`
}

type InvasionSpawnRow struct {
	EventIndex, Group, MonsterIndex, SpawnRadius, MapNumber int
	RangeStartX, RangeStartY, RangeEndX, RangeEndY          int
	MaxSpawnCount, Stage, RegenType, RegenTime, KillMessage int
	Comment                                                 string `toml:",omitempty" legacy:"comment"`
}

type KalimaFile struct {
	Rows []KalimaRow `toml:",inline"`
}

type KalimaRow struct {
	KALevel, CreateNextPortal int
	Comment                   string `toml:",omitempty"`
}

// LoadEventConfig reads config/data/event.toml.
func LoadEventConfig(configDir string) (*EventConfig, error) {
	return loadConfig[EventConfig](configDir, "event")
}
