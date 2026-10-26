package legacy

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/config/gsconfig"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/config/servercfg"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/config/tomlconf"
)

// Instances maps the original game server folders to their file under
// config/gameserver.
var Instances = map[string]string{
	"4.GameServer":   "gameserver",
	"5.GameServerCS": "gameserver-cs",
}

// Server builds config/server.toml from root/1.ConnectServer
// (ConnectServer.ini, ServerList.dat, BlackList.txt),
// root/3.JoinServer/JoinServer.ini and root/2.DataServer/BadSyntax.txt.
func Server(root string) ([]byte, error) {
	cs := filepath.Join(root, "1.ConnectServer")
	var out bytes.Buffer
	out.WriteString(header("1.ConnectServer/ConnectServer.ini", "1.ConnectServer/BlackList.txt",
		"1.ConnectServer/ServerList.dat", "3.JoinServer/JoinServer.ini", "2.DataServer/BadSyntax.txt"))
	out.WriteString("\n# ConnectServer.ini [ConnectServerInfo]\n")
	if err := convertINI(&out, filepath.Join(cs, "ConnectServer.ini"), map[string]string{"ConnectServerInfo": "ConnectServer"}); err != nil {
		return nil, err
	}

	lines, err := File(filepath.Join(cs, "BlackList.txt"))
	if err != nil {
		return nil, err
	}
	sl, err := File(filepath.Join(cs, "ServerList.dat"))
	if err != nil {
		return nil, err
	}
	rows, err := Table[servercfg.ServerRow](sl, false)
	if err != nil {
		return nil, fmt.Errorf("ServerList.dat: %w", err)
	}
	lists, err := tomlconf.Marshal(struct {
		BlackList  []string              `comment:"BlackList.txt: client addresses the ConnectServer refuses"`
		ServerList []servercfg.ServerRow `toml:",inline" comment:"ServerList.dat\nServerCode: 0~3 Gold-NoPvP | 4 Gold-PvP | 5~6 No-PvP | 7~18 PvP\nServerName: max 20 characters; Visible: 0 hides the server"`
	}{FirstTokens(lines), rows})
	if err != nil {
		return nil, err
	}
	out.WriteString("\n")
	out.Write(lists)

	out.WriteString("\n# JoinServer.ini [JoinServerInfo]\n")
	if err := convertINI(&out, filepath.Join(root, "3.JoinServer", "JoinServer.ini"), map[string]string{"JoinServerInfo": "JoinServer"}); err != nil {
		return nil, err
	}

	bad, err := File(filepath.Join(root, "2.DataServer", "BadSyntax.txt"))
	if err != nil {
		return nil, err
	}
	type dataServer struct {
		BadSyntax []string `comment:"BadSyntax.txt: a new character name containing one of these is refused"`
	}
	ds, err := tomlconf.Marshal(struct{ DataServer dataServer }{dataServer{FirstTokens(bad)}})
	if err != nil {
		return nil, err
	}
	out.WriteString("\n")
	out.Write(ds)
	return out.Bytes(), nil
}

// GameServer builds config/gameserver/<instance>.toml from the eight
// "GameServerInfo - <name>.dat" files of an instance DATA folder; each file
// becomes the table [<name>].
func GameServer(dataDir string) ([]byte, error) {
	var out bytes.Buffer
	var srcs []string
	for _, name := range gsconfig.Files {
		srcs = append(srcs, "DATA/GameServerInfo - "+name+".dat")
	}
	out.WriteString(header(srcs...))
	for _, name := range gsconfig.Files {
		out.WriteString("\n# ==============================================================================\n")
		out.WriteString("# GameServerInfo - " + name + ".dat\n")
		out.WriteString("# ==============================================================================\n")
		path := filepath.Join(dataDir, "GameServerInfo - "+name+".dat")
		if err := convertINI(&out, path, map[string]string{"GameServerInfo": name}); err != nil {
			return nil, err
		}
	}
	return out.Bytes(), nil
}

func convertINI(out *bytes.Buffer, path string, sections map[string]string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	t, err := INIToTOML(b, sections)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	out.Write(bytes.TrimRight(t, "\n"))
	out.WriteString("\n")
	return nil
}
