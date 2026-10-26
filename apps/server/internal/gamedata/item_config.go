package gamedata

// ItemConfig is config/data/item.toml: the Data/Item/*.txt tables.
type ItemConfig struct {
	Item                ItemTable               `comment:"Item.txt (CItemManager): one array per item category; Slot -1 = not equippable"`
	ItemOption          ItemOptionFile          `comment:"ItemOption.txt: OptionIndex codes in \"Tools/Reference Item Options.txt\"; -1 = any"`
	ItemOptionRate      ItemOptionRateFile      `comment:"ItemOptionRate.txt: weights of each value of an option, by row Index"`
	ExcellentOptionRate ExcellentOptionRateFile `comment:"ExcellentOptionRate.txt: excellent option weights by item range (DropRate 0~100)"`
	ItemDrop            ItemDropFile            `comment:"ItemDrop.txt: specific item drops; DropRate per account level (AL0 free .. AL3 VIP3) of 1,000,000, -1 = always"`
	ItemMoneyDrop       ItemMoneyDropFile       `comment:"ItemMoneyDrop.txt: zen drops; -1 = any, MoneyAmount -1 = the monster's experience"`
	ItemLevel           ItemLevelFile           `comment:"ItemLevel.txt: item names per level (max 32 characters)"`
	ItemLoot            ItemLootFile            `comment:"ItemLoot.txt: AllowLoot 0 = only who dropped it can pick it up; ItemLevel -1 = any"`
	ItemMove            ItemMoveFile            `comment:"ItemMove.txt: items without a row can be dropped, sold, traded and stored"`
	ItemStack           ItemStackFile           `comment:"ItemStack.txt: stack limits (ItemLevel -1 = any) and stack packs"`
	ItemValue           ItemValueFile           `comment:"ItemValue.txt: sell value; ItemLevel / ItemGrade -1 = any"`
	SetItemType         SetItemTypeFile         `comment:"SetItemType.txt: ancient items; OptionIndex1/2 are SetItemOption rows"`
	SetItemOption       SetItemOptionFile       `comment:"SetItemOption.txt: Option = 12 [code, value] pairs (tier k = pairs 2k, 2k+1), FullOption = 5 full-set pairs; -1 = none"`
	WingOption          WingOptionFile          `comment:"WingOption.txt (CWingOption)"`
}

// --- Item.txt ------------------------------------------------------------------

// ItemTable holds Item.txt by category (the numbered sections of the
// original; a category may appear more than once there, later rows win).
type ItemTable struct {
	Swords      []ItemWeapon `toml:",inline" comment:"category 0"`
	Axes        []ItemWeapon `toml:",inline" comment:"category 1"`
	Maces       []ItemWeapon `toml:",inline" comment:"category 2"`
	Spears      []ItemWeapon `toml:",inline" comment:"category 3"`
	Bows        []ItemWeapon `toml:",inline" comment:"category 4"`
	Staffs      []ItemWeapon `toml:",inline" comment:"category 5"`
	Shields     []ItemShield `toml:",inline" comment:"category 6"`
	Helms       []ItemArmor  `toml:",inline" comment:"category 7"`
	Armors      []ItemArmor  `toml:",inline" comment:"category 8"`
	Pants       []ItemArmor  `toml:",inline" comment:"category 9"`
	Gloves      []ItemGloves `toml:",inline" comment:"category 10"`
	Boots       []ItemBoots  `toml:",inline" comment:"category 11"`
	Wings       []ItemWing   `toml:",inline" comment:"category 12"`
	Accessories []ItemHelper `toml:",inline" comment:"category 13"`
	Consumables []ItemPotion `toml:",inline" comment:"category 14"`
	Scrolls     []ItemScroll `toml:",inline" comment:"category 15"`
}

// ItemHead is the column prefix shared by every category.
type ItemHead struct {
	Type, Slot, Skill, Width, Height, HaveSerial, HaveOption, DropItem int
	Name                                                               string
}

