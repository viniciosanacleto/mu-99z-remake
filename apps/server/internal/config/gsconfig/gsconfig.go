// Package gsconfig loads the configuration of one game server instance,
// config/gameserver/<instance>.toml. Each table is one of the original
// "GameServerInfo - <name>.dat" files of the instance DATA folder
// (4.GameServer, 5.GameServerCS), with its keys unchanged. The original
// reads each key with GetPrivateProfileIntA/StringA (CServerInfo loaders
// 0x4ecb40, 0x4eff70, 0x4eda70, 0x4eeee0, 0x4ecf70, ... — the offset/key
// dictionary is in .re/cfgmap.txt), so lookups stay case-insensitive and
// a number read from a string value follows the GetPrivateProfileIntA rules.
package gsconfig

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	toml "github.com/pelletier/go-toml/v2"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/config/tomlconf"
)

// Files are the tables of an instance file, one per original
// "GameServerInfo - <name>.dat".
var Files = []string{"Common", "Character", "ChaosMix", "Command", "Custom", "Event", "Item", "Skill"}

// Config gives typed access to the instance configuration.
type Config struct {
	tables map[string]map[string]any // lower-case table -> lower-case key -> int64 | string
	Path   string                    // the instance .toml file

	CustomerName       string
	ServerName         string // 0x20 buffer
	ServerCode         int
	ServerType         int
	ServerLock         int
	ServerPort         int
	ServerVersion      [5]byte // characters 0,2,3,5,6 of "1.00.00"
	ServerSerial       string  // 16 chars
	MaxUsers           int     // ServerMaxUserNumber (capped at 1000)
	ServerMinLevel     int
	ServerMaxLevel     int
	MaxConnectionPerIP int
	MaxConnectionPerHD int
	PersonalCodeCheck  bool
	ConnectMemberCheck bool
	CharCreateSwitch   bool
	CharDeleteSwitch   bool
	CharDeleteMaxLevel int
	MGCreateLevel      [4]int // per account level AL0..AL3
	DLCreateLevel      [4]int
}

// Load reads an instance file. Every table of Files must be present: the
// original refuses to start without any of its files.
func Load(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	c, err := Parse(b)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	c.Path = path
	return c, nil
}

// Parse decodes the contents of an instance file.
func Parse(data []byte) (*Config, error) {
	var raw map[string]any
	if err := toml.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	c := &Config{tables: map[string]map[string]any{}}
	for name, t := range raw {
		tab, ok := t.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%s: expected a table", name)
		}
		keys := map[string]any{}
		for k, v := range tab {
			switch v := v.(type) {
			case int64:
				keys[strings.ToLower(k)] = v
			case string:
				keys[strings.ToLower(k)] = tomlconf.ANSI(v)
			default:
				return nil, fmt.Errorf("%s.%s: expected an integer or a string, got %T", name, k, v)
			}
		}
		c.tables[strings.ToLower(name)] = keys
	}
	for _, name := range Files {
		if c.tables[strings.ToLower(name)] == nil {
			return nil, fmt.Errorf("missing table [%s]", name)
		}
	}
	c.fill()
	return c, nil
}

func (c *Config) fill() {
	c.CustomerName = c.String("Common", "CustomerName", 0x20)
	c.ServerName = c.String("Common", "ServerName", 0x20)
	c.ServerCode = c.Int("Common", "ServerCode", 0)
	c.ServerType = c.Int("Common", "ServerType", 0)
	c.ServerLock = c.Int("Common", "ServerLock", 0)
	c.ServerPort = c.Int("Common", "ServerPort", 0)
	v := c.String("Common", "ServerVersion", 0x100)
	for i, src := range []int{0, 2, 3, 5, 6} {
		if src < len(v) {
			c.ServerVersion[i] = v[src]
		}
	}
	c.ServerSerial = c.String("Common", "ServerSerial", 0x11)
	c.MaxUsers = min(c.Int("Common", "ServerMaxUserNumber", c.Int("Common", "limiteuser", 0x32)), 1000)
	c.ServerMinLevel = c.Int("Common", "ServerMinLevel", 0)
	c.ServerMaxLevel = c.Int("Common", "ServerMaxLevel", 0)
	c.MaxConnectionPerIP = c.Int("Common", "MaxConnectionPerIP", 0)
	c.MaxConnectionPerHD = c.Int("Common", "MaxConnectionPerHID", 0)
	c.PersonalCodeCheck = c.Int("Common", "PersonalCodeCheck", 0) != 0
	c.ConnectMemberCheck = c.Int("Common", "ConnectMemberCheck", 0) != 0
	c.CharCreateSwitch = c.Int("Common", "CharacterCreateSwitch", 0) != 0
	c.CharDeleteSwitch = c.Int("Common", "CharacterDeleteSwitch", 0) != 0
	c.CharDeleteMaxLevel = c.Int("Common", "CharacterDeleteMaxLevel", 0)
	for al := 0; al < 4; al++ {
		c.MGCreateLevel[al] = c.Int("Common", fmt.Sprintf("MGCreateLevel_AL%d", al), 0)
		c.DLCreateLevel[al] = c.Int("Common", fmt.Sprintf("DLCreateLevel_AL%d", al), 0)
	}
}

