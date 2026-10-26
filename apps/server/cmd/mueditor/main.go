// Command mueditor is the web character editor: it edits stats, zen and
// inventory / vault items of offline characters directly in PostgreSQL.
//
//	mueditor -db postgres://... [-config config] [-instance gameserver] [-addr 127.0.0.1:8080] [-password x]
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/config/gsconfig"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/editor"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/store"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if err := run(ctx, os.Args[1:], log, nil); err != nil && !errors.Is(err, context.Canceled) {
		log.Error("mueditor stopped", "err", err)
		os.Exit(1)
	}
}

type config struct {
	configDir   string
	databaseURL string
	addr        string
	password    string
	instance    string
}

func parseFlags(args []string) (*config, error) {
	fs := flag.NewFlagSet("mueditor", flag.ContinueOnError)
	c := &config{}
	fs.StringVar(&c.configDir, "config", "config", "TOML configuration directory")
	fs.StringVar(&c.databaseURL, "db", os.Getenv("MU_DATABASE_URL"), "PostgreSQL URL (required)")
	fs.StringVar(&c.addr, "addr", "127.0.0.1:8080", "HTTP listen address")
	fs.StringVar(&c.password, "password", os.Getenv("MU_EDITOR_PASSWORD"), "HTTP basic auth password (empty = no auth)")
	fs.StringVar(&c.instance, "instance", "gameserver", "game server instance (<config>/gameserver/<instance>.toml) whose ServerMaxLevel limits the level")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	if c.databaseURL == "" {
		return nil, errors.New("-db (or MU_DATABASE_URL) is required")
	}
	return c, nil
}

// newEditor loads the item data and wires the editor over st.
func newEditor(configDir, instance string, st editor.Store) (*editor.Editor, error) {
	items, err := gamedata.LoadItems(configDir)
	if err != nil {
		return nil, err
	}
	opts, err := gamedata.LoadItemOptions(configDir)
	if err != nil {
		return nil, err
	}
	gcfg, err := gsconfig.Load(filepath.Join(configDir, "gameserver", instance+".toml"))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", instance, err)
	}
	lim := editor.Limits{MaxLevel: gcfg.ServerMaxLevel, Exp: gamedata.NewExperience(
		int32(gcfg.Int("Common", "ExperienceMultiplierConstA", 0)),
		int32(gcfg.Int("Common", "ExperienceMultiplierConstB", 0)),
		int32(gcfg.Int("Common", "PetExperienceMultiplierConstA", 0)))}
	for al := range lim.MaxStat {
		lim.MaxStat[al] = gcfg.ALInt("Common", "MaxStatPoint", al, 0)
	}
	return editor.New(st, editor.NewCatalog(items, opts), lim), nil
}

// run serves the editor until ctx is done; ready receives the address.
func run(ctx context.Context, args []string, log *slog.Logger, ready chan<- string) error {
	cfg, err := parseFlags(args)
	if err != nil {
		return err
	}
	st, err := store.OpenPostgres(ctx, cfg.databaseURL)
	if err != nil {
		return err
	}
	defer st.Close()
	ed, err := newEditor(cfg.configDir, cfg.instance, st)
	if err != nil {
		return err
	}
	ln, err := net.Listen("tcp", cfg.addr)
	if err != nil {
		return err
	}
	srv := &http.Server{Handler: editor.Handler(ed, cfg.password, log), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(sctx)
	}()
	log.Info("editor listening", "url", "http://"+ln.Addr().String(), "items", len(ed.Catalog().Items),
		"auth", cfg.password != "")
	if ready != nil {
		ready <- ln.Addr().String()
	}
	if err := srv.Serve(ln); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return ctx.Err()
}
