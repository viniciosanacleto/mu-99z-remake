package game

import (
	"context"
	"time"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/devilsquare"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/store"
)

// Custom GM commands of FUN_00435400 (CommandManager indexes 100..129).
// The GM level, delay, level, reset and zen checks are the common part of
// runCommand (CommandManager.txt gives 8 or 32 to every row). Each handler
// receives the text after the command; token n is commandToken(arg, n).
// Messages sent with FUN_004c08f0 go to the GM unless the comment says
// otherwise (the recipient is passed in EDI, read in the assembly).
//
// Index 112 (/setcoin, FUN_0043ade0) has no CommandManager.txt row and
// only logs coin changes that this build never stores; it is not
// reproduced.

func registerCommandGm(h *hooks) {
	for idx, handler := range map[int]func(w *World, o *Object, c command, arg string){
		100: (*World).commandGMMove,
		101: (*World).commandGMPost,
		102: (*World).commandTrace,
		103: (*World).commandTrack,
		104: (*World).commandDisconnect,
		105: (*World).commandFirework,
		106: (*World).commandMake,
		107: (*World).commandDrop,
		108: (*World).commandSkin,
		109: (*World).commandSetMoney,
		110: (*World).commandHide,
		111: (*World).commandEdit,
		113: (*World).commandSetPK,
		114: (*World).commandSetVip,
		115: (*World).commandMoveAll,
		116: (*World).commandMoveGuild,
		117: (*World).commandMakeMob,
		118: (*World).commandMakeSet,
		119: func(w *World, o *Object, c command, arg string) { w.commandBan(o, c, arg, false) },
		120: func(w *World, o *Object, c command, arg string) { w.commandBan(o, c, arg, true) },
		121: (*World).commandBanChat,
		122: func(w *World, o *Object, c command, arg string) { w.commandUnban(o, c, arg, false) },
		123: func(w *World, o *Object, c command, arg string) { w.commandUnban(o, c, arg, true) },
		124: (*World).commandUnbanChat,
		125: (*World).commandBCStart,
		126: (*World).commandDSStart,
		127: (*World).commandCCStart,
		129: (*World).commandSetBuff,
	} {
		h.commandHandlers[idx] = handler
	}
}

// gmState holds the per-player fields used by the GM commands and the
// Custom events.
type gmState struct {
	Skin      int  // obj+0x314 (Change): -1 none
	InEvent   bool // obj+0x24f0: in a Sobre / Duel / Pair event
	RestorePK bool // obj+0x24f8: PK level back to 3 when alive (FUN_0050e280)
	SendHome  bool // obj+0x24f9: sent to town when alive (FUN_0050e280)
}

func gmOf(o *Object) *gmState {
	if o.P.gm == nil {
		o.P.gm = &gmState{Skin: -1}
	}
	return o.P.gm
}

// Messages of the GM commands (Lang/*/Message.txt).
const (
	msgGMTargetNotFound = 0x171
	msgGMPost           = 0x172
	msgGMItemCreated    = 0x173
	msgGMEdited         = 0x174
	msgGMEditedBy       = 0x175
	msgGMPKSet          = 0x178
	msgGMVipChanged     = 0x179
	msgGMVipSet         = 0x17A
	msgGMGuildNotFound  = 0x17B
	msgGMMobSafeZone    = 0x17C
	msgGMMobFailed      = 0x17D
	msgGMMobNPC         = 0x17E
	msgGMBuffMissing    = 0x18B
	msgGMBanned         = 0x2AD
	msgGMBlocked        = 0x2AE
	msgGMUnblocked      = 0x2AF
	msgGMNameShort      = 0x2B1
	msgGMNameMissing    = 0x2B7
	msgGMChatMinutes    = 0x2BB
	msgGMChatUnbanned   = 0x2BC
	msgGMChatBanned     = 0x2BD
	msgAccountTypeBase  = 0x9B // 0x9B + AccountLevel: "Tipo de conta: ..."
	effGMHide           = 0x12
	gmMaxMoney          = 2000000000
)

func (w *World) msg(o *Object, n int, args ...any) { w.systemMessage(o, w.messageFor(o, n, args...)) }

// gmTarget is FUN_004352a0(name, 0xB) + FUN_0050c6d0: the playing
// character named by token 0, or message 0x171.
func (w *World) gmTarget(o *Object, arg string) *Object {
	t := w.findPlayer(commandToken(arg, 0, 0xB))
	if t == nil {
		w.msg(o, msgGMTargetNotFound)
	}
	return t
}

