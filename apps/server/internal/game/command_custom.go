package game

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/store"
)

// Custom commands 54..62 of FUN_00435400: /gift (CCustomGift),
// /marry /accept /divorce /trackmarry (CCustomMarriage), /buyvip
// (CCustomBuyVip) and /pack /unpack (CCustomJewelPack). The offline modes
// are in offline.go, the custom shop in customstore.go, the auto attack in
// customattack.go, the auto pick in custompick.go and the hard-coded
// commands in command_hardcoded.go.

func init() {
	commandHandlers[54] = (*World).commandGift
	commandHandlers[55] = (*World).commandMarry
	commandHandlers[56] = (*World).commandMarryAccept
	commandHandlers[57] = (*World).commandDivorce
	commandHandlers[58] = (*World).commandTrackMarry
	commandHandlers[60] = (*World).commandBuyVip
	commandHandlers[61] = func(w *World, o *Object, c command, arg string) { w.commandJewelPack(o, c, arg, true) }
	commandHandlers[62] = func(w *World, o *Object, c command, arg string) { w.commandJewelPack(o, c, arg, false) }
	for i := 50; i <= 65; i++ {
		delete(pendingCommands, i)
	}
	delete(pendingCommands, 21)
	startHooks = append(startHooks, loadCustomWorld)
}

// customState holds the per-player fields of the commands 21 and 50..65
// and of the hard-coded commands.
type customState struct {
	Offline offlineState // obj+0x2100..0x2110 (offline.go)
	Attack  attackState  // obj+0x124, 0x20d8..0x20f4 (customattack.go)
	Store   storeState   // obj+0x249c..0x249f (customstore.go)
	Pick    pickState    // obj+0x22bc..0x2464, 0x24a0 (custompick.go)
	Marry   marryRequest // obj+0xd39..0xd40
	Ad      adState      // obj+0x125..0x130 (/vendo, /compro)
	Combo   comboState   // obj+0xcc..0xdc (/combo)
	InfoAt  time.Time    // FUN_00405cd0 tick of /minhainfo
}

func custom(o *Object) *customState {
	if o.P.cust == nil {
		o.P.cust = &customState{}
	}
	return o.P.cust
}

// customWorld holds the data files of the commands.
type customWorld struct {
	attack    *gamedata.CustomAttack
	gifts     []gamedata.Gift
	marriage  *gamedata.CustomMarriage
	vip       map[string]gamedata.VipPlan
	pickBlock []gamedata.PickBlock
	superDrop bool // DAT_00772d7c (/superdrop)
}

func (w *World) cw() *customWorld {
	if w.custom == nil {
		w.custom = &customWorld{attack: &gamedata.CustomAttack{Skills: map[int]gamedata.AttackSkill{}},
			marriage: &gamedata.CustomMarriage{}, vip: map[string]gamedata.VipPlan{}}
	}
	return w.custom
}

func loadCustomWorld(w *World) error {
	c := w.cw()
	if w.dataDir == "" {
		return nil
	}
	var err error
	if c.attack, err = gamedata.LoadCustomAttack(w.dataDir); err != nil {
		return err
	}
	if c.gifts, err = gamedata.LoadCustomGift(w.dataDir); err != nil {
		return err
	}
	if c.marriage, err = gamedata.LoadCustomMarriage(w.dataDir); err != nil {
		return err
	}
	if c.vip, err = gamedata.LoadCustomBuyVip(w.dataDir); err != nil {
		return err
	}
	c.pickBlock, err = gamedata.LoadCustomPick(w.dataDir)
	return err
}

// customMsg sends Message.txt n as a blue notice (FUN_004c08f0 type 1).
func (w *World) customMsg(o *Object, n int, args ...any) {
	w.systemMessage(o, w.messageFor(o, n, args...))
}

// customNoticeMsg is FUN_004dc920: the C1 86 F3 E4 notice with the text
// of Message.txt n.
func (w *World) customNoticeMsg(o *Object, typ, color byte, n int, args ...any) {
	w.send(o, packetCustomNotice(typ, color, w.messageFor(o, n, args...)))
}

// customStore returns the CustomCommandStore, nil when the store does not
// implement it.
func (w *World) customStore() store.CustomCommandStore {
	cs, _ := w.store.(store.CustomCommandStore)
	return cs
}

