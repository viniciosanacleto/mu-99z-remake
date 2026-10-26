package tomlconf

import (
	"bytes"
	"fmt"
	"math"
	"reflect"
	"sort"
	"strconv"
	"strings"
)

// Marshal encodes v (a struct or a map with string keys) as TOML. It is used
// by cmd/configconv and writes a stable, readable layout:
//
//   - fields keep their declaration order and map keys are sorted;
//   - a `comment:"..."` struct tag writes "# ..." lines before the key;
//   - a slice of structs tagged `toml:"name,inline"` is written as an array
//     with one inline table per line, its columns aligned, instead of one
//     [[name]] block per row (tables such as Item.txt have thousands of rows);
//   - `toml:",omitempty"` skips zero values;
//   - the fields of an embedded struct are written in the parent table.
//
// Strings are written as they are: callers pass UTF-8 (see UTF8).
func Marshal(v any) ([]byte, error) {
	e := &encoder{}
	rv := reflect.ValueOf(v)
	for rv.Kind() == reflect.Pointer {
		rv = rv.Elem()
	}
	if err := e.table(nil, rv, true, ""); err != nil {
		return nil, err
	}
	return bytes.TrimLeft(e.buf.Bytes(), "\n"), nil
}

type encoder struct{ buf bytes.Buffer }

// entry is one key of a table.
type entry struct {
	key     string
	comment string
	inline  bool
	v       reflect.Value
}

func (e *encoder) entries(v reflect.Value) ([]entry, error) {
	var out []entry
	switch v.Kind() {
	case reflect.Struct:
		t := v.Type()
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			if !f.IsExported() {
				continue
			}
			name, opts, _ := strings.Cut(f.Tag.Get("toml"), ",")
			if name == "-" {
				continue
			}
			if f.Anonymous && name == "" && f.Type.Kind() == reflect.Struct {
				// Embedded struct: its fields belong to the parent table.
				sub, err := e.entries(v.Field(i))
				if err != nil {
					return nil, err
				}
				out = append(out, sub...)
				continue
			}
			if name == "" {
				name = f.Name
			}
			fv := v.Field(i)
			if hasOpt(opts, "omitempty") && fv.IsZero() {
				continue
			}
			out = append(out, entry{key: name, comment: f.Tag.Get("comment"), inline: hasOpt(opts, "inline"), v: fv})
		}
	case reflect.Map:
		keys := v.MapKeys()
		names := make([]string, len(keys))
		for i, k := range keys {
			s, err := mapKey(k)
			if err != nil {
				return nil, err
			}
			names[i] = s
		}
		idx := make([]int, len(keys))
		for i := range idx {
			idx[i] = i
		}
		sort.Slice(idx, func(a, b int) bool { return names[idx[a]] < names[idx[b]] })
		for _, i := range idx {
			out = append(out, entry{key: names[i], v: v.MapIndex(keys[i])})
		}
	default:
		return nil, fmt.Errorf("tomlconf: cannot encode %s as a table", v.Type())
	}
	return out, nil
}

func hasOpt(opts, want string) bool {
	for _, o := range strings.Split(opts, ",") {
		if o == want {
			return true
		}
	}
	return false
}

func mapKey(k reflect.Value) (string, error) {
	switch k.Kind() {
	case reflect.String:
		return k.String(), nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(k.Int(), 10), nil
	}
	return "", fmt.Errorf("tomlconf: unsupported map key %s", k.Type())
}

func deref(v reflect.Value) reflect.Value {
	for v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface {
		if v.IsNil() {
			return reflect.Value{}
		}
		v = v.Elem()
	}
	return v
}

// kind classifies an entry: a sub-table, an array of tables, or a value.
func kind(en entry) string {
	v := deref(en.v)
	if !v.IsValid() {
		return "skip"
	}
	if en.inline {
		return "value"
	}
	switch v.Kind() {
	case reflect.Struct, reflect.Map:
		return "table"
	case reflect.Slice, reflect.Array:
		if v.Type().Elem().Kind() == reflect.Uint8 {
			return "value"
		}
		et := v.Type().Elem()
		for et.Kind() == reflect.Pointer {
			et = et.Elem()
		}
		if et.Kind() == reflect.Struct || et.Kind() == reflect.Map {
			return "array"
		}
	}
	return "value"
}

func (e *encoder) table(path []string, v reflect.Value, root bool, comment string) error {
	ents, err := e.entries(v)
	if err != nil {
		return err
	}
	var values, subs []entry
	for _, en := range ents {
		switch kind(en) {
		case "value":
			values = append(values, en)
		case "table", "array":
			subs = append(subs, en)
		}
	}
	if !root && (len(values) > 0 || len(subs) == 0) {
		e.buf.WriteByte('\n')
		e.comment(comment)
		e.buf.WriteString("[" + joinKeys(path) + "]\n")
	} else if comment != "" {
		e.buf.WriteByte('\n')
		e.comment(comment)
	}
	for _, en := range values {
		e.comment(en.comment)
		e.buf.WriteString(quoteKey(en.key) + " = ")
		if err := e.value(deref(en.v), en.inline, true); err != nil {
			return fmt.Errorf("%s: %w", joinKeys(append(path, en.key)), err)
		}
		e.buf.WriteByte('\n')
	}
	for _, en := range subs {
		p := append(append([]string{}, path...), en.key)
		v := deref(en.v)
		if kind(en) == "table" {
			if err := e.table(p, v, false, en.comment); err != nil {
				return err
			}
			continue
		}
		for i := 0; i < v.Len(); i++ {
			e.buf.WriteByte('\n')
			if i == 0 {
				e.comment(en.comment)
			}
			e.buf.WriteString("[[" + joinKeys(p) + "]]\n")
			if err := e.body(deref(v.Index(i)), p); err != nil {
				return err
			}
		}
	}
	return nil
}

