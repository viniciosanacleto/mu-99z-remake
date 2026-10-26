package legacy

import (
	"testing"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"
)

func TestShopGroup(t *testing.T) {
	cfg := convertGroup(t, "shop", map[string]string{
		"ShopManager.txt": "253 0 127 86 1 1 1 1 1 * \"253 - Potion Girl Amy\"\n" +
			"254 0 120 80 1 1 0 1 1 3 \"253 - Potion Girl Amy\"\nend\n",
		"Shop/253 - Potion Girl Amy.txt": "14,000 0 050 0 0 0 0 0 //Apple\n99,999 0 1 1 1 4 63 0 extra\nend\n",
		"Shop/239 - Arena Guard.txt":     "not referenced, not read",
	})
	shops, err := gamedata.LoadShops(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(shops) != 2 || shops[1].Number != 1 || shops[1].GMLevel != 3 || shops[1].AL != [4]bool{true, false, true, true} ||
		shops[0].GMLevel != -1 || shops[0].X != 127 {
		t.Fatalf("shops = %+v", shops)
	}
	it := shops[0].Items
	if len(it) != 2 || it[0].Index != 14*512 || it[0].Durability != 50 || it[1].Index != -1 || !it[1].Skill || it[1].Excellent != 63 {
		t.Fatalf("items (out-of-range index = -1) = %+v", it)
	}
	root := writeTree(t, map[string]string{"Data/ShopManager.txt": "253 0 127 86 1 1 1 1 1 * \"Missing\"\nend\n"})
	if _, err := Convert(root, t.TempDir(), map[string]bool{"shop": true}); err == nil {
		t.Fatal("missing shop file accepted")
	}
}