// mapZone reads a "<key> = -1 | m1,m2,..." map list (CustomStoreMapZone,
// CustomPickMapZone, CustomAttackMapZone): -1 allows every map, otherwise
// only the listed maps that exist in MapManager.txt.
func (w *World) mapZone(key string, mp int) bool {
	if w.cfg.Int("Custom", key, 0) == -1 {
		return true
	}
	for _, tok := range strings.Split(w.cfg.String("Custom", key, 0x100), ",") {
		n, err := strconv.Atoi(strings.TrimSpace(tok))
		if err != nil || n != mp {
			continue
		}
		if _, ok := w.data.Maps[n]; ok {
			return true
		}
	}
	return false
}

// --- /gift (54) ----------------------------------------------------------------------

// commandGift is /gift code (FUN_00463780): CustomGiftSwitch, then the
// first CustomGift.txt row with that code whose Class (-1 any) and
// Switch_AL match. Level and reset limits give 0x25E..0x261; a missing
// code 0x25D. The DataServer counts the gift (DS 0x0F 03, GiftData) and
// the reply (FUN_00463a10) gives it. No delay or charge.
func (w *World) commandGift(o *Object, c command, arg string) {
	if w.cfg.Int("Custom", "CustomGiftSwitch", 0) == 0 {
		return
	}
	code := commandToken(arg, 0, 0x20)
	ch := o.P.Char
	al := accountLevel(o)
	for i := range w.cw().gifts {
		g := &w.cw().gifts[i]
		if g.Code != code || (g.Class != -1 && g.Class != o.Class) || g.Switch[al] == 0 {
			continue
		}
		switch {
		case g.MinLevel != -1 && ch.Level < g.MinLevel:
			w.customMsg(o, 0x25E)
		case g.MaxLevel != -1 && g.MaxLevel < ch.Level:
			w.customMsg(o, 0x25F)
		case g.MinReset != -1 && ch.Resets < g.MinReset:
			w.customMsg(o, 0x260)
		case g.MaxReset != -1 && g.MaxReset < ch.Resets:
			w.customMsg(o, 0x261)
		default:
			w.requestGift(o, *g)
		}
		return
	}
	w.customMsg(o, 0x25D)
}

// requestGift sends C1 18 0F 03 index name gift maxCount and handles the
// C1 16 0F 03 reply.
func (w *World) requestGift(o *Object, g gamedata.Gift) {
	cs := w.customStore()
	if cs == nil {
		return
	}
	name, conn := o.P.Char.Name, o.conn
	w.async(func(ctx context.Context) func() {
		ok, err := cs.TakeGift(ctx, name, g.Index, g.MaxCount)
		if err != nil {
			w.log.Error("gift", "name", name, "err", err)
			return nil
		}
		return func() {
			if o := w.objectOf(conn); o != nil && o.State == StatePlaying && o.P.Char != nil {
				w.giftReply(o, g, ok)
			}
		}
	})
}

// giftReply is FUN_00463a10: result 0 gives message 0x263; otherwise the
// level up points (F3 E0 and the save of FUN_00475c30), the zen (capped
// at 2,000,000,000 when it would pass MaxMoney), the items on the ground
// near the player (FUN_0050c860, 10 tries) with their duration, the
// C1 07 F3 40 00 x y firework to the player and the viewport and 0x264.
func (w *World) giftReply(o *Object, g gamedata.Gift, ok bool) {
	if !ok {
		w.customMsg(o, 0x263)
		return
	}
	ch := o.P.Char
	if g.LevelUpPoint > 0 {
		ch.LevelUpPoints += g.LevelUpPoint
		w.send(o, w.packetCharRefresh(o))
		w.saveCharacter(o, nil)
	}
	if g.Money != 0 {
		if moneyFits(ch.Money, int64(g.Money)) {
			ch.Money += int64(g.Money)
		} else {
			ch.Money = maxMoney
		}
		w.sendMoney(o)
	}
	for _, it := range g.Items {
		x, y, _ := w.randomNearCell(o.Map, o.X, o.Y, 2, 2, 10)
		w.createItem(o, o.Map, x, y, giftSpec(it))
	}
	fw := packetFirework(0, byte(o.X), byte(o.Y))
	w.send(o, fw)
	w.broadcast(o, fw)
	w.customMsg(o, 0x264)
	w.log.Info("[CommandGift]", "account", o.P.Account.Login, "name", ch.Name, "gift", g.Index, "code", g.Code)
}

