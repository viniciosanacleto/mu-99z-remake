package game

import (
	"bytes"
	"testing"
	"time"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/devilsquare"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/item"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/frame"
)

func TestNextSchedule(t *testing.T) {
	daily := func(h, m int) gamedata.Schedule {
		return gamedata.Schedule{Year: -1, Month: -1, Day: -1, DayOfWeek: -1, Hour: h, Minute: m}
	}
	rows := []gamedata.Schedule{daily(0, 30), daily(4, 30), daily(20, 30)}
	at := func(h, m, s int) time.Time { return time.Date(2026, 10, 22, h, m, s, 0, time.UTC) }
	for _, c := range []struct{ now, want time.Time }{
		{at(0, 0, 0), at(0, 30, 0)},
		{at(4, 30, 0), at(4, 30, 0)},
		{at(4, 30, 1), at(20, 30, 0)},
		{at(21, 0, 0), time.Date(2026, 10, 23, 0, 30, 0, 0, time.UTC)},
	} {
		if got, ok := nextSchedule(rows, c.now); !ok || !got.Equal(c.want) {
			t.Fatalf("next(%v) = %v want %v", c.now, got, c.want)
		}
	}
	// 2026-10-22 is a Thursday (4); Sundays only at 12:00.
	sun := []gamedata.Schedule{{Year: -1, Month: -1, Day: -1, DayOfWeek: 0, Hour: 12}}
	if got, _ := nextSchedule(sun, at(13, 0, 0)); !got.Equal(time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)) {
		t.Fatalf("sunday = %v", got)
	}
	if _, ok := nextSchedule(nil, at(0, 0, 0)); ok {
		t.Fatal("empty schedule matched")
	}
}

func TestDSRankingAndPacket(t *testing.T) {
	b := newBareWorld(t)
	a := b.player(PlayerStart, "Alpha", 9, 130, 90)
	c := b.player(PlayerStart+1, "Beta", 9, 131, 90)
	s := &b.ds().sq[0]
	s.Users[0] = devilsquare.User{Index: a.Index, Score: 10}
	s.Users[3] = devilsquare.User{Index: c.Index, Score: 30}
	b.dsRank(s)
	if s.Users[0].Rank != 1 || s.Users[3].Rank != 0 {
		t.Fatalf("ranks %+v %+v", s.Users[0], s.Users[3])
	}
	s.Users[0].Exp, s.Users[0].Money = 4000, 25000
	p := packetDSRanking(b.World, s, s.Users[0])
	// self entry + ranks 1 and 2.
	if len(p) != 5+3*24 || p[1] != byte(len(p)) || p[2] != 0x93 || p[3] != 2 || p[4] != 3 ||
		string(p[5:10]) != "Alpha" || p[17] != 10 || p[21] != 0xA0 || p[22] != 0x0F ||
		string(p[29:33]) != "Beta" || string(p[53:58]) != "Alpha" {
		t.Fatalf("ranking % X", p)
	}
}

