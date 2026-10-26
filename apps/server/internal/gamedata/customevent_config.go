package gamedata

import "github.com/viniciosanacleto/mu-remake/apps/server/internal/config/tomlconf"

// CustomEventConfig is config/data/custom-event.toml: the WizardTeam events
// of Data/Custom/Event. Active > 0 enables the event; Syntax is the player
// command; Day 0 = Sunday; Time in minutes. The converter fills the values
// the script reader gives to missing cells (-1, map / x / y 255).
type CustomEventConfig struct {
	DeathEvent  KillEventFile   `comment:"DeathEvent.txt: Duration in minutes"`
	DuelEvent   FightEventFile  `comment:"DuelEvent.txt: Class of the duel in each start row"`
	PairEvent   FightEventFile  `comment:"PairEvent.txt"`
	RaffleEvent RaffleEventFile `comment:"RaffleEvent.txt"`
	RoletaEvent RoletaEventFile `comment:"RoletaEvent.txt: no schedule, only /rodar starts it"`
	SobreEvent  SobreEventFile  `comment:"SobreEvent.txt: MinPlayers to start"`
	TheftEvent  KillEventFile   `comment:"TheftEvent.txt: Duration in minutes"`
}

type KillEventFile struct {
	Active, Patente int
	Syntax          string
	Level           int
	Map, X, Y       int
	Start           []EventStartRow `toml:",inline"`
}

type FightEventFile struct {
	Active, Patente          int
	Syntax                   string
	Level                    int
	WaitMap, WaitX, WaitY    int
	FightMap, FightX, FightY int
	Start                    []EventStartRow `toml:",inline"`
}

type SobreEventFile struct {
	Active, Patente int
	Syntax          string
	Level           int
	Map, X, Y       int
	MinPlayers      int
	Start           []EventStartRow `toml:",inline"`
}

type RoletaEventFile struct {
	Active    int
	Syntax    string
	Level     int
	Map, X, Y int
}

type RaffleEventFile struct {
	Active int
	Start  []EventStartRow `toml:",inline"`
}

// EventStartRow is a start time (section 1); Extra is the fifth column
// (DuelEvent Class, DeathEvent / TheftEvent Duration), -1 when the file has
// none.
type EventStartRow struct {
	Day, Hour, Minute, Time, Extra int
	Comment                        string `toml:",omitempty"`
}

func (r EventStartRow) start() EventStart {
	return EventStart{Day: r.Day, Hour: r.Hour, Minute: r.Minute, Time: r.Time, Extra: r.Extra}
}

func eventStartsOf(rows []EventStartRow) []EventStart {
	var out []EventStart
	for _, r := range rows {
		out = append(out, r.start())
	}
	return out
}

// LoadCustomEventConfig reads config/data/custom-event.toml.
func LoadCustomEventConfig(configDir string) (*CustomEventConfig, error) {
	return loadConfig[CustomEventConfig](configDir, "custom-event")
}

// XShopConfig is config/data/xshop.toml: Data/Custom/CustomXShop.txt and
// CustomXShopRecharge.txt. The server rewrites this file when a GM changes
// a price (FA price packet) or uploads the recharge data (FA 10).
type XShopConfig struct {
	CustomXShop         XShopFile         `comment:"CustomXShop.txt: prices in credits; negative prices read as 0"`
	CustomXShopRecharge XShopRechargeFile `comment:"CustomXShopRecharge.txt [Recarga]: the PIX data shown in the X Shop;\nImagemBMP is a file in data/Data/Custom (a bare name) or a path"`
}

type XShopFile struct {
	Category []XShopCategoryRow `toml:",inline" comment:"section 0: price of every item of the category (default 25)"`
	Item     []XShopItemRow     `toml:",inline" comment:"section 1: price of one item"`
}

type XShopCategoryRow struct {
	Type, Price int
	Comment     string `toml:",omitempty" legacy:"comment"`
}

type XShopItemRow struct {
	Type, Index, Price int
	Comment            string `toml:",omitempty" legacy:"comment"`
}

type XShopRechargeFile struct {
	Ativo                         int
	Titular, ChavePix, CopiaECola string
	ImagemURL, ImagemBMP, Valor   string
}

// LoadXShopConfig reads an xshop.toml file.
func LoadXShopConfig(path string) (*XShopConfig, error) {
	var f XShopConfig
	if err := tomlconf.Load(path, &f); err != nil {
		return nil, err
	}
	return &f, nil
}
