package game

import (
	"context"
	"encoding/binary"
	"fmt"
	"math/rand/v2"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/chaosmix"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/item"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/packet"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/frame"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/store"
)

// Chaos Machine (CChaosBox, FUN_0042a800..FUN_0042ff70;
// docs/server-plan/09c-chaos-loja-pets.md).
//
//   - Chaos Goblin talk (NPC 0xEE, FUN_004c2390) opens interface type 7,
//     sends C3 0B 30 03, C1 06 B2 1A 01 tax and begins the inventory
//     transaction (FUN_0050d380).
//   - 24 moves with flags 3 and 5..20 move items between the inventory and
//     the 8x4 chaos box (obj+0x420, 32 CItems).
//   - 86 [3] mix type (FUN_0042ebd0) runs one recipe; 87 (FUN_0042fcb0)
//     closes the box (clear + commit, C1 03 87); 88 [4..7] mix type
//     (FUN_0042ee40) previews rate, zen and tax (C1 10 88).
//   - Results: C1 0B 86 result item[7] (FUN_0042ff70): 0 failure, 1 success,
//     2 not enough zen, 7 wrong items, 8 wrong level, 0xF0 too many charms.
//     Items made by a mix go through the DataServer serial request
//     (FUN_004753e0 with map 0xFF) and land in chaos box slot 0 when the
//     answer arrives (FUN_004742b0).

// ifChaosBox2 is interface 13 (0x340): the second mix window (pet
// trainer box). The 24 flag 5 and the pet mix failure list (C2 31 05) use
// it.
const ifChaosBox2 = 13

// chaosRand stands for FUN_005138d0 / FUN_00513720 (non-negative random
// numbers); tests replace it.
var chaosRand = func() int { return int(rand.Int32()) }

// chaosState is the per-player chaos box state.
type chaosState struct {
	Box   [chaosmix.BoxSlots]item.Item // obj+0x420
	Lock  bool                         // obj+0x430: a mix is running (cleared by the 86 result)
	Money int64                        // obj+0x428: zen of the current mix (with tax)
	Rate  int                          // obj+0x42c: success rate of the current mix
	Mixed bool                         // obj+0x5d7: a mix was attempted since the box opened

	// Inventory transaction (obj+0x3dc; FUN_0050d380 begin, FUN_0050d620
	// commit, FUN_0050d750 rollback): the box works on a copy of the
	// inventory; the saved inventory is the one from before the box opened
	// until the transaction is committed.
	Trans       int // 0 none, 1 open, 2 committed, 3 rolled back
	backup      [InventorySlots]item.Item
	skillBackup [MagicSlots]Skill
}

func chaos(o *Object) *chaosState {
	if o.P == nil {
		return &chaosState{}
	}
	if o.P.Chaos == nil {
		c := &chaosState{}
		for i := range c.Box {
			c.Box[i] = item.Empty()
		}
		o.P.Chaos = c
	}
	return o.P.Chaos
}

// chaosmix.Data returns the World's tables (worlds of the same process load
// them independently).
func (w *World) chaosData() *chaosmix.Data { return w.chaos }

func registerChaos(h *hooks) {
	h.npcTalk[chaosmix.Goblin] = func(w *World, o, npc *Object) bool {
		w.chaosTalk(o)
		return true
	}
	h.worldHandlers[0x86] = func(w *World, o *Object, p []byte) { w.handleChaosMix(o, p) }
	h.worldHandlers[0x87] = func(w *World, o *Object, p []byte) { w.handleChaosClose(o) }
	h.worldHandlers[0x88] = func(w *World, o *Object, p []byte) { w.handleChaosRate(o, p) }
	h.start = append(h.start, loadChaosData)
	h.leaveWorld = append(h.leaveWorld, func(w *World, o *Object) { w.chaosLogout(o) })
	h.characterSave = append(h.characterSave, func(w *World, o *Object, c *store.Character) {
		// GJSetCharacterInfo writes Inventory1: while the chaos box is open
		// the saved inventory is the one from before it opened.
		if o.P != nil && o.P.Chaos != nil && o.P.Chaos.Trans == 1 {
			inv := make([]byte, store.InventorySize)
			for i, it := range o.P.Chaos.backup {
				it.EncodeDB(inv[i*store.ItemDBSize:])
			}
			c.Inventory = inv
		}
	})
}

func loadChaosData(w *World) error {
	d := &chaosmix.Data{}
	var err error
	if d.Bags, err = gamedata.LoadEventItemBags(w.configDir); err != nil {
		return err
	}
	if d.Rates, err = gamedata.LoadItemOptionRate(w.configDir); err != nil {
		return err
	}
	if d.Exc, err = gamedata.LoadExcellentOptionRate(w.configDir); err != nil {
		return err
	}
	if d.Sets, err = gamedata.LoadSetItemType(w.configDir); err != nil {
		return err
	}
	w.chaos = d
	return nil
}

// --- inventory transaction ---------------------------------------------------

// chaosBegin is FUN_0050d380.
func (w *World) chaosBegin(o *Object) {
	c := chaos(o)
	if c.Trans == 1 {
		return
	}
	c.backup = o.P.Inventory
	c.skillBackup = o.P.Skills
	c.Trans = 1
}

// chaosCommit is FUN_0050d620: true when a transaction was open.
func (w *World) chaosCommit(o *Object) bool {
	c := chaos(o)
	if c.Trans != 1 {
		return false
	}
	c.Trans = 2
	return true
}

// chaosTransactionOpen is the "obj+0x3dc == 1" test (chaos box or trade).
func chaosTransactionOpen(o *Object) bool {
	if o.P == nil {
		return false
	}
	if o.P.Chaos != nil && o.P.Chaos.Trans == 1 {
		return true
	}
	return o.P.Trade != nil && o.P.Trade.backupValid
}

// chaosLogout is the chaos part of FUN_004c4b40: when a mix happened in
// the open box, commit and give the box items back to the inventory
// (FUN_0042ad20); otherwise the inventory from before the box is kept.
func (w *World) chaosLogout(o *Object) {
	if o.P == nil || o.P.Chaos == nil {
		return
	}
	c := o.P.Chaos
	st := items(o)
	open := st.If.Use && (st.If.Type == ifChaosBox || st.If.Type == ifChaosBox2)
	if open && c.Mixed {
		if w.chaosCommit(o) {
			for _, it := range c.Box {
				if !it.IsEmpty() {
					w.insertInventory(o, it)
				}
			}
		}
	} else if c.Trans == 1 {
		o.P.Inventory = c.backup
		o.P.Skills = c.skillBackup
		c.Trans = 3
	}
	w.chaosClear(o)
	c.Lock = false
	c.Mixed = false
}

// --- packets -----------------------------------------------------------------

// packetTaxInfo is C1 06 B2 1A type rate (FUN_004dae50).
func packetTaxInfo(typ, rate byte) []byte {
	p := packet.New(frame.C1, 0xB2, 0x1A, 6)
	p[4], p[5] = typ, rate
	return p
}

// chaosSendResult is FUN_0042ff70: it also releases the mix lock.
func (w *World) chaosSendResult(o *Object, result byte, it *item.Item) {
	w.send(o, chaosmix.PacketResult(result, it))
	chaos(o).Lock = false
}

