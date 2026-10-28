package main

import (
	"context"
	"testing"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/store"
)

func TestCommandCharacterGM(t *testing.T) {
	ctx := context.Background()
	st, err := store.NewMemory()
	if err != nil {
		t.Fatal(err)
	}
	acc, err := st.CreateAccount(ctx, "teste2", "plain:x", "")
	if err != nil {
		t.Fatal(err)
	}
	if r, _, err := st.CreateCharacter(ctx, acc.ID, "Admin", 0x10); err != nil || r != store.CreateOK {
		t.Fatalf("create %v %v", r, err)
	}
	if err := command(ctx, st, []string{"character", "gm", "Admin", "32"}); err != nil {
		t.Fatal(err)
	}
	if c, _ := st.LoadCharacter(ctx, acc.ID, "Admin"); c.GMLevel != 32 {
		t.Fatalf("gm level %d", c.GMLevel)
	}
	for _, args := range [][]string{
		{"character", "gm", "Nobody", "8"},
		{"character", "gm", "Admin", "x"},
		{"character", "gm", "Admin", "-1"},
		{"character", "gm", "Admin"},
	} {
		if err := command(ctx, st, args); err == nil {
			t.Fatalf("%v accepted", args)
		}
	}
}
