package game

import (
	"bytes"
	"testing"
	"time"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/item"
)

// secondMonster places another monster of m's class dx cells from p.
func secondMonster(t *testing.T, b *bareWorld, m, p *Object, dx int) *Object {
	t.Helper()
	for i := 0; i < SummonStart; i++ {
		o := b.objects[i]
		if o != m && o.State == StatePlaying && o.Mon != nil && o.Map == m.Map && o.Class == m.Class {
			b.monsterOccupy(o, false)
			o.Move = nil
			o.X, o.Y, o.TX, o.TY = p.X+dx, p.Y, p.X+dx, p.Y
			o.Live = LiveNormal
			b.monsterOccupy(o, true)
			return o
		}
	}
	t.Fatal("no second monster")
	return nil
}

// ravenOwner is a level 400 Dark Lord with a level 10 Dark Raven in slot 1.
func ravenOwner(t *testing.T, b *bareWorld) *Object {
	t.Helper()
	o := statsPlayer(b, PlayerStart, 4, 400)
	o.P.Char.Leadership = 2000
	o.P.Inventory[1] = item.Item{Index: idxDarkRaven, Durability: 255, PetLevel: 10}
	b.calcCharacter(o)
	b.drain(o)
	if !pet(o).Owner {
		t.Fatal("raven not active")
	}
	return o
}

func TestRavenPackets(t *testing.T) {
	if p := packetRavenAttack(true, 0x1234, 0x0567); !bytes.Equal(p, []byte{0xC1, 9, 0xA8, 0, 1, 0x12, 0x34, 0x05, 0x67}) {
		t.Fatalf("A8 = % X", p)
	}
	if p := packetRavenReset(); !bytes.Equal(p, []byte{0xC1, 7, 0xA7, 0, 0, 0xFF, 0xFF}) {
		t.Fatalf("A7 reset = % X", p)
	}
}

func TestRavenSyncResetsMode(t *testing.T) {
	b := newBareWorld(t)
	o := ravenOwner(t, b)
	ps := pet(o)
	ps.Mode = 2
	// Re-equipping (owner off -> on) drops the mode with A7 00 00 FF FF.
	o.P.Inventory[1] = item.Empty()
	b.calcCharacter(o)
	if ps.Owner || ps.Target != -1 {
		t.Fatalf("no raven: %+v", ps)
	}
	o.P.Inventory[1] = item.Item{Index: idxDarkRaven, Durability: 255, PetLevel: 10}
	b.calcCharacter(o)
	if r := find(b.drain(o), 0xA7, -1); !bytes.Equal(r, packetRavenReset()) || ps.Mode != 0 || !ps.Owner {
		t.Fatalf("reset = % X mode %d", r, ps.Mode)
	}
	// A gate move also drops the mode (FUN_0050f6d0).
	ps.Mode = 1
	b.ravenReset(o)
	if ps.Mode != 0 || find(b.drain(o), 0xA7, -1) == nil {
		t.Fatal("gate move kept the mode")
	}
}

func TestRavenRandomAttackTiming(t *testing.T) {
	b := newBareWorld(t)
	o := ravenOwner(t, b)
	m := b.monsterNear(0, o, 2)
	b.see(o, m)
	ps := pet(o)
	ps.Mode = 1
	fixRand(t, 0)
	b.ravenTick(o)
	r := find(b.drain(o), 0xA8, -1)
	if !bytes.Equal(r, packetRavenAttack(false, o.Index, m.Index)) {
		t.Fatalf("A8 = % X", r)
	}
	if want := b.now().Add(time.Duration((150-ps.AttackSpeed)*10) * time.Millisecond); !ps.Next.Equal(want) {
		t.Fatalf("next = %v, want %v", ps.Next, want)
	}
	st := sk(o)
	if len(st.Pending) != 1 || st.Pending[0].Code != 0x33 || st.Pending[0].Target != m.Index || !st.Pending[0].Send ||
		st.Pending[0].Param != 0 || !st.Pending[0].Due.Equal(b.now().Add(600*time.Millisecond)) {
		t.Fatalf("pending = %+v", st.Pending)
	}
	// Not before the attack delay.
	b.ravenTick(o)
	if find(b.drain(o), 0xA8, -1) != nil {
		t.Fatal("attacked twice")
	}
	// The hit lands 600 ms later (message 0x33 -> FUN_0046b5a0).
	m.Mon.Life, m.Mon.MaxLife = 100000, 100000
	life := m.Mon.Life
	fixRand(t, 99) // hit roll (rand % rate >= defense rate), no critical
	b.advance(601 * time.Millisecond)
	b.skillMessages(o)
	if find(b.drain(o), 0xD7, -1) == nil {
		t.Fatal("no damage packet")
	}
	if m.Mon.Life >= life && m.Alive {
		t.Fatalf("life %v -> %v", life, m.Mon.Life)
	}
	// Out of 5 cells nothing is picked; a safe zone stops the raven.
	ps.Next = time.Time{}
	m.X = o.X + 6
	b.ravenTick(o)
	if find(b.drain(o), 0xA8, -1) != nil {
		t.Fatal("attacked a far monster")
	}
}