func TestDevilSquareCycle(t *testing.T) {
	b := newBareWorld(t)
	b.clock = time.Date(2026, 10, 22, 0, 24, 0, 0, time.UTC)
	for i := range b.ds().sq {
		b.ds().sq[i].State = devilsquare.StateEmpty
		b.dsEnterEmpty(&b.ds().sq[i])
	}
	o := questPlayer(b, PlayerStart, 1, 50) // DK 50 -> square 0
	o.Map, o.X, o.Y = 0, 140, 130
	o.P.Char.Money = 0
	tick := func(d time.Duration) [][]byte {
		b.advance(d)
		b.dsTick()
		return b.drain(o)
	}
	s := &b.ds().sq[0]
	if s.State != devilsquare.StateEmpty || s.Remain != 360 {
		t.Fatalf("empty: state %d remain %d", s.State, s.Remain)
	}
	// 5 minutes before the start: "%d minute(s)" (725) with 5.
	pk := tick(time.Minute)
	if n := find(pk, 0x0D, -1); n == nil || !bytes.Contains(n, []byte("5")) {
		t.Fatalf("no 725 notice: % X", pk)
	}
	pk = tick(5 * time.Minute)
	if s.State != devilsquare.StateReady || !s.Open || s.Remain != 300 || find(pk, 0x0D, -1) == nil {
		t.Fatalf("ready: state %d open %v remain %d", s.State, s.Open, s.Remain)
	}

	// Charon without an invitation: C1 07 F3 40 01 02 00.
	b.dsCharonTalk(o)
	if p := find(b.drain(o), 0xF3, 0x40); !bytes.Equal(p, []byte{0xC1, 0x07, 0xF3, 0x40, 0x01, 0x02, 0x00}) {
		t.Fatalf("charon = % X", p)
	}
	slot := b.insertInventory(o, item.Item{Index: devilsquare.ItemInvitation, Level: 2, Durability: 1})
	b.handleDSEnter(o, frame.Build(frame.C1, 0x90, 0, byte(slot+12)))
	if p := find(b.drain(o), 0x90, -1); !bytes.Equal(p, devilsquare.PacketResult(1)) {
		t.Fatalf("wrong invitation level = % X", p)
	}
	o.P.Inventory[slot].Level = 1
	b.handleDSEnter(o, frame.Build(frame.C1, 0x90, 1, byte(slot+12)))
	if p := find(b.drain(o), 0x90, -1); !bytes.Equal(p, devilsquare.PacketResult(1)) {
		t.Fatalf("level 1 invitation for square 1 = % X", p)
	}
	b.handleDSEnter(o, frame.Build(frame.C1, 0x90, 0, byte(slot+12)))
	pk = b.drain(o)
	if p := find(pk, 0x90, -1); !bytes.Equal(p, devilsquare.PacketResult(0)) {
		t.Fatalf("enter = % X", p)
	}
	if !o.P.Inventory[slot].IsEmpty() || s.User(o.Index) == nil || o.Map != 9 {
		t.Fatalf("not inside: map %d", o.Map)
	}
	// 0x91 during READY reports the user count.
	b.handleEventTime(o, frame.Build(frame.C1, 0x91, 1))
	if p := find(b.drain(o), 0x91, -1); !bytes.Equal(p, []byte{0xC1, 0x07, 0x91, 1, 0, 1, 0}) {
		t.Fatalf("91 = % X", p)
	}

	pk = tick(4*time.Minute + 30*time.Second) // 30 s left
	if p := find(pk, 0x92, -1); !bytes.Equal(p, devilsquare.PacketState(0)) {
		t.Fatalf("92 00 missing: % X", pk)
	}
	tick(30 * time.Second)
	if s.State != devilsquare.StateStand || s.Open {
		t.Fatalf("stand: state %d", s.State)
	}
	tick(time.Minute)
	if s.State != devilsquare.StateStart || len(s.Mobs) == 0 {
		t.Fatalf("start: state %d mobs %d", s.State, len(s.Mobs))
	}
	stage0 := len(s.Mobs)
	m := b.objects[s.Mobs[0]]
	if m.Map != 9 || (m.Class != 15 && m.Class != 17) {
		t.Fatalf("stage 0 monster class %d map %d", m.Class, m.Map)
	}
	// Kill it as the top hitter (FUN_004b6df0 needs the player within 20
	// cells): score += level * (square+1).
	o.X, o.Y = m.X, m.Y
	m.Mon.Hits[0] = damageEntry{Index: o.Index, Damage: 1, Time: b.now()}
	b.dsMonsterKilled(m)
	if u := s.User(o.Index); u.Score != m.Mon.Level {
		t.Fatalf("score %d level %d", u.Score, m.Mon.Level)
	}
	// 20 % of 20 minutes later: stage 1.
	tick(4 * time.Minute)
	if s.Stage != 1 || len(s.Mobs) <= stage0 {
		t.Fatalf("stage %d mobs %d", s.Stage, len(s.Mobs))
	}
	pk = tick(16 * time.Minute)
	if s.State != devilsquare.StateClean || len(s.Mobs) != 0 {
		t.Fatalf("clean: state %d mobs %d", s.State, len(s.Mobs))
	}
	// Rank 0 of square 0: 30000 zen and the ranking window.
	if o.P.Char.Money != 30000 || find(pk, 0x93, -1) == nil {
		t.Fatalf("money %d ranking % X", o.P.Char.Money, find(pk, 0x93, -1))
	}
	tick(4 * time.Minute)
	if s.State != devilsquare.StateEmpty || s.User(o.Index) != nil {
		t.Fatalf("back to empty: state %d", s.State)
	}
}
