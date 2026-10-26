package gamedata

// CustomConfig is config/data/custom.toml: the WizardTeam customs of
// Data/Custom (CustomXShop and CustomXShopRecharge are in xshop.toml, the
// Custom/Event files in custom-event.toml). -1 = any / none.
type CustomConfig struct {
	CreationGold           CreationGoldFile           `comment:"CreationGold.txt: Gold credited by a Jewel of Creation (0 disables it); read on every use"`
	CustomArena            CustomArenaFile            `comment:"CustomArena.txt: EventIndex 0~29; times in minutes; DW..DL 0 refused, n allowed from change-up n-1"`
	CustomAttack           CustomAttackFile           `comment:"CustomAttack.txt: the auto attack"`
	CustomAttribute        CustomAttributeFile        `comment:"CustomAttribute.txt: bonuses of equipped items"`
	CustomBuyVip           CustomBuyVipFile           `comment:"CustomBuyVip.txt: AccountLevel -1 = any (else the minimum current level); the first row of a Code wins"`
	CustomDeathMessage     CustomDeathMessageFile     `comment:"CustomDeathMessage.txt: what a monster says after a kill; the first row of a class wins"`
	CustomEventDrop        CustomEventDropFile        `comment:"CustomEventDrop.txt: EventIndex 0~9; times in minutes, DropDelay in seconds"`
	CustomEvolution        CustomEvolutionFile        `comment:"CustomEvolution.txt: /evo turns ItemTarget into ItemResult; the first row of an Index wins"`
	CustomGHRS             CustomGHRSFile             `comment:"CustomGHRS.ini (CastleSiege.exe only)"`
	CustomGate             CustomGateFile             `comment:"CustomGate.txt: standing on (MapStart, RangeStart) moves to (MapEnd, RangeEnd); the first row of a GateIndex wins"`
	CustomGift             CustomGiftFile             `comment:"CustomGift.txt: gift codes; MaxCount -1 = unlimited"`
	CustomGuard            MonsterClassListFile       `comment:"CustomGuard.txt: monster classes that never drop (100 at most)"`
	CustomHealthBar        CustomHealthBarFile        `comment:"CustomHealthBar.txt: the first matching row wins"`
	CustomJewel            CustomJewelFile            `comment:"CustomJewel.txt: JewelIndex 0~49; Rate in percent"`
	CustomLuckyWheel       CustomLuckyWheelFile       `comment:"CustomLuckyWheel.txt: the first row of an IndexWheel wins"`
	CustomLukeHelper       CustomLukeHelperFile       `comment:"CustomLukeHelper.txt: EventTime in minutes"`
	CustomMarriage         CustomMarriageFile         `comment:"CustomMarriage.txt"`
	CustomMobCombo         MonsterClassListFile       `comment:"CustomMobCombo.txt: monster classes that are not pushed back (100 at most)"`
	CustomMonster          CustomMonsterFile          `comment:"CustomMonster.txt: percentages applied to the monster; the first matching row wins"`
	CustomMonsterKillCount CustomMonsterKillCountFile `comment:"CustomMonsterKillCount.txt: the first row of a DBIndex wins; Duration in seconds"`
	CustomMove             CustomMoveFile             `comment:"CustomMove.txt: custom /move commands; the first row of an Id wins"`
	CustomNPCCommand       CustomNPCCommandFile       `comment:"CustomNPCCommand.txt: talking to the NPC on that cell runs the command"`
	CustomPatente          CustomPatenteFile          `comment:"CustomPatente.txt: rank values (50 at most); empty when the file does not exist"`
	CustomPet              CustomPetFile              `comment:"CustomPet.txt"`
	CustomPick             CustomPickFile             `comment:"CustomPick.txt: items the auto pick never takes for account levels up to AccountLevel"`
	CustomQuiz             CustomQuizFile             `comment:"CustomQuiz.txt: Index 0~49; times in minutes; the first row of a question ID wins"`
	CustomRespawn          CustomRespawnFile          `comment:"CustomRespawn.txt: respawn gate per account level: -1 the death cell, 0 the death cell moved down, -2 nothing"`
	CustomWing             CustomWingFile             `comment:"CustomWing.txt: Index 0~99"`
	DailyReward            DailyRewardFile            `comment:"DailyReward.txt: the first row of a day wins; Duration in seconds"`
	SummonScroll           SummonScrollFile           `comment:"SummonScroll.txt: CreateRate is the weight of the row inside its group"`
}

type CreationGoldFile struct {
	GoldValue int
}

// IndexedScheduleRow is a schedule row of one entry of a multi-entry file.
type IndexedScheduleRow struct {
	EventIndex                                                               int
	StartYear, StartMonth, StartDay, StartDoW, StartHour, StartMin, StartSec int
	Comment                                                                  string `toml:",omitempty" legacy:"comment"`
}

