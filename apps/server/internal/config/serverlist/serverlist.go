// Package serverlist builds the ConnectServer server list from the
// ServerList rows of config/server.toml (ServerList.dat of the original,
// docs/server-plan/03-connect-server.md §1):
//
//	ServerCode   ServerName   IpAddress   Port   Visible
package serverlist

import (
	"fmt"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/config/servercfg"
)

// Entry is one game server shown to clients.
type Entry struct {
	Code    int
	Name    string // up to 20 characters per the file header; sent in a 32-byte field
	Address string
	Port    int
	Visible bool
}

// Build validates the configured rows and converts them into entries.
func Build(rows []servercfg.ServerRow) ([]Entry, error) {
	var out []Entry
	seen := map[int]bool{}
	for i, r := range rows {
		if r.ServerCode < 0 || r.ServerCode > 0xFF {
			// F4 03 carries the code as a single byte (ConnectServer FUN_004035f0).
			return nil, fmt.Errorf("ServerList row %d: server code %d out of range 0..255", i+1, r.ServerCode)
		}
		if r.Port < 1 || r.Port > 0xFFFF {
			return nil, fmt.Errorf("ServerList row %d: port %d out of range", i+1, r.Port)
		}
		if seen[r.ServerCode] {
			return nil, fmt.Errorf("ServerList row %d: duplicate server code %d", i+1, r.ServerCode)
		}
		seen[r.ServerCode] = true
		out = append(out, Entry{
			Code:    r.ServerCode,
			Name:    r.ServerName,
			Address: r.IpAddress,
			Port:    r.Port,
			Visible: r.Visible != 0,
		})
	}
	return out, nil
}
