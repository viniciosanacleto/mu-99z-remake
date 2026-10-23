package game

import (
	"context"
	"math"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/item"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/frame"
)

// Quest system (CQuest / CQuestObjective / CQuestReward, Data/Quest/*.txt).
// Each quest index < 200 keeps a 2-bit state in the 50-byte Quest column
// (obj+0x58e): 3 = never started (the column starts as 0xFF), 0 and 1 in
// progress, 2 = completed. Quest.txt has one row per (quest, state); the
// row matching the current state carries the requirements.

const (
	questMax  = 200
	ifQuest   = 10 // obj+0x3d4 = (x & 0xffff0281) | 0x281: use 1, type 10
	maxZen    = 2000000000
	questNone = 3
)

// Objective and reward types read by FUN_004de6e0 / FUN_004de880 /
// FUN_004df440.
const (
	questObjItem  = 1
	questObjMoney = 2

	questRewardPoints     = 1
	questRewardEvolution  = 2
	questRewardExp        = 3
	questRewardHero       = 4
	questRewardMoney      = 5
	questRewardItem       = 6
	questRewardEffect     = 7
	questRewardFlag       = 8
	questRewardEvolution2 = 0x10
)

var (
	// questTalkHook runs before the NpcTalk.txt messages (npc.go).
	questTalkHook func(w *World, o, npc *Object) bool
	// questDropHook is the quest item drop of the monster death
	// (combat.go): true replaces the normal drop.
	questDropHook func(w *World, mon *Object) bool
	// questEffectHook applies a reward of type 7 (FUN_00478ae0, effect
	// index and duration); set by the effect module.
	questEffectHook func(w *World, o *Object, effect, value int)
)

func init() {
	startHooks = append(startHooks, func(w *World) error {
		if w.dataDir == "" {
			w.quests = &gamedata.Quests{}
			return nil
		}
		q, err := gamedata.LoadQuests(w.dataDir)
		if err != nil {
			return err
		}
		w.quests = q
		return nil
	})
	questTalkHook = func(w *World, o, npc *Object) bool { return w.questNpcTalk(o, npc) }
	questDropHook = func(w *World, mon *Object) bool { return w.questItemDrop(mon) }
	worldHandlers[0xA0] = (*World).handleQuestList
	worldHandlers[0xA2] = (*World).handleQuestState
}

// questState is FUN_004dd680 / the 2-bit read: 0 for indexes >= 200.
func questState(o *Object, idx int) int {
	q := o.P.Char.Quest
	if idx < 0 || idx >= questMax || idx/4 >= len(q) {
		return 0
	}
	return int(q[idx/4]>>(uint(idx%4)*2)) & 3
}

// questByte is the whole state byte holding a quest (sent in A1/A2).
func questByte(o *Object, idx int) byte {
	q := o.P.Char.Quest
	if idx < 0 || idx >= questMax || idx/4 >= len(q) {
		return 0
	}
	return q[idx/4]
}

// setQuestState is FUN_004dd620.
func setQuestState(o *Object, idx, st int) bool {
	q := o.P.Char.Quest
	if idx < 0 || idx >= questMax || idx/4 >= len(q) {
		return false
	}
	sh := uint(idx%4) * 2
	q[idx/4] = q[idx/4]&^(3<<sh) | byte(st&3)<<sh
	return true
}

// changeUp is obj+0xe4 (the low nibble of the DB class byte, obj+0xe2).
func changeUp(o *Object) int { return int(o.P.Char.Class & 0x0F) }

// classColumnOK is the class check shared by the three tables: the column
// of the character class must be non-zero and <= ChangeUp + 1.
func classColumnOK(o *Object, col [5]int) bool {
	if o.Class < 0 || o.Class >= len(col) {
		return false
	}
	v := col[o.Class]
	return v != 0 && v <= changeUp(o)+1
}

