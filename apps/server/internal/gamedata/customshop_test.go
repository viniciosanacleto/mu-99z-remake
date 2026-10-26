package gamedata

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadCustomJewelOriginal(t *testing.T) {
	// The original file only has commented demonstration rows.
	c, err := LoadCustomJewel(dataDir())
	if err != nil {
		t.Fatal(err)
	}
	for i, j := range c.Slots {
		if j.Index != -1 {
			t.Fatalf("slot %d loaded %+v", i, j)
		}
	}
	if _, ok := c.Find(14*512 + 200); ok {
		t.Fatal("commented jewel found")
	}
}

func writeShopFile(t *testing.T, name, body string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "Custom"), 0o755); err != nil {
		t.Fatal(err)
	}
	putOriginal(t, filepath.Join(dir, "Custom", name), body)
	return dir
}

func TestLoadCustomJewelSections(t *testing.T) {
	dir := writeShopFile(t, "CustomJewel.txt", `0
0   14,200  1  0  1   60 61 62 63   50000000
3   14 201  0  1  0   10 20 30 40   7 // two tokens for the item
60  14,202  1  1  1   1 1 1 1 1
end
1
0   0  12  0  *  *  6  *  15  1
end
2
0   1  0  1  2  1  5
end
3
0   2  1  0  1  6  1
end
`)
	c, err := LoadCustomJewel(dir)
	if err != nil {
		t.Fatal(err)
	}
	j, ok := c.Find(14*512 + 200)
	if !ok || j.Index != 0 || j.Weapon != 1 || j.Armor != 0 || j.Wing != 1 || j.Rate != [4]int{60, 61, 62, 63} ||
		j.SalePrice != 50000000 {
		t.Fatalf("slot 0 %+v", j)
	}
	if j.MinLevel != 0 || j.MaxLevel != 12 || j.Skill != 0 || j.Luck != -1 || j.MinOption != -1 ||
		j.MaxOption != 6 || j.MinExc != -1 || j.MaxExc != 15 || j.MaxSetOption != 1 {
		t.Fatalf("requirements %+v", j)
	}
	if j.Success != (JewelChange{1, 0, 1, 2, 1, 5}) || j.Failure != (JewelChange{2, 1, 0, 1, 6, 1}) {
		t.Fatalf("changes %+v %+v", j.Success, j.Failure)
	}
	if j, ok := c.Find(14*512 + 201); !ok || j.Index != 3 || j.SalePrice != 7 {
		t.Fatalf("slot 3 %+v", j)
	}
	// Index 60 is beyond the 50 slots.
	if _, ok := c.Find(14*512 + 202); ok {
		t.Fatal("index 60 loaded")
	}
}

