// Package chaoscastle holds the Chaos Castle event (CChaosCastle,
// 0x4301a0..0x434080; Event/ChaosCastle.dat) data that does not need the
// world actor: castle limits, gates, fees, the hole rectangles, the
// participant record and the client packets. The game package runs the
// castles.
package chaoscastle

import (
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/packet"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"
)

// Castles (maps 18..23), limits and the entry item.
const (
	Castles      = 6
	MaxUsers     = 70  // 0x46 user slots of 0x14 bytes at record+0x30
	MaxMobs      = 100 // monster list at record+0x5a8
	EntrySection = 2   // EventEntryLevel.dat section
	StageSection = 1   // EventStageSpawn.dat section
	KickGate     = 0x16
	ItemArmor    = 0x1A1D // Armor of Guardsman (13,29)
)

// Gates are the entry gates of FUN_004338c0 per castle (case 6 = 0x110
// is unreachable since the castle is at most 5).
var Gates = [Castles]int{0x52, 0x53, 0x54, 0x55, 0x56, 0x57}

// Fee is the entry fee table DAT_00547978.
var Fee = [Castles]int{25000, 80000, 150000, 250000, 400000, 650000}

// Rectangles (x1, y1, x2, y2) read from 0x547928..0x547977.
var (
	// SafeRect is DAT_00547928, sent with the safe-zone overlay.
	SafeRect = [4]byte{23, 75, 44, 108}
	// HoleRects are DAT_00547930 + set*16: the holes of steps 1..3 (set
	// 0 is a copy of set 1 and is never used).
	HoleRects = [4][4][4]byte{
		{{23, 75, 44, 76}, {43, 77, 44, 108}, {23, 107, 42, 108}, {23, 77, 24, 106}},
		{{23, 75, 44, 76}, {43, 77, 44, 108}, {23, 107, 42, 108}, {23, 77, 24, 106}},
		{{25, 77, 42, 78}, {41, 79, 42, 106}, {25, 105, 40, 106}, {25, 79, 26, 104}},
		{{27, 79, 40, 80}, {39, 81, 40, 104}, {27, 103, 38, 104}, {27, 81, 28, 102}},
	}
	// CenterRects are DAT_00547970/74 (FUN_00432830).
	CenterRects = [2][4]byte{{32, 84, 35, 87}, {32, 96, 35, 99}}
)

// User is one 0x14-byte user slot: index, score, users killed, monsters
// killed, experience.
type User struct {
	Index, Score, UserKills, MonsterKills, Exp int
}

// PacketResult is C1 1D 93 success FE name[10] ... (FUN_00433e80, and
// the delayed copy of FUN_00433fb0 with success 0): bytes 15..16 are not
// written by the original (0 here), [17] monsters killed, [21]
// experience, [25] users killed.
func PacketResult(success byte, name string, u User) []byte {
	p := make([]byte, 0x1D)
	p[0], p[1], p[2], p[3], p[4] = 0xC1, 0x1D, 0x93, success, 0xFE
	copy(p[5:15], name)
	packet.PutUint32(p[17:], uint32(u.MonsterKills))
	packet.PutUint32(p[21:], uint32(u.Exp))
	packet.PutUint32(p[25:], uint32(u.UserKills))
	return p
}

// PacketEnterResult is C1 05 AF 01 result (FUN_004338c0): 0 ok, 1
// refused, 2 closed, 5 full, 7 not enough zen, 8 PK.
func PacketEnterResult(r byte) []byte { return []byte{0xC1, 0x05, 0xAF, 0x01, r} }

// SetAttr sets or clears attr on the terrain cell (x, y); cells outside
// the map are ignored.
func SetAttr(t *gamedata.Terrain, x, y int, attr byte, set bool) {
	if x < 0 || y < 0 || x >= t.Width || y >= t.Height {
		return
	}
	if set {
		t.Set(x, y, t.Get(x, y)|attr)
	} else {
		t.Set(x, y, t.Get(x, y)&^attr)
	}
}