// questRowOK is FUN_004dd6a0: state, required quest, level range, class.
func questRowOK(o *Object, r gamedata.QuestInfo) bool {
	if r.Index < 0 || r.Index >= questMax || questState(o, r.Index) != r.State {
		return false
	}
	if r.RequireIndex != -1 && (r.RequireIndex < 0 || questState(o, r.RequireIndex) != r.RequireState) {
		return false
	}
	lv := o.P.Char.Level
	if r.RequireMinLevel != -1 && lv < r.RequireMinLevel {
		return false
	}
	if r.RequireMaxLv != -1 && lv > r.RequireMaxLv {
		return false
	}
	return classColumnOK(o, r.Class)
}

// questFind is FUN_004dd5c0: the first row of index the player matches.
func (w *World) questFind(o *Object, idx int) *gamedata.QuestInfo {
	for i := range w.quests.Quests {
		r := &w.quests.Quests[i]
		if r.Index == idx && questRowOK(o, *r) {
			return r
		}
	}
	return nil
}

// objectiveActive is FUN_004de780: the objective's required quest is in
// RequireState and the class column allows the character.
func objectiveActive(o *Object, ob gamedata.QuestObjective) bool {
	if ob.RequireIndex != -1 {
		if ob.RequireIndex < 0 || ob.RequireIndex >= questMax || questState(o, ob.RequireIndex) != ob.RequireState {
			return false
		}
	}
	return classColumnOK(o, ob.Class)
}

// rewardActive is FUN_004df390 (same checks on QuestReward.txt).
func rewardActive(o *Object, r gamedata.QuestReward) bool {
	if r.RequireIndex != -1 {
		if r.RequireIndex < 0 || r.RequireIndex >= questMax || questState(o, r.RequireIndex) != r.RequireState {
			return false
		}
	}
	return classColumnOK(o, r.Class)
}

// objectiveCount is FUN_004de6e0: items of Index/Level in the inventory
// (FUN_004948f0, stacks count their durability) or the zen.
func (w *World) objectiveCount(o *Object, ob gamedata.QuestObjective) int64 {
	switch ob.Type {
	case questObjItem:
		return int64(w.countItems(o, ob.Index, ob.Level))
	case questObjMoney:
		return o.P.Char.Money
	}
	return 0
}

// countItems is FUN_004948f0 over the whole inventory (level -1 = any).
func (w *World) countItems(o *Object, index, level int) int {
	n := 0
	for _, it := range o.P.Inventory {
		if it.IsEmpty() || it.Index != index || (level != -1 && it.Level != level) {
			continue
		}
		if w.isStackItem(it) {
			n += int(it.Durability)
		} else {
			n++
		}
	}
	return n
}

func (w *World) isStackItem(it item.Item) bool {
	return w.itemData != nil && w.itemData.stack != nil && w.itemData.stack.Max(it.Index, it.Level) > 0
}

// deleteItems is FUN_00496ee0: remove count items of index/level, slot
// by slot (C1 05 28 slot 1); a stack bigger than the rest only loses
// durability (C1 06 2A slot dur 0).
func (w *World) deleteItems(o *Object, index, level, count int) {
	for s := 0; s < len(o.P.Inventory) && count > 0; s++ {
		it := o.P.Inventory[s]
		if it.IsEmpty() || it.Index != index || (level != -1 && it.Level != level) {
			continue
		}
		if w.isStackItem(it) && float32(count) < it.Durability {
			o.P.Inventory[s].Durability -= float32(count)
			w.send(o, packetItemDurability(s, byte(math.Round(float64(o.P.Inventory[s].Durability))), 0))
			return
		}
		if w.isStackItem(it) {
			count -= int(it.Durability)
		} else {
			count--
		}
		w.deleteInventory(o, s)
		w.send(o, packetItemDelete(s, 1))
	}
}

// objectivesDone is FUN_004de830: every active objective of the quest has
// its quantity.
func (w *World) objectivesDone(o *Object, idx int) bool {
	for _, ob := range w.quests.Objectives {
		if ob.RequireIndex == idx && objectiveActive(o, ob) && w.objectiveCount(o, ob) < int64(ob.Quantity) {
			return false
		}
	}
	return true
}

