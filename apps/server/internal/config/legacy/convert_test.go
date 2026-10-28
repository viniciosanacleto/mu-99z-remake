package legacy

import (
	"path/filepath"
	"slices"
	"testing"
)

func TestOutputs(t *testing.T) {
	var groups, paths []string
	for _, o := range Outputs() {
		if o.Build == nil {
			t.Fatalf("%s: no Build", o.Path)
		}
		if slices.Contains(paths, o.Path) {
			t.Fatalf("duplicate output %s", o.Path)
		}
		paths = append(paths, o.Path)
		if o.Group != "gameserver" {
			groups = append(groups, o.Group)
		}
	}
	expected := []string{
		"server", "character", "common", "custom", "custom-event", "xshop", "event", "eventitembag",
		"item", "lang", "monster", "spawn", "move", "quest", "shop", "skill", "hack", "util",
	}
	if !slices.Equal(groups, expected) {
		t.Fatalf("groups = %v, want %v", groups, expected)
	}
	if !slices.Contains(paths, filepath.Join("data", "custom-event.toml")) {
		t.Fatalf("data group path missing: %v", paths)
	}
}
