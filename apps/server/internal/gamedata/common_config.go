package gamedata

// CommonConfig is config/data/common.toml: the files at the root of the
// original Data/ folder.
type CommonConfig struct {
	MapManager    MapManagerFile    `comment:"MapManager.txt: -1 = default; rates in percent (ExcItemDropRate / SetItemDropRate of 1,000,000)"`
	MapServerInfo MapServerInfoFile `comment:"MapServerInfo.txt: the game servers of each group and the maps each one hands over"`
	MapRateInfo   MapRateInfoFile   `comment:"MapRateInfo.txt: ItemOptionRate rows per map and drop type (0 common, 1 excellent, 2 set); -1 = any"`

	BlackList          BlackListFile          `comment:"BlackList.txt (GameServer): refused IP addresses (16 characters) and hardware ids (44)"`
	CommandManager     CommandManagerFile     `comment:"CommandManager.txt: AL0..AL3 enable the command per account level; Delay in seconds; -1 = no limit"`
	CommandRequirement CommandRequirementFile `comment:"CommandRequirement.txt: ReqItemIndex \"*\" = none; -1 = any"`
	ConnectMember      ConnectMemberFile      `comment:"ConnectMember.txt: accounts let in when ConnectMemberCheck is on"`
}

type BlackListFile struct {
	IpAddress  []string `comment:"section 0"`
	HardwareId []string `comment:"section 1"`
}

type CommandManagerFile struct {
	Rows []CommandManagerRow `toml:",inline"`
}

type CommandManagerRow struct {
	Index, AL0, AL1, AL2, AL3                  int
	Syntax                                     string
	Delay, GMLevel, MinLevel, MaxLevel         int
	MinReset, MaxReset                         int
	Money_AL0, Money_AL1, Money_AL2, Money_AL3 int
	Comment                                    string `toml:",omitempty" legacy:"comment"`
}

type CommandRequirementFile struct {
	Rows []CommandRequirementRow `toml:",inline"`
}

type CommandRequirementRow struct {
	Index, AccountLevel, MinLevel, MaxLevel, MinReset, MaxReset, CheckItem int
	ReqItemIndex                                                           string
	ReqItemCount, ReqItemLevel, ReqEffectState, ReqQuestIndex              int
	ReqQuestState, ReqPKLevel, ReqPKCount, ReqGuildRank                    int
	MapIndex, MapStartX, MapStartY, MapEndX, MapEndY                       int
	DayOfWeek, MinHour, MaxHour, FailMessage                               int
	DW, DK, FE, MG, DL                                                     int
	Comment                                                                string `toml:",omitempty"`
}

type ConnectMemberFile struct {
	Account []string
}

type MapManagerFile struct {
	Rows []MapManagerRow `toml:",inline"`
}

type MapManagerRow struct {
	MapNumber, NonPK, ViewRange, ExperienceRate, ItemDropRate, MoneyDropRate int
	ExcItemDropRate, SetItemDropRate, ItemDrop, PKItemDrop, MonsterHPBar     int
	TradeEnable, PShopEnable, ChatEnable, PartyEnable, DuelEnable            int
	GuildWarEnable, SpawnGate, RespawnGate                                   int
	Name                                                                     string
	Comment                                                                  string `toml:",omitempty" legacy:"comment"`
}

type MapServerInfoFile struct {
	Server []MapServerRow     `toml:",inline" comment:"section 0"`
	Move   []MapServerMoveRow `toml:",inline" comment:"section 1: MoveOption 0 = NextMap is on NextServerCode, 1 = stay; NextServerCode -1 = none"`
}

type MapServerRow struct {
	ServerCode, ServerGroup, InitSetVal int
	IpAddress                           string
	Port                                int
	Comment                             string `toml:",omitempty"`
}

type MapServerMoveRow struct {
	ServerCode, MoveOption, NextMap, NextServerCode int
	Comment                                         string `toml:",omitempty" legacy:"comment"`
}

type MapRateInfoFile struct {
	Rows []MapRateInfoRow `toml:",inline"`
}

// MapRateInfoRow: Option = the ItemOptionRate rows of Level, Skill, Luck,
// JoL, Exc and Set.
type MapRateInfoRow struct {
	MapNumber, ItemDropType int
	Option                  [6]int
	Comment                 string `toml:",omitempty"`
}

// LoadCommonConfig reads config/data/common.toml.
func LoadCommonConfig(configDir string) (*CommonConfig, error) {
	return loadConfig[CommonConfig](configDir, "common")
}
