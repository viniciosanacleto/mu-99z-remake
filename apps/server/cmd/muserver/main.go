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
	"net"
	"os"
	"os/signal"
	"path/filepath"
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
	ccfg := connect.ConfigFrom(srv.ConnectServer)
	ccfg.BlackList = blackList(cfg, ccfg.BlackList)
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

	authSvc := auth.New(st, auth.Config{
		MaxConnectionPerIP:  srv.JoinServer.MaxConnectionPerIP,
		MaxConnectionPerHID: srv.JoinServer.MaxConnectionPerHID,
	})

	hub := game.NewHub()
	g, gctx := errgroup.WithContext(ctx)
	rd := Ready{Games: map[int]string{}}
	for _, inst := range strings.Split(cfg.instances, ",") {
		inst = strings.TrimSpace(inst)
		if inst == "" {
			continue
		}
		gcfg, err := gsconfig.Load(filepath.Join(cfg.configDir, "gameserver", inst+".toml"))
		if err != nil {
			return err
		}
		w, err := game.New(game.Options{DataDir: filepath.Join(cfg.dataDir, "Data"), ConfigDir: cfg.configDir, Config: gcfg,
			Store: st, Auth: authSvc, Reporter: reg, Log: log.With("module", "game"), PublicIP: cfg.publicIP, Hub: hub,
			BadSyntax: srv.DataServer.BadSyntax})
		if err != nil {
			return fmt.Errorf("%s: %w", inst, err)
		}
		ln, err := net.Listen("tcp", net.JoinHostPort(cfg.bindHost, fmt.Sprint(gcfg.ServerPort)))
		if err != nil {
			return err
		}
		rd.Games[gcfg.ServerCode] = ln.Addr().String()
		log.Info("game server listening", "name", gcfg.ServerName, "code", gcfg.ServerCode, "addr", ln.Addr().String())
		g.Go(func() error { return w.Serve(gctx, ln) })
	}

	addr := cfg.connectAddr
	if addr == "" {
		addr = net.JoinHostPort(cfg.bindHost, fmt.Sprint(ccfg.Port))
	}
	cln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	rd.Connect = cln.Addr().String()
	log.Info("connect server listening", "addr", rd.Connect, "servers", len(entries))
	g.Go(func() error { return connect.NewServer(ccfg, reg, log.With("module", "connect")).Serve(gctx, cln) })
	if ready != nil {
		ready <- rd
	}
	return g.Wait()
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
	}
	return fmt.Errorf("unknown command %q (use: account create <login> <password>)", strings.Join(args, " "))
}
