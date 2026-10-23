package game

import (
	"testing"
	"time"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/item"
)

// kalimaGateOf returns the gate object a player owns.
func (b *bareWorld) kalimaGateOf(o *Object) (*Object, *kalimaGate) {
	b.t.Helper()
	if !o.P.hasGate {
		b.t.Fatal("no gate")
	}
	g := b.objects[o.P.gate]
	return g, b.kalima().gates[g.Index]
}

// kalimaMonster creates a monster of class at (mp, x, y).
func (b *bareWorld) kalimaMonster(class, mp, x, y int) *Object {
	b.t.Helper()
	o := b.objects[b.allocMonsterIndex()]
	o.reset()
	o.State = StateConnected
	o.Map, o.X, o.Y, o.TX, o.TY = mp, x, y, x, y
	if !b.setMonster(o, class) {
		b.t.Fatalf("class %d", class)
	}
	o.Live = LiveNormal
	return o
}

func TestLostMapOpensGate(t *testing.T) {
	b := newBareWorld(t)
	x, y := b.openRow(0, 6)
	p := b.player(PlayerStart, "Owner", 0, x, y)
	p.P.Inventory[BagStart] = item.Item{Index: itemLostMap, Level: 1, Durability: 1}
	p.P.Inventory[BagStart+1] = item.Item{Index: itemLostMap, Level: 3, Durability: 1}

	b.handleDrop(p, []byte{0xC1, 0x06, 0x23, byte(x + 2), byte(y), BagStart})
	if r := find(b.drain(p), 0x23, -1); r == nil || r[3] != 1 {
		t.Fatalf("drop result %x", r)
	}
	if !p.P.Inventory[BagStart].IsEmpty() {
		t.Fatal("lost map not consumed")
	}
	g, kg := b.kalimaGateOf(p)
	if g.Class != 152 || g.Mon.Attribute != kalimaGateAttr || g.X != x+2 || g.Y != y || g.Mon.MoveRange != 0 ||
		kg.Owner != p.Index || kg.Level != 0 || kg.Count != 0 {
		t.Fatalf("gate class %d attr %d at %d,%d %+v", g.Class, g.Mon.Attribute, g.X, g.Y, kg)
	}

	// A second Lost Map is refused while the gate is open (obj+0x2ac).
	b.handleDrop(p, []byte{0xC1, 0x06, 0x23, byte(x + 3), byte(y), BagStart + 1})
	if r := find(b.drain(p), 0x23, -1); r == nil || r[3] != 0 || p.P.Inventory[BagStart+1].IsEmpty() {
		t.Fatalf("second drop %x", r)
	}
}

func TestLostMapRefusals(t *testing.T) {
	b := newBareWorld(t)
	x, y := b.openRow(0, 4)
	p := b.player(PlayerStart, "Owner", 0, x, y)
	for _, lvl := range []int{0, 8} {
		if b.lostMapDrop(p, lvl, x+1, y) {
			t.Fatalf("level %d accepted", lvl)
		}
	}
	for _, mp := range []int{9, 10, 11, 17, 18, 23, 24, 29, 32, 36} {
		if !noLostMapMap(mp) {
			t.Fatalf("map %d allowed", mp)
		}
	}
	for _, mp := range []int{0, 2, 3, 7, 8, 30, 33} {
		if noLostMapMap(mp) {
			t.Fatalf("map %d refused", mp)
		}
	}
	// From a safe zone (Lorencia town).
	sx, sy := -1, -1
	for yy := 100; yy < 160 && sx < 0; yy++ {
		for xx := 100; xx < 160; xx++ {
			if b.attrAt(0, xx, yy) == 1 {
				sx, sy = xx, yy
				break
			}
		}
	}
	if sx < 0 {
		t.Fatal("no safe cell")
	}
	p.X, p.Y = sx, sy
	if b.lostMapDrop(p, 1, x+1, y) {
		t.Fatal("accepted from a safe zone")
	}
}

