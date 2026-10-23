package connect

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/config/serverlist"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/frame"
)

var entries = []serverlist.Entry{
	{Code: 0, Name: "Servidor 01", Address: "192.168.1.1", Port: 55900, Visible: true},
	{Code: 19, Name: "EventServer", Address: "192.168.1.1", Port: 55919, Visible: false},
	{Code: 1, Name: "Down", Address: "10.0.0.1", Port: 55901, Visible: true},
}

func liveRegistry() *Registry {
	r := NewRegistry(entries)
	r.SetAuthStatus(true, 0)
	r.Report(0, 25, 100)
	r.Report(19, 0, 0)
	return r
}

func TestHello(t *testing.T) {
	if !bytes.Equal(Hello(), []byte{0xC1, 0x04, 0x00, 0x01}) {
		t.Fatalf("Hello = % X", Hello())
	}
}

func TestServerList(t *testing.T) {
	got := liveRegistry().ServerList()
	// Only code 0 is visible and alive: load 25*100/100 = 25.
	want := []byte{0xC2, 0x00, 0x0B, 0xF4, 0x06, 0x00, 0x01, 0x00, 0x00, 25, 0xCC}
	if !bytes.Equal(got, want) {
		t.Fatalf("ServerList = % X, want % X", got, want)
	}
}

func TestServerListCodeIsLittleEndian(t *testing.T) {
	r := NewRegistry([]serverlist.Entry{{Code: 0x12, Name: "x", Address: "a", Port: 1, Visible: true}})
	r.SetAuthStatus(true, 0)
	r.Report(0x12, 1, 3)
	got := r.ServerList()
	if got[7] != 0x12 || got[8] != 0x00 || got[9] != 33 {
		t.Fatalf("entry = % X", got[7:])
	}
}

func TestServerNames(t *testing.T) {
	got := liveRegistry().ServerNames()
	// Alive servers 0 and 19 (visibility is not checked): 6 + 2*34 bytes.
	if len(got) != 74 || got[0] != 0xC2 || got[1] != 0 || got[2] != 74 ||
		got[3] != 0xF3 || got[4] != 0xEA || got[5] != 2 {
		t.Fatalf("header = % X (len %d)", got[:6], len(got))
	}
	if got[6] != 0 || got[7] != 0 || string(got[8:19]) != "Servidor 01" || got[19] != 0 {
		t.Fatalf("entry 0 = % X", got[6:40])
	}
	if got[40] != 19 || got[41] != 0 || string(got[42:53]) != "EventServer" {
		t.Fatalf("entry 1 = % X", got[40:])
	}
}

func TestServerInfo(t *testing.T) {
	r := liveRegistry()
	got, ok := r.ServerInfo(0)
	want := append([]byte{0xC1, 0x16, 0xF4, 0x03}, []byte("192.168.1.1\x00\x00\x00\x00\x00")...)
	want = append(want, 0x5C, 0xDA) // 55900 = 0xDA5C, little-endian
	if !ok || !bytes.Equal(got, want) {
		t.Fatalf("ServerInfo = % X, want % X", got, want)
	}
	for _, code := range []byte{19 /* invisible */, 1 /* down */, 7 /* unknown */} {
		if _, ok := r.ServerInfo(code); ok {
			t.Errorf("code %d: expected no answer", code)
		}
	}
}

func TestAuthGate(t *testing.T) {
	r := liveRegistry()
	r.SetAuthStatus(true, 101)
	if got := r.ServerList(); got[5] != 0 || got[6] != 0 || len(got) != 7 {
		t.Fatalf("ServerList with full queue = % X", got)
	}
	if got := r.ServerNames(); got[5] != 0 || len(got) != 6 {
		t.Fatalf("ServerNames with full queue = % X", got)
	}
	r.SetAuthStatus(false, 0)
	if _, ok := r.ServerInfo(0); ok {
		t.Fatal("ServerInfo with auth down")
	}
	r.SetAuthStatus(true, 100)
	if _, ok := r.ServerInfo(0); !ok {
		t.Fatal("queue 100 is still below the limit")
	}
	r.MarkDown(0)
	if _, ok := r.ServerInfo(0); ok {
		t.Fatal("ServerInfo after MarkDown")
	}
}

func TestLoadDivisionByZero(t *testing.T) {
	if (ServerStatus{Users: 5}).Load() != 0 {
		t.Fatal("Load with MaxUsers 0")
	}
}