func (r IndexedScheduleRow) Schedule() Schedule {
	return Schedule{r.StartYear, r.StartMonth, r.StartDay, r.StartDoW, r.StartHour, r.StartMin, r.StartSec}
}

type CustomArenaFile struct {
	Schedule []IndexedScheduleRow `toml:",inline" comment:"section 0"`
	Rule     []CustomArenaRuleRow `toml:",inline" comment:"section 1"`
}

type CustomArenaRuleRow struct {
	EventIndex                                       int
	Name                                             string
	AlarmTime, StandTime, EventTime, CloseTime       int
	StartGate, FinalGate, MinUser, MaxUser, MaxDeath int
	MinLevel, MaxLevel, MinReset, MaxReset           int
	MinMasterReset, MaxMasterReset, RewardBagIndex   int
	DW, DK, FE, MG, DL                               int
	Comment                                          string `toml:",omitempty" legacy:"comment"`
}

type CustomAttackFile struct {
	Speed []CustomAttackSpeedRow `toml:",inline" comment:"section 0: AttackDelay in ms, -1 = BaseSpeed - MainSpeed * speed"`
	Skill []CustomAttackSkillRow `toml:",inline" comment:"section 1: Index 0~213; Group 0 buff, 1 single target, 2 area magic, 3 area skill, 4 special"`
}

type CustomAttackSpeedRow struct {
	SkillIndex, MinDexterity, MaxDexterity, MinCharSpeed, MaxCharSpeed int
	AttackDelay, BaseSpeed, MainSpeed                                  int
	Comment                                                            string `toml:",omitempty" legacy:"comment"`
}

type CustomAttackSkillRow struct {
	Index, Group, Value, DW, DK, FE, MG, DL int
	Comment                                 string `toml:",omitempty" legacy:"comment"`
}

type CustomAttributeFile struct {
	Rows []CustomAttributeRow `toml:",inline"`
}

type CustomAttributeRow struct {
	ItemIndex                                                     string
	AttackSuccess, Defense, SucessDefense, AddLife, DamageReflect int
	DamageMin, DamageMax, CriticalDamage, ExcellentDamageRate     int
	Comment                                                       string `toml:",omitempty"`
}

type CustomBuyVipFile struct {
	Rows []CustomBuyVipRow `toml:",inline"`
}

type CustomBuyVipRow struct {
	AccountLevel, NewAccountLevel, Days, Money int
	Code                                       string
	Comment                                    string `toml:",omitempty" legacy:"comment"`
}

type CustomDeathMessageFile struct {
	Rows []CustomDeathMessageRow `toml:",inline"`
}

type CustomDeathMessageRow struct {
	MobIndex int
	Message  string
	Comment  string `toml:",omitempty" legacy:"comment"`
}

type CustomEventDropFile struct {
	Schedule []IndexedScheduleRow     `toml:",inline" comment:"section 0"`
	Rule     []CustomEventDropRuleRow `toml:",inline" comment:"section 1"`
	Item     []CustomEventDropItemRow `toml:",inline" comment:"section 2"`
}

type CustomEventDropRuleRow struct {
	EventIndex                                  int
	Name                                        string
	MapNumber, LocationX, LocationY, DropRadius int
	AlarmTime, EventTime                        int
	Comment                                     string `toml:",omitempty" legacy:"comment"`
}

type CustomEventDropItemRow struct {
	EventIndex                      int
	ItemIndex                       string
	ItemLevel, DropCount, DropDelay int
	Comment                         string `toml:",omitempty" legacy:"comment"`
}

type CustomEvolutionFile struct {
	Rows []CustomEvolutionRow `toml:",inline"`
}

type CustomEvolutionRow struct {
	Index                  int
	ItemTarget, ItemResult string
	Comment                string `toml:",omitempty"`
}

type CustomGHRSFile struct {
	WEEK       GHRSWeek `comment:"bonus per weekday"`
	TIMEUPDATE GHRSTime
}

type GHRSWeek struct {
	Monday, Tuesday, Wednesday, Thursday, Friday, Saturday, Sunday int
}

type GHRSTime struct {
	Hour, Min, Sec int
}

type CustomGateFile struct {
	Rows []CustomGateConfigRow `toml:",inline"`
}

type CustomGateConfigRow struct {
	GateIndex, MapStart, RangeStartX, RangeStartY, MapEnd, RangeEndX, RangeEndY int
	MinLevel, MaxLevel, MinReset, MaxReset, AccountLevel                        int
	Comment                                                                     string `toml:",omitempty" legacy:"comment"`
}

type CustomGiftFile struct {
	Gift []CustomGiftRow     `toml:",inline" comment:"section 0"`
	Item []CustomGiftItemRow `toml:",inline" comment:"section 1: items of the gift with that GiftIndex"`
}

