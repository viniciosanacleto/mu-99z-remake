package mutext

import (
	"bufio"
	"bytes"
	"fmt"
	"strings"
)

// TokenizeScript splits a WizardTeam script file (Custom/Event/*.txt,
// Custom/CustomPatente.txt, read by FUN_00409600 / FUN_00451390) into lines
// of tokens. It differs from Tokenize: ';' also starts a comment, "//" only
// starts one between tokens, and 0xA0 is not a blank.
func TokenizeScript(data []byte) ([]Line, error) {
	var lines []Line
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for no := 1; sc.Scan(); no++ {
		toks, err := scriptTokens(sc.Text())
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", no, err)
		}
		if len(toks) > 0 {
			lines = append(lines, Line{No: no, Tokens: toks})
		}
	}
	return lines, sc.Err()
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
