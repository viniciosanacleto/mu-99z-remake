package game

import (
	"runtime"
	"testing"
	"weak"
)

// TestWorldIsCollectable guards against package-level caches keyed by
// *World or *Hub: each World holds ~150 MB of objects, and the package's
// tests build hundreds of them.
func TestWorldIsCollectable(t *testing.T) {
	b := newBareWorld(t)
	b.chaosData()
	b.petTable()
	b.csSync()
	b.guildMgr()
	wp, hp := weak.Make(b.World), weak.Make(b.hub)
	b = nil
	runtime.GC()
	if wp.Value() != nil || hp.Value() != nil {
		t.Fatal("World or Hub still reachable after GC")
	}
}

func TestHubSharesCastleSyncAndGuilds(t *testing.T) {
	a, b := newBareWorld(t), newBareWorld(t)
	b.hub = a.hub
	if a.csSync() != b.csSync() || a.guildMgr() != b.guildMgr() {
		t.Fatal("instances of a hub must share the castle sync and the guild manager")
	}
	c := newBareWorld(t)
	if c.csSync() == a.csSync() || c.guildMgr() == a.guildMgr() {
		t.Fatal("different hubs must not share state")
	}
	if c.petTable() != c.petTable() {
		t.Fatal("pet table not cached")
	}
}
