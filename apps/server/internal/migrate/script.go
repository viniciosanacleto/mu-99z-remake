package migrate

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"
)

// ScriptSource reads the INSERT statements of a SQL Server "Generate
// Scripts" file (UTF-16 or UTF-8), like MuServer/DB/MuOnline.sql.
type ScriptSource struct {
	tables map[string][]Row // lower-case table name
}

// LoadScript parses a script file.
func LoadScript(path string) (*ScriptSource, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ParseScript(decodeText(data))
}

// decodeText handles the UTF-16 LE/BE byte-order marks of SSMS exports.
func decodeText(b []byte) string {
	if len(b) >= 2 && (b[0] == 0xFF && b[1] == 0xFE || b[0] == 0xFE && b[1] == 0xFF) {
		order := binary.ByteOrder(binary.LittleEndian)
		if b[0] == 0xFE {
			order = binary.BigEndian
		}
		b = b[2:]
		u := make([]uint16, len(b)/2)
		for i := range u {
			u[i] = order.Uint16(b[i*2:])
		}
		return string(utf16.Decode(u))
	}
	return strings.TrimPrefix(string(b), "\uFEFF")
}

// ParseScript extracts the rows of every INSERT statement.
func ParseScript(text string) (*ScriptSource, error) {
	s := &ScriptSource{tables: map[string][]Row{}}
	for i := 0; ; {
		// Data statements are "INSERT [dbo].[T] (...) VALUES (...)" at the
		// start of a line; INSERT INTO inside stored procedures is skipped.
		j := strings.Index(text[i:], "INSERT [")
		if j < 0 {
			return s, nil
		}
		if at := i + j; at > 0 && text[at-1] != '\n' {
			i = at + 1
			continue
		}
		p := &scriptParser{s: text, i: i + j + len("INSERT ")}
		table, row, err := p.insert()
		if err != nil {
			return nil, fmt.Errorf("offset %d: %w", i+j, err)
		}
		if table != "" {
			s.tables[strings.ToLower(table)] = append(s.tables[strings.ToLower(table)], row)
		}
		i = p.i
	}
}

// Rows implements Source.
func (s *ScriptSource) Rows(_ context.Context, table string) ([]Row, error) {
	return s.tables[strings.ToLower(table)], nil
}

type scriptParser struct {
	s string
	i int
}

func (p *scriptParser) ws() {
	for p.i < len(p.s) && strings.ContainsRune(" \t\r\n", rune(p.s[p.i])) {
		p.i++
	}
}

func (p *scriptParser) peek(t string) bool {
	p.ws()
	return strings.HasPrefix(strings.ToUpper(p.s[p.i:]), strings.ToUpper(t))
}

func (p *scriptParser) expect(t string) error {
	if !p.peek(t) {
		return fmt.Errorf("expected %q near %q", t, p.near())
	}
	p.i += len(t)
	return nil
}

func (p *scriptParser) near() string {
	end := min(p.i+40, len(p.s))
	return p.s[p.i:end]
}

// ident reads [name] or a bare name.
func (p *scriptParser) ident() (string, error) {
	p.ws()
	if p.i < len(p.s) && p.s[p.i] == '[' {
		end := strings.IndexByte(p.s[p.i:], ']')
		if end < 0 {
			return "", fmt.Errorf("unterminated identifier")
		}
		name := p.s[p.i+1 : p.i+end]
		p.i += end + 1
		return name, nil
	}
	start := p.i
	for p.i < len(p.s) && (isWord(p.s[p.i])) {
		p.i++
	}
	if start == p.i {
		return "", fmt.Errorf("expected identifier near %q", p.near())
	}
	return p.s[start:p.i], nil
}

func isWord(c byte) bool {
	return c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9'
}