// chaosSendList is FUN_0042fd60(second): only when the matching interface
// is open and its state is not 1.
func (w *World) chaosSendList(o *Object, second bool) {
	st := items(o).If
	want, typ := ifChaosBox, byte(3)
	if second {
		want, typ = ifChaosBox2, 5
	}
	if st.Type != want || st.State == 1 {
		return
	}
	w.send(o, chaosmix.PacketList(typ, chaos(o).Box[:]))
}

// chaosClear is FUN_0042a910.
func (w *World) chaosClear(o *Object) {
	c := chaos(o)
	for i := range c.Box {
		c.Box[i] = item.Empty()
	}
}

// castleTax is CCastleSiegeSync+8 through FUN_00429aa0 (0 for the owner
// guild and its alliance; castlesiege_tax.go).
func (w *World) castleTax(o *Object) int64 { return int64(w.castleChaosTax(o)) }

func (w *World) chaosLog(format string, args ...any) {
	if w.cfg.Int("Common", "WriteChaosMixLog", 0) != 0 {
		w.log.Info(fmt.Sprintf(format, args...), "log", "CHAOS_MIX_LOG")
	}
}

// --- NPC talk / close ------------------------------------------------------------

// chaosTalk is FUN_004c2390 (NPC 0xEE through FUN_004c1490: no interface
// may be in use and the personal shop must be closed).
func (w *World) chaosTalk(o *Object) {
	st := items(o)
	if st.If.Use || pshopOpen(o) {
		return
	}
	c := chaos(o)
	if c.Trans == 1 {
		return
	}
	st.If = ifState{Use: true, State: 0, Type: ifChaosBox}
	w.send(o, packetNpcWindow(3))
	w.send(o, packetTaxInfo(1, byte(w.castleTax(o))))
	c.Mixed = false
	w.chaosBegin(o)
}

// handleChaosClose is 87 (FUN_0042fcb0).
func (w *World) handleChaosClose(o *Object) {
	if !playerReady(o) || o.P.Moving {
		return
	}
	st := items(o)
	if !st.If.Use || st.If.Type != ifChaosBox {
		return
	}
	st.If = ifState{}
	w.chaosClear(o)
	w.chaosCommit(o)
	c := chaos(o)
	c.Lock, c.Mixed = false, false
	w.send(o, packet.New(frame.C1, 0x87, -1, 3))
}

// --- item moves (24 flags 3, 5..20) ------------------------------------------------

// isChaosFlag reports the chaos box flags of 24.
func isChaosFlag(f int) bool { return f == 3 || (f > 4 && f < 0x15) }

// chaosMoveAllowed is the chaos part of the validation of FUN_00499410:
// flags 3 and 6..20 need interface 7, flag 5 interface 13, both with no
// mix running.
func chaosMoveAllowed(o *Object, sf, tf int) bool {
	st := items(o).If
	c := chaos(o)
	chk := func(f int) bool {
		switch {
		case f == 5:
			return st.Use && st.Type == ifChaosBox2 && !c.Lock
		case isChaosFlag(f):
			return st.Use && st.Type == ifChaosBox && !c.Lock
		}
		return true
	}
	return chk(sf) && chk(tf)
}

// chaosFits is the placement test of FUN_004965b0 / FUN_00496540 on the
// 8x4 box.
func (w *World) chaosFits(c *chaosState, index, slot, skip int) bool {
	wd, ht := w.itemSize(index)
	return gridFits(w, c.Box[:], 4, index, slot, skip, wd, ht)
}

// accountLockRefused is the "obj+0x24fc >= 1" test with message 0x6F2.
func (w *World) accountLockRefused(o *Object) bool {
	if o.P.Account.Lock < 1 {
		return false
	}
	w.systemMessage(o, w.messageFor(o, 0x6f2))
	return true
}

// inventoryToChaos is FUN_00497fc0 (with the item test FUN_004950b0).
func (w *World) inventoryToChaos(o *Object, src, dst, flag int) int {
	if src < 0 || src >= BagEnd || dst < 0 || dst >= chaosmix.BoxSlots {
		return -1
	}
	it := o.P.Inventory[src]
	if it.IsEmpty() || it.Flag1 {
		return -1
	}
	// FUN_0048ae70 (CLuckyItem) knows no lucky items in this build: flag
	// 16 (lucky item box) refuses everything, the others accept all.
	if flag == 0x10 {
		return -1
	}
	if w.accountLockRefused(o) {
		return -1
	}
	c := chaos(o)
	if !c.Box[dst].IsEmpty() || !w.chaosFits(c, it.Index, dst, -1) {
		return -1
	}
	c.Box[dst] = it
	w.deleteInventory(o, src)
	w.equipmentChanged(o, src)
	return flag
}

// chaosToInventory is FUN_004979a0.
func (w *World) chaosToInventory(o *Object, src, dst int) int {
	if src < 0 || src >= chaosmix.BoxSlots || dst < 0 || dst >= BagEnd {
		return -1
	}
	c := chaos(o)
	it := c.Box[src]
	if it.IsEmpty() || !w.canEquip(o, it, dst) {
		return -1
	}
	if w.placeInventory(o, it, dst) < 0 {
		return -1
	}
	c.Box[src] = item.Empty()
	w.equipmentChanged(o, dst)
	return moveInventory
}

// chaosToChaos is FUN_00498090.
func (w *World) chaosToChaos(o *Object, src, dst, flag int) int {
	if src < 0 || src >= chaosmix.BoxSlots || dst < 0 || dst >= chaosmix.BoxSlots || src == dst {
		return -1
	}
	c := chaos(o)
	it := c.Box[src]
	if it.IsEmpty() || !c.Box[dst].IsEmpty() || !w.chaosFits(c, it.Index, dst, src) {
		return -1
	}
	c.Box[dst] = it
	c.Box[src] = item.Empty()
	return flag
}

// --- mixes (86) ----------------------------------------------------------------

// handleChaosMix is 86 (FUN_0042ebd0).
func (w *World) handleChaosMix(o *Object, p []byte) {
	if len(p) < 4 || !playerReady(o) || o.P.Moving || pshopOpen(o) {
		return
	}
	c := chaos(o)
	if c.Lock {
		return
	}
	c.Lock, c.Money, c.Rate, c.Mixed = true, 0, 0, true
	switch p[3] {
	case 1:
		w.mixChaosItem(o)
	case 2:
		w.mixDevilSquare(o)
	case 3:
		w.mixPlusItem(o, 0)
	case 4:
		w.mixPlusItem(o, 1)
	case 0x16:
		w.mixPlusItem(o, 2)
	case 0x17:
		w.mixPlusItem(o, 3)
	case 0x31:
		w.mixPlusItem(o, 4)
	case 0x32:
		w.mixPlusItem(o, 5)
	case 5:
		w.mixDinorant(o)
	case 6:
		w.mixFruit(o)
	case 7:
		w.mixWing2(o, 0)
	case 0x18:
		w.mixWing2(o, 1)
	case 8:
		w.mixBloodCastle(o)
	case 0xB:
		w.mixWing1(o)
	case 0xD:
		w.mixPet(o, 0)
	case 0xE:
		w.mixPet(o, 1)
	case 0xF:
		w.mixSiegePotion(o, 0)
	case 0x10:
		w.mixSiegePotion(o, 1)
	case 0x11:
		w.mixLifeStone(o)
	case 0x12:
		w.mixSenior(o)
	case 0x28:
		w.mixChaosCard(o)
	case 0x29:
		w.mixCherryBlossom(o)
	default:
		// Other types keep the lock set, as the original does.
	}
}

