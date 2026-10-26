package gamedata

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadCustomJewelOriginal(t *testing.T) {
	// The original file only has commented demonstration rows.
	c, err := LoadCustomJewel(configDir())
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

func TestLoadLuckyWheelOriginal(t *testing.T) {
	w, err := LoadLuckyWheel(configDir())
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

func TestLoadXShopPricesOriginal(t *testing.T) {
	x, ok, err := LoadXShopPrices(filepath.Join(configDir(), "data", "xshop.toml"))
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

// writeXShop writes an xshop.toml and returns its path.
func writeXShop(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "xshop.toml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

const xshopSample = `# header
[CustomXShop]
Category = [
  { Type = 0, Price = 100 },
  { Type = 3, Price = -5 },
  { Type = 16, Price = 9 },
]
Item = [
  { Type = 0, Index = 0, Price = 20, Comment = "Kris" },
  { Type = 0, Index = 1, Price = 10 },
  { Type = 3, Index = 2, Price = -1 },
  { Type = 0, Index = 600, Price = 7 },
]

[CustomXShopRecharge]
Ativo = 1
Titular = "Jo\u00e3o"
ChavePix = ""
CopiaECola = ""
ImagemURL = ""
ImagemBMP = ""
Valor = ""
`

func TestLoadXShopPricesMissingAndNegative(t *testing.T) {
	x, ok, err := LoadXShopPrices(filepath.Join(t.TempDir(), "none.toml"))
	if err != nil || ok || x.Price(5*512) != XShopDefaultPrice || x.Price(0x1fff) != XShopDefaultPrice {
		t.Fatalf("missing %v %v %+v", ok, err, x)
	}
	x, _, err = LoadXShopPrices(writeXShop(t, xshopSample))
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
	path := writeXShop(t, xshopSample)
	for _, c := range []struct {
		index int
		price uint32
	}{{0, 55}, {1, 11}, {7*512 + 3, 99}} {
		if err := WriteXShopPrice(path, c.index, c.price); err != nil {
			t.Fatal(err)
		}
	}
	b, _ := os.ReadFile(path)
	for _, want := range []string{"# header\n", `Price = 55, Comment = "Kris"`, `Type = 7, Index = 3,   Price = 99, Comment = "alterado/criado ingame por GM"`,
		`Titular = "João"`} {
		if !strings.Contains(string(b), want) {
			t.Fatalf("%q missing from:\n%s", want, b)
		}
	}
	x, _, err := LoadXShopPrices(path)
	if err != nil || x.Price(0) != 55 || x.Price(1) != 11 || x.Price(7*512+3) != 99 {
		t.Fatalf("reloaded %+v %v", x.Overrides, err)
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Fatal("temporary file left")
	}
	if err := WriteXShopPrice(filepath.Join(t.TempDir(), "missing.toml"), 0, 1); err == nil {
		t.Fatal("missing file written")
	}
}

func TestWriteXShopRecharge(t *testing.T) {
	path := writeXShop(t, xshopSample)
	r := XShopRechargeFile{Ativo: 1, Titular: "Jos\xe9", ChavePix: "key", ImagemBMP: "pix1.tmp.bmp", Valor: "25.00"}
	if err := WriteXShopRecharge(path, r); err != nil {
		t.Fatal(err)
	}
	f, err := LoadXShopConfig(path)
	if err != nil || f.CustomXShopRecharge != r {
		t.Fatalf("recharge %+v %v", f.CustomXShopRecharge, err)
	}
	if x, _, _ := LoadXShopPrices(path); x.Price(0) != 20 {
		t.Fatal("prices lost")
	}
	b, _ := os.ReadFile(path)
	if !strings.Contains(string(b), `Titular = "José"`) {
		t.Fatalf("file:\n%s", b)
	}
	// A missing file is created.
	other := filepath.Join(t.TempDir(), "new.toml")
	if err := WriteXShopRecharge(other, r); err != nil {
		t.Fatal(err)
	}
}
