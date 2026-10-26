package gamedata

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/config/tomlconf"
)

// The game data lives in config/data/<group>.toml, one file per directory of
// the original Data/ folder (docs/server-plan/11-arquivos-de-configuracao.md).
// Each original file is a table named after it; its rows keep the original
// column names. The loaders take the config directory (apps/server/config)
// and build the same structures the original loaders filled.

// loadConfig decodes config/data/<group>.toml. Every loader of a group
// reads the same file, so the decoded value is cached while the file keeps
// its size and modification time; callers must treat it as read-only and
// copy the slices they keep.
func loadConfig[T any](configDir, group string) (*T, error) {
	path := filepath.Join(configDir, "data", group+".toml")
	st, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	key := configKey{path: path, typ: reflect.TypeFor[T](), size: st.Size(), mod: st.ModTime().UnixNano()}
	configCache.Lock()
	v, ok := configCache.m[key]
	configCache.Unlock()
	if ok {
		return v.(*T), nil
	}
	f := new(T)
	if err := tomlconf.Load(path, f); err != nil {
		return nil, err
	}
	configCache.Lock()
	for k := range configCache.m {
		if k.path == path && k.typ == key.typ {
			delete(configCache.m, k) // an older version of the file
		}
	}
	configCache.m[key] = f
	configCache.Unlock()
	return f, nil
}

type configKey struct {
	path string
	typ  reflect.Type
	size int64
	mod  int64 // modification time, ns
}

var configCache = struct {
	sync.Mutex
	m map[configKey]any
}{m: map[configKey]any{}}

// SplitItemRef splits a "cc,iii" item reference into its two numbers,
// without range checks (the tables the original reads as two numbers).
func SplitItemRef(s string) (cat, idx int, err error) {
	c, i, ok := strings.Cut(s, ",")
	if !ok {
		return 0, 0, fmt.Errorf("item reference %q: want cc,iii", s)
	}
	if cat, err = strconv.Atoi(strings.TrimSpace(c)); err != nil {
		return 0, 0, fmt.Errorf("item reference %q: %w", s, err)
	}
	if idx, err = strconv.Atoi(strings.TrimSpace(i)); err != nil {
		return 0, 0, fmt.Errorf("item reference %q: %w", s, err)
	}
	return cat, idx, nil
}

// FormatItemRef writes an item reference the way the original files do
// ("07,000").
func FormatItemRef(cat, idx int) string {
	if cat < 0 || idx < 0 {
		return strconv.Itoa(cat) + "," + strconv.Itoa(idx)
	}
	return fmt.Sprintf("%02d,%03d", cat, idx)
}
