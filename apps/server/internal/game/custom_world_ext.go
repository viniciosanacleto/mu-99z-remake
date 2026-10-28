package game

// State of the remaining world customs of Data/Custom (block 12, doc 10f):
// CustomArena (customarena.go), CustomEventDrop (customeventdrop.go),
// CustomLukeHelper (lukehelper.go), CustomHealthBar (healthbar.go),
// CustomMonsterKillCount (killcount.go), SummonScroll (summonscroll.go),
// CreationGold (creationgold.go), DailyReward (dailyreward.go),
// CustomPatente (patente.go) and the event time window (windowtime.go).
type worldCustomsExt struct {
	arena    *customArena
	edrop    *customEventDrop
	luke     *lukeHelper
	hpBar    *healthBar
	kills    *killCount
	summon   *summonScroll
	daily    *dailyReward
	patente  []int
	window   map[int]bool // obj+0x24a8: the event window is open
	creation map[int]*creationClick
}

func (w *World) wcx() *worldCustomsExt {
	c := w.wcs()
	if c.x == nil {
		c.x = &worldCustomsExt{window: map[int]bool{}}
	}
	return c.x
}

func registerCustomWorldExt(h *hooks) {
	h.viewEntry = (*World).customViewEntry
	h.viewportAdd = (*World).patenteViewport
}

// customViewEntry: an arena player is shown with PK level 6
// (FUN_00443f00 in FUN_00515160), and the viewer gets the patente of the
// entry's player (the original sent every playing player's, patente.go).
func (w *World) customViewEntry(viewer, t *Object, entry []byte) {
	if w.arenaListed(t) {
		entry[40] = entry[40]&0xF0 | 6
	}
	w.patenteEntry(viewer, t)
}

// arenaOrCustomRespawn is FUN_004c53c0's choice: the arena respawn on the
// arena map when its MapManager RespawnGate is "*" (FUN_004443f0), then
// CustomRespawn (FUN_00458770) at the end, which wins when it has a row.
func (w *World) arenaOrCustomRespawn(o *Object) (int, int, int, bool) {
	am, ax, ay, aok := 0, 0, 0, false
	if info, ok := w.data.Maps[o.Map]; (!ok || info.RespawnGate == -1) && o.Map == w.arena().mapNumber {
		am, ax, ay, aok = w.arenaRespawn(o)
	}
	if m, x, y, ok := w.customRespawn(o); ok {
		return m, x, y, true
	}
	return am, ax, ay, aok
}
