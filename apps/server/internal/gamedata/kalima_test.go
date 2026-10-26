package gamedata

import (
	"testing"
)

func TestLoadKalima(t *testing.T) {
	r, err := LoadKalima(configDir())
	if err != nil {
		t.Fatal(err)
	}
	if r != [KalimaLevels]int{1, 1, 1, 1, 1, 0} {
		t.Fatalf("rows %v", r)
	}
}
