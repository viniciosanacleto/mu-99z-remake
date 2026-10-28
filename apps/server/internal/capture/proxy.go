package capture

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/frame"
)

// Proxy forwards client connections to an upstream server and records them.
type Proxy struct {
	Keys Keys
	// Log receives one line per packet; nil disables the log.
	Log io.Writer
	// GoldenDir, when set, receives one golden-vector file per session.
	GoldenDir string
	// Rewrite makes the proxy replace the GameServer address in the
	// ConnectServer answer C1 16 F4 03 (FUN_004035f0) and in the map server
	// move C3 2C B1 00 (FUN_004a1c10) with a proxy listener on RewriteHost
	// that forwards to the original address, so the client's next
	// connection is captured too.
	Rewrite     bool
	RewriteHost string
	// Dial connects to an upstream address (net.Dialer by default).
	Dial func(ctx context.Context, addr string) (net.Conn, error)
	// Now is the clock (time.Now by default).
	Now func() time.Time
	// OnSession is called with every finished session (after saving).
	OnSession func(*Session)

	mu       sync.Mutex
	ctx      context.Context
	nextID   int
	routes   map[string]net.Listener // upstream address -> rewrite listener
	logMu    sync.Mutex
	wg       sync.WaitGroup
	sessions sync.WaitGroup
}

func (p *Proxy) now() time.Time {
	if p.Now != nil {
		return p.Now()
	}
	return time.Now()
}

func (p *Proxy) dial(ctx context.Context, addr string) (net.Conn, error) {
	if p.Dial != nil {
		return p.Dial(ctx, addr)
	}
	var d net.Dialer
	return d.DialContext(ctx, "tcp", addr)
}

// Serve accepts clients on ln and forwards each one to upstream, speaking
// kind. It returns when ctx is done (after closing ln and every rewrite
// listener) or when ln fails.
func (p *Proxy) Serve(ctx context.Context, ln net.Listener, kind Kind, upstream string) error {
	p.mu.Lock()
	if p.ctx == nil {
		p.ctx = ctx
	}
	p.mu.Unlock()
	stop := context.AfterFunc(ctx, func() { ln.Close() })
	defer stop()
	for {
		c, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("accepting: %w", err)
		}
		p.sessions.Go(func() { p.handle(ctx, c, kind, upstream) })
	}
}

// Wait blocks until every session and rewrite listener has finished.
func (p *Proxy) Wait() {
	p.wg.Wait()
	p.sessions.Wait()
}

// route returns the proxy address that forwards to upstream, opening a
// GameServer listener on RewriteHost the first time.
func (p *Proxy) route(upstream string) (host string, port int, err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.routes == nil {
		p.routes = map[string]net.Listener{}
	}
	ln := p.routes[upstream]
	if ln == nil {
		h := p.RewriteHost
		if h == "" {
			h = "127.0.0.1"
		}
		if ln, err = net.Listen("tcp", net.JoinHostPort(h, "0")); err != nil {
			return "", 0, err
		}
		p.routes[upstream] = ln
		ctx := p.ctx
		if ctx == nil {
			ctx = context.Background()
		}
		p.wg.Go(func() {
			if err := p.Serve(ctx, ln, Game, upstream); err != nil {
				p.logf("route %s -> %s: %v", ln.Addr(), upstream, err)
			}
		})
		p.logf("route %s -> %s", ln.Addr(), upstream)
	}
	a := ln.Addr().(*net.TCPAddr)
	host = p.RewriteHost
	if host == "" {
		host = a.IP.String()
	}
	return host, a.Port, nil
}

func (p *Proxy) handle(ctx context.Context, client net.Conn, kind Kind, upstream string) {
	defer client.Close()
	p.mu.Lock()
	p.nextID++
	id := p.nextID
	p.mu.Unlock()
	s := &Session{Version: FormatVersion, ID: id, Kind: kind, Client: client.RemoteAddr().String(),
		Upstream: upstream, Start: p.now()}
	if kind == Game {
		s.StreamAdd, s.StreamXor = p.Keys.Stream.Add, p.Keys.Stream.Xor
	}
	server, err := p.dial(ctx, upstream)
	if err != nil {
		p.logf("#%d %s: dial %s: %v", id, kind, upstream, err)
		return
	}
	defer server.Close()
	p.logf("#%d %s: %s <-> %s", id, kind, s.Client, upstream)
	stop := context.AfterFunc(ctx, func() { client.Close(); server.Close() })
	defer stop()

	done := make(chan struct{}, 2)
	go func() {
		p.pump(s, newSplitter(kind, ClientToServer, p.Keys), client, server)
		done <- struct{}{}
	}()
	go func() {
		p.pump(s, newSplitter(kind, ServerToClient, p.Keys), server, client)
		done <- struct{}{}
	}()
	<-done
	client.Close()
	server.Close()
	<-done
	p.logf("#%d %s: closed, %d packets", id, kind, len(s.Records))
	if p.GoldenDir != "" {
		if path, err := s.Save(p.GoldenDir); err != nil {
			p.logf("#%d save: %v", id, err)
		} else {
			p.logf("#%d saved %s", id, path)
		}
	}
	if p.OnSession != nil {
		p.OnSession(s)
	}
}

