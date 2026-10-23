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

type petTestStore interface {
	Store
	PetStore
}

func petConformance(t *testing.T, s petTestStore) {
	ctx := context.Background()
	// DS 0x09 (FUN_0040dfc0): a serial without a row answers level 1, exp 0
	// and the row is created.
	got, err := s.LoadPetInfo(ctx, []uint32{100, 0xFFFFFFF0})
	if err != nil || len(got) != 2 || got[0] != (PetInfo{Serial: 100, Level: 1}) ||
		got[1] != (PetInfo{Serial: 0xFFFFFFF0, Level: 1}) {
		t.Fatalf("load new = %+v %v", got, err)
	}
	// DS 0x34 (FUN_0040f0b0): update existing rows, insert new ones.
	if err := s.SavePetInfo(ctx, []PetInfo{{Serial: 100, Level: 7, Exp: 123456}, {Serial: 200, Level: 2, Exp: 9}}); err != nil {
		t.Fatal(err)
	}
	got, err = s.LoadPetInfo(ctx, []uint32{200, 100})
	if err != nil || got[0] != (PetInfo{Serial: 200, Level: 2, Exp: 9}) || got[1] != (PetInfo{Serial: 100, Level: 7, Exp: 123456}) {
		t.Fatalf("load saved = %+v %v", got, err)
	}
	// DS 0x07 (FUN_0040d8c0): creation never overwrites an existing row.
	if err := s.CreatePetInfo(ctx, PetInfo{Serial: 100, Level: 1}); err != nil {
		t.Fatal(err)
	}
	if err := s.CreatePetInfo(ctx, PetInfo{Serial: 300, Level: 1}); err != nil {
		t.Fatal(err)
	}
	got, _ = s.LoadPetInfo(ctx, []uint32{100, 300})
	if got[0].Level != 7 || got[1] != (PetInfo{Serial: 300, Level: 1}) {
		t.Fatalf("after create = %+v", got)
	}
	if got, _ := s.LoadPetInfo(ctx, nil); len(got) != 0 {
		t.Fatalf("empty request = %+v", got)
	}
}

func TestMemoryPets(t *testing.T) {
	m, _ := NewMemory()
	petConformance(t, m)
}

func TestPostgresPets(t *testing.T) {
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
	name := fmt.Sprintf("mu_test_pet_%d", time.Now().UnixNano())
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
	petConformance(t, p)
}
