package game

import "github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"

// viewRange is the MapManager ViewRange of a map (default 12).
func (w *World) viewRange(mp int) int {
	if info, ok := w.data.Maps[mp]; ok && info.ViewRange >= 0 {
		return info.ViewRange
	}
	return gamedata.DefaultViewRange
}

// inRange is FUN_00513bf0: same map and inside the +-range square around
// (x, y).
func inRange(t *Object, mp, x, y, r int) bool {
	return t.Map == mp && x-r <= t.X && t.X <= x+r && y-r <= t.Y && t.Y <= y+r
}

// vpFind returns the slot of index in a viewport list, or -1.
func vpFind(list *[ViewportSize]VPEntry, index int, typ byte) int {
	for i, e := range list {
		if e.State != VPEmpty && e.Index == index && e.Type == typ {
			return i
		}
	}
	return -1
}

// vpAdd is FUN_00513d10 / FUN_00513da0: first empty slot, state 1.
func vpAdd(list *[ViewportSize]VPEntry, index int, typ byte) bool {
	if vpFind(list, index, typ) >= 0 {
		return false
	}
	for i := range list {
		if list[i].State == VPEmpty {
			list[i] = VPEntry{State: VPNew, Index: index, Type: typ}
			return true
		}
	}
	return false
}

// viewportRemove marks entries that left the view (FUN_00513e60 for
// players, FUN_00513fb0 for monsters) and drops stale "who sees me"
// entries (FUN_005142c0).
func (w *World) viewportRemove(o *Object) {
	r := w.viewRange(o.Map)
	for i := range o.VP {
		e := &o.VP[i]
		if e.State != VPNew && e.State != VPVisible {
			continue
		}
		if e.Index < 0 || e.Index >= MaxObjects {
			continue
		}
		t := w.objects[e.Index]
		gone := t.State != StatePlaying && t.Type == TypePlayer
		if t.Type != TypePlayer {
			gone = t.State == StateEmpty
		}
		if gone || (t.Live == LiveNew && !t.Alive) || t.Teleporting || t.Live == LiveRemoved ||
			t.Live == LiveTeleport || !inRange(t, o.Map, o.X, o.Y, r) {
			e.State = VPRemove
		}
	}
	for i := range o.VP2 {
		e := &o.VP2[i]
		if (e.State != VPNew && e.State != VPVisible) || e.Index < 0 || e.Index >= MaxObjects {
			continue
		}
		t := w.objects[e.Index]
		if t.State == StateEmpty || (t.Type == TypePlayer && t.State != StatePlaying) ||
			!inRange(o, t.Map, t.X, t.Y, w.viewRange(t.Map)) {
			*e = VPEntry{Index: -1}
		}
	}
	if w.hooks.itemViewport != nil {
		w.hooks.itemViewport(w, o, vpItemRemove) // FUN_00514470
	}
}

// viewportAdd adds new objects in range (FUN_00514700 for players,
// FUN_00514860 for monsters and NPCs).
func (w *World) viewportAdd(o *Object) {
	r := w.viewRange(o.Map)
	if o.Type == TypePlayer {
		for i := 0; i < MaxObjects; i++ {
			t := w.objects[i]
			if i == o.Index || t.State != StatePlaying || t.Map != o.Map ||
				(t.Live != LiveNew && t.Live != LiveNormal) || !inRange(t, o.Map, o.X, o.Y, r) {
				continue
			}
			if t.Type == TypeMonster && (t.Class == 0x20B || t.Class == 0x25B) {
				continue
			}
			vpAdd(&o.VP, i, byte(t.Type))
			vpAdd(&t.VP2, o.Index, byte(o.Type))
		}
		if w.hooks.viewportAdd != nil { // patentes (patente.go)
			w.hooks.viewportAdd(w, o)
		}
		if w.hooks.itemViewport != nil {
			w.hooks.itemViewport(w, o, vpItemAdd) // FUN_00514a00
		}
		return
	}
	for i := SummonStart; i < MaxObjects; i++ {
		t := w.objects[i]
		if i == o.Index || t.State != StatePlaying || t.Map != o.Map ||
			(t.Live != LiveNew && t.Live != LiveNormal) {
			continue
		}
		if inRange(o, t.Map, t.X, t.Y, r) {
			vpAdd(&o.VP, i, byte(t.Type))
			vpAdd(&t.VP2, o.Index, byte(o.Type))
		}
	}
}

