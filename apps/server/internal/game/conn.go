package game

import (
	"net"
	"sync"
	"time"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/gscodec"
)

// sendQueue bounds the per-connection output, like the 0x4000-byte secondary
// buffer of FUN_00504f00 whose overflow disconnects the client.
const sendQueue = 1024

// conn is one client socket. The reader goroutine decodes packets and posts
// them to the world; the writer goroutine encodes (serial, SimpleModulus,
// stream cipher) and writes. The codec halves are never shared.
type conn struct {
	nc    net.Conn
	codec *gscodec.Server
	ip    string
	out   chan []byte
	done  chan struct{}
	once  sync.Once
	index int
}

func newConn(nc net.Conn, keys gscodec.Keys) *conn {
	ip := nc.RemoteAddr().String()
	if host, _, err := net.SplitHostPort(ip); err == nil {
		ip = host
	}
	return &conn{nc: nc, codec: gscodec.NewServer(keys), ip: ip,
		out: make(chan []byte, sendQueue), done: make(chan struct{}), index: -1}
}

// send queues a plain packet. It never blocks the world goroutine: a full
// queue closes the connection.
func (c *conn) send(p []byte) {
	select {
	case <-c.done:
	case c.out <- p:
	default:
		c.close()
	}
}

// sendAndClose queues a final packet and closes after it is written
// (FUN_005126b0 blocking send followed by closesocket).
func (c *conn) sendAndClose(p []byte) {
	c.send(p)
	c.send(nil)
}

func (c *conn) close() {
	c.once.Do(func() {
		close(c.done)
		c.nc.Close()
	})
}

func (c *conn) writer() {
	for {
		select {
		case <-c.done:
			return
		case p := <-c.out:
			if p == nil {
				c.close()
				return
			}
			enc, err := c.codec.Encode(p)
			if err != nil {
				c.close()
				return
			}
			c.nc.SetWriteDeadline(time.Now().Add(30 * time.Second))
			if _, err := c.nc.Write(enc); err != nil {
				c.close()
				return
			}
		}
	}
}

// reader feeds decoded packets to onPacket until the socket fails, then
// calls onClose once.
func (c *conn) reader(onPacket func(gscodec.Packet), onClose func()) {
	defer onClose()
	defer c.close()
	buf := make([]byte, 0x2000)
	for {
		n, err := c.nc.Read(buf)
		if err != nil {
			return
		}
		pkts, err := c.codec.Feed(buf[:n])
		for _, p := range pkts {
			onPacket(p)
		}
		if err != nil {
			return
		}
	}
}
