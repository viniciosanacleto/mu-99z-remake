package gamedata

// EventItemBagConfig is config/data/eventitembag.toml:
// Data/EventItemBagManager.txt and the Data/EventItemBag files it names,
// keyed by BagPath with "/" separators.
type EventItemBagConfig struct {
	EventItemBagManager EventItemBagManagerFile `comment:"EventItemBagManager.txt: what opens each bag; -1 = any"`
	Bag                 map[string]ItemBagFile  `comment:"EventItemBag/<BagPath>.txt (CItemBagEx)"`
}

type EventItemBagManagerFile struct {
	Rows []EventItemBagManagerRow `toml:",inline"`
}

type EventItemBagManagerRow struct {
	ItemIndex, ItemLevel, MonsterClass, SpecialValue, SpecialLevel, SpecialState int
	BagPath                                                                      string
	Comment                                                                      string `toml:",omitempty"`
}

// ItemBagFile keeps the sections of a bag file: Settings (section 2, the
// last row wins), Bag (section 3, the first row of an index wins), Drop
// (section 4) and Items (sections 5 and up).
type ItemBagFile struct {
	Settings []ItemBagSettingsRow `toml:",inline" comment:"section 2: ItemRate % item instead of zen; BagUseEffect 1/2 firework, 3 other; DropType 0 ground, 1 inventory"`
	Bag      []ItemBagRow         `toml:",inline" comment:"section 3: DropRate of 10000; MapNumber -1 = any"`
	Drop     []ItemBagDropRow     `toml:",inline" comment:"section 4: Section -1 = set item; Class = required change-up + 1 per class (0 = not allowed)"`
	Items    []ItemBagSection     `comment:"sections 5~16: the item lists; Option = ItemOptionRate rows (-1 = default); Duration in seconds"`
}

type ItemBagSettingsRow struct {
	ItemRate, MoneyDrop, BagUseEffect, DropType int
	Comment                                     string `toml:",omitempty"`
}

type ItemBagRow struct {
	BagIndex, DropRate, MapNumber int
	Comment                       string `toml:",omitempty"`
}

type ItemBagDropRow struct {
	BagIndex, Section, SectionRate, SetDropRate, MinDropCount, MaxDropCount int
	Class                                                                   [5]int
	Comment                                                                 string `toml:",omitempty"`
}

type ItemBagSection struct {
	Section int
	Rows    []ItemBagItemRow `toml:",inline"`
}

type ItemBagItemRow struct {
	ItemIndex             string
	ItemLevel, GradeCount int
	Option                [6]int
	Duration              int
	Comment               string `toml:",omitempty"`
}

// LoadEventItemBagConfig reads config/data/eventitembag.toml.
func LoadEventItemBagConfig(configDir string) (*EventItemBagConfig, error) {
	return loadConfig[EventItemBagConfig](configDir, "eventitembag")
}
