package capture

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"net"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/frame"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/gscodec"
)

func repo(parts ...string) string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(append([]string{filepath.Dir(file), "..", ".."}, parts...)...)
}

func testKeys(t *testing.T) Keys {
	t.Helper()
	k, err := LoadKeys(repo("config", "gameservers", "4.GameServer.json"), repo("data", "Data", "Hack"),
		repo("..", "client", "assets", "Data"))
	if err != nil {
		t.Fatal(err)
	}
	return k
}

func serverCodec(k Keys) *gscodec.Server {
	return gscodec.NewServer(gscodec.Keys{Stream: k.Stream, Encrypt: k.ServerEnc, Decrypt: k.ServerDec})
}

func clientCodec(k Keys) *gscodec.Client {
	return gscodec.NewClient(gscodec.Keys{Stream: k.Stream, Encrypt: k.ClientEnc, Decrypt: k.ClientDec})
}

func listen(t *testing.T) net.Listener {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	return ln
}

// syncBuf is a goroutine-safe log sink.
type syncBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuf) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuf) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

// startProxy runs a proxy for kind/upstream and returns its address and a
// channel of finished sessions.
func startProxy(t *testing.T, p *Proxy, kind Kind, upstream string) (string, chan *Session) {
	t.Helper()
	sessions := make(chan *Session, 16)
	p.OnSession = func(s *Session) { sessions <- s }
	ln := listen(t)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		p.Serve(ctx, ln, kind, upstream)
	}()
	t.Cleanup(func() {
		cancel()
		<-done
		p.Wait()
	})
	return ln.Addr().String(), sessions
}

// fakeGame serves GameServer connections: every decoded client packet is
// passed to reply, whose packets are sent back through gscodec.Server.
func fakeGame(t *testing.T, k Keys, reply func(p gscodec.Packet) [][]byte) (net.Listener, chan gscodec.Packet) {
	t.Helper()
	ln := listen(t)
	got := make(chan gscodec.Packet, 64)
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				codec := serverCodec(k)
				buf := make([]byte, 4096)
				for {
					n, err := c.Read(buf)
					if err != nil {
						return
					}
					pkts, err := codec.Feed(buf[:n])
					if err != nil {
						t.Errorf("fake server: %v", err)
						return
					}
					for _, p := range pkts {
						got <- p
						for _, out := range reply(p) {
							w, err := codec.Encode(out)
							if err != nil {
								t.Errorf("fake server encode: %v", err)
								return
							}
							if _, err := c.Write(w); err != nil {
								return
							}
						}
					}
				}
			}()
		}
	}()
	return ln, got
}

// readPackets reads from c through the client codec until n packets arrived.
func readPackets(t *testing.T, c net.Conn, codec *gscodec.Client, n int) []gscodec.Packet {
	t.Helper()
	var out []gscodec.Packet
	buf := make([]byte, 4096)
	c.SetReadDeadline(time.Now().Add(5 * time.Second))
	for len(out) < n {
		m, err := c.Read(buf)
		if err != nil {
			t.Fatalf("read after %d packets: %v", len(out), err)
		}
		pkts, err := codec.Feed(buf[:m])
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, pkts...)
	}
	return out
}

func waitSession(t *testing.T, ch chan *Session, kind Kind) *Session {
	t.Helper()
	for {
		select {
		case s := <-ch:
			if s.Kind == kind {
				return s
			}
		case <-time.After(5 * time.Second):
			t.Fatal("no session")
		}
	}
}

