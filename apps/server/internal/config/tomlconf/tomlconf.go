// Package tomlconf reads and writes the server configuration under
// apps/server/config (docs/server-plan/11-arquivos-de-configuracao.md §TOML).
//
// The files are UTF-8 TOML, but the original servers and the client work
// with Windows-1252 bytes (messages, notices, names are sent as they are in
// the original text files). Load therefore converts every decoded string to
// Windows-1252, so the rest of the server sees the same bytes it saw when it
// read the original files. Characters with no Windows-1252 code become '?'.
package tomlconf

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"reflect"
	"unicode/utf8"

	toml "github.com/pelletier/go-toml/v2"
	"golang.org/x/text/encoding/charmap"
)

// Load decodes the file at path into v. Keys that v has no field for are an
// error, so a typo in a configuration file does not go unnoticed.
func Load(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := Unmarshal(b, v); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}

// Unmarshal decodes TOML data into v (strict, see Load) and converts the
// strings to Windows-1252.
func Unmarshal(data []byte, v any) error {
	d := toml.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		return describe(err)
	}
	ToANSI(reflect.ValueOf(v))
	return nil
}

// describe turns go-toml errors into one-line messages with the position.
func describe(err error) error {
	var de *toml.DecodeError
	if errors.As(err, &de) {
		row, col := de.Position()
		return fmt.Errorf("line %d column %d: %s", row, col, de.Error())
	}
	var se *toml.StrictMissingError
	if errors.As(err, &se) && len(se.Errors) > 0 {
		row, col := se.Errors[0].Position()
		return fmt.Errorf("line %d column %d: unknown key %q", row, col, se.Errors[0].Key())
	}
	return err
}

// ANSI converts a UTF-8 string to Windows-1252 bytes.
func ANSI(s string) string {
	ascii := true
	for i := 0; i < len(s); i++ {
		if s[i] >= utf8.RuneSelf {
			ascii = false
			break
		}
	}
	if ascii {
		return s
	}
	b := make([]byte, 0, len(s))
	for _, r := range s {
		c, ok := charmap.Windows1252.EncodeRune(r)
		if !ok {
			c = '?'
		}
		b = append(b, c)
	}
	return string(b)
}

// UTF8 converts Windows-1252 bytes to UTF-8; valid UTF-8 is returned as it
// is (most original files are UTF-8 or plain ASCII).
func UTF8(s string) string {
	if utf8.ValidString(s) {
		return s
	}
	out, err := charmap.Windows1252.NewDecoder().String(s)
	if err != nil {
		return s
	}
	return out
}

// ToANSI converts, in place, every string reachable from v (struct fields,
// slice and array elements, map keys and values) to Windows-1252.
func ToANSI(v reflect.Value) {
	walkStrings(v, ANSI)
}

func walkStrings(v reflect.Value, conv func(string) string) {
	switch v.Kind() {
	case reflect.Pointer, reflect.Interface:
		if v.IsNil() {
			return
		}
		e := v.Elem()
		if v.Kind() == reflect.Interface {
			// Values inside interfaces are not addressable: rebuild them.
			c := reflect.New(e.Type()).Elem()
			c.Set(e)
			walkStrings(c, conv)
			if v.CanSet() {
				v.Set(c)
			}
			return
		}
		walkStrings(e, conv)
	case reflect.String:
		if v.CanSet() {
			v.SetString(conv(v.String()))
		}
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			if v.Type().Field(i).IsExported() {
				walkStrings(v.Field(i), conv)
			}
		}
	case reflect.Slice, reflect.Array:
		for i := 0; i < v.Len(); i++ {
			walkStrings(v.Index(i), conv)
		}
	case reflect.Map:
		if v.IsNil() {
			return
		}
		keys := v.MapKeys()
		for _, k := range keys {
			val := reflect.New(v.Type().Elem()).Elem()
			val.Set(v.MapIndex(k))
			walkStrings(val, conv)
			nk := reflect.New(k.Type()).Elem()
			nk.Set(k)
			walkStrings(nk, conv)
			if !nk.Equal(k) {
				v.SetMapIndex(k, reflect.Value{})
			}
			v.SetMapIndex(nk, val)
		}
	}
}

// LoadUTF8 is Load without the Windows-1252 conversion: for the files the
// server rewrites (Save), whose strings must stay UTF-8.
func LoadUTF8(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	d := toml.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		return fmt.Errorf("%s: %w", path, describe(err))
	}
	return nil
}

// Save writes v to path (through a temporary file renamed over it),
// keeping the leading "#" comment lines of the current file.
func Save(path string, v any) error {
	b, err := Marshal(v)
	if err != nil {
		return err
	}
	var head []byte
	if old, err := os.ReadFile(path); err == nil {
		for _, l := range bytes.SplitAfter(old, []byte("\n")) {
			if !bytes.HasPrefix(bytes.TrimSpace(l), []byte("#")) {
				break
			}
			head = append(head, l...)
		}
	}
	if len(head) > 0 {
		head = append(head, '\n')
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(head, b...), 0o644); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}