// jewelValue is FUN_0048ced0: the value a jewel adds to the rate of the
// value-based mixes (other items add their buy price, CItem+0x7c).
func (w *World) jewelValue(it item.Item) int64 {
	switch it.Index {
	case chaosmix.ItemBless:
		return 100000
	case chaosmix.ItemSoul:
		return 70000
	case chaosmix.ItemChaos:
		return 40000
	case chaosmix.ItemCreation, chaosmix.ItemLife:
		return 450000
	}
	return w.itemPrice(it).Buy
}

// isExcellent is FUN_0048ad90 (Dinorant and Fenrir options are not
// excellent).
func isExcellent(it item.Item) bool {
	return it.Index != chaosmix.ItemDinorant && it.Index != chaosmix.ItemFenrir && it.Excellent != 0
}

// isSet is FUN_0048adc0 (CItem+0xb3 & 3).
func isSet(it item.Item) bool { return it.SetOption&3 != 0 }

// charmBonus is FUN_0042ae00: every Charm of Luck adds its durability to
// the rate; more than 10 points refuse the mix (0xF0).
func (w *World) charmBonus(c *chaosState, rate *int) bool {
	n := 0
	for _, it := range c.Box {
		if it.Index == chaosmix.ItemCharmLuck {
			n += int(it.Durability)
		}
	}
	*rate += n
	return n < 11
}

// bonusRate is CBonusManager FUN_004188f0 type 2 (chaos rate, bonus.go).
func (w *World) bonusRate(o *Object, v int) int { return w.bonusValue(o, BonusChaosMix, v) }

// chaosCharge adds the castle tax to c.Money, checks and takes the zen
// and sends C3 10 22 FE. false (after result 2) when the zen is short.
func (w *World) chaosCharge(o *Object) bool {
	c := chaos(o)
	tax := c.Money * w.castleTax(o) / 100
	c.Money += tax
	if uint32(c.Money) > uint32(o.P.Char.Money) {
		w.chaosSendResult(o, chaosmix.ResultNoMoney, nil)
		return false
	}
	o.P.Char.Money -= c.Money
	w.sendMoney(o)
	w.castleAddTax(tax) // CCastleSiegeSync+0x14 (castlesiege_tax.go)
	return true
}

// chaosRateCfg reads a ChaosMix rate key for the account level.
func (w *World) chaosRateCfg(o *Object, key string) int {
	return w.cfg.ALInt("ChaosMix", key, accountLevel(o), 0)
}

// chaosFailClear is the common failure: clear the box, send the (empty)
// list and result 0.
func (w *World) chaosFailClear(o *Object, second bool) {
	w.chaosClear(o)
	w.chaosSendList(o, second)
	w.chaosSendResult(o, chaosmix.ResultFail, nil)
}

// chaosRoll returns rnd()%100 < rate (FUN_005138d0 / FUN_00513720).
func chaosRoll(rate int) bool { return chaosRand()%100 < rate }

// mixChaosItem is FUN_0042ae60 (86 01): Chaos Weapon.
func (w *World) mixChaosItem(o *Object) {
	c := chaos(o)
	chaosN, items, value := 0, 0, int64(0)
	for _, it := range c.Box {
		if it.IsEmpty() {
			continue
		}
		switch it.Index {
		case chaosmix.ItemChaos:
			chaosN++
			value += w.jewelValue(it)
		case chaosmix.ItemBless, chaosmix.ItemSoul:
			value += w.jewelValue(it)
		default:
			if it.Level > 3 && it.Option != 0 {
				items++
				value += w.itemPrice(it).Buy
			}
		}
	}
	if chaosN == 0 || items == 0 {
		w.chaosSendResult(o, chaosmix.ResultWrongItems, nil)
		return
	}
	cfg := w.chaosRateCfg(o, "ChaosItemMixRate")
	c.Rate = cfg
	if cfg == -1 {
		c.Rate = int(value / 20000)
	}
	if !w.charmBonus(c, &c.Rate) {
		w.chaosSendResult(o, chaosmix.ResultTooManyCharms, nil)
		return
	}
	c.Rate = min(w.bonusRate(o, c.Rate), 100)
	c.Money = int64(c.Rate) * 10000
	if !w.chaosCharge(o) {
		return
	}
	if chaosRoll(c.Rate) {
		res, ok := w.chaosBagItem(o, 0, 0)
		if !ok {
			w.chaosSendResult(o, chaosmix.ResultFail, nil)
			return
		}
		w.chaosCreateItem(o, res, false)
		w.chaosLog("[ChaosItemMix][Success][%s][%s] - (ChaosSuccessRate: %d,ChaosMoney: %d)",
			o.P.Account.Login, o.P.Char.Name, c.Rate, c.Money)
		return
	}
	w.chaosFailClear(o, false)
	w.chaosLog("[ChaosItemMix][Failure][%s][%s] - (ChaosSuccessRate: %d,ChaosMoney: %d)",
		o.P.Account.Login, o.P.Char.Name, c.Rate, c.Money)
}

// pairedLevels counts the chaos jewels and the two paired ticket parts of
// the Devil Square / Blood Castle mixes (last seen level of each part).
func pairedLevels(c *chaosState, a, b int) (chaosN, na, nb, la, lb int) {
	for _, it := range c.Box {
		switch {
		case it.IsEmpty():
		case it.Index == chaosmix.ItemChaos:
			chaosN++
		case it.Index == a:
			na++
			la = it.Level
		case it.Index == b:
			nb++
			lb = it.Level
		}
	}
	return
}

// mixDevilSquare is FUN_0042b2a0 (86 02): Devil's Invitation.
func (w *World) mixDevilSquare(o *Object) {
	c := chaos(o)
	chaosN, ne, nk, le, lk := pairedLevels(c, chaosmix.ItemDSEye, chaosmix.ItemDSKey)
	if chaosN != 1 || ne != 1 || nk != 1 || le != lk {
		w.chaosSendResult(o, chaosmix.ResultWrongItems, nil)
		return
	}
	if le > 7 || lk > 7 {
		w.chaosSendResult(o, chaosmix.ResultWrongLevel, nil)
		return
	}
	money := [8]int64{0, 100000, 200000, 400000, 700000, 1100000, 1600000, 2000000}
	if le >= 1 && le <= 6 {
		c.Rate = w.chaosRateCfg(o, fmt.Sprintf("DevilSquareMixRate%d", le))
	}
	// Level 7 reads CServerInfo+0x87c, which no key loads (0).
	c.Money = money[le]
	if !w.charmBonus(c, &c.Rate) {
		w.chaosSendResult(o, chaosmix.ResultTooManyCharms, nil)
		return
	}
	c.Rate = min(w.bonusRate(o, c.Rate), 100)
	if !w.chaosCharge(o) {
		return
	}
	if chaosRoll(c.Rate) {
		w.chaosCreateItem(o, chaosmix.Item{Index: chaosmix.ItemDSInvite, Level: le, Durability: 1}, false)
		w.chaosLog("[DevilSquareMix][Success][%s][%s] - (ChaosSuccessRate: %d,ChaosMoney: %d)",
			o.P.Account.Login, o.P.Char.Name, c.Rate, c.Money)
		return
	}
	w.chaosFailClear(o, false)
	w.chaosLog("[DevilSquareMix][Failure][%s][%s] - (ChaosSuccessRate: %d,ChaosMoney: %d)",
		o.P.Account.Login, o.P.Char.Name, c.Rate, c.Money)
}

