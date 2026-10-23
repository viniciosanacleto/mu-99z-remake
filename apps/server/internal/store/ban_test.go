package store

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"
)

// testBanStore checks WZ_BanAccount / WZ_BanCharacter: the value is
// written (1 bans, 0 lifts) and a missing row reports not found.
func testBanStore(t *testing.T, s Store) {
	bs, ok := s.(BanStore)
	if !ok {
		t.Fatal("no BanStore")
	}
	ctx := context.Background()
	suffix := fmt.Sprintf("%06d", time.Now().UnixNano()/1000%1000000)
	login := "ban" + suffix
	acc, err := s.CreateAccount(ctx, login, "plain:x", "")
	if err != nil {
		t.Fatal(err)
	}
	name := "Ban" + suffix
	if r, _, err := s.CreateCharacter(ctx, acc.ID, name, 0x10); err != nil || r != CreateOK {
		t.Fatalf("create %v %v", r, err)
	}
	if found, err := bs.BanAccount(ctx, login, 1); err != nil || !found {
		t.Fatalf("ban account %v %v", found, err)
	}
	if a, _ := s.AccountByLogin(ctx, login); a.BlockCode != 1 {
		t.Fatalf("block code %d", a.BlockCode)
	}
	if found, _ := bs.BanAccount(ctx, login, 0); !found {
		t.Fatal("unban account")
	}
	if a, _ := s.AccountByLogin(ctx, login); a.BlockCode != 0 {
		t.Fatalf("block code after unban %d", a.BlockCode)
	}
	if found, err := bs.BanCharacter(ctx, name, 1); err != nil || !found {
		t.Fatalf("ban character %v %v", found, err)
	}
	if c, _ := s.LoadCharacter(ctx, acc.ID, name); c == nil || c.CtlCode != 1 {
		t.Fatalf("ctl code %+v", c)
	}
	if found, _ := bs.BanCharacter(ctx, name, 0); !found {
		t.Fatal("unban character")
	}
	if c, _ := s.LoadCharacter(ctx, acc.ID, name); c.CtlCode != 0 {
		t.Fatalf("ctl code after unban %d", c.CtlCode)
	}
	if found, err := bs.BanAccount(ctx, "nobody"+suffix, 1); err != nil || found {
		t.Fatalf("missing account %v %v", found, err)
	}
	if found, err := bs.BanCharacter(ctx, "Nobody"+suffix, 1); err != nil || found {
		t.Fatalf("missing character %v %v", found, err)
	}
}

func TestMemoryBanStore(t *testing.T) {
	m, _ := NewMemory()
	testBanStore(t, m)
}

func TestPostgresBanStore(t *testing.T) {
	url := os.Getenv("MU_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("MU_TEST_DATABASE_URL not set")
	}
	p, err := OpenPostgres(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	testBanStore(t, p)
}
