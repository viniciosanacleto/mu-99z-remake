package game

import (
	"context"
	"testing"
	"time"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/auth"
)

// offlinePlayer is a playing character with a stored account and
// character, outside the event maps and not in a safe zone.
func offlinePlayer(t *testing.T, b *bareWorld, index int, name string) *Object {
	t.Helper()
	x, y := b.openRow(0, 2)
	o := b.player(index, name, 0, x, y)
	ctx := context.Background()
	hash, err := auth.HashPassword("pw" + name)
	if err != nil {
		t.Fatal(err)
	}
	acc, err := b.store.CreateAccount(ctx, o.P.Account.Login, hash, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := b.store.CreateCharacter(ctx, acc.ID, name, 0); err != nil {
		t.Fatal(err)
	}
	ch, err := b.store.LoadCharacter(ctx, acc.ID, name)
	if err != nil {
		t.Fatal(err)
	}
	ch.Level, ch.Money = 400, 1000000
	o.P.Account = *acc
	o.P.Char = ch
	return o
}

func TestHelperOfflineKeepsTheObject(t *testing.T) {
	b := newBareWorld(t)
	o := offlinePlayer(t, b, PlayerStart, "Offy")
	old := o.conn
	b.runCommand(o, "/offline")
	if !isOffline(o) || !detached(o) || o.State != StatePlaying {
		t.Fatalf("offline %v detached %v state %v", isOffline(o), detached(o), o.State)
	}
	select {
	case <-old.done:
	default:
		t.Fatal("the client socket was not closed")
	}
	c := custom(o).Offline
	if c.X != o.X || c.Y != o.Y || !c.Since.Equal(b.clock) {
		t.Fatalf("offline state %+v", c)
	}
	// The reader's disconnect of the old socket is ignored.
	b.disconnected(old)
	if o.State != StatePlaying {
		t.Fatal("removed by the old socket")
	}
	// Sends are dropped, the 60-second timeout never fires.
	w := b.World
	w.send(o, packetSuperDrop(true))
	b.advance(2 * time.Minute)
	w.tick1000()
	if o.State != StatePlaying || !o.P.LastPacket.Equal(b.clock) {
		t.Fatalf("state %v last packet %v", o.State, o.P.LastPacket)
	}
	// A second /offline does nothing.
	b.helperOffline(o)
	if !detached(o) {
		t.Fatal("not detached")
	}
}

func TestHelperOfflineRefusals(t *testing.T) {
	b := newBareWorld(t)
	o := b.player(PlayerStart, "Evt", 11, 100, 100)
	b.runCommand(o, "/offline")
	hasText(t, b.drain(o), "Nao e permitido usar /offline dentro de eventos.")
	if isOffline(o) || detached(o) {
		t.Fatal("went offline in an event map")
	}
	o.Map = 0
	items(o).If.Use = true
	b.runCommand(o, "/OFFLINE")
	hasText(t, b.drain(o), "Feche as interfaces antes de usar /offline.")
	if isOffline(o) {
		t.Fatal("went offline with an interface open")
	}
	// Index 21 runs the same function.
	items(o).If.Use = false
	b.hooks.commandHandlers[21](b.World, o, command{Index: 21}, "")
	if !isOffline(o) {
		t.Fatal("index 21")
	}
}

func TestOfflineKickRemovesAndSaves(t *testing.T) {
	b := newBareWorld(t)
	o := offlinePlayer(t, b, PlayerStart, "Kicky")
	o.P.Session = &auth.Session{Account: o.P.Account}
	accID := o.P.Account.ID
	b.helperOffline(o)
	o.P.Char.Money = 4321
	b.KickAccount(o.P.Account.Login)
	(<-b.events)()
	b.runAsync()
	if o.State != StateEmpty || o.P != nil {
		t.Fatalf("still in the world: %v", o.State)
	}
	ch, err := b.store.LoadCharacter(context.Background(), accID, "Kicky")
	if err != nil || ch.Money != 4321 {
		t.Fatalf("not saved: %+v %v", ch, err)
	}
}

func TestDCFriendClosesOfflineObject(t *testing.T) {
	b := newBareWorld(t)
	o := offlinePlayer(t, b, PlayerStart, "Gone")
	b.helperOffline(o)
	f := b.player(PlayerStart+1, "Friend", 0, 130, 130)
	b.runCommand(f, "/dcfriend Gone pwGone")
	if o.State != StateEmpty {
		t.Fatal("offline object not closed at once")
	}
	hasText(t, b.drain(f), b.messageFor(f, 0x2BA))
}

func TestOfflineStoreClosesAfterFiveSeconds(t *testing.T) {
	b := newBareWorld(t)
	x, y := b.safeCell(0)
	o := offlinePlayer(t, b, PlayerStart, "Shop")
	o.X, o.Y = x, y
	withConfig(t, b, "Custom", map[string]string{"CustomStoreOfflineSwitch": "1", "CustomStoreSkinChange": "26"})
	pshop(o).Open = true
	b.runCommand(o, "/offstore")
	if !isOffline(o) || custom(o).Store.Offline != 1 || gmOf(o).Skin != 26 {
		t.Fatalf("offline store %+v", custom(o).Store)
	}
	b.pshopClose(o)
	if s := custom(o).Store; s.Offline != 2 || s.Countdown != 5 {
		t.Fatalf("closing %+v", s)
	}
	for i := 0; i < 4; i++ {
		b.offlineSecond()
	}
	if o.State != StatePlaying {
		t.Fatal("closed too early")
	}
	b.offlineSecond()
	if o.State != StateEmpty {
		t.Fatal("not removed after 5 seconds")
	}
}

func TestOffStoreRefusals(t *testing.T) {
	b := newBareWorld(t)
	x, y := b.openRow(0, 1)
	o := b.player(PlayerStart, "NoShop", 0, x, y)
	withConfig(t, b, "Custom", map[string]string{"CustomStoreOfflineSwitch": "1"})
	b.runCommand(o, "/offstore")
	hasText(t, b.drain(o), b.messageFor(o, 0x21C))
	pshop(o).Open = true
	b.runCommand(o, "/offstore")
	hasText(t, b.drain(o), b.messageFor(o, 0x21D))
	if isOffline(o) {
		t.Fatal("offline outside a safe zone")
	}
}