// gmLog writes the [CommandXxx] log line of the original.
func (w *World) gmLog(tag string, o *Object, kv ...any) {
	w.log.Info("["+tag+"]", append([]any{"account", o.P.Account.Login, "name", o.P.Char.Name}, kv...)...)
}

// gmWarp is FUN_0050fd50: a teleport to (map, x, y) that keeps the
// direction; offline objects (obj+0x20d9, 0x2110, 0x249e) do not exist in
// this rewrite.
func (w *World) gmWarp(o *Object, mp, x, y int) {
	w.teleportTo(o, mp, x, y, o.Dir)
}

// gmDisconnect is FUN_005102a0: the six second "close game" countdown
// (obj+0x0a = 6) when none is running.
func gmDisconnect(o *Object) {
	if o.P.LogoutCountdown < 1 {
		o.P.LogoutCountdown, o.P.LogoutType = 6, 0
	}
}

// --- 100..105 ----------------------------------------------------------------------------

// commandGMMove is /gmove name map x y (FUN_004394b0): the character goes
// to the place.
func (w *World) commandGMMove(o *Object, c command, arg string) {
	t := w.gmTarget(o, arg)
	if t == nil {
		return
	}
	mp, x, y := cmdInt(arg, 1), cmdInt(arg, 2), cmdInt(arg, 3)
	w.gmWarp(t, mp, x, y)
	w.gmLog("CommandGMMove", o, "target", t.P.Char.Name, "map", mp, "x", x, "y", y)
	w.commandDone(o, c)
}

// commandGMPost is /gg text (FUN_00439620): Message.txt 0x172 "[%s] %s"
// with the GM name and the whole text, sent as a type 0 notice to the
// server group (FUN_00475b50, DS C1 94 21).
func (w *World) commandGMPost(o *Object, c command, arg string) {
	w.groupNotice(NoticeGold, w.messageFor(nil, msgGMPost, o.P.Char.Name, arg))
	w.gmLog("CommandGMPost", o, "message", arg)
	w.commandDone(o, c)
}

// commandTrace is /trace name (FUN_00439760, logged as CommandTrack): the
// GM goes to the character.
func (w *World) commandTrace(o *Object, c command, arg string) {
	t := w.gmTarget(o, arg)
	if t == nil {
		return
	}
	w.gmWarp(o, t.Map, t.X, t.Y)
	w.gmLog("CommandTrack", o, "target", t.P.Char.Name)
	w.commandDone(o, c)
}

// commandTrack is /track name (FUN_004398a0, logged as CommandTrace): the
// character comes to the GM.
func (w *World) commandTrack(o *Object, c command, arg string) {
	t := w.gmTarget(o, arg)
	if t == nil {
		return
	}
	w.gmWarp(t, o.Map, o.X, o.Y)
	w.gmLog("CommandTrace", o, "target", t.P.Char.Name)
	w.commandDone(o, c)
}

// commandDisconnect is /dc name (FUN_004399e0).
func (w *World) commandDisconnect(o *Object, c command, arg string) {
	t := w.gmTarget(o, arg)
	if t == nil {
		return
	}
	gmDisconnect(t)
	w.gmLog("CommandDisconnect", o, "target", t.P.Char.Name)
	w.commandDone(o, c)
}

// commandFirework is /firework (FUN_00439b10): ten C1 07 F3 40 00 x y to
// the GM and its viewport, x = X + (r2%5-2)*2 and y = Y + (r1%5-2)*2 with
// two draws per firework (r1 first).
func (w *World) commandFirework(o *Object, c command, arg string) {
	for i := 0; i < 10; i++ {
		r1 := monsterRand()
		r2 := monsterRand()
		x := byte((r2%5-2)*2 + o.X)
		y := byte((r1%5-2)*2 + o.Y)
		p := packetFirework(0, x, y)
		w.send(o, p)
		w.broadcast(o, p)
	}
	w.gmLog("CommandFireworks", o, "map", o.Map, "x", o.X, "y", o.Y)
	w.commandDone(o, c)
}

// --- 106 /make, 107 /drop, 118 /makeset -------------------------------------------------

// gmItemArgs are the item columns of /make and /drop starting at token
// first: section, index, level, skill, luck, option, exc, set and (only
// for /make) Jewel of Harmony and 380 option, then the empty socket count
// and the duration in seconds.
type gmItemArgs struct {
	Index                      int
	Level, Skill, Luck, Option int
	Exc, Set                   int
	Duration                   int
}

