package game

import (
	"bytes"
	"testing"
	"time"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/chaoscastle"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/devilsquare"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/item"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/store"
)

func TestCCPacketLayouts(t *testing.T) {
	r := chaoscastle.PacketResult(1, "Hero", chaoscastle.User{UserKills: 0x01020304, MonsterKills: 7, Exp: 7000})
	want := make([]byte, 0x1D)
	copy(want, []byte{0xC1, 0x1D, 0x93, 1, 0xFE, 'H', 'e', 'r', 'o'})
	copy(want[17:], []byte{7, 0, 0, 0, 0x58, 0x1B, 0, 0, 4, 3, 2, 1})
	if !bytes.Equal(r, want) {
		t.Fatalf("93 = % X", r)
	}
	if s := packetCCStatus(0x0258, 100, 41); !bytes.Equal(s, []byte{0xC1, 0x0E, 0x9B, 6, 0x58, 0x02, 100, 0, 41, 0, 0, 0, 0xFF, 0}) {
		t.Fatalf("9B 06 = % X", s)
	}
	if p := chaoscastle.PacketEnterResult(7); !bytes.Equal(p, []byte{0xC1, 0x05, 0xAF, 0x01, 7}) {
		t.Fatalf("AF 01 = % X", p)
	}
	// FUN_00432570 (CL = 0) and FUN_00432640 (CL = 1), DL = 1 rectangle.
	if p := packetMapAttr(1, false, chaoscastle.SafeRect); !bytes.Equal(p, []byte{0xC1, 0x0B, 0x46, 0, 1, 0, 1, 23, 75, 44, 108}) {
		t.Fatalf("46 set = % X", p)
	}
	if p := packetMapAttr(1, true, chaoscastle.SafeRect); !bytes.Equal(p, []byte{0xC1, 0x0B, 0x46, 0, 1, 1, 1, 23, 75, 44, 108}) {
		t.Fatalf("46 clear = % X", p)
	}
}

// ccWorld is a bare world with every castle in EMPTY at 00:54 (next start
// 01:00).
func ccWorld(t *testing.T) *bareWorld {
	b := newBareWorld(t)
	b.clock = time.Date(2026, 10, 22, 0, 54, 0, 0, time.UTC)
	for i := range b.cc().c {
		b.cc().c[i].State = devilsquare.StateEmpty
		b.ccEnterEmpty(&b.cc().c[i])
	}
	return b
}

func ccEnterPacket(slot int) []byte { return []byte{0xC1, 0x06, 0xAF, 0x01, 0x00, byte(slot)} }

func ccArmor(b *bareWorld, o *Object) int {
	return b.insertInventory(o, item.Item{Index: chaoscastle.ItemArmor, Durability: 1})
}

