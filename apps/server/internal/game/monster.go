package game

import (
	"fmt"
	"math/rand/v2"
	"time"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/monsterskill"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/packet"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/frame"
)

// Monsters and NPCs: data (CMonsterManager FUN_004ba0c0, CMonsterSetBase
// FUN_004bbaa0), index allocation (FUN_004b4b00 / FUN_0050bee0), creation
// (FUN_004b4fe0), start-up spawn (FUN_004baa20), death/regen state
// (FUN_004c37f0 / FUN_004c3b10) and the C2 13 viewport (FUN_00515510).

// monsterRand is the generator behind FUN_005138d0/FUN_00513720: a
// non-negative random int. Tests replace it with a fixed sequence.
var monsterRand = func() int { return int(rand.Int32()) }

// rnd returns monsterRand() % n like the original "rand() % n" (n <= 0
// returns 0).
func rnd(n int) int {
	if n <= 0 {
		return 0
	}
	return monsterRand() % n
}

// monsterWorld is the per-instance monster data.
type monsterWorld struct {
	info   map[int]*gamedata.MonsterInfo
	spawns []gamedata.SpawnRecord
	talk   []gamedata.NpcTalkEntry
	damage [2][]damageRule
	drops  [150][]int         // CItemDrop level table (FUN_004ba760)
	tables *dropTables        // drop.go: drop/option/bag/set data files
	skills *monsterskill.Data // monster_skill.go: Monster/Skill/*.txt

	summonOf   map[int]int // summon.go: player index -> summon (obj+0x310)
	nextSummon int         // DAT_00772d48: summon round-robin pointer

	next      int                // DAT_00772d4c: round-robin pointer 0..0x5013
	allocated [SummonStart]bool  // object already created once (FUN_004b2620)
	freed     []freedIndex       // FUN_004b2840 pool, in insertion order
	deaths    map[int]*deadState // players waiting for respawn (obj+0x286)
}

// freedIndex is one entry of the deleted-index pool.
type freedIndex struct {
	Index int
	Time  time.Time
}

// deadState is the death bookkeeping of a player (obj+0x285/0x286/0x28c).
type deadState struct {
	Die      int       // obj+0x286: 1 dead, 2 regen pending
	Time     time.Time // obj+0x28c
	Respawn  int       // obj+0x285: 0 normal, 1 killed by a monster
	KillerID int
}

// monsterMsg is one of the 20 delayed messages of an object
// (DAT_01090774, FUN_00510550): code, sender, due time, parameter.
type monsterMsg struct {
	Code   int
	Sender int
	Due    time.Time
	Param  int
	Used   bool
}

// damageEntry is one of the 40 hit records (obj+0x3d0, 16 bytes):
// player index, accumulated damage, last hit time.
type damageEntry struct {
	Index  int
	Damage int
	Time   time.Time
}

// monsterState is the monster/NPC part of an object.
type monsterState struct {
	Info       *gamedata.MonsterInfo
	SpawnIndex int // obj+0x294 (-1 = not from a spawn record)
	StartX     int // obj+0x1c8
	StartY     int // obj+0x1c9

	Life, MaxLife float32 // obj+0x110 / 0x114
	Mana, MaxMana float32 // obj+0x11c / 0x120
	Level         int     // obj+0xe6
	DamageMin     int     // obj+0x320
	DamageMax     int     // obj+0x324
	Defense       int     // obj+0x368
	MagicDefense  int     // obj+0x36c
	AttackRate    int     // obj+0x35c
	DefenseRate   int     // obj+0x370
	AttackSpeed   int     // obj+0x360
	MoveRange     int     // obj+0x376
	AttackRange   int     // obj+0x378
	AttackType    int     // obj+0x37a
	ViewRange     int     // obj+0x37c
	Attribute     int     // obj+0x37e
	ItemRate      int     // obj+0x380
	MoneyRate     int     // obj+0x382
	RegenTime     int     // obj+0x290 (ms)
	Resistance    [7]byte // obj+0x3a8
	Owner         int     // obj+0x310 (summoner, -1)
	SpecialBag    bool    // obj+0x24b4 (CustomMonster SpecialBag: no normal drop)
	Invasion      int     // obj+0x24a4 invasion index (-1 = none), invasion.go
	Skills        []int   // obj+0x388 MagicList of the monster (FUN_004f8b60), monster_skill.go

	// AI state (CMonster).
	ActAttack    bool      // obj+0x258
	ActMove      bool      // obj+0x259
	Emotion      int       // obj+0x25a
	EmotionCount int       // obj+0x25b
	Target       int       // obj+0x318
	LastAttacker int       // obj+0x31e
	Delay        int       // obj+0x264 (ms)
	LastAct      time.Time // obj+0x260
	MTX, MTY     int       // obj+0x1d2 / 0x1d4

	Die      int       // obj+0x286
	DieTime  time.Time // obj+0x28c
	ExpGiven int64     // obj+0xf8
	Hits     [40]damageEntry
	Msgs     [20]monsterMsg
}