func TestRavenTargetModes(t *testing.T) {
	b := newBareWorld(t)
	o := ravenOwner(t, b)
	m := b.monsterNear(0, o, 2)
	n := secondMonster(t, b, m, o, 3)
	b.see(o, m)
	b.see(o, n)
	ps := pet(o)
	ps.Mode = 2
	// The owner's attacks give the target (FUN_0046b370).
	b.ravenSetTarget(o, m)
	if ps.Target != m.Index {
		t.Fatalf("target = %d", ps.Target)
	}
	// rand%100 = 99 >= DarkSpiritRangeAttackRate (30): single strike.
	fixRand(t, 99)
	b.ravenTick(o)
	if r := find(b.drain(o), 0xA8, -1); r == nil || r[4] != 0 {
		t.Fatalf("strike = % X", r)
	}
	// rand%100 = 0: the splash attack (main target + neighbours, 60%).
	sk(o).Pending = nil
	ps.Next = time.Time{}
	fixRand(t, 0)
	b.ravenTick(o)
	if r := find(b.drain(o), 0xA8, -1); r == nil || r[4] != 1 {
		t.Fatalf("splash = % X", r)
	}
	st := sk(o)
	if len(st.Pending) != 2 || st.Pending[0].Target != m.Index || st.Pending[1].Target != n.Index ||
		st.Pending[1].Send || st.Pending[1].Param != 1 || st.Pending[0].Param != 1 {
		t.Fatalf("splash queue = %+v", st.Pending)
	}
	// Mode 3 with a dead target: back to mode 0 (FUN_0046b3b0).
	ps.Mode, ps.Target, ps.Next = 3, m.Index, time.Time{}
	m.Alive = false
	b.ravenTick(o)
	if r := find(b.drain(o), 0xA7, -1); !bytes.Equal(r, packetRavenReset()) || ps.Mode != 0 || ps.Target != -1 {
		t.Fatalf("lost target = % X %+v", r, ps)
	}
}

func TestRavenOfflineHelperRests(t *testing.T) {
	b := newBareWorld(t)
	o := ravenOwner(t, b)
	m := b.monsterNear(0, o, 2)
	b.see(o, m)
	pet(o).Mode = 1
	custom(o).Offline.Helper = true
	b.ravenTick(o)
	if find(b.drain(o), 0xA8, -1) != nil {
		t.Fatal("raven acted for an offline helper")
	}
}

