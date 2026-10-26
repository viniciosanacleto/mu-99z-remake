package game

import (
	"math"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/frame"
)

// CCustomHealthBar (Custom/CustomHealthBar.txt, FUN_0044c3b0) and the
// monster health bar packet C2 F3 E2 (FUN_004dc610), sent every second to
// each player (FUN_00510810) on a map whose MapManager MonsterHPBar is 1
// (-1 = MonsterHealthBarSwitch of GameServerInfo - Common.dat): one entry
// per visible monster whose CustomHealthBar row has BarSwitch 1.

type healthBar struct {
	rows []gamedata.HealthBarRow
}

func (w *World) hpBar() *healthBar {
	x := w.wcx()
	if x.hpBar == nil {
		x.hpBar = &healthBar{}
	}
	return x.hpBar
}

func init() {
	startHooks = append(startHooks, func(w *World) error {
		w.wcx().hpBar = nil
		if w.configDir == "" {
			return nil
		}
		rows, err := gamedata.LoadCustomHealthBar(w.configDir)
		w.hpBar().rows = rows
		return err
	})
	tick1000Hooks = append(tick1000Hooks, (*World).healthBarSecond)
}

// healthBarMap is FUN_004ae830: the MapManager MonsterHPBar flag, "*" =
// the global switch.
func (w *World) healthBarMap(mp int) bool {
	if info, ok := w.data.Maps[mp]; ok && info.MonsterHPBar != -1 {
		return info.MonsterHPBar == 1
	}
	return w.cfg.Int("Common", "MonsterHealthBarSwitch", 0) != 0
}

// healthPercent is Life * 100 / (MaxLife + AddLife) rounded by fistp
// (round half to even).
func healthPercent(o *Object) byte {
	max := float64(objMaxLife(o))
	if max == 0 {
		return 0
	}
	return byte(int(math.RoundToEven(float64(objLife(o)) * 100 / max)))
}

// packetHealthBar is FUN_004dc610 for one player: C2 size F3 E2 [5] count,
// then {index u16 LE, object type, percent} for every viewport monster
// (state new or visible, alive, live state normal, not a summon) with a
// switched-on row.
func (w *World) packetHealthBar(o *Object) []byte {
	var entries [][4]byte
	for _, e := range o.VP {
		if (e.State != VPNew && e.State != VPVisible) || e.Type != byte(TypeMonster) || !validIdx(e.Index) {
			continue
		}
		m := w.objects[e.Index]
		if !m.Alive || m.Live != LiveNormal || (m.Mon != nil && validIdx(m.Mon.Owner)) {
			continue
		}
		if gamedata.HealthBarSwitch(w.hpBar().rows, m.Class, m.Map) == 0 {
			continue
		}
		entries = append(entries, [4]byte{byte(m.Index), byte(m.Index >> 8), byte(m.Type), healthPercent(m)})
	}
	p := newPacket(frame.C2, 0xF3, 0xE2, 6+4*len(entries))
	p[5] = byte(len(entries))
	for i, e := range entries {
		copy(p[6+4*i:], e[:])
	}
	return p
}

// healthBarSecond runs the health bar part of FUN_00510810.
func (w *World) healthBarSecond() {
	for i := PlayerStart; i < MaxObjects; i++ {
		o := w.objects[i]
		if o.State != StatePlaying || !o.IsPlayer() || !w.healthBarMap(o.Map) {
			continue
		}
		w.send(o, w.packetHealthBar(o))
	}
}
