package game

import "github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"

// Game uses of the small data files parsed in gamedata/datafiles.go
// (docs/server-plan/16-cobertura-dados.md). Each system lives in its own
// file:
//
//   - freebies.go: Character/DefaultClassFreebies.txt (FUN_0046d0b0);
//   - cmdrequirement.go: CommandRequirement.txt (FUN_00440660);
//   - monstermove.go: Monster/MonsterMove.txt (FUN_004b1450);
//   - monsterschedule.go: Monster/MonsterSchedule.txt (FUN_004bb560,
//     FUN_004bb8d0);
//   - blacklist.go: Data/BlackList.txt IPs and hardware ids;
//   - customevo.go: Custom/CustomEvolution.txt (FUN_004ce0b0, item 14,211);
//   - itemlevel.go: Item/ItemLevel.txt names (FUN_00494a40);
//   - skillspeed.go: Hack/HackSkillSpeedCheck.txt (FUN_00488550);
//   - ghrs.go: Custom/CustomGHRS.ini (CastleSiege.exe only).

// dataUses is the state of those systems.
type dataUses struct {
	freebies     *gamedata.ClassFreebies
	requirements []gamedata.CommandRequirement
	blackIPs     map[string]bool
	blackHWIDs   map[string]bool
	evolution    []gamedata.EvolutionRow // std::map order (by Index)
	itemLevels   []gamedata.ItemLevelName
	speedRules   []gamedata.SkillSpeedRule
	speed        map[int]*skillSpeedState // by object index (obj+0xd44 / +0x16ec)

	moves    []*monsterMoveNode // std::map order (by monster class)
	schedule [scheduleGroups]scheduleGroup

	ghrs *ghrsState // nil outside the CastleSiege server
}

func (w *World) dus() *dataUses {
	if w.du == nil {
		w.du = &dataUses{freebies: &gamedata.ClassFreebies{}, blackIPs: map[string]bool{},
			blackHWIDs: map[string]bool{}, speed: map[int]*skillSpeedState{}}
	}
	return w.du
}

func init() {
	startHooks = append(startHooks, loadDataUses)
	tick1000Hooks = append(tick1000Hooks, (*World).monsterMoveSecond, (*World).monsterScheduleSecond)
	leaveWorldHooks = append(leaveWorldHooks, func(w *World, o *Object) { delete(w.dus().speed, o.Index) })
}

// loadDataUses reads the files at start-up (FUN_004e8520, FUN_004e85f0,
// FUN_004e86f0, FUN_004e8a50, FUN_004ea5d0, FUN_004ea820, FUN_004eb5f0;
// CastleSiege.exe FUN_004e2250 for CustomGHRS.ini).
func loadDataUses(w *World) error {
	w.du = nil
	d := w.dus()
	if w.configDir == "" {
		return nil
	}
	var err error
	if d.freebies, err = gamedata.LoadDefaultClassFreebies(w.configDir); err != nil {
		return err
	}
	if d.requirements, err = gamedata.LoadCommandRequirement(w.configDir); err != nil {
		return err
	}
	bl, err := gamedata.LoadBlackList(w.configDir)
	if err != nil {
		return err
	}
	for _, ip := range bl.IPs {
		d.blackIPs[ip] = true
	}
	for _, hw := range bl.HWIDs {
		d.blackHWIDs[hw] = true
	}
	evo, err := gamedata.LoadCustomEvolution(w.configDir)
	if err != nil {
		return err
	}
	d.evolution = sortedEvolution(evo)
	if d.itemLevels, err = gamedata.LoadItemLevel(w.configDir); err != nil {
		return err
	}
	if d.speedRules, err = gamedata.LoadHackSkillSpeedCheck(w.configDir); err != nil {
		return err
	}
	moves, err := gamedata.LoadMonsterMove(w.configDir)
	if err != nil {
		return err
	}
	w.setMonsterMoves(moves)
	sched, err := gamedata.LoadMonsterSchedule(w.configDir)
	if err != nil {
		return err
	}
	w.setMonsterSchedule(sched)
	if w.cfg.ServerType == 1 {
		g, err := gamedata.LoadCustomGHRS(w.configDir)
		if err != nil {
			return err
		}
		d.ghrs = &ghrsState{cfg: *g}
	}
	return nil
}
