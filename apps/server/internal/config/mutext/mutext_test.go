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
		{No: 2, Tokens: []string{"0", "Servidor 01", "192.168.1.1", "55900", "1"}, Comment: "trailing"},
		{No: 4, Tokens: []string{"1", "*", "abc"}, Comment: "glued"},
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

func TestNumTokLikeOriginalTokenizer(t *testing.T) {
	cases := []struct {
		tok  string
		prev int
		want int
	}{{"12", 0, 12}, {"*", 5, -1}, {"N", 7, 7}, {"-3", 0, -3}, {"2.9", 0, 2}, {"", 4, 4}}
	for _, c := range cases {
		if got := NumTok(c.tok, c.prev); got != c.want {
			t.Errorf("NumTok(%q,%d) = %d want %d", c.tok, c.prev, got, c.want)
		}
	}
	l := Line{Tokens: []string{"07,000", "0", "0,", "8", "N"}}
	if got := NumRow(NumFields(l), 0, 6); got[0] != 7 || got[1] != 0 || got[3] != 0 || got[4] != 8 || got[5] != 8 {
		t.Fatalf("NumRow = %v", got)
	}
}

func TestStreamRows(t *testing.T) {
	rows := []Line{{Tokens: []string{"1", "2", "3"}}, {Tokens: []string{"4,5"}}, {Tokens: []string{"6", "7"}}}
	got := StreamRows(rows, 3)
	if !reflect.DeepEqual(got, [][]int{{1, 2, 3}, {4, 5, 6}, {7, 7, 7}}) {
		t.Fatalf("records = %v", got)
	}
}
