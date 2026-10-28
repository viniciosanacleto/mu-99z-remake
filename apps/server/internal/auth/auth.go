// Package auth replaces the original JoinServer: account login, the
// single-session-per-account rule, account levels and the authentication
// codes used when a character moves between game server instances
// (docs/server-plan/04-auth-join-server.md).
package auth

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/store"
)

// Login result codes (JoinServer FUN_00404a50, local_5d). The GameServer
// forwards them to the client in F1 01.
const (
	ResultWrongPassword  byte = 0
	ResultOK             byte = 1
	ResultInvalidAccount byte = 2
	ResultConnected      byte = 3
	ResultServerFull     byte = 4
	ResultIPLimit        byte = 7
)

// maxSessions is the JoinServer capacity check (iVar8 < 10000).
const maxSessions = 10000

// Config mirrors [JoinServerInfo] in JoinServer.ini.
type Config struct {
	MaxConnectionPerIP  int // 0 = unlimited
	MaxConnectionPerHID int // 0 = unlimited
	// Log receives the store failures of best-effort steps (password
	// rehash, logout); nil uses slog.Default().
	Log *slog.Logger
}

// Kicker is implemented by the game instance holding a session; it receives
// the JoinServer 0x30 "already connected" notification.
type Kicker interface {
	KickAccount(account string)
}

// Session is an online account (the JoinServer in-memory account table).
type Session struct {
	Moving     bool // between instances: the old instance must not log out
	Account    store.Account
	Password   string
	IP, HWID   string
	ServerCode int
	kicker     Kicker
}

// LoginRequest is the GS -> JS 0x01 request.
type LoginRequest struct {
	Account    string
	Password   string
	IP         string
	HWID       string
	ServerCode int
	ServerName string
	Kicker     Kicker
}

// MoveTicket is issued by BeginServerMove (JS 0x03) and redeemed by
// CompleteServerMove (JS 0x04).
type MoveTicket struct {
	Account    string
	Name       string
	ServerCode int
	Map, X, Y  int
	Codes      [4]uint32
	expires    time.Time
}

// Service is safe for concurrent use by every game instance.
type Service struct {
	store store.Store
	cfg   Config
	now   func() time.Time

	mu       sync.Mutex
	sessions map[string]*Session // key: lowercase login (FUN_00401530 lowercases)
	tickets  map[string]*MoveTicket
}

// New builds the account service on top of s.
func New(s store.Store, cfg Config) *Service {
	if cfg.Log == nil {
		cfg.Log = slog.Default()
	}
	return &Service{
		store:    s,
		cfg:      cfg,
		now:      time.Now,
		sessions: map[string]*Session{},
		tickets:  map[string]*MoveTicket{},
	}
}

func key(account string) string { return strings.ToLower(account) }

// Login reproduces FUN_00404a50.
func (s *Service) Login(ctx context.Context, req LoginRequest) (byte, *Session, error) {
	if strings.ContainsAny(req.Account, " \"'") {
		return ResultInvalidAccount, nil, nil
	}

	s.mu.Lock()
	if cur := s.sessions[key(req.Account)]; cur != nil {
		kicker, same := cur.kicker, cur.Password == req.Password
		offline, hash := cur.Password == "", cur.Account.PasswordHash
		s.mu.Unlock()
		if offline { // LoginOffline session (offline.go)
			same = offlineSessionMatches(hash, req.Password)
		}
		if same && kicker != nil {
			kicker.KickAccount(cur.Account.Login)
		}
		return ResultConnected, nil, nil
	}
	if !s.withinLimitsLocked(req.IP, req.HWID) {
		s.mu.Unlock()
		return ResultIPLimit, nil, nil
	}
	if len(s.sessions) >= maxSessions {
		s.mu.Unlock()
		return ResultServerFull, nil, nil
	}
	s.mu.Unlock()

	acc, err := s.store.AccountByLogin(ctx, req.Account)
	if errors.Is(err, store.ErrNotFound) {
		return ResultInvalidAccount, nil, nil
	}
	if err != nil {
		return 0, nil, err
	}
	// The original re-reads memb___id and compares it with strcmp, so the
	// login is case-sensitive even though SQL Server matched it ignoring case.
	if acc.Login != req.Account {
		return ResultInvalidAccount, nil, nil
	}
	ok, rehash, err := CheckPassword(acc.PasswordHash, req.Password)
	if err != nil {
		return 0, nil, err
	}
	if !ok {
		return ResultWrongPassword, nil, nil
	}
	if rehash {
		s.upgradePasswordHash(ctx, acc.ID, req.Password)
	}
	if err := s.expireLevel(ctx, acc); err != nil {
		return 0, nil, err
	}

	sess := &Session{Account: *acc, Password: req.Password, IP: req.IP, HWID: req.HWID,
		ServerCode: req.ServerCode, kicker: req.Kicker}
	s.mu.Lock()
	if s.sessions[key(req.Account)] != nil {
		s.mu.Unlock()
		return ResultConnected, nil, nil
	}
	s.sessions[key(req.Account)] = sess
	s.mu.Unlock()

	if err := s.store.ConnectAccount(ctx, acc.ID, req.ServerName, req.IP); err != nil {
		s.Logout(ctx, req.Account)
		return 0, nil, err
	}
	return ResultOK, sess, nil
}

