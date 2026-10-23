package mutext

import (
	"reflect"
	"testing"
)

func TestTokenize(t *testing.T) {
	src := "// header\r\n" +
		"   0   \"Servidor 01\"  \"192.168.1.1\"  55900  1 // trailing\r\n" +
		"\r\n" +
		"1\t*\tabc//glued\n" +
		"end\n"
	lines, err := Tokenize([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	want := []Line{
		{No: 2, Tokens: []string{"0", "Servidor 01", "192.168.1.1", "55900", "1"}},
		{No: 4, Tokens: []string{"1", "*", "abc"}},
		{No: 5, Tokens: []string{"end"}},
	}
	if !reflect.DeepEqual(lines, want) {
		t.Fatalf("got %#v", lines)
	}
	if rows := Table(lines); len(rows) != 2 {
		t.Fatalf("Table rows = %d", len(rows))
	}
	if n, err := lines[1].Int(1); err != nil || n != -1 {
		t.Fatalf("Int(*) = %d %v", n, err)
	}
	if _, err := lines[1].Int(2); err == nil {
		t.Fatal("expected error for non-number")
	}
	if _, err := lines[1].String(9); err == nil {
		t.Fatal("expected missing column error")
	}
}

func TestTokenizeUnterminatedString(t *testing.T) {
	if _, err := Tokenize([]byte("1 \"abc\n")); err == nil {
		t.Fatal("expected error")
	}
}

func TestSections(t *testing.T) {
	lines, err := Tokenize([]byte("0\n1 2\nend\n3\n4 5\n6 7\nend\n"))
	if err != nil {
		t.Fatal(err)
	}
	secs, err := Sections(lines)
	if err != nil {
		t.Fatal(err)
	}
	if len(secs) != 2 || secs[0].Index != 0 || len(secs[0].Rows) != 1 ||
		secs[1].Index != 3 || len(secs[1].Rows) != 2 {
		t.Fatalf("got %#v", secs)
	}

	for _, bad := range []string{"0\n1 2\n", "x\nend\n", "1 2\nend\n"} {
		lines, _ := Tokenize([]byte(bad))
		if _, err := Sections(lines); err == nil {
			t.Errorf("%q: expected error", bad)
		}
	}
}
