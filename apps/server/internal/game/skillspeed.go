package game

import (
	"time"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"
)

// CHackSkillSpeedCheck (Hack/HackSkillSpeedCheck.txt, DAT_0173e374,
// reader FUN_00488340, getter FUN_004e8290). FUN_00488550 is called for
// players before:
//   - C3 08 19 (FUN_00501570) and C3 09 1E (FUN_00501740): a failure drops
//     the packet;
//   - each Nova charge step (FUN_0050e640, skill 0x28): a failure skips
//     the step;
//   - skill 215 (FUN_004fdbd0, Chain Lightning, skill_chain.go): a
//     failure cancels the cast (no mana taken).

// skillSpeedState is the per-object part: obj+0xd44 + skill*4 (time of
// the last count reset) and obj+0x16ec + skill*4 (use count).
type skillSpeedState struct {
	Tick       map[int]time.Time
	Count      map[int]int
	BlockUntil time.Time // obj+0x2094 (set by Action 3; its reader is not identified)
}

// skillSpeedHook is FUN_00488550 for a player and a skill: false stops
// the caller.
var skillSpeedHook func(w *World, o *Object, skill int) bool

func init() {
	skillSpeedHook = (*World).skillSpeedOK
}

// skillSpeedAllowed runs the hook for players (the callers test obj+0x6c
// == 1 first).
func (w *World) skillSpeedAllowed(o *Object, skill int) bool {
	return o.Type != TypePlayer || skillSpeedHook == nil || skillSpeedHook(w, o, skill)
}

// skillSpeedOK is FUN_00488550. The speed is the magic speed (obj+0x364)
// for the Dark Wizard and the attack speed (obj+0x360) otherwise. Every
// rule of the skill whose Class (-1 any), level, reset and speed ranges
// match is applied in file order:
//   - MaxDelay != -1 and at least MaxDelay ms since the last reset: the
//     time is taken and the count goes back to 0;
//   - MinCount != -1: count + 1; above MinCount and above MaxCount
//     (MaxCount != -1) the use is refused ("[HackSkillSpeedCheck]" log)
//     and Action runs: 1 sends message 0x2CF, 2 closes the game
//     (FUN_005102a0), 3 sends message 0x2CF as a type 2 color 6 notice and
//     sets obj+0x2094 = now + 10 s; other values only refuse.
func (w *World) skillSpeedOK(o *Object, skill int) bool {
	rules := w.dus().speedRules
	if len(rules) == 0 || o.P == nil || o.P.Char == nil {
		return true
	}
	st := w.statsOf(o)
	speed := st.AttackSpeed
	if o.Class == 0 {
		speed = st.MagicSpeed
	}
	ss := w.dus().speed[o.Index]
	if ss == nil {
		ss = &skillSpeedState{Tick: map[int]time.Time{}, Count: map[int]int{}}
		w.dus().speed[o.Index] = ss
	}
	ch := o.P.Char
	now := w.now()
	for i := range rules {
		r := &rules[i]
		if r.Skill != skill || (r.Class != -1 && r.Class != o.Class) || !inReqRange(r.MinLevel, r.MaxLevel, ch.Level) ||
			!inReqRange(r.MinReset, r.MaxReset, ch.Resets) || !inReqRange(r.MinSpeed, r.MaxSpeed, speed) {
			continue
		}
		if r.MaxDelay != -1 && now.Sub(ss.Tick[skill]) >= time.Duration(uint32(r.MaxDelay))*time.Millisecond {
			ss.Tick[skill] = now
			ss.Count[skill] = 0
		}
		if r.MinCount == -1 {
			continue
		}
		ss.Count[skill]++
		n := uint32(ss.Count[skill])
		if n <= uint32(r.MinCount) || r.MaxCount == -1 || n <= uint32(r.MaxCount) {
			continue
		}
		w.log.Info("[HackSkillSpeedCheck] Skill Speed count error", "account", o.P.Account.Login,
			"name", ch.Name, "skill", skill, "speed", speed, "count", n, "max", r.MaxCount)
		w.skillSpeedAction(o, r)
		return false
	}
	return true
}

func (w *World) skillSpeedAction(o *Object, r *gamedata.SkillSpeedRule) {
	switch r.Action {
	case 1:
		w.systemMessage(o, w.messageFor(o, 0x2CF))
	case 2:
		gmDisconnect(o)
	case 3:
		w.customNotice(o, 2, 6, "%s", w.messageFor(o, 0x2CF))
		if s := w.dus().speed[o.Index]; s != nil {
			s.BlockUntil = w.now().Add(10 * time.Second)
		}
	}
}