// withinLimitsLocked is FUN_004019c0: counts sessions with the same IP and
// hardware id against MaxConnectionPerIP / MaxConnectionPerHID.
func (s *Service) withinLimitsLocked(ip, hwid string) bool {
	if s.cfg.MaxConnectionPerIP == 0 && s.cfg.MaxConnectionPerHID == 0 {
		return true
	}
	ips, hids := 0, 0
	for _, sess := range s.sessions {
		if sess.IP == ip {
			ips++
		}
		if sess.HWID == hwid {
			hids++
		}
	}
	if s.cfg.MaxConnectionPerIP > 0 && ips >= s.cfg.MaxConnectionPerIP {
		return false
	}
	if s.cfg.MaxConnectionPerHID > 0 && hids >= s.cfg.MaxConnectionPerHID {
		return false
	}
	return true
}

// expireLevel follows WZ_GetAccountLevel: an expired VIP level drops to 0
// and the change is persisted.
func (s *Service) expireLevel(ctx context.Context, acc *store.Account) error {
	if acc.Level != 0 && !acc.LevelExpiresAt.IsZero() && s.now().After(acc.LevelExpiresAt) {
		acc.Level = 0
		return s.store.SetAccountLevel(ctx, acc.ID, 0, acc.LevelExpiresAt)
	}
	return nil
}

// Logout replaces JS 0x02 (WZ_DISCONNECT_MEMB).
func (s *Service) Logout(ctx context.Context, account string) {
	s.mu.Lock()
	sess := s.sessions[key(account)]
	delete(s.sessions, key(account))
	s.mu.Unlock()
	if sess == nil {
		return
	}
	if err := s.store.DisconnectAccount(ctx, sess.Account.ID); err != nil {
		s.cfg.Log.Warn("auth: marking the account disconnected", "account", account, "err", err)
	}
}

// upgradePasswordHash stores a current hash of password after a login
// that matched an outdated one. A failure keeps the old hash, which still
// works, so it is only logged.
func (s *Service) upgradePasswordHash(ctx context.Context, accountID int64, password string) {
	h, err := HashPassword(password)
	if err == nil {
		err = s.store.SetPasswordHash(ctx, accountID, h)
	}
	if err != nil {
		s.cfg.Log.Warn("auth: upgrading the password hash", "account_id", accountID, "err", err)
	}
}

// Online reports whether an account holds a session.
func (s *Service) Online(account string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sessions[key(account)] != nil
}

// Count returns the number of sessions (the JS queue/user counters).
func (s *Service) Count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.sessions)
}

// SetAccountLevel replaces WZ_SetAccountLevel: the same level extends the
// current expiration, a different level starts from now.
func (s *Service) SetAccountLevel(ctx context.Context, account string, level int, d time.Duration) error {
	acc, err := s.store.AccountByLogin(ctx, account)
	if err != nil {
		return err
	}
	base := s.now()
	if acc.Level == level && acc.LevelExpiresAt.After(base) {
		base = acc.LevelExpiresAt
	}
	expires := base.Add(d)
	if err := s.store.SetAccountLevel(ctx, acc.ID, level, expires); err != nil {
		return err
	}
	s.mu.Lock()
	if sess := s.sessions[key(account)]; sess != nil {
		sess.Account.Level, sess.Account.LevelExpiresAt = level, expires
	}
	s.mu.Unlock()
	return nil
}

// SetLock replaces JS 0x12.
func (s *Service) SetLock(ctx context.Context, account string, lock int) error {
	acc, err := s.store.AccountByLogin(ctx, account)
	if err != nil {
		return err
	}
	return s.store.SetAccountLock(ctx, acc.ID, lock)
}

// BeginServerMove (JS 0x03) issues four random authentication codes that
// the destination instance must present.
func (s *Service) BeginServerMove(account, name string, serverCode, mp, x, y int) (MoveTicket, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return MoveTicket{}, err
	}
	t := MoveTicket{Account: account, Name: name, ServerCode: serverCode, Map: mp, X: x, Y: y,
		expires: s.now().Add(30 * time.Second)}
	for i := range t.Codes {
		t.Codes[i] = binary.LittleEndian.Uint32(b[i*4:])
	}
	s.mu.Lock()
	s.tickets[key(account)] = &t
	s.mu.Unlock()
	return t, nil
}

// CompleteServerMove (JS 0x04) validates the codes. ok is false for result 4
// (authentication failure).
func (s *Service) CompleteServerMove(account, name string, codes [4]uint32) (MoveTicket, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t := s.tickets[key(account)]
	if t == nil || t.Codes != codes || !strings.EqualFold(t.Name, name) || s.now().After(t.expires) {
		return MoveTicket{}, false
	}
	delete(s.tickets, key(account))
	return *t, true
}

// SetSessionServer updates the instance holding a session after a move.
func (s *Service) SetSessionServer(account string, serverCode int, k Kicker) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sess := s.sessions[key(account)]; sess != nil {
		sess.ServerCode, sess.kicker = serverCode, k
	}
}

// SetMoving marks a session as moving between instances.
func (s *Service) SetMoving(account string, moving bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sess := s.sessions[key(account)]; sess != nil {
		sess.Moving = moving
	}
}

// Session returns a copy of an online session, or nil.
func (s *Service) Session(account string) *Session {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess := s.sessions[key(account)]
	if sess == nil {
		return nil
	}
	c := *sess
	return &c
}

// LogoutUnlessMoving is called when an instance drops a client: a session
// that is moving to another instance survives.
func (s *Service) LogoutUnlessMoving(ctx context.Context, account string) {
	s.mu.Lock()
	sess := s.sessions[key(account)]
	if sess != nil && sess.Moving {
		s.mu.Unlock()
		return
	}
	s.mu.Unlock()
	s.Logout(ctx, account)
}