func gmItem(arg string, first int, harmony bool) gmItemArgs {
	t := func(n int) int { return cmdInt(arg, first+n) }
	a := gmItemArgs{Index: t(0)*512 + t(1), Level: t(2), Skill: t(3), Luck: t(4), Option: t(5), Exc: t(6), Set: t(7)}
	// JoH, 380 and the socket count (t(8)..t(10) or t(8)) have no meaning
	// in 97D/99z items; the duration (CPeriodicItem) is not kept either.
	if harmony {
		a.Duration = t(11)
	} else {
		a.Duration = t(9)
	}
	return a
}

func (a gmItemArgs) spec(inventory bool) dropSpec {
	return dropSpec{Index: a.Index, Level: a.Level, Skill: a.Skill, Luck: a.Luck, Option: a.Option,
		Exc: byte(a.Exc), Set: byte(a.Set), Duration: a.Duration, ToInventory: inventory}
}

// gmItemValid is FUN_00513740: the index must exist in Item.txt.
func (w *World) gmItemValid(index int) bool {
	_, ok := w.data.Items[index]
	return ok
}

// commandMake is /make (FUN_00439c60): "/make sec idx lvl skill luck opt
// exc set joh 380 socket dur" creates the item in the GM's inventory
// (map 0xEB); a negative first token is a count and moves the columns by
// one. An unknown index stops silently; otherwise message 0x173.
func (w *World) commandMake(o *Object, c command, arg string) {
	n := cmdInt(arg, 0)
	count, first := 1, 0
	if n < 0 {
		count, first = -n, 1
	}
	a := gmItem(arg, first, true)
	if !w.gmItemValid(a.Index) {
		return
	}
	for i := 0; i < count; i++ {
		w.createItem(o, 0xEB, 0, 0, a.spec(true))
	}
	w.gmLog("CommandMake", o, "count", count, "index", a.Index, "level", a.Level)
	w.msg(o, msgGMItemCreated)
	w.commandDone(o, c)
}

// commandDrop is /drop (FUN_0043a0c0): the same columns without JoH/380,
// the item drops at the GM's cell with the GM as loot owner. With a count
// every item after the first moves from the previous cell
// (FUN_0050c860, +-2 cells, 10 tries; failure falls back to the GM's
// cell).
func (w *World) commandDrop(o *Object, c command, arg string) {
	n := cmdInt(arg, 0)
	if n >= 0 {
		a := gmItem(arg, 0, false)
		if !w.gmItemValid(a.Index) {
			return
		}
		w.createItem(o, o.Map, o.X&0xFF, o.Y&0xFF, a.spec(false))
		w.gmLog("CommandDrop", o, "index", a.Index, "level", a.Level)
	} else {
		a := gmItem(arg, 1, false)
		if !w.gmItemValid(a.Index) {
			return
		}
		x, y := o.X, o.Y
		for i := 0; i < -n; i++ {
			nx, ny, ok := w.randomNearCell(o.Map, x, y, 2, 2, 10)
			if ok {
				x, y = nx, ny
			} else {
				x, y = o.X, o.Y
			}
			w.createItem(o, o.Map, x, y, a.spec(false))
		}
		w.gmLog("CommandDrop", o, "count", -n, "index", a.Index, "level", a.Level)
	}
	w.msg(o, msgGMItemCreated)
	w.commandDone(o, c)
}

// commandMakeSet is /makeset type lvl luck opt exc set joh 380 socket dur
// (FUN_0043bbf0): the helm, armor, pants, gloves and boots (sections
// 7..11) of index type that exist in Item.txt go to the GM's inventory,
// without skill. Message 0x173 even when none exists.
func (w *World) commandMakeSet(o *Object, c command, arg string) {
	t := func(n int) int { return cmdInt(arg, n) }
	typ := t(0)
	for sec := 7; sec <= 11; sec++ {
		idx := sec*512 + typ
		if !w.gmItemValid(idx) {
			continue
		}
		w.createItem(o, 0xEB, 0, 0, dropSpec{Index: idx, Level: t(1), Luck: t(2), Option: t(3),
			Exc: byte(t(4)), Set: byte(t(5)), Duration: t(9), ToInventory: true})
	}
	w.gmLog("CommandMakeSet", o, "type", typ, "level", t(1))
	w.msg(o, msgGMItemCreated)
	w.commandDone(o, c)
}