// consumeObjectives is FUN_004de880: take the items and the zen.
func (w *World) consumeObjectives(o *Object, idx int) {
	for _, ob := range w.quests.Objectives {
		if ob.RequireIndex != idx || !objectiveActive(o, ob) {
			continue
		}
		switch ob.Type {
		case questObjItem:
			w.deleteItems(o, ob.Index, ob.Level, ob.Quantity)
		case questObjMoney:
			o.P.Char.Money -= int64(ob.Quantity)
			w.sendMoney(o)
		}
	}
}

// giveRewards is FUN_004df440.
func (w *World) giveRewards(o *Object, idx int) {
	c := o.P.Char
	for _, r := range w.quests.Rewards {
		if r.RequireIndex != idx || !rewardActive(o, r) {
			continue
		}
		switch r.Type {
		case questRewardPoints:
			c.LevelUpPoints += r.Quantity
			w.sendQuestReward(o, r.Index, r.Quantity)
		case questRewardEvolution, questRewardEvolution2:
			to := 1
			if r.Type == questRewardEvolution2 {
				to = 2
			}
			if changeUp(o) < to {
				c.Class = c.Class&0xF0 + byte(to)
			}
			w.rebuildCharSet(o) // FUN_004c6ba0
			w.sendQuestReward(o, r.Index, int(evolutionByte(o)))
		case questRewardExp:
			// CServerInfo 0x340 + AL*4 and 0x3cc have no configuration key
			// in this build: zero (open unknown U-QUEST-01).
			exp := int64(questExpRate(o)) * int64(r.Quantity)
			if !w.levelUp(o, exp, 0) {
				w.send(o, packetKillExp(0xFFFF, uint32(exp), 0, uint32(c.Experience), o.P.NextExp))
			}
		case questRewardHero:
			if o.Class == 2 {
				if s := w.addSkill(o, 0x4D, 0); s >= 0 {
					w.send(o, packetSkillChange(0xFE, s, o.P.Skills[s]))
				}
			}
			n := 0
			if min := w.cfg.Int("Common", "PlusStatMinLevel", 0); c.Level > min {
				n = c.Level - min
			}
			pts := w.cfg.Int("Common", "PlusStatPoint", 0) * n
			c.LevelUpPoints += pts
			w.sendQuestReward(o, r.Index, pts)
		case questRewardMoney:
			if v := c.Money + int64(r.Quantity); r.Quantity >= 0 && v <= maxZen {
				c.Money = v
			} else {
				c.Money = maxZen
			}
			w.sendMoney(o)
		case questRewardItem:
			for i := 0; i < r.Quantity; i++ {
				w.createInventoryItem(o, item.Item{Index: r.Index, Level: r.Level, Skill: r.Option1 != 0,
					Luck: r.Option2 != 0, Option: r.Option3, Excellent: byte(r.NewOption)})
			}
		case questRewardEffect:
			if questEffectHook != nil {
				questEffectHook(w, o, r.Index, r.Quantity)
			}
		case questRewardFlag:
			w.sendQuestReward(o, r.Index, r.Quantity)
		}
	}
}

func questExpRate(o *Object) int { return 0 }

// evolutionByte is the value sent for evolution rewards:
// (ChangeUp<<4) + (Class<<5) - ((ChangeUp<<4)>>5), as bytes.
func evolutionByte(o *Object) byte {
	cu := byte(changeUp(o)) << 4
	return cu + (byte(o.Class)<<5 - cu>>5)
}

// createInventoryItem is FUN_004753e0 with map 0xEB: the DataServer gives
// a serial and the reply puts the item in the first free bag position
// (FUN_00495610) and sends C1 0C F3 14. Without room the item is lost.
func (w *World) createInventoryItem(o *Object, it item.Item) {
	if it.Durability == 0 {
		it.Durability = float32(w.itemMaxDurability(it))
	}
	c := o.conn
	w.async(func(ctx context.Context) func() {
		serial, err := w.store.NextItemSerial(ctx)
		return func() {
			o := w.objectOf(c)
			if err != nil || o == nil || o.State != StatePlaying {
				return
			}
			it.Serial = serial
			if s := w.insertInventory(o, it); s >= 0 {
				w.send(o, packetItemModify(s, it))
			}
		}
	})
}