// plusRateKeys are the rate tables of the +10..+15 mixes: CServerInfo
// 0x88c / 0x8ec + (AL + p*4)*4. Only p 0..3 have keys; +14/+15 read
// offsets no key loads (0).
func (w *World) plusRate(o *Object, p int, excOrSet bool) int {
	if p > 3 {
		return 0
	}
	key := "PlusCommonItemLevelMixRate"
	if excOrSet {
		key = "PlusExcSetItemLevelMixRate"
	}
	return w.chaosRateCfg(o, fmt.Sprintf("%s%d", key, p+1))
}

// mixPlusItem is FUN_0042b790 (86 03/04/16/17/31/32): item +(10+p).
func (w *World) mixPlusItem(o *Object, p int) {
	c := chaos(o)
	chaosN, bless, soul, count, slot := 0, 0, 0, 0, 0
	for i, it := range c.Box {
		switch {
		case it.IsEmpty():
		case it.Index == chaosmix.ItemChaos:
			chaosN++
		case it.Index == chaosmix.ItemBless:
			bless++
		case it.Index == chaosmix.ItemSoul:
			soul++
		case it.Index != chaosmix.ItemNoUpgrade1 && it.Index != chaosmix.ItemNoUpgrade2 && it.Level == p+9:
			count++
			slot = i
		}
	}
	if chaosN != 1 || soul < p+1 || bless < p+1 || count != 1 {
		w.chaosSendResult(o, chaosmix.ResultWrongItems, nil)
		return
	}
	it := c.Box[slot]
	c.Rate = w.plusRate(o, p, isExcellent(it) || isSet(it))
	if it.Luck {
		c.Rate += w.cfg.ALInt("Common", "AddLuckSuccessRate2", accountLevel(o), 0)
	}
	if !w.charmBonus(c, &c.Rate) {
		w.chaosSendResult(o, chaosmix.ResultTooManyCharms, nil)
		return
	}
	c.Rate = min(w.bonusRate(o, c.Rate), 100)
	c.Money = int64(p+1) * 2000000
	if !w.chaosCharge(o) {
		return
	}
	if chaosRoll(c.Rate) {
		oldMax := float32(w.itemMaxDurability(it))
		up := it
		up.Level++
		newMax := float32(w.itemMaxDurability(up))
		if oldMax > 0 {
			up.Durability = it.Durability * newMax / oldMax
		}
		w.chaosClear(o)
		c.Box[0] = up
		w.chaosSendResult(o, chaosmix.ResultSuccess, &up)
		w.chaosAnnounce(o, p, up) // FUN_005120b0 (chaos_announce.go)
		w.chaosLog("[PlusItemLevelMix][Success][%s][%s] - (Type: %d,ChaosSuccessRate: %d,ChaosMoney: %d)",
			o.P.Account.Login, o.P.Char.Name, p, c.Rate, c.Money)
		return
	}
	mode := w.chaosRateCfg(o, "FailMixMode")
	switch mode {
	case 0:
		w.chaosClear(o)
	case 1:
		// FUN_0042aa50: keep the item at level 0 (durability rescaled).
		kept := it
		kept.Level = 0
		if m := float32(w.itemMaxDurability(it)); m > 0 {
			kept.Durability = it.Durability / m * float32(w.itemMaxDurability(kept))
		}
		w.chaosClear(o)
		c.Box[slot] = kept
	case 2:
		// FUN_0042abd0: keep the item untouched.
		w.chaosClear(o)
		c.Box[slot] = it
	}
	w.chaosSendList(o, false)
	w.chaosSendResult(o, chaosmix.ResultFail, nil)
	w.chaosLog("[PlusItemLevelMix][Fail] [%s][%s] - FailMode: %d", o.P.Account.Login, o.P.Char.Name, mode)
}

// mixDinorant is FUN_0042bde0 (86 05).
func (w *World) mixDinorant(o *Object) {
	c := chaos(o)
	chaosN, uniria := 0, 0
	for _, it := range c.Box {
		switch {
		case it.IsEmpty():
		case it.Index == chaosmix.ItemChaos:
			chaosN++
		case it.Index == chaosmix.ItemUniria && it.Durability == 255:
			uniria++
		}
	}
	if chaosN != 1 || uniria != 10 {
		w.chaosSendResult(o, chaosmix.ResultWrongItems, nil)
		return
	}
	c.Rate = w.chaosRateCfg(o, "DinorantMixRate")
	if !w.charmBonus(c, &c.Rate) {
		w.chaosSendResult(o, chaosmix.ResultTooManyCharms, nil)
		return
	}
	c.Rate = min(w.bonusRate(o, c.Rate), 100)
	c.Money = 500000
	if !w.chaosCharge(o) {
		return
	}
	if chaosRoll(c.Rate) {
		// ItemOptionRate section 4 row 3 gives the excellent count,
		// FUN_0049f0d0 the bits (at most 3 for the Dinorant).
		var exc byte
		if d := w.chaosData(); d != nil {
			if n, ok := d.Rates.Pick(4, 3, chaosRand); ok {
				exc = chaosMakeExcellent(d.Exc, chaosmix.ItemDinorant, n, 3)
			}
		}
		w.chaosCreateItem(o, chaosmix.Item{Index: chaosmix.ItemDinorant, Durability: 0xFF, Excellent: exc}, false)
		w.chaosLog("[DinorantMix][Success][%s][%s] - (ChaosSuccessRate: %d,ChaosMoney: %d)",
			o.P.Account.Login, o.P.Char.Name, c.Rate, c.Money)
		return
	}
	w.chaosFailClear(o, false)
	w.chaosLog("[DinorantMix][Failure][%s][%s] - (ChaosSuccessRate: %d,ChaosMoney: %d)",
		o.P.Account.Login, o.P.Char.Name, c.Rate, c.Money)
}

// mixFruit is FUN_0042c230 (86 06).
func (w *World) mixFruit(o *Object) {
	c := chaos(o)
	chaosN, creation := 0, 0
	for _, it := range c.Box {
		switch {
		case it.IsEmpty():
		case it.Index == chaosmix.ItemChaos:
			chaosN++
		case it.Index == chaosmix.ItemCreation:
			creation++
		}
	}
	if chaosN != 1 || creation != 1 {
		w.chaosSendResult(o, chaosmix.ResultWrongItems, nil)
		return
	}
	c.Rate = w.chaosRateCfg(o, "FruitMixRate")
	if !w.charmBonus(c, &c.Rate) {
		w.chaosSendResult(o, chaosmix.ResultTooManyCharms, nil)
		return
	}
	c.Rate = min(w.bonusRate(o, c.Rate), 100)
	c.Money = 3000000
	if !w.chaosCharge(o) {
		return
	}
	if chaosRoll(c.Rate) {
		w.chaosCreateItem(o, chaosmix.Item{Index: chaosmix.ItemFruit, Level: chaosRand() % 5}, false)
		w.chaosLog("[FruitMix][Success][%s][%s] - (ChaosSuccessRate: %d,ChaosMoney: %d)",
			o.P.Account.Login, o.P.Char.Name, c.Rate, c.Money)
		return
	}
	w.chaosFailClear(o, false)
	w.chaosLog("[FruitMix][Failure][%s][%s] - (ChaosSuccessRate: %d,ChaosMoney: %d)",
		o.P.Account.Login, o.P.Char.Name, c.Rate, c.Money)
}

