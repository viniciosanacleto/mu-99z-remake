package game

import (
	"bytes"
	"testing"
)

// chatLine builds a 00 chat packet.
func chatLine(o *Object, msg string) []byte {
	p := make([]byte, chatPktSize)
	p[0], p[1], p[2] = 0xC1, chatPktSize, 0x00
	copy(p[chatNameOff:chatNameOff+10], o.P.Char.Name)
	copy(p[chatMsgOff:], msg)
	return p
}

func TestCustomMove(t *testing.T) {
	b := newBareWorld(t)
	o := b.player(PlayerStart, "Mover", 0, 140, 140)
	b.handleChat(o, chatLine(o, "/BAR"))
	if o.Map != 0 || o.X != 125 || o.Y != 125 {
		t.Fatalf("not moved %d %d,%d", o.Map, o.X, o.Y)
	}
	// The move leaves the player teleporting until the client answers
	// (F3 12); then /lorencia2 needs level 10.
	b.customMove(o, "/lorencia2")
	hasText(t, b.drain(o), b.messageFor(o, 0xE2))
	o.Live = LiveNormal
	o.P.Char.Level = 5
	b.drain(o)
	b.customMove(o, "/lorencia2")
	hasText(t, b.drain(o), b.messageFor(o, 0xE0, 10))
	if o.X != 125 {
		t.Fatal("moved without the level")
	}
	o.P.Char.Level = 400
	o.PKLevel = PKOutlaw2
	b.customMove(o, "/lorencia2")
	if o.X != 76 || o.Y != 155 {
		t.Fatalf("PKMove 1 refused %d,%d", o.X, o.Y)
	}
	// Event players leave their event when they move.
	o.Live = LiveNormal
	gmOf(o).InEvent = true
	sb := &b.cev().sobre
	sb.state, sb.list = sobreOpen, []int{o.Index}
	b.customMove(o, "/bar")
	if sb.has(o) || gmOf(o).InEvent {
		t.Fatal("still in the event")
	}
}

func TestCustomGate(t *testing.T) {
	b := newBareWorld(t)
	o := b.player(PlayerStart, "Walker", 7, 233, 209)
	b.customGateSecond()
	if o.Map != 25 || o.X != 10 || o.Y != 116 {
		t.Fatalf("not moved %d %d,%d", o.Map, o.X, o.Y)
	}
	b.customGateSecond()
	if o.Map != 25 {
		t.Fatal("moved again")
	}
}

func TestCustomRespawn(t *testing.T) {
	b := newBareWorld(t)
	o := b.player(PlayerStart, "Gladiator", 6, 60, 110)
	o.Alive = false
	b.respawnPlayer(o)
	if o.Map != 6 || o.X != 60 || o.Y != 110 {
		t.Fatalf("respawn %d %d,%d", o.Map, o.X, o.Y)
	}
	p := b.player(PlayerStart+1, "Townie", 0, 140, 140)
	p.Alive = false
	b.respawnPlayer(p)
	if p.X == 140 && p.Y == 140 {
		t.Fatal("normal respawn overridden")
	}
}

func TestCustomNPCCommand(t *testing.T) {
	b := newBareWorld(t)
	o := b.player(PlayerStart, "Talker", 0, 130, 127)
	o.P.Char.Level = 10
	npc := fakeNPC(b, 100, 247, 0, 130, 126)
	if !b.npcTalk(o, npc) {
		t.Fatal("talk not handled")
	}
	hasText(t, b.drain(o), b.messageFor(o, 0x151, 400))
	other := fakeNPC(b, 101, 247, 0, 131, 126)
	if b.customNPCCommand(o, other) {
		t.Fatal("another cell ran the command")
	}
}

func TestCustomDeathMessage(t *testing.T) {
	b := newBareWorld(t)
	o := b.player(PlayerStart, "Prey", 0, 130, 130)
	m := fakeNPC(b, 102, 1, 0, 131, 130)
	m.Type = TypeMonster
	b.customDeathMessage(m, o)
	if p := find(b.drain(o), 0x01, -1); !bytes.Equal(p, packetObjectChat(m.Index, "Hihihih I'm Very Strong Monster")) {
		t.Fatalf("chat % X", p)
	}
	b.customDeathMessage(o, m)
	if len(b.drain(o)) != 0 {
		t.Fatal("player kill said something")
	}
}
