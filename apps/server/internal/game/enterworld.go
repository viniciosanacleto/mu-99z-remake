package game

import (
	"bytes"
	"context"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/charset"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/item"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/store"
)

// fruitLimit reproduces the CFruit tables (FUN_0047c350 / FUN_0047c400).
func fruitLimit(class, level int) int {
	if level < 1 || level > 1000 {
		return 0
	}
	normal, mg, dl := 2, 2, 2
	v := 0x21
	for l := 1; l <= level; l++ {
		if l%10 == 0 {
			normal += 2 + v/400
			mg += 2 + v/700
			dl += 2 + v/500
		}
		v += 3
	}
	switch class {
	case 3:
		return mg
	case 4:
		return dl
	}
	return normal
}

// classDefaultSkills are the skills FUN_004cc730 grants after loading the
// MagicList (the requirement checks of AddSkill still apply).
func classDefaultSkills(class int, quest []byte) []int {
	var s []int
	switch class {
	case 0:
		s = []int{0x11, 0x2D}
	case 1:
		s = []int{0x2C}
	case 2:
		s = []int{0x2E}
		if len(quest) > 0 && quest[0]&0x30 == 0x20 {
			s = append(s, 0x4D)
		}
	case 3:
		s = []int{0x39, 0x49}
	case 4:
		s = []int{0x3C, 0x4A}
	case 5:
		s = []int{0x2D}
	case 6:
		s = []int{0x10D}
	}
	return append(s, 0x43, 0x44, 0x45, 0x46, 0x47, 0x48)
}

// addSkill is FUN_004f8b60 for players: requirement checks, no duplicates,
// first free of the 60 slots. Returns the slot or -1.
func (w *World) addSkill(o *Object, index int, level byte) int {
	info, ok := w.data.Skills[index]
	if !ok {
		return -1
	}
	c := o.P.Char
	if !info.CanLearn(c.Level, c.Energy, c.Leadership, o.Class) {
		return -1
	}
	free := -1
	for i, s := range o.P.Skills {
		if s.Index == index {
			return -1
		}
		if s.Index < 0 && free < 0 {
			free = i
		}
	}
	if free >= 0 {
		o.P.Skills[free] = Skill{Index: index, Level: level}
	}
	return free
}

// decodeMagicList reads the 60x3-byte blob: [0] index low, [1] level,
// [2] index high; "FF ?? 00" and "FF ?? FF" are empty.
func decodeMagicList(b []byte) [MagicSlots]Skill {
	var out [MagicSlots]Skill
	for i := range out {
		out[i].Index = -1
		if len(b) < (i+1)*3 {
			continue
		}
		lo, lvl, hi := b[i*3], b[i*3+1], b[i*3+2]
		if lo == 0xFF && hi == 0 {
			hi = 0xFF
		}
		idx := int(hi)<<8 | int(lo)
		if idx != 0xFFFF {
			out[i] = Skill{Index: idx, Level: lvl}
		}
	}
	return out
}

func encodeMagicList(skills [MagicSlots]Skill) []byte {
	b := make([]byte, store.MagicListSize)
	for i, s := range skills {
		r := b[i*3:]
		if s.Index < 0 {
			r[0], r[1], r[2] = 0xFF, 0, 0xFF
			continue
		}
		r[0], r[1], r[2] = byte(s.Index), s.Level, byte(s.Index>>8)
	}
	return b
}

