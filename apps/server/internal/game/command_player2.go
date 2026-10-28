package game

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/auth"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/item"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/store"
)

// Custom player commands of FUN_00435400 (CommandManager indexes 3,
// 12..15, 19, 20, 22..28) and the hard-coded /bau. /guildwar and
// /guildsoccer (1, 2) live in guild_war.go; index 21 and /offline
// (CHelperOffline, FUN_004888e0) are pending.

func registerCommandPlayer2(h *hooks) {
	h.commandHandlers[3] = (*World).commandRequest
	h.commandHandlers[12] = (*World).commandChange
	h.commandHandlers[13] = (*World).commandWare
	h.commandHandlers[14] = (*World).commandReset
	h.commandHandlers[15] = (*World).commandMasterReset
	h.commandHandlers[19] = (*World).commandRename
	h.commandHandlers[20] = (*World).commandDCFriend
	h.commandHandlers[22] = (*World).commandOpenWare
	h.commandHandlers[23] = (*World).commandChangeClass
	h.commandHandlers[24] = (*World).commandEZen
	h.commandHandlers[25] = (*World).commandAutoPotion
	h.commandHandlers[26] = (*World).commandLock
	h.commandHandlers[27] = (*World).commandUnlock
	h.commandHandlers[28] = (*World).commandQuizAnswer
	h.hardCodedHandlers["/bau"] = (*World).commandBau
	h.start = append(h.start, loadCmdWorld)
	h.tick1000 = append(h.tick1000, (*World).autoResetSecond, (*World).autoPotionSecond)
}

// cmdState holds the per-player fields of these commands.
type cmdState struct {
	ResetPending bool      // obj+0x69c: DS 0x0F 00/01 request in flight
	AutoReset    bool      // obj+0x654
	AutoPoints   [5]int    // obj+0x658..0x668: /f../c auto amounts after an auto reset
	AutoPotion   bool      // obj+0x24ec
	PotionTime   time.Time // obj+0x24ac: next potion use (GetTickCount + 600)
}

func cmdOf(o *Object) *cmdState {
	if o.P.cmd == nil {
		o.P.cmd = &cmdState{}
	}
	return o.P.cmd
}

// cmdWorld holds Util/ResetTable.txt, Util/ResetLimit.txt and the
// CustomQuiz state.
type cmdWorld struct {
	resetTable []resetTableRow
	resetLimit []resetLimitRow
	quiz       *customQuiz
}

// resetTableRow is a CResetTable row (0x1a ints, 14 read): MinReset
// MaxReset Level_AL0..3 Money_AL0..3 Point_AL0..3; -1 uses the
// Command.dat value.
type resetTableRow struct {
	MinReset, MaxReset int
	Level, Money       [4]int
	Point              [4]int
}

// resetLimitRow is a CResetLimit row (14 ints): MinReset MaxReset
// LimitDay_AL0..3 LimitWek_AL0..3 LimitMon_AL0..3.
type resetLimitRow struct {
	MinReset, MaxReset int
	Day, Week, Month   [4]int
}

func loadCmdWorld(w *World) error {
	w.cmd = &cmdWorld{}
	if w.configDir == "" {
		return nil
	}
	f, err := gamedata.LoadUtilConfig(w.configDir)
	if err != nil {
		return err
	}
	for _, r := range f.ResetTable.Rows {
		w.cmd.resetTable = append(w.cmd.resetTable, resetTableRow{MinReset: r.MinReset, MaxReset: r.MaxReset,
			Level: r.Level, Money: r.Money, Point: r.Point})
	}
	for _, r := range f.ResetLimit.Rows {
		w.cmd.resetLimit = append(w.cmd.resetLimit, resetLimitRow{MinReset: r.MinReset, MaxReset: r.MaxReset,
			Day: r.LimitDay, Week: r.LimitWeek, Month: r.LimitMonth})
	}
	return w.loadQuiz()
}

func (w *World) cmdWorld() *cmdWorld {
	if w.cmd == nil {
		w.cmd = &cmdWorld{}
	}
	return w.cmd
}

// commandByIndex finds the CommandManager row of an index (the delay and
// charge of FUN_00436c30 when the command completes outside the chat).
func (w *World) commandByIndex(index int) command {
	if w.moveData != nil {
		for _, c := range w.moveData.commands {
			if c.Index == index {
				return c
			}
		}
	}
	return command{Index: index}
}

// commandFinish is FUN_00436c30 for paths that did not come through
// runCommand (DataServer replies, timers).
func (w *World) commandFinish(o *Object, c command) {
	m := mv(o)
	if m.CommandTicks == nil {
		m.CommandTicks = map[int]time.Time{}
	}
	w.commandDone(o, c)
}

// customNotice is FUN_004dc890: the custom C1 86 F3 E4 text with
// printf formatting.
func (w *World) customNotice(o *Object, typ, color byte, format string, args ...any) {
	text := format
	if len(args) > 0 || strings.Contains(format, "%%") {
		text = fmt.Sprintf(format, args...)
	}
	w.send(o, packetCustomNotice(typ, color, text))
}

// commandBusy is the test shared by /reset, /mreset, /rename and the auto
// reset: an open interface (obj+0x3d4 & 3), live state 0x20, dying
// (obj+0x286), teleporting (obj+0x284) or an open personal shop
// (obj+0x2468), or a Summon Party countdown (obj+0x5d2, skill_guild.go).
func (w *World) commandBusy(o *Object) bool {
	return ifBusy(o) || w.commandUnsettled(o) || pshopOpen(o) || w.summonCounting(o)
}

// summonCounting reports a running Summon Party countdown (obj+0x5d2 != 0).
func (w *World) summonCounting(o *Object) bool {
	if w.rq == nil {
		return false
	}
	s := w.rq.summons[o.Index]
	return s != nil && s.count != 0
}

// commandUnsettled is live state 0x20, dying or teleporting.
func (w *World) commandUnsettled(o *Object) bool {
	if o.Live == LiveTeleport || o.Teleporting {
		return true
	}
	if w.mon != nil {
		if d := w.mw().deaths[o.Index]; d != nil && d.Die != 0 {
			return true
		}
	}
	return false
}