// mixWing2 is FUN_0042c620 (86 07 second wing, 86 18 cape).
func (w *World) mixWing2(o *Object, cape int) {
	c := chaos(o)
	chaosN, feather, crest, wings := 0, 0, 0, 0
	wingValue, itemValue, choice := int64(0), int64(0), 0
	for _, it := range c.Box {
		switch {
		case it.IsEmpty():
		case it.Index == chaosmix.ItemChaos:
			chaosN++
		case it.Index == chaosmix.ItemFeather && it.Level == 0:
			feather++
		case it.Index == chaosmix.ItemFeather && it.Level == 1:
			crest++
		case chaosmix.IsFirstWing(it.Index):
			wings++
			wingValue += w.itemPrice(it).Buy
		case isExcellent(it) && it.Level >= 4:
			itemValue += w.itemPrice(it).Buy
		case it.Index > 0x1A57 && it.Index < 0x1A5D:
			choice = it.Index - 0x1A57
		}
	}
	need := feather
	if cape == 1 {
		need = crest
	}
	if chaosN != 1 || need != 1 || wings != 1 {
		w.chaosSendResult(o, chaosmix.ResultWrongItems, nil)
		return
	}
	cfg := w.chaosRateCfg(o, "Wing2MixRate")
	c.Rate = cfg
	if cfg == -1 {
		c.Rate = int(itemValue/40000 + wingValue/4000000)
	}
	if !w.charmBonus(c, &c.Rate) {
		w.chaosSendResult(o, chaosmix.ResultTooManyCharms, nil)
		return
	}
	c.Rate = w.bonusRate(o, c.Rate)
	if cfg == -1 {
		c.Rate = min(c.Rate, 90)
	} else {
		c.Rate = min(c.Rate, 100)
	}
	c.Money = 5000000
	if !w.chaosCharge(o) {
		return
	}
	if chaosRoll(c.Rate) {
		res, ok := w.chaosBagItem(o, 0, 2+cape)
		if !ok {
			w.chaosSendResult(o, chaosmix.ResultFail, nil)
			return
		}
		if cape == 0 && choice > 0 {
			res.Index = chaosmix.Wing2Choice[choice]
		}
		w.chaosCreateItem(o, res, false)
		w.chaosLog("[Wing2Mix][Success][%s][%s] - (Type: %d,ChaosSuccessRate: %d,ChaosMoney: %d)",
			o.P.Account.Login, o.P.Char.Name, cape, c.Rate, c.Money)
		return
	}
	w.chaosFailClear(o, false)
	w.chaosLog("[Wing2Mix][Failure][%s][%s] - (Type: %d,ChaosSuccessRate: %d,ChaosMoney: %d)",
		o.P.Account.Login, o.P.Char.Name, cape, c.Rate, c.Money)
}

// mixBloodCastle is FUN_0042cc50 (86 08): Invisibility Cloak.
func (w *World) mixBloodCastle(o *Object) {
	c := chaos(o)
	chaosN, ns, nb, ls, lb := pairedLevels(c, chaosmix.ItemArchangel, chaosmix.ItemBloodBone)
	if chaosN != 1 || ns != 1 || nb != 1 || ls != lb {
		w.chaosSendResult(o, chaosmix.ResultWrongItems, nil)
		return
	}
	if ls > 8 || lb > 8 {
		w.chaosSendResult(o, chaosmix.ResultWrongLevel, nil)
		return
	}
	money := [9]int64{0, 50000, 80000, 150000, 250000, 400000, 600000, 850000, 1050000}
	if ls >= 1 && ls <= 7 {
		c.Rate = w.chaosRateCfg(o, fmt.Sprintf("BloodCastleMixRate%d", ls))
	}
	// Level 8 reads CServerInfo+0xa4c, which no key loads (0).
	c.Money = money[ls]
	if !w.charmBonus(c, &c.Rate) {
		w.chaosSendResult(o, chaosmix.ResultTooManyCharms, nil)
		return
	}
	c.Rate = min(w.bonusRate(o, c.Rate), 100)
	if !w.chaosCharge(o) {
		return
	}
	// FUN_0042cc50 fails only when rate < rand%100 (rand%100 == rate wins).
	if c.Rate < chaosRand()%100 {
		w.chaosFailClear(o, false)
		w.chaosLog("[BloodCastleMix][Failure][%s][%s] - (ChaosSuccessRate: %d,ChaosMoney: %d)",
			o.P.Account.Login, o.P.Char.Name, c.Rate, c.Money)
		return
	}
	w.chaosCreateItem(o, chaosmix.Item{Index: chaosmix.ItemCloak, Level: ls, Durability: 1}, false)
	w.chaosLog("[BloodCastleMix][Success][%s][%s] - (ChaosSuccessRate: %d,ChaosMoney: %d)",
		o.P.Account.Login, o.P.Char.Name, c.Rate, c.Money)
}

// mixWing1 is FUN_0042d170 (86 0B).
func (w *World) mixWing1(o *Object) {
	c := chaos(o)
	chaosN, weapons, choice := 0, 0, 0
	value := int64(0)
	for _, it := range c.Box {
		switch {
		case it.IsEmpty():
		case it.Index == chaosmix.ItemChaos:
			chaosN++
			value += w.jewelValue(it)
		case it.Index == chaosmix.ItemBless || it.Index == chaosmix.ItemSoul:
			value += w.jewelValue(it)
		case chaosmix.IsChaosWeapon(it.Index) && it.Level > 3 && it.Option != 0:
			weapons++
			value += w.itemPrice(it).Buy
		case it.Level >= 4 && it.Option != 0:
			value += w.itemPrice(it).Buy
		case it.Index > 0x1A52 && it.Index < 0x1A57:
			choice = it.Index - 0x1A52
		}
	}
	if chaosN == 0 || weapons == 0 {
		w.chaosSendResult(o, chaosmix.ResultWrongItems, nil)
		return
	}
	cfg := w.chaosRateCfg(o, "Wing1MixRate")
	c.Rate = cfg
	if cfg == -1 {
		c.Rate = int(value / 20000)
	}
	if !w.charmBonus(c, &c.Rate) {
		w.chaosSendResult(o, chaosmix.ResultTooManyCharms, nil)
		return
	}
	c.Rate = min(w.bonusRate(o, c.Rate), 100)
	c.Money = int64(c.Rate) * 10000
	if !w.chaosCharge(o) {
		return
	}
	if chaosRoll(c.Rate) {
		res, ok := w.chaosBagItem(o, 0, 1)
		if !ok {
			w.chaosSendResult(o, chaosmix.ResultFail, nil)
			return
		}
		if choice > 0 {
			res.Index = chaosmix.Wing1Choice[choice]
		}
		w.chaosCreateItem(o, res, false)
		w.chaosLog("[Wing1Mix][Success][%s][%s] - (ChaosSuccessRate: %d,ChaosMoney: %d)",
			o.P.Account.Login, o.P.Char.Name, c.Rate, c.Money)
		return
	}
	w.chaosFailClear(o, false)
	w.chaosLog("[Wing1Mix][Failure][%s][%s] - (ChaosSuccessRate: %d,ChaosMoney: %d)",
		o.P.Account.Login, o.P.Char.Name, c.Rate, c.Money)
}