// enterWorld builds the playing object from the database row
// (FUN_004cc730) and sends the world entry packets (FUN_00473a10).
func (w *World) enterWorld(o *Object, ch *store.Character) {
	if !validDBClass(ch.Class) {
		w.closeClient(o, "invalid class")
		return
	}
	p := o.P
	p.Char = ch
	o.Class = int(ch.Class >> 4)
	o.Map, o.X, o.Y, o.Dir = ch.Map, ch.X, ch.Y, ch.Dir
	o.PKLevel = ch.PKLevel

	if _, ok := w.data.Maps[o.Map]; !ok || w.terrain(o.Map) == nil {
		o.Map = 0
		o.X, o.Y = w.randomGatePosition(w.respawnGate(0))
	} else if info := w.data.Maps[o.Map]; info.SpawnGate >= 0 {
		o.X, o.Y = w.randomGatePosition(info.SpawnGate)
	}
	if srv := w.data.MapServer.ServerForMap(w.cfg.ServerCode, o.Map); srv != w.cfg.ServerCode {
		// The character is on a map hosted by another instance: the original
		// asks the JoinServer for a server move (FUN_004a2820).
		w.requestServerMove(o, srv)
		return
	}

	for i := range p.Inventory {
		p.Inventory[i] = item.Empty()
		if (i+1)*store.ItemDBSize <= len(ch.Inventory) {
			if it, ok := item.DecodeDB(ch.Inventory[i*store.ItemDBSize:]); ok {
				p.Inventory[i] = it
			}
		}
	}
	p.Skills = decodeMagicList(ch.MagicList)
	for _, s := range classDefaultSkills(o.Class, ch.Quest) {
		w.addSkill(o, s, 0)
	}
	for _, h := range characterLoadHooks { // saved effects (effect_save.go)
		h(w, o)
	}

	if floor := w.data.Experience.Floor(ch.Level); uint32(ch.Experience) < floor {
		ch.Experience = int64(floor)
	}
	p.NextExp = w.data.Experience.Next(ch.Level)
	isNew := ch.IsNew // DataServer 04 IsNewChar (packet +0x8f0), cleared at load
	ch.IsNew = false

	o.State = StatePlaying
	o.Live = LiveNew
	o.Alive = true
	o.TX, o.TY = o.X, o.Y
	p.LastSave = w.now()
	w.occupy(o, true)
	w.rebuildCharSet(o)

	w.send(o, packetCharInfo(w.charInfo(o)))
	w.send(o, packetInventory(p.Inventory[:], nil))
	w.send(o, packetSkills(p.Skills[:]))
	o.P.questSent = false // FUN_0050ae50 clears obj+0x5c0 on character load
	w.sendQuestList(o)    // FUN_00473a10 -> FUN_004dde70
	w.showSelf(o)
	for _, h := range enterWorldHooks {
		h(w, o)
	}
	if isNew && newCharacterHook != nil { // DefaultClassFreebies (freebies.go)
		newCharacterHook(w, o)
	}
	w.log.Info("AddCharacterInfo", "index", o.Index, "name", ch.Name, "account", p.Account.Login)
}

func (w *World) charInfo(o *Object) charInfo {
	c := o.P.Char
	life := int(roundf(c.Life))
	maxLife := int(roundf(float32(o.P.AddLife) + c.MaxLife))
	mana := int(roundf(c.Mana))
	maxMana := int(roundf(float32(o.P.AddMana) + c.MaxMana))
	fruit := fruitLimit(o.Class, c.Level)
	return charInfo{
		X: o.X, Y: o.Y, Map: o.Map, Dir: o.Dir,
		Exp: uint32(c.Experience), NextExp: o.P.NextExp, LevelUpPoints: c.LevelUpPoints,
		Str: c.Strength, Agi: c.Dexterity, Vit: c.Vitality, Ene: c.Energy,
		Life: life, MaxLife: maxLife, Mana: mana, MaxMana: maxMana,
		BP: int(c.BP), MaxBP: int(c.MaxBP) + o.P.AddBP,
		Money: uint32(c.Money), PKLevel: c.PKLevel, CtlCode: c.CtlCode,
		FruitAdd: c.FruitAddPoints, MaxFruit: fruit, Leadership: c.Leadership,
		FruitSub: c.FruitSubPoints, MaxFruitS: fruit, Resets: uint32(c.Resets), Level: c.Level,
		Extra: uint32(c.Patente), // obj+0x2500 Patente (customevent.go)
	}
}

func roundf(f float32) float32 {
	if f < 0 {
		return -roundf(-f)
	}
	return float32(int(f + 0.5))
}