func (w *World) mw() *monsterWorld {
	if w.mon == nil {
		w.mon = &monsterWorld{info: map[int]*gamedata.MonsterInfo{}, deaths: map[int]*deadState{}}
	}
	return w.mon
}

// loadMonsterData is the start-up part of FUN_004eb5f0 for monsters.
func (w *World) loadMonsterData() error {
	mw := w.mw()
	rates := gamedata.MonsterRates{
		MaxLife:            w.cfg.Int("Common", "MonsterMaxLifeRate", 100),
		Defense:            w.cfg.Int("Common", "MonsterDefenseRate", 100),
		DefenseSuccessRate: w.cfg.Int("Common", "MonsterDefenseSuccessRateRate", 100),
		PhysiDamage:        w.cfg.Int("Common", "MonsterPhysiDamageRate", 100),
		AttackSuccessRate:  w.cfg.Int("Common", "MonsterAttackSuccessRateRate", 100),
	}
	var err error
	if mw.info, err = gamedata.LoadMonsterList(w.configDir, rates); err != nil {
		return err
	}
	mapOK := func(mp int) bool { _, ok := w.data.Maps[mp]; return ok }
	if mw.spawns, err = gamedata.LoadSpawns(w.configDir, mapOK, monsterRand); err != nil {
		return err
	}
	if mw.talk, err = gamedata.LoadNpcTalk(w.configDir); err != nil {
		return err
	}
	if mw.damage, err = loadDamageTable(w.configDir); err != nil {
		return err
	}
	mw.drops = buildDropTable(w.data.Items)
	return nil
}

// allocMonsterIndex is FUN_004b4b00: first an index freed more than 10 s
// ago (the longest-freed one, FUN_004b2580), otherwise the next empty slot
// from the round-robin pointer over 0..0x5013.
func (w *World) allocMonsterIndex() int {
	mw := w.mw()
	now := w.now()
	best, bestAge := -1, time.Duration(0)
	for i, f := range mw.freed {
		age := now.Sub(f.Time)
		if age > 10*time.Second && age > bestAge && w.objects[f.Index].State == StateEmpty {
			best, bestAge = i, age
		}
	}
	if best >= 0 {
		idx := mw.freed[best].Index
		mw.freed = append(mw.freed[:best], mw.freed[best+1:]...)
		return idx
	}
	p := mw.next
	for n := 0; n < SummonStart; n++ {
		if w.objects[p].State == StateEmpty {
			mw.unfree(p)
			if !mw.allocated[p] {
				// FUN_0050bee0 advances the pointer only when the object
				// is created for the first time.
				mw.allocated[p] = true
				mw.next = (mw.next + 1) % SummonStart
			}
			return p
		}
		p = (p + 1) % SummonStart
	}
	return -1
}

// deleteMonster is FUN_0050c4a0 for monsters and NPCs.
func (w *World) deleteMonster(o *Object) {
	if o.Mon != nil && o.Alive {
		w.monsterOccupy(o, false)
	}
	w.viewportClear(o)
	idx := o.Index
	o.reset()
	if idx < SummonStart {
		mw := w.mw()
		mw.unfree(idx)
		mw.freed = append(mw.freed, freedIndex{idx, w.now()})
	}
}

func (mw *monsterWorld) unfree(idx int) {
	for i, f := range mw.freed {
		if f.Index == idx {
			mw.freed = append(mw.freed[:i], mw.freed[i+1:]...)
			return
		}
	}
}

// monsterCell returns the cell holding the object's 0x02 bit (the walk
// target kept by the movement module, otherwise the position).
func monsterCell(o *Object) (int, int) {
	if o.Move != nil {
		return o.Move.OldX, o.Move.OldY
	}
	return o.X, o.Y
}

