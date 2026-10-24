package store

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

type adminStore interface {
	Store
	SearchCharacters(ctx context.Context, query string, limit int) ([]CharacterRef, error)
	AccountOnline(ctx context.Context, accountID int64) (bool, error)
}

func testAdminStore(t *testing.T, s adminStore) {
	ctx := context.Background()
	a, err := s.CreateAccount(ctx, "joao", "plain:x", "")
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.CreateAccount(ctx, "maria_1", "plain:x", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		acc  int64
		name string
	}{{a.ID, "Zeus"}, {a.ID, "Atena"}, {b.ID, "Hermes"}} {
		if res, _, err := s.CreateCharacter(ctx, c.acc, c.name, 16); err != nil || res != CreateOK {
			t.Fatalf("create %s = %v, %v", c.name, res, err)
		}
	}

	got, err := s.SearchCharacters(ctx, "JOAO", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Name != "Atena" || got[1].Name != "Zeus" || got[0].Login != "joao" ||
		got[0].AccountID != a.ID || got[0].Class != 16 || got[0].Level != 1 || got[0].Online {
		t.Fatalf("search by login = %+v", got)
	}
	if got, _ = s.SearchCharacters(ctx, "erm", 10); len(got) != 1 || got[0].Name != "Hermes" {
		t.Fatalf("search by name = %+v", got)
	}
	// "_" and "%" are literals, not ILIKE wildcards.
	if got, _ = s.SearchCharacters(ctx, "a_1", 10); len(got) != 1 || got[0].Login != "maria_1" {
		t.Fatalf("search with underscore = %+v", got)
	}
	if got, _ = s.SearchCharacters(ctx, "o_o", 10); len(got) != 0 {
		t.Fatalf("underscore matched as wildcard: %+v", got)
	}
	if got, _ = s.SearchCharacters(ctx, "", 2); len(got) != 2 {
		t.Fatalf("limit = %+v", got)
	}

	if on, err := s.AccountOnline(ctx, a.ID); err != nil || on {
		t.Fatalf("online before connect = %v, %v", on, err)
	}
	if err := s.ConnectAccount(ctx, a.ID, "Principal", "127.0.0.1"); err != nil {
		t.Fatal(err)
	}
	if on, _ := s.AccountOnline(ctx, a.ID); !on {
		t.Fatal("online after connect = false")
	}
	if got, _ = s.SearchCharacters(ctx, "Zeus", 10); len(got) != 1 || !got[0].Online {
		t.Fatalf("search online flag = %+v", got)
	}
	if err := s.DisconnectAccount(ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	if on, _ := s.AccountOnline(ctx, a.ID); on {
		t.Fatal("online after disconnect = true")
	}
}

func TestMemoryAdminStore(t *testing.T) {
	m, err := NewMemory()
	if err != nil {
		t.Fatal(err)
	}
	testAdminStore(t, m)
}

func TestPostgresAdminStore(t *testing.T) {
	base := os.Getenv("MU_TEST_DATABASE_URL")
	if base == "" {
		t.Skip("MU_TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	admin, err := pgx.Connect(ctx, base)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close(ctx)
	name := fmt.Sprintf("mu_test_admin_%d", time.Now().UnixNano())
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		t.Fatal(err)
	}
	defer admin.Exec(ctx, "DROP DATABASE "+name+" WITH (FORCE)")
	u, _ := url.Parse(base)
	u.Path = "/" + name
	p, err := OpenPostgres(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	testAdminStore(t, p)
}
