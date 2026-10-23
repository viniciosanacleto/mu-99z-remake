package gamedata

import (
	"fmt"
	"path/filepath"
	"strconv"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/config/mutext"
)

// ItemInfo is one Item/Item.txt row (CItemManager). Columns that a
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

// itemColumns lists, per category, the column names after the common
// "Type Slot Skill Width Height HaveSerial HaveOption DropItem Name" prefix
// (headers of Item.txt).
var itemColumns = map[int][]string{
	0: weaponColumns, 1: weaponColumns, 2: weaponColumns, 3: weaponColumns, 4: weaponColumns, 5: weaponColumns,
	6: {"Level", "Defense", "DefenseSuccessRate", "Durability", "ReqLevel", "ReqStrength", "ReqDexterity", "ReqEnergy", "ReqVitality", "ReqLeadership", "Attr", "DW", "DK", "FE", "MG", "DL"},
	7: armorColumns, 8: armorColumns, 9: armorColumns,
	10: {"Level", "Defense", "AttackSpeed", "Durability", "ReqLevel", "ReqStrength", "ReqDexterity", "ReqEnergy", "ReqVitality", "ReqLeadership", "Attr", "DW", "DK", "FE", "MG", "DL"},
	11: {"Level", "Defense", "WalkSpeed", "Durability", "ReqLevel", "ReqStrength", "ReqDexterity", "ReqEnergy", "ReqVitality", "ReqLeadership", "Attr", "DW", "DK", "FE", "MG", "DL"},
	12: {"Level", "Defense", "Durability", "ReqLevel", "ReqEnergy", "ReqStrength", "ReqDexterity", "ReqLeadership", "BuyMoney", "DW", "DK", "FE", "MG", "DL"},
	13: {"Level", "Durability", "Resistance1", "Resistance2", "Resistance3", "Resistance4", "Resistance5", "Resistance6", "Resistance7", "Attr", "DW", "DK", "FE", "MG", "DL"},
	14: {"Value", "Level"},
	15: {"Level", "ReqLevel", "ReqEnergy", "BuyMoney", "DW", "DK", "FE", "MG", "DL"},
}

var weaponColumns = []string{"Level", "DamageMin", "DamageMax", "AttackSpeed", "Durability", "MagicDurability",
	"MagicDamageRate", "ReqLevel", "ReqStrength", "ReqDexterity", "ReqEnergy", "ReqVitality", "ReqLeadership",
	"Attr", "DW", "DK", "FE", "MG", "DL"}

var armorColumns = []string{"Level", "Defense", "MagicDefense", "Durability", "ReqLevel", "ReqStrength",
	"ReqDexterity", "ReqEnergy", "ReqVitality", "ReqLeadership", "Attr", "DW", "DK", "FE", "MG", "DL"}

func (it *ItemInfo) set(col string, v int) {
	switch col {
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

// LoadItems reads Data/Item/Item.txt. The file is a sequence of numbered
// category sections; categories may appear more than once (the WizardTeam
// additions at the end) and later rows override earlier ones.
func LoadItems(dataDir string) (map[int]ItemInfo, error) {
	lines, err := mutext.LoadFile(filepath.Join(dataDir, "Item", "Item.txt"))
	if err != nil {
		return nil, err
	}
	secs, err := mutext.Sections(lines)
	if err != nil {
		return nil, fmt.Errorf("Item.txt: %w", err)
	}
	out := map[int]ItemInfo{}
	for _, s := range secs {
		cols, ok := itemColumns[s.Index]
		if !ok {
			return nil, fmt.Errorf("Item.txt: unknown category %d", s.Index)
		}
		for _, l := range s.Rows {
			if len(l.Tokens) != 9+len(cols) {
				return nil, fmt.Errorf("Item.txt line %d: %d columns, want %d", l.No, len(l.Tokens), 9+len(cols))
			}
			var it ItemInfo
			it.Category = s.Index
			head := [8]int{}
			for i := 0; i < 8; i++ {
				if head[i], err = l.Int(i); err != nil {
					return nil, err
				}
			}
			it.Type, it.Slot, it.Skill, it.Width, it.Height = head[0], head[1], head[2], head[3], head[4]
			it.HaveSerial, it.HaveOption, it.DropItem = head[5], head[6], head[7]
			it.Name = l.Tokens[8]
			for i, c := range cols {
				v, err := l.Int(9 + i)
				if err != nil {
					return nil, err
				}
				it.set(c, v)
			}
			if it.Type < 0 || it.Type >= 512 {
				return nil, fmt.Errorf("Item.txt line %d: type %d", l.No, it.Type)
			}
			it.Index = s.Index*512 + it.Type
			out[it.Index] = it
		}
	}
	return out, nil
}

// ItemName formats an index as "cc,iii" for logs.
func ItemName(index int) string {
	return strconv.Itoa(index/512) + "," + strconv.Itoa(index%512)
}
