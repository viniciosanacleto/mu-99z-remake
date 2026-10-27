package main

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/pprof"
	"runtime"
	"sort"
	"sync"
	"time"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game"
)

// userCounter forwards the GameServer user reports to the ConnectServer
// registry and keeps the last value of each server for /debug/load.
type userCounter struct {
	next  game.Reporter
	mu    sync.Mutex
	users map[int]int
}

func newUserCounter(next game.Reporter) *userCounter {
	return &userCounter{next: next, users: map[int]int{}}
}

func (u *userCounter) Report(code, users, maxUsers int) {
	u.mu.Lock()
	u.users[code] = users
	u.mu.Unlock()
	u.next.Report(code, users, maxUsers)
}

func (u *userCounter) MarkDown(code int) {
	u.mu.Lock()
	delete(u.users, code)
	u.mu.Unlock()
	u.next.MarkDown(code)
}

// ServerUsers is one GameServer entry of /debug/load.
type ServerUsers struct {
	Code  int `json:"code"`
	Users int `json:"users"`
}

func (u *userCounter) snapshot() []ServerUsers {
	u.mu.Lock()
	defer u.mu.Unlock()
	out := make([]ServerUsers, 0, len(u.users))
	for c, n := range u.users {
		out = append(out, ServerUsers{Code: c, Users: n})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Code < out[j].Code })
	return out
}

// LoadSample is the /debug/load body that cmd/muload samples.
type LoadSample struct {
	UnixMilli    int64         `json:"unix_ms"`
	CPUUserSec   float64       `json:"cpu_user_s"`
	CPUSysSec    float64       `json:"cpu_sys_s"`
	MaxRSSBytes  int64         `json:"max_rss_bytes"`
	Goroutines   int           `json:"goroutines"`
	HeapAlloc    uint64        `json:"heap_alloc"`
	HeapInuse    uint64        `json:"heap_inuse"`
	StackInuse   uint64        `json:"stack_inuse"`
	Sys          uint64        `json:"sys"`
	NumGC        uint32        `json:"num_gc"`
	GCPauseTotal float64       `json:"gc_pause_total_s"`
	Users        []ServerUsers `json:"users"`
}

func loadSample(u *userCounter) LoadSample {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	s := LoadSample{
		UnixMilli:    time.Now().UnixMilli(),
		Goroutines:   runtime.NumGoroutine(),
		HeapAlloc:    ms.HeapAlloc,
		HeapInuse:    ms.HeapInuse,
		StackInuse:   ms.StackInuse,
		Sys:          ms.Sys,
		NumGC:        ms.NumGC,
		GCPauseTotal: float64(ms.PauseTotalNs) / 1e9,
		Users:        u.snapshot(),
	}
	s.CPUUserSec, s.CPUSysSec, s.MaxRSSBytes = processUsage()
	return s
}

// metricsHandler serves pprof under /debug/pprof/ and the sample under
// /debug/load.
func metricsHandler(u *userCounter) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	mux.HandleFunc("/debug/load", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(loadSample(u))
	})
	return mux
}

func serveMetrics(ctx context.Context, ln net.Listener, u *userCounter) error {
	srv := &http.Server{Handler: metricsHandler(u), ReadHeaderTimeout: 5 * time.Second}
	go func() {
		<-ctx.Done()
		srv.Close()
	}()
	if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