// ItemClass is the class column suffix (0 no, 1 yes, 2 second class only).
type ItemClass struct{ DW, DK, FE, MG, DL int }

// ItemReq is the stat requirement block of equipment.
type ItemReq struct {
	ReqLevel, ReqStrength, ReqDexterity, ReqEnergy, ReqVitality, ReqLeadership int
}

type ItemWeapon struct {
	ItemHead
	Level, DamageMin, DamageMax, AttackSpeed, Durability, MagicDurability, MagicDamageRate int
	ItemReq
	Attr int
	ItemClass
	Comment string `toml:",omitempty" legacy:"comment"`
}

type ItemShield struct {
	ItemHead
	Level, Defense, DefenseSuccessRate, Durability int
	ItemReq
	Attr int
	ItemClass
	Comment string `toml:",omitempty" legacy:"comment"`
}

type ItemArmor struct {
	ItemHead
	Level, Defense, MagicDefense, Durability int
	ItemReq
	Attr int
	ItemClass
	Comment string `toml:",omitempty" legacy:"comment"`
}

type ItemGloves struct {
	ItemHead
	Level, Defense, AttackSpeed, Durability int
	ItemReq
	Attr int
	ItemClass
	Comment string `toml:",omitempty" legacy:"comment"`
}

type ItemBoots struct {
	ItemHead
	Level, Defense, WalkSpeed, Durability int
	ItemReq
	Attr int
	ItemClass
	Comment string `toml:",omitempty" legacy:"comment"`
}

type ItemWing struct {
	ItemHead
	Level, Defense, Durability, ReqLevel, ReqEnergy, ReqStrength, ReqDexterity, ReqLeadership, BuyMoney int
	ItemClass
	Comment string `toml:",omitempty" legacy:"comment"`
}

type ItemHelper struct {
	ItemHead
	Level, Durability                                                                         int
	Resistance1, Resistance2, Resistance3, Resistance4, Resistance5, Resistance6, Resistance7 int
	Attr                                                                                      int
	ItemClass
	Comment string `toml:",omitempty" legacy:"comment"`
}

type ItemPotion struct {
	ItemHead
	Value, Level int
	Comment      string `toml:",omitempty" legacy:"comment"`
}

type ItemScroll struct {
	ItemHead
	Level, ReqLevel, ReqEnergy, BuyMoney int
	ItemClass
	Comment string `toml:",omitempty" legacy:"comment"`
}

// --- the other Item/ files -------------------------------------------------------

type ItemOptionFile struct {
	Rows []ItemOptionRow `toml:",inline"`
}

type ItemOptionRow struct {
	Index, OptionIndex, OptionValue int
	ItemIndexMin, ItemIndexMax      string
	SkillSwitch, LuckSwitch         int
	Aditional, Excellent            int
	Comment                         string `toml:",omitempty" legacy:"comment"`
}

// ItemOptionRateFile has one array per section of the original.
type ItemOptionRateFile struct {
	Level     []OptionRateRow `toml:",inline" comment:"section 0: item level +0..+15"`
	Skill     []OptionRateRow `toml:",inline" comment:"section 1: skill 0/1"`
	Luck      []OptionRateRow `toml:",inline" comment:"section 2: luck 0/1"`
	Option    []OptionRateRow `toml:",inline" comment:"section 3: Jewel of Life option 0..7"`
	Excellent []OptionRateRow `toml:",inline" comment:"section 4: number of excellent options 0..6"`
	Set       []OptionRateRow `toml:",inline" comment:"section 5: set option 0..6"`
}

// OptionRateRow is Index followed by one weight per value (Rate0..RateN).
type OptionRateRow struct {
	Index   int
	Rate    []int
	Comment string `toml:",omitempty"`
}

type ExcellentOptionRateFile struct {
	Rows []ExcellentOptionRateRow `toml:",inline"`
}