func TestChaosCastleRun(t *testing.T) {
	b := ccWorld(t)
	c := &b.cc().c[1]
	tr := b.terrain(c.Map)
	if tr.Get(30, 80)&gamedata.AttrSafeZone == 0 || tr.Get(44, 108)&gamedata.AttrSafeZone == 0 || tr.Get(44, 109)&gamedata.AttrSafeZone != 0 ||
		tr.Get(33, 85)&gamedata.AttrNoGround == 0 || tr.Get(33, 97)&gamedata.AttrNoGround == 0 {
		t.Fatalf("safe zone / center holes not set while EMPTY: %X %X %X %X", tr.Get(30, 80), tr.Get(44, 109), tr.Get(33, 85), tr.Get(33, 97))
	}
	a := questPlayer(b, PlayerStart, 1, 50) // castle 1 (50..119)
	d := questPlayer(b, PlayerStart+1, 1, 60)
	a.P.Char.Money, d.P.Char.Money = 200000, 200000
	tick := func(dur time.Duration) {
		b.advance(dur)
		b.ccTick()
	}
	tick(6 * time.Minute)
	if c.State != devilsquare.StateReady || !c.Open {
		t.Fatalf("ready: state %d", c.State)
	}
	// 91 type 4 while READY: users of the castle.
	if out := b.ccRemainTime(a, nil); !bytes.Equal(out, []byte{0xC1, 0x07, 0x91, 4, 0, 0, 0}) {
		t.Fatalf("91 = % X", out)
	}
	for _, o := range []*Object{a, d} {
		slot := ccArmor(b, o)
		b.drain(o)
		b.handleCCEnter(o, ccEnterPacket(slot))
		pk := b.drain(o)
		if p := find(pk, 0xAF, 1); !bytes.Equal(p, chaoscastle.PacketEnterResult(0)) || o.Map != 19 || o.P.Char.Money != 120000 ||
			!o.P.Inventory[slot].IsEmpty() || find(pk, 0x28, -1) == nil || find(pk, 0x22, -1) == nil || o.P.entries.CC != 1 {
			t.Fatalf("enter = % X map %d money %d", p, o.Map, o.P.Char.Money)
		}
	}
	if out := b.ccRemainTime(a, nil); out[5] != 2 {
		t.Fatalf("91 users = % X", out)
	}
	place := func(o *Object, x, y int) {
		o.X, o.Y, o.TX, o.TY = x, y, x, y
		mv(o).OldX, mv(o).OldY, mv(o).Regen = x, y, 0
	}
	place(a, 30, 92)
	place(d, 36, 92)
	if tr.Get(30, 92)&gamedata.AttrNoGround != 0 || tr.Get(36, 92)&gamedata.AttrNoGround != 0 {
		t.Fatal("test cells are holes")
	}
	tick(4*time.Minute + 30*time.Second)
	if p := find(b.drain(a), 0x92, -1); !bytes.Equal(p, []byte{0xC1, 0x04, 0x92, 0x0A}) {
		t.Fatalf("92 0A = % X", p)
	}
	tick(30 * time.Second)
	if c.State != devilsquare.StateStand || a.P.Char.Money+d.P.Char.Money != 240000+128000 {
		t.Fatalf("stand: state %d jackpot %d", c.State, a.P.Char.Money+d.P.Char.Money)
	}
	b.drain(a)
	tick(time.Minute)
	pk := b.drain(a)
	if c.State != devilsquare.StateStart || c.mobCount() != 98 || c.Total != 100 || tr.Get(30, 80)&gamedata.AttrSafeZone != 0 {
		t.Fatalf("start: state %d mobs %d total %d", c.State, c.mobCount(), c.Total)
	}
	if p := find(pk, 0x46, -1); !bytes.Equal(p, packetMapAttr(1, true, chaoscastle.SafeRect)) || find(pk, 0x9B, -1)[3] != 5 {
		t.Fatalf("start packets 46 % X 9B % X", p, find(pk, 0x9B, -1))
	}
	// Step 1 when users + monsters < 41.
	removed := 0
	for i, idx := range c.Mobs {
		if ccIndex(idx) && removed < 60 {
			b.deleteMonster(b.objects[idx])
			c.Mobs[i] = -1
			removed++
		}
	}
	tick(time.Second)
	pk = b.drain(a)
	if c.Step != 1 || tr.Get(23, 75)&gamedata.AttrNoGround == 0 || tr.Get(25, 77)&gamedata.AttrNoGround != 0 {
		t.Fatalf("step %d", c.Step)
	}
	var st []byte
	for _, p := range pk {
		if p[2] == 0x9B && p[3] == 6 {
			st = p
		}
	}
	if find(pk, 0x9B, -1)[3] != 8 || st == nil || st[6] != 100 || st[8] != 40 {
		t.Fatalf("9B 08 / status % X", st)
	}
	// A monster standing on a hole is moved; d falls into one.
	var mob *Object
	for _, idx := range c.Mobs {
		if ccIndex(idx) {
			mob = b.objects[idx]
			break
		}
	}
	place(mob, 23, 76)
	place(d, 44, 90)
	b.drain(d)
	tick(time.Second)
	if tr.Get(mob.X, mob.Y)&gamedata.AttrNoGround != 0 || (mob.X == 23 && mob.Y == 76) {
		t.Fatalf("monster still on a hole at %d,%d", mob.X, mob.Y)
	}
	if c.user(d.Index) != nil || d.Alive || find(b.drain(d), 0x17, -1) == nil || len(c.Delayed) != 1 {
		t.Fatal("fall not processed")
	}
	// a kills a monster far from everyone: score 2, one monster kill.
	place(mob, 60, 60)
	mob.Mon.Hits[0] = damageEntry{Index: a.Index, Damage: 1, Time: b.now()}
	b.ccMonsterKilled(a, mob)
	if u := c.user(a.Index); u.Score != 2 || u.MonsterKills != 1 || c.mobSlot(mob.Index) >= 0 {
		t.Fatalf("kill score %+v", *u)
	}
	// The delayed result of d after 8 s.
	tick(8 * time.Second)
	if r := find(b.drain(d), 0x93, -1); r == nil || r[3] != 0 || r[4] != 0xFE {
		t.Fatalf("delayed 93 = % X", r)
	}
	// No monster left and one user: CLEAN, a wins.
	b.ccClearMobs(c)
	var drops []item.Item
	b.hooks.dropMapItem = func(w *World, mp, x, y int, it item.Item, owner int) bool { drops = append(drops, it); return true }
	b.drain(a)
	tick(time.Second)
	pk = b.drain(a)
	r := find(pk, 0x93, -1)
	if c.State != devilsquare.StateClean || c.Winner != a.Index || r == nil || r[3] != 1 || r[4] != 0xFE || r[17] != 1 ||
		r[21] != byte(7000&0xFF) || r[22] != byte(7000>>8) {
		t.Fatalf("clean: state %d winner %d result % X", c.State, c.Winner, r)
	}
	// FUN_00476ce0 reports the user's +4 field (the score bestUser ranks).
	if n := b.eventRank(t, store.RankingChaosCastle, a.P.Char.Name); n != 20 {
		t.Fatalf("cc ranking %d", n)
	}
	if len(drops) != 7 || drops[0].Index != 0x1C0B || drops[0].Level != 12 || drops[5].Index != 0x1C0E || drops[6].Index != 0x180F {
		t.Fatalf("drops %+v", drops)
	}
	tick(4 * time.Minute)
	if c.State != devilsquare.StateEmpty || c.userCount() != 0 || a.Map == 19 || c.Step != 0 || tr.Get(23, 75)&gamedata.AttrNoGround != 0 {
		t.Fatalf("empty: state %d users %d map %d", c.State, c.userCount(), a.Map)
	}
}