// inSafeZone is the map attribute bit 1 at the player's position
// (FUN_004ad4d0(1)).
func (w *World) inSafeZone(o *Object) bool {
	return w.attrAt(o.Map, o.X, o.Y)&1 != 0
}

// --- /request (3) --------------------------------------------------------------------

// commandRequest is /request (FUN_00437070): "on" sets obj+0x434 bits 0..1
// and clears bit 2 (message 0x140), "off" clears bits 0..2 (0x141), "auto"
// sets bits 0..2 and keeps argument 1 as the auto-party password (0x142).
// The password field is cleared in all three cases; other words do
// nothing.
func (w *World) commandRequest(o *Object, c command, arg string) {
	ps := w.partyStateOf(o)
	var msg int
	switch commandToken(arg, 0, 5) {
	case "on":
		ps.Flags, ps.Password, msg = ps.Flags&^4|3, "", 0x140
	case "off":
		ps.Flags, ps.Password, msg = ps.Flags&^7, "", 0x141
	case "auto":
		ps.Flags, msg = ps.Flags|7, 0x142
		ps.Password = commandToken(arg, 1, 0xB)
	default:
		return
	}
	w.systemMessage(o, w.messageFor(o, msg))
	w.commandDone(o, c)
}

// --- /evo (12) -----------------------------------------------------------------------

// commandChange is /evo (FUN_00437d50): refused (message 0x14B) when the
// evolution level (ChangeUp, +1 for MG/DL/class 6) reached
// CommandChangeLimit_AL; otherwise every quest 0..3 (0..6 when the level
// is not 0) not yet completed is completed with its rewards (quest 3 only
// for the Dark Knight), the quest list is sent again and message 0x14C.
func (w *World) commandChange(o *Object, c command, arg string) {
	cu := changeUp(o)
	if o.Class == 3 || o.Class == 4 || o.Class == 6 {
		cu++
	}
	if w.cfg.ALInt("Command", "CommandChangeLimit", accountLevelOf(o), 0) <= cu {
		w.systemMessage(o, w.messageFor(o, 0x14B))
		return
	}
	last := 3
	if cu != 0 {
		last = 6
	}
	for q := 0; q <= last; q++ {
		if questState(o, q) == 2 || (q == 3 && o.Class != 1) {
			continue
		}
		setQuestState(o, q, 1)
		if w.quests != nil {
			w.giveRewards(o, q) // FUN_004df440
		}
		setQuestState(o, q, 2)
	}
	o.P.questSent = false // obj+0x5c0
	w.sendQuestList(o)
	w.systemMessage(o, w.messageFor(o, 0x14C))
	w.log.Info("[CommandChange]", "account", o.P.Account.Login, "name", o.P.Char.Name, "changeup", cu)
	w.commandDone(o, c)
}

// --- /bau (13 and hard-coded) --------------------------------------------------------

// wareNumberLimit is CommandWareNumber_AL0..3: FUN_004eeee0 reads the keys
// and then overwrites the four values with 20 (0x14).
const wareNumberLimit = 20

// clearWarehouse is the common part of both /bau variants: the vault in
// memory is dropped (obj+0x418 money, items, interface bits and obj+0x588)
// and the new number is kept in obj+0x614.
func (w *World) selectWarehouse(o *Object, n int) {
	st := items(o)
	st.WarehouseMoney = 0
	for i := range st.Warehouse {
		st.Warehouse[i] = item.Empty()
	}
	st.If = ifState{}
	st.WarehouseLoaded = false
	st.WarehouseNumber = n
}

// wareBusy is the check of both /bau variants (no obj+0x2468/0x5d2 test,
// but the vault must not be loaded, obj+0x588).
func (w *World) wareBusy(o *Object) bool {
	return ifBusy(o) || w.commandUnsettled(o) || items(o).WarehouseLoaded
}

// commandWare is /bau of the CommandManager table (FUN_00438110), only
// reachable when the table syntax differs from the hard-coded "/bau":
// 0 <= number < 20 selects the vault (message 0x14F), otherwise 0x14E;
// busy gives 0x14D.
func (w *World) commandWare(o *Object, c command, arg string) {
	if w.wareBusy(o) {
		w.systemMessage(o, w.messageFor(o, 0x14D))
		return
	}
	n := cmdInt(arg, 0)
	if n < 0 || n >= wareNumberLimit {
		w.systemMessage(o, w.messageFor(o, 0x14E))
		return
	}
	w.selectWarehouse(o, n)
	w.systemMessage(o, w.messageFor(o, 0x14F, n))
	w.log.Info("[CommandWare]", "account", o.P.Account.Login, "name", o.P.Char.Name, "number", n)
	w.commandDone(o, c)
}

// commandBau is the hard-coded "/bau N" of FUN_00435400, tested before the
// CommandManager table (so the table entry 13 never runs with the shipped
// CommandManager.txt): N = 1..20 selects vault N-1 and, in a safe zone,
// opens it at once (FUN_00517f70). No delay, level or zen checks.
func (w *World) commandBau(o *Object, arg string) {
	if w.wareBusy(o) {
		w.customNotice(o, 1, 2, "Feche a interface atual antes de trocar de bau.")
		return
	}
	n := cmdInt(arg, 0)
	if uint(n-1) > 0x13 {
		w.customNotice(o, 1, 2, "Uso correto: /bau 1 ate /bau 20")
		return
	}
	w.selectWarehouse(o, n-1)
	w.customNotice(o, 1, 0, "Bau %d selecionado.", n)
	if w.inSafeZone(o) {
		w.openWarehouse(o)
	}
}

// --- /reset (14) ---------------------------------------------------------------------

// resetRowFor is the CResetTable lookup (FUN_004e3eb0 / FUN_004e3f10):
// the first row whose range holds resets+1.
func (w *World) resetRowFor(n int) *resetTableRow {
	for i := range w.cmdWorld().resetTable {
		r := &w.cmdWorld().resetTable[i]
		if r.MinReset <= n && n <= r.MaxReset {
			return r
		}
	}
	return nil
}