func TestGameSessionDecodeAndReplay(t *testing.T) {
	k := testKeys(t)
	joinResult := frame.Build(frame.C1, 0xF1, 0x00, 0x01, 0x53, 0xFC, '1', '0', '4', '0', '4')
	charList := frame.Build(frame.C3, 0xF3, append([]byte{0x00}, bytes.Repeat([]byte{7}, 40)...)...)
	big := frame.Build(frame.C2, 0x13, make([]byte, 600)...)
	gs, got := fakeGame(t, k, func(p gscodec.Packet) [][]byte {
		if frame.Head(p.Data) == 0xF1 {
			return [][]byte{joinResult, charList, big}
		}
		return nil
	})
	logBuf := &syncBuf{}
	golden := t.TempDir()
	p := &Proxy{Keys: k, Log: logBuf, GoldenDir: golden}
	addr, sessions := startProxy(t, p, Game, gs.Addr().String())

	c, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	cli := clientCodec(k)
	login := frame.Build(frame.C3, 0xF1, append([]byte{0x01}, bytes.Repeat([]byte{'a'}, 45)...)...)
	walk := frame.Build(frame.C1, 0xD3, 0x80, 0x80, 0x24, 0x00)
	var wire []byte
	for _, pk := range [][]byte{login, walk} {
		w, err := cli.Encode(pk)
		if err != nil {
			t.Fatal(err)
		}
		wire = append(wire, w...)
	}
	// Byte-sized writes exercise the incremental framing of the proxy.
	for i := range wire {
		if _, err := c.Write(wire[i : i+1]); err != nil {
			t.Fatal(err)
		}
	}
	back := readPackets(t, c, cli, 3)
	if !bytes.Equal(back[0].Data, joinResult) || !back[1].Encrypted || back[1].Data[2] != 0xF3 ||
		!bytes.Equal(back[2].Data, big) {
		t.Fatalf("client received %+v", back)
	}
	for i := 0; i < 2; i++ {
		select {
		case pk := <-got:
			if want := [][]byte{{0xC1, login[1], 0xF1}, walk[:3]}[i]; !bytes.Equal(pk.Data[:3], want) {
				t.Errorf("server packet %d = %x", i, pk.Data)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("server got nothing")
		}
	}
	c.Close()

	s := waitSession(t, sessions, Game)
	if len(s.Records) != 5 {
		t.Fatalf("%d records", len(s.Records))
	}
	var c2s, s2c []Record
	for _, r := range s.Records {
		if r.Err != "" {
			t.Fatalf("record error %s", r.Err)
		}
		if r.Dir == ClientToServer {
			c2s = append(c2s, r)
		} else {
			s2c = append(s2c, r)
		}
	}
	// The client packets decode to the XOR32-decrypted plain form.
	if len(c2s) != 2 || !c2s[0].Encrypted || c2s[0].Serial != 0 || !bytes.Equal(c2s[0].Plain, append([]byte{0xC1, login[1]}, login[2:]...)) ||
		!bytes.Equal(c2s[1].Plain, walk) {
		t.Fatalf("c2s = %+v", c2s)
	}
	if len(s2c) != 3 || !bytes.Equal(s2c[0].Plain, joinResult) || !s2c[1].Encrypted || s2c[1].WireKind() != frame.C3 {
		t.Fatalf("s2c = %+v", s2c)
	}
	if err := Replay(s, k); err != nil {
		t.Fatal(err)
	}

	// The golden file holds the same session and replays too.
	files, _ := filepath.Glob(filepath.Join(golden, "*-game.json"))
	if len(files) != 1 {
		t.Fatalf("golden files %v", files)
	}
	loaded, err := LoadSession(files[0])
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Records) != 5 || loaded.StreamAdd != k.Stream.Add || loaded.StreamXor != k.Stream.Xor {
		t.Fatalf("loaded = %+v", loaded)
	}
	if err := Replay(loaded, k); err != nil {
		t.Fatal(err)
	}

	log := logBuf.String()
	for _, want := range []string{"#1 game c2s C3 F1 01 len=", "serial=00", "#1 game c2s C1 D3 80 len=",
		"#1 game s2c C1 F1 00 len=12 c10cf1000153fc3130343034", "#1 game s2c C2 13 00 len=604", "saved "} {
		if !strings.Contains(log, want) {
			t.Errorf("log lacks %q:\n%s", want, log)
		}
	}
}

