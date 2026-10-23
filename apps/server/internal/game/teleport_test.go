package game

import (
	"bytes"
	"testing"
	"time"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/item"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/frame"
)

func TestTeleportPacketLayouts(t *testing.T) {
	if got := packetTeleport(5, 1, 108, 247, 1); !bytes.Equal(got, []byte{0xC3, 0x08, 0x1C, 0x01, 1, 108, 247, 1}) {
		t.Fatalf("1C gate = % X", got)
	}
	if got := packetTeleport(0, 0, 10, 20, 3); !bytes.Equal(got, []byte{0xC3, 0x08, 0x1C, 0x00, 0, 10, 20, 3}) {
		t.Fatalf("1C magic = % X", got)
	}
	if got := packetCastleStateNotice(0); !bytes.Equal(got, []byte{0xC1, 0x05, 0xB2, 0x17, 0x00}) {
		t.Fatalf("B2 17 = % X", got)
	}
	if got := packetMagicUse(6, 0x53FC, 0x53FC, true); !bytes.Equal(got, []byte{0xC3, 0x08, 0x19, 0x06, 0x53, 0xFC, 0xD3, 0xFC}) {
		t.Fatalf("19 = % X", got)
	}
}

func TestGatePositionExcludesUpperBound(t *testing.T) {
	b := newBareWorld(t)
	// Gate 1 targets gate 2: map 1, X 107..110, Y 247 (X2 never chosen).
	seen := map[int]bool{}
	for i := 0; i < 500; i++ {
		g, x, y, ok := b.gatePosition(1)
		if !ok || g.Index != 2 || g.Map != 1 || y != 247 || x < 107 || x > 109 {
			t.Fatalf("gatePosition(1) = %d %d,%d %v", g.Index, x, y, ok)
		}
		seen[x] = true
	}
	if seen[110] {
		t.Fatal("X2 chosen")
	}
}

func TestGateAllowed(t *testing.T) {
	b := newBareWorld(t)
	a := b.player(PlayerStart, "Alpha", 0, 122, 232)
	a.P.Char.Level = 20
	if !b.gateAllowed(a, 1) {
		t.Fatal("level 20 near gate 1 refused")
	}
	a.P.Char.Level = 19
	if b.gateAllowed(a, 1) {
		t.Fatal("level 19 allowed")
	}
	a.Class = 3 // MG needs 2/3: 13
	a.P.Char.Level = 13
	if !b.gateAllowed(a, 1) {
		t.Fatal("MG level 13 refused")
	}
	a.Class = 0
	a.P.Char.Level = 50
	a.X = 123 + 6 // the check is around (X2, Y2) +-5
	if b.gateAllowed(a, 1) {
		t.Fatal("6 cells away allowed")
	}
}

func TestGateMoveSequence(t *testing.T) {
	b := newBareWorld(t)
	a := b.player(PlayerStart, "Alpha", 0, 122, 232)
	a.P.Char.Level = 20
	c := b.player(PlayerStart+1, "Beta", 0, 124, 233)
	b.see(a, c)

	b.handleTeleport(a, frame.Build(frame.C3, 0x1C, 1))
	pk := b.drain(a)
	tp := find(pk, 0x1C, -1)
	if tp == nil || tp[3] != 1 || tp[4] != 1 || tp[5] < 107 || tp[5] > 109 || tp[6] != 247 || tp[7] != 1 {
		t.Fatalf("1C = % X", tp)
	}
	if p := find(pk, 0x0B, -1); !bytes.Equal(p, []byte{0xC1, 0x05, 0x0B, 0x00, 0x00}) {
		t.Fatalf("0B = % X", p)
	}
	if p := find(pk, 0x12, -1); p == nil || p[4] != 1 {
		t.Fatalf("self C2 12 = % X", p)
	}
	if p := find(pk, 0xB2, 0x17); p == nil {
		t.Fatal("no B2 17")
	}
	m := a.Move
	if a.Map != 1 || a.Live != LiveTeleport || m.Regen != 1 || a.Y != 247 || a.Dir != 1 {
		t.Fatalf("after gate map=%d live=%d regen=%d", a.Map, a.Live, m.Regen)
	}
	for _, e := range a.VP2 {
		if e.State != VPEmpty {
			t.Fatal("VP2 not cleared")
		}
	}
	// Walking is refused until the regen sequence ends.
	b.advance(time.Second)
	b.handleWalk(a, walkPacket(a.X, a.Y, 3, 3))
	if find(b.drain(a), 0xD3, -1) != nil {
		t.Fatal("walk while obj+0x287 > 0")
	}
	// The observer loses Alpha on its next viewport cycle.
	b.tick1000()
	if p := find(b.drain(c), 0x14, -1); p == nil || !destroys(p, PlayerStart) {
		t.Fatalf("Beta destroy = % X", p)
	}
	// F3 12: the client is ready.
	b.handleRegenReady(a, frame.Build(frame.C1, 0xF3, 0x12))
	if m.Regen != 2 {
		t.Fatalf("regen %d after F3 12", m.Regen)
	}
	b.tick1000()
	if m.Regen != 0 || a.Live != LiveNormal || a.Map != 1 {
		t.Fatalf("after cycle regen=%d live=%d", m.Regen, a.Live)
	}
}