func (w *World) monsterOccupy(o *Object, on bool) {
	x, y := monsterCell(o)
	t := w.terrain(o.Map)
	if t == nil || x < 0 || y < 0 || x >= t.Width || y >= t.Height {
		return
	}
	a := t.Get(x, y)
	if on {
		t.Set(x, y, a|gamedata.AttrCharacter)
	} else {
		t.Set(x, y, a&^gamedata.AttrCharacter)
	}
}

// spawnPosition is FUN_004bbff0: sections 0 and 4 use the fixed cell,
// sections 1 and 3 a random cell of the rectangle and section 2 a random
// cell of the 6x6 box around the (already jittered) position.
func (w *World) spawnPosition(r gamedata.SpawnRecord) (int, int, bool) {
	switch r.Section {
	case gamedata.SpawnNPC, gamedata.SpawnEvent:
		return r.X, r.Y, true
	case gamedata.SpawnArea, gamedata.SpawnInvasion:
		return w.randomAreaCell(r.Map, r.X, r.Y, r.TX, r.TY)
	case gamedata.SpawnSingle:
		return w.randomAreaCell(r.Map, r.X-3, r.Y-3, r.X+3, r.Y+3)
	}
	return 0, 0, false
}

// randomAreaCell is FUN_004bc080: up to 100 tries of
// (x1 + rand%max(x2-x1,1), y1 + rand%max(y2-y1,1)) rejecting cells with
// the 0x01, 0x04 or 0x08 attribute (out of the map counts as 0x04).
func (w *World) randomAreaCell(mp, x1, y1, x2, y2 int) (int, int, bool) {
	t := w.terrain(mp)
	if t == nil {
		return 0, 0, false
	}
	dx, dy := max(x2-x1, 1), max(y2-y1, 1)
	for try := 0; try < 100; try++ {
		x := monsterRand()%dx + x1
		y := monsterRand()%dy + y1
		in := x >= 0 && y >= 0 && x < t.Width && y < t.Height
		var a byte = gamedata.AttrBlock
		if in {
			a = t.Get(x, y)
		}
		if in && a&gamedata.AttrSafeZone == 0 && a&(gamedata.AttrBlock|gamedata.AttrNoGround) == 0 {
			return x, y, true
		}
	}
	return 0, 0, false
}

// setMonsterPosition is FUN_004b5810.
func (w *World) setMonsterPosition(o *Object, spawn int) bool {
	mw := w.mw()
	if spawn < 0 || spawn >= len(mw.spawns) {
		return false
	}
	r := mw.spawns[spawn]
	x, y, ok := w.spawnPosition(r)
	if !ok {
		return false
	}
	o.X, o.Y, o.TX, o.TY = x, y, x, y
	o.Dir, o.Map = r.Dir, r.Map
	if o.Mon == nil {
		o.Mon = &monsterState{}
	}
	o.Mon.SpawnIndex = spawn
	o.Mon.StartX, o.Mon.StartY = x&0xFF, y&0xFF
	return true
}

// setMonster is FUN_004b4fe0: copy the MonsterList row into the object,
// mark it playing/alive/new and set the 0x02 bit of its cell.
func (w *World) setMonster(o *Object, class int) bool {
	info := w.mw().info[class]
	if info == nil {
		return false
	}
	m := o.Mon
	if m == nil {
		m = &monsterState{SpawnIndex: -1, StartX: o.X & 0xFF, StartY: o.Y & 0xFF}
		o.Mon = m
	}
	o.State, o.Alive, o.Live = StatePlaying, true, LiveNew
	o.Teleporting = false
	o.Class = class
	o.Type = TypeMonster
	if info.Type != 0 {
		o.Type = TypeNPC
	}
	m.Info = info
	m.Level = info.Level
	m.AttackSpeed = info.AttackSpeed
	m.DamageMin, m.DamageMax = info.DamageMin, info.DamageMax
	m.Defense, m.MagicDefense = info.Defense, info.MagicDefense
	m.AttackRate, m.DefenseRate = info.AttackRate, info.DefenseRate
	m.Life, m.MaxLife = float32(info.MaxLife), float32(info.MaxLife)
	m.Mana, m.MaxMana = float32(info.MaxMana), float32(info.MaxMana)
	m.MoveRange, m.AttackRange, m.AttackType = info.MoveRange, info.AttackRange, info.AttackType
	m.ViewRange, m.Attribute = info.ViewRange, info.Attribute
	m.ItemRate, m.MoneyRate = info.ItemRate, info.MoneyRate
	m.RegenTime = info.RegenTime * 1000
	for i, r := range info.Resistance {
		m.Resistance[i] = byte(min(r, 0xFF))
	}
	m.Target, m.LastAttacker, m.Owner = -1, -1, -1
	m.Invasion = -1
	m.Die = 0
	m.SpecialBag = false
	w.applyCustomMonster(o) // FUN_00466d40
	m.Skills = monsterskill.List(class, info.AttackType)
	// The movement module keeps the occupied cell and the walk speed
	// (obj+0x374 = MoveSpeed).
	o.Move = nil
	w.setMoveSpeed(o, info.MoveSpeed)
	w.monsterOccupy(o, true)
	return true
}

