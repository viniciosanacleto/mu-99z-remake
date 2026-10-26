package game

import (
	"strings"
	"testing"
	"time"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/config/gsconfig"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"
)

// cdConfig loads an instance configuration with CastleDeepEvent = 1 (the
// shipped files have it off).
func cdConfig(t *testing.T, instance string) *gsconfig.Config {
	t.Helper()
	cfg := instanceConfig(t, instance, map[string]map[string]string{"Event": {"CastleDeepEvent": "1"}})
	if cfg.Int("Event", "CastleDeepEvent", 0) != 1 || cfg.Int("Event", "CastleDeepEventTime", 0) != 30 {
		t.Fatal("config not patched")
	}
	return cfg
}

func cdWorld(t *testing.T, at time.Time) *bareWorld {
	b := newBareWorld(t)
	b.cfg = cdConfig(t, "4.GameServer")
	b.clock = at
	b.cdInit()
	return b
}

func cdMobs(b *bareWorld) map[int]int {
	out := map[int]int{}
	for i := 0; i < SummonStart; i++ {
		if o := b.objects[i]; o.State == StatePlaying && o.Mon != nil && o.Mon.Attribute == cdAttribute {
			out[o.Class]++
		}
	}
	return out
}

func cdCount(m map[int]int) int {
	n := 0
	for _, v := range m {
		n += v
	}
	return n
}

// cdNotice returns the text of the first C1 0D notice.
func cdNotice(pkts [][]byte) string {
	n := find(pkts, 0x0D, -1)
	if n == nil {
		return ""
	}
	return strings.TrimRight(string(n[13:]), "\x00")
}

func TestCastleDeepOffByDefault(t *testing.T) {
	b := newBareWorld(t)
	cd := b.deep()
	if cd == nil || cd.State != cdEmpty || !cd.Target.IsZero() || len(cd.data.Monsters) != 59 {
		t.Fatalf("state %+v", cd)
	}
	b.advance(time.Hour)
	b.cdTick()
	if cd.Remain != 0 || cd.State != cdEmpty {
		t.Fatalf("ticked while off: %+v", cd)
	}
}