func TestLoadLuckyWheelOriginal(t *testing.T) {
	w, err := LoadLuckyWheel(dataDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(w) != 12 {
		t.Fatalf("%d rows", len(w))
	}
	if r := w[1]; r != (WheelReward{Index: 1, Type: 14, Item: 11, Level: 10}) {
		t.Fatalf("row 1 %+v", r)
	}
	if r := w[11]; r.Item != 11 || r.Level != 12 {
		t.Fatalf("row 11 %+v", r)
	}
}

func TestLoadLuckyWheelColumns(t *testing.T) {
	dir := writeShopFile(t, "CustomLuckyWheel.txt", "0 12 3 4 1 0 5 32\n0 1 1 1 1 1 1 1\nend\n")
	w, err := LoadLuckyWheel(dir)
	if err != nil {
		t.Fatal(err)
	}
	// Skill is the fifth column and Luck the sixth; the first row wins.
	if r := w[0]; r != (WheelReward{Index: 0, Type: 12, Item: 3, Level: 4, Skill: 1, Luck: 0, Option: 5, Excellent: 32}) {
		t.Fatalf("row %+v", r)
	}
}

func TestLoadXShopPricesOriginal(t *testing.T) {
	x, ok, err := LoadXShopPrices(filepath.Join(dataDir(), "Custom", "CustomXShop.json"))
	if err != nil || !ok {
		t.Fatal(ok, err)
	}
	for i, p := range x.Category {
		if p != 100 {
			t.Fatalf("category %d = %d", i, p)
		}
	}
	if x.Price(0) != 20 || x.Price(1) != 10 || x.Price(3) != 12 {
		t.Fatalf("prices %d %d %d", x.Price(0), x.Price(1), x.Price(3))
	}
	if len(x.Overrides) < 100 {
		t.Fatalf("%d overrides", len(x.Overrides))
	}
}

func TestLoadXShopPricesMissingAndNegative(t *testing.T) {
	x, ok, err := LoadXShopPrices(filepath.Join(t.TempDir(), "none.json"))
	if err != nil || ok || x.Price(5*512) != XShopDefaultPrice || x.Price(0x1fff) != XShopDefaultPrice {
		t.Fatalf("missing %v %v %+v", ok, err, x)
	}
	dir := writeShopFile(t, "CustomXShop.txt", "0\n3 -5\n16 9\nend\n1\n3 2 -1\n0 600 7\nend\n")
	x, _, err = LoadXShopPrices(filepath.Join(dir, "Custom", "CustomXShop.json"))
	if err != nil {
		t.Fatal(err)
	}
	if x.Category[3] != 0 || x.Price(3*512+2) != 0 || x.Price(3*512+3) != 0 {
		t.Fatalf("negative prices %+v", x)
	}
	// 0,600 is 600 = 1,88: the original adds Type*512 + Index without
	// checking Index.
	if x.Price(600) != 7 {
		t.Fatalf("overflowing index %d", x.Price(600))
	}
}

func TestWriteXShopPrice(t *testing.T) {
	dir := writeShopFile(t, "CustomXShop.txt",
		"// header\r\n0\r\n0 100\r\nend\r\n\r\n1\r\n// comment\r\n0   0   20   // Kris\r\n0 1 10\r\nend\r\n")
	path := filepath.Join(dir, "Custom", "CustomXShop.json")
	if err := WriteXShopPrice(path, 0, 55); err != nil {
		t.Fatal(err)
	}
	if err := WriteXShopPrice(path, 1, 11); err != nil {
		t.Fatal(err)
	}
	if err := WriteXShopPrice(path, 7*512+3, 99); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	want := `{
  "sections": [
    {"index": 0, "rows": [
      {"Col1": 0, "Col2": 100}
    ]},
    {"index": 1, "rows": [
      {"Col1": 0, "Col2": 0, "Col3": 55},
      {"Col1": 0, "Col2": 1, "Col3": 11},
      {"Type": 7, "Index": 3, "Preco": 99}
    ]}
  ]
}
`
	if string(b) != want {
		t.Fatalf("file:\n%s\nwant\n%s", b, want)
	}
	x, _, _ := LoadXShopPrices(path)
	if x.Price(0) != 55 || x.Price(1) != 11 || x.Price(7*512+3) != 99 {
		t.Fatalf("reloaded %+v", x.Overrides)
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Fatal("temporary file left")
	}
}

func TestWriteXShopPriceWithoutSection(t *testing.T) {
	dir := writeShopFile(t, "CustomXShop.txt", "0\n0 100\nend\n")
	path := filepath.Join(dir, "Custom", "CustomXShop.json")
	if err := WriteXShopPrice(path, 2*512+1, 30); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	if !strings.HasSuffix(string(b), `{"index": 1, "rows": [
      {"Type": 2, "Index": 1, "Preco": 30}
    ]}
  ]
}
`) {
		t.Fatalf("%s", b)
	}
	if err := WriteXShopPrice(filepath.Join(dir, "missing.json"), 0, 1); err == nil {
		t.Fatal("missing file written")
	}
}
