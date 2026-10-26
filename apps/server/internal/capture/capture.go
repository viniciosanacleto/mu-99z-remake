// Package capture is the man-in-the-middle used to record sessions between
// the original Main.exe and a ConnectServer or GameServer (original or ours)
// and to turn them into golden vectors (docs/server-plan/15-captura.md).
//
// Bytes are forwarded unchanged, packet by packet. A copy is decoded with
// the same layers the server and client use (internal/protocol/frame,
// crypto and gscodec):
//
//	ConnectServer: plain C1/C2 framing in both directions;
//	GameServer, client -> server: stream cipher -> framing ->
//	  [C3/C4: SimpleModulus with Dec1.dat + serial] -> XOR32 (gscodec.Server);
//	GameServer, server -> client: stream cipher -> framing ->
//	  [C3/C4: SimpleModulus with Dec2.dat + serial] (gscodec.Client).
package capture

import (
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/config/gsconfig"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/crypto"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/frame"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/gscodec"
)

// Kind is the protocol spoken on a proxied connection.
type Kind string

const (
	Connect Kind = "connect" // ConnectServer: plain
	Game    Kind = "game"    // GameServer: stream cipher, SimpleModulus, XOR32
)

// Dir is the direction of a packet.
type Dir string

const (
	ClientToServer Dir = "c2s"
	ServerToClient Dir = "s2c"
)

// Keys are the cipher keys of both ends of a GameServer connection.
type Keys struct {
	// Stream is derived from CustomerName/ServerSerial of the GameServer's
	// Common settings ("GameServerInfo - Common.dat", FUN_00487a60).
	Stream crypto.StreamKeys
	// ServerEnc/ServerDec are Data/Hack/Enc2.dat and Dec1.dat; ClientEnc
	// and ClientDec are the client's Data/Enc1.dat and Dec2.dat.
	ServerEnc, ServerDec crypto.ModulusKeys
	ClientEnc, ClientDec crypto.ModulusKeys
}

// LoadKeys reads the stream keys from the GameServer instance file
// (gsConfig, e.g. config/gameservers/4.GameServer.json), Enc2.dat/Dec1.dat
// from serverHackDir (data/Data/Hack) and Enc1.dat/Dec2.dat from
// clientDataDir (the client's Data folder).
func LoadKeys(gsConfig, serverHackDir, clientDataDir string) (Keys, error) {
	var k Keys
	cfg, err := gsconfig.Load(gsConfig)
	if err != nil {
		return k, err
	}
	if k.Stream, err = crypto.DeriveStreamKeys(cfg.CustomerName, cfg.ServerSerial); err != nil {
		return k, err
	}
	files := []struct {
		dst  *crypto.ModulusKeys
		path string
	}{
		{&k.ServerEnc, filepath.Join(serverHackDir, "Enc2.dat")},
		{&k.ServerDec, filepath.Join(serverHackDir, "Dec1.dat")},
		{&k.ClientEnc, filepath.Join(clientDataDir, "Enc1.dat")},
		{&k.ClientDec, filepath.Join(clientDataDir, "Dec2.dat")},
	}
	for _, f := range files {
		if *f.dst, err = crypto.LoadKeyFile(f.path); err != nil {
			return k, err
		}
	}
	return k, nil
}

// Hex is a byte slice that marshals as a hex string.
type Hex []byte

func (h Hex) MarshalText() ([]byte, error) { return []byte(hex.EncodeToString(h)), nil }

func (h *Hex) UnmarshalText(b []byte) error {
	v, err := hex.DecodeString(string(b))
	*h = v
	return err
}

// Record is one captured packet.
type Record struct {
	// T is the offset from the session start, in microseconds.
	T   int64 `json:"t"`
	Dir Dir   `json:"dir"`
	// Wire is the packet exactly as the sender put it on the socket.
	Wire Hex `json:"wire"`
	// Plain is the decoded packet in C1/C2 form (for C3/C4 the serial byte
	// is removed and kept in Serial). Empty when decoding failed.
	Plain     Hex  `json:"plain,omitempty"`
	Encrypted bool `json:"enc,omitempty"`
	Serial    byte `json:"serial,omitempty"`
	// Forwarded is set when the proxy rewrote the packet (server address
	// rewriting); it holds the bytes actually sent to the peer.
	Forwarded Hex    `json:"fwd,omitempty"`
	Err       string `json:"err,omitempty"`
}

// Head returns the head and sub-code of the decoded packet.
func (r *Record) Head() (head, sub byte, hasSub bool) {
	if len(r.Plain) < 3 || len(r.Plain) <= frame.HeaderLen(r.Plain[0]) {
		return 0, 0, false
	}
	sub, hasSub = frame.Sub(r.Plain)
	return frame.Head(r.Plain), sub, hasSub
}

