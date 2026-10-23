package game

import (
	"context"
	"testing"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/store"
)

func TestEventEntriesSavedAndLoaded(t *testing.T) {
	b := newBareWorld(t)
	o := b.player(PlayerStart, "Entrant", 0, 130, 130)
	o.P.entries = store.EventEntry{BC: 1, DS: 2}
	c := *o.P.Char
	for _, h := range characterSaveHooks {
		h(b.World, o, &c)
	}
	b.wg.Wait()
	es := b.store.(store.EventEntryStore)
	if e, _ := es.GetEventEntry(context.Background(), o.P.Account.Login, "Entrant"); e != o.P.entries {
		t.Fatalf("stored %+v", e)
	}
	// Entering the world loads them back.
	o.P.entries = store.EventEntry{}
	for _, h := range enterWorldHooks {
		h(b.World, o)
	}
	b.wg.Wait()
	for len(b.events) > 0 {
		(<-b.events)()
	}
	if o.P.entries != (store.EventEntry{BC: 1, DS: 2}) {
		t.Fatalf("loaded %+v", o.P.entries)
	}
}
