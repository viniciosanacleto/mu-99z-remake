// Command muserver is the single Go service that replaces the original
// ConnectServer, JoinServer, DataServer and GameServer processes.
//
//	muserver [flags]                          run every module
//	muserver account create <login> <pass>    create an account
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"math"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"golang.org/x/sync/errgroup"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/auth"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/config/gsconfig"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/config/servercfg"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/config/serverlist"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/connect"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/store"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: logLevel()}))
	if err := run(ctx, os.Args[1:], log, nil); err != nil && !errors.Is(err, context.Canceled) {
		log.Error("muserver stopped", "err", err)
		os.Exit(1)
	}
}

func logLevel() slog.Level {
	if os.Getenv("MU_DEBUG") != "" {
		return slog.LevelDebug
	}
	return slog.LevelInfo
}

// Ready reports the listening addresses once everything is up.
type Ready struct {
	Connect string
	Games   map[int]string // server code -> address
	Metrics string         // -metrics-addr listener, empty when off
}

type config struct {
	configDir   string
	dataDir     string
	databaseURL string
	connectAddr string
	blackList   string
	bindHost    string
	publicIP    string
	instances   string
	// Load test (docs/server-plan/18-teste-de-carga.md).
	metricsAddr  string
	maxConnPerIP int
	maxUsers     int
	seedLoad     int
}

func parseFlags(args []string) (*config, []string, error) {
	fs := flag.NewFlagSet("muserver", flag.ContinueOnError)
	c := &config{}
	fs.StringVar(&c.configDir, "config", "config", "TOML configuration directory (server.toml, gameserver/, data/)")
	fs.StringVar(&c.dataDir, "data", "data", "directory with the binary data files (Data/Terrain, Data/Script, ...)")
	fs.StringVar(&c.databaseURL, "db", os.Getenv("MU_DATABASE_URL"), "PostgreSQL URL; empty uses the in-memory store")
	fs.StringVar(&c.connectAddr, "connect-addr", "", "ConnectServer listen address (default :ConnectServerPortTCP)")
	fs.StringVar(&c.blackList, "blacklist", "", "comma-separated client addresses to refuse instead of BlackList in server.toml (\"none\" for an empty list)")
	fs.StringVar(&c.bindHost, "bind", "", "host for every listener (default all interfaces)")
	fs.StringVar(&c.publicIP, "public-ip", "", "address sent to clients instead of the ServerList / MapServerInfo addresses")
	fs.StringVar(&c.instances, "instances", "gameserver,gameserver-cs", "game server instances (files under <config>/gameserver, without .toml)")
	fs.StringVar(&c.metricsAddr, "metrics-addr", "", "address for the pprof and /debug/load HTTP endpoint (empty = off)")
	fs.IntVar(&c.maxConnPerIP, "max-conn-per-ip", -1, "MaxConnectionPerIP for the ConnectServer, JoinServer and every GameServer (0 = unlimited, -1 = configured)")
	fs.IntVar(&c.maxUsers, "max-users", 0, "ServerMaxUserNumber of every GameServer (0 = configured, at most 1000)")
	fs.IntVar(&c.seedLoad, "seed-load", 0, "create N load test accounts (load0001/load) with one character each before serving")
	if err := fs.Parse(args); err != nil {
		return nil, nil, err
	}
	return c, fs.Args(), nil
}

