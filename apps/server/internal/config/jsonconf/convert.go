package jsonconf

import (
	"bufio"
	"bytes"
	"fmt"
	"path"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/encoding/charmap"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/config/ini"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/config/mutext"
)

// Tokenizer splits an original data file into lines of tokens.
type Tokenizer func([]byte) ([]mutext.Line, error)

// Hints help name the columns of a file.
type Hints struct {
	// Rename gives the columns of header comments that do not split into
	// one word per column. The key is the header words joined by one space
	// ("ACTIVE SYNTAX PLAYER PATENTE").
	Rename map[string][]string
	// Headers name rows that have no header comment of their width.
	Headers [][]string
}

var identifier = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// TableFromText converts an original data file. Column names come from the
// header comments ("// Index   Type   Name ...", a line made only of
// identifiers). A row takes, in order: the last header above it with as
// many words as the row has tokens, the header in force or an earlier
// header (or hints.Headers) of that width, the
// last header above it with more words, or the header in force. Columns
// without a header name are "ColN" (1-based). The result is checked to give
// back exactly the tokens of tok(data).
func TableFromText(data []byte, tok Tokenizer, hints Hints) (*Table, error) {
	want, err := tok(data)
	if err != nil {
		return nil, err
	}
	t := &Table{}
	decode := func(s string) string { return s }
	if !utf8.Valid(data) {
		t.Encoding = Windows1252
		d := charmap.Windows1252.NewDecoder()
		decode = func(s string) string {
			if isASCII(s) {
				return s
			}
			u, _ := d.String(s) // every byte maps to a rune
			return u
		}
	}

	var header []string
	var candidates [][]string
	seen := append([][]string(nil), hints.Headers...)
	var cur *Section
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		lines, err := tok([]byte(line))
		if err != nil {
			return nil, err
		}
		if len(lines) == 0 {
			if words := headerWords(line, hints.Rename); words != nil {
				candidates = append(candidates, words)
			}
			continue
		}
		toks := lines[0].Tokens
		if cur == nil {
			if n, err := strconv.Atoi(toks[0]); len(toks) == 1 && err == nil && strconv.Itoa(n) == toks[0] {
				t.Sections = append(t.Sections, Section{Index: &n})
				cur = &t.Sections[len(t.Sections)-1]
				continue
			}
			t.Sections = append(t.Sections, Section{})
			cur = &t.Sections[len(t.Sections)-1]
		}
		if len(toks) == 1 && strings.EqualFold(toks[0], "end") {
			cur = nil
			continue
		}
		header, seen = pickHeader(header, seen, candidates, len(toks))
		candidates = nil
		row := make(Row, len(toks))
		used := map[string]int{}
		for i, v := range toks {
			name := fmt.Sprintf("Col%d", i+1)
			if i < len(header) {
				name = header[i]
			}
			if used[name]++; used[name] > 1 {
				name = fmt.Sprintf("%s_%d", name, used[name])
			}
			row[i] = NewCell(name, decode(v))
		}
		cur.Rows = append(cur.Rows, row)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if cur != nil {
		cur.NoEnd = true
	}
	if t.Encoding != "" && !hasNonASCII(t) {
		t.Encoding = "" // only the comments were not UTF-8
	}

	got, err := t.Lines()
	if err != nil {
		return nil, err
	}
	if !sameTokens(got, want) {
		return nil, fmt.Errorf("conversion does not give back the original tokens")
	}
	return t, nil
}

func hasNonASCII(t *Table) bool {
	for _, s := range t.Sections {
		for _, r := range s.Rows {
			for _, c := range r {
				if !isASCII(c.Value) {
					return true
				}
			}
		}
	}
	return false
}

// headerWords returns the column names of a header comment line, or nil.
func headerWords(line string, rename map[string][]string) []string {
	s := strings.TrimSpace(line)
	switch {
	case strings.HasPrefix(s, "//"):
		s = strings.TrimLeft(s, "/")
	case strings.HasPrefix(s, ";"):
		s = strings.TrimLeft(s, ";")
	default:
		return nil
	}
	if i := strings.Index(s, "//"); i >= 0 {
		s = s[:i] // "Type   Index   Preco   // Nome do item"
	}
	words := strings.Fields(s)
	if len(words) > 0 && words[0] == "#" {
		words = words[1:]
	}
	if len(words) == 0 {
		return nil
	}
	if o, ok := rename[strings.Join(words, " ")]; ok {
		return o
	}
	for i, w := range words {
		w = strings.TrimPrefix(w, "#") // "#Index   #Year"
		if !identifier.MatchString(w) {
			return nil
		}
		words[i] = w
	}
	return words
}