// insert parses `[dbo].[T] ([a], [b]) VALUES (v1, v2)`.
func (p *scriptParser) insert() (string, Row, error) {
	name, err := p.ident()
	if err != nil {
		return "", nil, err
	}
	for p.peek(".") {
		p.i++
		if name, err = p.ident(); err != nil {
			return "", nil, err
		}
	}
	if err := p.expect("("); err != nil {
		return "", nil, err
	}
	var cols []string
	for {
		c, err := p.ident()
		if err != nil {
			return "", nil, err
		}
		cols = append(cols, c)
		if p.peek(")") {
			p.i++
			break
		}
		if err := p.expect(","); err != nil {
			return "", nil, err
		}
	}
	if err := p.expect("VALUES"); err != nil {
		return "", nil, err
	}
	if err := p.expect("("); err != nil {
		return "", nil, err
	}
	row := Row{}
	for k := 0; ; k++ {
		v, err := p.value()
		if err != nil {
			return "", nil, err
		}
		if k < len(cols) {
			row[cols[k]] = v
		}
		if p.peek(")") {
			p.i++
			break
		}
		if err := p.expect(","); err != nil {
			return "", nil, err
		}
	}
	return name, row, nil
}

func (p *scriptParser) value() (any, error) {
	p.ws()
	switch {
	case p.peek("NULL"):
		p.i += 4
		return nil, nil
	case p.peek("N'"):
		p.i++
		return p.str()
	case p.peek("'"):
		return p.str()
	case p.peek("0x"):
		p.i += 2
		start := p.i
		for p.i < len(p.s) && strings.ContainsRune("0123456789abcdefABCDEF", rune(p.s[p.i])) {
			p.i++
		}
		return hex.DecodeString(p.s[start:p.i])
	case p.peek("CAST("):
		p.i += 5
		v, err := p.value()
		if err != nil {
			return nil, err
		}
		if err := p.expect("AS"); err != nil {
			return nil, err
		}
		typ, err := p.ident()
		if err != nil {
			return nil, err
		}
		if p.peek("(") { // Decimal(18, 0)
			end := strings.IndexByte(p.s[p.i:], ')')
			p.i += end + 1
		}
		if err := p.expect(")"); err != nil {
			return nil, err
		}
		return castValue(v, strings.ToLower(typ))
	}
	start := p.i
	for p.i < len(p.s) && strings.ContainsRune("+-.0123456789eE", rune(p.s[p.i])) {
		p.i++
	}
	lit := p.s[start:p.i]
	if lit == "" {
		return nil, fmt.Errorf("unexpected value near %q", p.near())
	}
	if n, err := strconv.ParseInt(lit, 10, 64); err == nil {
		return n, nil
	}
	return strconv.ParseFloat(lit, 64)
}

func (p *scriptParser) str() (string, error) {
	p.i++ // opening quote
	var b strings.Builder
	for p.i < len(p.s) {
		c := p.s[p.i]
		if c == '\'' {
			if p.i+1 < len(p.s) && p.s[p.i+1] == '\'' {
				b.WriteByte('\'')
				p.i += 2
				continue
			}
			p.i++
			return b.String(), nil
		}
		b.WriteByte(c)
		p.i++
	}
	return "", fmt.Errorf("unterminated string")
}

var sqlEpoch = time.Date(1900, 1, 1, 0, 0, 0, 0, time.UTC)

// castValue decodes CAST(0x... AS SmallDateTime / DateTime) and textual
// date casts; other casts keep the value.
func castValue(v any, typ string) (any, error) {
	switch b := v.(type) {
	case []byte:
		switch typ {
		case "smalldatetime":
			if len(b) != 4 {
				return nil, fmt.Errorf("smalldatetime of %d bytes", len(b))
			}
			days := binary.BigEndian.Uint16(b)
			mins := binary.BigEndian.Uint16(b[2:])
			return sqlEpoch.AddDate(0, 0, int(days)).Add(time.Duration(mins) * time.Minute), nil
		case "datetime":
			if len(b) != 8 {
				return nil, fmt.Errorf("datetime of %d bytes", len(b))
			}
			days := int32(binary.BigEndian.Uint32(b))
			ticks := binary.BigEndian.Uint32(b[4:])
			return sqlEpoch.AddDate(0, 0, int(days)).Add(time.Duration(ticks) * time.Second / 300), nil
		}
	case string:
		if strings.Contains(typ, "date") {
			for _, f := range []string{"2006-01-02T15:04:05.000", "2006-01-02T15:04:05", "2006-01-02 15:04:05", "2006-01-02"} {
				if t, err := time.Parse(f, b); err == nil {
					return t, nil
				}
			}
		}
	}
	return v, nil
}