// resetLevel is FUN_004e3eb0 (table Level_AL, else CommandResetLevel_AL).
func (w *World) resetLevel(o *Object) int {
	al := accountLevelOf(o)
	if r := w.resetRowFor(o.P.Char.Resets + 1); r != nil && r.Level[al] != -1 {
		return r.Level[al]
	}
	return w.cfg.ALInt("Command", "CommandResetLevel", al, 0)
}

// resetMoney is FUN_004e3f10 (table Money_AL, else CommandResetMoney_AL).
func (w *World) resetMoney(o *Object) int64 {
	al := accountLevelOf(o)
	if r := w.resetRowFor(o.P.Char.Resets + 1); r != nil && r.Money[al] != -1 {
		return int64(uint32(r.Money[al]))
	}
	return int64(uint32(w.cfg.ALInt("Command", "CommandResetMoney", al, 0)))
}

// resetPoints is FUN_004e3f70: the sum over every reset 1..n of the table
// Point_AL of that reset (CommandResetPoint_AL when no row or -1).
func (w *World) resetPoints(o *Object) int {
	al := accountLevelOf(o)
	def := w.cfg.ALInt("Command", "CommandResetPoint", al, 0)
	total := 0
	for n := 1; n <= o.P.Char.Resets; n++ {
		v := def
		if r := w.resetRowFor(n); r != nil && r.Point[al] != -1 {
			v = r.Point[al]
		}
		total += v
	}
	return total
}

// resetLimits is FUN_004e36c0 / 0x4e37b0 / 0x4e38a0 (ResetLimit.txt for
// resets+1, else CommandResetLimitDay/Wek/Mon_AL).
func (w *World) resetLimits(o *Object) (day, week, month int) {
	al := accountLevelOf(o)
	day = w.cfg.ALInt("Command", "CommandResetLimitDay", al, 0)
	week = w.cfg.ALInt("Command", "CommandResetLimitWek", al, 0)
	month = w.cfg.ALInt("Command", "CommandResetLimitMon", al, 0)
	n := o.P.Char.Resets + 1
	for _, r := range w.cmdWorld().resetLimit {
		if r.MinReset <= n && n <= r.MaxReset {
			if r.Day[al] != -1 {
				day = r.Day[al]
			}
			if r.Week[al] != -1 {
				week = r.Week[al]
			}
			if r.Month[al] != -1 {
				month = r.Month[al]
			}
			break
		}
	}
	return
}

// equipped reports an item in one of the 12 equipment slots
// (FUN_0048ad60 over slots 0..11).
func equipped(o *Object) bool {
	for s := 0; s < EquipSlots; s++ {
		if !o.P.Inventory[s].IsEmpty() {
			return true
		}
	}
	return false
}

// resetCheck is the request part shared by /reset (FUN_004382d0) and the
// auto reset (FUN_0043cf10): it returns the message (0 = ok) and its
// value.
func (w *World) resetCheck(o *Object) (msg int, val any) {
	ch, al := o.P.Char, accountLevelOf(o)
	switch {
	case w.commandBusy(o):
		return 0x150, nil
	case ch.Level < w.resetLevel(o):
		return 0x151, w.resetLevel(o)
	case uint32(ch.Money) < uint32(w.resetMoney(o)):
		return 0x152, w.resetMoney(o)
	case ch.Resets >= w.cfg.ALInt("Command", "CommandResetLimit", al, 0):
		return 0x156, w.cfg.ALInt("Command", "CommandResetLimit", al, 0)
	case w.cfg.ALInt("Command", "CommandResetCheckItem", al, 0) != 0 && equipped(o):
		return 0x157, nil
	}
	return 0, nil
}

// commandReset is /reset (FUN_004382d0): "auto" toggles the automatic
// reset (FUN_00438590); otherwise the checks run and the DataServer is
// asked for the day/week/month counters (DS 0x0F 00, obj+0x69c pending).
func (w *World) commandReset(o *Object, c command, arg string) {
	if commandToken(arg, 0, 5) == "auto" {
		w.commandResetAuto(o, arg)
		return
	}
	if w.ghrsBlocks(o) { // CastleSiege.exe GHRS cap, 0x42f79d (ghrs.go)
		return
	}
	if msg, val := w.resetCheck(o); msg != 0 {
		if val != nil {
			w.systemMessage(o, w.messageFor(o, msg, val))
		} else {
			w.systemMessage(o, w.messageFor(o, msg))
		}
		return
	}
	w.requestReset(o, c, false)
}

// commandResetAuto is FUN_00438590: CommandResetAutoEnable_AL == 0 gives
// 0x158; otherwise it toggles obj+0x654, keeping arguments 1..4 (and 5
// for the Dark Lord) as the /f /a /v /e /c auto amounts applied after each
// reset (0x159), or clearing them (0x15A). No delay or charge.
func (w *World) commandResetAuto(o *Object, arg string) {
	if w.cfg.ALInt("Command", "CommandResetAutoEnable", accountLevelOf(o), 0) == 0 {
		w.systemMessage(o, w.messageFor(o, 0x158))
		return
	}
	s := cmdOf(o)
	if !s.AutoReset {
		s.AutoReset = true
		for i := 0; i < 4; i++ {
			s.AutoPoints[i] = cmdInt(arg, i+1)
		}
		s.AutoPoints[4] = 0
		if o.Class == 4 {
			s.AutoPoints[4] = cmdInt(arg, 5)
		}
		w.systemMessage(o, w.messageFor(o, 0x159))
		return
	}
	s.AutoReset, s.AutoPoints = false, [5]int{}
	w.systemMessage(o, w.messageFor(o, 0x15A))
}

// autoResetSecond is the obj+0x654 part of FUN_00435180 (every second):
// players with the auto reset and no open interface run FUN_0043cf10, the
// silent version of the /reset checks.
func (w *World) autoResetSecond() {
	for i := PlayerStart; i < MaxObjects; i++ {
		o := w.objects[i]
		if !noticePlayer(o) || o.P.Char == nil || o.P.cmd == nil || !o.P.cmd.AutoReset || ifBusy(o) {
			continue
		}
		if msg, _ := w.resetCheck(o); msg == 0 && !w.ghrsBlocks(o) { // GHRS: 0x434470 (ghrs.go)
			w.requestReset(o, w.commandByIndex(14), false)
		}
	}
}

