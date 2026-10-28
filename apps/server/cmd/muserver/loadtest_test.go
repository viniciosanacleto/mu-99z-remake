package main

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/loadbot"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/store"
)

func TestLoadFlags(t *testing.T) {
	c, _, err := parseFlags(nil)
	if err != nil {
		t.Fatal(err)
	}
	if c.metricsAddr != "" || c.maxConnPerIP != -1 || c.maxUsers != 0 || c.seedLoad != 0 {
		t.Fatalf("defaults = %+v", c)
	}
	c, _, err = parseFlags([]string{"-metrics-addr", ":6060", "-max-conn-per-ip", "0", "-max-users", "200", "-seed-load", "5"})
	if err != nil {
		t.Fatal(err)
	}
	if c.metricsAddr != ":6060" || c.maxConnPerIP != 0 || c.maxUsers != 200 || c.seedLoad != 5 {
		t.Fatalf("flags = %+v", c)
	}
}

func loadTerrain(t *testing.T) *gamedata.Terrain {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "data", "Data", "Terrain", "Terrain1.att"))
	if err != nil {
		t.Fatal(err)
	}
	tr, err := gamedata.ParseTerrain("Terrain1.att", b)
	if err != nil {
		t.Fatal(err)
	}
	return tr
}

func TestSeedLoad(t *testing.T) {
	ctx := context.Background()
	st, err := store.NewMemory()
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join("..", "..", "data", "Data")
	if err := seedLoad(ctx, st, dir, 4); err != nil {
		t.Fatal(err)
	}
	// Seeding again keeps the accounts and only places the characters.
	if err := seedLoad(ctx, st, dir, 4); err != nil {
		t.Fatal(err)
	}
	tr := loadTerrain(t)
	seen := map[[2]int]bool{}
	for i := 1; i <= 4; i++ {
		login, name := loadbot.Account(i)
		acc, err := st.AccountByLogin(ctx, login)
		if err != nil {
			t.Fatalf("%s: %v", login, err)
		}
		ch, err := st.LoadCharacter(ctx, acc.ID, name)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if ch.Map != 0 || ch.X < seedArea.x0 || ch.X > seedArea.x1 || ch.Y < seedArea.y0 || ch.Y > seedArea.y1 {
			t.Errorf("%s at map %d (%d,%d)", name, ch.Map, ch.X, ch.Y)
		}
		if a := tr.Get(ch.X, ch.Y); a&(gamedata.AttrSafeZone|gamedata.AttrBlock|gamedata.AttrNoGround) != 0 {
			t.Errorf("%s on cell attribute %#x", name, a)
		}
		if ch.Level != seedLevel || ch.Class>>4 != loadbot.AccountClass(i)>>4 {
			t.Errorf("%s level %d class %#x", name, ch.Level, ch.Class)
		}
		if seen[[2]int{ch.X, ch.Y}] {
			t.Errorf("%s shares (%d,%d)", name, ch.X, ch.Y)
		}
		seen[[2]int{ch.X, ch.Y}] = true
	}
}

func TestUserCounter(t *testing.T) {
	next := &fakeReporter{}
	u := newUserCounter(next)
	u.Report(1, 5, 100)
	u.Report(0, 3, 100)
	if got := u.snapshot(); len(got) != 2 || got[0] != (ServerUsers{0, 3}) || got[1] != (ServerUsers{1, 5}) {
		t.Fatalf("snapshot = %+v", got)
	}
	u.MarkDown(1)
	if got := u.snapshot(); len(got) != 1 || next.reports != 2 || next.downs != 1 {
		t.Fatalf("after MarkDown %+v, %+v", got, next)
	}
}

type fakeReporter struct{ reports, downs int }

func (f *fakeReporter) Report(int, int, int) { f.reports++ }
func (f *fakeReporter) MarkDown(int)         { f.downs++ }