func TestGateRefused(t *testing.T) {
	b := newBareWorld(t)
	a := b.player(PlayerStart, "Alpha", 0, 122, 232)
	a.P.Char.Level = 5
	b.handleTeleport(a, frame.Build(frame.C3, 0x1C, 1))
	pk := b.drain(a)
	if p := find(pk, 0x1C, -1); !bytes.Equal(p, []byte{0xC3, 0x08, 0x1C, 0x01, 0, 122, 232, 0}) {
		t.Fatalf("refused 1C = % X", p)
	}
	if find(pk, 0xB2, 0x17) == nil || find(pk, 0x12, -1) != nil {
		t.Fatal("refusal must send B2 17 and no self C2 12")
	}
	if a.Map != 0 || a.Move.Regen != 0 {
		t.Fatal("refused gate moved the player")
	}
}

func TestGateIcarusNeedsWings(t *testing.T) {
	b := newBareWorld(t)
	a := b.player(PlayerStart, "Alpha", 0, 122, 232)
	var gate int
	for i, g := range b.data.Gates {
		if g.Map == 10 && g.Flag != 1 {
			gate = i
			break
		}
	}
	if gate == 0 {
		t.Skip("no Icarus gate")
	}
	b.gateMove(a, gate)
	if a.Map == 10 {
		t.Fatal("moved to Icarus without wings")
	}
	b.drain(a)
	a.Move.Regen = 0
	a.P.Inventory[7] = item.Item{Index: item.Make(12, 0)}
	b.gateMove(a, gate)
	if a.Map != 10 {
		t.Fatal("wings refused")
	}
}

func TestMagicTeleport(t *testing.T) {
	b := newBareWorld(t)
	x, y := b.openRow(0, 6)
	a := b.player(PlayerStart, "Alpha", 0, x, y)
	c := b.player(PlayerStart+1, "Beta", 0, x, y+5)
	b.see(a, c)
	// Without the skill: refused, nothing changes.
	b.handleTeleport(a, frame.Build(frame.C3, 0x1C, 0, byte(x+5), byte(y)))
	if find(b.drain(a), 0x19, -1) != nil || a.X != x {
		t.Fatal("teleport without the skill")
	}
	a.P.Skills[0] = Skill{Index: teleportSkill}
	b.advance(10 * time.Second)
	mana := a.P.Char.Mana
	b.handleTeleport(a, frame.Build(frame.C3, 0x1C, 0, byte(x+5), byte(y)))
	if p := find(b.drain(a), 0x19, -1); p == nil || p[3] != 6 {
		t.Fatalf("19 = % X", p)
	}
	if p := find(b.drain(c), 0x14, -1); !bytes.Equal(p, []byte{0xC1, 0x06, 0x14, 0x01, 0x53, 0xFC}) {
		t.Fatalf("observer destroy = % X", p)
	}
	m := a.Move
	if a.X != x+5 || m.TeleportState != 1 || !a.Teleporting || a.P.Char.Mana >= mana {
		t.Fatalf("X=%d state=%d mana %v->%v", a.X, m.TeleportState, mana, a.P.Char.Mana)
	}
	b.advance(501 * time.Millisecond)
	b.teleportSecond()
	if m.TeleportState != 2 {
		t.Fatalf("state %d after 501 ms", m.TeleportState)
	}
	b.teleportSecond()
	if p := find(b.drain(a), 0x1C, -1); !bytes.Equal(p, []byte{0xC3, 0x08, 0x1C, 0x00, 0, byte(x + 5), byte(y), byte(a.Dir)}) {
		t.Fatalf("end 1C = % X", p)
	}
	if m.TeleportState != 0 || a.Teleporting {
		t.Fatal("teleport not finished")
	}
}

func TestTeleportHook(t *testing.T) {
	b := newBareWorld(t)
	a := b.player(PlayerStart, "Alpha", 0, 130, 130)
	teleportHook(b.World, a, 2, 200, 50, 4)
	if a.Map != 2 || a.X != 200 || a.Y != 50 || a.Dir != 4 || a.Move.Regen != 1 {
		t.Fatalf("hook: map %d %d,%d", a.Map, a.X, a.Y)
	}
	if p := find(b.drain(a), 0x1C, -1); !bytes.Equal(p, []byte{0xC3, 0x08, 0x1C, 0x01, 2, 200, 50, 4}) {
		t.Fatalf("1C = % X", p)
	}
}

func TestEndToEndGateTeleport(t *testing.T) {
	env := newTestEnv(t)
	a := enterAt(t, env, "tester", "Alpha", 0x00, 0, 122, 232, 20)
	c := enterAt(t, env, "second", "Beta", 0x00, 0, 124, 231, 20)
	waitSees(t, c, PlayerStart)

	a.send(frame.Build(frame.C3, 0x1C, 1))
	p := a.next(0x1C, -1)
	if !a.lastEncrypted || p[3] != 1 || p[4] != 1 || p[6] != 247 {
		t.Fatalf("1C = % X", p)
	}
	a.send(frame.Build(frame.C1, 0xF3, 0x12))
	// Beta's next viewport cycle destroys Alpha (now on map 1).
	for !destroys(c.next(0x14, -1), PlayerStart) {
	}
}

// destroys reports whether a C1 14 packet lists index.
func destroys(p []byte, index int) bool {
	for i := 0; i < int(p[3]); i++ {
		if int(p[4+2*i]&0x7F)<<8|int(p[5+2*i]) == index {
			return true
		}
	}
	return false
}