func (w *World) commandStore() store.CommandStore {
	s, _ := w.store.(store.CommandStore)
	return s
}

// requestReset sends DS 0x0F 00/01 (C1 1C 0F sub index account[10]
// name[10]) once: the reply runs resetReply / masterResetReply.
func (w *World) requestReset(o *Object, c command, master bool) {
	s := cmdOf(o)
	cs := w.commandStore()
	if s.ResetPending || cs == nil {
		return
	}
	s.ResetPending = true
	conn, ch := o.conn, o.P.Char
	acc, name := o.P.Account.Login, ch.Name
	w.async(func(ctx context.Context) func() {
		counts, err := cs.ResetInfo(ctx, acc, name, master)
		return func() {
			o := w.objectOf(conn)
			if o == nil || o.P == nil || o.P.Char != ch || o.State != StatePlaying {
				return // FUN_0050cd20: "Invalid Account"
			}
			cmdOf(o).ResetPending = false
			if err != nil {
				w.log.Error("reset info", "err", err)
				return
			}
			if master {
				w.masterResetReply(o, c, counts)
			} else {
				w.resetReply(o, c, counts)
			}
		}
	})
}

// saveResetInfo is FUN_00476a70 / FUN_00476b10 (WZ_SetResetInfo /
// WZ_SetMasterResetInfo).
func (w *World) saveResetInfo(o *Object, master bool, counts store.ResetCounts) {
	cs := w.commandStore()
	if cs == nil {
		return
	}
	acc, name := o.P.Account.Login, o.P.Char.Name
	reset, mreset := o.P.Char.Resets, o.P.Char.MasterResets
	w.async(func(ctx context.Context) func() {
		if err := cs.SetResetInfo(ctx, acc, name, master, reset, mreset, counts); err != nil {
			w.log.Error("save reset info", "err", err)
		}
		return nil
	})
}

// resetGate is the gate of the Move option (FUN_0050f6d0): Lorencia
// (17) for classes 0, 1, 3, 4 and 6, Noria (27) for the elf, 267 for
// class 5.
func resetGate(class int) int {
	switch class {
	case 0, 1, 3, 4, 6:
		return 0x11
	case 2:
		return 0x1B
	case 5:
		return 0x10B
	}
	return -1
}

// clearQuestsForReset is the ClearQuest option of both replies: the class
// byte loses its evolution, the A3 reward 0xC9 carries the evolution byte
// (evo is the value of obj+0xe4 the original uses: 0 after /reset, the
// old one after /mreset), the quest bytes become 0xFF and the list is
// sent (only if not sent yet, FUN_004dde70).
func (w *World) clearQuestsForReset(o *Object, evo byte) {
	ch := o.P.Char
	ch.Class &= 0xF0
	w.rebuildCharSet(o) // FUN_004c6ba0
	w.sendQuestReward(o, 0xC9, int(evo))
	for i := range ch.Quest {
		ch.Quest[i] = 0xFF
	}
	w.sendQuestList(o)
}

// clearSkillsForReset is the ClearSkill option: the 60 skills are emptied,
// the class skills (FUN_004cc730 list) and the skill of every equipped
// item with the skill option are learned again and F3 11 is sent.
func (w *World) clearSkillsForReset(o *Object) {
	for i := range o.P.Skills {
		o.P.Skills[i] = Skill{Index: -1}
	}
	for _, s := range classDefaultSkills(o.Class, o.P.Char.Quest) {
		w.addSkill(o, s, 0)
	}
	for s := 0; s < EquipSlots; s++ {
		it := o.P.Inventory[s]
		if !it.IsEmpty() && it.Skill {
			if sk := w.itemSkill(it.Index); sk > 0 {
				w.addSkill(o, sk, byte(it.Level))
			}
		}
	}
	w.send(o, packetSkills(o.P.Skills[:]))
	w.calcCharacter(o)
}

// resetBaseStats keeps or restores the stats (Keep* = 0 restores the
// DefaultClassInfo value).
func (w *World) resetBaseStats(o *Object, prefix string) {
	ch := o.P.Char
	var ci struct{ Str, Dex, Vit, Ene, Lead int }
	if w.statsData != nil {
		if d, ok := w.statsData.classes[o.Class]; ok {
			ci.Str, ci.Dex, ci.Vit, ci.Ene, ci.Lead = d.Strength, d.Dexterity, d.Vitality, d.Energy, d.Lead
		}
	}
	keep := func(key string, v *int, def int) {
		if w.cfg.Int("Command", prefix+key, 0) == 0 {
			*v = def
		}
	}
	keep("KeepStrength", &ch.Strength, ci.Str)
	keep("KeepDexterity", &ch.Dexterity, ci.Dex)
	keep("KeepVitality", &ch.Vitality, ci.Vit)
	keep("KeepEnergy", &ch.Energy, ci.Ene)
	keep("KeepLeadership", &ch.Leadership, ci.Lead)
}

// levelPoints is the part common to both point formulas: (level-1) x
// <Class>LevelUpPoint_AL + quest points (FUN_004dd790) + the hero bonus
// (quest 2 done and level > PlusStatMinLevel) + FruitAddPoint.
func (w *World) levelPoints(o *Object) int {
	ch := o.P.Char
	plus := 0
	if len(ch.Quest) > 0 && ch.Quest[0]&0x30 == 0x20 {
		if minLv := w.cfg.Int("Common", "PlusStatMinLevel", 0); minLv < ch.Level {
			plus = w.cfg.Int("Common", "PlusStatPoint", 0) * (ch.Level - minLv)
		}
	}
	quest := 0
	if w.quests != nil {
		quest = w.questLevelUpPoints(o)
	}
	return (ch.Level-1)*w.levelUpPoints(o) + quest + plus + ch.FruitAddPoints
}

