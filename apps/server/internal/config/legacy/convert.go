package legacy

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/config/mutext"
)

// Output is one generated file under the config directory.
type Output struct {
	Group string // name accepted by configconv -only
	Path  string // relative to the config directory
	Build func(root string) ([]byte, error)
}

// dataOutputs are the config/data/*.toml files, registered by the files
// that convert each Data/ group.
var dataOutputs []Output

func register(group string, build func(dataDir string) ([]byte, error)) {
	dataOutputs = append(dataOutputs, Output{Group: group, Path: filepath.Join("data", group+".toml"),
		Build: func(root string) ([]byte, error) { return build(filepath.Join(root, "Data")) }})
}

// Outputs lists every file configconv writes, for an original MuServer
// folder layout (1.ConnectServer, 3.JoinServer, 4.GameServer/DATA,
// 5.GameServerCS/DATA, Data).
func Outputs() []Output {
	out := []Output{{Group: "server", Path: "server.toml", Build: Server}}
	dirs := make([]string, 0, len(Instances))
	for dir := range Instances {
		dirs = append(dirs, dir)
	}
	sort.Strings(dirs)
	for _, dir := range dirs {
		out = append(out, Output{Group: "gameserver", Path: filepath.Join("gameserver", Instances[dir]+".toml"),
			Build: func(root string) ([]byte, error) { return GameServer(filepath.Join(root, dir, "DATA")) }})
	}
	return append(out, dataOutputs...)
}

// Convert writes the outputs whose group is in only (all when only is
// empty) from the original folder root into the config directory dst.
func Convert(root, dst string, only map[string]bool) ([]string, error) {
	var written []string
	for _, o := range Outputs() {
		if len(only) > 0 && !only[o.Group] {
			continue
		}
		b, err := o.Build(root)
		if err != nil {
			return written, fmt.Errorf("%s: %w", o.Path, err)
		}
		path := filepath.Join(dst, o.Path)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return written, err
		}
		if err := os.WriteFile(path, b, 0o644); err != nil {
			return written, err
		}
		written = append(written, o.Path)
	}
	return written, nil
}

// step converts one original file (path relative to Data/) of a group.
type step struct {
	file string
	conv func([]mutext.Line) error
}

// runSteps tokenizes and converts each file; it returns the source list for
// the generated header.
func runSteps(dataDir string, steps []step) ([]string, error) {
	var srcs []string
	for _, st := range steps {
		srcs = append(srcs, "Data/"+st.file)
		lines, err := File(filepath.Join(dataDir, filepath.FromSlash(st.file)))
		if err != nil {
			return nil, err
		}
		if err := st.conv(lines); err != nil {
			return nil, fmt.Errorf("%s: %w", st.file, err)
		}
	}
	return srcs, nil
}
