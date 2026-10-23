package auth

import (
	"context"
	"errors"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/store"
)

// LoginOffline is the JoinServer login (JS 0x01, FUN_004a26e0) the
// GameServer sends for an offline character it re-creates at start-up
// from the reconnect data ("Offline Reconnect", FUN_004e2020). The
// original record carries the plaintext password; the rewrite does not
// keep it, so the password check is skipped and the session gets an empty
// password. A later client login of the account then compares its
// password with the stored hash to decide the "already connected" kick
// (Login).
func (s *Service) LoginOffline(ctx context.Context, req LoginRequest) (byte, *Session, error) {
	s.mu.Lock()
	if s.sessions[key(req.Account)] != nil {
		s.mu.Unlock()
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
	if acc.Login != req.Account {
		return ResultInvalidAccount, nil, nil
	}
	if err := s.expireLevel(ctx, acc); err != nil {
		return 0, nil, err
	}
	sess := &Session{Account: *acc, IP: req.IP, HWID: req.HWID, ServerCode: req.ServerCode, kicker: req.Kicker}
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

// offlineSessionMatches is the password comparison of the "already
// connected" kick for a LoginOffline session (empty password): the
// request password is checked against the account hash.
func offlineSessionMatches(hash, pw string) bool {
	ok, _, err := CheckPassword(hash, pw)
	return err == nil && ok
}