// pointRate is Command(Master)ResetPointRate<Class>; classes 5 and 6 read
// the next two integers of CServerInfo like the original.
func (w *World) pointRate(o *Object, master bool) int {
	keys := []string{"DW", "DK", "FE", "MG", "DL"}
	if o.Class >= 0 && o.Class < len(keys) {
		if master {
			return w.cfg.Int("Command", "CommandMasterResetPointRate"+keys[o.Class], 0)
		}
		return w.cfg.Int("Command", "CommandResetPointRate"+keys[o.Class], 0)
	}
	if !master {
		switch o.Class {
		case 5:
			return w.cfg.Int("Command", "CommandMasterResetType", 0)
		case 6:
			return w.cfg.Int("Command", "CommandMasterResetPointType", 0)
		}
	}
	return 0
}

// resetReply is the DS 0x0F 00 reply (FUN_0043d080).
func (w *World) resetReply(o *Object, c command, counts store.ResetCounts) {
	ch, al := o.P.Char, accountLevelOf(o)
	day, week, month := w.resetLimits(o)
	switch {
	case uint32(day) <= uint32(counts.Day):
		w.systemMessage(o, w.messageFor(o, 0x15B, day))
		return
	case uint32(week) <= uint32(counts.Week):
		w.systemMessage(o, w.messageFor(o, 0x15C, week))
		return
	case uint32(month) <= uint32(counts.Month):
		w.systemMessage(o, w.messageFor(o, 0x15D, month))
		return
	}
	if w.hooks.commandEventBlock != nil && w.hooks.commandEventBlock(w, o) {
		w.systemMessage(o, "Proibido usar o comando no evento.")
		return
	}
	ch.Money -= w.resetMoney(o)
	w.sendMoney(o)
	w.commandFinish(o, c)
	if start := w.cfg.ALInt("Command", "CommandResetStartLevel", al, 0); start == -1 {
		ch.Level = int(int16(ch.Level - w.cfg.ALInt("Command", "CommandResetLevel", al, 0)))
	} else {
		ch.Level = int(int16(start))
	}
	ch.Level = max(ch.Level, 1)
	ch.Experience = int64(w.data.Experience.Floor(ch.Level))
	n := w.cfg.ALInt("Command", "CommandResetCount", al, 0)
	ch.Resets += n
	counts.Day, counts.Week, counts.Month = counts.Day+n, counts.Week+n, counts.Month+n
	o.P.autoPoints = nil // obj+0x63c..0x650
	if w.cfg.ALInt("Command", "CommandResetClearQuest", al, 0) != 0 {
		w.clearQuestsForReset(o, evolutionByteOf(o.Class, 0))
	}
	if w.cfg.ALInt("Command", "CommandResetClearSkill", al, 0) != 0 {
		w.clearSkillsForReset(o)
	}
	if w.cfg.ALInt("Command", "CommandResetClearParty", al, 0) != 0 {
		if pt := w.partyOf(o); pt != nil {
			w.partyDestroy(pt) // FUN_004d2500
		}
	}
	if w.cfg.Int("Command", "CommandResetType", 0) == 1 {
		pts := w.pointRate(o, false)*w.resetPoints(o)/100 + w.levelPoints(o)
		if ch.MasterResets > 0 {
			pts += w.cfg.ALInt("Command", "CommandMasterResetPoint", al, 0) * ch.MasterResets
		}
		ch.LevelUpPoints = pts
		w.resetBaseStats(o, "CommandReset")
	}
	w.calcCharacter(o)
	w.send(o, w.packetCharRefresh(o))
	w.saveCharacter(o, nil) // FUN_00475c30
	w.saveResetInfo(o, false, counts)
	w.ghrsResetDone(o) // CastleSiege.exe 0x4357a3 (ghrs.go)
	w.systemMessage(o, w.messageFor(o, 0x15E, ch.Resets))
	if w.cfg.ALInt("Command", "CommandResetMove", al, 0) != 0 {
		if g := resetGate(o.Class); g >= 0 {
			w.gateMove(o, g)
		}
	}
	// The auto reset amounts become the /f../c auto amounts.
	s := cmdOf(o)
	a := autoPoints(o)
	a.Amount = s.AutoPoints
	a.recount()
	if ch.LevelUpPoints != 0 && a.Count > 0 {
		w.spreadAutoPoints(o)
	}
	if w.hooks.itemBag != nil {
		w.hooks.itemBag(w, o, 0x17, -1, o.Map, o.X, o.Y)
	}
	w.log.Info("[CommandReset]", "account", o.P.Account.Login, "name", ch.Name, "reset", ch.Resets)
}

// evolutionByteOf is the A3 value (ChangeUp<<4) + (Class<<5) -
// (ChangeUp>>1) for an explicit ChangeUp.
func evolutionByteOf(class, cu int) byte {
	c := byte(cu) << 4
	return c + (byte(class)<<5 - c>>5)
}

// --- /mreset (15) --------------------------------------------------------------------

// commandMasterReset is /mreset (FUN_00438730): level < MasterResetLevel
// (0x160), resets < MasterResetReset (0x161), zen < MasterResetMoney
// (0x162), master resets >= MasterResetLimit (0x166) and CheckItem (0x167);
// the Coin1..3 costs (0x163..0x165) have no configuration key and are 0.
// Then DS 0x0F 01.
func (w *World) commandMasterReset(o *Object, c command, arg string) {
	ch, al := o.P.Char, accountLevelOf(o)
	key := func(k string) int { return w.cfg.ALInt("Command", "CommandMasterReset"+k, al, 0) }
	msg := func(id int, v ...any) { w.systemMessage(o, w.messageFor(o, id, v...)) }
	switch {
	case w.commandBusy(o):
		msg(0x15F)
	case ch.Level < key("Level"):
		msg(0x160, key("Level"))
	case ch.Resets < key("Reset"):
		msg(0x161, key("Reset"))
	case uint32(ch.Money) < uint32(key("Money")):
		msg(0x162, key("Money"))
	case ch.MasterResets >= key("Limit"):
		msg(0x166, key("Limit"))
	case key("CheckItem") != 0 && equipped(o):
		msg(0x167)
	default:
		w.requestReset(o, c, true)
	}
}

