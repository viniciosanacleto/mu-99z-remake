package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func TestRunArguments(t *testing.T) {
	ctx := context.Background()
	for _, args := range [][]string{
		{},
		{"-to", "postgres://x"},
		{"-to", "postgres://x", "-from", "sqlserver://y", "-script", "z.sql"},
	} {
		if err := run(ctx, args, &bytes.Buffer{}); err == nil {
			t.Fatalf("%v accepted", args)
		}
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
