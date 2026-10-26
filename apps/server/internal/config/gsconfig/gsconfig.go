// Package gsconfig loads the configuration of one game server instance,
// config/gameservers/<instance>.json: one section per original
// "GameServerInfo - <name>.dat" file of 4.GameServer/DATA or
// 5.GameServerCS/DATA, holding that file's [GameServerInfo] keys under
// their original names. The original reads each key with
// GetPrivateProfileIntA/StringA (CServerInfo loaders 0x4ecb40, 0x4eff70,
// 0x4eda70, 0x4eeee0, 0x4ecf70, ... — the offset/key dictionary is in
// .re/cfgmap.txt).
package gsconfig

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/config/ini"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/config/jsonconf"
)

const section = "GameServerInfo"

// Files are the eight original per-instance configuration files, the
// sections of the JSON file.
var Files = []string{"Common", "Character", "ChaosMix", "Command", "Custom", "Event", "Item", "Skill"}

// Config gives typed access to the instance configuration.
type Config struct {
	files map[string]*ini.File
	Path  string // the JSON file
	Name  string // the instance, the file name without ".json" ("4.GameServer")

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

// Load reads an instance file. A missing section is an error: the
// original refuses to start without one of its files.
func Load(path string) (*Config, error) {
	s, err := jsonconf.LoadSettings(path)
	if err != nil {
		return nil, err
	}
	return FromSettings(s, path)
}

// FromSettings builds the configuration of an instance file already read.
func FromSettings(s *jsonconf.Settings, path string) (*Config, error) {
	c := &Config{files: map[string]*ini.File{}, Path: path,
		Name: strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))}
	for _, name := range Files {
		if !s.Has(name) {
			return nil, fmt.Errorf("%s: missing section %q", path, name)
		}
		c.files[name] = s.SectionINI(name, section)
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

// Int reads a key from one of the files (GetPrivateProfileIntA semantics).
func (c *Config) Int(file, key string, def int) int {
	f := c.files[file]
	if f == nil {
		return def
	}
	return f.Int(section, key, def)
}

// String reads a key; size mirrors the destination buffer size of the
// original (0 = unlimited).
func (c *Config) String(file, key string, size int) string {
	f := c.files[file]
	if f == nil {
		return ""
	}
	return f.String(section, key, "", size)
}

// ALInt reads a "<key>_AL<level>" key, the per-account-level variant used
// throughout the configuration.
func (c *Config) ALInt(file, key string, accountLevel, def int) int {
	return c.Int(file, fmt.Sprintf("%s_AL%d", key, accountLevel), def)
}

// WriteKeys sets keys of one section in the instance file (the
// WritePrivateProfileStringA of the original). The loaded values do not
// change.
func (c *Config) WriteKeys(file string, keys map[string]string) error {
	s, err := jsonconf.LoadSettings(c.Path)
	if err != nil {
		return err
	}
	for k, v := range keys {
		s.Set(file, k, v)
	}
	return jsonconf.WriteSettings(c.Path, s)
}

// Exists reports whether path is an instance file.
func Exists(path string) bool {
	st, err := os.Stat(path)
	return err == nil && !st.IsDir()
}
