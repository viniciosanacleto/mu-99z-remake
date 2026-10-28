package main

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/editor"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/store"
)

// newEditor builds the web editor of the -editor-addr listener: limits of
// the first game server instance, and edits of playing characters through
// the hub.
func newEditor(cfg *config, st store.Store, hub *game.Hub) (*editor.Editor, error) {
	es, ok := st.(editor.Store)
	if !ok {
		return nil, errors.New("the store does not support the editor")
	}
	inst, _, _ := strings.Cut(cfg.instances, ",")
	ed, err := editor.Load(cfg.configDir, strings.TrimSpace(inst), es)
	if err != nil {
		return nil, err
	}
	return ed.WithLive(hub), nil
}

func serveEditor(ctx context.Context, ln net.Listener, ed *editor.Editor, password string, log *slog.Logger) error {
	srv := &http.Server{Handler: editor.Handler(ed, password, log), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(sctx)
	}()
	if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