// --- packets -----------------------------------------------------------

// packetQuestList is C1 36 A0 count state[50] (FUN_004dde70): count is
// one more than the highest quest index < 200 of Quest.txt.
func packetQuestList(count int, states []byte) []byte {
	p := newPacket(frame.C1, 0xA0, -1, 0x36)
	p[3] = byte(count)
	copy(p[4:], states)
	return p
}

// packetQuestInfo is C1 05 A1 quest stateByte (FUN_004ddfa0).
func packetQuestInfo(idx int, state byte) []byte {
	p := newPacket(frame.C1, 0xA1, -1, 5)
	p[3], p[4] = byte(idx), state
	return p
}

// packetQuestResult is C1 06 A2 quest result stateByte (FUN_004de060):
// result 0 ok, 0xFF refused.
func packetQuestResult(idx int, result, state byte) []byte {
	p := newPacket(frame.C1, 0xA2, -1, 6)
	p[3], p[4], p[5] = byte(idx), result, state
	return p
}

// packetQuestReward is C1 0C A3 index(BE) reward value 00 LevelUpPoint(u32)
// (FUN_004de110), sent to the player and its viewers.
func packetQuestReward(index, reward, value, points int) []byte {
	p := newPacket(frame.C1, 0xA3, -1, 0x0C)
	p[3], p[4] = byte(index>>8), byte(index)
	p[5], p[6] = byte(reward), byte(value)
	u32le(p[8:], uint32(points))
	return p
}

func (w *World) questCount() int {
	n := 0
	for _, r := range w.quests.Quests {
		if r.Index >= n && r.Index < questMax {
			n = r.Index + 1
		}
	}
	return n
}

// sendQuestList is FUN_004dde70: sent once per character entry.
func (w *World) sendQuestList(o *Object) {
	if o.P.questSent || w.quests == nil {
		return
	}
	w.send(o, packetQuestList(w.questCount(), o.P.Char.Quest))
	o.P.questSent = true
}

func (w *World) sendQuestReward(o *Object, reward, value int) {
	w.sendQuestList(o)
	p := packetQuestReward(o.Index, reward, value, o.P.Char.LevelUpPoints)
	w.send(o, p)
	w.broadcast(o, p)
}

// --- handlers ----------------------------------------------------------

// questPlayerOK is the precondition of A0/A2 (FUN_004ddb20/FUN_004ddb70):
// playing player, obj+10 < 1 and obj+0x964 == 0 (two flags not identified:
// the logout countdown and the server move are used instead).
func questPlayerOK(o *Object) bool {
	return o.State == StatePlaying && o.IsPlayer() && o.P.Char != nil && o.P.LogoutCountdown < 1 && !o.P.Moving
}

// handleQuestList is A0 (FUN_004ddb20).
func (w *World) handleQuestList(o *Object, p []byte) {
	if questPlayerOK(o) {
		w.sendQuestList(o)
	}
}

// handleQuestState is A2 (FUN_004ddb70): [3] quest index.
func (w *World) handleQuestState(o *Object, p []byte) {
	if len(p) < 4 || !questPlayerOK(o) {
		return
	}
	r := w.questFind(o, int(p[3]))
	if r == nil {
		return
	}
	idx := r.Index
	if !w.objectivesDone(o, idx) {
		w.send(o, packetQuestResult(idx, 0xFF, questByte(o, idx)))
		return
	}
	w.consumeObjectives(o, idx)
	w.giveRewards(o, idx)
	result := byte(0)
	switch r.State {
	case 0, questNone:
		setQuestState(o, idx, 1)
	case 1, 2:
		setQuestState(o, idx, 2)
		w.saveCharacter(o, nil) // FUN_00475c30
		if idx == 3 && r.State == 1 {
			w.send(o, packetCustomNotice(1, 0, "Quest de Marlon concluida! Combo do BK liberado."))
		}
		if r.State == 2 {
			result = 0xFF
		}
	}
	w.send(o, packetQuestResult(idx, result, questByte(o, idx)))
}