// WireKind is the packet type byte as it went on the wire (C1..C4).
func (r *Record) WireKind() byte {
	if len(r.Plain) == 0 {
		return 0
	}
	k := r.Plain[0]
	if r.Encrypted {
		k += 2 // C1 -> C3, C2 -> C4
	}
	return k
}

// errPassthrough marks a direction that could not be framed any more; its
// bytes are then forwarded raw.
var errPassthrough = errors.New("capture: framing lost, forwarding raw bytes")

// splitter cuts one direction of a connection into packets and decodes them.
type splitter struct {
	kind Kind
	dir  Dir
	keys Keys
	buf  []byte // stream-decoded bytes not yet framed
	srv  *gscodec.Server
	cli  *gscodec.Client
}

func newSplitter(kind Kind, dir Dir, k Keys) *splitter {
	s := &splitter{kind: kind, dir: dir, keys: k}
	if kind == Game {
		if dir == ClientToServer {
			s.srv = gscodec.NewServer(gscodec.Keys{Stream: k.Stream, Decrypt: k.ServerDec})
		} else {
			s.cli = gscodec.NewClient(gscodec.Keys{Stream: k.Stream, Decrypt: k.ClientDec})
		}
	}
	return s
}

func (s *splitter) maxSize() int {
	if s.kind == Game {
		return frame.MaxSizeGame
	}
	return frame.MaxSizeConnect
}

// feed adds raw socket bytes and returns every complete packet. A framing
// error returns errPassthrough; pending then gives the bytes to forward.
func (s *splitter) feed(raw []byte) ([]Record, error) {
	chunk := append([]byte(nil), raw...)
	if s.kind == Game {
		s.keys.Stream.Decode(chunk)
	}
	s.buf = append(s.buf, chunk...)
	var out []Record
	for {
		n, err := frame.Next(s.buf, s.maxSize())
		if err != nil {
			return out, fmt.Errorf("%w: %v", errPassthrough, err)
		}
		if n == 0 {
			return out, nil
		}
		out = append(out, s.decode(append([]byte(nil), s.buf[:n]...)))
		s.buf = s.buf[n:]
	}
}

// pending returns the unframed bytes in their wire form.
func (s *splitter) pending() []byte {
	b := append([]byte(nil), s.buf...)
	if s.kind == Game {
		s.keys.Stream.Encode(b) // the stream cipher works byte by byte
	}
	s.buf = nil
	return b
}

// decode turns one framed packet (stream-decoded) into a Record.
func (s *splitter) decode(framed []byte) Record {
	r := Record{Dir: s.dir}
	if s.kind != Game {
		r.Wire, r.Plain = framed, append(Hex(nil), framed...)
		return r
	}
	wire := append([]byte(nil), framed...)
	s.keys.Stream.Encode(wire)
	r.Wire = wire
	var pkts []gscodec.Packet
	var err error
	if s.srv != nil {
		pkts, err = s.srv.Feed(wire)
	} else {
		pkts, err = s.cli.Feed(wire)
	}
	switch {
	case err != nil:
		r.Err = err.Error()
		s.resetCodec()
	case len(pkts) != 1:
		r.Err = fmt.Sprintf("codec returned %d packets", len(pkts))
		s.resetCodec()
	default:
		r.Plain, r.Encrypted, r.Serial = pkts[0].Data, pkts[0].Encrypted, pkts[0].Serial
	}
	return r
}

// resetCodec drops the codec's buffer after a decode error so the next
// packet starts clean (the framing itself is kept by the splitter).
func (s *splitter) resetCodec() {
	if s.srv != nil {
		s.srv = gscodec.NewServer(gscodec.Keys{Stream: s.keys.Stream, Decrypt: s.keys.ServerDec})
	} else if s.cli != nil {
		s.cli = gscodec.NewClient(gscodec.Keys{Stream: s.keys.Stream, Decrypt: s.keys.ClientDec})
	}
}

// encodeServerPacket rebuilds the wire form of a server -> client packet
// from its plain form: C3/C4 get the serial and SimpleModulus with
// Enc2.dat again (FUN_00504f00), then the stream cipher. SimpleModulus is
// deterministic, so an unchanged packet gives back the captured bytes.
func encodeServerPacket(k Keys, plain []byte, encrypted bool, serial byte) ([]byte, error) {
	p := append([]byte(nil), plain...)
	if encrypted {
		p[0] += 2
	}
	enc := gscodec.NewServer(gscodec.Keys{Stream: k.Stream, Encrypt: k.ServerEnc})
	enc.SetSerial(serial)
	return enc.Encode(p)
}