func TestKalimaGateTakesOwnerAndParty(t *testing.T) {
	b := newBareWorld(t)
	x, y := b.openRow(0, 8)
	owner := b.player(PlayerStart, "Owner", 0, x, y)
	mate := b.player(PlayerStart+1, "Mate", 0, x+1, y)
	other := b.player(PlayerStart+2, "Other", 0, x+3, y)
	b.partyStateOf(owner).Number, b.partyStateOf(mate).Number, b.partyStateOf(other).Number = 7, 7, 8
	if !b.lostMapDrop(owner, 2, x+2, y) {
		t.Fatal("no gate")
	}
	g, kg := b.kalimaGateOf(owner)
	g.Live = LiveNormal
	for _, p := range []*Object{other, mate, owner} {
		b.see(p, g)
	}

	b.advance(time.Second)
	b.kalimaTick()
	if owner.Map != 0 || mate.Map != 0 {
		t.Fatal("moved before 2 s")
	}
	b.advance(time.Second)
	b.kalimaTick()
	// Gate 0x59 is Kalima 2 (map 25); the owner leaving the map deletes
	// the gate (FUN_004a3480 from gObjMoveGate).
	if mate.Map != 25 || owner.Map != 25 || other.Map != 0 {
		t.Fatalf("maps mate %d owner %d other %d", mate.Map, owner.Map, other.Map)
	}
	if kg.Count != 2 || owner.P.hasGate || b.objects[g.Index].State != StateEmpty {
		t.Fatalf("count %d hasGate %v gate state %d", kg.Count, owner.P.hasGate, b.objects[g.Index].State)
	}
}

func TestKalimaGateLifetime(t *testing.T) {
	b := newBareWorld(t)
	x, y := b.openRow(0, 6)
	owner := b.player(PlayerStart, "Owner", 0, x, y)
	watcher := b.player(PlayerStart+1, "Watcher", 0, x+4, y)
	if !b.lostMapDrop(owner, 1, x+2, y) {
		t.Fatal("no gate")
	}
	g, _ := b.kalimaGateOf(owner)
	g.Live = LiveNormal
	b.see(watcher, g)
	b.advance(179 * time.Second)
	b.kalimaTick()
	if !owner.P.hasGate {
		t.Fatal("gate gone before 180 s")
	}
	b.drain(watcher)
	b.advance(time.Second)
	b.kalimaTick()
	if owner.P.hasGate || b.objects[g.Index].State != StateEmpty {
		t.Fatal("gate kept after 180 s")
	}
	if d := find(b.drain(watcher), 0x17, -1); d == nil || int(d[3])<<8|int(d[4]) != g.Index || d[5] != 0 {
		t.Fatalf("die packet %x", d)
	}

	// An owner that is no longer usable (logging out) loses the gate but
	// keeps obj+0x2ac set, like the original.
	owner.P.hasGate = false
	if !b.lostMapDrop(owner, 1, x+2, y) {
		t.Fatal("no second gate")
	}
	g, _ = b.kalimaGateOf(owner)
	g.Live = LiveNormal
	owner.P.LogoutCountdown = 3
	b.kalimaTick()
	if b.objects[g.Index].State != StateEmpty || !owner.P.hasGate {
		t.Fatalf("gate state %d hasGate %v", b.objects[g.Index].State, owner.P.hasGate)
	}
}

func TestKundunOpensNextGate(t *testing.T) {
	b := newBareWorld(t)
	b.kalima().next = [6]int{1, 1, 1, 1, 1, 0}
	x, y := b.openRow(24, 6)
	p := b.player(PlayerStart, "Hunter", 24, x, y)
	k := b.kalimaMonster(0xA1, 24, x+2, y)
	b.monsterDie(k, p.Index)
	g, kg := b.kalimaGateOf(p)
	if g.Class != 0x99 || kg.Level != 1 || g.Map != 24 || objDistance(g, k) > 4 {
		t.Fatalf("gate class %#x level %d map %d at %d,%d", g.Class, kg.Level, g.Map, g.X, g.Y)
	}

	// Kalima.dat row 5 = 0: the Kundun of Kalima 6 opens nothing.
	q := b.player(PlayerStart+1, "Late", 24, x, y+1)
	k6 := b.kalimaMonster(0x152, 24, x+3, y)
	b.monsterDie(k6, q.Index)
	if q.P.hasGate {
		t.Fatal("gate from a disabled level")
	}
	// Outside Kalima nothing happens.
	b.kalima().next[5] = 1
	q.Map, k6.Map = 0, 0
	b.kundunDie(k6, q.Index)
	if q.P.hasGate {
		t.Fatal("gate outside Kalima")
	}
}

func TestKundunRegen(t *testing.T) {
	b := newBareWorld(t)
	x, y := b.openRow(36, 2)
	k := b.kalimaMonster(kundunClass, 36, x, y)
	k.Mon.Life = 100
	b.kalimaTick()
	if want := min(100+k.Mon.MaxLife/4, k.Mon.MaxLife); k.Mon.Life != want {
		t.Fatalf("life %v want %v", k.Mon.Life, want)
	}
	life := k.Mon.Life
	b.advance(299 * time.Second)
	b.kalimaTick()
	if k.Mon.Life != life {
		t.Fatal("regen before 300 s")
	}
	b.advance(time.Second)
	b.kalimaTick()
	if k.Mon.Life <= life && life < k.Mon.MaxLife {
		t.Fatal("no regen after 300 s")
	}
}
