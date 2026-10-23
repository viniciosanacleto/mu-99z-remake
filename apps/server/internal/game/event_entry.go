package game

import (
	"context"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/store"
)

// Daily event entries (EventEntryCount): loaded with the character
// (DataServer FUN_0040c4c0, WZ_GetEventEntryInfo, reset on a new day) and
// written with every character save (FUN_0040e9c0, WZ_SetEventEntryInfo).

func init() {
	enterWorldHooks = append(enterWorldHooks, func(w *World, o *Object) {
		es, ok := w.store.(store.EventEntryStore)
		if !ok || o.P == nil || o.P.Char == nil {
			return
		}
		account, name, c := o.P.Account.Login, o.P.Char.Name, o.conn
		w.async(func(ctx context.Context) func() {
			e, err := es.GetEventEntry(ctx, account, name)
			return func() {
				if err != nil {
					w.log.Error("event entry load", "name", name, "err", err)
					return
				}
				if o := w.objectOf(c); o != nil && o.P != nil && o.P.Char != nil && o.P.Char.Name == name {
					o.P.entries = e
				}
			}
		})
	})
	characterSaveHooks = append(characterSaveHooks, func(w *World, o *Object, c *store.Character) {
		es, ok := w.store.(store.EventEntryStore)
		if !ok {
			return
		}
		account, name, e := o.P.Account.Login, c.Name, o.P.entries
		w.async(func(ctx context.Context) func() {
			if err := es.SetEventEntry(ctx, account, name, e); err != nil {
				w.log.Error("event entry save", "name", name, "err", err)
			}
			return nil
		})
	})
}