// run parses flags, wires the modules and serves until ctx is done.
func run(ctx context.Context, args []string, log *slog.Logger, ready chan<- Ready) error {
	cfg, rest, err := parseFlags(args)
	if err != nil {
		return err
	}
	st, err := openStore(ctx, cfg.databaseURL)
	if err != nil {
		return err
	}
	defer st.Close()
	if len(rest) > 0 {
		return command(ctx, st, rest)
	}
	if cfg.databaseURL == "" {
		log.Warn("no -db given: using the in-memory store (data is lost on exit)")
	}

	srv, err := servercfg.Load(filepath.Join(cfg.configDir, "server.toml"))
	if err != nil {
		return err
	}
	if cfg.maxConnPerIP >= 0 {
		srv.JoinServer.MaxConnectionPerIP = cfg.maxConnPerIP
	}
	if cfg.seedLoad > 0 {
		if err := seedLoad(ctx, st, filepath.Join(cfg.dataDir, "Data"), cfg.seedLoad); err != nil {
			return fmt.Errorf("seed-load: %w", err)
		}
		log.Info("load test accounts created", "count", cfg.seedLoad)
	}
	entries, err := serverlist.Build(srv.ConnectServer.ServerList)
	if err != nil {
		return fmt.Errorf("server list: %w", err)
	}
	if cfg.publicIP != "" {
		for i := range entries {
			entries[i].Address = cfg.publicIP
		}
	}
	reg := connect.NewRegistry(entries)
	reg.SetAuthStatus(true, 0)
	m := &modules{
		cfg:   cfg,
		srv:   srv,
		store: st,
		auth: auth.New(st, auth.Config{
			MaxConnectionPerIP:  srv.JoinServer.MaxConnectionPerIP,
			MaxConnectionPerHID: srv.JoinServer.MaxConnectionPerHID,
			Log:                 log.With("module", "auth"),
		}),
		reg:     reg,
		servers: len(entries),
		users:   newUserCounter(reg),
		hub:     game.NewHub(),
		log:     log,
	}

	// Cancelled on a start-up failure too, so the modules already serving
	// stop before the deferred st.Close.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	g, gctx := errgroup.WithContext(ctx)
	rd, err := m.start(gctx, g)
	if err != nil {
		cancel()
		return errors.Join(err, g.Wait())
	}
	if ready != nil {
		ready <- rd
	}
	return g.Wait()
}

// modules is what the listeners of one muserver process share.
type modules struct {
	cfg   *config
	srv   *servercfg.File
	store store.Store
	auth  *auth.Service
	reg   *connect.Registry
	// servers is the number of ServerList entries in reg.
	servers int
	users   *userCounter
	hub     *game.Hub
	log     *slog.Logger
}

// start opens every listener and serves each one in g. On error, the
// modules already started are left to the caller to stop through ctx.
func (m *modules) start(ctx context.Context, g *errgroup.Group) (Ready, error) {
	rd := Ready{Games: map[int]string{}}
	if m.cfg.metricsAddr != "" {
		ln, err := listen(m.cfg.metricsAddr)
		if err != nil {
			return rd, err
		}
		rd.Metrics = ln.Addr().String()
		m.log.Info("metrics listening", "addr", rd.Metrics)
		g.Go(func() error { return serveMetrics(ctx, ln, m.users) })
	}
	for _, inst := range strings.Split(m.cfg.instances, ",") {
		inst = strings.TrimSpace(inst)
		if inst == "" {
			continue
		}
		code, addr, err := m.startGameServer(ctx, g, inst)
		if err != nil {
			return rd, fmt.Errorf("game server %s: %w", inst, err)
		}
		rd.Games[code] = addr
	}
	addr, err := m.startConnectServer(ctx, g)
	if err != nil {
		return rd, fmt.Errorf("connect server: %w", err)
	}
	rd.Connect = addr
	return rd, nil
}

