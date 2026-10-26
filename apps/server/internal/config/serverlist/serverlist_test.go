package serverlist

import (
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/config/jsonconf"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/config/mutext"
)

func TestLoadOriginalServerList(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	sf, err := jsonconf.LoadServer(filepath.Join(filepath.Dir(file), "..", "..", "..", "config", "server.json"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := FromTable(sf.ServerList)
	if err != nil {
		t.Fatal(err)
	}
	want := []Entry{
		{Code: 0, Name: "Servidor 01", Address: "192.168.1.1", Port: 55900, Visible: true},
		{Code: 19, Name: "EventServer", Address: "192.168.1.1", Port: 55919, Visible: false},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v", got)
	}
}

func TestParseErrors(t *testing.T) {
	for _, src := range []string{
		"0 \"a\" \"1.1.1.1\" 55900\nend\n",             // missing column
		"x \"a\" \"1.1.1.1\" 55900 1\nend\n",           // bad code
		"256 \"a\" \"1.1.1.1\" 55900 1\nend\n",         // code does not fit a byte
		"0 \"a\" \"1.1.1.1\" 70000 1\nend\n",           // bad port
		"0 \"a\" \"1.1.1.1\" 1 1\n0 \"b\" \"x\" 2 1\n", // duplicate code
	} {
		lines, err := mutext.Tokenize([]byte(src))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := Parse(lines); err == nil {
			t.Errorf("%q: expected error", src)
		}
	}
}
