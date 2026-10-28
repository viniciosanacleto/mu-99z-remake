package game

import (
	"context"
	"testing"
	"time"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/item"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/reconnect"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/store"
)

// withConfigs is withConfig for several files at once.
func withConfigs(t *testing.T, b *bareWorld, files map[string]map[string]string) {
	t.Helper()
	b.cfg = instanceConfig(t, "gameserver", files)
}

// settle runs the continuations the async work posts until none comes
// for 100 ms (the memory store answers at once).
func (b *bareWorld) settle() {
	for {
		select {
		case f := <-b.events:
			f()
		case <-time.After(100 * time.Millisecond):
			return
		}
	}
}

func reconnectRows(t *testing.T, b *bareWorld) []store.ReconnectRow {
	t.Helper()
	if last := b.rc().last; last != nil {
		<-last // the ReconnectData writes run in order
	}
	rows, err := b.store.(store.ReconnectStore).ReconnectRows(context.Background(), b.cfg.ServerCode)
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

// Leaving the game stores the record (and the ReconnectData row); after
// ReconnectTime (300) + 1 seconds it is erased (FUN_004e1260).
func TestReconnectRecordExpires(t *testing.T) {
	b := newBareWorld(t)
	o := b.player(PlayerStart, "Leaver", 0, 130, 130)
	b.removePlayer(o)
	r := b.rc().records["Leaver"]
	if r == nil || r.Time != 300 || r.Account != "accLeaver" || r.Party != -1 || r.PartyFlags != 3 {
		t.Fatalf("record %+v", r)
	}
	if rows := reconnectRows(t, b); len(rows) != 1 || rows[0].Name != "Leaver" || rows[0].Time != 300 {
		t.Fatalf("rows %+v", rows)
	}
	for i := 0; i < 300; i++ {
		b.reconnectSecond()
	}
	if b.rc().records["Leaver"] == nil {
		t.Fatal("erased too early")
	}
	b.reconnectSecond()
	if b.rc().records["Leaver"] != nil {
		t.Fatal("not erased")
	}
	if rows := reconnectRows(t, b); len(rows) != 0 {
		t.Fatalf("row not deleted %+v", rows)
	}
}

// FUN_004e1970: a relog gets the party, auto points, auto reset, party
// flags, auto attack (F3 EB) and auto pick back; the record is consumed.
func TestReconnectRestoresState(t *testing.T) {
	b := newBareWorld(t)
	a := b.player(PlayerStart, "Alpha", 0, 130, 130)
	c := b.player(PlayerStart+1, "Beta", 0, 131, 130)
	b.partyStateOf(a)
	b.partyStateOf(c)
	if !b.partyCreate(a) || !b.partyAdd(b.partyOf(a), c) {
		t.Fatal("party")
	}
	n := b.partyNumber(a)
	ap := autoPoints(a)
	ap.Count, ap.Amount = 1, [5]int{0, 10, 0, 0, 0}
	cmdOf(a).AutoReset, cmdOf(a).AutoPoints = true, [5]int{1, 2, 3, 4, 5}
	b.partyStateOf(a).Flags, b.partyStateOf(a).Password = 7, "secret"
	at := attackOf(a)
	at.Active, at.Buff, at.Skill, at.Map, at.X, at.Y = true, 1, 9, 0, 130, 130
	pickOf(a).On = true
	b.removePlayer(a)
	if pt := b.partyOf(c); pt == nil || pt.count != 1 {
		t.Fatal("ReconnectTime keeps the party with one member")
	}

	o := b.player(PlayerStart+2, "Alpha", 0, 130, 130)
	b.reconnectRestore(o)
	if b.partyNumber(o) != n || b.partyOf(c).count != 2 {
		t.Fatalf("party %d", b.partyNumber(o))
	}
	if ap := autoPoints(o); ap.Count != 1 || ap.Amount[1] != 10 {
		t.Fatalf("auto points %+v", ap)
	}
	if s := cmdOf(o); !s.AutoReset || s.AutoPoints != [5]int{1, 2, 3, 4, 5} {
		t.Fatalf("auto reset %+v", s)
	}
	if ps := b.partyStateOf(o); ps.Flags != 7 || ps.Password != "secret" {
		t.Fatalf("party flags %+v", ps)
	}
	if at := attackOf(o); !at.Active || at.Offline || at.Skill != 9 || at.Buff != 1 || at.X != 130 || at.Start != b.clock {
		t.Fatalf("attack %+v", at)
	}
	if !pickOf(o).On {
		t.Fatal("pick")
	}
	if st := find(b.drain(o), 0xF3, 0xEB); st == nil || st[4] != 1 {
		t.Fatalf("F3 EB % X", st)
	}
	if b.rc().records["Alpha"] != nil {
		t.Fatal("record not consumed")
	}
	if rows := reconnectRows(t, b); len(rows) != 0 {
		t.Fatalf("row not deleted %+v", rows)
	}
	// No record: nothing happens (no F3 EB).
	b.reconnectRestore(o)
	if find(b.drain(o), 0xF3, 0xEB) != nil {
		t.Fatal("F3 EB without record")
	}
}

// FUN_004e1ae0 / FUN_004e1bb0: when the whole party left, the first member
// back creates it again under the same number and the next one joins.
func TestReconnectRecreatesParty(t *testing.T) {
	b := newBareWorld(t)
	a := b.player(PlayerStart, "Alpha", 0, 130, 130)
	c := b.player(PlayerStart+1, "Beta", 0, 131, 130)
	b.partyStateOf(a)
	b.partyStateOf(c)
	b.partyCreate(a)
	b.partyAdd(b.partyOf(a), c)
	n := b.partyNumber(a)
	b.removePlayer(a)
	b.removePlayer(c)
	if b.parties.parties[n] != nil {
		t.Fatal("party survived")
	}
	c2 := b.player(PlayerStart+3, "Beta", 0, 131, 130)
	b.reconnectRestore(c2)
	pt := b.parties.parties[n]
	if pt == nil || pt.leader() != c2.Index || pt.count != 1 || b.partyNumber(c2) != n {
		t.Fatalf("party %+v", pt)
	}
	if find(b.drain(c2), 0x42, -1) == nil {
		t.Fatal("no party list")
	}
	a2 := b.player(PlayerStart+2, "Alpha", 0, 130, 130)
	b.reconnectRestore(a2)
	if pt.count != 2 || b.partyNumber(a2) != n {
		t.Fatalf("second member %+v", pt)
	}
	// A lone record of a party that no longer exists is not restored.
	b.removePlayer(a2) // count 2 -> a2 removed, party kept with c2
	b.removePlayer(c2) // alone: destroyed
	b.reconnectRestore(b.player(PlayerStart+4, "Beta", 0, 131, 130))
	b.rc().records["Alpha"].Party = 99
	b.parties.parties[n] = nil
	delete(b.parties.parties, n)
	a3 := b.player(PlayerStart+5, "Alpha", 0, 130, 130)
	b.reconnectRestore(a3)
	if b.partyNumber(a3) != -1 {
		t.Fatalf("lone record joined %d", b.partyNumber(a3))
	}
}

// With ReconnectOfflineSwitch, an offline attack in a safe zone leaves no
// record; outside it the record keeps the offline flags.
func TestReconnectOfflineSwitchSafeZone(t *testing.T) {
	b := newBareWorld(t)
	withConfig(t, b, "Common", map[string]string{"ReconnectOfflineSwitch": "1"})
	o := b.player(PlayerStart, "Robot", 0, 130, 130) // Lorencia town: safe zone
	if !b.inSafeZone(o) {
		t.Skip("130,130 is not a safe zone")
	}
	at := attackOf(o)
	at.Active, at.Offline = true, true
	b.reconnectSave(o)
	if b.rc().records["Robot"] != nil {
		t.Fatal("record in a safe zone")
	}
	x, y := b.openRow(0, 2)
	o.X, o.Y = x, y
	if b.inSafeZone(o) {
		t.Skip("open row in a safe zone")
	}
	b.reconnectSave(o)
	if r := b.rc().records["Robot"]; r == nil || !r.AttackOffline || !r.AttackActive {
		t.Fatalf("record %+v", r)
	}
}

// Removing an offline attack (relog of the account) keeps the attack flags
// in the record, so the next login runs the auto attack online.
func TestReconnectAfterOfflineAttackKick(t *testing.T) {
	b := newBareWorld(t)
	o := b.player(PlayerStart, "Robot", 0, 130, 130)
	at := attackOf(o)
	at.Active, at.Offline, at.Skill = true, true, 9
	b.detachClient(o)
	if !b.offlineKick(o) {
		t.Fatal("not offline")
	}
	r := b.rc().records["Robot"]
	if r == nil || !r.AttackActive || !r.AttackOffline {
		t.Fatalf("record %+v", r)
	}
	n := b.player(PlayerStart+1, "Robot", 0, 130, 130)
	b.reconnectRestore(n)
	if a := attackOf(n); !a.Active || a.Offline || a.Skill != 9 {
		t.Fatalf("attack %+v", a)
	}
}

// FUN_004e1e70: for an object without client, the shop prices come back
// and the custom shop reopens offline.
func TestReconnectOfflineStore(t *testing.T) {
	b := newBareWorld(t)
	withConfigs(t, b, map[string]map[string]string{"Custom": {"CustomStoreSwitch": "1", "CustomStoreMapZone": "-1"}})
	o := b.player(PlayerStart, "Seller", 0, 130, 130)
	if !b.inSafeZone(o) {
		t.Skip("130,130 is not a safe zone")
	}
	o.P.Inventory[ShopStart] = item.Item{Index: item.Make(14, 13), Durability: 1}
	o.P.Inventory[ShopStart+1] = item.Item{Index: item.Make(14, 14), Durability: 1}
	r := &reconnect.Record{Name: "Seller", PShopOpen: true, StoreOffline: 1, StoreType: 2, Party: -1}
	r.Prices[0], r.Prices[1] = 5, 0
	b.rc().records["Seller"] = r

	// With a client nothing is restored.
	b.reconnectRestore(o)
	if o.P.Inventory[ShopStart].PShopPrice != 0 || pshopOpen(o) {
		t.Fatal("restored with a client")
	}
	b.rc().records["Seller"] = r
	b.detachClient(o)
	b.reconnectRestore(o)
	if o.P.Inventory[ShopStart].PShopPrice != 5 || o.P.Inventory[ShopStart+1].PShopPrice != 0 || !pshopOpen(o) ||
		custom(o).Store.Offline != 1 {
		t.Fatalf("store %+v open %v", custom(o).Store, pshopOpen(o))
	}
}

// FUN_004e22e0 / FUN_004e2020: the replayed records fill the cache and,
// with ReconnectOfflineSwitch and CustomAttackOfflineSwitch, an offline
// attack character is created again without client and keeps attacking.
func TestOfflineReconnectAtStart(t *testing.T) {
	b := newBareWorld(t)
	ctx := context.Background()
	acc, err := b.store.CreateAccount(ctx, "robotacc", "plain:x", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := b.store.CreateCharacter(ctx, acc.ID, "Robot", 0x10); err != nil {
		t.Fatal(err)
	}
	r := &reconnect.Record{Account: "robotacc", Name: "Robot", IP: "10.1.1.1", HWID: "HW", Time: 300, Party: -1,
		PartyFlags: 3, AttackActive: true, AttackOffline: true, AttackSkill: 19, AttackX: 130, AttackY: 130}
	other := &reconnect.Record{Account: "robotacc", Name: "Normal", IP: "10.1.1.1", HWID: "HW", Time: 300, Party: -1}
	rows := []store.ReconnectRow{{Name: "Normal", Data: other.Encode()}, {Name: "Robot", Data: r.Encode()}}

	// Switch off: only the cache is filled.
	b.reconnectLoaded(rows)
	b.settle()
	if b.rc().records["Robot"] == nil || b.rc().records["Normal"] == nil || b.users != 0 {
		t.Fatalf("cache %v users %d", b.rc().records, b.users)
	}

	withConfigs(t, b, map[string]map[string]string{"Common": {"ReconnectOfflineSwitch": "1"},
		"Custom": {"CustomAttackOfflineSwitch": "1"}})
	b.reconnectLoaded(rows)
	b.settle()
	var o *Object
	for i := PlayerStart; i < MaxObjects; i++ {
		if x := b.objects[i]; x.State == StatePlaying && x.P != nil && x.P.Char != nil && x.P.Char.Name == "Robot" {
			o = x
		}
	}
	if o == nil || b.users != 1 {
		t.Fatalf("not re-created (users %d)", b.users)
	}
	if !detached(o) || !isOffline(o) || o.P.IP != "10.1.1.1" || o.P.Account.Login != "robotacc" || !b.auth.Online("robotacc") {
		t.Fatalf("object %+v", o.P)
	}
	if a := attackOf(o); !a.Active || !a.Offline || a.Skill != 19 {
		t.Fatalf("attack %+v", a)
	}
	if b.rc().records["Robot"] != nil {
		t.Fatal("record not consumed")
	}
	// The account owner logging in removes it (KickAccount -> offlineKick).
	b.KickAccount("robotacc")
	b.settle()
	if o.State != StateEmpty || b.users != 0 {
		t.Fatal("not removed by the kick")
	}
}

// A re-created object that got no offline mode back (the /offline flag is
// never restored) is removed after the client timeout.
func TestOfflineReconnectHelperTimesOut(t *testing.T) {
	b := newBareWorld(t)
	ctx := context.Background()
	acc, _ := b.store.CreateAccount(ctx, "helperacc", "plain:x", "")
	b.store.CreateCharacter(ctx, acc.ID, "Helper", 0x10)
	withConfigs(t, b, map[string]map[string]string{"Common": {"ReconnectOfflineSwitch": "1"},
		"Custom": {"CustomStoreOfflineSwitch": "1"}})
	r := &reconnect.Record{Account: "helperacc", Name: "Helper", IP: "10.1.1.2", HWID: "HW", Time: 300, Party: -1,
		Helper: true}
	b.reconnectLoaded([]store.ReconnectRow{{Name: "Helper", Data: r.Encode()}})
	b.settle()
	if b.users != 1 {
		t.Fatalf("users %d", b.users)
	}
	b.advance(clientIdleTimeout + time.Second)
	b.reconnectSecond()
	b.settle()
	if b.users != 0 || b.auth.Online("helperacc") {
		t.Fatal("helper object kept")
	}
}

// The DataServer replay (FUN_0041a7f0 -> FUN_004e22e0) runs once, on the
// first second of the instance: the rows of its ServerCode fill the cache.
func TestReconnectReplayOnFirstSecond(t *testing.T) {
	b := newBareWorld(t)
	r := &reconnect.Record{Account: "a", Name: "Stored", Time: 300, Party: -1}
	rs := b.store.(store.ReconnectStore)
	ctx := context.Background()
	rs.SaveReconnect(ctx, store.ReconnectRow{Name: "Stored", ServerCode: b.cfg.ServerCode, Data: r.Encode(), Time: 300})
	rs.SaveReconnect(ctx, store.ReconnectRow{Name: "Elsewhere", ServerCode: b.cfg.ServerCode + 1, Data: r.Encode(), Time: 300})
	b.reconnectSecond()
	b.settle()
	if got := b.rc().records["Stored"]; got == nil || got.Time != 300 || b.rc().records["Elsewhere"] != nil {
		t.Fatalf("records %v", b.rc().records)
	}
	b.reconnectSecond()
	b.settle()
	if got := b.rc().records["Stored"]; got == nil || got.Time != 299 {
		t.Fatalf("replayed twice %+v", got)
	}
}
