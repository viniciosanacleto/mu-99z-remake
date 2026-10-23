package game

import "github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"

// CCommandRequirement (CommandRequirement.txt, reader FUN_004402a0, 0x78
// bytes per row). The CommandManager part of FUN_00435400 calls
// FUN_00440660 after the delay/GM/level/reset/zen checks and the Lua
// OnCommandManager hook, right before the switch on the command index; a
// true result (AL = 1) ends the command without running it or charging
// it.

// commandRequirementHook is FUN_00440660; true blocks the command.
var commandRequirementHook func(w *World, o *Object, index int) bool

func init() {
	commandRequirementHook = (*World).commandRequirementFails
}

// commandRequirementFails is FUN_00440660. The rows of the command whose
// AccountLevel, level and reset ranges and class column (non-zero and <=
// ChangeUp+1) match the character are the ones checked ("*" = -1 = any);
// rows that do not match are skipped. In each matching row, in this
// order:
//   - CheckItem != -1: any item in the equipment slots 0..11 fails;
//   - ReqItemIndex != -1: fewer than ReqItemCount items of ReqItemLevel
//     (FUN_004948f0) fail;
//   - ReqEffectState != -1: the effect must be active (FUN_00478ec0);
//   - ReqQuestIndex != -1: the quest must be in ReqQuestState
//     (FUN_004dd740; a negative index fails);
//   - ReqPKLevel must equal obj+0x19c, ReqPKCount <= obj+0x1a0,
//     ReqGuildRank must equal the guild status (obj+0x30c);
//   - MapIndex must equal the map; with MapStartX and MapStartY set, the
//     position must be inside StartX..EndX, StartY..EndY;
//   - DayOfWeek (1 = Sunday) and MinHour..MaxHour use GetSystemTime, so
//     they are UTC.
//
// The first failure sends FailMessage (Message.txt, blue notice
// FUN_004c08f0 type 1) when it is not -1 and returns true; when every
// matching row passes it returns false.
func (w *World) commandRequirementFails(o *Object, index int) bool {
	if o.P == nil || o.P.Char == nil {
		return false
	}
	for i := range w.dus().requirements {
		r := &w.dus().requirements[i]
		if r.Index != index || !w.requirementApplies(o, r) {
			continue
		}
		if !w.requirementPasses(o, r) {
			if r.FailMessage != -1 {
				w.systemMessage(o, w.messageFor(o, r.FailMessage))
			}
			return true
		}
	}
	return false
}

// requirementApplies is the row filter of FUN_00440660.
func (w *World) requirementApplies(o *Object, r *gamedata.CommandRequirement) bool {
	ch := o.P.Char
	return (r.AccountLevel == -1 || r.AccountLevel <= o.P.Account.Level) &&
		inReqRange(r.MinLevel, r.MaxLevel, ch.Level) && inReqRange(r.MinReset, r.MaxReset, ch.Resets) &&
		classColumnOK(o, r.Class)
}

func inReqRange(lo, hi, v int) bool { return (lo == -1 || lo <= v) && (hi == -1 || v <= hi) }

// requirementPasses runs the checks of one matching row.
func (w *World) requirementPasses(o *Object, r *gamedata.CommandRequirement) bool {
	ch := o.P.Char
	if r.CheckItem != -1 {
		for s := 0; s < 12; s++ {
			if !o.P.Inventory[s].IsEmpty() {
				return false
			}
		}
	}
	if r.ReqItem != -1 && w.countItems(o, r.ReqItem, r.ReqItemLevel) < r.ReqItemCount {
		return false
	}
	if r.ReqEffect != -1 && !hasEffectIndex(o, r.ReqEffect) {
		return false
	}
	if r.ReqQuestIndex != -1 && (r.ReqQuestIndex < 0 || r.ReqQuestIndex >= 200 ||
		questState(o, r.ReqQuestIndex) != r.ReqQuestState) {
		return false
	}
	if r.ReqPKLevel != -1 && r.ReqPKLevel != o.PKLevel&0xFF {
		return false
	}
	if r.ReqPKCount != -1 && r.ReqPKCount > ch.PKCount {
		return false
	}
	if r.ReqGuildRnk != -1 && r.ReqGuildRnk != guildOf(o).Status {
		return false
	}
	if r.Map != -1 && r.Map != o.Map&0xFF {
		return false
	}
	if r.StartX != -1 && r.StartY != -1 &&
		(o.X < r.StartX || o.X > r.EndX || o.Y < r.StartY || o.Y > r.EndY) {
		return false
	}
	now := w.now().UTC() // GetSystemTime
	if r.DayOfWeek != -1 && r.DayOfWeek != int(now.Weekday())+1 {
		return false
	}
	if r.MinHour != -1 && now.Hour() < r.MinHour {
		return false
	}
	if r.MaxHour != -1 && now.Hour() > r.MaxHour {
		return false
	}
	return true
}