// startGameServer builds the game instance configured in
// <config>/gameserver/<inst>.toml and serves it in g.
func (m *modules) startGameServer(ctx context.Context, g *errgroup.Group, inst string) (code int, addr string, err error) {
	gcfg, err := gsconfig.Load(filepath.Join(m.cfg.configDir, "gameserver", inst+".toml"))
	if err != nil {
		return 0, "", err
	}
	if m.cfg.maxConnPerIP >= 0 {
		gcfg.MaxConnectionPerIP = m.cfg.maxConnPerIP
	}
	if m.cfg.maxUsers > 0 {
		gcfg.MaxUsers = min(m.cfg.maxUsers, 1000)
	}
	w, err := game.New(game.Options{
		DataDir:   filepath.Join(m.cfg.dataDir, "Data"),
		ConfigDir: m.cfg.configDir,
		Config:    gcfg,
		Store:     m.store,
		Auth:      m.auth,
		Reporter:  m.users,
		Log:       m.log.With("module", "game"),
		PublicIP:  m.cfg.publicIP,
		Hub:       m.hub,
		BadSyntax: m.srv.DataServer.BadSyntax,
	})
	if err != nil {
		return 0, "", err
	}
	ln, err := listen(net.JoinHostPort(m.cfg.bindHost, strconv.Itoa(gcfg.ServerPort)))
	if err != nil {
		return 0, "", err
	}
	addr = ln.Addr().String()
	m.log.Info("game server listening", "name", gcfg.ServerName, "code", gcfg.ServerCode, "addr", addr)
	g.Go(func() error { return w.Serve(ctx, ln) })
	return gcfg.ServerCode, addr, nil
}

// startConnectServer serves the ConnectServer in g.
func (m *modules) startConnectServer(ctx context.Context, g *errgroup.Group) (string, error) {
	ccfg := connect.ConfigFrom(m.srv.ConnectServer)
	ccfg.BlackList = blackList(m.cfg, ccfg.BlackList)
	if m.cfg.maxConnPerIP >= 0 {
		ccfg.MaxPerIP = m.cfg.maxConnPerIP
	}
	addr := m.cfg.connectAddr
	if addr == "" {
		addr = net.JoinHostPort(m.cfg.bindHost, strconv.Itoa(ccfg.Port))
	}
	ln, err := listen(addr)
	if err != nil {
		return "", err
	}
	addr = ln.Addr().String()
	m.log.Info("connect server listening", "addr", addr, "servers", m.servers)
	srv := connect.NewServer(ccfg, m.reg, m.log.With("module", "connect"))
	g.Go(func() error { return srv.Serve(ctx, ln) })
	return addr, nil
}

// listen opens a TCP listener on addr.
func listen(addr string) (net.Listener, error) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("listening on %s: %w", addr, err)
	}
	return ln, nil
}

// blackList applies the -blacklist flag to the configured list.
func blackList(cfg *config, configured []string) []string {
	switch cfg.blackList {
	case "":
		return configured
	case "none":
		return nil
	}
	var out []string
	for _, ip := range strings.Split(cfg.blackList, ",") {
		if ip = strings.TrimSpace(ip); ip != "" {
			out = append(out, ip)
		}
	}
	return out
}

func openStore(ctx context.Context, url string) (store.Store, error) {
	if url == "" {
		return store.NewMemory()
	}
	return store.OpenPostgres(ctx, url)
}

// command runs the administrative subcommands.
func command(ctx context.Context, st store.Store, args []string) error {
	switch {
	case len(args) == 4 && args[0] == "account" && args[1] == "create":
		h, err := auth.HashPassword(args[3])
		if err != nil {
			return err
		}
		a, err := st.CreateAccount(ctx, args[2], h, "")
		if err != nil {
			return err
		}
		fmt.Printf("account %q created (id %d)\n", a.Login, a.ID)
		return nil
	case len(args) == 4 && args[0] == "character" && args[1] == "gm":
		lv, err := strconv.Atoi(args[3])
		if err != nil || lv < 0 || lv > math.MaxInt16 {
			return fmt.Errorf("invalid GM level %q", args[3])
		}
		gs, ok := st.(store.GMStore)
		if !ok {
			return errors.New("the store cannot set GM levels")
		}
		found, err := gs.SetGMLevel(ctx, args[2], lv)
		if err != nil {
			return err
		}
		if !found {
			return fmt.Errorf("character %q not found", args[2])
		}
		fmt.Printf("character %q GM level %d (applies on the next login)\n", args[2], lv)
		return nil
	}
	return fmt.Errorf("unknown command %q (use: account create <login> <password> | character gm <name> <level>)", strings.Join(args, " "))
}