func TestCastleDeepCycle(t *testing.T) {
	b := cdWorld(t, time.Date(2026, 10, 22, 11, 54, 0, 0, time.UTC))
	fixRand(t, 10) // group 1 (10 < 50); every cell draw uses 10
	cd := b.deep()
	if cd.State != cdEmpty || cd.Stage != 1 || cd.Group != 0 || cd.Remain != 360 ||
		!cd.Target.Equal(time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)) {
		t.Fatalf("empty %+v", cd)
	}
	o := b.player(PlayerStart, "Watcher", 0, 130, 130)

	// 301 s left: no warning yet; 299 s: the 5-minute warning, once.
	b.advance(59 * time.Second)
	b.cdTick()
	if cd.Warned || len(b.drain(o)) != 0 {
		t.Fatal("warned too early")
	}
	b.advance(2 * time.Second)
	b.cdTick()
	if txt := cdNotice(b.drain(o)); !cd.Warned || txt != b.messageFor(nil, cdMsgWarn) {
		t.Fatalf("warning %q", txt)
	}
	b.advance(time.Second)
	b.cdTick()
	if len(b.drain(o)) != 0 {
		t.Fatal("warning repeated")
	}

	// 12:00: start notice, START for CastleDeepEventTime minutes, group 1.
	b.clock = cd.Target
	b.cdTick()
	if txt := cdNotice(b.drain(o)); txt != b.messageFor(nil, cdMsgStart) {
		t.Fatalf("start notice %q", txt)
	}
	if cd.State != cdStart || cd.Group != 1 || cd.Remain != 1800 || cdCount(cdMobs(b)) != 0 {
		t.Fatalf("start %+v", cd)
	}

	// Next second: elapsed minute 0 = stage 1 of group 1 (10 rows x 15).
	b.advance(time.Second)
	b.cdTick()
	mobs := cdMobs(b)
	if cd.Stage != 2 || cdCount(mobs) != 150 || mobs[39] != 15 || mobs[62] != 15 {
		t.Fatalf("stage 1: stage %d mobs %v", cd.Stage, mobs)
	}
	for i := 0; i < SummonStart; i++ {
		m := b.objects[i]
		if m.State != StatePlaying || m.Mon == nil || m.Mon.Attribute != cdAttribute {
			continue
		}
		if m.Map != cdMap || m.Mon.RegenTime != cdRegen || m.Mon.SpawnIndex != -1 || m.Dir != 10%8 {
			t.Fatalf("monster map %d regen %d spawn %d dir %d", m.Map, m.Mon.RegenTime, m.Mon.SpawnIndex, m.Dir)
		}
		// FUN_0050c860 around (60,28): +-10 with the fixed draws, else the corner.
		if !(m.X == 50 && m.Y == 18) && !(m.X == 60 && m.Y == 28) {
			t.Fatalf("cell %d,%d", m.X, m.Y)
		}
		break
	}
	// Minute 9 does nothing; minute 10 spawns stage 2.
	b.clock = cd.Target.Add(-21 * time.Minute)
	b.cdTick()
	if cd.Stage != 2 {
		t.Fatal("stage 2 too early")
	}
	b.clock = cd.Target.Add(-20 * time.Minute)
	b.cdTick()
	if mobs := cdMobs(b); cd.Stage != 3 || cdCount(mobs) != 300 || mobs[69] != 15 {
		t.Fatalf("stage 2: %d %v", cd.Stage, mobs)
	}
	b.clock = cd.Target.Add(-10 * time.Minute)
	b.cdTick()
	// Classes 49 and 293 are not in MonsterList.txt: FUN_004b4fe0 fails and
	// the index is freed, so stage 3 adds 71 instead of 91.
	if mobs := cdMobs(b); cd.Stage != 4 || cdCount(mobs) != 371 || mobs[295] != 1 || mobs[49]+mobs[293] != 0 {
		t.Fatalf("stage 3: %d %v", cd.Stage, mobs)
	}
	b.drain(o)

	// End: local notice 54, every attribute-7 monster removed, next start.
	b.clock = cd.Target
	b.cdTick()
	if txt := cdNotice(b.drain(o)); txt != b.messageFor(o, cdMsgEnd) {
		t.Fatalf("end notice %q", txt)
	}
	if cd.State != cdEmpty || cd.Stage != 1 || cd.Group != 0 || cd.Warned || cdCount(cdMobs(b)) != 0 ||
		!cd.Target.Equal(time.Date(2026, 10, 22, 20, 0, 0, 0, time.UTC)) {
		t.Fatalf("end %+v", cd)
	}
}

func TestCastleDeepGroupDraw(t *testing.T) {
	b := cdWorld(t, time.Date(2026, 10, 22, 11, 0, 0, 0, time.UTC))
	cd := b.deep()
	// Rows are tried in file order; the first rand%100 < rate wins and the
	// draw repeats until one does.
	fixRand(t, 60, 40)
	cd.Group = 0
	b.cdEnterStart()
	if cd.Group != 2 {
		t.Fatalf("group %d", cd.Group)
	}
	fixRand(t, 99, 99, 149)
	cd.Group = 0
	b.cdEnterStart()
	if cd.Group != 1 {
		t.Fatalf("redraw group %d", cd.Group)
	}
	// No row can win: the original never returns; here the group stays 0.
	cd.data = &gamedata.CastleDeep{Groups: []gamedata.CastleDeepGroup{{Group: 1, Rate: 0}}}
	cd.Group = 0
	b.cdEnterStart()
	if cd.Group != 0 || cd.Remain != 1800 {
		t.Fatalf("undrawable %+v", cd)
	}
}