type ExcellentOptionRateRow struct {
	Index                      int
	ItemMinIndex, ItemMaxIndex string
	Name                       string
	DropRate                   int
	Comment                    string `toml:",omitempty"`
}

type ItemDropFile struct {
	Rows []ItemDropRow `toml:",inline"`
}

// ItemDropRow: the *Option columns are ItemOptionRate rows of each section
// (-1 = default), Duration in seconds (0 = permanent), -1 = any.
type ItemDropRow struct {
	ItemIndex                            string
	ItemLevel, ItemGrade                 int
	LevelOption, SkillOption, LuckOption int
	JoLOption, ExcOption, SetOption      int
	Duration                             int
	MapNumber, MonsterIndex              int
	MonsterMinLevel, MonsterMaxLevel     int
	DropRate                             [4]int
	Comment                              string `toml:",omitempty"`
}

type ItemMoneyDropFile struct {
	Rows []ItemMoneyDropRow `toml:",inline"`
}

type ItemMoneyDropRow struct {
	MapNumber, MonsterIndex, MonsterMinLevel, MonsterMaxLevel, AccountLevel int
	MoneyAmount, MoneyDropRate                                              int
	Comment                                                                 string `toml:",omitempty"`
}

type ItemLevelFile struct {
	Rows []ItemLevelRow `toml:",inline"`
}

type ItemLevelRow struct {
	ItemIndex string
	ItemLevel int
	Name      string
	Comment   string `toml:",omitempty"`
}

type ItemLootFile struct {
	Rows []ItemLootRow `toml:",inline"`
}

type ItemLootRow struct {
	ItemIndex            string
	ItemLevel, AllowLoot int
	Comment              string `toml:",omitempty" legacy:"comment"`
}

type ItemMoveFile struct {
	Rows []ItemMoveRow `toml:",inline"`
}

type ItemMoveRow struct {
	ItemIndex                                    string
	AllowDrop, AllowSell, AllowTrade, AllowVault int
	Comment                                      string `toml:",omitempty" legacy:"comment"`
}

type ItemStackFile struct {
	Stack []ItemStackRow     `toml:",inline" comment:"section 0"`
	Pack  []ItemStackPackRow `toml:",inline" comment:"section 1: ItemCount items of ItemIndex become one NextItemIndex"`
}

type ItemStackRow struct {
	ItemIndex           string
	ItemLevel, MaxStack int
	Comment             string `toml:",omitempty" legacy:"comment"`
}

type ItemStackPackRow struct {
	ItemIndex     string
	ItemCount     int
	NextItemIndex string
	Comment       string `toml:",omitempty" legacy:"comment"`
}

type ItemValueFile struct {
	Rows []ItemValueRow `toml:",inline"`
}

type ItemValueRow struct {
	ItemIndex            string
	ItemLevel, ItemGrade int
	MoneyValue           int64
	Comment              string `toml:",omitempty" legacy:"comment"`
}

type SetItemTypeFile struct {
	Rows []SetItemTypeRow `toml:",inline"`
}

type SetItemTypeRow struct {
	ItemIndex                  string
	StatType                   int
	OptionIndex1, OptionIndex2 int
	ItemDropRate               int
	Comment                    string `toml:",omitempty"`
}

type SetItemOptionFile struct {
	Rows []SetItemOptionRow `toml:",inline"`
}

type SetItemOptionRow struct {
	Index      int
	Name       string
	Option     [12][2]int
	FullOption [5][2]int
	Comment    string `toml:",omitempty"`
}

type WingOptionFile struct {
	Rows []WingOptionRow `toml:",inline"`
}

type WingOptionRow struct {
	ItemIndex                        string
	DefenseConstA                    int
	IncDamageConstA, IncDamageConstB int
	DecDamageConstA, DecDamageConstB int
	MaxNewOption                     int
	Comment                          string `toml:",omitempty" legacy:"comment"`
}

func loadItemConfig(configDir string) (*ItemConfig, error) {
	return loadConfig[ItemConfig](configDir, "item")
}