// pump copies src to dst packet by packet, recording each packet. After a
// framing error the rest of the direction is forwarded raw.
func (p *Proxy) pump(s *Session, sp *splitter, src, dst net.Conn) {
	buf := make([]byte, 0x4000)
	raw := false
	for {
		n, err := src.Read(buf)
		if n > 0 {
			if raw {
				if _, werr := dst.Write(buf[:n]); werr != nil {
					return
				}
			} else {
				recs, ferr := sp.feed(buf[:n])
				var out []byte
				for _, r := range recs {
					r.T = p.now().Sub(s.Start).Microseconds()
					if p.Rewrite && r.Dir == ServerToClient {
						p.rewrite(s, &r)
					}
					if r.Forwarded != nil {
						out = append(out, r.Forwarded...)
					} else {
						out = append(out, r.Wire...)
					}
					s.add(r)
					p.logRecord(s, &r)
				}
				if ferr != nil {
					raw = true
					p.logf("#%d %s %s: %v", s.ID, s.Kind, sp.dir, ferr)
					out = append(out, sp.pending()...)
				}
				if len(out) > 0 {
					if _, werr := dst.Write(out); werr != nil {
						return
					}
				}
			}
		}
		if err != nil {
			return
		}
	}
}

// rewrite replaces the GameServer address of F4 03 / B1 00 with a proxy
// route and sets r.Forwarded.
func (p *Proxy) rewrite(s *Session, r *Record) {
	head, sub, ok := r.Head()
	if !ok || len(r.Plain) < 0x16 || r.Plain[0] != frame.C1 {
		return
	}
	// Both packets carry ip[16] at offset 4 and the port (u16 LE) at 20:
	// C1 16 F4 03 ip[16] port, and C3 2C B1 00 ip[16] port code codes[4].
	isInfo := s.Kind == Connect && head == 0xF4 && sub == 0x03
	isMove := s.Kind == Game && head == 0xB1 && sub == 0x00
	if !isInfo && !isMove {
		return
	}
	ip := cstring(r.Plain[4:20])
	port := int(binary.LittleEndian.Uint16(r.Plain[20:]))
	host, lport, err := p.route(net.JoinHostPort(ip, strconv.Itoa(port)))
	if err != nil {
		p.logf("#%d rewrite %s:%d: %v", s.ID, ip, port, err)
		return
	}
	if len(host) > 15 {
		p.logf("#%d rewrite: host %q longer than 15 bytes", s.ID, host)
		return
	}
	plain := append([]byte(nil), r.Plain...)
	clear(plain[4:20])
	copy(plain[4:19], host)
	binary.LittleEndian.PutUint16(plain[20:], uint16(lport))
	if s.Kind == Connect {
		r.Forwarded = plain
	} else {
		fwd, err := encodeServerPacket(p.Keys, plain, r.Encrypted, r.Serial)
		if err != nil {
			p.logf("#%d rewrite encode: %v", s.ID, err)
			return
		}
		r.Forwarded = fwd
	}
	p.logf("#%d rewrite %s:%d -> %s:%d", s.ID, ip, port, host, lport)
}

func cstring(b []byte) string {
	if i := strings.IndexByte(string(b), 0); i >= 0 {
		b = b[:i]
	}
	return string(b)
}

func (p *Proxy) logf(format string, args ...any) {
	if p.Log == nil {
		return
	}
	p.logMu.Lock()
	defer p.logMu.Unlock()
	fmt.Fprintf(p.Log, "%s %s\n", p.now().Format("2006-01-02T15:04:05.000000"), fmt.Sprintf(format, args...))
}

// logRecord writes one packet line:
//
//	<time> #<session> <kind> <dir> <C1..C4> <head> [<sub>] len=<n> [serial=<s>] <plain hex>
func (p *Proxy) logRecord(s *Session, r *Record) {
	if p.Log == nil {
		return
	}
	var b strings.Builder
	fmt.Fprintf(&b, "#%d %s %s ", s.ID, s.Kind, r.Dir)
	if r.Err != "" || len(r.Plain) == 0 {
		fmt.Fprintf(&b, "undecoded len=%d err=%q wire=%s", len(r.Wire), r.Err, hex.EncodeToString(r.Wire))
		p.logf("%s", b.String())
		return
	}
	head, sub, hasSub := r.Head()
	fmt.Fprintf(&b, "%02X %02X", r.WireKind(), head)
	if hasSub {
		fmt.Fprintf(&b, " %02X", sub)
	}
	fmt.Fprintf(&b, " len=%d", len(r.Plain))
	if r.Encrypted {
		fmt.Fprintf(&b, " serial=%02X", r.Serial)
	}
	if r.Forwarded != nil {
		b.WriteString(" rewritten")
	}
	fmt.Fprintf(&b, " %s", hex.EncodeToString(r.Plain))
	p.logf("%s", b.String())
}
