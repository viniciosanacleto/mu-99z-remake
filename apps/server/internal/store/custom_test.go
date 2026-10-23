package store

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"
)

func testCustomCommandStore(t *testing.T, s Store, setCredits func(id int64, n int64)) {
	cs, ok := s.(CustomCommandStore)
	if !ok {
		t.Fatal("no CustomCommandStore")
	}
	ctx := context.Background()
	suffix := fmt.Sprintf("%06d", time.Now().UnixNano()/1000%1000000)
	login := "cus" + suffix
	acc, err := s.CreateAccount(ctx, login, "plain:x", "")
	if err != nil {
		t.Fatal(err)
	}
	name := "Gft" + suffix
	if r, _, err := s.CreateCharacter(ctx, acc.ID, name, 0x10); err != nil || r != CreateOK {
		t.Fatalf("create %v %v", r, err)
	}

	// GiftData: MaxCount 2 allows two takes; -1 is unlimited.
	for i, want := range []bool{true, true, false, false} {
		if got, err := cs.TakeGift(ctx, name, 7, 2); err != nil || got != want {
			t.Fatalf("take %d: %v %v", i, got, err)
		}
	}
	if got, _ := cs.TakeGift(ctx, name, 8, 0); got {
		t.Fatal("MaxCount 0 must refuse")
	}
	for i := 0; i < 3; i++ {
		if got, _ := cs.TakeGift(ctx, name, 9, -1); !got {
			t.Fatal("unlimited gift refused")
		}
	}

	// Marriage of an offline partner.
	ch, _ := s.LoadCharacter(ctx, acc.ID, name)
	ch.Married, ch.MarryCouple = true, "Partner"
	if err := s.SaveCharacter(ctx, ch); err != nil {
		t.Fatal(err)
	}
	if err := cs.ClearMarriage(ctx, name); err != nil {
		t.Fatal(err)
	}
	if ch, _ = s.LoadCharacter(ctx, acc.ID, name); ch.Married || ch.MarryCouple != "" {
		t.Fatalf("still married %+v %q", ch.Married, ch.MarryCouple)
	}

	// CustomPick: a missing row is empty; save inserts then updates.
	if d, err := cs.LoadPick(ctx, name); err != nil || d != (PickData{}) {
		t.Fatalf("empty pick %+v %v", d, err)
	}
	var d PickData
	copy(d.List[:], "Jewel")
	copy(d.List[PickTextSize:], "Excellent")
	d.Money, d.Jewel, d.Exc, d.Set = 1, 2, 3, 4
	if err := cs.SavePick(ctx, name, d); err != nil {
		t.Fatal(err)
	}
	d.Set = 0
	if err := cs.SavePick(ctx, name, d); err != nil {
		t.Fatal(err)
	}
	if got, err := cs.LoadPick(ctx, name); err != nil || got != d {
		t.Fatalf("pick %+v %v", got, err)
	}

	// Rename moves the GiftData and CustomPick rows.
	newName := "Ren" + suffix
	if ok, err := s.(CommandStore).RenameCharacter(ctx, acc.ID, login, name, newName); err != nil || !ok {
		t.Fatalf("rename %v %v", ok, err)
	}
	if got, _ := cs.LoadPick(ctx, newName); got != d {
		t.Fatalf("renamed pick %+v", got)
	}
	if got, _ := cs.TakeGift(ctx, newName, 7, 2); got {
		t.Fatal("renamed gift count lost")
	}
	// Deleting the character removes them.
	if ok, err := s.DeleteCharacter(ctx, acc.ID, newName); err != nil || !ok {
		t.Fatalf("delete %v %v", ok, err)
	}
	if got, _ := cs.LoadPick(ctx, newName); got != (PickData{}) {
		t.Fatalf("pick after delete %+v", got)
	}
	if got, _ := cs.TakeGift(ctx, newName, 7, 1); !got {
		t.Fatal("gift row survived the delete")
	}

	// Account info.
	setCredits(acc.ID, 1234)
	info, err := cs.AccountInfo(ctx, login)
	if err != nil || info.Credits != 1234 || info.OnlineHours != 0 || info.SessionSeconds < 0 {
		t.Fatalf("info %+v %v", info, err)
	}
	if _, err := cs.AccountInfo(ctx, "nobody"+suffix); err != ErrNotFound {
		t.Fatalf("missing account %v", err)
	}
}

func TestMemoryCustomCommandStore(t *testing.T) {
	m, _ := NewMemory()
	testCustomCommandStore(t, m, m.SetCredits)
}

func TestPostgresCustomCommandStore(t *testing.T) {
	url := os.Getenv("MU_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("MU_TEST_DATABASE_URL not set")
	}
	p, err := OpenPostgres(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	testCustomCommandStore(t, p, func(id, n int64) {
		if _, err := p.pool.Exec(context.Background(), `UPDATE accounts SET credits=$2 WHERE id=$1`, id, n); err != nil {
			t.Fatal(err)
		}
	})
}