// viewportSend is FUN_0050dbf0: destroy packets for removed entries, clear
// them, creation packets for new entries, then new -> visible.
func (w *World) viewportSend(o *Object) {
	if o.State != StatePlaying {
		return
	}
	if o.IsPlayer() {
		var gone []int
		for _, e := range o.VP {
			if e.State == VPRemove && e.Type >= 1 && e.Type <= 3 && e.Index >= 0 && e.Index < MaxObjects {
				gone = append(gone, e.Index)
			}
		}
		if len(gone) > 0 {
			w.send(o, packetDestroy(gone))
		}
		if w.hooks.itemViewport != nil {
			w.hooks.itemViewport(w, o, vpItemDestroy) // FUN_00514e80
		}
	}
	for i := range o.VP {
		if o.VP[i].State == VPRemove {
			o.VP[i] = VPEntry{Index: -1}
		}
	}
	for i := range o.VPItem {
		if o.VPItem[i].State == VPRemove {
			o.VPItem[i] = VPEntry{Index: -1}
		}
	}
	if o.IsPlayer() {
		w.sendNewPlayers(o)
		w.sendNewMonsters(o)
		if w.hooks.itemViewport != nil {
			w.hooks.itemViewport(w, o, vpItemCreate) // FUN_00515b00
		}
		if w.hooks.guildViewport != nil {
			w.hooks.guildViewport(w, o) // FUN_005160a0 / FUN_005162f0
		}
	}
	for i := range o.VP {
		if o.VP[i].State == VPNew {
			o.VP[i].State = VPVisible
		}
	}
	for i := range o.VPItem {
		if o.VPItem[i].State == VPNew {
			o.VPItem[i].State = VPVisible
		}
	}
}

// sendNewPlayers is FUN_00515160.
func (w *World) sendNewPlayers(o *Object) {
	var entries [][]byte
	for _, e := range o.VP {
		if e.State != VPNew || e.Type != byte(TypePlayer) || e.Index < 0 || e.Index >= MaxObjects {
			continue
		}
		t := w.objects[e.Index]
		if t.P == nil || t.P.Char == nil || gmHidden(t) { // gmHidden: /hide (command_gm.go)
			continue
		}
		b := make([]byte, 44)
		viewportPlayerEntry(b, t, t.Live == LiveNew && !t.Teleporting)
		if w.hooks.viewEntry != nil { // arena PK 6 and patentes (custom_world_ext.go)
			w.hooks.viewEntry(w, o, t, b)
		}
		entries = append(entries, b)
	}
	for len(entries) > 0 {
		n := min(len(entries), 0x2000/44-1)
		w.send(o, packetViewportPlayers(entries[:n]))
		entries = entries[n:]
	}
}

// sendNewMonsters is FUN_00515510 (implemented with the monster module).
func (w *World) sendNewMonsters(o *Object) {
	if w.hooks.monsterViewport != nil {
		w.hooks.monsterViewport(w, o)
	}
}

// showSelf is FUN_005167b0: the player's own C2 12 entry, to itself and to
// everyone who already sees it.
func (w *World) showSelf(o *Object) {
	b := make([]byte, 44)
	viewportPlayerEntry(b, o, o.Live == LiveNew && !o.Teleporting)
	if w.hooks.viewEntry != nil { // arena PK 6 and patentes (custom_world_ext.go)
		w.hooks.viewEntry(w, o, o, b)
	}
	p := packetViewportPlayers([][]byte{b})
	w.send(o, p)
	w.broadcast(o, p)
}

// viewportClear removes an object from every viewport (leaving the map,
// teleport, logout): observers get a destroy on their next send.
func (w *World) viewportClear(o *Object) {
	for i := 0; i < MaxObjects; i++ {
		t := w.objects[i]
		if t.State == StateEmpty || t == o {
			continue
		}
		for k := range t.VP {
			if t.VP[k].State != VPEmpty && t.VP[k].Index == o.Index {
				t.VP[k].State = VPRemove
			}
		}
		for k := range t.VP2 {
			if t.VP2[k].State != VPEmpty && t.VP2[k].Index == o.Index {
				t.VP2[k] = VPEntry{Index: -1}
			}
		}
	}
	for i := range o.VP {
		o.VP[i] = VPEntry{Index: -1}
		o.VP2[i] = VPEntry{Index: -1}
		o.VPItem[i] = VPEntry{Index: -1}
	}
}

// viewportProc runs the 1-second viewport cycle for every object
// (FUN_0050a020: 0x50de60 then 0x50dbf0 over the whole table).
func (w *World) viewportProc() {
	for i := 0; i < MaxObjects; i++ {
		o := w.objects[i]
		if o.State != StatePlaying {
			continue
		}
		w.viewportRemove(o)
		w.viewportAdd(o)
	}
	for i := 0; i < MaxObjects; i++ {
		w.viewportSend(w.objects[i])
	}
	for i := 0; i < MaxObjects; i++ {
		if o := w.objects[i]; o.State == StatePlaying && o.Live == LiveNew && o.Alive {
			o.Live = LiveNormal
		}
	}
}
