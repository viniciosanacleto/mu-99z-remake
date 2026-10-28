package auth

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/store"
)

type kickRecorder struct{ kicked []string }

func (k *kickRecorder) KickAccount(a string) { k.kicked = append(k.kicked, a) }

func setup(t *testing.T, cfg Config) (*Service, *store.Memory) {
	t.Helper()
	m, err := store.NewMemory()
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err := m.CreateAccount(ctx, "player", "plain:secret", "1234567"); err != nil {
		t.Fatal(err)
	}
	h, err := HashPassword("pw2")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.CreateAccount(ctx, "second", h, ""); err != nil {
		t.Fatal(err)
	}
	return New(m, cfg), m
}

func login(s *Service, acc, pw, ip string, k Kicker) byte {
	r, _, err := s.Login(context.Background(), LoginRequest{Account: acc, Password: pw, IP: ip, HWID: "hw", Kicker: k})
	if err != nil {
		panic(err)
	}
	return r
}

func TestLoginResults(t *testing.T) {
	s, m := setup(t, Config{})
	cases := []struct {
		acc, pw string
		want    byte
	}{
		{"pla yer", "x", ResultInvalidAccount},
		{"o'neil", "x", ResultInvalidAccount},
		{"nobody", "x", ResultInvalidAccount},
		{"PLAYER", "secret", ResultInvalidAccount}, // strcmp on memb___id
		{"player", "Secret", ResultWrongPassword},
	}
	for _, c := range cases {
		if got := login(s, c.acc, c.pw, "1.1.1.1", nil); got != c.want {
			t.Errorf("%s/%s = %d, want %d", c.acc, c.pw, got, c.want)
		}
	}
	if got := login(s, "player", "secret", "1.1.1.1", nil); got != ResultOK {
		t.Fatalf("ok login = %d", got)
	}
	if !s.Online("PLAYER") || s.Count() != 1 {
		t.Fatal("session not registered")
	}
	acc, _ := m.AccountByLogin(context.Background(), "player")
	if !strings.HasPrefix(acc.PasswordHash, schemeArgon2) {
		t.Fatalf("plain password not upgraded: %q", acc.PasswordHash)
	}
	if !m.Online(acc.ID) {
		t.Fatal("ConnectAccount not called")
	}
	s.Logout(context.Background(), "player")
	if s.Online("player") || m.Online(acc.ID) {
		t.Fatal("logout did not clear the session")
	}
	// The upgraded hash still accepts the password.
	if got := login(s, "player", "secret", "1.1.1.1", nil); got != ResultOK {
		t.Fatalf("login after rehash = %d", got)
	}
}

// failingStore fails the best-effort writes of Login and Logout.
type failingStore struct{ *store.Memory }

var errStoreDown = errors.New("store down")

func (failingStore) SetPasswordHash(context.Context, int64, string) error { return errStoreDown }
func (failingStore) DisconnectAccount(context.Context, int64) error       { return errStoreDown }

func TestBestEffortStoreFailuresAreLogged(t *testing.T) {
	_, m := setup(t, Config{})
	var logs bytes.Buffer
	s := New(failingStore{m}, Config{Log: slog.New(slog.NewTextHandler(&logs, nil))})
	// "player" has a plain password, so the login tries to rehash it.
	if got := login(s, "player", "secret", "1.1.1.1", nil); got != ResultOK {
		t.Fatalf("login with a failing rehash = %d, want %d", got, ResultOK)
	}
	if !strings.Contains(logs.String(), "upgrading the password hash") {
		t.Fatalf("rehash failure not logged: %q", logs.String())
	}
	s.Logout(context.Background(), "player")
	if s.Online("player") {
		t.Fatal("logout kept the session after a store failure")
	}
	if !strings.Contains(logs.String(), "marking the account disconnected") {
		t.Fatalf("logout failure not logged: %q", logs.String())
	}
}

func TestDuplicateLoginKicksOnlyWithSamePassword(t *testing.T) {
	s, _ := setup(t, Config{})
	k := &kickRecorder{}
	if login(s, "second", "pw2", "1.1.1.1", k) != ResultOK {
		t.Fatal("first login")
	}
	if got := login(s, "Second", "wrong", "2.2.2.2", nil); got != ResultConnected || len(k.kicked) != 0 {
		t.Fatalf("different password: %d, kicked %v", got, k.kicked)
	}
	if got := login(s, "second", "pw2", "2.2.2.2", nil); got != ResultConnected || len(k.kicked) != 1 {
		t.Fatalf("same password: %d, kicked %v", got, k.kicked)
	}
}