// masterResetReply is the DS 0x0F 01 reply (FUN_0043dad0).
func (w *World) masterResetReply(o *Object, c command, counts store.ResetCounts) {
	ch, al := o.P.Char, accountLevelOf(o)
	key := func(k string) int { return w.cfg.ALInt("Command", "CommandMasterReset"+k, al, 0) }
	switch {
	case uint32(key("LimitDay")) <= uint32(counts.Day):
		w.systemMessage(o, w.messageFor(o, 0x168, key("LimitDay")))
		return
	case uint32(key("LimitWek")) <= uint32(counts.Week):
		w.systemMessage(o, w.messageFor(o, 0x169, key("LimitWek")))
		return
	case uint32(key("LimitMon")) <= uint32(counts.Month):
		w.systemMessage(o, w.messageFor(o, 0x16A, key("LimitMon")))
		return
	}
	if w.hooks.commandEventBlock != nil && w.hooks.commandEventBlock(w, o) {
		w.systemMessage(o, "Proibido usar o comando no evento.")
		return
	}
	ch.Money -= int64(uint32(key("Money")))
	w.sendMoney(o)
	w.commandFinish(o, c)
	if start := key("StartLevel"); start == -1 {
		ch.Level = int(int16(ch.Level - key("Level")))
	} else {
		ch.Level = int(int16(start))
	}
	ch.Level = max(ch.Level, 1)
	ch.Experience = int64(w.data.Experience.Floor(ch.Level))
	if start := key("StartReset"); start == -1 {
		ch.Resets -= key("Reset")
	} else {
		ch.Resets = start
	}
	ch.Resets = max(ch.Resets, 0)
	n := key("Count")
	ch.MasterResets += n
	counts.Day, counts.Week, counts.Month = counts.Day+n, counts.Week+n, counts.Month+n
	if key("ClearQuest") != 0 {
		// The original clears only the class byte (obj+0xe2): obj+0xe4
		// keeps the old evolution, which goes in the A3 value.
		w.clearQuestsForReset(o, evolutionByteOf(o.Class, changeUp(o)))
	}
	if key("ClearSkill") != 0 {
		w.clearSkillsForReset(o)
	}
	// CommandMasterResetClearParty_AL1..3 are read from the _AL0 key.
	if w.cfg.ALInt("Command", "CommandMasterResetClearParty", 0, 0) != 0 {
		if pt := w.partyOf(o); pt != nil {
			w.partyDestroy(pt)
		}
	}
	if w.cfg.Int("Command", "CommandMasterResetType", 0) == 1 {
		pts := 0
		switch w.cfg.Int("Command", "CommandMasterResetPointType", 0) {
		case 0:
			pts = w.pointRate(o, true)*key("Point")*ch.MasterResets/100 + w.levelPoints(o)
		case 1:
			pts = key("Point") * ch.MasterResets
		case 2:
			pts = key("Point")
		}
		ch.LevelUpPoints = pts
		w.resetBaseStats(o, "CommandMasterReset")
	}
	w.calcCharacter(o)
	w.send(o, w.packetCharRefresh(o))
	w.saveCharacter(o, nil) // FUN_00475c30
	w.saveResetInfo(o, true, counts)
	w.systemMessage(o, w.messageFor(o, 0x16B, ch.MasterResets))
	if key("Move") != 0 {
		if g := resetGate(o.Class); g >= 0 {
			w.gateMove(o, g)
		}
	}
	if w.hooks.itemBag != nil {
		w.hooks.itemBag(w, o, 0x16, -1, o.Map, o.X, o.Y)
	}
	w.log.Info("[CommandMasterReset]", "account", o.P.Account.Login, "name", ch.Name, "mreset", ch.MasterResets)
}

// --- /rename (19) --------------------------------------------------------------------

// validRename is FUN_00513850: no space, double quote, percent sign,
// apostrophe, slash, backslash or underscore.
func validRename(s string) bool { return !strings.ContainsAny(s, " \"%'/\\_") }

// commandRename is /rename (FUN_00439040): a guild member gets 0x2C8; the
// new name (argument 0, up to 10 characters) must pass FUN_00513850
// (0x2D3), have 4+ characters (0x2B1), the player must stand in a safe
// zone (0x2B2) and be free (0x2B3). Then DS 0x0F 04.
func (w *World) commandRename(o *Object, c command, arg string) {
	msg := func(id int) { w.systemMessage(o, w.messageFor(o, id)) }
	name := commandToken(arg, 0, 0xB)
	switch {
	case guildNumberOf(o) != 0:
		msg(0x2C8)
		return
	case !validRename(name):
		msg(0x2D3)
		return
	case len(name) < 4:
		msg(0x2B1)
		return
	case !w.inSafeZone(o):
		msg(0x2B2)
		return
	case w.commandBusy(o):
		msg(0x2B3)
		return
	}
	cs := w.commandStore()
	if cs == nil {
		return
	}
	conn, ch := o.conn, o.P.Char
	accID, acc, old := o.P.Account.ID, o.P.Account.Login, ch.Name
	w.async(func(ctx context.Context) func() {
		ok, err := cs.RenameCharacter(ctx, accID, acc, old, name)
		return func() {
			o := w.objectOf(conn)
			if o == nil || o.P == nil || o.P.Char != ch {
				return
			}
			w.renameReply(o, c, ok, err, old, name)
		}
	})
}

// renameReply is the DS 0x0F 04 reply (FUN_0043e540): result 0 gives
// 0x2B5 (name in use), 2 gives 0x2B4 (error); otherwise the name changes,
// message 0x2B6, the command is charged and the player goes back to the
// character list after 6 seconds (obj+0x0a = 0x106).
func (w *World) renameReply(o *Object, c command, ok bool, err error, old, name string) {
	if err != nil {
		w.log.Error("rename", "err", err)
		w.systemMessage(o, w.messageFor(o, 0x2B4))
		return
	}
	if !ok {
		w.systemMessage(o, w.messageFor(o, 0x2B5))
		return
	}
	// The online-name map (DS 0x70/0x71) follows the new name so that the
	// logout removes it (the original keeps the old entry).
	if w.hub != nil && w.hub.Leave(old, w.cfg.ServerCode, o.Index) {
		w.hub.Enter(OnlineChar{Name: name, Account: o.P.Account.Login, ServerCode: w.cfg.ServerCode, Index: o.Index})
	}
	o.P.Char.Name = name
	w.saveCharacter(o, nil) // FUN_00475c30
	w.commandFinish(o, c)
	w.systemMessage(o, w.messageFor(o, 0x2B6))
	if o.P.LogoutCountdown < 1 {
		o.P.LogoutCountdown, o.P.LogoutType = 6, 1
	}
	w.log.Info("[CommandRename]", "account", o.P.Account.Login, "old", old, "name", name)
}