// spawnMonsters is FUN_004baa20: every record except sections 3 and 4.
func (w *World) spawnMonsters() {
	mw := w.mw()
	for i, r := range mw.spawns {
		if r.Section == gamedata.SpawnInvasion || r.Section == gamedata.SpawnEvent {
			continue
		}
		idx := w.allocMonsterIndex()
		if idx < 0 {
			return
		}
		o := w.objects[idx]
		o.reset()
		o.State = StateConnected // obj+4 = 1 while being built
		if !w.setMonsterPosition(o, i) || !w.setMonster(o, r.Class) {
			w.deleteMonster(o)
		}
	}
}

// regenPosition is FUN_004b4e40: reset the AI and pick a new cell: in
// place for Attribute 6, near the death cell for Attribute 5/7/8/9, from
// the spawn record otherwise.
func (w *World) regenPosition(o *Object) bool {
	m := o.Mon
	m.Target, m.LastAttacker = -1, -1
	m.Delay = 5000
	m.ActAttack = false
	switch m.Attribute {
	case 6: // regenerates in place (FUN_0050d930/FUN_0050df60/FUN_0050da50)
		m.Die = 0
		o.Live = LiveNew
		w.monsterOccupy(o, true)
		return false
	case 5, 7, 8, 9: // a free cell within +-5 of the death cell (30 tries)
		if x, y, ok := w.randomNearCell(o.Map, o.X, o.Y, 5, 5, 30); ok {
			o.X, o.Y, o.TX, o.TY = x, y, x, y
			m.MTX, m.MTY = x, y
			m.StartX, m.StartY = x&0xFF, y&0xFF
			o.Move = nil
			w.setMoveSpeed(o, m.Info.MoveSpeed)
			return true
		}
		o.Alive = false
		o.Live = LiveDying
		m.Die = 1
		m.DieTime = w.now()
		return false
	}
	if m.SpawnIndex >= 0 {
		r := w.mw().spawns[m.SpawnIndex]
		if x, y, ok := w.spawnPosition(r); ok {
			o.X, o.Y, o.TX, o.TY = x, y, x, y
			m.MTX, m.MTY = x, y
			o.Move = nil
			w.setMoveSpeed(o, m.Info.MoveSpeed)
			return true
		}
	}
	o.Alive = false
	o.Live = LiveDying
	m.Die = 1
	m.DieTime = w.now()
	return false
}

// monsterStateProc runs every second for monsters and NPCs: the dead
// timer of FUN_004c37f0 (RegenTime + 1000 ms after death the object is
// removed from the viewports, live state 8) and the regen of FUN_004c3b10.
func (w *World) monsterStateProc() {
	now := w.now()
	for i := 0; i < PlayerStart; i++ { // summons included (FUN_004c3b10 scans every object)
		o := w.objects[i]
		if o.State != StatePlaying || o.Mon == nil {
			continue
		}
		m := o.Mon
		switch m.Die {
		case 2:
			if m.Attribute == 3 {
				w.deleteMonster(o)
				continue
			}
			o.Alive = true
			o.ActionState = 0
			o.Teleporting = false
			m.Life = m.MaxLife
			m.Mana = m.MaxMana
			if w.regenPosition(o) {
				m.Die = 0
				o.Live = LiveNew
				w.monsterOccupy(o, true)
			}
		case 1:
			if now.Sub(m.DieTime) > time.Duration(m.RegenTime+1000)*time.Millisecond {
				m.Die = 2
				o.Live = LiveRemoved
				w.monsterOccupy(o, false)
			}
		}
	}
}

