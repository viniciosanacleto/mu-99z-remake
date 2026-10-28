package legacy

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadGameMasters(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "Data", "Util")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	src := "//Account Name Level\n\"teste2\" \"Admin\" 32\n\"lipee13\" \"BrizaGM\" 8 // support\n\"nolevel\" \"x\"\nend\n"
	if err := os.WriteFile(filepath.Join(dir, "GameMaster.txt"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	gms, err := ReadGameMasters(root)
	if err != nil {
		t.Fatal(err)
	}
	want := []GameMaster{{"teste2", "Admin", 32}, {"lipee13", "BrizaGM", 8}}
	if len(gms) != len(want) || gms[0] != want[0] || gms[1] != want[1] {
		t.Fatalf("got %+v", gms)
	}
	if _, err := ReadGameMasters(t.TempDir()); err == nil {
		t.Fatal("missing file accepted")
	}
}
