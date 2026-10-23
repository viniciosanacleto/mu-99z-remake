// Package connect replaces the original ConnectServer: it greets clients,
// lists the game servers and hands out their addresses
// (docs/server-plan/03-connect-server.md).
package connect

import (
	"sort"
	"sync"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/config/serverlist"
)

// authQueueLimit: the list is only served while the JoinServer queue is
// below 101 (DAT_0040f5ac < 0x65 in FUN_004035f0/FUN_00405c40/FUN_00405d00).
const authQueueLimit = 0x65

// ServerStatus is what the original kept per ServerList.dat entry; Alive and
// the user counters came from the GameServer UDP heartbeat (0xA1).
type ServerStatus struct {
	serverlist.Entry
	Alive    bool
	Users    int
	MaxUsers int
}

// Load is the percentage sent in F4 06. The original divides without
// checking for zero; here an unknown maximum reports 0.
func (s ServerStatus) Load() byte {
	if s.MaxUsers <= 0 {
		return 0
	}
	return byte(s.Users * 100 / s.MaxUsers)
}

// Registry holds the live state that the GameServer and JoinServer
// heartbeats used to provide. It is safe for concurrent use.
type Registry struct {
	mu        sync.RWMutex
	servers   map[int]*ServerStatus
	authAlive bool
	authQueue int
}

// NewRegistry starts with every server down and the auth module down.
func NewRegistry(entries []serverlist.Entry) *Registry {
	r := &Registry{servers: map[int]*ServerStatus{}}
	for _, e := range entries {
		r.servers[e.Code] = &ServerStatus{Entry: e}
	}
	return r
}

// Report replaces the 0xA1 heartbeat: the game instance `code` is up with
// the given user counts. Unknown codes are ignored, as in the original.
func (r *Registry) Report(code, users, maxUsers int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if s := r.servers[code]; s != nil {
		s.Alive, s.Users, s.MaxUsers = true, users, maxUsers
	}
}

// MarkDown replaces the heartbeat timeout for a game instance.
func (r *Registry) MarkDown(code int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if s := r.servers[code]; s != nil {
		s.Alive = false
	}
}

// SetAuthStatus replaces the 0xA2 JoinServer heartbeat.
func (r *Registry) SetAuthStatus(alive bool, queue int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.authAlive, r.authQueue = alive, queue
}

// serving reports whether lists and addresses may be handed out.
func (r *Registry) serving() bool { return r.authAlive && r.authQueue < authQueueLimit }

// snapshot returns the servers ordered by code (the original iterates a
// std::map keyed by code) and whether the auth gate is open.
func (r *Registry) snapshot() ([]ServerStatus, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]ServerStatus, 0, len(r.servers))
	for _, s := range r.servers {
		out = append(out, *s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Code < out[j].Code })
	return out, r.serving()
}

func (r *Registry) lookup(code int) (ServerStatus, bool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s := r.servers[code]
	if s == nil {
		return ServerStatus{}, false, r.serving()
	}
	return *s, true, r.serving()
}