// mixPet is FUN_0042d680 (86 0D Dark Horse, 86 0E Dark Raven).
func (w *World) mixPet(o *Object, t int) {
	c := chaos(o)
	chaosN, spirit, bless, soul, creation := 0, 0, 0, 0, 0
	for _, it := range c.Box {
		switch {
		case it.IsEmpty():
		case it.Index == chaosmix.ItemChaos:
			chaosN++
		case it.Index == chaosmix.ItemSpirit && it.Level == t:
			spirit++
		case it.Index == chaosmix.ItemSpirit && (it.Level == 0 || it.Level == 1):
		case it.Index == chaosmix.ItemBless:
			bless++
		case it.Index == chaosmix.ItemSoul:
			soul++
		case it.Index == chaosmix.ItemCreation:
			creation++
		}
	}
	need := 5 - 3*t
	if chaosN != 1 || spirit != 1 || bless != need || soul != need || creation != 1 {
		w.chaosSendResult(o, chaosmix.ResultWrongItems, nil)
		return
	}
	c.Rate = w.chaosRateCfg(o, "PetMixRate")
	if !w.charmBonus(c, &c.Rate) {
		w.chaosSendResult(o, chaosmix.ResultTooManyCharms, nil)
		return
	}
	c.Rate = min(w.bonusRate(o, c.Rate), 100)
	c.Money = 5000000 - int64(t)*4000000
	if !w.chaosCharge(o) {
		return
	}
	if chaosRoll(c.Rate) {
		w.chaosCreateItem(o, chaosmix.Item{Index: chaosmix.ItemDarkHorse + t}, false)
		w.chaosLog("[PetMix][Success][%s][%s] - (Type: %d,ChaosSuccessRate: %d,ChaosMoney: %d)",
			o.P.Account.Login, o.P.Char.Name, t, c.Rate, c.Money)
		return
	}
	// The failure list goes to interface 13 only (FUN_0042fd60(1)).
	w.chaosFailClear(o, true)
	w.chaosLog("[PetMix][Failure][%s][%s] - (Type: %d,ChaosSuccessRate: %d,ChaosMoney: %d)",
		o.P.Account.Login, o.P.Char.Name, t, c.Rate, c.Money)
}

// mixSiegePotion is FUN_0042d9a0 (86 0F Bless potion, 86 10 Soul potion):
// 1..25 jewels, always succeeds, 10 potions per jewel.
func (w *World) mixSiegePotion(o *Object, t int) {
	c := chaos(o)
	bless, soul := 0, 0
	for _, it := range c.Box {
		switch it.Index {
		case chaosmix.ItemBless:
			bless++
		case chaosmix.ItemSoul:
			soul++
		}
	}
	n := bless
	if t == 1 {
		n = soul
	}
	if n == 0 || n > 25 {
		w.chaosSendResult(o, chaosmix.ResultWrongItems, nil)
		return
	}
	c.Money = int64(2-t) * 50000
	if !w.chaosCharge(o) {
		return
	}
	w.chaosCreateItem(o, chaosmix.Item{Index: chaosmix.ItemSiegePot, Level: t, Durability: byte((bless + soul) * 10)}, false)
	w.chaosLog("[SiegePotionMix][Success][%s][%s] - (Type: %d,ChaosSuccessRate: %d,ChaosMoney: %d)",
		o.P.Account.Login, o.P.Char.Name, t, c.Rate, c.Money)
}

// mixLifeStone is FUN_0042dcf0 (86 11): always succeeds.
func (w *World) mixLifeStone(o *Object) {
	c := chaos(o)
	chaosN, bless, soul, guardian := 0, 0, 0, 0
	for _, it := range c.Box {
		switch it.Index {
		case chaosmix.ItemChaos:
			chaosN++
		case chaosmix.ItemBless:
			bless++
		case chaosmix.ItemSoul:
			soul++
		case chaosmix.ItemGuardian:
			guardian++
		}
	}
	if chaosN != 1 || bless != 5 || soul != 5 || guardian != 1 {
		w.chaosSendResult(o, chaosmix.ResultWrongItems, nil)
		return
	}
	c.Money = 5000000
	if !w.chaosCharge(o) {
		return
	}
	w.chaosCreateItem(o, chaosmix.Item{Index: chaosmix.ItemLifeStone, Level: 1}, false)
	w.chaosLog("[LifeStoneMix][Success][%s][%s] - (ChaosSuccessRate: %d,ChaosMoney: %d)",
		o.P.Account.Login, o.P.Char.Name, c.Rate, c.Money)
}

// mixSenior is FUN_0042dff0 (86 12): only the master of the guild that owns
// the castle, once a day. No castle has an owner here, so the original
// branch for everyone else applies: notice 0x2E7 and result 0.
func (w *World) mixSenior(o *Object) {
	w.systemMessage(o, w.messageFor(o, 0x2e7))
	w.chaosSendResult(o, chaosmix.ResultFail, nil)
}

// mixChaosCard is FUN_0042e4c0 (86 28): one chaos card of the last kind
// seen; the item comes from the bag with SpecialValue 7, level kind-1.
func (w *World) mixChaosCard(o *Object) {
	c := chaos(o)
	kind := 0
	var count [5]int
	for _, it := range c.Box {
		switch it.Index {
		case 0x1C36:
			kind = 1
		case 0x1C5C:
			kind = 2
		case 0x1C5D:
			kind = 3
		case 0x1C5F:
			kind = 4
		default:
			continue
		}
		count[kind]++
	}
	if kind == 0 || count[kind] != 1 {
		w.chaosSendResult(o, chaosmix.ResultWrongItems, nil)
		return
	}
	res, ok := w.chaosBagItem(o, 7, kind-1)
	if !ok {
		w.chaosSendResult(o, chaosmix.ResultFail, nil)
		return
	}
	w.chaosCreateItem(o, res, false)
	w.chaosLog("[ChaosCardMix][Success][%s][%s] - (Type: %d,ChaosSuccessRate: %d,ChaosMoney: %d)",
		o.P.Account.Login, o.P.Char.Name, kind, c.Rate, c.Money)
}

// mixCherryBlossom is FUN_0042e900 (86 29): 10 / 30 / 255 blossoms of the
// last kind seen (stack sizes summed); bag SpecialValue 9, level kind-1.
func (w *World) mixCherryBlossom(o *Object) {
	c := chaos(o)
	kind := 0
	var sum [4]int
	for _, it := range c.Box {
		switch it.Index {
		case 0x1C58:
			kind = 1
		case 0x1C59:
			kind = 2
		case 0x1C5A:
			kind = 3
		default:
			continue
		}
		sum[kind] += int(it.Durability)
	}
	need := [4]int{0, 10, 30, 255}
	if kind == 0 || sum[kind] != need[kind] {
		w.chaosSendResult(o, chaosmix.ResultWrongItems, nil)
		return
	}
	res, ok := w.chaosBagItem(o, 9, kind-1)
	if !ok {
		w.chaosSendResult(o, chaosmix.ResultFail, nil)
		return
	}
	w.chaosCreateItem(o, res, false)
	w.chaosLog("[CherryBlossomMix][Success][%s][%s] - (Type: %d,ChaosSuccessRate: %d,ChaosMoney: %d)",
		o.P.Account.Login, o.P.Char.Name, kind, c.Rate, c.Money)
}