// --- 108..111 ----------------------------------------------------------------------------

// commandSkin is /skin name n (FUN_0043a510): obj+0x314 = n and the
// character is shown again (FUN_0050da50). The C2 45 "changed character"
// viewport list (FUN_00515db0) is not reconstructed (U-GM-01), so other
// players keep seeing the normal character.
func (w *World) commandSkin(o *Object, c command, arg string) {
	t := w.gmTarget(o, arg)
	if t == nil {
		return
	}
	gmOf(t).Skin = cmdInt(arg, 1)
	w.reshow(t)
	w.gmLog("CommandSkin", o, "target", t.P.Char.Name, "change", gmOf(t).Skin)
	w.commandDone(o, c)
}

// commandSetMoney is /setzen name n (FUN_0043a690): n < 1 sets the zen to
// 0, otherwise it is added up to 2,000,000,000 (FUN_0050cf10); C3 10 22
// FE to the character.
func (w *World) commandSetMoney(o *Object, c command, arg string) {
	t := w.gmTarget(o, arg)
	if t == nil {
		return
	}
	n := cmdInt(arg, 1)
	ch := t.P.Char
	switch {
	case n < 1:
		ch.Money = 0
	case ch.Money+int64(n) > gmMaxMoney:
		ch.Money = gmMaxMoney
	default:
		ch.Money += int64(n)
	}
	w.sendMoney(t)
	w.gmLog("CommandSetMoney", o, "target", ch.Name, "money", n)
	w.commandDone(o, c)
}

// commandHide is /hide (FUN_0043a850): without effect 0x12 (Invisibility)
// it is added for good (FUN_00478ae0, duration 0) and every viewer gets
// C1 06 14 01 index (FUN_0050d9b0); with it the effect is removed and the
// GM is shown again (FUN_0050da50). The player viewport skips invisible
// objects of Authority 4/8 (FUN_00515160).
func (w *World) commandHide(o *Object, c command, arg string) {
	if hasEffectIndex(o, effGMHide) {
		w.removeEffect(o, effGMHide)
		w.reshow(o)
		w.gmLog("CommandHide", o, "state", 0)
	} else {
		w.addEffect(o, effGMHide, 0, [4]int{}, true)
		w.broadcast(o, packetDestroy([]int{o.Index}))
		w.gmLog("CommandHide", o, "state", 1)
	}
	w.commandDone(o, c)
}

// gmHidden is the invisibility test of FUN_00515160: effect 0x12 on an
// object whose Authority (obj+0x250) is 4 or gmFull.
func gmHidden(o *Object) bool {
	if o.Move == nil || (o.Move.Authority != 4 && !gmFull(o)) {
		return false
	}
	return hasEffectIndex(o, effGMHide)
}

// commandEdit is /edit name level points str agi vit ene cmd
// (FUN_0043a950). A negative value resets the field (level 1, points 0,
// stats to DefaultClassInfo, leadership only for the Dark Lord), a
// positive one is added; the level stops at MaxLevel and the stats at
// MaxStatPoint of the character's account level. Then the experience is
// raised to the level floor (FUN_0050be30), F3 E0, CalcCharacter and
// messages 0x174 (GM) and 0x175 (character, when not the GM).
func (w *World) commandEdit(o *Object, c command, arg string) {
	t := w.gmTarget(o, arg)
	if t == nil {
		return
	}
	v := func(n int) int { return cmdInt(arg, n) }
	ch := t.P.Char
	maxStat := w.cfg.ALInt("Common", "MaxStatPoint", t.P.Account.Level, 0)
	var base struct{ Str, Dex, Vit, Ene, Lead int }
	if w.statsData != nil && t.Class < 5 {
		if d, ok := w.statsData.classes[t.Class]; ok {
			base.Str, base.Dex, base.Vit, base.Ene = d.Strength, d.Dexterity, d.Vitality, d.Energy
		}
		if d, ok := w.statsData.classes[4]; ok {
			base.Lead = d.Lead
		}
	}
	if n := v(1); n < 0 {
		ch.Level = 1
	} else {
		ch.Level = int(int16(ch.Level + n))
	}
	if ml := w.cfg.Int("Common", "MaxLevel", 0); ml < ch.Level {
		ch.Level = ml
	}
	if n := v(2); n < 0 {
		ch.LevelUpPoints = 0
	} else {
		ch.LevelUpPoints += n
	}
	stat := func(p *int, n, def int) {
		if n < 0 {
			*p = def
		} else {
			*p += n
		}
		if *p > maxStat {
			*p = maxStat
		}
	}
	stat(&ch.Strength, v(3), base.Str)
	stat(&ch.Dexterity, v(4), base.Dex)
	stat(&ch.Vitality, v(5), base.Vit)
	stat(&ch.Energy, v(6), base.Ene)
	if t.Class == 4 {
		stat(&ch.Leadership, v(7), base.Lead)
	}
	if w.data.Experience != nil {
		if floor := int64(w.data.Experience.Floor(ch.Level)); ch.Experience < floor {
			ch.Experience = floor
		}
	}
	w.send(t, w.packetCharRefresh(t))
	w.calcCharacter(t)
	w.msg(o, msgGMEdited)
	if t != o {
		w.msg(t, msgGMEditedBy, o.P.Char.Name)
	}
	w.gmLog("CommandGMEdit", o, "target", ch.Name, "level", ch.Level, "points", ch.LevelUpPoints,
		"str", ch.Strength, "agi", ch.Dexterity, "vit", ch.Vitality, "ene", ch.Energy, "cmd", ch.Leadership)
	w.commandDone(o, c)
}