func TestReplayDetectsMismatch(t *testing.T) {
	k := testKeys(t)
	srv := serverCodec(k)
	pkt := frame.Build(frame.C3, 0xF3, 0x03, 1, 2, 3)
	wire, err := srv.Encode(pkt)
	if err != nil {
		t.Fatal(err)
	}
	sp := newSplitter(Game, ServerToClient, k)
	recs, err := sp.feed(wire)
	if err != nil || len(recs) != 1 {
		t.Fatalf("%v %v", recs, err)
	}
	s := &Session{Version: FormatVersion, Kind: Game, StreamAdd: k.Stream.Add, StreamXor: k.Stream.Xor, Records: recs}
	if err := Replay(s, k); err != nil {
		t.Fatal(err)
	}
	s.Records[0].Plain[4] ^= 0xFF
	if err := Replay(s, k); err == nil {
		t.Fatal("tampered plain accepted")
	}
	s.Records[0].Plain[4] ^= 0xFF
	s.Records[0].Serial++
	if err := Replay(s, k); err == nil {
		t.Fatal("wrong serial accepted")
	}
}

func TestEncodeServerPacketRoundTrip(t *testing.T) {
	k := testKeys(t)
	srv := serverCodec(k)
	srv.SetSerial(0x7A)
	pkt := frame.Build(frame.C3, 0xB1, append([]byte{0x00}, make([]byte, 40)...)...)
	wire, err := srv.Encode(pkt)
	if err != nil {
		t.Fatal(err)
	}
	recs, err := newSplitter(Game, ServerToClient, k).feed(wire)
	if err != nil || len(recs) != 1 || recs[0].Serial != 0x7A {
		t.Fatalf("%+v %v", recs, err)
	}
	again, err := encodeServerPacket(k, recs[0].Plain, true, recs[0].Serial)
	if err != nil || !bytes.Equal(again, wire) {
		t.Fatalf("re-encoded %x, want %x (%v)", again, wire, err)
	}
}

// fakeConnect is a ConnectServer upstream: hello, then F4 03 answers with
// the address gsAddr for any request.
func fakeConnect(t *testing.T, gsAddr string) net.Listener {
	t.Helper()
	ln := listen(t)
	host, ps, _ := net.SplitHostPort(gsAddr)
	port, _ := strconv.Atoi(ps)
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				c.Write([]byte{0xC1, 0x04, 0x00, 0x01})
				buf := make([]byte, 64)
				for {
					n, err := c.Read(buf)
					if err != nil {
						return
					}
					if n >= 5 && buf[2] == 0xF4 && buf[3] == 0x03 {
						body := make([]byte, 19)
						body[0] = 0x03
						copy(body[1:16], host)
						binary.LittleEndian.PutUint16(body[17:], uint16(port))
						c.Write(frame.Build(frame.C1, 0xF4, body...))
					}
				}
			}()
		}
	}()
	return ln
}

func readServerInfo(t *testing.T, c net.Conn) (string, int) {
	t.Helper()
	c.SetReadDeadline(time.Now().Add(5 * time.Second))
	hello := make([]byte, 4)
	if _, err := io.ReadFull(c, hello); err != nil || !bytes.Equal(hello, []byte{0xC1, 0x04, 0x00, 0x01}) {
		t.Fatalf("hello %x %v", hello, err)
	}
	if _, err := c.Write([]byte{0xC1, 0x05, 0xF4, 0x03, 0x00}); err != nil {
		t.Fatal(err)
	}
	info := make([]byte, 0x16)
	if _, err := io.ReadFull(c, info); err != nil {
		t.Fatal(err)
	}
	if info[0] != 0xC1 || info[1] != 0x16 || info[2] != 0xF4 || info[3] != 0x03 {
		t.Fatalf("info %x", info)
	}
	return cstring(info[4:20]), int(binary.LittleEndian.Uint16(info[20:]))
}

