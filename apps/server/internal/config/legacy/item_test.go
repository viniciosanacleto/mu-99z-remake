package legacy

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"
)

// convertGroup writes the original files under Data/, converts one group
// and returns the config directory holding data/<group>.toml. Files of the
// group the test does not give are created empty.
func convertGroup(t *testing.T, group string, files map[string]string) string {
	t.Helper()
	dst, err := tryConvertGroup(t, group, files)
	if err != nil {
		t.Fatal(err)
	}
	return dst
}

func tryConvertGroup(t *testing.T, group string, files map[string]string) (string, error) {
	t.Helper()
	tree := map[string]string{}
	for name, body := range files {
		tree["Data/"+name] = body
	}
	root := writeTree(t, tree)
	dst := t.TempDir()
	for i := 0; ; i++ {
		_, err := Convert(root, dst, map[string]bool{group: true})
		var pe *fs.PathError
		if i > 200 || !errors.As(err, &pe) || !errors.Is(err, fs.ErrNotExist) {
			if err == nil {
				_, err = os.Stat(filepath.Join(dst, "data", group+".toml"))
			}
			return dst, err
		}
		if filepath.Ext(pe.Path) == "" {
			err = os.MkdirAll(pe.Path, 0o755)
		} else if err = os.MkdirAll(filepath.Dir(pe.Path), 0o755); err == nil {
			err = os.WriteFile(pe.Path, nil, 0o644)
		}
		if err != nil {
			t.Fatal(err)
		}
	}
}

var itemFiles = map[string]string{
	"Item/Item.txt": "0\n" +
		"//Type Slot Skill Width Height HaveSerial HaveOption DropItem Name Level DamageMin DamageMax AttackSpeed Durability MagicDurability MagicDamageRate ReqLevel ReqStrength ReqDexterity ReqEnergy ReqVitality ReqLeadership Attr DW DK FE MG DL\n" +
		"0 0 0 1 2 1 1 1 \"Kris\" 6 6 11 50 20 0 0 0 40 40 0 0 0 1 1 1 1 1 1\n" +
		"end\n" +
		"12\n" +
		"203 7 0 4 3 1 1 1 \"Capa del Se\xf1or\" 153 163 250 400 0 0 0 0 0 1 1 1 1 1 // cape\n" +
		"end\n" +
		"14\n" +
		"13 * 0 1 1 0 0 1 \"Jewel of Bless\" 9000000 0\n" +
		"end\n" +
		"0\n" +
		"0 0 0 1 2 1 1 1 \"Kris+\" 7 6 11 50 20 0 0 0 40 40 0 0 0 1 1 1 1 1 1\n" +
		"end\n",
	"Item/ItemOption.txt":          "1 84 5 00,000 04,511 * 1 * * //General Weapon\nend\n",
	"Item/ItemOptionRate.txt":      "0\n0 7000 1500 1000 500 0 0 0 0 0 0 0 0 0 0 0 0 //Level\n0 1 1 1 1 1 1 1 1 1 1 1 1 1 1 1 1\nend\n1\n0 10000 0\nend\n9\n0 1\nend\n",
	"Item/ExcellentOptionRate.txt": "1 00,000 04,511 \"Add Hunt MP\" 60 //General Weapon\nend\n",
	"Item/ItemDrop.txt":            "12,015 0 0 * * * N N N 0 * * 12 150 500 940 360 200 //Jewel of Chaos\nend\n",
	"Item/ItemMoneyDrop.txt":       "* * * * * * 1000 // Zen\nend\n",
	"Item/ItemLevel.txt":           "12,011 2 \"Assassin Summoning Orb\"\nend\n",
	"Item/ItemLoot.txt":            "14,023 * 0 //Scroll of the Emperor\nend\n",
	"Item/ItemMove.txt":            "12,000 0 1 1 1 //Wings of Elf\nend\n",
	"Item/ItemStack.txt":           "0\n14,000 * 255 //Apple\nend\n1\n14,021 10 14,028\nend\n",
	"Item/ItemValue.txt":           "04,007 1 * 1400 //Bolt 1\nend\n",
	"Item/SetItemType.txt":         "00,000 0 2 0 10000 //\n00,000 1 3 0 5\nend\n",
	"Item/SetItemOption.txt":       "1 \"Warrior\" 0 10 * * 19 10 * * 13 20 * * 14 5 * * 10 20 * * 1 2 * * 3 4 5 6 7 8 9 10 11 12 13 14\nend\n",
	"Item/WingOption.txt":          "12,000 3 112 2 88 2 0 //Wings of Elf\nend\n",
}