// --- 113 /setpk, 114 /setvip --------------------------------------------------------------

// commandSetPK is /setpk name level (FUN_0043b080): level 0..6; PKCount
// and PKTime follow the level (0 and 6: count 3 and PKDownRequirePoint3;
// 1 and 5: Point2; 2 and 4: Point1; 3: 0), C1 07 F3 08 to the character
// and its viewport, message 0x178.
func (w *World) commandSetPK(o *Object, c command, arg string) {
	t := w.gmTarget(o, arg)
	if t == nil {
		return
	}
	lv := min(max(cmdInt(arg, 1), 0), 6)
	ch := t.P.Char
	t.PKLevel, ch.PKLevel = lv, lv
	ch.PKCount = 0
	switch lv {
	case 0, 6:
		ch.PKCount = 3
		ch.PKTime = w.cfg.Int("Common", "PKDownRequirePoint3", 0)
	case 1, 5:
		ch.PKTime = w.cfg.Int("Common", "PKDownRequirePoint2", 0)
	case 2, 4:
		ch.PKTime = w.cfg.Int("Common", "PKDownRequirePoint1", 0)
	case 3:
		ch.PKTime = 0
	}
	w.sendPKLevel(t)
	w.msg(o, msgGMPKSet)
	w.gmLog("CommandGMSetPK", o, "target", ch.Name, "pklevel", lv)
	w.commandDone(o, c)
}

// commandSetVip is /setvip name level days (FUN_0043b2f0): a negative
// value or a level above 3 turns both into 0. JS 0x11 (FUN_004a2a80,
// WZ_SetAccountLevel: the same level extends the expiration, another one
// starts from now) then JS 0x05 reloads the level (reply FUN_004a2270,
// gmAccountLevelReply). Messages 0x179 (character, when not the GM) and
// 0x17A (GM) are sent at once.
func (w *World) commandSetVip(o *Object, c command, arg string) {
	t := w.gmTarget(o, arg)
	if t == nil {
		return
	}
	level, days := cmdInt(arg, 1), cmdInt(arg, 2)
	if days < 0 || level < 0 || level > 3 {
		level, days = 0, 0
	}
	w.setAccountLevel(t, level, time.Duration(days)*24*time.Hour)
	if t != o {
		w.msg(t, msgGMVipChanged)
	}
	w.msg(o, msgGMVipSet)
	w.gmLog("CommandGMSetVip", o, "target_account", t.P.Account.Login, "level", level, "days", days)
	w.commandDone(o, c)
}

// setAccountLevel is JS 0x11 + JS 0x05 for a playing character.
func (w *World) setAccountLevel(t *Object, level int, d time.Duration) {
	login := t.P.Account.Login
	idx, conn := t.Index, t.conn
	w.async(func(ctx context.Context) func() {
		var err error
		if w.auth != nil {
			err = w.auth.SetAccountLevel(ctx, login, level, d)
		} else if w.store != nil {
			err = storeSetAccountLevel(ctx, w.store, login, level, d, w.now())
		}
		if err != nil {
			w.log.Error("set account level", "account", login, "err", err)
			return nil
		}
		var acc *store.Account
		if w.store != nil {
			acc, err = w.store.AccountByLogin(ctx, login)
		}
		return func() {
			if err != nil || acc == nil {
				return
			}
			t := w.objects[idx]
			if t.conn != conn || t.P == nil || !t.IsPlayer() {
				return
			}
			w.gmAccountLevelReply(t, acc.Level, acc.LevelExpiresAt)
		}
	})
}