func TestChaosCastleStandCancel(t *testing.T) {
	b := ccWorld(t)
	c := &b.cc().c[1]
	c.State, c.Open = devilsquare.StateReady, true
	c.setRemain(b.now(), 1)
	b.advance(2 * time.Second)
	b.ccTick()
	if c.State != devilsquare.StateStand {
		t.Fatalf("state %d", c.State)
	}
	// ChaosCastleMinUser = 1 and no user: back to EMPTY.
	b.advance(time.Second)
	b.ccTick()
	if c.State != devilsquare.StateEmpty {
		t.Fatalf("state %d", c.State)
	}
}

func TestChaosCastleEntryChecks(t *testing.T) {
	b := ccWorld(t)
	c := &b.cc().c[1]
	c.Open = true
	o := questPlayer(b, PlayerStart, 1, 50)
	o.P.Char.Money = 1000000
	try := func() []byte {
		b.drain(o)
		slot := -1
		for s, it := range o.P.Inventory {
			if !it.IsEmpty() && it.Index == chaoscastle.ItemArmor {
				slot = s
			}
		}
		if slot < 0 {
			slot = 12
		}
		b.handleCCEnter(o, ccEnterPacket(slot))
		pk := b.drain(o)
		if p := find(pk, 0xAF, 1); p != nil {
			return p
		}
		return find(pk, 0xF3, 0xE4)
	}
	expect := func(name string, r byte) {
		t.Helper()
		if p := try(); !bytes.Equal(p, chaoscastle.PacketEnterResult(r)) {
			t.Fatalf("%s: % X", name, p)
		}
	}
	notice := func(name string) {
		t.Helper()
		if p := try(); p == nil || p[2] != 0xF3 {
			t.Fatalf("%s: % X", name, p)
		}
	}
	expect("no armor", 1)
	ccArmor(b, o)
	o.P.Inventory[8] = item.Item{Index: 0x1A03, Durability: 255}
	notice("dinorant")
	o.P.Inventory[8] = item.Empty()
	o.P.PShop = &pshopState{Open: true}
	expect("personal shop", 1)
	o.P.PShop = nil
	o.Duel = &duelState{Opponent: PlayerStart + 5, Asked: -1, AskedBy: -1}
	expect("duel", 1)
	o.Duel = nil
	o.P.Inventory[10] = item.Item{Index: 0x1A0A, Durability: 10}
	notice("transformation ring")
	o.P.Inventory[10] = item.Empty()
	c.Open = false
	expect("closed", 2)
	c.Open = true
	o.P.Char.Money = 79999
	expect("zen", 7)
	o.P.Char.Money = 80000
	o.PKLevel = 5
	expect("pk", 8)
	o.PKLevel = 3
	for i := range c.Users {
		c.Users[i].Index = PlayerStart + 100 + i
	}
	expect("full", 5)
	for i := range c.Users {
		c.Users[i] = chaoscastle.User{Index: -1}
	}
	o.P.Char.Level = 10
	notice("level")
	o.P.Char.Level = 50
	expect("ok", 0)
	if o.P.Char.Money != 0 || o.Map != 19 {
		t.Fatalf("money %d map %d", o.P.Char.Money, o.Map)
	}
}