// --- result items ------------------------------------------------------------------

// chaosCreateItem is FUN_004753e0 with map 0xFF followed by the answer
// FUN_004742b0: the serial comes from the store, then (if the chaos box is
// still open) the box is cleared, the item is put in slot 0 and 86 01 is
// sent. second is unused for map 0xFF (interface 7 only).
func (w *World) chaosCreateItem(o *Object, m chaosmix.Item, second bool) {
	cn := o.conn
	w.async(func(ctx context.Context) func() {
		serial, err := w.store.NextItemSerial(ctx)
		if err == nil && (m.Index == chaosmix.ItemDarkHorse || m.Index == chaosmix.ItemDarkRaven) {
			if ps, ok := w.store.(store.PetStore); ok {
				err = ps.CreatePetInfo(ctx, store.DefaultPetInfo(serial))
			}
		}
		return func() {
			o := w.objectOf(cn)
			if o == nil || o.State != StatePlaying || o.P == nil {
				return
			}
			if err != nil {
				w.log.Error("chaos item serial", "err", err)
				return
			}
			if st := items(o).If; !st.Use || st.Type != ifChaosBox {
				return
			}
			w.chaosPlaceResult(o, m, serial)
		}
	})
}

// chaosPlaceResult is the map 0xFF branch of FUN_004742b0.
func (w *World) chaosPlaceResult(o *Object, m chaosmix.Item, serial uint32) {
	it := item.Item{Index: m.Index, Level: m.Level, Serial: serial, Skill: m.Skill, Luck: m.Luck,
		Option: m.Option, Excellent: m.Excellent & 0x3F, SetOption: m.SetOption}
	if chaosmix.FixedDurability(m.Index) {
		it.Durability = float32(m.Durability)
	} else {
		info, _ := w.itemInfo(m.Index)
		it.Durability = float32(maxDurability(info, m.Index, m.Level, it.Excellent != 0, isSet(it)))
	}
	if m.Index == chaosmix.ItemDarkHorse || m.Index == chaosmix.ItemDarkRaven {
		petInit(&it, 1, 0, w.maxPetLevel())
	}
	w.chaosClear(o)
	chaos(o).Box[0] = it
	w.chaosSendResult(o, chaosmix.ResultSuccess, &it)
}

// chaosBagItem rolls the item of the bag with (SpecialValue, SpecialLevel)
// (FUN_004912f0 + FUN_0048f980) for the player.
func (w *World) chaosBagItem(o *Object, value, level int) (chaosmix.Item, bool) {
	d := w.chaosData()
	if d == nil {
		return chaosmix.Item{}, false
	}
	var bag *gamedata.ItemBag
	for _, e := range d.Bags {
		if e.SpecialValue == value && (e.SpecialLevel == -1 || e.SpecialLevel == level) && e.SpecialState == -1 {
			bag = e.Bag
			break
		}
	}
	if bag == nil {
		return chaosmix.Item{}, false
	}
	return chaosRollBag(d, bag, o.Class, int(o.P.Char.Class&0x0F), chaosRand)
}

// chaosRollBag is FUN_0048f980:
//  1. section 3 rows in BagIndex order: the first with rnd()%10000 <
//     DropRate is used (none: false);
//  2. its section-4 rows allowed for the class (value != 0 and <=
//     ChangeUp+1) with SectionRate != 0 are weighted (rnd() % sum);
//  3. an item of that section is taken with rnd() % count;
//  4. Option0..5 are ItemOptionRate rows of sections 0..5 (a missing row
//     keeps the item level and 0 for the rest); the excellent bits come
//     from FUN_0049f0d0 mode 1 and the ancient byte from FUN_0049f270.
//     Without excellent bits the GradeCount is passed as the "new option"
//     of CItem::Convert.
func chaosRollBag(d *chaosmix.Data, bag *gamedata.ItemBag, class, changeUp int, rnd func() int) (chaosmix.Item, bool) {
	var row *gamedata.BagRow
	for i := range bag.Bags {
		if rnd()%10000 < bag.Bags[i].DropRate {
			row = &bag.Bags[i]
			break
		}
	}
	if row == nil || class < 0 || class >= 5 {
		return chaosmix.Item{}, false
	}
	var drops []gamedata.BagDrop
	total := 0
	for _, dr := range row.Drops {
		v := dr.Class[class]
		if v == 0 || v > changeUp+1 || dr.SectionRate == 0 {
			continue
		}
		drops = append(drops, dr)
		total += dr.SectionRate
	}
	if total == 0 {
		return chaosmix.Item{}, false
	}
	x := rnd() % total
	var sec *gamedata.BagDrop
	start := 0
	for i := range drops {
		if x >= start && x < start+drops[i].SectionRate {
			sec = &drops[i]
			break
		}
		start += drops[i].SectionRate
	}
	if sec == nil {
		return chaosmix.Item{}, false
	}
	list := bag.Sections[sec.Section]
	if len(list) == 0 {
		return chaosmix.Item{}, false
	}
	bi := list[uint(rnd())%uint(len(list))]
	if bi.Index < 0 {
		return chaosmix.Item{}, false
	}
	m := chaosmix.Item{Index: bi.Index, Level: bi.Level}
	pick := func(s int) (int, bool) { return d.Rates.Pick(s, bi.Option[s], rnd) }
	if v, ok := pick(0); ok {
		m.Level = v
	}
	if v, ok := pick(1); ok {
		m.Skill = v != 0
	}
	if v, ok := pick(2); ok {
		m.Luck = v != 0
	}
	if v, ok := pick(3); ok {
		m.Option = v
	}
	excN, setN := 0, 0
	if v, ok := pick(4); ok {
		excN = v
	}
	if v, ok := pick(5); ok {
		setN = v
	}
	maxExc := 6
	if bi.Index == chaosmix.ItemDinorant || bi.Index == chaosmix.ItemFenrir {
		maxExc = 3
	}
	m.Excellent = chaosMakeExcellent(d.Exc, bi.Index, excN, maxExc)
	m.SetOption = chaosmix.MakeSet(d.Sets, bi.Index, setN, rnd)
	if m.Excellent == 0 {
		m.Excellent = byte(bi.Grade)
	}
	return m, true
}

// chaosMakeExcellent is FUN_0049f0d0 mode 1: count distinct excellent
// bits (at most maxOptions). The original loops until it has them; the
// loop is bounded so an item with fewer candidate bits cannot hang.
func chaosMakeExcellent(rows []gamedata.ExcellentOptionRate, index, count, maxOptions int) byte {
	var exc byte
	added := 0
	for tries := 0; added < count && added < maxOptions && tries < 64; tries++ {
		bit := gamedata.PickExcellentOption(rows, index, chaosRand)
		if bit == 0 {
			break
		}
		if exc&byte(bit) == 0 {
			exc |= byte(bit)
			added++
		}
	}
	return exc
}

// --- rate preview (88) --------------------------------------------------------------

