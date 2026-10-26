package legacy

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// The WizardTeam script reader of Data/Custom/Event and CustomPatente.txt.
// A missing cell reads as -1 for the integer fields and 0xFF for the byte
// fields (map, x, y).

// EventSection is one numbered section of a script file: rows of cells.
type EventSection struct {
	Index int
	Rows  [][]string
}

// LoadEventScript reads a WizardTeam script file (FUN_00409600 /
// FUN_00451390): numbered sections closed by "end", ';' and "//" comments,
// quoted strings.
func LoadEventScript(path string) ([]EventSection, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var out []EventSection
	var cur *EventSection
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for no := 1; sc.Scan(); no++ {
		toks, err := scriptTokens(sc.Text())
		if err != nil {
			return nil, fmt.Errorf("%s line %d: %w", path, no, err)
		}
		if len(toks) == 0 {
			continue
		}
		if cur == nil {
			n, err := strconv.Atoi(toks[0])
			if err != nil || len(toks) != 1 {
				return nil, fmt.Errorf("%s line %d: expected section number, got %q", path, no, toks)
			}
			out = append(out, EventSection{Index: n})
			cur = &out[len(out)-1]
			continue
		}
		if len(toks) == 1 && strings.EqualFold(toks[0], "end") {
			cur = nil
			continue
		}
		cur.Rows = append(cur.Rows, toks)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if cur != nil {
		return nil, fmt.Errorf("%s: section %d without end", path, cur.Index)
	}
	return out, nil
}

// scriptTokens splits a line: blanks separate cells, quotes keep spaces,
// ';' and "//" start a comment.
func scriptTokens(s string) ([]string, error) {
	var toks []string
	for i := 0; i < len(s); {
		c := s[i]
		switch {
		case c == ' ' || c == '\t' || c == '\r' || c == '\n':
			i++
		case c == ';' || (c == '/' && i+1 < len(s) && s[i+1] == '/'):
			return toks, nil
		case c == '"':
			end := strings.IndexByte(s[i+1:], '"')
			if end < 0 {
				return nil, fmt.Errorf("unterminated string")
			}
			toks = append(toks, s[i+1:i+1+end])
			i += end + 2
		default:
			j := i
			for j < len(s) && s[j] != ' ' && s[j] != '\t' && s[j] != '\r' && s[j] != '"' && s[j] != ';' {
				j++
			}
			toks = append(toks, s[i:j])
			i = j
		}
	}
	return toks, nil
}

// eventCells gives typed access to the cells of a section with the
// defaults of FUN_004524e0 (missing row or column).
type eventCells struct{ secs []EventSection }

func (c eventCells) section(n int) *EventSection {
	for i := range c.secs {
		if c.secs[i].Index == n {
			return &c.secs[i]
		}
	}
	return nil
}

func (c eventCells) str(sec, row, col int) (string, bool) {
	s := c.section(sec)
	if s == nil || row >= len(s.Rows) || col >= len(s.Rows[row]) {
		return "", false
	}
	return s.Rows[row][col], true
}

// int is atoi of a cell, -1 when it is missing.
func (c eventCells) int(sec, row, col int) int {
	v, ok := c.str(sec, row, col)
	if !ok {
		return -1
	}
	return cAtoi(v)
}

// byte is a byte cell (map, x, y), 0xFF when it is missing.
func (c eventCells) byte(sec, row, col int) int {
	v, ok := c.str(sec, row, col)
	if !ok {
		return 0xFF
	}
	return cAtoi(v) & 0xFF
}

// rows returns the rows of a section as ints (atoi; a missing cell -1).
func (c eventCells) rows(sec, cols int) [][]int {
	s := c.section(sec)
	if s == nil {
		return nil
	}
	out := make([][]int, len(s.Rows))
	for i, r := range s.Rows {
		v := make([]int, cols)
		for k := range v {
			v[k] = -1
			if k < len(r) {
				v[k] = cAtoi(r[k])
			}
		}
		out[i] = v
	}
	return out
}

// cAtoi is the C atoi (leading blanks, sign, digits).
func cAtoi(s string) int {
	s = strings.TrimLeft(s, " \t")
	neg := false
	if s != "" && (s[0] == '-' || s[0] == '+') {
		neg = s[0] == '-'
		s = s[1:]
	}
	n := 0
	for i := 0; i < len(s) && s[i] >= '0' && s[i] <= '9'; i++ {
		n = n*10 + int(s[i]-'0')
	}
	if neg {
		return -n
	}
	return n
}