func giftSpec(it gamedata.GiftItem) dropSpec {
	return dropSpec{Index: it.Index, Level: it.Level, Dur: it.Durability, Skill: it.Skill, Luck: it.Luck,
		Option: it.Option, Exc: byte(it.Excellent), Set: byte(it.SetOption), Duration: it.Duration}
}

// maxMoney is the zen cap of the custom commands (2,000,000,000).
const maxMoney = 2000000000

// --- marriage (55..58) ---------------------------------------------------------------

// marryRequest is obj+0xd39 (pending proposal), obj+0xd3c (proposer
// index) and obj+0xd40 (proposal tick).
type marryRequest struct {
	Pending bool
	From    int
	At      time.Time
}

// marryAcceptTime is the 60-second window of /accept (0xea61 ms).
const marryAcceptTime = 60 * time.Second

func femaleClass(class int) bool { return class == 2 || class == 5 }

// marriageArea is FUN_00466840: both characters on CustomMarriageMapZone
// inside the StartX..EndX / StartY..EndY rectangle.
func (w *World) marriageArea(a, b *Object) bool {
	mp := w.cfg.Int("Custom", "CustomMarriageMapZone", 0)
	sx, sy := w.cfg.Int("Custom", "CustomMarriageStartX", 0), w.cfg.Int("Custom", "CustomMarriageStartY", 0)
	ex, ey := w.cfg.Int("Custom", "CustomMarriageEndX", 0), w.cfg.Int("Custom", "CustomMarriageEndY", 0)
	in := func(o *Object) bool { return o.Map == mp && sx <= o.X && o.X <= ex && sy <= o.Y && o.Y <= ey }
	return in(a) && in(b)
}

// hasMarriageItem is FUN_004668c0 (section 0) / FUN_00466950 (section 1).
func (w *World) hasMarriageItem(o *Object, r gamedata.MarriageItem) bool {
	return r.Enable == 0 || w.countItems(o, r.Index, r.Level) >= r.Count
}

// commandMarry is /marry name (FUN_00465e70): messages 0x171 (not
// online), 0x28A (self), 0x28B (an interface open), 0x28C / 0x28D
// (already married), 0x28E (same gender without CustomMarriageHomoSexual;
// classes 2 and 5 are female), 0x28F (not both at the altar), 0x290
// (the section 0 item missing on either). The target gets the proposal
// (0x292 with the name) and the proposer 0x291; the command is charged.
func (w *World) commandMarry(o *Object, c command, arg string) {
	if w.cfg.Int("Custom", "CustomMarriageSwitch", 0) == 0 {
		return
	}
	t := w.findPlayer(commandToken(arg, 0, 0xB))
	m := w.cw().marriage
	switch {
	case t == nil:
		w.customMsg(o, 0x171)
	case t == o:
		w.customMsg(o, 0x28A)
	case ifBusy(o) || ifBusy(t):
		w.customMsg(o, 0x28B)
	case o.P.Char.Married:
		w.customMsg(o, 0x28C, o.P.Char.MarryCouple)
	case t.P.Char.Married:
		w.customMsg(o, 0x28D, t.P.Char.Name)
	case femaleClass(o.Class) == femaleClass(t.Class) && w.cfg.Int("Custom", "CustomMarriageHomoSexual", 0) == 0:
		w.customMsg(o, 0x28E)
	case !w.marriageArea(o, t):
		w.customMsg(o, 0x28F)
	case !w.hasMarriageItem(o, m.Marry) || !w.hasMarriageItem(t, m.Marry):
		w.customMsg(o, 0x290, m.Marry.Count, w.itemName(m.Marry.Index, m.Marry.Level))
	default:
		custom(t).Marry = marryRequest{Pending: true, From: o.Index, At: w.now()}
		w.customMsg(o, 0x291)
		w.customMsg(t, 0x292, o.P.Char.Name)
		w.log.Info("[CommandMarry] Request of marry", "account", o.P.Account.Login, "name", o.P.Char.Name,
			"target_account", t.P.Account.Login, "target", t.P.Char.Name)
		w.commandDone(o, c)
	}
}

