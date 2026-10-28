package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/store"
)

func TestRunArguments(t *testing.T) {
	ctx := context.Background()
	for _, args := range [][]string{
		{},
		{"-to", "postgres://x"},
		{"-to", "postgres://x", "-from", "sqlserver://y", "-script", "z.sql"},
		{"-gamemasters", "."},
		{"-to", "postgres://x", "-gamemasters", t.TempDir()}, // no Data/Util/GameMaster.txt
	} {
		if err := run(ctx, args, &bytes.Buffer{}); err == nil {
			t.Fatalf("%v accepted", args)
		}
	}
}

// TestRunGameMasters imports GameMaster.txt alone into a database that
// already has the characters: matching rows set gm_level, the others are
// reported.
func TestRunGameMasters(t *testing.T) {
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
	db := fmt.Sprintf("mu_migrate_gm_%d", time.Now().UnixNano())
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+db); err != nil {
		t.Fatal(err)
	}
	defer admin.Exec(ctx, "DROP DATABASE "+db+" WITH (FORCE)")
	url := strings.Replace(base, "/mu?", "/"+db+"?", 1)
	st, err := store.OpenPostgres(ctx, url)
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
	st.Close()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "Data", "Util"), 0o755); err != nil {
		t.Fatal(err)
	}
	src := "\"teste2\" \"Admin\" 32\n\"other\" \"Admin\" 8\nend\n"
	if err := os.WriteFile(filepath.Join(root, "Data", "Util", "GameMaster.txt"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := run(ctx, []string{"-gamemasters", root, "-to", url}, &out); err != nil {
		t.Fatal(err)
	}
	if want := "game masters 1\ngame master not found: other/Admin\n"; out.String() != want {
		t.Fatalf("output %q", out.String())
	}
	st, err = store.OpenPostgres(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if c, err := st.LoadCharacter(ctx, acc.ID, "Admin"); err != nil || c.GMLevel != 32 {
		t.Fatalf("gm level %+v %v", c, err)
	}
}

func TestRunScriptIntoPostgres(t *testing.T) {
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
	db := fmt.Sprintf("mu_migrate_cli_%d", time.Now().UnixNano())
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+db); err != nil {
		t.Fatal(err)
	}
	defer admin.Exec(ctx, "DROP DATABASE "+db+" WITH (FORCE)")
	url := strings.Replace(base, "/mu?", "/"+db+"?", 1)
	var out bytes.Buffer
	if err := run(ctx, []string{"-script", "../../data/DB/MuOnline.sql", "-to", url}, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out.String(), "accounts 2, characters 0") {
		t.Fatalf("output %q", out.String())
	}
}