// rebuildCharSet is FUN_004c6ba0.
func (w *World) rebuildCharSet(o *Object) {
	var in charset.Input
	in.DBClass = o.P.Char.Class
	in.ActionState = o.PoseState
	for k := 0; k < charset.Slots; k++ {
		it := o.P.Inventory[k]
		if it.IsEmpty() {
			in.Items[k] = charset.Piece{Index: -1}
			continue
		}
		in.Items[k] = charset.Piece{Index: it.Index, Level: it.Level, Excellent: it.Excellent, SetOption: it.SetOption}
	}
	o.CharSet = charset.Build(in, w.data.Custom)
	// FUN_004c6ba0 runs CalcCharacter (FUN_004c9790) every time.
	if recalcHook != nil {
		recalcHook(w, o)
	}
}

// terrain returns the attribute grid of a map.
func (w *World) terrain(mp int) *gamedata.Terrain {
	if mp < 0 || mp >= gamedata.MaxMaps {
		return nil
	}
	return w.terrains[mp]
}

// occupy sets or clears the 0x02 "character" attribute of the object's cell.
func (w *World) occupy(o *Object, on bool) {
	t := w.terrain(o.Map)
	a := t.Get(o.X, o.Y)
	if on {
		t.Set(o.X, o.Y, a|gamedata.AttrCharacter)
	} else {
		t.Set(o.X, o.Y, a&^gamedata.AttrCharacter)
	}
}

// respawnGate returns the town gate of a map (MapManager RespawnGate, with
// Lorencia's gate 17 as fallback).
func (w *World) respawnGate(mp int) int {
	if info, ok := w.data.Maps[mp]; ok && info.RespawnGate >= 0 {
		return info.RespawnGate
	}
	return 17
}

// randomGatePosition picks a walkable cell inside a gate's rectangle.
func (w *World) randomGatePosition(gate int) (int, int) {
	g, ok := w.data.Gates[gate]
	if !ok {
		return 125, 125
	}
	t := w.terrain(g.Map)
	for tries := 0; tries < 50; tries++ {
		x := g.X1 + w.rand(g.X2-g.X1+1)
		y := g.Y1 + w.rand(g.Y2-g.Y1+1)
		if t.Get(x, y)&(gamedata.AttrBlock|gamedata.AttrNoGround|gamedata.AttrCharacter) == 0 {
			return x, y
		}
	}
	return g.X1, g.Y1
}

// leaveWorld removes a playing character from the map and viewports.
func (w *World) leaveWorld(o *Object) {
	for _, h := range leaveWorldHooks {
		h(w, o)
	}
	w.occupy(o, false)
	w.viewportClear(o)
}

// characterSaveHooks run when saveCharacter builds the DataServer 0x30
// record (FUN_00475c30 also sends the pet save 0x34); they may adjust it.
var characterSaveHooks []func(w *World, o *Object, c *store.Character)

// saveCharacter writes the character (DataServer 0x30, FUN_00475c30).
func (w *World) saveCharacter(o *Object, done func(error)) {
	p := o.P
	if p == nil || p.Char == nil {
		return
	}
	c := *p.Char
	c.Map, c.X, c.Y, c.Dir = o.Map, o.X, o.Y, o.Dir
	c.PKLevel = o.PKLevel
	inv := make([]byte, store.InventorySize)
	for i, it := range p.Inventory {
		it.EncodeDB(inv[i*store.ItemDBSize:])
	}
	c.Inventory = inv
	c.MagicList = encodeMagicList(p.Skills)
	c.Quest = bytes.Clone(p.Char.Quest)
	c.EffectList = bytes.Clone(p.Char.EffectList)
	for _, h := range characterSaveHooks {
		h(w, o, &c)
	}
	w.async(func(ctx context.Context) func() {
		err := w.store.SaveCharacter(ctx, &c)
		if err != nil {
			w.log.Error("save character", "name", c.Name, "err", err)
		}
		if done == nil {
			return nil
		}
		return func() { done(err) }
	})
}

// requestServerMove sends the character to the instance that hosts its map.
// Server moves are completed in servermove.go.
func (w *World) requestServerMove(o *Object, serverCode int) {
	w.startServerMove(o, serverCode, o.Map, o.X, o.Y)
}
