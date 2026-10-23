package store

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"testing"
	"time"
)

// testReconnectStore checks the DataServer C0 00 / C0 01 / replay queries:
// a new name is inserted with its server code and time, an existing name
// only gets its Data replaced (on every server code), the delete needs
// the name and the server code, and the replay lists one server code.
func testReconnectStore(t *testing.T, s Store) {
	rs, ok := s.(ReconnectStore)
	if !ok {
		t.Fatal("no ReconnectStore")
	}
	ctx := context.Background()
	suffix := fmt.Sprintf("%06d", time.Now().UnixNano()/1000%1000000)
	code := 9000 + int(time.Now().UnixNano()%500)
	name := "Rc" + suffix
	if err := rs.SaveReconnect(ctx, ReconnectRow{Name: name, ServerCode: code, Data: []byte{1, 2, 3}, Time: 300}); err != nil {
		t.Fatal(err)
	}
	rows, err := rs.ReconnectRows(ctx, code)
	if err != nil || len(rows) != 1 || rows[0].Name != name || rows[0].Time != 300 || len(rows[0].Data) != ReconnectDataSize ||
		!bytes.Equal(rows[0].Data[:4], []byte{1, 2, 3, 0}) {
		t.Fatalf("rows %+v %v", rows, err)
	}
	// Leaving another server: UPDATE ... WHERE Name only.
	if err := rs.SaveReconnect(ctx, ReconnectRow{Name: "RC" + suffix, ServerCode: code + 1, Data: []byte{9}, Time: 10}); err != nil {
		t.Fatal(err)
	}
	if other, _ := rs.ReconnectRows(ctx, code+1); len(other) != 0 {
		t.Fatalf("new row for the other server %+v", other)
	}
	rows, _ = rs.ReconnectRows(ctx, code)
	if len(rows) != 1 || rows[0].Data[0] != 9 || rows[0].Time != 300 {
		t.Fatalf("update %+v", rows)
	}
	if err := rs.DeleteReconnect(ctx, name, code+1); err != nil {
		t.Fatal(err)
	}
	if rows, _ = rs.ReconnectRows(ctx, code); len(rows) != 1 {
		t.Fatal("deleted with the wrong server code")
	}
	if err := rs.DeleteReconnect(ctx, "rc"+suffix, code); err != nil {
		t.Fatal(err)
	}
	if rows, _ = rs.ReconnectRows(ctx, code); len(rows) != 0 {
		t.Fatalf("not deleted %+v", rows)
	}
}

func TestMemoryReconnectStore(t *testing.T) {
	m, _ := NewMemory()
	testReconnectStore(t, m)
}

func TestPostgresReconnectStore(t *testing.T) {
	url := os.Getenv("MU_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("MU_TEST_DATABASE_URL not set")
	}
	p, err := OpenPostgres(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	testReconnectStore(t, p)
}
