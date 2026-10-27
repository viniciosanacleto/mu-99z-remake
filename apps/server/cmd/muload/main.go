// Command muload is the load test of the server: it puts 1, 10, 50, 100...
// fake players on a running muserver (accounts from -seed-load) and samples
// the server's CPU and memory (docs/server-plan/18-teste-de-carga.md).
//
//	muload -connect 127.0.0.1:44405 -metrics http://127.0.0.1:6060/debug/load -pid <muserver pid>
package main

import (
	"context"
	"encoding/csv"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/loadbot"
)

type config struct {
	connect    string
	serverCode int
	gameHost   string
	gsConfig   string
	keyDir     string
	dataDir    string
	steps      []int
	first      int
	ramp       float64
	idle       time.Duration
	steady     time.Duration
	cooldown   time.Duration
	interval   time.Duration
	metrics    string
	pid        int
	container  string
	out        string
	profile    loadbot.Profile
}

func parseFlags(args []string) (*config, error) {
	fs := flag.NewFlagSet("muload", flag.ContinueOnError)
	c := &config{profile: loadbot.DefaultProfile}
	var steps string
	fs.StringVar(&c.connect, "connect", "127.0.0.1:44405", "ConnectServer address")
	fs.IntVar(&c.serverCode, "server-code", 0, "GameServer code asked to the ConnectServer")
	fs.StringVar(&c.gameHost, "game-host", "", "host to dial instead of the address the ConnectServer answers with (default the -connect host; \"server\" keeps the answered address)")
	fs.StringVar(&c.gsConfig, "gs-config", "config/gameserver/gameserver.toml", "GameServer instance file (version, serial, stream keys)")
	fs.StringVar(&c.keyDir, "keys", "internal/protocol/crypto/testdata", "folder with the client keys Enc1.dat and Dec2.dat")
	fs.StringVar(&c.dataDir, "data", "data", "server data folder (Data/Terrain/Terrain1.att keeps the bots off blocked cells; optional)")
	fs.StringVar(&steps, "steps", "1,10,50,100", "comma-separated number of simultaneous players of each step")
	fs.IntVar(&c.first, "first", 1, "first load test account (load0001 = 1)")
	fs.Float64Var(&c.ramp, "ramp", 5, "logins started per second")
	fs.DurationVar(&c.idle, "idle", 15*time.Second, "idle baseline sampled before the first step")
	fs.DurationVar(&c.steady, "steady", 2*time.Minute, "time every step keeps all its players in the world")
	fs.DurationVar(&c.cooldown, "cooldown", 30*time.Second, "pause after each step (players logged out)")
	fs.DurationVar(&c.interval, "interval", time.Second, "sampling interval")
	fs.StringVar(&c.metrics, "metrics", "", "muserver /debug/load URL (muserver -metrics-addr)")
	fs.IntVar(&c.pid, "pid", 0, "muserver process id, sampled with ps (RSS and CPU)")
	fs.StringVar(&c.container, "docker", "", "muserver container, sampled with docker stats (RSS and CPU)")
	fs.StringVar(&c.out, "out", "", "output folder (default loadtest-<time>)")
	fs.DurationVar(&c.profile.WalkEvery, "walk-every", c.profile.WalkEvery, "random walk interval (jittered up to 2x)")
	fs.DurationVar(&c.profile.AttackEvery, "attack-every", c.profile.AttackEvery, "attack interval while a monster is in reach")
	fs.DurationVar(&c.profile.ChatEvery, "chat-every", c.profile.ChatEvery, "chat interval (0 = no chat)")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	var err error
	if c.steps, err = parseSteps(steps); err != nil {
		return nil, err
	}
	if c.ramp <= 0 || c.first < 1 || c.interval <= 0 {
		return nil, errors.New("-ramp and -interval must be positive and -first at least 1")
	}
	if c.metrics == "" && c.pid == 0 && c.container == "" {
		return nil, errors.New("nothing to measure: give -metrics, -pid or -docker")
	}
	if c.pid != 0 && c.container != "" {
		return nil, errors.New("-pid and -docker are exclusive")
	}
	switch c.gameHost {
	case "":
		host, _, err := net.SplitHostPort(c.connect)
		if err != nil {
			return nil, fmt.Errorf("-connect: %w", err)
		}
		c.gameHost = host
	case "server":
		c.gameHost = ""
	}
	if c.out == "" {
		c.out = "loadtest-" + time.Now().Format("20060102-150405")
	}
	return c, nil
}

func parseSteps(s string) ([]int, error) {
	var out []int
	for _, f := range strings.Split(s, ",") {
		if f = strings.TrimSpace(f); f == "" {
			continue
		}
		n, err := strconv.Atoi(f)
		if err != nil || n < 1 {
			return nil, fmt.Errorf("-steps: bad step %q", f)
		}
		out = append(out, n)
	}
	if len(out) == 0 {
		return nil, errors.New("-steps: no step")
	}
	return out, nil
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if err := run(ctx, os.Args[1:], log); err != nil {
		log.Error("muload", "err", err)
		os.Exit(1)
	}
}