func TestHandle(t *testing.T) {
	r := liveRegistry()
	if got := r.Handle(frame.Build(frame.C1, 0xF4, 0x06)); got == nil || got[4] != 0x06 {
		t.Fatalf("F4 06 -> % X", got)
	}
	if got := r.Handle(frame.Build(frame.C1, 0xF4, 0x03, 0x00)); got == nil || got[3] != 0x03 {
		t.Fatalf("F4 03 -> % X", got)
	}
	for _, p := range [][]byte{
		frame.Build(frame.C1, 0xF4, 0x03),       // missing code
		frame.Build(frame.C1, 0xF4, 0x07),       // unknown sub
		frame.Build(frame.C1, 0xF3, 0x06),       // other head
		frame.Build(frame.C2, 0xF4, 0x06, 0x00), // sub read at [3] is the head
	} {
		if got := r.Handle(p); got != nil {
			t.Errorf("% X -> % X, want nothing", p, got)
		}
	}
}

func TestLoadConfig(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	dir := filepath.Join(filepath.Dir(file), "..", "..", "data", "1.ConnectServer")
	cfg, err := LoadConfig(filepath.Join(dir, "ConnectServer.ini"), filepath.Join(dir, "BlackList.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Port != 44405 || cfg.MaxIdle != 60*time.Second || cfg.MaxPerIP != 1 ||
		cfg.MaxPacketPerSecond != 10 || len(cfg.BlackList) != 1 || cfg.BlackList[0] != "127.0.0.1" {
		t.Fatalf("cfg = %+v", cfg)
	}
}

func startServer(t *testing.T, cfg Config) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	srv := NewServer(cfg, liveRegistry(), slog.New(slog.NewTextHandler(io.Discard, nil)))
	done := make(chan error, 1)
	go func() { done <- srv.Serve(ctx, ln) }()
	t.Cleanup(func() {
		cancel()
		if err := <-done; err != nil {
			t.Errorf("Serve: %v", err)
		}
	})
	return ln.Addr().String()
}

func readPacket(t *testing.T, c net.Conn) []byte {
	t.Helper()
	c.SetReadDeadline(time.Now().Add(2 * time.Second))
	var buf []byte
	tmp := make([]byte, 512)
	for {
		if n, _ := frame.Next(buf, frame.MaxSizeConnect); n > 0 {
			return buf[:n]
		}
		n, err := c.Read(tmp)
		if err != nil {
			t.Fatalf("read: %v (have % X)", err, buf)
		}
		buf = append(buf, tmp[:n]...)
	}
}

func TestServerSession(t *testing.T) {
	addr := startServer(t, Config{MaxIdle: time.Minute, MaxPerIP: 1, MaxPacketPerSecond: 10})
	c, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	// hello and F3 EA arrive in one or more TCP reads; parse them in order.
	c.SetReadDeadline(time.Now().Add(2 * time.Second))
	var stream []byte
	tmp := make([]byte, 512)
	for len(stream) < 4+74 {
		n, err := c.Read(tmp)
		if err != nil {
			t.Fatal(err)
		}
		stream = append(stream, tmp[:n]...)
	}
	if !bytes.Equal(stream[:4], Hello()) || stream[7] != 0xF3 || stream[8] != 0xEA {
		t.Fatalf("greeting = % X", stream[:9])
	}

	c.Write(frame.Build(frame.C1, 0xF4, 0x06))
	if got := readPacket(t, c); got[3] != 0xF4 || got[4] != 0x06 || got[6] != 1 {
		t.Fatalf("list = % X", got)
	}
	c.Write(frame.Build(frame.C1, 0xF4, 0x03, 0x00))
	if got := readPacket(t, c); len(got) != 0x16 || got[3] != 0x03 {
		t.Fatalf("info = % X", got)
	}

	// MaxConnectionPerIP = 1: a second connection from 127.0.0.1 is closed.
	c2, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer c2.Close()
	c2.SetReadDeadline(time.Now().Add(2 * time.Second))
	if n, err := c2.Read(tmp); err == nil {
		t.Fatalf("second connection got % X", tmp[:n])
	}
}

func TestServerDisconnects(t *testing.T) {
	cases := []struct {
		name string
		cfg  Config
		send []byte
	}{
		{"bad header", Config{}, []byte{0x00, 0x01, 0x02}},
		{"flood", Config{MaxPacketPerSecond: 3}, bytes.Repeat(frame.Build(frame.C1, 0xF4, 0x07), 3)},
		{"idle", Config{MaxIdle: 100 * time.Millisecond}, nil},
		{"blacklisted", Config{BlackList: []string{"127.0.0.1"}}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, err := net.Dial("tcp", startServer(t, tc.cfg))
			if err != nil {
				t.Fatal(err)
			}
			defer c.Close()
			if tc.send != nil {
				c.Write(tc.send)
			}
			c.SetReadDeadline(time.Now().Add(3 * time.Second))
			tmp := make([]byte, 512)
			for {
				_, err := c.Read(tmp)
				if err == io.EOF {
					return // closed by the server
				}
				if ne, ok := err.(net.Error); ok && ne.Timeout() {
					t.Fatal("server did not close the connection")
				}
				if err != nil {
					return // reset also counts as closed
				}
			}
		})
	}
}