// monsterViewportEntry writes one 16-byte C2 13 entry (FUN_00515510):
// [0..1] index BE (bit 7 = new, bit 6 = teleporting when new), [2..3]
// class BE, [4..7] obj+0x274 view skill state LE, [8] X [9] Y [10] TX
// [11] TY [12] dir<<4 | obj+0x19c&0x0F, [13..15] padding.
func monsterViewportEntry(dst []byte, t *Object) {
	dst[0], dst[1] = byte(t.Index>>8), byte(t.Index)
	if t.Live == LiveNew {
		dst[0] |= 0x80
		if t.Teleporting {
			dst[0] |= 0x40
		}
	}
	dst[2], dst[3] = byte(t.Class>>8), byte(t.Class)
	packet.PutUint32(dst[4:], t.ViewSkillState)
	dst[8], dst[9], dst[10], dst[11] = byte(t.X), byte(t.Y), byte(t.TX), byte(t.TY)
	dst[12] = byte(t.Dir)<<4 | byte(t.PKLevel)&0x0F
}

// packetViewportMonsters is C2 size 13 count + 16-byte entries.
func packetViewportMonsters(entries [][]byte) []byte {
	p := packet.New(frame.C2, 0x13, -1, 5+16*len(entries))
	p[4] = byte(len(entries))
	for i, e := range entries {
		copy(p[5+16*i:], e)
	}
	return p
}

// sendMonsterViewport is FUN_00515510: new VP entries of type 2/3 that
// are not summons (obj+0x310 invalid).
func (w *World) sendMonsterViewport(o *Object) {
	var entries [][]byte
	for _, e := range o.VP {
		if e.State != VPNew || (e.Type != byte(TypeMonster) && e.Type != byte(TypeNPC)) ||
			e.Index < 0 || e.Index >= MaxObjects {
			continue
		}
		t := w.objects[e.Index]
		if t.Mon != nil && t.Mon.Owner >= 0 && t.Mon.Owner < MaxObjects {
			continue
		}
		b := make([]byte, 16)
		monsterViewportEntry(b, t)
		if w.hooks.monsterEntry != nil { // castle objects (castlesiege_hooks.go)
			w.hooks.monsterEntry(w, o, t, b)
		}
		entries = append(entries, b)
	}
	if len(entries) > 0 {
		w.send(o, packetViewportMonsters(entries))
	}
	w.sendSummonViewport(o) // FUN_00515890 (C2 1F)
}

// monsterByClass returns the first monster object of a class on a map
// (used by tests and tools).
func (w *World) monsterByClass(mp, class int) *Object {
	for i := 0; i < SummonStart; i++ {
		o := w.objects[i]
		if o.State == StatePlaying && o.Mon != nil && o.Map == mp && o.Class == class {
			return o
		}
	}
	return nil
}

func registerMonster(h *hooks) {
	h.monsterViewport = (*World).sendMonsterViewport
	h.start = append(h.start, func(w *World) error {
		if err := w.loadMonsterData(); err != nil {
			return fmt.Errorf("monsters: %w", err)
		}
		w.spawnMonsters()
		return nil
	})
	h.tick100 = append(h.tick100, (*World).monsterTick)
	h.tick1000 = append(h.tick1000, (*World).monsterStateProc, (*World).playerDeathProc)
}

// spawnFixedNPC places an NPC of class at a fixed cell (CShopManager
// FUN_004f55e0 spawns the shop NPCs this way, outside the Spawn files).
func (w *World) spawnFixedNPC(class, mp, x, y, dir int) *Object {
	idx := w.allocMonsterIndex()
	if idx < 0 {
		return nil
	}
	o := w.objects[idx]
	o.reset()
	o.State = StateConnected
	o.Map, o.X, o.Y, o.TX, o.TY, o.Dir = mp, x, y, x, y, dir
	if !w.setMonster(o, class) {
		w.deleteMonster(o)
		return nil
	}
	return o
}

func registerSpawnNPC(h *hooks) {
	h.spawnNPC = func(w *World, class, mp, x, y, dir int) *Object { return w.spawnFixedNPC(class, mp, x, y, dir) }
}