func (c *Config) lookup(table, key string) (any, bool) {
	t := c.tables[strings.ToLower(table)]
	if t == nil {
		return nil, false
	}
	v, ok := t[strings.ToLower(key)]
	return v, ok
}

// Int reads a key (GetPrivateProfileIntA semantics): def when missing, the
// leading digits of a string value.
func (c *Config) Int(table, key string, def int) int {
	v, ok := c.lookup(table, key)
	if !ok {
		return def
	}
	switch v := v.(type) {
	case int64:
		return int(v)
	case string:
		return ProfileInt(v)
	}
	return def
}

// String reads a key; size mirrors the destination buffer size of the
// original (0 = unlimited), the value is cut to size-1 bytes.
func (c *Config) String(table, key string, size int) string {
	v, ok := c.lookup(table, key)
	if !ok {
		return ""
	}
	var s string
	switch v := v.(type) {
	case int64:
		s = strconv.FormatInt(v, 10)
	case string:
		s = v
	}
	if size > 0 && len(s) > size-1 {
		s = s[:size-1]
	}
	return s
}

// ALInt reads a "<key>_AL<level>" key, the per-account-level variant used
// throughout the configuration.
func (c *Config) ALInt(table, key string, accountLevel, def int) int {
	return c.Int(table, fmt.Sprintf("%s_AL%d", key, accountLevel), def)
}

// ProfileInt parses a value like GetPrivateProfileIntA: leading decimal
// digits with an optional sign, the rest ignored ("10// x" is 10), 0 when
// there are no digits.
func ProfileInt(v string) int {
	v = strings.TrimSpace(v)
	neg := false
	i := 0
	if i < len(v) && (v[i] == '-' || v[i] == '+') {
		neg = v[i] == '-'
		i++
	}
	n := 0
	for ; i < len(v) && v[i] >= '0' && v[i] <= '9'; i++ {
		n = n*10 + int(v[i]-'0')
	}
	if neg {
		return -n
	}
	return n
}

// SetInt rewrites "key = <n>" inside [table] of the instance file on disk
// (the WritePrivateProfileStringA calls of the original), keeping the rest
// of the file — comments included — as it is, and updates the loaded value.
// A key missing from the table is appended to it.
func (c *Config) SetInt(table, key string, n int) error {
	if t := c.tables[strings.ToLower(table)]; t != nil {
		t[strings.ToLower(key)] = int64(n)
	}
	if c.Path == "" {
		return nil
	}
	b, err := os.ReadFile(c.Path)
	if err != nil {
		return err
	}
	return os.WriteFile(c.Path, setKey(b, table, key, strconv.Itoa(n)), 0o644)
}

// setKey replaces the value of key in [table], or inserts it at the end of
// the table.
func setKey(data []byte, table, key, value string) []byte {
	lines := strings.Split(string(data), "\n")
	in, last := false, -1
	for i, l := range lines {
		s := strings.TrimSpace(l)
		if strings.HasPrefix(s, "[") {
			if in {
				break
			}
			end := strings.IndexByte(s, ']')
			in = end > 0 && strings.EqualFold(strings.Trim(strings.TrimSpace(s[1:end]), `"`), table)
			if in {
				last = i
			}
			continue
		}
		if !in {
			continue
		}
		if s != "" && s[0] != '#' {
			last = i
		}
		k, _, ok := strings.Cut(s, "=")
		if ok && strings.EqualFold(strings.Trim(strings.TrimSpace(k), `"`), key) {
			cr := strings.HasSuffix(l, "\r")
			lines[i] = strings.TrimSpace(k) + " = " + value
			if cr {
				lines[i] += "\r"
			}
			return []byte(strings.Join(lines, "\n"))
		}
	}
	entry := key + " = " + value
	if last < 0 {
		lines = append(lines, "["+table+"]", entry)
	} else {
		lines = append(lines[:last+1], append([]string{entry}, lines[last+1:]...)...)
	}
	return []byte(strings.Join(lines, "\n"))
}
