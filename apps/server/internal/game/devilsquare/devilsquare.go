// Package devilsquare holds the Devil Square event (CDevilSquare,
// 0x46dcf0..0x4705d0; Event/DevilSquare.dat) data that does not need the
// world actor: the square states and limits, the per-square record and its
// client packets. The game package runs the 1-second state machine.
package devilsquare

import "time"

// Square states: EMPTY -> READY (entry open) -> STAND -> START -> CLEAN.
const (
	StateBlank = iota
	StateEmpty
	StateReady
	StateStand
	StateStart
	StateClean
)

// Squares (maps 9 for 0..3, 32 for 4..5), limits and entry items.
const (
	Squares        = 6
	MaxUsers       = 50
	MaxMobs        = 200
	EntrySection   = 1 // EventEntryLevel.dat section
	StageSection   = 2 // EventStageSpawn.dat section
	KickGate       = 0x1B
	Charon         = 0xED
	ItemInvitation = 0x1C13 // Devil's Invitation (14,19), level = square + 1
	ItemFreeTicket = 0x1A2E // free entrance (13,46)
)

// Gates are the entry gates of FUN_004701f0 per square.
var Gates = [Squares]int{0x3A, 0x3B, 0x3C, 0x3D, 0x6F, 0x70}

// User is one participant slot of a square (Index -1 = empty).
type User struct {
	Index, Score, Rank, Exp, Money int
}

// Square is the record of one square (the r[] block of CDevilSquare).
type Square struct {
	Index, State, Map int
	Remain            int       // r[3], seconds
	Target            time.Time // r[4]
	Tick              time.Time // r[5]
	Open              bool      // r[6]
	LastMin           int       // r[7]
	Warned            bool      // r[8]
	Stage             int       // r[9]
	Users             [MaxUsers]User
	Mobs              []int // r[0x104], at most 200
}

// PacketResult is C1 04 90 result (FUN_004701f0).
func PacketResult(r byte) []byte { return []byte{0xC1, 0x04, 0x90, r} }

// PacketState is C1 04 92 state: 0 entry closing, 1 starting, 2 ending.
func PacketState(s byte) []byte { return []byte{0xC1, 0x04, 0x92, s} }

// User returns the participant slot of the object index, or nil.
func (s *Square) User(index int) *User {
	for i := range s.Users {
		if s.Users[i].Index == index {
			return &s.Users[i]
		}
	}
	return nil
}

// SetRemain starts a countdown of sec seconds from now.
func (s *Square) SetRemain(now time.Time, sec int) {
	s.Remain = sec
	s.Target = now.Add(time.Duration(sec) * time.Second)
}
