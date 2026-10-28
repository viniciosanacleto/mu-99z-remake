package game

import (
	"fmt"
	"testing"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/chaosmix"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/item"
)

// announceText is Message.txt 500 with FUN_005120b0's arguments.
func announceText(b *bareWorld, name string, index int) string {
	return fmt.Sprintf(b.messageFor(nil, 500), name, b.itemName(index, 13), 13)
}

// FUN_005120b0: only type 3 (+13), only with ChaosMixPlusItemAnnounce, and
// the AnnounceMinLevelChaosMix limit is compared with the item index.
func TestChaosAnnounceRules(t *testing.T) {
	b := newBareWorld(t)
	o := chaosPlayer(b)
	staff := item.Item{Index: item.Make(5, 0), Level: 13, Durability: 10}
	kris := item.Item{Index: item.Make(0, 0), Level: 13, Durability: 10}

	// Shipped configuration: switch off.
	b.chaosAnnounce(o, 3, staff)
	if len(texts(b.drain(o))) != 0 {
		t.Fatal("announced with ChaosMixPlusItemAnnounce = 0")
	}

	withConfig(t, b, "Common", map[string]string{"ChaosMixPlusItemAnnounce": "1", "AnnounceMinLevelChaosMix": "11"})
	for _, typ := range []int{0, 1, 2, 4, 5} {
		b.chaosAnnounce(o, typ, staff)
	}
	if len(texts(b.drain(o))) != 0 {
		t.Fatal("announced a mix other than +13")
	}
	// Index 0 (Kris) < 11: the original bug skips it whatever its level.
	b.chaosAnnounce(o, 3, kris)
	if len(texts(b.drain(o))) != 0 {
		t.Fatal("index below AnnounceMinLevelChaosMix announced")
	}
	b.chaosAnnounce(o, 3, staff)
	got := texts(b.drain(o))
	if want := announceText(b, o.P.Char.Name, staff.Index); len(got) != 1 || got[0] != want {
		t.Fatalf("mode 1 %q want %q", got, want)
	}

	// Mode 2 goes to the server group (C1 94 21 relayed by the Hub).
	withConfig(t, b, "Common", map[string]string{"ChaosMixPlusItemAnnounce": "2", "AnnounceMinLevelChaosMix": "11"})
	b.chaosAnnounce(o, 3, staff)
	got = texts(b.drain(o))
	if want := announceText(b, o.P.Char.Name, staff.Index); len(got) != 1 || got[0] != want {
		t.Fatalf("mode 2 %q want %q", got, want)
	}
	// Other modes send nothing.
	withConfig(t, b, "Common", map[string]string{"ChaosMixPlusItemAnnounce": "3"})
	b.chaosAnnounce(o, 3, staff)
	if len(texts(b.drain(o))) != 0 {
		t.Fatal("mode 3 announced")
	}
}

// The +13 mix (86 17) announces after the result; +10 does not.
func TestChaosAnnounceAfterPlus13Mix(t *testing.T) {
	b := newBareWorld(t)
	o := chaosPlayer(b)
	withConfig(t, b, "Common", map[string]string{"ChaosMixPlusItemAnnounce": "1"})
	c := chaos(o)
	plusBox(o, 9, false)
	fixChaosRand(t, 0)
	b.handleChaosMix(o, []byte{0xC1, 4, 0x86, 3})
	if len(texts(b.drain(o))) != 0 {
		t.Fatal("+10 announced")
	}
	plusBox(o, 12, false)
	for i := 0; i < 3; i++ {
		c.Box[4+2*i] = item.Item{Index: chaosmix.ItemBless, Durability: 1}
		c.Box[5+2*i] = item.Item{Index: chaosmix.ItemSoul, Durability: 1}
	}
	c.Box[3].Index = item.Make(5, 0)
	b.handleChaosMix(o, []byte{0xC1, 4, 0x86, 0x17})
	pk := b.drain(o)
	if r := find(pk, 0x86, -1); r == nil || r[3] != 1 || c.Box[0].Level != 13 {
		t.Fatalf("+13 mix % X box %+v", r, c.Box[0])
	}
	got := texts(pk)
	if want := announceText(b, o.P.Char.Name, item.Make(5, 0)); len(got) != 1 || got[0] != want {
		t.Fatalf("%q want %q", got, want)
	}
}