// commandMarryAccept is /accept (FUN_00466170): only with a pending
// proposal; after 60 seconds it is dropped (0x293). The proposer must
// still be playing (0x294), both at the altar (0x28F) with the item
// (0x290). Each one becomes married to the other, loses the section 0
// item (with the section 1 count, as the original reads +0x44), gets the
// section 2 rewards in the inventory and is saved. Ten fireworks around
// the accepter, 0x295 (gold) to both, 0x296 to everyone with
// CustomMarriageAnnounce; the command is charged to the accepter.
func (w *World) commandMarryAccept(o *Object, c command, arg string) {
	if w.cfg.Int("Custom", "CustomMarriageSwitch", 0) == 0 {
		return
	}
	req := &custom(o).Marry
	if !req.Pending {
		return
	}
	if w.now().Sub(req.At) > marryAcceptTime {
		req.Pending = false
		w.customMsg(o, 0x293)
		return
	}
	if ifBusy(o) {
		w.customMsg(o, 0x28B)
		return
	}
	// The original reads the object table without checking the slot;
	// here a proposer that left gives 0x294.
	var t *Object
	if req.From >= PlayerStart && req.From < MaxObjects {
		t = w.objects[req.From]
	}
	if t == nil || t.State != StatePlaying || !t.IsPlayer() || t.P.Char == nil {
		w.customMsg(o, 0x294)
		return
	}
	m := w.cw().marriage
	if !w.marriageArea(o, t) {
		w.customMsg(o, 0x28F)
		return
	}
	if !w.hasMarriageItem(o, m.Marry) || !w.hasMarriageItem(t, m.Marry) {
		w.customMsg(o, 0x290, m.Marry.Count, w.itemName(m.Marry.Index, m.Marry.Level))
		return
	}
	wed := func(a, b *Object) {
		a.P.Char.Married, a.P.Char.MarryCouple = true, b.P.Char.Name
		custom(a).Marry = marryRequest{}
		if m.Marry.Enable != 0 {
			w.deleteItems(a, m.Marry.Index, m.Marry.Level, m.Divorce.Count)
		}
		w.marriageRewards(a)
		w.saveCharacter(a, nil)
	}
	wed(o, t)
	wed(t, o)
	for i := 0; i < 10; i++ {
		y := o.Y - 4 + (monsterRand()%5)*2
		x := o.X - 4 + (monsterRand()%5)*2
		fw := packetFirework(0, byte(x), byte(y))
		w.send(o, fw)
		w.broadcast(o, fw)
	}
	w.goldMessage(o, w.messageFor(o, 0x295, t.P.Char.Name))
	w.goldMessage(t, w.messageFor(t, 0x295, o.P.Char.Name))
	if w.cfg.Int("Custom", "CustomMarriageAnnounce", 0) != 0 {
		w.noticeAllMsg(NoticeGold, 0x296, o.P.Char.Name, t.P.Char.Name)
	}
	w.log.Info("[CommandMarry] New marriage", "account", o.P.Account.Login, "name", o.P.Char.Name,
		"partner_account", t.P.Account.Login, "partner", t.P.Char.Name)
	w.commandDone(o, c)
}

// marriageRewards is FUN_004669e0: the section 2 items into the inventory.
func (w *World) marriageRewards(o *Object) {
	for _, it := range w.cw().marriage.Rewards {
		s := giftSpec(it)
		s.ToInventory = true
		w.createItem(o, 0xEB, 0, 0, s)
	}
}

// commandDivorce is /divorce (FUN_00466490): only married; an open
// interface gives 0x297, the section 1 item missing 0x298. An online
// partner is unmarried and saved (0x299 gold); an offline one is updated
// by the DataServer (DS 0x0F 02). Then the player is unmarried, loses the
// item, is saved and gets 0x299; the command is charged.
func (w *World) commandDivorce(o *Object, c command, arg string) {
	ch := o.P.Char
	if w.cfg.Int("Custom", "CustomMarriageSwitch", 0) == 0 || !ch.Married {
		return
	}
	m := w.cw().marriage
	if ifBusy(o) {
		w.customMsg(o, 0x297)
		return
	}
	if !w.hasMarriageItem(o, m.Divorce) {
		w.customMsg(o, 0x298, m.Divorce.Count, w.itemName(m.Divorce.Index, m.Divorce.Level))
		return
	}
	couple := ch.MarryCouple
	if t := w.findPlayer(couple); t == nil {
		if cs := w.customStore(); cs != nil {
			w.async(func(ctx context.Context) func() {
				if err := cs.ClearMarriage(ctx, couple); err != nil {
					w.log.Error("divorce", "name", couple, "err", err)
				}
				return nil
			})
		}
	} else {
		t.P.Char.Married, t.P.Char.MarryCouple = false, ""
		w.saveCharacter(t, nil)
		w.goldMessage(t, w.messageFor(t, 0x299))
	}
	w.log.Info("[CommandDivorce]", "account", o.P.Account.Login, "name", ch.Name, "partner", couple)
	ch.Married, ch.MarryCouple = false, ""
	if m.Divorce.Enable != 0 {
		w.deleteItems(o, m.Divorce.Index, m.Divorce.Level, m.Divorce.Count)
	}
	w.saveCharacter(o, nil)
	w.goldMessage(o, w.messageFor(o, 0x299))
	w.commandDone(o, c)
}