type CustomGiftRow struct {
	GiftIndex, Class, AL0, AL1, AL2, AL3                           int
	MinLevel, MaxLevel, MinReset, MaxReset, MaxCount, LevelUpPoint int
	Money                                                          int
	Code                                                           string
	Comment                                                        string `toml:",omitempty"`
}

// CustomItemColumns are the item columns shared by several custom files.
type CustomItemColumns struct {
	ItemIndex                                                            string
	ItemLevel, Durability, SkillSwitch, LuckSwitch, Aditional, Excellent int
	SetOption, Duration                                                  int
}

type CustomGiftItemRow struct {
	GiftIndex int
	CustomItemColumns
	Comment string `toml:",omitempty" legacy:"comment"`
}

type MonsterClassListFile struct {
	MonsterClass []int `comment:"section 0"`
}

type CustomHealthBarFile struct {
	Rows []CustomHealthBarRow `toml:",inline"`
}

type CustomHealthBarRow struct {
	MonsterIndex, MapNumber, BarSwitch int
	Comment                            string `toml:",omitempty" legacy:"comment"`
}

type CustomJewelFile struct {
	Jewel       []CustomJewelRow       `toml:",inline" comment:"section 0"`
	Requirement []CustomJewelReqRow    `toml:",inline" comment:"section 1: -1 = any; MaxItemExcellent < 10 is a count, > 10 a forbidden bit mask (value - 10)"`
	Success     []CustomJewelChangeRow `toml:",inline" comment:"section 2: added to the item on success"`
	Failure     []CustomJewelChangeRow `toml:",inline" comment:"section 3: removed from the item on failure"`
}

type CustomJewelRow struct {
	JewelIndex                                        int
	ItemIndex                                         string
	EnableSlotWeapon, EnableSlotArmor, EnableSlotWing int
	Rate_AL0, Rate_AL1, Rate_AL2, Rate_AL3, SalePrice int
	Comment                                           string `toml:",omitempty"`
}

type CustomJewelReqRow struct {
	JewelIndex, MinItemLevel, MaxItemLevel, ItemWithSkill, ItemWithLuck    int
	ItemMinAditional, ItemMaxAditional, MinItemExcellent, MaxItemExcellent int
	MaxItemSetOption                                                       int
	Comment                                                                string `toml:",omitempty" legacy:"comment"`
}

type CustomJewelChangeRow struct {
	JewelIndex, ItemLevel, SkillSwitch, LuckSwitch, Aditional, Excellent, SetOption int
	Comment                                                                         string `toml:",omitempty" legacy:"comment"`
}

type CustomLuckyWheelFile struct {
	Rows []CustomLuckyWheelRow `toml:",inline"`
}

type CustomLuckyWheelRow struct {
	IndexWheel, Type, Index, Level, Skill, Luck, Option, Excellent int
	Comment                                                        string `toml:",omitempty" legacy:"comment"`
}

type CustomLukeHelperFile struct {
	EventTime []int                     `comment:"section 0 (the last value wins)"`
	Spot      []CustomLukeHelperSpotRow `toml:",inline" comment:"section 1"`
	Schedule  []ScheduleRow             `toml:",inline" comment:"section 2"`
}

type CustomLukeHelperSpotRow struct {
	MapNumber, MonsterClass, RangeStartX, RangeStartY, RangeEndX, RangeEndY, BagSpecial int
	Comment                                                                             string `toml:",omitempty" legacy:"comment"`
}

type CustomMarriageFile struct {
	Marry   []CustomMarriageItemRow `toml:",inline" comment:"section 0 (the last row wins)"`
	Divorce []CustomMarriageItemRow `toml:",inline" comment:"section 1 (the last row wins)"`
	Reward  []CustomRewardItemRow   `toml:",inline" comment:"section 2"`
}

type CustomMarriageItemRow struct {
	Enable               int
	ItemIndex            string
	ItemLevel, ItemCount int
	Comment              string `toml:",omitempty" legacy:"comment"`
}

type CustomRewardItemRow struct {
	CustomItemColumns
	Comment string `toml:",omitempty" legacy:"comment"`
}

type CustomMonsterFile struct {
	Rows []CustomMonsterRowConfig `toml:",inline"`
}

type CustomMonsterRowConfig struct {
	MonsterIndex, MapNumber, MaxLife, DamageMin, DamageMax, Defense  int
	AttackRate, DefenseRate, ExperienceRate, KillMessage, SpecialBag int
	Comment                                                          string `toml:",omitempty"`
}

type CustomMonsterKillCountFile struct {
	Kill []CustomKillCountRow     `toml:",inline" comment:"section 0"`
	Item []CustomKillCountItemRow `toml:",inline" comment:"section 1: items of the row with that DBIndex"`
}

