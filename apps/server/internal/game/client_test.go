package game

import (
	"context"
	"io"
	"log/slog"
	"net"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/auth"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/config/gsconfig"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/crypto"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/frame"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/gscodec"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/store"
)

func repoData(parts ...string) string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(append([]string{filepath.Dir(file), "..", "..", "data"}, parts...)...)
}

type testEnv struct {
	world *World
	store *store.Memory
	auth  *auth.Service
	addr  string
	stop  func()
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	cfg, err := gsconfig.Load(repoConfig("gameserver", "gameserver.toml"))
	if err != nil {
		t.Fatal(err)
	}
	mem, err := store.NewMemory()
	if err != nil {
		t.Fatal(err)
	}
	h, _ := auth.HashPassword("pass")
	if _, err := mem.CreateAccount(context.Background(), "tester", h, "1234567890123"); err != nil {
		t.Fatal(err)
	}
	a := auth.New(mem, auth.Config{})
	w, err := New(Options{DataDir: repoData("Data"), ConfigDir: repoConfig(), Config: cfg, Store: mem, Auth: a,
		Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
	if err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- w.Serve(ctx, ln) }()
	env := &testEnv{world: w, store: mem, auth: a, addr: ln.Addr().String()}
	// stop may run from the test body and again from t.Cleanup.
	env.stop = sync.OnceFunc(func() {
		cancel()
		if err := <-done; err != nil {
			t.Errorf("Serve: %v", err)
		}
	})
	t.Cleanup(env.stop)
	return env
}

// testClient speaks the original client's wire format.
type testClient struct {
	t     *testing.T
	nc    net.Conn
	codec *gscodec.Client
	queue []gscodec.Packet
	// lastEncrypted reports whether the last packet returned by next
	// arrived as C3/C4.
	lastEncrypted bool
}

func loadKey(t *testing.T, parts ...string) crypto.ModulusKeys {
	t.Helper()
	k, err := crypto.LoadKeyFile(filepath.Join(parts...))
	if err != nil {
		t.Fatal(err)
	}
	return k
}

func (e *testEnv) dial(t *testing.T) *testClient {
	t.Helper()
	nc, err := net.Dial("tcp", e.addr)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { nc.Close() })
	_, file, _, _ := runtime.Caller(0)
	td := filepath.Join(filepath.Dir(file), "..", "protocol", "crypto", "testdata")
	stream, _ := crypto.DeriveStreamKeys("WizardTeam", "A9kT3wX5rBz7fQ1u")
	return &testClient{t: t, nc: nc, codec: gscodec.NewClient(gscodec.Keys{Stream: stream,
		Encrypt: loadKey(t, td, "Enc1.dat"), Decrypt: loadKey(t, td, "Dec2.dat")})}
}

func (c *testClient) send(p []byte) {
	c.t.Helper()
	enc, err := c.codec.Encode(p)
	if err != nil {
		c.t.Fatal(err)
	}
	if _, err := c.nc.Write(enc); err != nil {
		c.t.Fatal(err)
	}
}

// next returns the next packet whose head (and sub when >= 0) matches,
// skipping others.
func (c *testClient) next(head byte, sub int) []byte {
	c.t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	buf := make([]byte, 0x4000)
	for {
		for len(c.queue) > 0 {
			p := c.queue[0].Data
			enc := c.queue[0].Encrypted
			c.queue = c.queue[1:]
			if frame.Head(p) != head {
				continue
			}
			if s, ok := frame.Sub(p); sub >= 0 && (!ok || int(s) != sub) {
				continue
			}
			c.lastEncrypted = enc
			return p
		}
		c.nc.SetReadDeadline(deadline)
		n, err := c.nc.Read(buf)
		if err != nil {
			c.t.Fatalf("waiting for %02X %02X: %v", head, sub, err)
		}
		pkts, err := c.codec.Feed(buf[:n])
		if err != nil {
			c.t.Fatal(err)
		}
		c.queue = append(c.queue, pkts...)
	}
}

func bux(s string, n int) []byte {
	b := make([]byte, n)
	copy(b, s)
	crypto.Bux(b)
	return b
}

func (c *testClient) verify() {
	hw := "ABCDEFGH-ABCDEFGH-ABCDEFGH-ABCDEFGH-ABCDEFGH"
	body := append([]byte{0x09}, bux(hw, 45)...)
	body = append(body, 1) // [0x31]: client language 1 (Por)
	c.send(frame.Build(frame.C1, 0xF3, body...))
}

func (c *testClient) login(account, password string) byte {
	body := []byte{0x01}
	body = append(body, bux(account, 10)...)
	body = append(body, bux(password, 10)...)
	body = append(body, 0, 0, 0, 0)
	body = append(body, '1', '0', '0', '0', '0')
	body = append(body, []byte("A9kT3wX5rBz7fQ1u")...)
	c.send(frame.Build(frame.C3, 0xF1, body...))
	return c.next(0xF1, 0x01)[4]
}

