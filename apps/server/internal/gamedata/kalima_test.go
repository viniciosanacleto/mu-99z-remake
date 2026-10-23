package gamedata

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadKalima(t *testing.T) {
	r, err := LoadKalima(dataDir())
	if err != nil {
		t.Fatal(err)
	}
	if r != [KalimaLevels]int{1, 1, 1, 1, 1, 0} {
		t.Fatalf("rows %v", r)
	}
}

func TestLoadKalimaIgnoresOutOfRange(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "Event"), 0o755); err != nil {
		t.Fatal(err)
	}
	src := "// c\n2 1\n6 1\n-1 1\n0 1\nend\n3 1\n"
	if err := os.WriteFile(filepath.Join(dir, "Event", "Kalima.dat"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	r, err := LoadKalima(dir)
	if err != nil {
		t.Fatal(err)
	}
	if r != [KalimaLevels]int{1, 0, 1, 0, 0, 0} {
		t.Fatalf("rows %v", r)
	}
}
