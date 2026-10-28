package loadbot

import (
	"errors"
	"fmt"
	"io"
	"net"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/config/gsconfig"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/crypto"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/frame"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/gscodec"
)

// Client is what every bot needs to talk to a GameServer instance.
type Client struct {
	Keys    gscodec.Keys
	Version [5]byte
	Serial  string
}

// LoadClient reads the version, serial and stream keys from a GameServer
// instance file and the client keys Enc1.dat/Dec2.dat from keyDir.
func LoadClient(gsConfig, keyDir string) (Client, error) {
	var c Client
	cfg, err := gsconfig.Load(gsConfig)
	if err != nil {
		return c, err
	}
	c.Version, c.Serial = cfg.ServerVersion, cfg.ServerSerial
	if c.Keys.Stream, err = crypto.DeriveStreamKeys(cfg.CustomerName, cfg.ServerSerial); err != nil {
		return c, err
	}
	if c.Keys.Encrypt, err = crypto.LoadKeyFile(filepath.Join(keyDir, "Enc1.dat")); err != nil {
		return c, err
	}
	if c.Keys.Decrypt, err = crypto.LoadKeyFile(filepath.Join(keyDir, "Dec2.dat")); err != nil {
		return c, err
	}
	return c, nil
}

// Conn is one GameServer connection. Packets are read by their own
// goroutine and delivered on In, which is closed when the connection ends.
type Conn struct {
	nc    net.Conn
	codec *gscodec.Client
	In    <-chan []byte
	// BytesIn and BytesOut count the raw bytes on the wire.
	BytesIn, BytesOut atomic.Int64
	err               atomic.Value
}

// Dial opens a GameServer connection.
func Dial(addr string, keys gscodec.Keys, timeout time.Duration) (*Conn, error) {
	nc, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		return nil, err
	}
	in := make(chan []byte, 256)
	c := &Conn{nc: nc, codec: gscodec.NewClient(keys), In: in}
	go c.read(in)
	return c, nil
}

func (c *Conn) read(in chan<- []byte) {
	defer close(in)
	buf := make([]byte, 0x4000)
	for {
		n, err := c.nc.Read(buf)
		if n > 0 {
			c.BytesIn.Add(int64(n))
			pkts, ferr := c.codec.Feed(buf[:n])
			for _, p := range pkts {
				in <- p.Data
			}
			if ferr != nil {
				c.err.Store(ferr)
				return
			}
		}
		if err != nil {
			if !errors.Is(err, io.EOF) && !errors.Is(err, net.ErrClosed) {
				c.err.Store(err)
			}
			return
		}
	}
}

// Err returns the read error that ended the connection, if any.
func (c *Conn) Err() error {
	if e, ok := c.err.Load().(error); ok {
		return e
	}
	return nil
}

// Send encodes and writes one plain packet. Only the bot's own goroutine
// sends, so the C3 serial stays in order.
func (c *Conn) Send(p []byte) error {
	enc, err := c.codec.Encode(p)
	if err != nil {
		return err
	}
	c.nc.SetWriteDeadline(time.Now().Add(5 * time.Second))
	n, err := c.nc.Write(enc)
	c.BytesOut.Add(int64(n))
	return err
}

// Next waits for the next packet whose head (and sub when >= 0) matches,
// handing every other packet to skip (which may be nil).
func (c *Conn) Next(head byte, sub int, timeout time.Duration, skip func([]byte)) ([]byte, error) {
	t := time.NewTimer(timeout)
	defer t.Stop()
	for {
		select {
		case p, ok := <-c.In:
			if !ok {
				if err := c.Err(); err != nil {
					return nil, err
				}
				return nil, io.EOF
			}
			if Match(p, head, sub) {
				return p, nil
			}
			if skip != nil {
				skip(p)
			}
		case <-t.C:
			return nil, fmt.Errorf("timeout waiting for %02X %02X", head, sub&0xFF)
		}
	}
}

// Match reports whether p has the head (and sub when >= 0).
func Match(p []byte, head byte, sub int) bool {
	if len(p) < 3 || frame.Head(p) != head {
		return false
	}
	if sub < 0 {
		return true
	}
	s, ok := frame.Sub(p)
	return ok && int(s) == sub
}

// Close closes the connection.
func (c *Conn) Close() error { return c.nc.Close() }

// GameServerAddress asks the ConnectServer for a GameServer address the
// way the client does (F4 06 then F4 03). host, when set, replaces the
// address the ConnectServer answers with.
func GameServerAddress(connectAddr string, code byte, host string, timeout time.Duration) (string, error) {
	nc, err := net.DialTimeout("tcp", connectAddr, timeout)
	if err != nil {
		return "", err
	}
	defer nc.Close()
	nc.SetDeadline(time.Now().Add(timeout))
	var buf []byte
	next := func(head byte, sub int) ([]byte, error) {
		tmp := make([]byte, 0x1000)
		for {
			for {
				n, err := frame.Next(buf, frame.MaxSizeConnect)
				if err != nil {
					return nil, err
				}
				if n == 0 {
					break
				}
				p := buf[:n:n]
				buf = buf[n:]
				if Match(p, head, sub) {
					return p, nil
				}
			}
			n, err := nc.Read(tmp)
			if err != nil {
				return nil, fmt.Errorf("connect server: waiting for %02X %02X: %w", head, sub, err)
			}
			buf = append(buf, tmp[:n]...)
		}
	}
	if _, err := next(0x00, 0x01); err != nil { // C1 04 00 01 hello
		return "", err
	}
	if _, err := nc.Write(ServerList()); err != nil {
		return "", err
	}
	if _, err := next(0xF4, 0x06); err != nil {
		return "", err
	}
	if _, err := nc.Write(ServerInfo(code)); err != nil {
		return "", err
	}
	p, err := next(0xF4, 0x03)
	if err != nil {
		return "", err
	}
	return parseServerInfo(p, host)
}

// parseServerInfo reads C1 16 F4 03 ip[16] port(LE).
func parseServerInfo(p []byte, host string) (string, error) {
	if len(p) < 0x16 {
		return "", fmt.Errorf("short F4 03: % X", p)
	}
	ip := p[4:20]
	for i, b := range ip {
		if b == 0 {
			ip = ip[:i]
			break
		}
	}
	if host == "" {
		host = string(ip)
	}
	port := int(p[20]) | int(p[21])<<8
	return net.JoinHostPort(host, strconv.Itoa(port)), nil
}