// storeSetAccountLevel is WZ_SetAccountLevel without the auth service.
func storeSetAccountLevel(ctx context.Context, s store.Store, login string, level int, d time.Duration, now time.Time) error {
	acc, err := s.AccountByLogin(ctx, login)
	if err != nil {
		return err
	}
	base := now
	if acc.Level == level && acc.LevelExpiresAt.After(now) {
		base = acc.LevelExpiresAt
	}
	return s.SetAccountLevel(ctx, acc.ID, level, base.Add(d))
}

// gmAccountLevelReply is the JS 0x05 reply (FUN_004a2270): a level below
// ServerLock closes the game (FUN_005102a0); the same level and expire
// date only update obj+0x618/0x61c; otherwise they change and message
// 0x9B + level shows the account type with the expire date.
func (w *World) gmAccountLevelReply(t *Object, level int, expires time.Time) {
	if level < w.cfg.ServerLock {
		gmDisconnect(t)
		return
	}
	same := t.P.Account.Level == level && t.P.Account.LevelExpiresAt.Equal(expires)
	t.P.Account.Level, t.P.Account.LevelExpiresAt = level, expires
	if same {
		return
	}
	w.msg(t, msgAccountTypeBase+level, expireText(expires))
}

// expireText is the AccountExpireDate column as the ODBC driver returns
// it (U-GM-03).
func expireText(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Local().Format("2006-01-02 15:04:05")
}

// --- 115..117 ----------------------------------------------------------------------------

// commandMoveAll is /moveall map x y (FUN_0043b560): every playing
// character without a logout countdown or server move, the GM included.
func (w *World) commandMoveAll(o *Object, c command, arg string) {
	mp, x, y := cmdInt(arg, 0), cmdInt(arg, 1), cmdInt(arg, 2)
	for i := PlayerStart; i < MaxObjects; i++ {
		t := w.objects[i]
		if t.State == StatePlaying && t.IsPlayer() && canPlayerAct(t) {
			w.gmWarp(t, mp, x, y)
		}
	}
	w.gmLog("CommandMoveAll", o, "map", mp, "x", x, "y", y)
	w.commandDone(o, c)
}

// commandMoveGuild is /moveguild guild map x y (FUN_0043b650): the guild
// (FUN_00487350, exact name of up to 8 characters) or message 0x17B; its
// members playing on this server are moved.
func (w *World) commandMoveGuild(o *Object, c command, arg string) {
	name := commandToken(arg, 0, 9)
	mp, x, y := cmdInt(arg, 1), cmdInt(arg, 2), cmdInt(arg, 3)
	g, ok := w.guildMgr().Find(name)
	if !ok {
		w.msg(o, msgGMGuildNotFound)
		return
	}
	for _, m := range g.Members {
		if m.Name == "" {
			continue
		}
		if t := w.findPlayer(m.Name); t != nil {
			w.gmWarp(t, mp, x, y)
		}
	}
	w.gmLog("CommandMoveGuild", o, "guild", g.Name, "map", mp, "x", x, "y", y)
	w.commandDone(o, c)
}

// commandMakeMob is /mob class amount (FUN_0043b820): refused in a safe
// zone (0x17C); amount 1..50 monsters are created on the GM's cell with a
// random direction (FUN_004b4b00 + FUN_004b4fe0). A failure (0x17D) or an
// NPC class (0x17E) deletes the object and stops.
func (w *World) commandMakeMob(o *Object, c command, arg string) {
	class, amount := cmdInt(arg, 0), cmdInt(arg, 1)
	if w.attrAt(o.Map, o.X, o.Y)&1 != 0 {
		w.msg(o, msgGMMobSafeZone)
		return
	}
	amount = min(max(amount, 1), 50)
	for i := 0; i < amount; i++ {
		idx := w.allocMonsterIndex()
		if idx < 0 {
			w.msg(o, msgGMMobFailed)
			return
		}
		m := w.objects[idx]
		m.reset()
		m.State = StateConnected
		m.X, m.Y, m.TX, m.TY = o.X, o.Y, o.X, o.Y
		m.Dir = monsterRand() & 7
		m.Map = o.Map
		if !w.setMonster(m, class) {
			w.msg(o, msgGMMobFailed)
			w.deleteMonster(m)
			return
		}
		if m.Type == TypeNPC {
			w.msg(o, msgGMMobNPC)
			w.deleteMonster(m)
			return
		}
	}
	w.gmLog("CommandMakeMob", o, "mob", class, "amount", amount, "map", o.Map, "x", o.X, "y", o.Y)
	w.commandDone(o, c)
}

