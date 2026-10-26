package main

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRunServesConnectAndGame(t *testing.T) {
	// The original BlackList.txt blocks 127.0.0.1, so use an empty one.
	empty := filepath.Join(t.TempDir(), "BlackList.json")
	if err := os.WriteFile(empty, []byte(`{"rows": []}`), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	ready := make(chan Ready, 1)
	done := make(chan error, 1)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	go func() {
		done <- run(ctx, []string{"-config", "../../config", "-data", "../../data", "-connect-addr", "127.0.0.1:0",
			"-bind", "127.0.0.1", "-blacklist", empty, "-instances", "4.GameServer"}, log, ready)
	}()

	var rd Ready
	select {
	case rd = <-ready:
	case err := <-done:
		t.Fatalf("run: %v", err)
	case <-time.After(10 * time.Second):
		t.Fatal("not ready")
	}
	c, err := net.Dial("tcp", rd.Connect)
	if err != nil {
		t.Fatal(err)
	}
	c.SetReadDeadline(time.Now().Add(2 * time.Second))
	hello := make([]byte, 4)
	if _, err := io.ReadFull(c, hello); err != nil || !bytes.Equal(hello, []byte{0xC1, 0x04, 0x00, 0x01}) {
		t.Fatalf("hello = % X, %v", hello, err)
	}
	c.Close()
	if rd.Games[0] == "" {
		t.Fatalf("game server 0 not listening: %+v", rd)
	}
	g, err := net.Dial("tcp", rd.Games[0])
	if err != nil {
		t.Fatal(err)
	}
	g.SetReadDeadline(time.Now().Add(2 * time.Second))
	join := make([]byte, 14)
	if _, err := io.ReadFull(g, join); err != nil {
		t.Fatalf("join: %v", err)
	}
	g.Close()

	cancel()
	if err := <-done; err != nil && err != context.Canceled {
		t.Fatalf("run: %v", err)
	}
}

func TestAccountCommand(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := run(context.Background(), []string{"-config", "../../config", "account", "create", "x", "y"}, log, nil); err != nil {
		t.Fatal(err)
	}
	if err := run(context.Background(), []string{"-config", "../../config", "bogus"}, log, nil); err == nil {
		t.Fatal("expected error")
	}
}

func TestRunFailsWithoutData(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := run(context.Background(), []string{"-config", t.TempDir()}, log, nil); err == nil {
		t.Fatal("expected error without config")
	}
	if err := run(context.Background(), []string{"-config", "../../config", "-data", t.TempDir(),
		"-connect-addr", "127.0.0.1:0", "-bind", "127.0.0.1"}, log, nil); err == nil {
		t.Fatal("expected error without the original data files")
	}
	if err := run(context.Background(), []string{"-config", "../../config", "-blacklist", "missing.json"}, log, nil); err == nil {
		t.Fatal("expected error for a missing blacklist")
	}
}
