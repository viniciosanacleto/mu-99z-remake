// Package servercfg reads config/server.toml: the settings of the original
// ConnectServer (ConnectServer.ini, ServerList.dat, BlackList.txt),
// JoinServer (JoinServer.ini) and DataServer (BadSyntax.txt). Keys keep
// their original names; the comments in the file come from the original
// files (cmd/configconv).
package servercfg

import "github.com/viniciosanacleto/mu-remake/apps/server/internal/config/tomlconf"

// File is config/server.toml.
type File struct {
	ConnectServer ConnectServer `toml:"ConnectServer"`
	JoinServer    JoinServer    `toml:"JoinServer"`
	DataServer    DataServer    `toml:"DataServer"`
}

// ConnectServer is [ConnectServerInfo] of ConnectServer.ini plus the two
// lists the ConnectServer loads (docs/server-plan/03-connect-server.md).
type ConnectServer struct {
	ConnectServerPortTCP int
	ConnectServerPortUDP int         // JoinServer/GameServer status channel; unused by the single service
	MaxConnectionIdle    int         // seconds
	MaxConnectionPerIP   int         // 0 = unlimited
	MaxPacketPerSecond   int         // 0 = unlimited
	BlackList            []string    // BlackList.txt: refused client addresses
	ServerList           []ServerRow `toml:",inline"` // ServerList.dat
}

// ServerRow is a ServerList.dat row.
type ServerRow struct {
	ServerCode int
	ServerName string // up to 20 characters
	IpAddress  string
	Port       int
	Visible    int // 0 hides the server from the list
}

// JoinServer is [JoinServerInfo] of JoinServer.ini.
type JoinServer struct {
	JoinServerODBC       string // unused: PostgreSQL replaces the ODBC source
	JoinServerPort       int    // unused: the JoinServer is an internal module
	ConnectServerAddress string // unused
	ConnectServerPort    int    // unused
	MaxConnectionPerIP   int
	MaxConnectionPerHID  int
	MD5Encryption        int // unused: passwords are argon2id
}

// DataServer holds the DataServer lists.
type DataServer struct {
	BadSyntax []string // BadSyntax.txt: character names containing one of these are refused
}

// Defaults are the GetPrivateProfileIntA defaults of the original loaders,
// used for keys missing from the file.
func Defaults() File {
	return File{ConnectServer: ConnectServer{ConnectServerPortTCP: 44405, MaxConnectionIdle: 60}}
}

// Load reads config/server.toml.
func Load(path string) (*File, error) {
	f := Defaults()
	if err := tomlconf.Load(path, &f); err != nil {
		return nil, err
	}
	return &f, nil
}