// --- 119..124 bans ------------------------------------------------------------------------

// commandBan is /banacc and /banchar name (FUN_0043bec0 / FUN_0043c0d0):
// DS C1 18 0F 05|06 index name[10] 1 (WZ_BanAccount on the character's
// account / WZ_BanCharacter on its name), message 0x2AD to the character,
// its game closes (FUN_005102a0) and message 0x2AE (name) to the GM. The
// ban is enforced by the login (bloc_code, result 5) and the character
// select (CtlCode bit 0).
func (w *World) commandBan(o *Object, c command, arg string, char bool) {
	t := w.gmTarget(o, arg)
	if t == nil {
		return
	}
	key, tag := t.P.Account.Login, "CommandBanAcc"
	if char {
		key, tag = t.P.Char.Name, "CommandBanChar"
		t.P.Char.CtlCode |= 1 // the logout save must not lift the ban
	}
	w.saveBan(key, char, 1, nil)
	w.msg(t, msgGMBanned)
	gmDisconnect(t)
	w.msg(o, msgGMBlocked, t.P.Char.Name)
	w.gmLog(tag, o, "target_account", t.P.Account.Login, "target", t.P.Char.Name)
	w.commandDone(o, c)
}

// commandUnban is /unbanacc account and /unbanchar name (FUN_0043c4a0 /
// FUN_0043c5b0): fewer than 4 characters gives 0x2B1; otherwise DS 0F
// 05|06 with 0. The reply (FUN_0043e710 / FUN_0043e850) sends 0x2B7 when
// the name does not exist, otherwise 0x2AF and charges the command.
func (w *World) commandUnban(o *Object, c command, arg string, char bool) {
	name := commandToken(arg, 0, 0xB)
	if len(name) < 4 {
		w.msg(o, msgGMNameShort)
		return
	}
	idx, conn := o.Index, o.conn
	w.saveBan(name, char, 0, func(found bool) {
		o := w.objects[idx]
		if o.conn != conn || !o.IsPlayer() || o.P.Char == nil {
			return
		}
		if !found {
			w.msg(o, msgGMNameMissing)
			return
		}
		w.msg(o, msgGMUnblocked, name)
		tag := "CommandUnbanAcc"
		if char {
			tag = "CommandUnBanChar"
		}
		w.gmLog(tag, o, "target", name)
		w.commandFinish(o, c)
	})
}

// saveBan is DS 0x0F 05/06 (WZ_BanAccount / WZ_BanCharacter).
func (w *World) saveBan(key string, char bool, code int, done func(found bool)) {
	bs, _ := w.store.(store.BanStore)
	if bs == nil {
		return
	}
	w.async(func(ctx context.Context) func() {
		var found bool
		var err error
		if char {
			found, err = bs.BanCharacter(ctx, key, code)
		} else {
			found, err = bs.BanAccount(ctx, key, code)
		}
		if err != nil {
			w.log.Error("ban", "name", key, "err", err)
		}
		if done == nil {
			return nil
		}
		return func() { done(err == nil && found) }
	})
}

// commandBanChat is /banchat name minutes (FUN_0043c2f0): minutes < 1
// gives 0x2BB; otherwise ChatLimitTime (obj+0x16c) = minutes, the second
// counter (obj+0x170) restarts and message 0x2BD (name, minutes).
func (w *World) commandBanChat(o *Object, c command, arg string) {
	n := cmdInt(arg, 1)
	if n < 1 {
		w.msg(o, msgGMChatMinutes)
		return
	}
	t := w.gmTarget(o, arg)
	if t == nil {
		return
	}
	t.P.Char.ChatLimitTime = n
	mv(t).ChatSeconds = 0
	w.msg(o, msgGMChatBanned, t.P.Char.Name, n)
	w.gmLog("CommandBanChat", o, "target", t.P.Char.Name, "minutes", n)
	w.commandDone(o, c)
}

