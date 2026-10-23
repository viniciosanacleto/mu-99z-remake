package gamedata

import (
	"os"
	"path/filepath"
	"testing"
)

const eventData = "../../data/Data"

func TestLoadCustomEvents(t *testing.T) {
	d, err := LoadDeathEvent(eventData)
	if err != nil {
		t.Fatal(err)
	}
	if !d.Active || d.Syntax != "/irdeath" || !d.Patente || d.Level != 400 || d.Place != (EventPlace{6, 116, 39}) {
		t.Fatalf("death %+v", d)
	}
	if len(d.Starts) != 7 || d.Starts[3] != (EventStart{3, 19, 30, 5, 5}) {
		t.Fatalf("death starts %+v", d.Starts)
	}
	th, err := LoadTheftEvent(eventData)
	if err != nil || th.Syntax != "/irevento" || th.Starts[0] != (EventStart{0, 22, 30, 5, 5}) {
		t.Fatalf("theft %+v %v", th, err)
	}
	du, err := LoadDuelEvent(eventData)
	if err != nil {
		t.Fatal(err)
	}
	if du.Syntax != "/irmatamata" || du.Level != 200 || du.Wait != (EventPlace{40, 22, 14}) ||
		du.Fight != (EventPlace{6, 71, 11}) || du.Starts[6] != (EventStart{6, 20, 30, 5, 5}) {
		t.Fatalf("duel %+v", du)
	}
	pa, err := LoadPairEvent(eventData)
	if err != nil || pa.Syntax != "/dupla" || pa.Level != 400 || pa.Wait != (EventPlace{0, 240, 130}) ||
		pa.Starts[0] != (EventStart{0, 21, 30, 5, -1}) {
		t.Fatalf("pair %+v %v", pa, err)
	}
	so, err := LoadSobreEvent(eventData)
	if err != nil || so.Syntax != "/prainhapvp" || so.Level != 210 || so.Place != (EventPlace{0, 240, 130}) ||
		so.MinPlayers != 2 || len(so.Starts) != 7 || so.Starts[0] != (EventStart{0, 18, 30, 5, -1}) {
		t.Fatalf("sobre %+v %v", so, err)
	}
	ro, err := LoadRoletaEvent(eventData)
	if err != nil || !ro.Active || ro.Syntax != "/irroleta" || ro.Level != 400 || ro.Place != (EventPlace{6, 94, 194}) {
		t.Fatalf("roleta %+v %v", ro, err)
	}
	ra, err := LoadRaffleEvent(eventData)
	if err != nil || !ra.Active || len(ra.Starts) != 21 || ra.Starts[2] != (EventStart{0, 23, 59, 0, -1}) {
		t.Fatalf("raffle %+v %v", ra, err)
	}
}

func TestEventScriptDefaults(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "Custom", "Event")
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatal(err)
	}
	// A missing cell reads as -1 (ints) or 0xFF (bytes); ';' and '//'
	// start comments.
	src := "; comment\n0\n 1 \"/x y\" // tail\nend\n"
	if err := os.WriteFile(filepath.Join(p, "SobreEvent.txt"), []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	so, err := LoadSobreEvent(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !so.Active || so.Syntax != "/x y" || so.Patente || so.Level != -1 || so.Place != (EventPlace{0xFF, 0xFF, 0xFF}) ||
		so.MinPlayers != 0xFF || so.Starts != nil {
		t.Fatalf("defaults %+v", so)
	}
	if _, err := LoadEventScript(filepath.Join(p, "missing.txt")); err == nil {
		t.Fatal("missing file read")
	}
	if err := os.WriteFile(filepath.Join(p, "bad.txt"), []byte("0\n1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadEventScript(filepath.Join(p, "bad.txt")); err == nil {
		t.Fatal("section without end accepted")
	}
}
