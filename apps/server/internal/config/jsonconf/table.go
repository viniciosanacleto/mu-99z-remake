// Package jsonconf reads and writes the JSON configuration under config/
// (docs/server-plan/11-arquivos-de-configuracao.md §0), which replaces the
// original .ini / .dat / .txt files. It keeps the original names (sections,
// keys, table columns) and gives the loaders the same tokens and key/value
// pairs the original files gave them, so the positional parsers of
// internal/gamedata and internal/game work unchanged:
//
//   - a Table is a tokenized data file (mutext): blocks of rows, each block
//     optionally opened by a section number and closed by "end"; a row is a
//     JSON object whose keys are the column names, in column order;
//   - Settings is an .ini file: sections of key/value pairs.
package jsonconf

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/text/encoding/charmap"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/config/mutext"
)

// Windows1252 is the Encoding of tables whose original file was not UTF-8
// (Portuguese texts of Message.txt, NpcTalk.txt, CustomQuiz.txt, ...). The
// JSON holds UTF-8; the loaders get the original bytes back.
const Windows1252 = "windows-1252"

// Cell is one column of a row.
type Cell struct {
	Name   string
	Value  string // the token as the original tokenizer returned it (UTF-8)
	Number bool   // written as a JSON number
}

// Row is one line of a table, in column order.
type Row []Cell

// Tokens returns the values of the row.
func (r Row) Tokens() []string {
	out := make([]string, len(r))
	for i, c := range r {
		out[i] = c.Value
	}
	return out
}

// Section is a block of rows. Index is the section number line that opens
// it, nil for a plain table; NoEnd marks a block that the original did not
// close with "end".
type Section struct {
	Index *int
	Rows  []Row
	NoEnd bool
}

// Table is a tokenized data file.
type Table struct {
	Encoding string
	Sections []Section
}

// Section returns the block opened by section number n, or nil.
func (t *Table) Section(n int) *Section {
	for i := range t.Sections {
		if s := &t.Sections[i]; s.Index != nil && *s.Index == n {
			return s
		}
	}
	return nil
}

// Lines gives the tokens the original file held, as mutext.Tokenize (or
// TokenizeScript) returned them: section numbers, rows and "end" lines.
// Line numbers count these lines from 1.
func (t *Table) Lines() ([]mutext.Line, error) {
	enc := func(s string) (string, error) { return s, nil }
	switch t.Encoding {
	case "":
	case Windows1252:
		e := charmap.Windows1252.NewEncoder()
		enc = func(s string) (string, error) {
			if isASCII(s) {
				return s, nil
			}
			return e.String(s)
		}
	default:
		return nil, fmt.Errorf("unknown encoding %q", t.Encoding)
	}
	var out []mutext.Line
	add := func(toks ...string) { out = append(out, mutext.Line{No: len(out) + 1, Tokens: toks}) }
	for _, s := range t.Sections {
		if s.Index != nil {
			add(strconv.Itoa(*s.Index))
		}
		for _, r := range s.Rows {
			if len(r) == 0 {
				continue
			}
			toks := r.Tokens()
			for i, v := range toks {
				var err error
				if toks[i], err = enc(v); err != nil {
					return nil, fmt.Errorf("row %v column %s: %w", toks, r[i].Name, err)
				}
			}
			add(toks...)
		}
		if !s.NoEnd {
			add("end")
		}
	}
	return out, nil
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return false
		}
	}
	return true
}

var canonicalInt = regexp.MustCompile(`^-?(0|[1-9][0-9]{0,17})$`)

// NewCell makes a cell, written as a JSON number when the token is an
// integer that reads back identically.
func NewCell(name, value string) Cell {
	return Cell{Name: name, Value: value, Number: canonicalInt.MatchString(value) && value != "-0"}
}

// --- JSON --------------------------------------------------------------------

type tableJSON struct {
	Encoding string        `json:"encoding,omitempty"`
	Rows     []Row         `json:"rows"`
	End      *bool         `json:"end,omitempty"`
	Sections []sectionJSON `json:"sections"`
}

type sectionJSON struct {
	Index *int  `json:"index"`
	Rows  []Row `json:"rows"`
	End   *bool `json:"end"`
}

// UnmarshalJSON reads {"rows": [...]} (one plain table) or
// {"sections": [{"index": n, "rows": [...]}, ...]}; "end": false marks a
// block without "end".
func (t *Table) UnmarshalJSON(b []byte) error {
	var j tableJSON
	if err := json.Unmarshal(b, &j); err != nil {
		return err
	}
	if j.Rows != nil && j.Sections != nil {
		return fmt.Errorf(`table has both "rows" and "sections"`)
	}
	*t = Table{Encoding: j.Encoding}
	if j.Sections == nil {
		t.Sections = []Section{{Rows: j.Rows, NoEnd: j.End != nil && !*j.End}}
		return nil
	}
	for _, s := range j.Sections {
		t.Sections = append(t.Sections, Section{Index: s.Index, Rows: s.Rows, NoEnd: s.End != nil && !*s.End})
	}
	return nil
}

