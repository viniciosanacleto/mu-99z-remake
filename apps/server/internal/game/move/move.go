// Package move holds the movement rules that do not need the world actor:
// the client directions, the path size, the step delay, the classes that
// never walk and the walk, position and action packets. The game package
// moves the objects on the terrain.
package move

import (
	"time"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/packet"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/frame"
)

// PathSlots is the size of the original path arrays (15) plus one spare
// slot: a client path of 15 steps writes index 15, which in the original
// aliases the next array (Y[0] / dir[0..1]); here it lands in the spare
// slot (see the report's open unknowns). The walker never uses it
// (PathCur < 15).
const PathSlots = 16

// PlayerMoveSpeed is the obj+0x374 value players get when loaded.
const PlayerMoveSpeed = 400

// dirTable is DAT_005683d4: {dx, dy} for the 8 client directions.
var dirTable = [8][2]int{{-1, -1}, {0, -1}, {1, -1}, {1, 0}, {1, 1}, {0, 1}, {-1, 1}, {-1, 0}}

// DirStep returns the offsets of a direction nibble. Nibbles 8..15 read
// past the original table (DAT_005683f4..); they are treated as no
// movement.
func DirStep(d byte) (int, int) {
	if d < 8 {
		return dirTable[d][0], dirTable[d][1]
	}
	return 0, 0
}

// PacketWalk is C1 08 D3 idxHi idxLo TX TY dir<<4 (FUN_004d8b80).
func PacketWalk(index, tx, ty, dir int) []byte {
	p := packet.New(frame.C1, 0xD3, -1, 8)
	p[3], p[4] = byte(index>>8), byte(index)
	p[5], p[6] = byte(tx), byte(ty)
	p[7] = byte(dir) << 4
	return p
}

// PacketPosition is C1 07 DF idxHi idxLo X Y (FUN_004d80d0).
func PacketPosition(index, x, y int) []byte {
	p := packet.New(frame.C1, 0xDF, -1, 7)
	p[3], p[4] = byte(index>>8), byte(index)
	p[5], p[6] = byte(x), byte(y)
	return p
}

// PacketAction is C1 09 18 idxHi idxLo dir action targetHi targetLo
// (FUN_004d8300; the two target bytes are copied from the request).
func PacketAction(index int, dir, action, t0, t1 byte) []byte {
	p := packet.New(frame.C1, 0x18, -1, 9)
	p[3], p[4] = byte(index>>8), byte(index)
	p[5], p[6], p[7], p[8] = dir, action, t0, t1
	return p
}

// StepToward moves v one unit toward target.
func StepToward(v, target int) int {
	if v > target {
		return v - 1
	}
	if v < target {
		return v + 1
	}
	return v
}

// NoWalkMonster lists the monster classes FUN_004c4480 never moves
// (castle/event statues and gates).
func NoWalkMonster(class int) bool {
	switch class {
	case 0x83, 0x84, 0x85, 0x86, 0xCC, 0xCD, 0xCE, 0xCF, 0xD0, 0xD1,
		0xD8, 0xD9, 0xDA, 0xDB, 0x115, 0x116, 0x11B, 0x120, 0x20C, 0x20D, 0x20F, 0x210:
		return true
	}
	return false
}

// StepDelay is the per-step delay of FUN_004c4480: (Slow ? 300 : 0) +
// MoveSpeed, times 1.3 truncated (FISTP with RC=chop) for even (diagonal)
// directions.
func StepDelay(dir byte, moveSpeed int, slow bool) time.Duration {
	base := moveSpeed
	if slow {
		base += 300
	}
	if dir%2 == 0 {
		base = int(float64(base) * 1.3)
	}
	return time.Duration(base) * time.Millisecond
}