func TestIPLimit(t *testing.T) {
	s, _ := setup(t, Config{MaxConnectionPerIP: 1})
	if login(s, "player", "secret", "9.9.9.9", nil) != ResultOK {
		t.Fatal("first")
	}
	if got := login(s, "second", "pw2", "9.9.9.9", nil); got != ResultIPLimit {
		t.Fatalf("second from same ip = %d", got)
	}
	if got := login(s, "second", "pw2", "8.8.8.8", nil); got == ResultIPLimit {
		t.Fatal("different ip blocked")
	}
}

func TestAccountLevelExpiry(t *testing.T) {
	s, m := setup(t, Config{})
	ctx := context.Background()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	if err := s.SetAccountLevel(ctx, "player", 2, time.Hour); err != nil {
		t.Fatal(err)
	}
	// Same level extends from the current expiration.
	if err := s.SetAccountLevel(ctx, "player", 2, time.Hour); err != nil {
		t.Fatal(err)
	}
	acc, _ := m.AccountByLogin(ctx, "player")
	if acc.Level != 2 || !acc.LevelExpiresAt.Equal(now.Add(2*time.Hour)) {
		t.Fatalf("level %d expires %v", acc.Level, acc.LevelExpiresAt)
	}
	// A different level restarts from now.
	if err := s.SetAccountLevel(ctx, "player", 1, time.Hour); err != nil {
		t.Fatal(err)
	}
	acc, _ = m.AccountByLogin(ctx, "player")
	if acc.Level != 1 || !acc.LevelExpiresAt.Equal(now.Add(time.Hour)) {
		t.Fatalf("level %d expires %v", acc.Level, acc.LevelExpiresAt)
	}
	// After expiry the login sees level 0 and persists it.
	now = now.Add(2 * time.Hour)
	_, sess, err := s.Login(ctx, LoginRequest{Account: "player", Password: "secret"})
	if err != nil || sess.Account.Level != 0 {
		t.Fatalf("expired level = %+v, %v", sess, err)
	}
	acc, _ = m.AccountByLogin(ctx, "player")
	if acc.Level != 0 {
		t.Fatal("expired level not persisted")
	}
}

func TestServerMoveTicket(t *testing.T) {
	s, _ := setup(t, Config{})
	tk, err := s.BeginServerMove("player", "Hero", 19, 30, 100, 120)
	if err != nil {
		t.Fatal(err)
	}
	bad := tk.Codes
	bad[0]++
	if _, ok := s.CompleteServerMove("player", "Hero", bad); ok {
		t.Fatal("wrong codes accepted")
	}
	got, ok := s.CompleteServerMove("PLAYER", "hero", tk.Codes)
	if !ok || got.Map != 30 || got.ServerCode != 19 {
		t.Fatalf("ticket = %+v %v", got, ok)
	}
	if _, ok := s.CompleteServerMove("player", "Hero", tk.Codes); ok {
		t.Fatal("ticket reused")
	}
}

func TestPasswordSchemes(t *testing.T) {
	if _, _, err := CheckPassword("md5:abc", "x"); err == nil {
		t.Fatal("unknown scheme accepted")
	}
	if _, _, err := CheckPassword("argon2id:bad", "x"); err == nil {
		t.Fatal("malformed hash accepted")
	}
	h, _ := HashPassword("abc")
	if ok, re, err := CheckPassword(h, "abc"); !ok || re || err != nil {
		t.Fatal("argon2 check failed")
	}
	if ok, _, _ := CheckPassword(h, "ABC"); ok {
		t.Fatal("password check must be case-sensitive")
	}
}

// LoginOffline (FUN_004a26e0 for an Offline Reconnect) needs no password;
// a client login with the account password kicks that session.
func TestLoginOfflineSession(t *testing.T) {
	s, _ := setup(t, Config{})
	k := &kickRecorder{}
	ctx := context.Background()
	r, sess, err := s.LoginOffline(ctx, LoginRequest{Account: "second", IP: "1.1.1.1", HWID: "hw", Kicker: k})
	if err != nil || r != ResultOK || sess == nil || sess.Password != "" || !s.Online("second") {
		t.Fatalf("offline login %d %+v %v", r, sess, err)
	}
	if r, _, _ := s.LoginOffline(ctx, LoginRequest{Account: "second"}); r != ResultConnected {
		t.Fatalf("second offline login %d", r)
	}
	if r, _, _ := s.LoginOffline(ctx, LoginRequest{Account: "nobody"}); r != ResultInvalidAccount {
		t.Fatalf("unknown account %d", r)
	}
	if got := login(s, "second", "wrong", "2.2.2.2", nil); got != ResultConnected || len(k.kicked) != 0 {
		t.Fatalf("wrong password: %d, kicked %v", got, k.kicked)
	}
	if got := login(s, "second", "pw2", "2.2.2.2", nil); got != ResultConnected || len(k.kicked) != 1 {
		t.Fatalf("right password: %d, kicked %v", got, k.kicked)
	}
}