// trackMarryBlocked lists the partner maps /trackmarry refuses: the
// CustomArena map, 24..29 (Kalima), 36 (Kalima 7), 9 and 32 (Devil
// Square), 11..17 (Blood Castle) and 18..23 (Chaos Castle).
func (w *World) trackMarryBlocked(mp int) bool {
	if mp == w.cfg.Int("Custom", "CustomArenaMapNumber", 0) {
		return true
	}
	return (mp >= 24 && mp <= 29) || mp == 36 || mp == 9 || mp == 32 || (mp >= 11 && mp <= 23)
}

// commandTrackMarry is /trackmarry (FUN_00466690): married only; the
// partner offline gives 0x29A. With no interface open and the partner on
// an allowed map (Icarus needs wings or a Dinorant / Fenrir) the player
// warps to the partner (FUN_0050fd50) unless CustomMarriageTracePK is set
// and the PK level is 5 or more (0x29C); otherwise 0x29B. The flag
// obj+0x20b0 tested first is never set (U-CUST-21).
func (w *World) commandTrackMarry(o *Object, c command, arg string) {
	ch := o.P.Char
	if w.cfg.Int("Custom", "CustomMarriageSwitch", 0) == 0 || !ch.Married {
		return
	}
	t := w.findPlayer(ch.MarryCouple)
	if t == nil {
		w.customMsg(o, 0x29A)
		return
	}
	if ifBusy(o) || w.trackMarryBlocked(t.Map) {
		w.customMsg(o, 0x29B)
		return
	}
	if t.Map == 10 {
		pet := w.slotItem(o, 8)
		if w.slotItem(o, 7).IsEmpty() && (pet.IsEmpty() || (pet.Index != 13*512+3 && pet.Index != 13*512+37)) {
			w.customMsg(o, 0x29B)
			return
		}
	}
	if o.PKLevel >= 5 && w.cfg.Int("Custom", "CustomMarriageTracePK", 0) != 0 {
		w.customMsg(o, 0x29C)
		return
	}
	w.teleportTo(o, t.Map, t.X, t.Y, o.Dir)
	w.log.Info("[CommandMarryTrack] Follow couple", "account", o.P.Account.Login, "name", ch.Name)
	w.commandDone(o, c)
}

// --- /buyvip (60) --------------------------------------------------------------------

// commandBuyVip is /buyvip code (FUN_00449060): an unknown code gives
// 0x2A2, an account level below the plan's AccountLevel 0x2A3, missing
// zen 0x2A4 (the coin prices have no column and are 0). The zen is paid,
// JS 0x11 sets NewAccountLevel for Days days (setAccountLevel, with the
// JS 0x05 reload), then 0x2A8 and the charge.
func (w *World) commandBuyVip(o *Object, c command, arg string) {
	code := commandToken(arg, 0, 0x20)
	p, ok := w.cw().vip[code]
	if !ok {
		w.customMsg(o, 0x2A2)
		return
	}
	if p.AccountLevel != -1 && accountLevel(o) < p.AccountLevel {
		w.customMsg(o, 0x2A3)
		return
	}
	ch := o.P.Char
	if uint32(ch.Money) < uint32(p.Money) {
		w.customMsg(o, 0x2A4, p.Money)
		return
	}
	if p.Money != 0 {
		ch.Money -= int64(p.Money)
		w.sendMoney(o)
	}
	w.setAccountLevel(o, p.NewAccountLevel, time.Duration(p.Days)*24*time.Hour)
	w.customMsg(o, 0x2A8)
	w.log.Info("[CommandBuyVip] Adquired plan", "account", o.P.Account.Login, "name", ch.Name, "plan", code)
	w.commandDone(o, c)
}

// --- /pack (61), /unpack (62) --------------------------------------------------------

