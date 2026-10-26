package main

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net"
	"testing"
	"time"
)

func TestRunServesConnectAndGame(t *testing.T) {
	// The original BlackList blocks 127.0.0.1, so empty it.
	ctx, cancel := context.WithCancel(context.Background())
	ready := make(chan Ready, 1)
	done := make(chan error, 1)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	go func() {
		done <- run(ctx, []string{"-config", "../../config", "-data", "../../data", "-connect-addr", "127.0.0.1:0",
			"-bind", "127.0.0.1", "-blacklist", "none", "-instances", "gameserver"}, log, ready)
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
	if err := run(context.Background(), []string{"-config", "../../config", "-data", "../../data", "account", "create", "x", "y"}, log, nil); err != nil {
		t.Fatal(err)
	}
	if err := run(context.Background(), []string{"-config", "../../config", "-data", "../../data", "bogus"}, log, nil); err == nil {
		t.Fatal("expected error")
	}
}

func TestRunFailsWithoutConfig(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := run(context.Background(), []string{"-config", t.TempDir(), "-data", "../../data"}, log, nil); err == nil {
		t.Fatal("expected error")
	}
	if err := run(context.Background(), []string{"-config", "../../config", "-data", "../../data",
		"-instances", "missing"}, log, nil); err == nil {
		t.Fatal("expected error for a missing instance")
	}
}

func TestBlackListFlag(t *testing.T) {
	for in, want := range map[string]int{"": 1, "none": 0, "1.2.3.4,5.6.7.8": 2} {
		c, _, err := parseFlags([]string{"-blacklist", in})
		if err != nil {
			t.Fatal(err)
		}
		if got := len(blackList(c, []string{"127.0.0.1"})); got != want {
			t.Errorf("-blacklist %q: %d addresses, want %d", in, got, want)
		}
	}
}
