package castlesiege

import "time"

// Battle limits.
const (
	CrownTime   = 30000 // FUN_0041edc0: 29999 < accumulated + elapsed
	CrownMax    = 31000 // FUN_00422bf0 clamp
	MaxMercs    = 100   // FUN_004b2bd0: count < 0x65
	StoneGrowth = 60    // FUN_004a38d0: 12 s per level, 5 levels
	WeaponDelay = 10 * time.Second
	WeaponHit   = 100 * time.Millisecond
	MinimapMax  = 1000
)

// Crown access states of B2 1A (FUN_00422bf0).
const (
	CrownBegin   = 0 // registration started
	CrownSuccess = 1
	CrownFail    = 2 // interrupted (time accumulated)
	CrownOther   = 3 // another player holds the crown
	CrownNoSwich = 4 // the switches are not held by the player's side
)