func TestChaosCastlePushAndKills(t *testing.T) {
	b := ccWorld(t)
	c := &b.cc().c[0]
	c.State = devilsquare.StateStart
	a := questPlayer(b, PlayerStart, 1, 20)
	v := questPlayer(b, PlayerStart+1, 1, 20)
	for i, o := range []*Object{a, v} {
		o.Map = 18
		c.Users[i] = chaoscastle.User{Index: o.Index}
	}
	set := func(o *Object, x, y int) { o.X, o.Y, o.TX, o.TY = x, y, x, y; mv(o).OldX, mv(o).OldY = x, y }
	// Same cell: random directions (0 = +1), distance 0 -> 4 cells.
	set(a, 30, 92)
	fixRand(t, 0)
	b.ccPush(a, 30, 92)
	if a.X != 34 || a.Y != 96 || mv(a).CCPushTick != b.now() {
		t.Fatalf("push to %d,%d", a.X, a.Y)
	}
	// dx = 2: x away (+1), y random (odd = -1); dist 2.
	b.advance(2 * time.Second)
	set(a, 32, 92)
	fixRand(t, 1, 1, 0)
	b.ccPush(a, 30, 92)
	if a.X != 35 || a.Y != 90 {
		t.Fatalf("push to %d,%d", a.X, a.Y)
	}
	// AF 02 moves a user inside its castle.
	b.advance(2 * time.Second)
	b.handleCCPacket(a, []byte{0xC1, 0x06, 0xAF, 0x02, 33, 91})
	if a.X != 33 || a.Y != 91 {
		t.Fatalf("AF 02 to %d,%d", a.X, a.Y)
	}
	// A user killed by another user.
	b.ccPlayerKilled(a, v)
	if u := c.user(a.Index); u.Score != 1 || u.UserKills != 1 || c.user(v.Index) != nil || len(c.Delayed) != 1 {
		t.Fatalf("pvp kill %+v", *u)
	}
	// Experience: table1 * users + table2 * monsters, then the event rate.
	u := c.user(a.Index)
	u.MonsterKills = 2
	b.ccExperience(c, u)
	if want := (1000*1 + 500*2) * 70; u.Exp != want || u.Score != 2000/5 {
		t.Fatalf("experience %+v want %d", *u, want)
	}
	// The PvP / walk rules see START.
	if b.hooks.chaosCastleState(b.World, 0) != devilsquare.StateStart || b.hooks.chaosCastleState(b.World, 6) != 0 {
		t.Fatal("state hook")
	}
}
