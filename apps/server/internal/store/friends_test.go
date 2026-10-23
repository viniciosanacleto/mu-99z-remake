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

type friendTestStore interface {
	Store
	FriendStore
}

func friendConformance(t *testing.T, s friendTestStore) {
	ctx := context.Background()
	a, _ := s.CreateAccount(ctx, "facc", "x", "")
	mk := func(name string, level int) {
		if r, _, err := s.CreateCharacter(ctx, a.ID, name, 0); r != CreateOK || err != nil {
			t.Fatalf("create %s: %d %v", name, r, err)
		}
		c, _ := s.LoadCharacter(ctx, a.ID, name)
		c.Level = level
		if err := s.SaveCharacter(ctx, c); err != nil {
			t.Fatal(err)
		}
	}
	mk("Ann", 10)
	mk("Bob", 10)
	mk("Low", 3)
	for _, n := range []string{"Ann", "Bob", "Low", "Ann"} {
		if err := s.FriendGuidCreate(ctx, n); err != nil {
			t.Fatal(err)
		}
	}
	annG, _, ok, _ := s.FriendMain(ctx, "ann")
	bobG, _, _, _ := s.FriendMain(ctx, "Bob")
	if !ok || annG == bobG {
		t.Fatal("guids")
	}
	if _, _, ok, _ := s.FriendMain(ctx, "Nobody"); ok {
		t.Fatal("missing main row")
	}

	check := func(what string, got int, err error, want int) {
		t.Helper()
		if err != nil || got != want {
			t.Fatalf("%s = %d (%v), want %d", what, got, err, want)
		}
	}
	r, err := s.WaitFriendAdd(ctx, "Ann", "Nobody")
	check("wait add missing", r, err, 0)
	r, err = s.WaitFriendAdd(ctx, "Ann", "Low")
	check("wait add low level", r, err, 6)
	r, err = s.WaitFriendAdd(ctx, "Ann", "Bob")
	check("wait add", r, err, 1)
	r, err = s.WaitFriendAdd(ctx, "Ann", "Bob")
	check("wait add twice", r, err, 4)
	w, _ := s.WaitingFriends(ctx, bobG)
	if len(w) != 1 || w[0] != "Ann" {
		t.Fatalf("waiting = %v", w)
	}
	// Bob accepts.
	r, err = s.FriendAdd(ctx, "Bob", "Ann")
	check("friend add", r, err, 1)
	r, err = s.FriendAdd(ctx, "Bob", "Ann")
	check("friend add twice", r, err, 2)
	if w, _ := s.WaitingFriends(ctx, bobG); len(w) != 0 {
		t.Fatalf("waiting after accept = %v", w)
	}
	if n, _ := s.FriendNames(ctx, annG); len(n) != 1 || n[0] != "Bob" {
		t.Fatalf("ann friends = %v", n)
	}
	if err := s.SetFriendDel(ctx, "Ann", 1); err != nil {
		t.Fatal(err)
	}
	r, err = s.FriendDel(ctx, "Ann", "Bob")
	check("friend del", r, err, 1)
	r, err = s.FriendDel(ctx, "Ann", "Bob")
	check("friend del twice", r, err, 2)
	r, err = s.WaitFriendDel(ctx, "Bob", "Ann")
	check("wait del none", r, err, 0)

	// Mail.
	r, _, err = s.WriteMail(ctx, Mail{From: "Ann", To: "Nobody", Subject: "x"})
	check("mail missing", r, err, 2)
	r, _, err = s.WriteMail(ctx, Mail{From: "Ann", To: "Low", Subject: "x"})
	check("mail low", r, err, 6)
	idx, guid, err := s.WriteMail(ctx, Mail{From: "Ann", To: "Bob", Subject: "Hi", Memo: []byte("hello"),
		Photo: make([]byte, 18), Dir: 2, Act: 3})
	check("mail index", idx, err, 11)
	if guid != bobG {
		t.Fatal("receiver guid")
	}
	list, _ := s.MailList(ctx, bobG)
	if len(list) != 1 || list[0].From != "Ann" || list[0].Subject != "Hi" || list[0].Read {
		t.Fatalf("mail list = %+v", list)
	}
	m, err := s.ReadMail(ctx, "Bob", 11)
	if err != nil || string(m.Memo) != "hello" || m.Dir != 2 || m.Act != 3 || !m.Read {
		t.Fatalf("read = %+v %v", m, err)
	}
	if _, total, _, _ := s.FriendMain(ctx, "Bob"); total != 1 {
		t.Fatalf("memo total = %d", total)
	}
	r, err = s.DelMail(ctx, "Bob", 99)
	check("del mail missing", r, err, 4)
	r, err = s.DelMail(ctx, "Bob", 11)
	check("del mail", r, err, 1)

	// Deleting the character removes its friend rows.
	if ok, _ := s.DeleteCharacter(ctx, a.ID, "Bob"); !ok {
		t.Fatal("delete")
	}
	if _, _, ok, _ := s.FriendMain(ctx, "Bob"); ok {
		t.Fatal("friend row survived character deletion")
	}
}

func TestMemoryFriends(t *testing.T) {
	m, _ := NewMemory()
	friendConformance(t, m)
}

func TestPostgresFriends(t *testing.T) {
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
	name := fmt.Sprintf("mu_test_fr_%d", time.Now().UnixNano())
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
	friendConformance(t, p)
}