func TestRavenHitAndDamage(t *testing.T) {
	if r, h := func() (bool, bool) { fixRand(t, 4); return ravenHitCheck(10, 20) }(); !r || !h {
		t.Fatal("low rate: 5% chance")
	}
	if r, h := func() (bool, bool) { fixRand(t, 5); return ravenHitCheck(10, 20) }(); !r || h {
		t.Fatal("low rate miss")
	}
	if _, h := func() (bool, bool) { fixRand(t, 30); return ravenHitCheck(100, 30) }(); !h {
		t.Fatal("rand >= defense rate hits")
	}
	if ravenDefense(CombatStats{Defense: 51}, effectOptions{AddDefense: 9}) != 30 {
		t.Fatal("defense is halved")
	}

	b := newBareWorld(t)
	o := ravenOwner(t, b)
	ps := pet(o)
	// No scepter: min + rand%(max-min); rand%100 = 99 is no critical.
	fixRand(t, 99)
	eff := 0
	if d := b.ravenDamage(o, 10, &eff); d != ps.DamageMin+99%(ps.DamageMax-ps.DamageMin)-10 || eff != 0 {
		t.Fatalf("damage = %d", d)
	}
	// rand%100 = 0 < DarkSpiritCriticalDamageRate (30): the maximum.
	fixRand(t, 0)
	if d := b.ravenDamage(o, 10, &eff); d != ps.DamageMax-10 || eff != effectCritical {
		t.Fatalf("critical = %d (%d)", d, eff)
	}
	// A scepter raises both ends by (magic power / 2 + level * 2) * factor %.
	o.P.Inventory[0] = item.Item{Index: item.Make(2, 8), Durability: 255}
	o.P.Char.Strength, o.P.Char.Dexterity = 2000, 2000
	b.calcCharacter(o)
	r := b.statsOf(o).Items[0]
	if !b.statsOf(o).Valid[0] || r.MagicPower == 0 {
		t.Skip("scepter not usable by the test character")
	}
	pc := int(float32(int(uint32(r.MagicPower)>>1)+r.Level*2) * r.DurFactor)
	fixRand(t, 0)
	if d := b.ravenDamage(o, 0, &eff); d != ps.DamageMax+pc*ps.DamageMax/100 {
		t.Fatalf("scepter damage = %d (pc %d)", d, pc)
	}
}

func TestRavenAttackRules(t *testing.T) {
	b := newBareWorld(t)
	o := ravenOwner(t, b)
	m := b.monsterNear(0, o, 2)
	// Physical immunity: miss (D7 with no damage).
	b.addEffect(m, effMonPhysiImmunity, 0, [4]int{}, false)
	life := m.Mon.Life
	if !b.ravenAttack(o, m, true, false) || m.Mon.Life != life {
		t.Fatal("immune monster hit")
	}
	if r := find(b.drain(o), 0xD7, -1); r == nil || r[5] != 0 || r[6] != 0 {
		t.Fatalf("miss = % X", r)
	}
	b.removeEffect(m, effMonPhysiImmunity)
	// The splash hit is 60% of the single one (same rolls).
	fixRand(t, 99)
	single := m.Mon.Life
	b.ravenAttack(o, m, true, false)
	single -= m.Mon.Life
	m.Mon.Life = m.Mon.MaxLife
	fixRand(t, 99)
	splash := m.Mon.Life
	b.ravenAttack(o, m, true, true)
	splash -= m.Mon.Life
	if single <= 0 || splash >= single {
		t.Fatalf("single %v splash %v", single, splash)
	}
	// The hit makes the monster the raven target.
	if pet(o).Target != m.Index {
		t.Fatal("target not set by the raven hit")
	}
}

func TestRavenDurability(t *testing.T) {
	b := newBareWorld(t)
	o := ravenOwner(t, b)
	// PetDurabilityRate 100: the counter fills at 800 (2% of the damage).
	b.ravenDurability(o, 39950)
	if o.P.Inventory[1].Durability != 255 || pet(o).DurCount != 799 {
		t.Fatalf("counter = %d", pet(o).DurCount)
	}
	b.ravenDurability(o, 50)
	it := o.P.Inventory[1]
	if it.Durability != 254 || pet(o).DurCount != 0 {
		t.Fatalf("durability %v counter %d", it.Durability, pet(o).DurCount)
	}
	out := b.drain(o)
	if r := find(out, 0x2A, -1); !bytes.Equal(r, []byte{0xC1, 6, 0x2A, 1, 254, 0}) {
		t.Fatalf("2A = % X", r)
	}
	if r := find(out, 0xA9, -1); !bytes.Equal(r, packetPetInfo(0, 0, 1, it.PetLevel, it.PetExp)) {
		t.Fatalf("A9 = % X", r)
	}
	// PetDamageTable.txt row 5 (rate 1000) keeps the damage.
	if b.petDamageRate(o, 5, 1234) != 1234 || len(b.q().petDamage) != 13 {
		t.Fatalf("pet damage table = %+v", b.q().petDamage)
	}
	b.q().petDamage = []petDamageRow{{Index: 5, MinLevel: -1, MaxLevel: -1, MinReset: -1, MaxReset: -1, Rate: 500}}
	if b.petDamageRate(o, 5, 1000) != 500 || b.petDamageRate(o, 4, 1000) != 1000 {
		t.Fatal("pet damage rate")
	}
}
