package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

func dataRoot() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..", "data")
}

func clientKeys() string {
	_, file, _, _ := runtime.Caller(0)
	// The client's Enc1.dat/Dec2.dat are kept with the crypto tests.
	return filepath.Join(filepath.Dir(file), "..", "..", "internal", "protocol", "crypto", "testdata")
}

func TestParseFlags(t *testing.T) {
	if _, err := parseFlags(nil, io.Discard); err == nil {
		t.Error("missing -upstream accepted")
	}
	if _, err := parseFlags([]string{"-upstream", "h:1", "-kind", "join"}, io.Discard); err == nil {
		t.Error("bad -kind accepted")
	}
	if _, err := parseFlags([]string{"-upstream", "h:1", "-rewrite-host", "very.long.host.name"}, io.Discard); err == nil {
		t.Error("long -rewrite-host accepted")
	}
	o, err := parseFlags([]string{"-upstream", "h:1", "-data", "d"}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if o.gsConfig != filepath.Join("config", "gameserver", "gameserver.toml") || o.hackDir != filepath.Join("d", "Data", "Hack") ||
		o.kind != "connect" || o.listen != "127.0.0.1:44405" {
		t.Fatalf("defaults = %+v", o)
	}
}

type lockedBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuf) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *lockedBuf) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

// TestRunConnect proxies a fake ConnectServer on localhost and checks the
// bytes and the packet log.
func TestRunConnect(t *testing.T) {
	up, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer up.Close()
	go func() {
		c, err := up.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		c.Write([]byte{0xC1, 0x04, 0x00, 0x01})
		io.Copy(io.Discard, c)
	}()
	ctx, cancel := context.WithCancel(context.Background())
	ready := make(chan string, 1)
	out := &lockedBuf{}
	golden := t.TempDir()
	done := make(chan error, 1)
	go func() {
		done <- run(ctx, []string{"-listen", "127.0.0.1:0", "-upstream", up.Addr().String(), "-data", dataRoot(),
			"-gs-config", filepath.Join(dataRoot(), "..", "config", "gameserver", "gameserver.toml"),
			"-client-keys", clientKeys(), "-golden", golden}, out, ready)
	}()
	var addr string
	select {
	case addr = <-ready:
	case err := <-done:
		t.Fatal(err)
	}
	c, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	hello := make([]byte, 4)
	c.SetReadDeadline(time.Now().Add(5 * time.Second))
	if _, err := io.ReadFull(c, hello); err != nil || !bytes.Equal(hello, []byte{0xC1, 0x04, 0x00, 0x01}) {
		t.Fatalf("hello %x %v", hello, err)
	}
	c.Close()
	deadline := time.Now().Add(5 * time.Second)
	for !strings.Contains(out.String(), "saved ") && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("run = %v", err)
	}
	log := out.String()
	if !strings.Contains(log, "#1 connect s2c C1 00 01 len=4 c1040001") {
		t.Fatalf("log:\n%s", log)
	}
	if files, _ := filepath.Glob(filepath.Join(golden, "*-connect.json")); len(files) != 1 {
		t.Fatalf("golden files %v", files)
	}
}
