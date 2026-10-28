package connect

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/config/servercfg"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/frame"
)

// maxClients: the original indexes clients in a 100-slot table (index < 100).
const maxClients = 100

// Config mirrors [ConnectServer] in config/server.toml ([ConnectServerInfo]
// of ConnectServer.ini).
type Config struct {
	Port               int
	MaxIdle            time.Duration // MaxConnectionIdle (seconds)
	MaxPerIP           int           // MaxConnectionPerIP; 0 = unlimited
	MaxPacketPerSecond int           // 0 = unlimited
	BlackList          []string      // BlackList
}

// ConfigFrom takes the [ConnectServer] table of config/server.toml.
func ConfigFrom(f servercfg.ConnectServer) Config {
	return Config{
		Port:               f.ConnectServerPortTCP,
		MaxIdle:            time.Duration(f.MaxConnectionIdle) * time.Second,
		MaxPerIP:           f.MaxConnectionPerIP,
		MaxPacketPerSecond: f.MaxPacketPerSecond,
		BlackList:          f.BlackList,
	}
}

// Server is the TCP front end on ConnectServerPortTCP.
type Server struct {
	cfg Config
	reg *Registry
	log *slog.Logger
	now func() time.Time

	mu      sync.Mutex
	clients int
	perIP   map[string]int
	black   map[string]bool
}

// NewServer builds a ConnectServer front end that lists the servers of reg.
func NewServer(cfg Config, reg *Registry, log *slog.Logger) *Server {
	s := &Server{
		cfg:   cfg,
		reg:   reg,
		log:   log,
		now:   time.Now,
		perIP: map[string]int{},
		black: map[string]bool{},
	}
	for _, ip := range cfg.BlackList {
		s.black[ip] = true
	}
	return s
}

// Serve accepts connections until ctx is cancelled or ln fails, then
// closes the open connections and waits for their handlers.
func (s *Server) Serve(ctx context.Context, ln net.Listener) error {
	var wg sync.WaitGroup
	defer wg.Wait()
	// Deferred after wg.Wait, so it runs first: a failed listener closes the
	// open clients too (handle closes its connection when ctx is done).
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	stopListener := context.AfterFunc(ctx, func() { ln.Close() })
	defer stopListener()
	for {
		c, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("accepting: %w", err)
		}
		ip := hostOf(c.RemoteAddr())
		if reason := s.admit(ip); reason != "" {
			s.log.Info("connection refused", "ip", ip, "reason", reason)
			c.Close()
			continue
		}
		wg.Go(func() {
			defer s.release(ip)
			s.handle(ctx, c)
		})
	}
}

func hostOf(a net.Addr) string {
	host, _, err := net.SplitHostPort(a.String())
	if err != nil {
		return a.String()
	}
	return host
}

func (s *Server) admit(ip string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch {
	case s.black[ip]:
		return "blacklisted"
	case s.clients >= maxClients:
		return "server full"
	case s.cfg.MaxPerIP > 0 && s.perIP[ip] >= s.cfg.MaxPerIP:
		return "too many connections from ip"
	}
	s.clients++
	s.perIP[ip]++
	return ""
}

func (s *Server) release(ip string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.clients--
	if s.perIP[ip]--; s.perIP[ip] <= 0 {
		delete(s.perIP, ip)
	}
}

var errFlood = errors.New("packets exceeded")

func (s *Server) handle(ctx context.Context, c net.Conn) {
	defer c.Close()
	stop := context.AfterFunc(ctx, func() { c.Close() })
	defer stop()

	log := s.log.With("ip", c.RemoteAddr().String())
	if err := s.write(c, Hello()); err != nil {
		return
	}
	if err := s.write(c, s.reg.ServerNames()); err != nil {
		return
	}

	buf := make([]byte, 0, frame.MaxSizeConnect)
	tmp := make([]byte, frame.MaxSizeConnect)
	var windowStart time.Time
	windowCount := 0
	for {
		if s.cfg.MaxIdle > 0 {
			c.SetReadDeadline(s.now().Add(s.cfg.MaxIdle))
		}
		n, err := c.Read(tmp)
		if err != nil {
			var ne net.Error
			if errors.As(err, &ne) && ne.Timeout() {
				log.Info("client disconnected, timeout")
			}
			return
		}
		buf = append(buf, tmp[:n]...)
		for {
			size, err := frame.Next(buf, frame.MaxSizeConnect)
			if err != nil {
				log.Info("protocol error", "err", err)
				return
			}
			if size == 0 {
				break
			}
			// Anti-flood: the original counts packets per client and drops it
			// once the count reaches MaxPacketPerSecond. The one-second window
			// is inferred from the key name.
			if now := s.now(); now.Sub(windowStart) >= time.Second {
				windowStart, windowCount = now, 0
			}
			windowCount++
			if s.cfg.MaxPacketPerSecond > 0 && windowCount >= s.cfg.MaxPacketPerSecond {
				log.Info("disconnected", "err", errFlood)
				return
			}
			if resp := s.reg.Handle(buf[:size]); resp != nil {
				if err := s.write(c, resp); err != nil {
					return
				}
			}
			buf = append(buf[:0], buf[size:]...)
		}
	}
}

func (s *Server) write(c net.Conn, p []byte) error {
	c.SetWriteDeadline(s.now().Add(10 * time.Second))
	if _, err := c.Write(p); err != nil {
		return fmt.Errorf("write: %w", err)
	}
	return nil
}
