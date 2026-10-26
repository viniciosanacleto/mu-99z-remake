package gamedata

import (
	"fmt"
	"reflect"
	"strconv"
)

// ItemInfo is one Item.txt row (CItemManager). Columns that a
// category does not have stay zero. "*" reads as -1.
type ItemInfo struct {
	Index      int // category*512 + Type
	Category   int
	Type       int
	Slot       int // -1 = not equippable
	Skill      int
	Width      int
	Height     int
	HaveSerial int
	HaveOption int
	DropItem   int
	Name       string
	Level      int

	// Weapons (0-5).
	DamageMin, DamageMax int
	AttackSpeed          int // also gloves (10)
	MagicDurability      int
	MagicDamageRate      int

	// Shields (6), armor (7-11), wings (12).
	Defense            int
	DefenseSuccessRate int // shields
	MagicDefense       int // helms, armors, pants
	WalkSpeed          int // boots

	Durability int

	// Pets/rings (13).
	Resistance [7]int

	// Consumables (14).
	Value int

	ReqLevel      int
	ReqStrength   int
	ReqDexterity  int
	ReqEnergy     int
	ReqVitality   int
	ReqLeadership int
	BuyMoney      int // wings (12), scrolls (15)
	Attr          int
	Class         [5]int // DW DK FE MG DL
}

func (it *ItemInfo) set(col string, v int) {
	switch col {
	case "Type":
		it.Type = v
	case "Slot":
		it.Slot = v
	case "Skill":
		it.Skill = v
	case "Width":
		it.Width = v
	case "Height":
		it.Height = v
	case "HaveSerial":
		it.HaveSerial = v
	case "HaveOption":
		it.HaveOption = v
	case "DropItem":
		it.DropItem = v
	case "Level":
		it.Level = v
	case "DamageMin":
		it.DamageMin = v
	case "DamageMax":
		it.DamageMax = v
	case "AttackSpeed":
		it.AttackSpeed = v
	case "Durability":
		it.Durability = v
	case "MagicDurability":
		it.MagicDurability = v
	case "MagicDamageRate":
		it.MagicDamageRate = v
	case "Defense":
		it.Defense = v
	case "DefenseSuccessRate":
		it.DefenseSuccessRate = v
	case "MagicDefense":
		it.MagicDefense = v
	case "WalkSpeed":
		it.WalkSpeed = v
	case "Value":
		it.Value = v
	case "ReqLevel":
		it.ReqLevel = v
	case "ReqStrength":
		it.ReqStrength = v
	case "ReqDexterity":
		it.ReqDexterity = v
	case "ReqEnergy":
		it.ReqEnergy = v
	case "ReqVitality":
		it.ReqVitality = v
	case "ReqLeadership":
		it.ReqLeadership = v
	case "BuyMoney":
		it.BuyMoney = v
	case "Attr":
		it.Attr = v
	case "DW":
		it.Class[0] = v
	case "DK":
		it.Class[1] = v
	case "FE":
		it.Class[2] = v
	case "MG":
		it.Class[3] = v
	case "DL":
		it.Class[4] = v
	default:
		if len(col) == 11 && col[:10] == "Resistance" {
			it.Resistance[col[10]-'1'] = v
		}
	}
}

// LoadItems reads the Item table of config/data/item.toml (Item.txt).
// Later rows of a category override earlier ones with the same Type.
func LoadItems(configDir string) (map[int]ItemInfo, error) {
	f, err := loadItemConfig(configDir)
	if err != nil {
		return nil, err
	}
	return BuildItems(&f.Item)
}

// BuildItems converts the Item table into ItemInfo by index.
func BuildItems(t *ItemTable) (map[int]ItemInfo, error) {
	out := map[int]ItemInfo{}
	for cat, rows := range t.Categories() {
		for i := 0; i < rows.Len(); i++ {
			it := ItemInfo{Category: cat}
			setItemFields(&it, rows.Index(i))
			if it.Type < 0 || it.Type >= 512 {
				return nil, fmt.Errorf("Item category %d row %d: type %d", cat, i+1, it.Type)
			}
			it.Index = cat*512 + it.Type
			out[it.Index] = it
		}
	}
	return out, nil
}

// Categories returns the arrays of the table by category number.
func (t *ItemTable) Categories() [16]reflect.Value {
	v := reflect.ValueOf(t).Elem()
	var out [16]reflect.Value
	for i := range out {
		out[i] = v.Field(i)
	}
	return out
}

// setItemFields copies a category row into it by column name.
func setItemFields(it *ItemInfo, row reflect.Value) {
	t := row.Type()
	for i := 0; i < t.NumField(); i++ {
		f, fv := t.Field(i), row.Field(i)
		switch {
		case f.Anonymous:
			setItemFields(it, fv)
		case f.Name == "Name":
			it.Name = fv.String()
		case fv.Kind() == reflect.Int:
			it.set(f.Name, int(fv.Int()))
		}
	}
}

// ItemName formats an index as "cc,iii" for logs.
func ItemName(index int) string {
	return strconv.Itoa(index/512) + "," + strconv.Itoa(index%512)
}
