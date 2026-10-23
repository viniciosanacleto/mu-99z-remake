package store

import (
	"bytes"
	"context"
	"fmt"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

type guildTestStore interface {
	Store
	GuildStore
}

func guildConformance(t *testing.T, s guildTestStore) {
	ctx := context.Background()
	a, _ := s.CreateAccount(ctx, "gacc", "x", "")
	for _, n := range []string{"Master", "Ann", "Bob", "Other"} {
		if r, _, err := s.CreateCharacter(ctx, a.ID, n, 0); r != CreateOK || err != nil {
			t.Fatalf("create %s: %d %v", n, r, err)
		}
	}
	mark := bytes.Repeat([]byte{0x12}, 32)
	num, code, err := s.GuildCreate(ctx, "Lions", "Master", mark, GuildMasterStatus)
	if err != nil || code != 0 || num == 0 {
		t.Fatalf("create = %d %d %v", num, code, err)
	}
	if _, code, _ := s.GuildCreate(ctx, "lions", "Other", mark, GuildMasterStatus); code != 1 {
		t.Fatalf("duplicate name code = %d", code)
	}
	if _, code, _ := s.GuildCreate(ctx, "Tigers", "Master", mark, GuildMasterStatus); code != 2 {
		t.Fatalf("master already member code = %d", code)
	}
	num2, code, _ := s.GuildCreate(ctx, "Tigers", "Other", nil, GuildMasterStatus)
	if code != 0 || num2 == num {
		t.Fatalf("second guild = %d %d", num2, code)
	}
	for _, n := range []string{"Bob", "Ann"} {
		if ok, err := s.GuildMemberAdd(ctx, "Lions", n, 0); !ok || err != nil {
			t.Fatalf("add %s = %v %v", n, ok, err)
		}
	}
	if ok, _ := s.GuildMemberAdd(ctx, "Tigers", "Ann", 0); ok {
		t.Fatal("member of two guilds")
	}
	if _, err := s.GuildMemberAdd(ctx, "Nobody", "Zed", 0); err == nil {
		t.Fatal("add to missing guild")
	}
	if err := s.SetGuildMemberStatus(ctx, "Ann", GuildAssistantStatus); err != nil {
		t.Fatal(err)
	}
	s.SetGuildNotice(ctx, "Lions", "hello")
	s.SetGuildType(ctx, "Lions", 1)
	s.SetGuildScore(ctx, "Lions", 7)
	s.SetGuildUnion(ctx, num, num, num2)
	s.SetGuildRival(ctx, 0, num2)

	gs, err := s.LoadGuilds(ctx)
	if err != nil || len(gs) != 2 {
		t.Fatalf("load = %v %v", gs, err)
	}
	g := gs[0]
	if g.Number != num || g.Name != "Lions" || g.Master != "Master" || !bytes.Equal(g.Mark, mark) ||
		g.Notice != "hello" || g.Type != 1 || g.Score != 7 || g.Union != num || g.Rival != 0 {
		t.Fatalf("guild = %+v", g)
	}
	want := []GuildMember{{"Master", GuildMasterStatus}, {"Bob", 0}, {"Ann", GuildAssistantStatus}}
	if fmt.Sprint(g.Members) != fmt.Sprint(want) {
		t.Fatalf("members = %v, want %v", g.Members, want)
	}
	if gs[1].Union != num || len(gs[1].Members) != 1 {
		t.Fatalf("tigers = %+v", gs[1])
	}

	// G_Status shows in the character list.
	list, _ := s.ListCharacters(ctx, a.ID)
	st := map[string]byte{}
	for _, c := range list {
		st[c.Name] = c.GuildStatus
	}
	if st["Ann"] != GuildAssistantStatus || st["Master"] != GuildMasterStatus || st["Bob"] != 0 {
		t.Fatalf("list statuses = %v", st)
	}

	if err := s.GuildMemberDelete(ctx, "Bob"); err != nil {
		t.Fatal(err)
	}
	if ok, _ := s.GuildMemberAdd(ctx, "Tigers", "Bob", 0); !ok {
		t.Fatal("re-add after leave")
	}
	if err := s.GuildDelete(ctx, "Lions"); err != nil {
		t.Fatal(err)
	}
	gs, _ = s.LoadGuilds(ctx)
	if len(gs) != 1 || gs[0].Name != "Tigers" || len(gs[0].Members) != 2 {
		t.Fatalf("after delete = %+v", gs)
	}
	list, _ = s.ListCharacters(ctx, a.ID)
	for _, c := range list {
		if c.Name == "Ann" && c.GuildStatus != GuildStatusNone {
			t.Fatalf("Ann still in a guild: %d", c.GuildStatus)
		}
	}
	// Deleting a character removes its GuildMember row.
	if ok, err := s.DeleteCharacter(ctx, a.ID, "Bob"); !ok || err != nil {
		t.Fatal("delete Bob")
	}
	gs, _ = s.LoadGuilds(ctx)
	if len(gs[0].Members) != 1 {
		t.Fatalf("members after character delete = %v", gs[0].Members)
	}
}

func TestMemoryGuilds(t *testing.T) {
	m, _ := NewMemory()
	guildConformance(t, m)
}

func TestPostgresGuilds(t *testing.T) {
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
	name := fmt.Sprintf("mu_test_gd_%d", time.Now().UnixNano())
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
	guildConformance(t, p)
}