type CustomKillCountRow struct {
	DBIndex, MonsterIndex, AL0, AL1, AL2, AL3, KillCount, Money int
	Message                                                     string
	Comment                                                     string `toml:",omitempty"`
}

type CustomKillCountItemRow struct {
	DBIndex int
	CustomItemColumns
	Comment string `toml:",omitempty" legacy:"comment"`
}

type CustomMoveFile struct {
	Rows []CustomMoveConfigRow `toml:",inline"`
}

type CustomMoveConfigRow struct {
	Id                                                int
	Command                                           string
	Map, X, Y, MinLevel, MaxLevel, MinReset, MaxReset int
	MinMReset, MaxMReset, AccountLevel, PKMove        int
	Comment                                           string `toml:",omitempty" legacy:"comment"`
}

type CustomNPCCommandFile struct {
	Rows []CustomNPCCommandConfigRow `toml:",inline"`
}

type CustomNPCCommandConfigRow struct {
	Npc, Map, X, Y int
	Command        string
	Comment        string `toml:",omitempty" legacy:"comment"`
}

type CustomPatenteFile struct {
	Value []int
}

type CustomPetFile struct {
	Rows []CustomPetRow `toml:",inline"`
}

type CustomPetRow struct {
	Index                                                       int
	ItemIndex                                                   string
	AddHP, AddMP, DamageRate, AttackSpeed, DoubleDmgRate        int
	TripleDmgRate, AddExp, ResistDoubleDmg, ResistIgnoreDefense int
	ResistIgnoreSD, ResistCriticalDMG, ResistExceDMG            int
	BlockStuck, ReflectRate, PetRecovery                        int
	Comment                                                     string `toml:",omitempty"`
}

type CustomPickFile struct {
	Rows []CustomPickRow `toml:",inline"`
}

type CustomPickRow struct {
	ItemIndex               string
	ItemLevel, AccountLevel int
	Comment                 string `toml:",omitempty" legacy:"comment"`
}

type CustomQuizFile struct {
	Schedule []IndexedScheduleRow `toml:",inline" comment:"section 0"`
	Quiz     []CustomQuizRow      `toml:",inline" comment:"section 1"`
	Question []CustomQuestionRow  `toml:",inline" comment:"section 2"`
}

type CustomQuizRow struct {
	Index                int
	Name                 string
	AlarmTime, EventTime int
	Comment              string `toml:",omitempty" legacy:"comment"`
}

type CustomQuestionRow struct {
	ID, Money        int
	Question, Answer string
	Comment          string `toml:",omitempty" legacy:"comment"`
}

type CustomRespawnFile struct {
	Rows []CustomRespawnConfigRow `toml:",inline"`
}

type CustomRespawnConfigRow struct {
	Mapa                                                                   int
	AccLevelGate_AL0, AccLevelGate_AL1, AccLevelGate_AL2, AccLevelGate_AL3 int
	Comment                                                                string `toml:",omitempty" legacy:"comment"`
}

type CustomWingFile struct {
	Rows []CustomWingRow `toml:",inline"`
}

type CustomWingRow struct {
	Index                                                  int
	ItemIndex                                              string
	DefenseConstA, IncDamageConstA, IncDamageConstB        int
	DecDamageConstA, DecDamageConstB                       int
	OptionIndex1, OptionValue1, OptionIndex2, OptionValue2 int
	OptionIndex3, OptionValue3                             int
	NewOptionIndex1, NewOptionValue1, NewOptionIndex2      int
	NewOptionValue2, NewOptionIndex3, NewOptionValue3      int
	NewOptionIndex4, NewOptionValue4                       int
	ModelType                                              int    `legacy:"-"` // optional column
	Name                                                   string `toml:",omitempty" legacy:"-"`
	Comment                                                string `toml:",omitempty"`
}

type DailyRewardFile struct {
	Rows []DailyRewardConfigRow `toml:",inline"`
}

type DailyRewardConfigRow struct {
	Day                                          int
	Item                                         string
	Level, Skill, Luck, AddOption, Dur, Exc, Anc int
	Duration                                     int
	Info1, Info2                                 string
	Comment                                      string `toml:",omitempty" legacy:"comment"`
}

type SummonScrollFile struct {
	Scroll  []SummonScrollRow        `toml:",inline" comment:"section 0"`
	Monster []SummonScrollMonsterRow `toml:",inline" comment:"section 1"`
}

type SummonScrollRow struct {
	Index     int
	ItemIndex string
	Comment   string `toml:",omitempty" legacy:"comment"`
}

type SummonScrollMonsterRow struct {
	Index, Group, MonsterClass, CreateRate int
	Comment                                string `toml:",omitempty" legacy:"comment"`
}

// LoadCustomConfig reads config/data/custom.toml.
func LoadCustomConfig(configDir string) (*CustomConfig, error) {
	return loadConfig[CustomConfig](configDir, "custom")
}
