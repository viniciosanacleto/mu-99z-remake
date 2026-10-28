package guild

import (
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/packet"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/frame"
)

// PacketWarRequestResult is C1 04 60 result (FUN_00485820).
func PacketWarRequestResult(result byte) []byte {
	p := packet.New(frame.C1, 0x60, -1, 4)
	p[3] = result
	return p
}

// PacketWarRequest is C1 0C 61 guild[8] type (FUN_004858c0), sent to
// the master of the challenged guild.
func PacketWarRequest(guildName string, typ byte) []byte {
	p := packet.New(frame.C1, 0x61, -1, 0x0C)
	packet.PutGuildName(p[3:], guildName)
	p[11] = typ
	return p
}

// PacketWarStart is C1 0D 62 enemy[8] type team (FUN_00485980).
func PacketWarStart(enemy string, typ, team byte) []byte {
	p := packet.New(frame.C1, 0x62, -1, 0x0D)
	packet.PutGuildName(p[3:], enemy)
	p[11], p[12] = typ, team
	return p
}

// PacketWarEnd is C1 0C 63 result enemy[8] (FUN_00485a50): 1 won, 0
// lost, 3/2 the war was abandoned (own / enemy side).
func PacketWarEnd(result byte, enemy string) []byte {
	p := packet.New(frame.C1, 0x63, -1, 0x0C)
	p[3] = result
	packet.PutGuildName(p[4:], enemy)
	return p
}

// PacketWarScore is C1 06 64 own enemy 0 (FUN_00485ab0).
func PacketWarScore(own, enemy byte) []byte {
	p := packet.New(frame.C1, 0x64, -1, 6)
	p[3], p[4] = own, enemy
	return p
}

// SameName reports whether two guild names match the way the original
// compares them: strncmp over the 8-byte name field.
func SameName(a, b string) bool { return cut8(a) == cut8(b) }

func cut8(s string) string {
	if len(s) > 8 {
		return s[:8]
	}
	return s
}