func TestConnectServerRewrite(t *testing.T) {
	k := testKeys(t)
	gs, got := fakeGame(t, k, func(p gscodec.Packet) [][]byte {
		return [][]byte{frame.Build(frame.C1, 0xF1, 0x00, 0x01)}
	})
	cs := fakeConnect(t, gs.Addr().String())
	gsPort := gs.Addr().(*net.TCPAddr).Port

	t.Run("off", func(t *testing.T) {
		addr, sessions := startProxy(t, &Proxy{Keys: k}, Connect, cs.Addr().String())
		c, err := net.Dial("tcp", addr)
		if err != nil {
			t.Fatal(err)
		}
		host, port := readServerInfo(t, c)
		c.Close()
		if host != "127.0.0.1" || port != gsPort {
			t.Fatalf("unchanged answer = %s:%d, want 127.0.0.1:%d", host, port, gsPort)
		}
		s := waitSession(t, sessions, Connect)
		if len(s.Records) != 3 || s.Records[0].Dir != ServerToClient || s.Records[1].Dir != ClientToServer {
			t.Fatalf("records = %+v", s.Records)
		}
		if err := Replay(s, k); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("on", func(t *testing.T) {
		logBuf := &syncBuf{}
		p := &Proxy{Keys: k, Rewrite: true, RewriteHost: "127.0.0.1", Log: logBuf}
		addr, sessions := startProxy(t, p, Connect, cs.Addr().String())
		c, err := net.Dial("tcp", addr)
		if err != nil {
			t.Fatal(err)
		}
		host, port := readServerInfo(t, c)
		c.Close()
		if host != "127.0.0.1" || port == gsPort {
			t.Fatalf("rewritten answer = %s:%d", host, port)
		}
		s := waitSession(t, sessions, Connect)
		info := s.Records[2]
		if info.Forwarded == nil || binary.LittleEndian.Uint16(info.Plain[20:]) != uint16(gsPort) {
			t.Fatalf("info record = %+v", info)
		}

		// The client's next connection goes through the proxy route.
		g, err := net.Dial("tcp", net.JoinHostPort(host, strconv.Itoa(port)))
		if err != nil {
			t.Fatal(err)
		}
		cli := clientCodec(k)
		w, _ := cli.Encode(frame.Build(frame.C3, 0xF1, 0x01, 'x'))
		g.Write(w)
		if pk := <-got; pk.Data[2] != 0xF1 {
			t.Fatalf("server got %x", pk.Data)
		}
		if back := readPackets(t, g, cli, 1); back[0].Data[2] != 0xF1 {
			t.Fatalf("client got %x", back[0].Data)
		}
		g.Close()
		gsess := waitSession(t, sessions, Game)
		if gsess.Upstream != gs.Addr().String() || len(gsess.Records) != 2 {
			t.Fatalf("routed session = %+v", gsess)
		}
		if err := Replay(gsess, k); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(logBuf.String(), "rewrite 127.0.0.1:"+strconv.Itoa(gsPort)) {
			t.Errorf("log:\n%s", logBuf.String())
		}
	})
}

func TestServerMoveRewrite(t *testing.T) {
	k := testKeys(t)
	gs2, got2 := fakeGame(t, k, func(gscodec.Packet) [][]byte { return nil })
	gs2Port := gs2.Addr().(*net.TCPAddr).Port
	move := make([]byte, 0x2C)
	move[0], move[1], move[2], move[3] = frame.C3, 0x2C, 0xB1, 0x00
	copy(move[4:], "127.0.0.1")
	binary.LittleEndian.PutUint16(move[20:], uint16(gs2Port))
	binary.LittleEndian.PutUint16(move[22:], 19)
	binary.LittleEndian.PutUint32(move[24:], 0xAABBCCDD)
	gs1, _ := fakeGame(t, k, func(gscodec.Packet) [][]byte { return [][]byte{move} })

	addr, sessions := startProxy(t, &Proxy{Keys: k, Rewrite: true, RewriteHost: "127.0.0.1"}, Game, gs1.Addr().String())
	c, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	cli := clientCodec(k)
	w, _ := cli.Encode(frame.Build(frame.C1, 0x0E, 0, 0, 0, 0))
	c.Write(w)
	back := readPackets(t, c, cli, 1)[0]
	c.Close()
	if !back.Encrypted || back.Data[2] != 0xB1 {
		t.Fatalf("client got %+v", back)
	}
	port := int(binary.LittleEndian.Uint16(back.Data[20:]))
	if cstring(back.Data[4:20]) != "127.0.0.1" || port == gs2Port ||
		binary.LittleEndian.Uint16(back.Data[22:]) != 19 || binary.LittleEndian.Uint32(back.Data[24:]) != 0xAABBCCDD {
		t.Fatalf("rewritten move = %x", back.Data)
	}
	s := waitSession(t, sessions, Game)
	if err := Replay(s, k); err != nil { // replays the upstream bytes, not the rewritten ones
		t.Fatal(err)
	}

	g, err := net.Dial("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	cli2 := clientCodec(k)
	w, _ = cli2.Encode(frame.Build(frame.C3, 0xB1, 0x01, 1, 2, 3))
	g.Write(w)
	select {
	case pk := <-got2:
		if pk.Data[2] != 0xB1 || pk.Data[3] != 0x01 {
			t.Fatalf("second server got %x", pk.Data)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("second server got nothing")
	}
}

func TestPassthroughAfterFramingError(t *testing.T) {
	ln := listen(t)
	garbage := []byte{0xC1, 0x04, 0x00, 0x01, 0xFF, 0x00, 0x11, 0x22}
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		c.Write(garbage)
		io.Copy(io.Discard, c)
	}()
	addr, sessions := startProxy(t, &Proxy{Keys: testKeys(t)}, Connect, ln.Addr().String())
	c, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	got := make([]byte, len(garbage))
	c.SetReadDeadline(time.Now().Add(5 * time.Second))
	if _, err := io.ReadFull(c, got); err != nil || !bytes.Equal(got, garbage) {
		t.Fatalf("got %x %v", got, err)
	}
	c.Close()
	if s := waitSession(t, sessions, Connect); len(s.Records) != 1 {
		t.Fatalf("records %+v", s.Records)
	}
}

func TestSplitterChunks(t *testing.T) {
	k := testKeys(t)
	cli := clientCodec(k)
	var wire []byte
	for _, p := range [][]byte{frame.Build(frame.C1, 0x00, 'h', 'i'), frame.Build(frame.C4, 0xF3, make([]byte, 300)...)} {
		w, err := cli.Encode(p)
		if err != nil {
			t.Fatal(err)
		}
		wire = append(wire, w...)
	}
	whole, err := newSplitter(Game, ClientToServer, k).feed(wire)
	if err != nil || len(whole) != 2 {
		t.Fatalf("%v %v", whole, err)
	}
	sp := newSplitter(Game, ClientToServer, k)
	var parts []Record
	for i := 0; i < len(wire); i += 5 {
		r, err := sp.feed(wire[i:min(i+5, len(wire))])
		if err != nil {
			t.Fatal(err)
		}
		parts = append(parts, r...)
	}
	if len(parts) != 2 {
		t.Fatalf("%d records", len(parts))
	}
	for i := range parts {
		if !bytes.Equal(parts[i].Wire, whole[i].Wire) || !bytes.Equal(parts[i].Plain, whole[i].Plain) {
			t.Errorf("record %d differs", i)
		}
	}
	if !bytes.Equal(append(append([]byte(nil), whole[0].Wire...), whole[1].Wire...), wire) {
		t.Fatal("wire bytes do not add up to the stream")
	}
	if whole[0].Plain[2] != 0x00 || whole[1].WireKind() != frame.C4 {
		t.Fatalf("decoded %x / %x", whole[0].Plain, whole[1].Plain)
	}
}

// TestGoldenVectors replays every captured session in testdata/golden
// (files written by cmd/muproxy -golden and copied here).
func TestGoldenVectors(t *testing.T) {
	files, _ := filepath.Glob(filepath.Join("testdata", "golden", "*.json"))
	if len(files) == 0 {
		t.Skip("no captured sessions in testdata/golden")
	}
	k := testKeys(t)
	for _, f := range files {
		s, err := LoadSession(f)
		if err != nil {
			t.Fatal(err)
		}
		if err := Replay(s, k); err != nil {
			t.Errorf("%s: %v", filepath.Base(f), err)
		}
	}
}