func TestCastleDeepNoScheduleDisables(t *testing.T) {
	b := cdWorld(t, time.Date(2026, 10, 22, 11, 0, 0, 0, time.UTC))
	cd := b.deep()
	cd.data = &gamedata.CastleDeep{}
	b.cdEnterEmpty()
	if cd.State != cdDisabled {
		t.Fatalf("state %d", cd.State)
	}
	b.advance(time.Hour)
	b.cdTick()
	if cd.State != cdDisabled {
		t.Fatal("left DISABLED")
	}
}

func TestCastleDeepKillNotice(t *testing.T) {
	b := newBareWorld(t)
	p := b.player(PlayerStart, "Slayer", cdMap, 90, 60)
	spawn := func(class, attr int) *Object {
		o := b.objects[b.allocMonsterIndex()]
		o.reset()
		o.State = StateConnected
		o.Map, o.X, o.Y = cdMap, 91, 60
		if !b.setMonster(o, class) {
			t.Fatalf("class %d", class)
		}
		o.Mon.Attribute = attr
		return o
	}
	boss := spawn(295, cdAttribute)
	boss.Mon.Hits[0] = damageEntry{Index: p.Index, Damage: 10, Time: b.now()}
	for _, h := range deathHooks {
		h(b.World, p, boss)
	}
	want := b.messageFor(p, cdMsgKill, monsterName(boss), "Slayer")
	if txt := cdNotice(b.drain(p)); txt != want || !strings.Contains(txt, "Slayer") {
		t.Fatalf("notice %q want %q", txt, want)
	}
	// Class 300..303 too; the killer is used when there is no top hitter.
	b.cdMonsterDie(p, spawn(302, cdAttribute))
	if txt := cdNotice(b.drain(p)); !strings.Contains(txt, "Slayer") {
		t.Fatalf("302 notice %q", txt)
	}
	// Other classes, or the same class without attribute 7: nothing.
	b.cdMonsterDie(p, spawn(39, cdAttribute))
	b.cdMonsterDie(p, spawn(295, 0))
	b.cdMonsterDie(p, spawn(304, cdAttribute))
	if pk := b.drain(p); find(pk, 0x0D, -1) != nil {
		t.Fatal("unexpected notice")
	}
}

func TestCastleDeepGroupNotice(t *testing.T) {
	b := newBareWorld(t)
	o := b.player(PlayerStart, "Here", 0, 130, 130)
	cfg, err := gsconfig.Load(repoInstance("5.GameServerCS"))
	if err != nil {
		t.Fatal(err)
	}
	b2 := newBareWorld(t)
	b2.cfg = cfg
	b2.hub = b.hub
	b.hub.register(b2.World)
	o2 := b2.player(PlayerStart, "There", 30, 100, 100)
	if b.mapServerGroup() != 0 || b2.mapServerGroup() != 0 {
		t.Fatalf("groups %d %d", b.mapServerGroup(), b2.mapServerGroup())
	}
	b.groupNotice(NoticeGold, "hello")
	for len(b2.events) > 0 {
		(<-b2.events)()
	}
	if cdNotice(b.drain(o)) != "hello" || cdNotice(b2.drain(o2)) != "hello" {
		t.Fatal("notice not relayed to the group")
	}
	// Another group, or a server missing from MapServerInfo: not shown.
	ms := *b2.data.MapServer
	ms.Servers = append([]gamedata.MapServerEntry(nil), ms.Servers...)
	for i := range ms.Servers {
		if ms.Servers[i].Code == 19 {
			ms.Servers[i].Group = 1
		}
	}
	b2.data.MapServer = &ms
	b.groupNotice(NoticeGold, "again")
	for len(b2.events) > 0 {
		(<-b2.events)()
	}
	if cdNotice(b.drain(o)) != "again" || len(b2.drain(o2)) != 0 {
		t.Fatal("notice crossed groups")
	}
	b.cfg.ServerCode = 77
	b.groupNotice(NoticeGold, "lost")
	if len(b.drain(o)) != 0 {
		t.Fatal("unlisted server showed the notice")
	}
}
