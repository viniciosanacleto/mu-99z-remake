// Package legacy converts the original MuServer configuration (.ini/.dat
// key=value files and the Data/**.txt tables) into the TOML files under
// apps/server/config. Only cmd/configconv uses it: the server itself reads
// TOML. The readers here keep the semantics of the original loaders
// (GetPrivateProfile*, the Data/ tokenizer), applied once at conversion.
package legacy

import (
	"bufio"
	"bytes"
	"fmt"
	"regexp"
	"strings"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/config/tomlconf"
)

// intValue matches the values written as TOML integers: a whole decimal
// integer, optionally followed by a "//" or ";" comment that is kept as a
// TOML comment. Anything else stays a string, read with the
// GetPrivateProfileIntA rules by gsconfig when used as a number.
var intValue = regexp.MustCompile(`^([+-]?[0-9]+)\s*((//|;).*)?$`)

// INIToTOML converts a Win32 profile file line by line, keeping its comments
// and blank lines. sections maps each original section name (any case) to
// its TOML table name; a section not in the map is an error. Keys are
// case-insensitive in the original and the first occurrence wins, so later
// duplicates are commented out.
func INIToTOML(src []byte, sections map[string]string) ([]byte, error) {
	names := map[string]string{}
	for k, v := range sections {
		names[strings.ToLower(k)] = v
	}
	var out bytes.Buffer
	seen := map[string]bool{}
	cur := ""
	sc := bufio.NewScanner(bytes.NewReader(src))
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for no := 1; sc.Scan(); no++ {
		line := tomlconf.UTF8(strings.TrimSpace(sc.Text()))
		switch {
		case line == "":
			out.WriteByte('\n')
		case line[0] == ';', strings.HasPrefix(line, "//"), line[0] == '#':
			out.WriteString(comment(line) + "\n")
		case line[0] == '[':
			end := strings.IndexByte(line, ']')
			if end < 0 {
				return nil, fmt.Errorf("line %d: bad section %q", no, line)
			}
			name, ok := names[strings.ToLower(strings.TrimSpace(line[1:end]))]
			if !ok {
				return nil, fmt.Errorf("line %d: unexpected section %q", no, line)
			}
			cur = name
			out.WriteString("[" + name + "]\n")
		default:
			eq := strings.IndexByte(line, '=')
			if eq <= 0 || cur == "" {
				// Windows ignores the line; keep it visible as a comment.
				out.WriteString(comment(line) + "\n")
				continue
			}
			key := strings.TrimSpace(line[:eq])
			id := cur + "." + strings.ToLower(key)
			if seen[id] {
				out.WriteString("# duplicate, ignored by the original: " + line + "\n")
				continue
			}
			seen[id] = true
			out.WriteString(tomlKey(key) + " = " + iniValue(strings.TrimSpace(line[eq+1:])) + "\n")
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// iniValue renders a profile value as TOML.
func iniValue(v string) string {
	if m := intValue.FindStringSubmatch(v); m != nil {
		n := strings.TrimPrefix(m[1], "+")
		if m[2] != "" {
			return n + " " + comment(m[2])
		}
		return n
	}
	if len(v) >= 2 && (v[0] == '"' || v[0] == '\'') && v[len(v)-1] == v[0] {
		v = v[1 : len(v)-1]
	}
	b, _ := tomlconf.Marshal(struct {
		V string `toml:"v"`
	}{v})
	return strings.TrimSuffix(strings.TrimPrefix(string(b), "v = "), "\n")
}

// comment turns a ";", "//" or "#" comment into a TOML comment.
func comment(c string) string {
	switch {
	case strings.HasPrefix(c, "//"):
		c = c[2:]
	case strings.HasPrefix(c, ";"), strings.HasPrefix(c, "#"):
		c = c[1:]
	}
	if c == "" {
		return "#"
	}
	return strings.TrimRight("# "+strings.TrimPrefix(c, " "), " ")
}

func tomlKey(k string) string {
	for i := 0; i < len(k); i++ {
		c := k[i]
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '-') {
			b, _ := tomlconf.Marshal(map[string]int{k: 0})
			return strings.TrimSuffix(string(b), " = 0\n")
		}
	}
	return k
}