func TestItemGroup(t *testing.T) {
	cfg := convertGroup(t, "item", itemFiles)

	items, err := gamedata.LoadItems(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if k := items[0]; k.Name != "Kris+" || k.Level != 7 || k.DamageMax != 11 || k.Class != [5]int{1, 1, 1, 1, 1} {
		t.Fatalf("later row of a repeated category must win: %+v", k)
	}
	if c := items[12*512+203]; c.Name != "Capa del Se\xf1or" || c.Defense != 163 || c.ReqLevel != 400 || c.Durability != 250 {
		t.Fatalf("wing = %+v", c)
	}
	if j := items[14*512+13]; j.Slot != -1 || j.Value != 9000000 || j.Level != 0 {
		t.Fatalf("jewel = %+v", j)
	}

	opts, err := gamedata.LoadItemOptions(cfg)
	if err != nil || len(opts[1]) != 1 || opts[1][0] != (gamedata.ItemOptionRule{Index: 1, OptionIndex: 84, Value: 5,
		ItemMin: 0, ItemMax: 4*512 + 511, Skill: -1, Luck: 1, Additional: -1, Excellent: -1}) {
		t.Fatalf("options = %+v %v", opts, err)
	}

	rates, err := gamedata.LoadItemOptionRate(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if got := rates.Sections[0][0]; got[0] != 7000 || got[3] != 500 || len(got) != 16 {
		t.Fatalf("first row of an index wins: %v", got)
	}
	if got := rates.Sections[1][0]; !reflect.DeepEqual(got, []int{10000, 0}) {
		t.Fatalf("skill rates = %v", got)
	}

	exc, err := gamedata.LoadExcellentOptionRate(cfg)
	if err != nil || len(exc) != 1 || exc[0] != (gamedata.ExcellentOptionRate{Option: 1, Min: 0, Max: 4*512 + 511, Name: "Add Hunt MP", Rate: 60}) {
		t.Fatalf("exc = %+v %v", exc, err)
	}

	drops, err := gamedata.LoadItemDrop(cfg)
	want := gamedata.ItemDropRule{Index: 12*512 + 15, Option: [6]int{-1, -1, -1, -1, -1, -1}, Map: -1, Monster: -1,
		MinLevel: 12, MaxLevel: 150, Rate: [4]int{500, 940, 360, 200}}
	if err != nil || len(drops) != 1 || drops[0] != want {
		t.Fatalf("drop (\"N\" repeats the previous number) = %+v %v", drops, err)
	}

	money, err := gamedata.LoadItemMoneyDrop(cfg)
	if err != nil || len(money) != 1 || money[0] != (gamedata.ItemMoneyDropRule{Map: -1, Monster: -1, MinLevel: -1, MaxLevel: -1, AccountLevel: -1, Amount: -1, Rate: 1000}) {
		t.Fatalf("money = %+v %v", money, err)
	}

	lv, err := gamedata.LoadItemLevel(cfg)
	if err != nil || len(lv) != 1 || lv[0] != (gamedata.ItemLevelName{Index: 12*512 + 11, Level: 2, Name: "Assassin Summoning Orb"}) {
		t.Fatalf("levels = %+v %v", lv, err)
	}

	loot, err := gamedata.LoadItemLoot(cfg)
	if err != nil || len(loot) != 1 || loot[0] != (gamedata.ItemLootRule{Index: 14*512 + 23, Level: -1}) {
		t.Fatalf("loot = %+v %v", loot, err)
	}

	mv, err := gamedata.LoadItemMove(cfg)
	if err != nil || mv[12*512] != (gamedata.ItemMoveRule{AllowSell: true, AllowTrade: true, AllowVault: true}) {
		t.Fatalf("move = %+v %v", mv, err)
	}

	st, err := gamedata.LoadItemStack(cfg)
	if err != nil || st.Max(14*512, 3) != 255 || len(st.Packs) != 1 || st.Packs[0] != (gamedata.ItemStackPack{Index: 14*512 + 21, Count: 10, Next: 14*512 + 28}) {
		t.Fatalf("stack = %+v %v", st, err)
	}

	val, err := gamedata.LoadItemValue(cfg)
	if err != nil || len(val) != 1 || val[0] != (gamedata.ItemValueRule{Index: 4*512 + 7, Level: 1, Grade: -1, Value: 1400}) {
		t.Fatalf("value = %+v %v", val, err)
	}

	types, err := gamedata.LoadSetItemType(cfg)
	if err != nil || len(types.Order) != 1 || types.ByIndex[0].Option != [2]int{2, 0} {
		t.Fatalf("set types (first row wins) = %+v %v", types, err)
	}

	sets, err := gamedata.LoadSetItemOption(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if w := sets[1]; w.Name != "Warrior" || w.Tier[0] != [2]gamedata.SetOptionPair{{Code: 0, Value: 10}, {Code: -1, Value: -1}} ||
		w.Tier[5] != [2]gamedata.SetOptionPair{{Code: 1, Value: 2}, {Code: -1, Value: -1}} || w.Full[4] != (gamedata.SetOptionPair{Code: 11, Value: 12}) || sets[0].Index != -1 {
		t.Fatalf("set option = %+v", w)
	}

	wings, err := gamedata.LoadWingOptions(cfg)
	if err != nil || wings[12*512].IncDamageConstA != 112 {
		t.Fatalf("wings = %+v %v", wings, err)
	}
}

func TestItemGroupErrors(t *testing.T) {
	for name, body := range map[string]string{
		"Item/Item.txt":           "16\n0 0 0 1 1 0 0 1 \"x\" 1 1\nend\n", // unknown category
		"Item/ItemStack.txt":      "0\n14,000 *\nend\n",                   // short row
		"Item/SetItemOption.txt":  "1 \"Warrior\" 0 10\nend\n",            // short row
		"Item/ItemLevel.txt":      "12,011 2\nend\n",                      // no name
		"Item/ItemOptionRate.txt": "0\n0 1 2\n",                           // missing end
	} {
		files := map[string]string{}
		for k, v := range itemFiles {
			files[k] = v
		}
		files[name] = body
		if _, err := tryConvertGroup(t, "item", files); err == nil {
			t.Errorf("%s: bad file accepted", name)
		}
	}
}
