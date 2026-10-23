package game

import "time"

// roundQWorld is the per-instance data of the round-4 leftovers: the Dark
// Raven damage table (pet_raven.go), the push-back message queue of the
// players and the CustomMobCombo list (pushback.go), the MoveSummon table
// and the Summon Party countdowns (skill_guild.go) and the battle soccer
// ground (guild_soccer.go).
type roundQWorld struct {
	petDamage  []petDamageRow // Util/PetDamageTable.txt
	noPush     []int          // Custom/CustomMobCombo.txt (classes never pushed)
	pushes     []pushMsg      // FUN_00510550(2, ...) on players (FUN_004c4120 case 2)
	moveSummon []moveSummonRow
	summons    map[int]*summonState // obj+0x5d2..0x5d6 Summon Party countdowns
	soccer     soccerGround
}

// pushMsg is one queued push-back of a player.
type pushMsg struct {
	target, sender int
	due            time.Time
}

func (w *World) q() *roundQWorld {
	if w.rq == nil {
		w.rq = &roundQWorld{soccer: soccerGround{ball: -1}}
		w.rq.soccer.reset()
	}
	return w.rq
}