// jewelPackTypes are the ten jewel types of CCustomJewelPack: the
// Custom.dat syntax key (types 5, 7, 8 and 9 have none, so they can never
// be typed), the single jewel (FUN_0044e080) and the 10-jewel pack
// (FUN_0044e100, level 0..5 = 10..60 jewels).
var jewelPackTypes = [10]struct {
	key          string
	single, pack int
}{
	{"CustomJewelPackCommandJoBSyntax", 14*512 + 13, 12*512 + 30},
	{"CustomJewelPackCommandJoSSyntax", 14*512 + 14, 12*512 + 31},
	{"CustomJewelPackCommandJoLSyntax", 14*512 + 16, 12*512 + 136},
	{"CustomJewelPackCommandJCRSyntax", 14*512 + 22, 12*512 + 137},
	{"CustomJewelPackCommandJoGSyntax", 14*512 + 31, 12*512 + 138},
	{"", 14*512 + 42, 12*512 + 140},
	{"CustomJewelPackCommandJoCSyntax", 12*512 + 15, 12*512 + 141},
	{"", 14*512 + 41, 12*512 + 139},
	{"", 14*512 + 43, 12*512 + 142},
	{"", 14*512 + 44, 12*512 + 143},
}

// commandJewelPack is /pack type count (FUN_0044d150 -> FUN_0044d930) and
// /unpack type count (FUN_0044d530 -> FUN_0044dc50). CustomJewelPackSwitch;
// an open interface or a dead player gives 0x183, no type 0x184, an
// unknown type nothing; count must be 10, 20, .., 60 (0x185). /pack needs
// count level-0 jewels (none: 0x186, fewer: 0x187), deletes them and
// creates the pack of level count/10-1. /unpack needs that pack (0x186)
// and count free bag cells (0x188), deletes one pack and creates count
// jewels. Both are charged (indexes 61 and 62).
func (w *World) commandJewelPack(o *Object, c command, arg string, pack bool) {
	if w.cfg.Int("Custom", "CustomJewelPackSwitch", 0) == 0 {
		return
	}
	if ifBusy(o) || !o.Alive {
		w.customMsg(o, 0x183)
		return
	}
	word := commandToken(arg, 0, 0x20)
	if word == "" {
		w.customMsg(o, 0x184)
		return
	}
	typ := -1
	for i, t := range jewelPackTypes {
		if t.key != "" && word == w.cfg.String("Custom", t.key, 0x20) {
			typ = i
			break
		}
	}
	if typ < 0 {
		return
	}
	n := cmdInt(arg, 1)
	if n != 10 && n != 20 && n != 30 && n != 40 && n != 50 && n != 60 {
		w.customMsg(o, 0x185)
		return
	}
	t := jewelPackTypes[typ]
	level := n/10 - 1
	if pack {
		have := w.countItems(o, t.single, 0)
		if have == 0 {
			w.customMsg(o, 0x186)
			return
		}
		if have < n {
			w.customMsg(o, 0x187)
			return
		}
		w.deleteItems(o, t.single, 0, n)
		w.createItem(o, 0xEB, 0, 0, dropSpec{Index: t.pack, Level: level, ToInventory: true})
		w.log.Info("[CommandCustomJewelPack][Pack]", "account", o.P.Account.Login, "name", o.P.Char.Name,
			"index", t.pack, "level", level)
	} else {
		if w.countItems(o, t.pack, level) == 0 {
			w.customMsg(o, 0x186)
			return
		}
		if w.freeBagCells(o) < n {
			w.customMsg(o, 0x188)
			return
		}
		w.deleteItems(o, t.pack, level, 1)
		for i := 0; i < n; i++ {
			w.createItem(o, 0xEB, 0, 0, dropSpec{Index: t.single, ToInventory: true})
		}
		w.log.Info("[CommandCustomJewelPack][Unpack]", "account", o.P.Account.Login, "name", o.P.Char.Name,
			"index", t.single, "count", n)
	}
	w.commandDone(o, c)
}

// freeBagCells is FUN_00494a10: the free cells of the 8x8 bag map.
func (w *World) freeBagCells(o *Object) int {
	g := w.occupancy(&o.P.Inventory, -1)
	n := 0
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			if !g[y][x] {
				n++
			}
		}
	}
	return n
}

// The jewel test of the auto pick and the custom shop (FUN_00496bf0) is
// w.jewelItem in customjewel.go (with the CustomJewel.txt jewels).