// commandUnbanChat is /unbanchat name (FUN_0043c6c0): message 0x2BC.
func (w *World) commandUnbanChat(o *Object, c command, arg string) {
	t := w.gmTarget(o, arg)
	if t == nil {
		return
	}
	t.P.Char.ChatLimitTime = 0
	mv(t).ChatSeconds = 0
	w.msg(o, msgGMChatUnbanned, t.P.Char.Name)
	w.gmLog("CommandUnbanChat", o, "target", t.P.Char.Name)
	w.commandDone(o, c)
}

// --- 125..127 event force start -----------------------------------------------------------

// forceStartRow is the schedule row the ForceStart functions append
// (FUN_00417c00 / FUN_00470880 / FUN_00434080 -> FUN_0048a840): today, at
// the current hour, minute + 1 and the current second, any weekday. At
// minute 59 the row asks for minute 60 and never matches, as in the
// original.
func forceStartRow(now time.Time) (r gamedata.Schedule) {
	r.Year, r.Month, r.Day, r.DayOfWeek = now.Year(), int(now.Month()), now.Day(), -1
	r.Hour, r.Minute, r.Second = now.Hour(), now.Minute()+1, now.Second()
	return r
}

// commandBCStart is /bcstart (FUN_0043c840 -> FUN_00417c00): the row is
// added to the Blood Castle schedule and every castle goes back to EMPTY
// (FUN_00413880), which also sends its users out.
func (w *World) commandBCStart(o *Object, c command, arg string) {
	if bc := w.ev().bc; bc != nil {
		bc.data.Schedule = append(bc.data.Schedule, forceStartRow(w.now()))
		for i := range bc.c {
			if w.bcOn() {
				bc.c[i].State = devilsquare.StateEmpty
				w.bcEnterEmpty(&bc.c[i])
			}
		}
	}
	w.gmLog("CommandBCStart", o)
	w.commandDone(o, c)
}

// commandDSStart is /dsstart (FUN_0043c910 -> FUN_00470880, FUN_0046de30).
func (w *World) commandDSStart(o *Object, c command, arg string) {
	if ds := w.ev().ds; ds != nil {
		ds.data.Schedule = append(ds.data.Schedule, forceStartRow(w.now()))
		for i := range ds.sq {
			if w.dsOn() {
				ds.sq[i].State = devilsquare.StateEmpty
				w.dsEnterEmpty(&ds.sq[i])
			}
		}
	}
	w.gmLog("CommandDSStart", o)
	w.commandDone(o, c)
}

// commandCCStart is /ccstart (FUN_0043c9b0 -> FUN_00434080, FUN_004302a0).
func (w *World) commandCCStart(o *Object, c command, arg string) {
	if cc := w.ev().cc; cc != nil {
		cc.data.Schedule = append(cc.data.Schedule, forceStartRow(w.now()))
		for i := range cc.c {
			if w.ccOn() {
				cc.c[i].State = devilsquare.StateEmpty
				w.ccEnterEmpty(&cc.c[i])
			}
		}
	}
	w.gmLog("CommandCCStart", o)
	w.commandDone(o, c)
}

// --- 129 /setbuff -------------------------------------------------------------------------

// commandSetBuff is /setbuff name buff type duration p1 p2 p3 p4
// (FUN_0043ca80): buff -1 removes every timed effect (FUN_0047a540); an
// index outside 0..127 or missing from EffectList.txt gives 0x18B (no
// charge); an active effect is removed, otherwise it is added with the
// duration and the four values. Type > 0 makes the original store
// time(NULL) + duration (an expiry date); here the duration is counted in
// seconds in both cases (U-GM-04).
func (w *World) commandSetBuff(o *Object, c command, arg string) {
	t := w.gmTarget(o, arg)
	if t == nil {
		return
	}
	v := func(n int) int { return cmdInt(arg, n) }
	buff := v(1)
	switch {
	case buff == -1:
		w.clearEffects(t)
		w.gmLog("CommandSetBuff", o, "target", t.P.Char.Name, "del_all", true)
	default:
		if _, ok := w.effectTable()[buff]; buff < 0 || buff > 0x7F || !ok {
			w.msg(o, msgGMBuffMissing)
			return
		}
		if hasEffectIndex(t, buff) {
			w.removeEffect(t, buff)
			w.gmLog("CommandSetBuff", o, "target", t.P.Char.Name, "del", buff)
		} else {
			w.addEffect(t, buff, v(3), [4]int{v(4), v(5), v(6), v(7)}, true)
			w.gmLog("CommandSetBuff", o, "target", t.P.Char.Name, "add", buff, "duration", v(3))
		}
	}
	w.commandDone(o, c)
}