// body writes the contents of an [[array]] element (values, then nested
// tables under the array path).
func (e *encoder) body(v reflect.Value, path []string) error {
	ents, err := e.entries(v)
	if err != nil {
		return err
	}
	for _, en := range ents {
		if kind(en) != "value" {
			return fmt.Errorf("%s.%s: nested tables in an array of tables are not supported", joinKeys(path), en.key)
		}
		e.comment(en.comment)
		e.buf.WriteString(quoteKey(en.key) + " = ")
		if err := e.value(deref(en.v), en.inline, true); err != nil {
			return err
		}
		e.buf.WriteByte('\n')
	}
	return nil
}

func (e *encoder) comment(c string) {
	if c == "" {
		return
	}
	for _, l := range strings.Split(c, "\n") {
		e.buf.WriteString(strings.TrimRight("# "+l, " ") + "\n")
	}
}

// value writes a TOML value. top is true for the value of a key line, where
// arrays of tables become one inline table per line.
func (e *encoder) value(v reflect.Value, inline, top bool) error {
	s, err := e.render(v, inline && top)
	if err != nil {
		return err
	}
	e.buf.WriteString(s)
	return nil
}

func (e *encoder) render(v reflect.Value, multiline bool) (string, error) {
	v = deref(v)
	if !v.IsValid() {
		return "", fmt.Errorf("nil value")
	}
	switch v.Kind() {
	case reflect.String:
		return quote(v.String()), nil
	case reflect.Bool:
		return strconv.FormatBool(v.Bool()), nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(v.Int(), 10), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return strconv.FormatUint(v.Uint(), 10), nil
	case reflect.Float32, reflect.Float64:
		f := v.Float()
		if math.IsInf(f, 0) || math.IsNaN(f) {
			return "", fmt.Errorf("unsupported float %v", f)
		}
		s := strconv.FormatFloat(f, 'f', -1, 64)
		if !strings.ContainsAny(s, ".e") {
			s += ".0"
		}
		return s, nil
	case reflect.Struct, reflect.Map:
		cells, err := e.cells(v)
		if err != nil {
			return "", err
		}
		if len(cells) == 0 {
			return "{}", nil
		}
		return "{ " + strings.Join(cells, ", ") + " }", nil
	case reflect.Slice, reflect.Array:
		et := v.Type().Elem()
		for et.Kind() == reflect.Pointer {
			et = et.Elem()
		}
		if multiline && (et.Kind() == reflect.Struct || et.Kind() == reflect.Map) {
			return e.rows(v)
		}
		parts := make([]string, v.Len())
		for i := range parts {
			s, err := e.render(v.Index(i), false)
			if err != nil {
				return "", err
			}
			parts[i] = s
		}
		return "[" + strings.Join(parts, ", ") + "]", nil
	}
	return "", fmt.Errorf("unsupported type %s", v.Type())
}

// cells renders the "key = value" pairs of an inline table.
func (e *encoder) cells(v reflect.Value) ([]string, error) {
	ents, err := e.entries(v)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, en := range ents {
		if !deref(en.v).IsValid() {
			continue
		}
		s, err := e.render(en.v, false)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", en.key, err)
		}
		out = append(out, quoteKey(en.key)+" = "+s)
	}
	return out, nil
}

// rows renders an array of inline tables, one per line, with the cells of
// each position padded to line up (rows may differ in their last, optional
// cells, such as Comment).
func (e *encoder) rows(v reflect.Value) (string, error) {
	if v.Len() == 0 {
		return "[]", nil
	}
	all := make([][]string, v.Len())
	var width []int
	for i := range all {
		c, err := e.cells(v.Index(i))
		if err != nil {
			return "", fmt.Errorf("row %d: %w", i, err)
		}
		all[i] = c
		for j, cell := range c[:max(len(c)-1, 0)] {
			if j == len(width) {
				width = append(width, 0)
			}
			width[j] = max(width[j], len(cell))
		}
	}
	var b strings.Builder
	b.WriteString("[\n")
	for _, r := range all {
		b.WriteString("  { ")
		for j, c := range r {
			b.WriteString(c)
			if j < len(r)-1 {
				b.WriteString(",")
				b.WriteString(strings.Repeat(" ", width[j]-len(c)))
				b.WriteString(" ")
			}
		}
		b.WriteString(" },\n")
	}
	b.WriteString("]")
	return b.String(), nil
}

func joinKeys(path []string) string {
	q := make([]string, len(path))
	for i, p := range path {
		q[i] = quoteKey(p)
	}
	return strings.Join(q, ".")
}

func quoteKey(k string) string {
	if k == "" {
		return `""`
	}
	for i := 0; i < len(k); i++ {
		c := k[i]
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '-') {
			return quote(k)
		}
	}
	return k
}

// quote writes a TOML basic string.
func quote(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			if r < 0x20 || r == 0x7f {
				fmt.Fprintf(&b, `\u%04X`, r)
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte('"')
	return b.String()
}
