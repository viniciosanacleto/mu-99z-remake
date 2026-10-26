package gamedata

// CharacterConfig is config/data/character.toml: Data/Character/*.txt.
type CharacterConfig struct {
	DefaultClassInfo     DefaultClassInfoFile     `comment:"DefaultClassInfo.txt (CDefaultClassInfo): Class 0 DW, 1 DK, 2 FE, 3 MG, 4 DL"`
	DefaultClassFreebies DefaultClassFreebiesFile `comment:"DefaultClassFreebies.txt: what a new character gets"`
}

type DefaultClassInfoFile struct {
	Rows []DefaultClassInfoRow `toml:",inline"`
}

type DefaultClassInfoRow struct {
	Class, Strength, Dexterity, Vitality, Energy, Leadership int
	MaxLife, MaxMana, LevelLife, LevelMana                   float64
	VitalityToLife, EnergyToMana                             float64
	Comment                                                  string `toml:",omitempty"`
}

type DefaultClassFreebiesFile struct {
	Class  []FreebieClassRow  `toml:",inline" comment:"section 0"`
	Item   []FreebieItemRow   `toml:",inline" comment:"section 1: Class 0~4 (other rows are ignored); Duration in seconds"`
	Effect []FreebieEffectRow `toml:",inline" comment:"section 2: EffectList effects; Duration in seconds"`
}

type FreebieClassRow struct {
	Class, InitialMoney, InitialPoint int
	Comment                           string `toml:",omitempty"`
}

type FreebieItemRow struct {
	Class                                                                int
	ItemIndex                                                            string
	ItemLevel, Durability, SkillSwitch, LuckSwitch, Aditional, Excellent int
	SetOption, Duration                                                  int
	Comment                                                              string `toml:",omitempty"`
}

type FreebieEffectRow struct {
	Class, EffectIndex, EffectType int
	Power                          [4]int
	Duration                       int
	Comment                        string `toml:",omitempty"`
}

// LoadCharacterConfig reads config/data/character.toml.
func LoadCharacterConfig(configDir string) (*CharacterConfig, error) {
	return loadConfig[CharacterConfig](configDir, "character")
}
