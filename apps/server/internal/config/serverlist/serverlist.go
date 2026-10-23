// Package serverlist loads the ConnectServer ServerList.dat
// (docs/server-plan/03-connect-server.md §1):
//
//	ServerCode   ServerName   IpAddress   Port   Visible
package serverlist

import (
	"fmt"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/config/mutext"
)

// Entry is one game server shown to clients.
type Entry struct {
	Code    int
	Name    string // up to 20 characters per the file header; sent in a 32-byte field
	Address string
	Port    int
	Visible bool
}

// Load reads ServerList.dat.
func Load(path string) ([]Entry, error) {
	lines, err := mutext.LoadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(lines)
}

// Parse converts tokenized lines into entries.
func Parse(lines []mutext.Line) ([]Entry, error) {
	var out []Entry
	seen := map[int]bool{}
	for _, l := range mutext.Table(lines) {
		if len(l.Tokens) != 5 {
			return nil, fmt.Errorf("line %d: expected 5 columns, got %d", l.No, len(l.Tokens))
		}
		code, err := l.Int(0)
		if err != nil {
			return nil, err
		}
		port, err := l.Int(3)
		if err != nil {
			return nil, err
		}
		visible, err := l.Int(4)
		if err != nil {
			return nil, err
		}
		if code < 0 || code > 0xFF {
			// F4 03 carries the code as a single byte (ConnectServer FUN_004035f0).
			return nil, fmt.Errorf("line %d: server code %d out of range 0..255", l.No, code)
		}
		if port < 1 || port > 0xFFFF {
			return nil, fmt.Errorf("line %d: port %d out of range", l.No, port)
		}
		if seen[code] {
			return nil, fmt.Errorf("line %d: duplicate server code %d", l.No, code)
		}
		seen[code] = true
		out = append(out, Entry{
			Code:    code,
			Name:    l.Tokens[1],
			Address: l.Tokens[2],
			Port:    port,
			Visible: visible != 0,
		})
	}
	return out, nil
}
