// Package mutext tokenizes the original data files under Data/ and the
// ConnectServer .dat lists (docs/server-plan/11-arquivos-de-configuracao.md §1):
// "//" comments to end of line, double-quoted strings, blank-separated
// columns, "end" closing a table, and optional numbered sections.
package mutext

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Line is one non-empty line of tokens.
type Line struct {
	No     int      // 1-based line number in the file
	Tokens []string // quotes removed
}

// Section is a block of rows closed by "end". Index is the leading section
// number for sectioned files, or -1 for a plain table.
type Section struct {
	Index int
	Rows  []Line
}

// Tokenize splits data into lines of tokens, dropping comments and blank lines.
func Tokenize(data []byte) ([]Line, error) {
	var lines []Line
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for no := 1; sc.Scan(); no++ {
		toks, err := splitLine(sc.Text())
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", no, err)
		}
		if len(toks) > 0 {
			lines = append(lines, Line{No: no, Tokens: toks})
		}
	}
	return lines, sc.Err()
}

func splitLine(s string) ([]string, error) {
	var toks []string
	for i := 0; i < len(s); {
		c := s[i]
		switch {
		case c == ' ' || c == '\t' || c == '\r' || c == '\n' || c == 0xA0:
			i++
		case c == '/' && i+1 < len(s) && s[i+1] == '/':
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
			for j < len(s) && s[j] != ' ' && s[j] != '\t' && s[j] != '\r' && s[j] != '"' &&
				!(s[j] == '/' && j+1 < len(s) && s[j+1] == '/') {
				j++
			}
			toks = append(toks, s[i:j])
			i = j
		}
	}
	return toks, nil
}

// Table returns the rows of a plain table: every line up to the first "end".
func Table(lines []Line) []Line {
	for i, l := range lines {
		if isEnd(l) {
			return lines[:i]
		}
	}
	return lines
}

// Sections splits a sectioned file: a line holding only a number opens a
// section, which is closed by "end".
func Sections(lines []Line) ([]Section, error) {
	var out []Section
	for i := 0; i < len(lines); i++ {
		l := lines[i]
		if len(l.Tokens) != 1 {
			return nil, fmt.Errorf("line %d: expected section number, got %q", l.No, l.Tokens)
		}
		idx, err := strconv.Atoi(l.Tokens[0])
		if err != nil {
			return nil, fmt.Errorf("line %d: expected section number, got %q", l.No, l.Tokens[0])
		}
		sec := Section{Index: idx}
		for i++; i < len(lines) && !isEnd(lines[i]); i++ {
			sec.Rows = append(sec.Rows, lines[i])
		}
		if i == len(lines) {
			return nil, fmt.Errorf("section %d: missing end", idx)
		}
		out = append(out, sec)
	}
	return out, nil
}

func isEnd(l Line) bool { return len(l.Tokens) == 1 && strings.EqualFold(l.Tokens[0], "end") }

// LoadFile tokenizes a file from disk.
func LoadFile(path string) ([]Line, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	lines, err := Tokenize(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return lines, nil
}

// Int parses token i of a line, reporting the line number on failure.
// "*" is accepted as -1 ("any").
func (l Line) Int(i int) (int, error) {
	if i >= len(l.Tokens) {
		return 0, fmt.Errorf("line %d: missing column %d", l.No, i+1)
	}
	if l.Tokens[i] == "*" {
		return -1, nil
	}
	n, err := strconv.Atoi(l.Tokens[i])
	if err != nil {
		return 0, fmt.Errorf("line %d column %d: %w", l.No, i+1, err)
	}
	return n, nil
}

// String returns token i, or an error when the column is missing.
func (l Line) String(i int) (string, error) {
	if i >= len(l.Tokens) {
		return "", fmt.Errorf("line %d: missing column %d", l.No, i+1)
	}
	return l.Tokens[i], nil
}
