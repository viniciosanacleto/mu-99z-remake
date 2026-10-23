package connect

import (
	"encoding/binary"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/frame"
)

// Heads and sub-codes handled by the ConnectServer dispatcher (FUN_00407930).
const (
	headServer      = 0xF4
	subServerInfo   = 0x03
	subServerList   = 0x06
	headCustom      = 0xF3
	subServerNames  = 0xEA
	serverNameField = 32 // strcpy_s(dst, 0x20, name) in FUN_00405d00
)

// Hello is sent right after accepting a connection: constant 0x010004C1
// stored little-endian (FUN_004025a0).
func Hello() []byte { return []byte{0xC1, 0x04, 0x00, 0x01} }

// ServerNames builds the WizardTeam custom C2 F3 EA packet (FUN_00403870 /
// FUN_00405d00): count u8, then {code u16 LE, name[32]} for every alive
// server, visible or not. Empty when the auth gate is closed.
func (r *Registry) ServerNames() []byte {
	servers, open := r.snapshot()
	body := []byte{subServerNames, 0}
	count := 0
	if open {
		for _, s := range servers {
			if !s.Alive {
				continue
			}
			var e [2 + serverNameField]byte
			binary.LittleEndian.PutUint16(e[0:], uint16(s.Code))
			copy(e[2:2+serverNameField-1], s.Name)
			body = append(body, e[:]...)
			count++
		}
	}
	body[1] = byte(count)
	return frame.Build(frame.C2, headCustom, body...)
}

// ServerList builds C2 F4 06 (FUN_00403740 / FUN_00405c40): count u16 BE,
// then {code u16 LE, load u8, 0xCC} for every visible and alive server.
func (r *Registry) ServerList() []byte {
	servers, open := r.snapshot()
	body := []byte{subServerList, 0, 0}
	count := 0
	if open {
		for _, s := range servers {
			if !s.Visible || !s.Alive {
				continue
			}
			body = append(body, byte(s.Code), byte(s.Code>>8), s.Load(), 0xCC)
			count++
		}
	}
	binary.BigEndian.PutUint16(body[1:], uint16(count))
	return frame.Build(frame.C2, headServer, body...)
}

// ServerInfo builds C1 16 F4 03 ip[16] port u16 LE for the requested code
// (FUN_004035f0). ok is false when the original sends nothing: auth gate
// closed, unknown code, invisible or down server.
func (r *Registry) ServerInfo(code byte) (p []byte, ok bool) {
	s, found, open := r.lookup(int(code))
	if !open || !found || !s.Visible || !s.Alive {
		return nil, false
	}
	body := make([]byte, 1+16+2)
	body[0] = subServerInfo
	copy(body[1:16], s.Address) // char[16], NUL-terminated
	binary.LittleEndian.PutUint16(body[17:], uint16(s.Port))
	return frame.Build(frame.C1, headServer, body...), true
}

// Handle answers one client packet. Only head F4 with sub 03/06 is handled;
// the sub is read at packet[3] whatever the packet type, exactly like the
// original, so C2 requests are effectively ignored.
func (r *Registry) Handle(p []byte) []byte {
	if frame.Head(p) != headServer || len(p) < 4 {
		return nil
	}
	switch p[3] {
	case subServerList:
		return r.ServerList()
	case subServerInfo:
		if len(p) < 5 {
			return nil
		}
		if resp, ok := r.ServerInfo(p[4]); ok {
			return resp
		}
	}
	return nil
}