// questNpcTalk is FUN_004dd850: the first quest of the NPC (in file order,
// once per index) that is not completed and that the player can take
// opens the quest window; otherwise the last completed one does.
func (w *World) questNpcTalk(o, npc *Object) bool {
	if w.quests == nil {
		return false
	}
	seen := map[int]bool{}
	chosen, completed := -1, -1
	for _, r := range w.quests.Quests {
		if r.NPC != npc.Class || seen[r.Index] {
			continue
		}
		seen[r.Index] = true
		st := 0
		if r.Index >= 0 && r.Index < questMax {
			st = questState(o, r.Index)
		}
		if st == 2 {
			completed = r.Index
			continue
		}
		if w.questFind(o, r.Index) != nil {
			chosen = r.Index
			break
		}
	}
	if chosen == -1 {
		chosen = completed
	}
	if chosen == -1 {
		return false
	}
	items(o).If = ifState{Use: true, Type: ifQuest}
	w.sendQuestList(o)
	w.send(o, packetQuestInfo(chosen, questByte(o, chosen)))
	return true
}

// questItemDrop is FUN_004de9b0 (party variant FUN_004deba0 when
// QuestMonsterItemDropParty is set and the top hitter has a party): the
// first active item objective of the right map and monster level range
// drops its item with ItemDropRate/10000 while the owner has fewer than
// Quantity.
func (w *World) questItemDrop(mon *Object) bool {
	if w.quests == nil || mon.Mon == nil {
		return false
	}
	top := w.topHitter(mon)
	if top < 0 || top >= MaxObjects {
		return false
	}
	p := w.objects[top]
	if p.P == nil || p.P.Char == nil {
		return false
	}
	owners := []*Object{p}
	if pt := w.partyOf(p); pt != nil && w.cfg.Int("Common", "QuestMonsterItemDropParty", 0) != 0 {
		owners = owners[:0]
		for _, idx := range pt.members {
			if idx >= 0 && idx < MaxObjects && w.objects[idx].P != nil && w.objects[idx].P.Char != nil {
				owners = append(owners, w.objects[idx])
			}
		}
	}
	lv := mon.Mon.Level
	for _, owner := range owners {
		for _, ob := range w.quests.Objectives {
			if ob.Type != questObjItem || !objectiveActive(owner, ob) {
				continue
			}
			if ob.Map != -1 && ob.Map != mon.Map {
				continue
			}
			if (ob.DropMinLevel != -1 && lv < ob.DropMinLevel) || (ob.DropMaxLvl != -1 && lv > ob.DropMaxLvl) {
				continue
			}
			if rnd(10000) >= ob.ItemDropRate || w.objectiveCount(owner, ob) >= int64(ob.Quantity) {
				continue
			}
			it := item.Item{Index: ob.Index, Level: ob.Level, Skill: ob.Option1 != 0, Luck: ob.Option2 != 0,
				Option: ob.Option3, Excellent: byte(ob.NewOption), Serial: math.MaxUint32}
			it.Durability = float32(w.itemMaxDurability(it))
			if dropMapItemHook != nil {
				dropMapItemHook(w, mon.Map, mon.X, mon.Y, it, owner.Index)
			}
			return true
		}
	}
	return false
}

// QuestLevelUpPoints is FUN_004dd790: the LevelUpPoint rewards (type 1)
// of every completed quest (used by the reset commands).
func (w *World) questLevelUpPoints(o *Object) int {
	total := 0
	for idx := 0; idx < questMax; idx++ {
		if questState(o, idx) != 2 {
			continue
		}
		for _, r := range w.quests.Rewards {
			if r.RequireIndex == idx && r.Type == questRewardPoints {
				total += r.Quantity
			}
		}
	}
	return total
}
