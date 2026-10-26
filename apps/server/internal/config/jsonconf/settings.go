package jsonconf

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/config/ini"
)

// SettingsSection is one [section] of an .ini file, keys in file order.
type SettingsSection struct {
	Name string
	Keys []Cell
}

// Settings is an .ini file: {"Section": {"Key": value, ...}, ...}.
type Settings struct {
	Sections []SettingsSection
}

// Get returns the value of a key (case-insensitive, like the ini reader).
func (s *Settings) Get(section, key string) (string, bool) {
	if sec := s.section(section); sec != nil {
		for _, c := range sec.Keys {
			if strings.EqualFold(c.Name, key) {
				return c.Value, true
			}
		}
	}
	return "", false
}

// Has reports whether a section exists.
func (s *Settings) Has(section string) bool { return s.section(section) != nil }

// Set changes a key, adding the key (and the section) when missing — the
// WritePrivateProfileStringA of the original.
func (s *Settings) Set(section, key, value string) {
	sec := s.section(section)
	if sec == nil {
		s.Sections = append(s.Sections, SettingsSection{Name: section})
		sec = &s.Sections[len(s.Sections)-1]
	}
	for i, c := range sec.Keys {
		if strings.EqualFold(c.Name, key) {
			sec.Keys[i] = NewCell(c.Name, value)
			return
		}
	}
	sec.Keys = append(sec.Keys, NewCell(key, value))
}

func (s *Settings) section(name string) *SettingsSection {
	for i := range s.Sections {
		if strings.EqualFold(s.Sections[i].Name, name) {
			return &s.Sections[i]
		}
	}
	return nil
}

// INI gives the settings the GetPrivateProfileIntA / StringA semantics of
// the ini package.
func (s *Settings) INI() *ini.File {
	m := map[string]map[string]string{}
	for _, sec := range s.Sections {
		keys := m[sec.Name]
		if keys == nil {
			keys = map[string]string{}
			m[sec.Name] = keys
		}
		for _, c := range sec.Keys {
			keys[c.Name] = c.Value
		}
	}
	return ini.FromSections(m)
}

// SectionINI returns one section as the [name] section of an ini.File,
// for the files that the original kept as one file per section (the
// GameServerInfo - <name>.dat files, all in [GameServerInfo]).
func (s *Settings) SectionINI(name, as string) *ini.File {
	keys := map[string]string{}
	if sec := s.section(name); sec != nil {
		for _, c := range sec.Keys {
			keys[c.Name] = c.Value
		}
	}
	return ini.FromSections(map[string]map[string]string{as: keys})
}

// UnmarshalJSON reads an object of objects, keeping the order.
func (s *Settings) UnmarshalJSON(b []byte) error {
	dec := json.NewDecoder(bytes.NewReader(b))
	if tok, err := dec.Token(); err != nil || tok != json.Delim('{') {
		return fmt.Errorf("settings: expected an object")
	}
	*s = Settings{}
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return err
		}
		name := tok.(string)
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return err
		}
		keys, err := decodeObject(raw)
		if err != nil {
			return fmt.Errorf("section %q: %w", name, err)
		}
		if s.section(name) != nil {
			return fmt.Errorf("duplicate section %q", name)
		}
		s.Sections = append(s.Sections, SettingsSection{Name: name, Keys: keys})
	}
	_, err := dec.Token()
	return err
}

// MarshalJSON writes one key per line.
func (s *Settings) MarshalJSON() ([]byte, error) {
	var b bytes.Buffer
	writeSettings(&b, s, "")
	return b.Bytes(), nil
}

func writeSettings(b *bytes.Buffer, s *Settings, ind string) {
	if len(s.Sections) == 0 {
		b.WriteString("{}")
		return
	}
	b.WriteString("{")
	for i, sec := range s.Sections {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString("\n" + ind + "  " + quote(sec.Name) + ": {")
		for j, c := range sec.Keys {
			if j > 0 {
				b.WriteString(",")
			}
			b.WriteString("\n" + ind + "    " + quote(c.Name) + ": " + cellValue(c))
		}
		if len(sec.Keys) > 0 {
			b.WriteString("\n" + ind + "  ")
		}
		b.WriteString("}")
	}
	b.WriteString("\n" + ind + "}")
}

// LoadSettings reads a settings file.
func LoadSettings(path string) (*Settings, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	s := &Settings{}
	if err := json.Unmarshal(data, s); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return s, nil
}

// LoadINI reads a settings file as an ini.File, the drop-in replacement of
// ini.Load for the converted files.
func LoadINI(path string) (*ini.File, error) {
	s, err := LoadSettings(path)
	if err != nil {
		return nil, err
	}
	return s.INI(), nil
}

// WriteSettings writes s to path through a temporary file.
func WriteSettings(path string, s *Settings) error {
	b, _ := s.MarshalJSON()
	return writeFile(path, append(b, '\n'))
}