// handleChaosRate is 88 [4..7] mix type (FUN_0042ee40): answers C1 10 88
// with the rate, the zen and the castle tax of the mix (nothing when the
// items do not match). Types without a preview send zeros.
func (w *World) handleChaosRate(o *Object, p []byte) {
	if len(p) < 8 || !playerReady(o) || o.P.Moving {
		return
	}
	st := items(o).If
	if !st.Use || (st.Type != ifChaosBox && st.Type != ifChaosBox2) {
		return
	}
	c := chaos(o)
	typ := binary.LittleEndian.Uint32(p[4:])
	rate, money, ok := w.chaosPreview(o, c, typ)
	if !ok {
		return
	}
	tax := money * w.castleTax(o) / 100
	if rate > 100 {
		rate = 100
	}
	rate = max(rate, 0)
	w.send(o, chaosmix.PacketRate(uint32(rate), uint32(money), uint32(tax)))
}

// chaosPreview reproduces the per-type branches of FUN_0042ee40; ok is
// false when the box does not hold the recipe (no answer).
func (w *World) chaosPreview(o *Object, c *chaosState, typ uint32) (int, int64, bool) {
	tmp := *c
	charms := func(r int) (int, bool) {
		if !w.charmBonus(&tmp, &r) {
			return 0, false
		}
		return w.bonusRate(o, r), true
	}
	switch typ {
	case 1, 0xB:
		chaosN, n, value := 0, 0, int64(0)
		for _, it := range c.Box {
			switch {
			case it.IsEmpty():
			case it.Index == chaosmix.ItemChaos:
				chaosN++
				value += w.jewelValue(it)
			case it.Index == chaosmix.ItemBless || it.Index == chaosmix.ItemSoul:
				value += w.jewelValue(it)
			case typ == 0xB && chaosmix.IsChaosWeapon(it.Index):
				if it.Level > 3 && it.Option != 0 {
					n++
					value += w.itemPrice(it).Buy
				}
			case it.Level > 3 && it.Option != 0:
				if typ == 1 {
					n++
				}
				value += w.itemPrice(it).Buy
			}
		}
		if chaosN == 0 || n == 0 {
			return 0, 0, false
		}
		key := "ChaosItemMixRate"
		if typ == 0xB {
			key = "Wing1MixRate"
		}
		r := w.chaosRateCfg(o, key)
		if r == -1 {
			r = int(value / 20000)
		}
		r, ok := charms(r)
		if !ok {
			return 0, 0, false
		}
		r = min(r, 100)
		return r, int64(r) * 10000, true
	case 2, 8:
		a, b, maxL := chaosmix.ItemDSEye, chaosmix.ItemDSKey, 7
		if typ == 8 {
			a, b, maxL = chaosmix.ItemArchangel, chaosmix.ItemBloodBone, 8
		}
		chaosN, na, nb, la, lb := pairedLevels(c, a, b)
		if chaosN != 1 || na != 1 || nb != 1 || la != lb || la > maxL {
			return 0, 0, false
		}
		var r int
		var money int64
		if typ == 2 {
			money = [8]int64{0, 100000, 200000, 400000, 700000, 1100000, 1600000, 2000000}[la]
			if la >= 1 && la <= 6 {
				r = w.chaosRateCfg(o, fmt.Sprintf("DevilSquareMixRate%d", la))
			}
		} else {
			money = [9]int64{0, 50000, 80000, 150000, 250000, 400000, 600000, 850000, 1050000}[la]
			if la >= 1 && la <= 7 {
				r = w.chaosRateCfg(o, fmt.Sprintf("BloodCastleMixRate%d", la))
			}
		}
		r, ok := charms(r)
		return r, money, ok
	case 3, 4, 0x16, 0x17:
		pv := map[uint32]int{3: 0, 4: 1, 0x16: 2, 0x17: 3}[typ]
		chaosN, bless, soul, n, slot := 0, 0, 0, 0, 0
		for i, it := range c.Box {
			switch {
			case it.IsEmpty():
			case it.Index == chaosmix.ItemChaos:
				chaosN++
			case it.Index == chaosmix.ItemBless:
				bless++
			case it.Index == chaosmix.ItemSoul:
				soul++
			case it.Level == pv+9:
				n++
				slot = i
			}
		}
		if chaosN != 1 || soul < pv+1 || bless < pv+1 || n != 1 {
			return 0, 0, false
		}
		it := c.Box[slot]
		r := w.plusRate(o, pv, isExcellent(it) || isSet(it))
		if it.Luck {
			r += w.cfg.ALInt("Common", "AddLuckSuccessRate2", accountLevel(o), 0)
		}
		r, ok := charms(r)
		return r, int64(pv+1) * 2000000, ok
	case 5:
		chaosN, uniria := 0, 0
		for _, it := range c.Box {
			if it.Index == chaosmix.ItemChaos {
				chaosN++
			} else if it.Index == chaosmix.ItemUniria && it.Durability == 255 {
				uniria++
			}
		}
		if chaosN != 1 || uniria != 10 {
			return 0, 0, false
		}
		r, ok := charms(w.chaosRateCfg(o, "DinorantMixRate"))
		return r, 500000, ok
	case 6:
		chaosN, creation := 0, 0
		for _, it := range c.Box {
			if it.Index == chaosmix.ItemChaos {
				chaosN++
			} else if it.Index == chaosmix.ItemCreation {
				creation++
			}
		}
		if chaosN != 1 || creation != 1 {
			return 0, 0, false
		}
		r, ok := charms(w.chaosRateCfg(o, "FruitMixRate"))
		return r, 3000000, ok
	case 7, 0x18:
		cape := 0
		if typ == 0x18 {
			cape = 1
		}
		chaosN, feather, crest, wings := 0, 0, 0, 0
		wingValue, itemValue := int64(0), int64(0)
		for _, it := range c.Box {
			switch {
			case it.IsEmpty():
			case it.Index == chaosmix.ItemChaos:
				chaosN++
			case it.Index == chaosmix.ItemFeather && it.Level == 0:
				feather++
			case it.Index == chaosmix.ItemFeather && it.Level == 1:
				crest++
			case chaosmix.IsFirstWing(it.Index):
				wings++
				wingValue += w.itemPrice(it).Buy
			case isExcellent(it) && it.Level > 3:
				itemValue += w.itemPrice(it).Buy
			}
		}
		need := feather
		if cape == 1 {
			need = crest
		}
		if chaosN != 1 || need != 1 || wings != 1 {
			return 0, 0, false
		}
		cfg := w.chaosRateCfg(o, "Wing2MixRate")
		r := cfg
		if cfg == -1 {
			r = int(itemValue/40000 + wingValue/4000000)
		}
		r, ok := charms(r)
		if cfg == -1 && r > 90 {
			r = 90
		}
		return r, 5000000, ok
	case 0xD, 0xE:
		t := int(typ - 0xD)
		chaosN, spirit, bless, soul, creation := 0, 0, 0, 0, 0
		for _, it := range c.Box {
			switch {
			case it.IsEmpty():
			case it.Index == chaosmix.ItemChaos:
				chaosN++
			case it.Index == chaosmix.ItemSpirit && it.Level == t:
				spirit++
			case it.Index == chaosmix.ItemBless:
				bless++
			case it.Index == chaosmix.ItemSoul:
				soul++
			case it.Index == chaosmix.ItemCreation:
				creation++
			}
		}
		need := 5 - 3*t
		if chaosN != 1 || spirit != 1 || bless != need || soul != need || creation != 1 {
			return 0, 0, false
		}
		r, ok := charms(w.chaosRateCfg(o, "PetMixRate"))
		return r, 5000000 - int64(t)*4000000, ok
	}
	return 0, 0, true
}
