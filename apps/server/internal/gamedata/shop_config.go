package gamedata

// ShopConfig is config/data/shop.toml: Data/ShopManager.txt and the
// Data/Shop/*.txt files it names (keyed by ShopPath).
type ShopConfig struct {
	ShopManager ShopManagerFile     `comment:"ShopManager.txt: the shop NPCs; AL0..AL3 enable the shop per account level; GMLevel -1 = any"`
	Shop        map[string]ShopFile `comment:"Shop/<ShopPath>.txt: Durability 0 = the item's maximum"`
}

type ShopManagerFile struct {
	Rows []ShopManagerRow `toml:",inline"`
}

type ShopManagerRow struct {
	NPCIndex, MapNumber, LocationX, LocationY, Direction int
	AL0, AL1, AL2, AL3, GMLevel                          int
	ShopPath                                             string
	Comment                                              string `toml:",omitempty" legacy:"comment"`
}

type ShopFile struct {
	Rows []ShopRow `toml:",inline"`
}

type ShopRow struct {
	ItemIndex                                                            string
	ItemLevel, Durability, SkillSwitch, LuckSwitch, Aditional, Excellent int
	SetOption                                                            int
	Comment                                                              string `toml:",omitempty" legacy:"comment"`
}

// LoadShopConfig reads config/data/shop.toml.
func LoadShopConfig(configDir string) (*ShopConfig, error) {
	return loadConfig[ShopConfig](configDir, "shop")
}