// state is what the sampler labels each sample with.
type state struct {
	mu    sync.Mutex
	step  int
	phase string
	stats *loadbot.Stats
}

func (s *state) set(step int, phase string, stats *loadbot.Stats) {
	s.mu.Lock()
	s.step, s.phase, s.stats = step, phase, stats
	s.mu.Unlock()
}

func (s *state) get() (int, string, int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var bots int64
	if s.stats != nil {
		bots = s.stats.Online.Load()
	}
	return s.step, s.phase, bots
}

func run(ctx context.Context, args []string, log *slog.Logger) error {
	cfg, err := parseFlags(args)
	if err != nil {
		return err
	}
	client, err := loadbot.LoadClient(cfg.gsConfig, cfg.keyDir)
	if err != nil {
		return err
	}
	terrain := loadTerrain(cfg.dataDir, log)
	if err := os.MkdirAll(cfg.out, 0o755); err != nil {
		return err
	}
	// Fail early when the GameServer cannot be found.
	if _, err := loadbot.GameServerAddress(cfg.connect, byte(cfg.serverCode), cfg.gameHost, 5*time.Second); err != nil {
		return fmt.Errorf("connect server %s: %w", cfg.connect, err)
	}

	st := &state{phase: "idle"}
	stopSampler, err := startSampler(ctx, cfg, st, log)
	if err != nil {
		return err
	}
	info := RunInfo{Started: time.Now(), Target: cfg.connect, Source: sourceName(cfg), Ramp: cfg.ramp, Steady: cfg.steady,
		Host:    fmt.Sprintf("%s/%s, %d CPUs", runtime.GOOS, runtime.GOARCH, runtime.NumCPU()),
		Profile: fmt.Sprintf("walk every %s, attack every %s, chat every %s", cfg.profile.WalkEvery, cfg.profile.AttackEvery, cfg.profile.ChatEvery)}

	var results []StepResult
	runErr := func() error {
		log.Info("idle baseline", "for", cfg.idle)
		if err := sleep(ctx, cfg.idle); err != nil {
			return err
		}
		for _, n := range cfg.steps {
			r, err := runStep(ctx, cfg, n, client, terrain, st, log)
			results = append(results, r)
			if err != nil {
				return err
			}
		}
		return nil
	}()
	all := stopSampler()

	f, err := os.Create(filepath.Join(cfg.out, "summary.md"))
	if err != nil {
		return err
	}
	defer f.Close()
	writeSummary(f, info, all, results)
	var sb strings.Builder
	writeSummary(&sb, info, all, results)
	fmt.Println(sb.String())
	log.Info("results written", "dir", cfg.out, "samples", len(all))
	if runErr != nil && !errors.Is(runErr, context.Canceled) {
		return runErr
	}
	return nil
}

func loadTerrain(dataDir string, log *slog.Logger) *gamedata.Terrain {
	b, err := os.ReadFile(filepath.Join(dataDir, "Data", "Terrain", "Terrain1.att"))
	if err != nil {
		log.Warn("no terrain: bots may walk into blocked cells", "err", err)
		return nil
	}
	t, err := gamedata.ParseTerrain("Terrain1.att", b)
	if err != nil {
		log.Warn("bad terrain", "err", err)
		return nil
	}
	return t
}

func sourceName(cfg *config) string {
	var s []string
	if cfg.metrics != "" {
		s = append(s, "/debug/load")
	}
	if cfg.pid != 0 {
		s = append(s, fmt.Sprintf("ps (pid %d)", cfg.pid))
	}
	if cfg.container != "" {
		s = append(s, fmt.Sprintf("docker stats (%s)", cfg.container))
	}
	return strings.Join(s, " + ")
}

func sleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// startSampler writes samples.csv every interval until the returned
// function is called, which returns every sample.
func startSampler(ctx context.Context, cfg *config, st *state, log *slog.Logger) (func() []Sample, error) {
	f, err := os.Create(filepath.Join(cfg.out, "samples.csv"))
	if err != nil {
		return nil, err
	}
	w := csv.NewWriter(f)
	w.Write(csvHeader)
	p := &probe{metricsURL: cfg.metrics, pid: cfg.pid, container: cfg.container, client: &http.Client{Timeout: 5 * time.Second}}
	var (
		mu      sync.Mutex
		samples []Sample
		lastGC  = -1.0
		warned  atomic.Bool
	)
	sctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		tk := time.NewTicker(cfg.interval)
		defer tk.Stop()
		p.read(sctx) // prime the CPU delta
		for {
			select {
			case <-sctx.Done():
				return
			case <-tk.C:
			}
			r, err := p.read(sctx)
			if err != nil && sctx.Err() == nil && !warned.Swap(true) {
				log.Warn("sampling", "err", err)
			}
			step, phase, bots := st.get()
			s := Sample{Time: time.Now(), Step: step, Phase: phase, Bots: bots, CPUPercent: r.cpuPercent, RSSBytes: r.rss}
			if r.server != nil {
				s.HeapInuse, s.Sys, s.Goroutines, s.NumGC = r.server.HeapInuse, r.server.Sys, r.server.Goroutines, r.server.NumGC
				for _, u := range r.server.Users {
					s.Users += u.Users
				}
				if lastGC >= 0 {
					s.GCPause = max(r.server.GCPauseTotal-lastGC, 0)
				}
				lastGC = r.server.GCPauseTotal
			}
			mu.Lock()
			samples = append(samples, s)
			mu.Unlock()
			writeSampleCSV(w, s)
			w.Flush()
		}
	}()
	return func() []Sample {
		cancel()
		<-done
		w.Flush()
		f.Close()
		mu.Lock()
		defer mu.Unlock()
		return append([]Sample(nil), samples...)
	}, nil
}

var addrPattern = regexp.MustCompile(`\d+\.\d+\.\d+\.\d+:\d+`)

// errorKey groups errors that differ only by the socket addresses.
func errorKey(err error) string {
	s := addrPattern.ReplaceAllString(err.Error(), "<addr>")
	if len(s) > 120 {
		s = s[:120]
	}
	return s
}

// runStep logs n bots in at the ramp rate, keeps them for the steady
// phase, logs them out and waits the cooldown.
func runStep(ctx context.Context, cfg *config, n int, client loadbot.Client, terrain *gamedata.Terrain, st *state, log *slog.Logger) (StepResult, error) {
	res := StepResult{Bots: n, Errors: map[string]int{}}
	stats := &loadbot.Stats{}
	st.set(n, "login", stats)
	log.Info("step", "players", n)
	bctx, stopBots := context.WithCancel(ctx)
	defer stopBots()
	var (
		wg     sync.WaitGroup
		mu     sync.Mutex
		logins []time.Duration
	)
	addErr := func(err error) {
		mu.Lock()
		res.Errors[errorKey(err)]++
		mu.Unlock()
	}
	start := time.Now()
	gap := time.Duration(float64(time.Second) / cfg.ramp)
	var started int64
	for i := 0; i < n; i++ {
		if i > 0 {
			if err := sleep(ctx, gap); err != nil {
				break
			}
		}
		idx := cfg.first + i
		account, char := loadbot.Account(idx)
		wg.Add(1)
		started++
		go func() {
			defer wg.Done()
			addr, err := loadbot.GameServerAddress(cfg.connect, byte(cfg.serverCode), cfg.gameHost, 10*time.Second)
			if err != nil {
				stats.LoginFail.Add(1)
				addErr(err)
				return
			}
			b := &loadbot.Bot{Client: client, Addr: addr, Account: account, Password: loadbot.Password, Char: char,
				Class: loadbot.AccountClass(idx) >> 4, Profile: cfg.profile, Terrain: terrain, Stats: stats}
			err = b.Run(bctx)
			if b.LoginTime > 0 {
				mu.Lock()
				logins = append(logins, b.LoginTime)
				mu.Unlock()
			}
			if err != nil {
				addErr(err)
			}
		}()
	}
	// The login phase ends when every bot is in or has failed.
	for ctx.Err() == nil && stats.LoginOK.Load()+stats.LoginFail.Load() < started {
		if time.Since(start) > 2*time.Minute+time.Duration(n)*gap {
			log.Warn("login phase timed out", "in", stats.LoginOK.Load(), "failed", stats.LoginFail.Load())
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	res.LoginPhase = time.Since(start)
	log.Info("players in", "ok", stats.LoginOK.Load(), "failed", stats.LoginFail.Load(), "took", res.LoginPhase.Round(time.Millisecond))

	st.set(n, "steady", stats)
	steadyStart := time.Now()
	err := sleep(ctx, cfg.steady)
	res.SteadyPhase = time.Since(steadyStart)
	stopBots()
	wg.Wait()

	res.LoginOK, res.LoginFail, res.Disconnects = stats.LoginOK.Load(), stats.LoginFail.Load(), stats.Disconnects.Load()
	res.Walks, res.Attacks, res.Kills, res.Deaths = stats.Walks.Load(), stats.Attacks.Load(), stats.Kills.Load(), stats.Deaths.Load()
	res.PacketsIn, res.BytesIn, res.BytesOut = stats.PacketsIn.Load(), stats.BytesIn.Load(), stats.BytesOut.Load()
	res.LoginTimes = logins
	log.Info("step done", "players", n, "disconnects", res.Disconnects, "attacks", res.Attacks, "kills", res.Kills)
	if err != nil {
		return res, err
	}
	st.set(n, "cooldown", nil)
	return res, sleep(ctx, cfg.cooldown)
}
