package serverlist

import (
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/config/servercfg"
)

func TestBuildShippedServerList(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	f, err := servercfg.Load(filepath.Join(filepath.Dir(file), "..", "..", "..", "config", "server.toml"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := Build(f.ConnectServer.ServerList)
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

func TestBuildErrors(t *testing.T) {
	for _, rows := range [][]servercfg.ServerRow{
		{{ServerCode: 256, Port: 55900}},                     // code does not fit a byte
		{{ServerCode: -1, Port: 55900}},                      // negative code
		{{ServerCode: 0, Port: 70000}},                       // bad port
		{{ServerCode: 0, Port: 0}},                           // bad port
		{{ServerCode: 0, Port: 1}, {ServerCode: 0, Port: 2}}, // duplicate code
	} {
		if _, err := Build(rows); err == nil {
			t.Errorf("%+v: expected error", rows)
		}
	}
}