// TestLoadBots runs the whole load test path in process: seeded accounts,
// ConnectServer lookup, three bots walking, chatting and attacking, and
// the /debug/load endpoint.
func TestLoadBots(t *testing.T) {
	if testing.Short() {
		t.Skip("long")
	}
	ctx, cancel := context.WithCancel(context.Background())
	ready := make(chan Ready, 1)
	done := make(chan error, 1)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	go func() {
		done <- run(ctx, []string{"-config", "../../config", "-data", "../../data", "-connect-addr", "127.0.0.1:0",
			"-bind", "127.0.0.1", "-blacklist", "none", "-instances", "gameserver", "-max-conn-per-ip", "0",
			"-seed-load", "3", "-metrics-addr", "127.0.0.1:0"}, log, ready)
	}()
	var rd Ready
	select {
	case rd = <-ready:
	case err := <-done:
		t.Fatalf("run: %v", err)
	case <-time.After(60 * time.Second):
		t.Fatal("not ready")
	}
	defer func() {
		cancel()
		<-done
	}()

	cl, err := loadbot.LoadClient("../../config/gameserver/gameserver.toml", "../../internal/protocol/crypto/testdata")
	if err != nil {
		t.Fatal(err)
	}
	// The registry learns the GameServer on its first report.
	var addr string
	for deadline := time.Now().Add(10 * time.Second); ; {
		if addr, err = loadbot.GameServerAddress(rd.Connect, 0, "127.0.0.1", 2*time.Second); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("connect server: %v", err)
		}
		time.Sleep(200 * time.Millisecond)
	}
	if addr != rd.Games[0] {
		t.Fatalf("game server %s, want %s", addr, rd.Games[0])
	}

	stats := &loadbot.Stats{}
	tr := loadTerrain(t)
	prof := loadbot.DefaultProfile
	prof.ChatEvery = 2 * time.Second
	prof.WalkEvery = 300 * time.Millisecond
	prof.AttackEvery = 300 * time.Millisecond
	bctx, bstop := context.WithTimeout(ctx, 8*time.Second)
	defer bstop()
	var wg sync.WaitGroup
	errs := make(chan error, 3)
	for i := 1; i <= 3; i++ {
		login, char := loadbot.Account(i)
		b := &loadbot.Bot{Client: cl, Addr: addr, Account: login, Password: loadbot.Password, Char: char,
			Class: loadbot.AccountClass(i) >> 4, Profile: prof, Terrain: tr, Stats: stats}
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := b.Run(bctx); err != nil {
				errs <- err
			}
		}()
	}

	// /debug/load sees the bots while they play.
	time.Sleep(4 * time.Second)
	resp, err := http.Get("http://" + rd.Metrics + "/debug/load")
	if err != nil {
		t.Fatal(err)
	}
	var s LoadSample
	err = json.NewDecoder(resp.Body).Decode(&s)
	resp.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if s.Goroutines == 0 || s.HeapInuse == 0 || s.Sys == 0 || s.UnixMilli == 0 {
		t.Errorf("sample = %+v", s)
	}
	if len(s.Users) != 1 || s.Users[0].Users != 3 {
		t.Errorf("users = %+v", s.Users)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
	if stats.LoginOK.Load() != 3 || stats.Disconnects.Load() != 0 {
		t.Fatalf("logins %d, disconnects %d", stats.LoginOK.Load(), stats.Disconnects.Load())
	}
	if stats.Walks.Load() == 0 || stats.Chats.Load() == 0 || stats.Attacks.Load() == 0 {
		t.Fatalf("walks %d chats %d attacks %d", stats.Walks.Load(), stats.Chats.Load(), stats.Attacks.Load())
	}
	t.Logf("walks %d attacks %d hits %d kills %d deaths %d packets %d",
		stats.Walks.Load(), stats.Attacks.Load(), stats.Hits.Load(), stats.Kills.Load(), stats.Deaths.Load(), stats.PacketsIn.Load())
}
