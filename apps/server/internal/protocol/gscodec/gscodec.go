// Package gscodec turns the raw byte stream of a GameServer client socket into
// plain C1/C2 packets and back, reproducing the original layering
// (docs/server-plan/02-protocolo-de-rede.md §2):
//
//	receive: stream cipher -> framing -> [C3/C4: SimpleModulus + serial] -> XOR32
//	send:    [C3/C4: serial + SimpleModulus] -> stream cipher
//
// The Client type implements the inverse layering of the original Main.exe and
// exists for tests and headless bots.
package gscodec

import (
	"errors"
	"fmt"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/crypto"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/frame"
)

// maxPlain is the largest plain packet body accepted after decryption
// (FUN_005049c0 rejects 0x7FE < len).
const maxPlain = 0x7FE

var (
	ErrTooLarge  = errors.New("gscodec: packet too large")
	ErrTooShort  = errors.New("gscodec: decrypted packet too short")
	ErrNotPacket = errors.New("gscodec: not a C1/C2/C3/C4 packet")
)

// Keys holds everything both ends need.
type Keys struct {
	Stream  crypto.StreamKeys
	Encrypt crypto.ModulusKeys // server: Enc2.dat, client: Enc1.dat
	Decrypt crypto.ModulusKeys // server: Dec1.dat, client: Dec2.dat
}

// Packet is a decoded packet in plain C1/C2 form.
type Packet struct {
	Data      []byte
	Encrypted bool // arrived as C3/C4
	Serial    byte // valid only when Encrypted
}

// Server is the per-connection codec used by the GameServer side.
type Server struct {
	keys      Keys
	buf       []byte
	serialOut byte
}

func NewServer(k Keys) *Server { return &Server{keys: k} }

// SetSerial sets the serial byte the next C3/C4 packet is sent with (used
// to replay captured sessions that did not start at serial 0).
func (s *Server) SetSerial(b byte) { s.serialOut = b }

// Feed decodes newly received raw bytes and returns every complete packet.
// An error means the connection must be closed.
func (s *Server) Feed(raw []byte) ([]Packet, error) {
	chunk := append([]byte(nil), raw...)
	s.keys.Stream.Decode(chunk)
	s.buf = append(s.buf, chunk...)

	var out []Packet
	for {
		n, err := frame.Next(s.buf, frame.MaxSizeGame)
		if err != nil {
			return out, err
		}
		if n == 0 {
			return out, nil
		}
		raw := s.buf[:n]
		p, err := s.decodePacket(raw)
		s.buf = s.buf[n:]
		if err != nil {
			return out, err
		}
		out = append(out, p)
	}
}

func (s *Server) decodePacket(raw []byte) (Packet, error) {
	switch raw[0] {
	case frame.C1, frame.C2:
		if len(raw)-1 > maxPlain {
			return Packet{}, ErrTooLarge
		}
		p := append([]byte(nil), raw...)
		crypto.Xor32Decrypt(p)
		return Packet{Data: p}, nil
	case frame.C3, frame.C4:
		p, serial, err := unwrapEncrypted(raw, s.keys.Decrypt)
		if err != nil {
			return Packet{}, err
		}
		crypto.Xor32Decrypt(p)
		return Packet{Data: p, Encrypted: true, Serial: serial}, nil
	}
	return Packet{}, ErrNotPacket
}

// unwrapEncrypted decrypts a C3/C4 packet and rebuilds it as C1/C2,
// returning the leading serial byte separately.
func unwrapEncrypted(raw []byte, k crypto.ModulusKeys) ([]byte, byte, error) {
	hl := frame.HeaderLen(raw[0])
	dec, err := k.Decrypt(raw[hl:])
	if err != nil {
		return nil, 0, err
	}
	if len(dec) < 2 {
		return nil, 0, ErrTooShort
	}
	kind := frame.C1
	if raw[0] == frame.C4 {
		kind = frame.C2
	}
	// C3 is limited to 0x7FE decrypted bytes, C4 to 0x7FD (0x7FE < len+1).
	if len(dec)+hl-2 > maxPlain {
		return nil, 0, ErrTooLarge
	}
	p := make([]byte, hl+len(dec)-1)
	p[0] = kind
	frame.SetSize(p, len(p))
	copy(p[hl:], dec[1:])
	return p, dec[0], nil
}

// Encode prepares a packet for sending (FUN_00504f00). C1/C2 go out as is,
// C3/C4 are encrypted with the per-connection serial. The whole result is
// passed through the stream cipher.
func (s *Server) Encode(p []byte) ([]byte, error) {
	out, err := wrapEncrypted(p, s.keys.Encrypt, &s.serialOut)
	if err != nil {
		return nil, err
	}
	s.keys.Stream.Encode(out)
	return out, nil
}

// wrapEncrypted returns a copy of p, encrypted when p is C3/C4.
func wrapEncrypted(p []byte, k crypto.ModulusKeys, serial *byte) ([]byte, error) {
	var out []byte
	switch p[0] {
	case frame.C1, frame.C2:
		out = append([]byte(nil), p...)
	case frame.C3, frame.C4:
		hl := frame.HeaderLen(p[0])
		plain := make([]byte, 0, len(p)-hl+1)
		plain = append(plain, *serial)
		plain = append(plain, p[hl:]...)
		*serial++
		enc := k.Encrypt(plain)
		size := hl + len(enc)
		if p[0] == frame.C3 && size > 0xFF {
			return nil, fmt.Errorf("%w: C3 needs %d bytes", ErrTooLarge, size)
		}
		out = make([]byte, size)
		out[0] = p[0]
		frame.SetSize(out, size)
		copy(out[hl:], enc)
	default:
		return nil, ErrNotPacket
	}
	if len(out) > frame.MaxSizeGame {
		return nil, ErrTooLarge
	}
	return out, nil
}

// Client is the inverse codec, matching the original client.
type Client struct {
	keys      Keys
	buf       []byte
	serialOut byte
}

func NewClient(k Keys) *Client { return &Client{keys: k} }

// SetSerial sets the serial byte the next C3/C4 packet is sent with.
func (c *Client) SetSerial(b byte) { c.serialOut = b }

// Encode prepares a client packet: XOR32 on the plain form, then SimpleModulus
// for C3/C4, then the stream cipher.
func (c *Client) Encode(p []byte) ([]byte, error) {
	plain := append([]byte(nil), p...)
	crypto.Xor32Encrypt(plain)
	out, err := wrapEncrypted(plain, c.keys.Encrypt, &c.serialOut)
	if err != nil {
		return nil, err
	}
	c.keys.Stream.Encode(out)
	return out, nil
}

// Feed decodes bytes received from the server.
func (c *Client) Feed(raw []byte) ([]Packet, error) {
	chunk := append([]byte(nil), raw...)
	c.keys.Stream.Decode(chunk)
	c.buf = append(c.buf, chunk...)

	var out []Packet
	for {
		n, err := frame.Next(c.buf, frame.MaxSizeGame)
		if err != nil || n == 0 {
			return out, err
		}
		raw := c.buf[:n]
		var pkt Packet
		switch raw[0] {
		case frame.C1, frame.C2:
			pkt = Packet{Data: append([]byte(nil), raw...)}
		default:
			p, serial, err := unwrapEncrypted(raw, c.keys.Decrypt)
			if err != nil {
				return out, err
			}
			pkt = Packet{Data: p, Encrypted: true, Serial: serial}
		}
		c.buf = c.buf[n:]
		out = append(out, pkt)
	}
}
