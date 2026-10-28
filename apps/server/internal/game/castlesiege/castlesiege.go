// Package castlesiege holds the Castle Siege rules that do not need the
// world actor: the cycle states, the castle object classes, the battle
// limits and the client packets of the siege. The game package runs the
// siege itself and calls into this package.
package castlesiege

// States of CCastleSiege+0x1c (SetState FUN_0041c690, process
// FUN_0041b780).
const (
	StateIdle1    = 0 // before the cycle start date
	StateRegSiege = 1 // "SIGN1": guild masters register (B2 01)
	StateIdle2    = 2
	StateRegMark  = 3 // "SIGN2": Sign of Lord marks (B2 04)
	StateIdle3    = 4
	StateNotify   = 5 // siege guilds chosen and announced
	StateReady    = 6
	StateStart    = 7 // the battle
	StateEnd      = 8
	StateEndCycle = 9
)

// Classes of the castle objects (MonsterList.txt).
const (
	Map          = 30
	TrialMap     = 31
	ClassShield  = 0xD7  // 215, removed when the crown room opens (FUN_0041f970)
	ClassCrown   = 0xD8  // 216
	ClassSwitch2 = 0xD9  // 217 (switch 1 of CCastleSiege+0x88)
	ClassSwitch1 = 0xDA  // 218 (switch 2 of CCastleSiege+0x8c)
	ClassLever   = 0xDB  // 219, the gate lever
	ClassKeeper  = 0xDC  // 220, Land of Trial gatekeeper
	ClassSlingA  = 0xDD  // 221, attack slingshot
	ClassSlingD  = 0xDE  // 222, defense slingshot
	ClassSenior  = 0xDF  // 223
	ClassGuard   = 0xE0  // 224, Guardsman
	ClassGate    = 0x115 // 277
	ClassStone   = 0x116 // 278, life stone
	ClassStatue  = 0x11B // 283, guardian statue
	ClassArcher  = 0x11E // 286, mercenary
	ClassSpear   = 0x11F // 287, mercenary
	ClassCanon   = 0x120 // 288, canon tower
)