// pickHeader chooses the names of a row of n tokens; seen collects the
// headers used so far.
func pickHeader(cur []string, seen, candidates [][]string, n int) ([]string, [][]string) {
	for i := len(candidates) - 1; i >= 0; i-- {
		if len(candidates[i]) == n {
			return candidates[i], append(seen, candidates[i])
		}
	}
	if len(cur) == n {
		return cur, seen
	}
	for i := len(seen) - 1; i >= 0; i-- {
		if len(seen[i]) == n {
			return seen[i], seen
		}
	}
	for i := len(candidates) - 1; i >= 0; i-- {
		if len(candidates[i]) > n {
			return candidates[i], append(seen, candidates[i])
		}
	}
	return cur, seen
}

// sameTokens compares token lines, ignoring line numbers and the case of
// "end" (the readers compare it case-insensitively).
func sameTokens(a, b []mutext.Line) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		x, y := a[i].Tokens, b[i].Tokens
		if len(x) == 1 && len(y) == 1 && strings.EqualFold(x[0], "end") && strings.EqualFold(y[0], "end") {
			continue
		}
		if !reflect.DeepEqual(x, y) {
			return false
		}
	}
	return true
}

// scriptFiles are the Data files read by the WizardTeam script reader.
var scriptFiles = []string{"Custom/Event/*.txt", "Custom/CustomPatente.txt"}

// TokenizerFor returns the tokenizer of a Data file (rel is its path under
// MuServer/Data, with '/' separators): mutext.TokenizeScript for the
// script files, mutext.Tokenize otherwise.
func TokenizerFor(rel string) Tokenizer {
	for _, p := range scriptFiles {
		if ok, _ := path.Match(p, rel); ok {
			return mutext.TokenizeScript
		}
	}
	return mutext.Tokenize
}

// IsINI reports whether the first line that is not blank or a ';' comment
// opens an [section].
func IsINI(data []byte) bool {
	for _, l := range strings.Split(string(data), "\n") {
		l = strings.TrimSpace(l)
		if l == "" || l[0] == ';' {
			continue
		}
		return l[0] == '[' && strings.IndexByte(l, ']') > 1
	}
	return false
}

// SettingsFromINI converts an .ini file, keeping the order and case of the
// first occurrence of every section and key. The result is checked to read
// back like ini.Parse(data).
func SettingsFromINI(data []byte) (*Settings, error) {
	if !utf8.Valid(data) {
		return nil, fmt.Errorf("ini file is not UTF-8")
	}
	s := &Settings{}
	var cur *SettingsSection
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || line[0] == ';' {
			continue
		}
		if line[0] == '[' {
			if end := strings.IndexByte(line, ']'); end > 0 {
				name := strings.TrimSpace(line[1:end])
				if cur = s.section(name); cur == nil {
					s.Sections = append(s.Sections, SettingsSection{Name: name})
					cur = &s.Sections[len(s.Sections)-1]
				}
			}
			continue
		}
		eq := strings.IndexByte(line, '=')
		if cur == nil || eq <= 0 {
			continue
		}
		key := strings.TrimSpace(line[:eq])
		if _, dup := s.Get(cur.Name, key); dup {
			continue
		}
		cur.Keys = append(cur.Keys, NewCell(key, iniValue(line[eq+1:])))
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	orig := ini.Parse(data)
	for _, sec := range s.Sections {
		for _, c := range sec.Keys {
			if v := orig.String(sec.Name, c.Name, "\x00", 0); v != c.Value {
				return nil, fmt.Errorf("[%s] %s: %q reads back as %q", sec.Name, c.Name, c.Value, v)
			}
		}
	}
	return s, nil
}

// iniValue trims a value and one pair of surrounding quotes, as ini.Parse.
func iniValue(v string) string {
	v = strings.TrimSpace(v)
	if len(v) >= 2 && (v[0] == '"' || v[0] == '\'') && v[len(v)-1] == v[0] {
		return v[1 : len(v)-1]
	}
	return v
}
