package mutext

import (
	"reflect"
	"testing"
)

func TestTokenizeScript(t *testing.T) {
	src := "; header\n0\n 1 \"/x y\" a//b // tail\n2;c\nend\n"
	lines, err := TokenizeScript([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	want := []Line{
		{No: 2, Tokens: []string{"0"}},
		{No: 3, Tokens: []string{"1", "/x y", "a//b"}},
		{No: 4, Tokens: []string{"2"}},
		{No: 5, Tokens: []string{"end"}},
	}
	if !reflect.DeepEqual(lines, want) {
		t.Fatalf("got %#v", lines)
	}
	if _, err := TokenizeScript([]byte("0\n\"open\n")); err == nil {
		t.Fatal("unterminated string accepted")
	}
}