// --- /dcfriend (20) ------------------------------------------------------------------

// commandDCFriend is /dcfriend name password (FUN_004392c0): the playing
// character (FUN_0050c6d0) whose account password matches gets the 6
// second "close game" countdown (FUN_005102a0: obj+0x0a = 6); messages
// 0x2B8 not found, 0x2B9 wrong password, 0x2BA done.
func (w *World) commandDCFriend(o *Object, c command, arg string) {
	t := w.findPlayer(commandToken(arg, 0, 0xB))
	if t == nil {
		w.systemMessage(o, w.messageFor(o, 0x2B8))
		return
	}
	if !w.accountPasswordMatches(t, commandToken(arg, 1, 0xB)) {
		w.systemMessage(o, w.messageFor(o, 0x2B9))
		return
	}
	tAccount, tName := t.P.Account.Login, t.P.Char.Name
	// FUN_005102a0: the offline modes (obj+0x20d9, 0x2110, 0x249e) close
	// the object at once (offline.go).
	if !w.offlineKick(t) && t.P.LogoutCountdown < 1 {
		t.P.LogoutCountdown, t.P.LogoutType = 6, 0
	}
	w.systemMessage(o, w.messageFor(o, 0x2BA))
	w.log.Info("[CommandDCFriend]", "account", o.P.Account.Login, "name", o.P.Char.Name,
		"target_account", tAccount, "target", tName)
	w.commandDone(o, c)
}

// accountPasswordMatches compares with the login password (obj+0x85, 10
// characters, strcmp). The rewrite keeps only the hash, so it is checked
// with the auth module.
func (w *World) accountPasswordMatches(o *Object, pw string) bool {
	if pw == "" {
		return false
	}
	ok, _, err := auth.CheckPassword(o.P.Account.PasswordHash, pw)
	return err == nil && ok
}

// --- /open (22) ----------------------------------------------------------------------

// commandOpenWare is /open (FUN_0043e990): without an open interface, in
// a safe zone (otherwise the custom notice "Utilize o Comando em Zona
// Segura"), it opens the vault like the Safety Guardian (FUN_00517f70).
func (w *World) commandOpenWare(o *Object, c command, arg string) {
	st := items(o)
	if st.If.Use {
		return
	}
	if !w.inSafeZone(o) {
		w.customNotice(o, 2, 6, "Utilize o Comando em Zona Segura")
		return
	}
	switch st.If.Type {
	case 7, 1, 8, 6:
		return
	}
	w.openWarehouse(o)
	w.commandDone(o, c)
}

// --- /classe (23) --------------------------------------------------------------------

// commandChangeClass is /classe dw|dk|elf|mg|dl (FUN_0043ea60): a missing
// or unknown word gives 0x38A, the current class 0x38B and any equipped
// item (slots 0..10) 0x38C. Choosing "dl" sets the leadership to 15 as
// soon as the word matches. Without an open interface the class byte
// becomes Class<<4 (no evolution), the A3 0xC9 reward and the quest reset
// follow, every skill is forgotten, the timed effects removed and the
// player goes back to the character list one second later (obj+0x0a =
// 0x101).
func (w *World) commandChangeClass(o *Object, c command, arg string) {
	msg := func(id int) { w.systemMessage(o, w.messageFor(o, id)) }
	word := commandToken(arg, 0, 4)
	cls := -1
	switch word {
	case "dw":
		cls = 0
	case "dk":
		cls = 1
	case "elf":
		cls = 2
	case "mg":
		cls = 3
	case "dl":
		cls = 4
	}
	if cls < 0 {
		msg(0x38A)
		return
	}
	if o.Class == cls {
		msg(0x38B)
		return
	}
	if cls == 4 {
		o.P.Char.Leadership = 15 // obj+0x10c
	}
	for s := 0; s < 11; s++ {
		if !o.P.Inventory[s].IsEmpty() {
			msg(0x38C)
			return
		}
	}
	if ifBusy(o) {
		return
	}
	ch := o.P.Char
	o.Class = cls
	ch.Class = byte(cls << 4)
	w.rebuildCharSet(o)
	w.sendQuestReward(o, 0xC9, int(evolutionByteOf(cls, 0)))
	for i := range ch.Quest {
		ch.Quest[i] = 0xFF
	}
	w.sendQuestList(o)
	for i := range o.P.Skills {
		o.P.Skills[i] = Skill{Index: -1}
	}
	w.clearEffects(o) // FUN_0047a540
	o.P.LogoutCountdown, o.P.LogoutType = 1, 1
	w.commandDone(o, c)
	w.log.Info("[CommandChangeClass]", "account", o.P.Account.Login, "name", ch.Name, "class", cls<<4)
}

// --- /ezen (24) ----------------------------------------------------------------------

// ezenItem is the item /ezen creates, (14,12).
const ezenItem = 14*512 + 12

// commandEZen is /ezen (FUN_0043f050): without bag room for (14,12) the
// notice 0x385; with less zen than eZentaxa (Custom.dat) 0x386; otherwise
// the item is created (FUN_004753e0), eZentaxa is paid (C3 10 22 FE) and
// 0x387. The notices are C1 86 F3 E4 type 2 color 2. There is no delay or
// charge.
func (w *World) commandEZen(o *Object, c command, arg string) {
	note := func(id int) { w.send(o, packetCustomNotice(2, 2, w.messageFor(o, id))) }
	if w.findBagSlot(&o.P.Inventory, ezenItem) < 0 {
		note(0x385)
		return
	}
	price := uint32(w.cfg.Int("Custom", "eZentaxa", 0))
	ch := o.P.Char
	if uint32(ch.Money) < price {
		note(0x386)
		return
	}
	w.createInventoryItem(o, item.Item{Index: ezenItem})
	ch.Money = int64(uint32(ch.Money) - price)
	w.sendMoney(o)
	note(0x387)
}

