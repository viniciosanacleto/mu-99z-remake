// Command muproxy is a TCP man-in-the-middle between the original Main.exe
// and a ConnectServer or GameServer (the original ones or muserver). It
// forwards every byte, logs each decoded packet and can save every session
// as a golden-vector file (internal/capture, docs/server-plan/15-captura.md).
//
//	muproxy -listen 127.0.0.1:44405 -upstream 192.168.1.10:44405 -rewrite -golden captures
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/capture"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:], os.Stdout, nil); err != nil && !errors.Is(err, context.Canceled) {
		fmt.Fprintln(os.Stderr, "muproxy:", err)
		os.Exit(1)
	}
}

type options struct {
	listen, upstream, kind     string
	dataDir, gsConfig, hackDir string
	clientKeys                 string
	logPath, golden            string
	rewrite                    bool
	rewriteHost                string
}

func parseFlags(args []string, out io.Writer) (options, error) {
	var o options
	fs := flag.NewFlagSet("muproxy", flag.ContinueOnError)
	fs.SetOutput(out)
	fs.StringVar(&o.listen, "listen", "127.0.0.1:44405", "address the client connects to")
	fs.StringVar(&o.upstream, "upstream", "", "address of the real server (required)")
	fs.StringVar(&o.kind, "kind", "connect", "protocol of -upstream: connect (ConnectServer) or game (GameServer)")
	fs.StringVar(&o.dataDir, "data", "data", "server data root (apps/server/data)")
	fs.StringVar(&o.gsConfig, "gs-config", filepath.Join("config", "gameservers", "4.GameServer.json"), "GameServer instance file (CustomerName / ServerSerial)")
	fs.StringVar(&o.hackDir, "server-keys", "", "folder with the server's Enc2.dat/Dec1.dat (default <data>/Data/Hack)")
	fs.StringVar(&o.clientKeys, "client-keys", filepath.Join("..", "client", "assets", "Data"), "folder with the client's Enc1.dat/Dec2.dat")
	fs.StringVar(&o.logPath, "log", "-", "packet log file (- = stdout)")
	fs.StringVar(&o.golden, "golden", "", "folder for one golden-vector JSON file per session (empty = off)")
	fs.BoolVar(&o.rewrite, "rewrite", false, "replace the GameServer address in F4 03 and B1 00 with a proxy listener")
	fs.StringVar(&o.rewriteHost, "rewrite-host", "127.0.0.1", "host the rewrite listeners bind to and announce (max 15 chars)")
	if err := fs.Parse(args); err != nil {
		return o, err
	}
	if o.upstream == "" {
		return o, errors.New("-upstream is required")
	}
	if o.kind != string(capture.Connect) && o.kind != string(capture.Game) {
		return o, fmt.Errorf("-kind %q: want connect or game", o.kind)
	}
	if len(o.rewriteHost) > 15 {
		return o, fmt.Errorf("-rewrite-host %q does not fit the 16-byte address field", o.rewriteHost)
	}
	if o.hackDir == "" {
		o.hackDir = filepath.Join(o.dataDir, "Data", "Hack")
	}
	return o, nil
}

// run starts the proxy; ready, when not nil, receives the listen address.
func run(ctx context.Context, args []string, stdout io.Writer, ready chan<- string) error {
	o, err := parseFlags(args, stdout)
	if err != nil {
		return err
	}
	keys, err := capture.LoadKeys(o.gsConfig, o.hackDir, o.clientKeys)
	if err != nil {
		return err
	}
	logw := stdout
	if o.logPath != "-" {
		f, err := os.OpenFile(o.logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			return err
		}
		defer f.Close()
		logw = f
	}
	p := &capture.Proxy{Keys: keys, Log: logw, GoldenDir: o.golden, Rewrite: o.rewrite, RewriteHost: o.rewriteHost}
	ln, err := net.Listen("tcp", o.listen)
	if err != nil {
		return err
	}
	fmt.Fprintf(logw, "muproxy %s %s -> %s (rewrite %v)\n", o.kind, ln.Addr(), o.upstream, o.rewrite)
	if ready != nil {
		ready <- ln.Addr().String()
	}
	err = p.Serve(ctx, ln, capture.Kind(o.kind), o.upstream)
	p.Wait()
	if err == nil {
		err = ctx.Err()
	}
	return err
}
