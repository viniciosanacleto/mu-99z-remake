package game

import (
	"sort"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"
)

// CCustomEvo (Custom/CustomEvolution.txt, DAT_015bbf00, reader
// FUN_0044aff0, getter FUN_004c3660). Its only user is FUN_004ce0b0, run
// by the item use handler (FUN_00499a20) for item 14,211 (0x1cd3)
// dropped on another inventory item. The /evo command (command_player2.go,
// FUN_00437d50) does not read it: it completes the class quests.

// evolutionItem is the item that evolves another one.
const evolutionItem = 14*512 + 211

func sortedEvolution(m map[int]gamedata.EvolutionRow) []gamedata.EvolutionRow {
	out := make([]gamedata.EvolutionRow, 0, len(m))
	for _, r := range m {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Index < out[j].Index })
	return out
}

// useEvolution is the 0x1cd3 branch of FUN_00499a20 around FUN_004ce0b0
// (src/dst = packet [3]/[4], both < 0x4c, both holding an item; the
// target must not be a periodic item, CItem+0xb7):
//   - the first row (in Index order) whose ItemTarget is the target's
//     index: without room for ItemResult (FUN_004952a0) the player gets
//     "You do not have space  to use the Item!" (blue) and C1 0C 26 FD,
//     and nothing else happens; otherwise ItemResult is created in the
//     inventory (FUN_004753e0, CL = 0xEB at 0x4ce2ff) with the target's
//     level, durability, skill, luck, option, excellent and set option,
//     and the evolution item and the target are deleted (C1 05 28 for
//     each);
//   - no row: a target of the kinds 0..7 (FUN_004944b0) keeps its options
//     and gets its durability rescaled to the recomputed maximum
//     (FUN_00494500 / FUN_0048aed0) and the charset is rebuilt; other
//     targets end the use with nothing done.
//
// After a success the caller deletes the evolution item again (another
// C1 05 28) and sends C1 0C F3 14 for the target slot.
func (w *World) useEvolution(o *Object, src, dst int) {
	if src < 0 || src >= BagEnd || dst < 0 || dst >= BagEnd {
		return
	}
	inv := &o.P.Inventory
	if inv[src].IsEmpty() || inv[dst].IsEmpty() || inv[dst].Flag1 {
		return
	}
	if !w.evolve(o, src, dst) {
		return
	}
	w.deleteInventory(o, src)
	w.send(o, packetItemDelete(src, 1))
	w.send(o, packetItemModify(dst, inv[dst]))
}

// evolve is FUN_004ce0b0 without the slot checks.
func (w *World) evolve(o *Object, src, dst int) bool {
	inv := &o.P.Inventory
	t := inv[dst]
	for _, r := range w.dus().evolution {
		if t.Index != r.Target {
			continue
		}
		if w.findBagSlot(inv, r.Result) < 0 {
			w.systemMessage(o, "You do not have space  to use the Item!")
			w.send(o, packetLife(0xFD, int(roundf(o.P.Char.Life))&0xFF))
			return false
		}
		w.createItem(o, 0xEB, 0, 0, dropSpec{Index: r.Result, Level: t.Level,
			Dur: int(roundf(t.Durability)) & 0xFF, Skill: b2i(t.Skill), Luck: b2i(t.Luck), Option: t.Option,
			Exc: t.Excellent, Set: t.SetOption, OptionEx: t.OptionEx, ToInventory: true})
		for _, s := range []int{src, dst} {
			w.deleteInventory(o, s)
			w.send(o, packetItemDelete(s, 1))
		}
		return true
	}
	info, ok := w.itemInfo(t.Index)
	if !ok || info.Slot < 0 || info.Slot > 7 {
		return false
	}
	old := w.itemMaxDurability(t)
	w.rescaleDurability(&inv[dst], old)
	w.rebuildCharSet(o)
	return true
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}