// MarshalJSON writes the table with one row per line.
func (t *Table) MarshalJSON() ([]byte, error) {
	var b bytes.Buffer
	writeTable(&b, t, "")
	return b.Bytes(), nil
}

func writeTable(b *bytes.Buffer, t *Table, ind string) {
	b.WriteString("{")
	if t.Encoding != "" {
		fmt.Fprintf(b, "\n%s  \"encoding\": %s,", ind, quote(t.Encoding))
	}
	if len(t.Sections) == 1 && t.Sections[0].Index == nil {
		s := t.Sections[0]
		fmt.Fprintf(b, "\n%s  \"rows\": ", ind)
		writeRows(b, s.Rows, ind+"  ")
		if s.NoEnd {
			b.WriteString(",\n" + ind + `  "end": false`)
		}
		b.WriteString("\n" + ind + "}")
		return
	}
	fmt.Fprintf(b, "\n%s  \"sections\": [", ind)
	for i, s := range t.Sections {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString("\n" + ind + "    {")
		if s.Index != nil {
			fmt.Fprintf(b, "\"index\": %d, ", *s.Index)
		}
		b.WriteString(`"rows": `)
		writeRows(b, s.Rows, ind+"    ")
		if s.NoEnd {
			b.WriteString(`, "end": false`)
		}
		b.WriteString("}")
	}
	b.WriteString("\n" + ind + "  ]\n" + ind + "}")
}

func writeRows(b *bytes.Buffer, rows []Row, ind string) {
	if len(rows) == 0 {
		b.WriteString("[]")
		return
	}
	b.WriteString("[")
	for i, r := range rows {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString("\n" + ind + "  ")
		writeObject(b, r)
	}
	b.WriteString("\n" + ind + "]")
}

// writeObject writes cells as a one-line JSON object.
func writeObject(b *bytes.Buffer, cells []Cell) {
	b.WriteString("{")
	for i, c := range cells {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(quote(c.Name) + ": " + cellValue(c))
	}
	b.WriteString("}")
}

func cellValue(c Cell) string {
	if c.Number {
		return c.Value
	}
	return quote(c.Value)
}

func quote(s string) string {
	var b bytes.Buffer
	e := json.NewEncoder(&b)
	e.SetEscapeHTML(false)
	_ = e.Encode(s)
	return strings.TrimSuffix(b.String(), "\n")
}

// UnmarshalJSON reads a row object keeping the key order, which is the
// column order.
func (r *Row) UnmarshalJSON(b []byte) error {
	cells, err := decodeObject(b)
	if err != nil {
		return err
	}
	*r = cells
	return nil
}

// MarshalJSON writes the row as a one-line object.
func (r Row) MarshalJSON() ([]byte, error) {
	var b bytes.Buffer
	writeObject(&b, r)
	return b.Bytes(), nil
}

// decodeObject reads a flat JSON object of strings and numbers in order.
func decodeObject(b []byte) ([]Cell, error) {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	if tok, err := dec.Token(); err != nil || tok != json.Delim('{') {
		return nil, fmt.Errorf("expected an object, got %s", bytes.TrimSpace(b))
	}
	var cells []Cell
	seen := map[string]bool{}
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		name := tok.(string)
		if seen[name] {
			return nil, fmt.Errorf("duplicate key %q", name)
		}
		seen[name] = true
		if tok, err = dec.Token(); err != nil {
			return nil, err
		}
		switch v := tok.(type) {
		case json.Number:
			cells = append(cells, Cell{Name: name, Value: v.String(), Number: true})
		case string:
			cells = append(cells, Cell{Name: name, Value: v})
		default:
			return nil, fmt.Errorf("key %q: value must be a string or a number", name)
		}
	}
	if _, err := dec.Token(); err != nil {
		return nil, err
	}
	return cells, nil
}

// --- files -------------------------------------------------------------------

// LoadTable reads a table file.
func LoadTable(path string) (*Table, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	t := &Table{}
	if err := json.Unmarshal(data, t); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return t, nil
}

// LoadLines reads a table file and returns its tokens, the drop-in
// replacement of mutext.LoadFile for the converted files.
func LoadLines(path string) ([]mutext.Line, error) {
	t, err := LoadTable(path)
	if err != nil {
		return nil, err
	}
	lines, err := t.Lines()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return lines, nil
}

// WriteTable writes t to path through a temporary file.
func WriteTable(path string, t *Table) error {
	b, _ := t.MarshalJSON()
	return writeFile(path, append(b, '\n'))
}

func writeFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// ToWindows1252 converts JSON text to the bytes the client expects; runes
// without a Windows-1252 byte become '?'.
func ToWindows1252(s string) string {
	if isASCII(s) {
		return s
	}
	out := make([]byte, 0, len(s))
	e := charmap.Windows1252
	for _, r := range s {
		b, ok := e.EncodeRune(r)
		if !ok {
			b = '?'
		}
		out = append(out, b)
	}
	return string(out)
}

// FromWindows1252 converts client bytes to JSON text.
func FromWindows1252(s string) string {
	if isASCII(s) {
		return s
	}
	u, _ := charmap.Windows1252.NewDecoder().String(s)
	return u
}