func TestLoginCreateEnterWorld(t *testing.T) {
	env := newTestEnv(t)
	c := env.dial(t)

	join := c.next(0xF1, 0x00)
	if len(join) != 14 || join[4] != 1 || int(join[5])<<8|int(join[6]) != PlayerStart ||
		string(join[7:12]) != "10000" || join[12] != 0 {
		t.Fatalf("F1 00 = % X", join)
	}
	c.verify()
	if r := c.login("tester", "bad"); r != auth.ResultWrongPassword {
		t.Fatalf("bad password result %d", r)
	}
	if r := c.login("tester", "pass"); r != auth.ResultOK {
		t.Fatalf("login result %d", r)
	}

	c.send(frame.Build(frame.C1, 0xF3, 0x00))
	list := c.next(0xF3, 0x00)
	if !c.lastEncrypted || list[4] != 4 || list[6] != 0 {
		t.Fatalf("empty list = % X", list)
	}
	info := c.next(0xDE, 0x00)
	if info[4] != 4 || int(info[12])|int(info[13])<<8 != 220 {
		t.Fatalf("DE 00 = % X", info)
	}

	name := make([]byte, 10)
	copy(name, "Hero")
	c.send(frame.Build(frame.C1, 0xF3, append(append([]byte{0x01}, name...), 0x10)...))
	cr := c.next(0xF3, 0x01)
	if cr[4] != 1 || string(cr[5:9]) != "Hero" || cr[15] != 0 || cr[16] != 1 || cr[18] != 0x20 || cr[19] != 0xFF {
		t.Fatalf("create = % X", cr)
	}
	// Invalid symbol in the name.
	bad := make([]byte, 10)
	copy(bad, "Bad_Name")
	c.send(frame.Build(frame.C1, 0xF3, append(append([]byte{0x01}, bad...), 0x10)...))
	if cr := c.next(0xF3, 0x01); cr[4] != 0 {
		t.Fatalf("invalid name result %d", cr[4])
	}

	c.send(frame.Build(frame.C1, 0xF3, 0x00))
	list = c.next(0xF3, 0x00)
	if list[6] != 1 || string(list[8:12]) != "Hero" || list[7+15] != 0x20 || list[7+33] != 0xFF {
		t.Fatalf("list = % X", list)
	}
	eq := c.next(0xF3, 0xF9)
	if len(eq) != 0x32 || eq[4] != 0 || eq[5] != 0x00 || eq[6] != 0x00 {
		t.Fatalf("F3 F9 = % X", eq)
	}

	c.send(frame.Build(frame.C1, 0xF3, append([]byte{0x03}, name...)...))
	ci := c.next(0xF3, 0x03)
	if !c.lastEncrypted || len(ci) != 0xB0 || ci[4] != 182 || ci[5] != 128 || ci[6] != 0 ||
		ci[18] != 28 || ci[0x9c] != 28 {
		t.Fatalf("F3 03 = % X", ci)
	}
	inv := c.next(0xF3, 0x10)
	if !c.lastEncrypted || inv[5] != 3 || inv[6] != 0 || inv[14] != 12 {
		t.Fatalf("F3 10 = % X", inv)
	}
	sk := c.next(0xF3, 0x11)
	if sk[4] == 0 {
		t.Fatalf("F3 11 = % X", sk)
	}
	self := c.next(0x12, -1)
	if self[4] != 1 || int(self[5]&0x7F)<<8|int(self[6]) != PlayerStart || self[5]&0x80 == 0 ||
		string(self[5+28:5+32]) != "Hero" {
		t.Fatalf("self C2 12 = % X", self)
	}
}

func TestSecondPlayerSeesFirst(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	h, _ := auth.HashPassword("pass")
	acc2, _ := env.store.CreateAccount(ctx, "second", h, "")
	acc1, _ := env.store.AccountByLogin(ctx, "tester")
	env.store.CreateCharacter(ctx, acc1.ID, "Alpha", 0x00)
	env.store.CreateCharacter(ctx, acc2.ID, "Beta", 0x20)

	enter := func(acc, char string) *testClient {
		c := env.dial(t)
		c.next(0xF1, 0x00)
		c.verify()
		if r := c.login(acc, "pass"); r != 1 {
			t.Fatalf("login %s = %d", acc, r)
		}
		n := make([]byte, 10)
		copy(n, char)
		c.send(frame.Build(frame.C1, 0xF3, append([]byte{0x03}, n...)...))
		c.next(0xF3, 0x03)
		return c
	}
	a := enter("tester", "Alpha")
	// Move Beta next to Alpha: both spawn in Lorencia (182,128), Elf in Noria.
	b := enter("second", "Beta")
	_ = b
	// Beta (elf) starts in Noria, so Alpha must not see her.
	a.nc.SetReadDeadline(time.Now().Add(1500 * time.Millisecond))
	for {
		var buf [4096]byte
		n, err := a.nc.Read(buf[:])
		if err != nil {
			break
		}
		pkts, _ := a.codec.Feed(buf[:n])
		for _, p := range pkts {
			if frame.Head(p.Data) == 0x12 && p.Data[4] == 1 &&
				int(p.Data[5]&0x7F)<<8|int(p.Data[6]) != PlayerStart {
				t.Fatalf("Alpha sees a player on another map")
			}
		}
	}
}
