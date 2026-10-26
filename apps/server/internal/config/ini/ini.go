// Package ini reads the original .ini/.dat configuration files with the same
// semantics as the Win32 GetPrivateProfileStringA / GetPrivateProfileIntA
// calls the original servers use: case-insensitive sections and keys, values
// trimmed of blanks and of one pair of surrounding quotes, ';' comment lines,
// first occurrence wins.
package ini

import (
	"bufio"
	"bytes"
	"os"
	"strings"
)

// File is a parsed configuration file.
type File struct {
	sections map[string]map[string]string
}

// Load reads and parses path.
func Load(path string) (*File, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(data), nil
}

// Parse parses file contents. It never fails: lines that are not sections or
// key=value pairs are ignored, as Windows does.
func Parse(data []byte) *File {
	f := &File{sections: map[string]map[string]string{}}
	var cur map[string]string
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || line[0] == ';' {
			continue
		}
		if line[0] == '[' {
			if end := strings.IndexByte(line, ']'); end > 0 {
				name := strings.ToLower(strings.TrimSpace(line[1:end]))
				if f.sections[name] == nil {
					f.sections[name] = map[string]string{}
				}
				cur = f.sections[name]
			}
			continue
		}
		eq := strings.IndexByte(line, '=')
		if cur == nil || eq <= 0 {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(line[:eq]))
		if _, dup := cur[key]; dup {
			continue
		}
		cur[key] = unquote(strings.TrimSpace(line[eq+1:]))
	}
	return f
}

// FromSections builds a File from section -> key -> value maps, as the
// JSON configuration holds them (values already unquoted). Names are
// case-insensitive, like Parse.
func FromSections(sections map[string]map[string]string) *File {
	f := &File{sections: map[string]map[string]string{}}
	for name, keys := range sections {
		sec := f.sections[strings.ToLower(name)]
		if sec == nil {
			sec = map[string]string{}
			f.sections[strings.ToLower(name)] = sec
		}
		for k, v := range keys {
			if _, dup := sec[strings.ToLower(k)]; !dup {
				sec[strings.ToLower(k)] = v
			}
		}
	}
	return f
}

func unquote(v string) string {
	if len(v) >= 2 && (v[0] == '"' || v[0] == '\'') && v[len(v)-1] == v[0] {
		return v[1 : len(v)-1]
	}
	return v
}

// String mirrors GetPrivateProfileStringA: def when the key is missing, and
// the value truncated to size-1 bytes when size > 0 (the original buffers are
// fixed-size, e.g. 0x20 for ServerName).
func (f *File) String(section, key, def string, size int) string {
	v, ok := f.lookup(section, key)
	if !ok {
		v = def
	}
	if size > 0 && len(v) > size-1 {
		v = v[:size-1]
	}
	return v
}

// Int mirrors GetPrivateProfileIntA: leading decimal digits with an optional
// sign are parsed and the rest of the value is ignored ("10// x" is 10);
// def when the key is missing, 0 when there are no digits.
func (f *File) Int(section, key string, def int) int {
	v, ok := f.lookup(section, key)
	if !ok {
		return def
	}
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

// Has reports whether the key exists.
func (f *File) Has(section, key string) bool {
	_, ok := f.lookup(section, key)
	return ok
}

func (f *File) lookup(section, key string) (string, bool) {
	s := f.sections[strings.ToLower(section)]
	if s == nil {
		return "", false
	}
	v, ok := s[strings.ToLower(key)]
	return v, ok
}