// --- /autopotion (25) ----------------------------------------------------------------

// commandAutoPotion is /autopotion (FUN_0043f240): only without an open
// interface, teleport or death; toggles obj+0x24ec (clearing the potion
// timer obj+0x24ac). Turning it on is charged.
func (w *World) commandAutoPotion(o *Object, c command, arg string) {
	if ifBusy(o) || o.Teleporting || w.dying(o) {
		return
	}
	s := cmdOf(o)
	s.PotionTime = time.Time{}
	if !s.AutoPotion {
		s.AutoPotion = true
		w.customNotice(o, 2, 2, "[AutoPotion] Ativado - HP 50%% / Mana 30%%")
		w.commandDone(o, c)
		return
	}
	s.AutoPotion = false
	w.customNotice(o, 2, 6, "[AutoPotion] Desativado")
}

func (w *World) dying(o *Object) bool {
	if w.mon == nil {
		return false
	}
	d := w.mw().deaths[o.Index]
	return d != nil && d.Die != 0
}

// potionCooldown is the 600 ms of FUN_00448980.
const potionCooldown = 600 * time.Millisecond

// autoPotionSecond is CAUTOHP (FUN_00448c80, 1-second event timer): for
// every playing player, FUN_004489a0 (life <= 50%: large, medium, small
// healing potion), FUN_00448ab0 (mana <= 30%: mana potions) and
// FUN_00448bc0 (poison effect: antidote) use the first matching item of
// slots 0..75 through the 26 handler (C1 06 26 slot FF 00), at most one
// use per 600 ms.
func (w *World) autoPotionSecond() {
	for i := PlayerStart; i < MaxObjects; i++ {
		o := w.objects[i]
		if !noticePlayer(o) || o.P.Char == nil || o.P.cmd == nil || !o.P.cmd.AutoPotion {
			continue
		}
		w.autoPotion(o)
	}
}

func (w *World) potionReady(o *Object) bool {
	t := cmdOf(o).PotionTime
	return t.IsZero() || !w.now().Before(t)
}

// autoUsePotion finds the first of the indexes in slots 0..75 (FUN_00494880)
// and uses it through FUN_00499a20.
func (w *World) autoUsePotion(o *Object, indexes ...int) {
	for _, idx := range indexes {
		for s := 0; s < BagEnd; s++ {
			if it := o.P.Inventory[s]; !it.IsEmpty() && it.Index == idx {
				w.handleUseItem(o, []byte{0xC1, 0x06, 0x26, byte(s), 0xFF, 0x00})
				cmdOf(o).PotionTime = w.now().Add(potionCooldown)
				return
			}
		}
	}
}

func (w *World) autoPotion(o *Object) {
	ch := o.P.Char
	alive := o.Alive && !w.dying(o)
	if alive && w.potionReady(o) {
		if top := float32(o.P.AddLife) + ch.MaxLife; top > 0 && ch.Life*100/top <= 50 {
			w.autoUsePotion(o, 14*512+3, 14*512+2, 14*512+1)
		}
	}
	if alive && w.potionReady(o) {
		if top := float32(o.P.AddMana) + ch.MaxMana; top > 0 && ch.Mana*100/top <= 30 {
			w.autoUsePotion(o, 14*512+6, 14*512+5, 14*512+4)
		}
	}
	// FUN_00448bc0: effect 0x37 (or a second effect passed in a register,
	// U-CUST-09) makes it drink an antidote (14,8).
	if hasEffectIndex(o, 0x37) && w.potionReady(o) {
		w.autoUsePotion(o, 14*512+8)
	}
}

// --- /lock (26), /unlock (27) -------------------------------------------------------

// lockMax is the largest password /lock accepts (99,999,999).
const lockMax = 99999999

// commandLock is /lock N (FUN_0043f2c0): with no lock (obj+0x24fc < 1) and
// 0 < N <= 99,999,999 the account is locked with N (message 0x6ED, JS
// 0x12 saves it); N too large gives 0x6EE, an existing lock 0x6EF. The
// lock blocks the item actions that test obj+0x24fc (message 0x6F2).
// There is no delay or charge.
func (w *World) commandLock(o *Object, c command, arg string) {
	if o.P.Account.Lock >= 1 {
		w.systemMessage(o, w.messageFor(o, 0x6EF))
		return
	}
	n := cmdInt(arg, 0)
	if n <= 0 {
		return
	}
	if n > lockMax {
		w.systemMessage(o, w.messageFor(o, 0x6EE))
		return
	}
	o.P.Account.Lock = n
	w.systemMessage(o, w.messageFor(o, 0x6ED))
	w.log.Info("[CommandLock]", "account", o.P.Account.Login, "name", o.P.Char.Name)
	w.saveAccountLock(o, n)
}

// commandUnlock is /unlock N (FUN_0043f450): only when locked; 0 < N <
// 100,000,000 equal to the lock clears it (0x6F0, JS 0x12 with 0), a
// different N gives 0x6F1 and a larger one 0x6EE.
func (w *World) commandUnlock(o *Object, c command, arg string) {
	if o.P.Account.Lock < 1 {
		return
	}
	n := cmdInt(arg, 0)
	switch {
	case n <= 0:
		return
	case n > lockMax:
		w.systemMessage(o, w.messageFor(o, 0x6EE))
		return
	case n != o.P.Account.Lock:
		w.systemMessage(o, w.messageFor(o, 0x6F1))
		return
	}
	o.P.Account.Lock = 0
	w.systemMessage(o, w.messageFor(o, 0x6F0))
	w.log.Info("[CommandUnLock]", "account", o.P.Account.Login, "name", o.P.Char.Name)
	w.saveAccountLock(o, 0)
}

// saveAccountLock is FUN_004a2b80 (JS 0x12: C1 18 12 index account[10]
// lock).
func (w *World) saveAccountLock(o *Object, lock int) {
	id := o.P.Account.ID
	w.async(func(ctx context.Context) func() {
		if err := w.store.SetAccountLock(ctx, id, lock); err != nil {
			w.log.Error("set lock", "err", err)
		}
		return nil
	})
}
