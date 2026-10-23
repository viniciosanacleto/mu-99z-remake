package game

import (
	"context"
	"encoding/binary"
	"testing"
	"time"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/item"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/store"
)

// The shipped files: no freebie items/effects, 3000 points per class, no
// CommandRequirement / BlackList / HackSkillSpeedCheck /
// MonsterSchedule rows, Marlon (229) in MonsterMove, GHRS only on the
// CastleSiege server.
func TestDataUsesShippedFiles(t *testing.T) {
	b := newBareWorld(t)
	d := b.dus()
	if d.freebies.Points[0] != 3000 || d.freebies.Money[4] != 0 || len(d.freebies.Items) != 0 {
		t.Fatalf("freebies = %+v", d.freebies)
	}
	if len(d.requirements) != 0 || len(d.blackIPs) != 0 || len(d.blackHWIDs) != 0 || len(d.speedRules) != 0 ||
		d.ghrs != nil {
		t.Fatal("unexpected rows")
	}
	// CustomEvolution.txt ships one row: Kris (0,0) becomes Short Sword (0,1).
	if len(d.evolution) != 1 || d.evolution[0] != (gamedata.EvolutionRow{Index: 1, Target: 0, Result: 1}) {
		t.Fatalf("evolution = %+v", d.evolution)
	}
	if len(d.moves) != 1 || d.moves[0].Class != 229 || len(d.moves[0].Rows) != 4 || d.moves[0].Index != -1 {
		t.Fatalf("moves = %+v", d.moves)
	}
	for i, g := range d.schedule {
		if g.State != scheduleOff {
			t.Fatalf("group %d state %d", i, g.State)
		}
	}
}

// --- DefaultClassFreebies (FUN_0046d0b0) ----------------------------------------

func TestFreebies(t *testing.T) {
	b := newBareWorld(t)
	o := b.player(PlayerStart, "Fresh", 0, 130, 130)
	o.Class = 1
	o.P.Char.Money, o.P.Char.LevelUpPoints = 1999999000, 5
	b.dus().freebies = &gamedata.ClassFreebies{
		Money: map[int]int{1: 5000}, Points: map[int]int{1: 300},
		Items: []gamedata.FreebieItem{
			{Class: 1, Index: 12*512 + 15, Level: 0},
			{Class: 0, Index: 14*512 + 13}, // another class
		},
		Effects: []gamedata.FreebieEffect{{Class: 1, Effect: 1, Type: 0, Duration: 900}},
	}
	b.giveFreebies(o)
	b.runAsync()
	pk := b.drain(o)
	if m := find(pk, 0x22, 0xFE); m == nil || binary.BigEndian.Uint32(m[4:]) != freebiesMaxMoney ||
		o.P.Char.Money != freebiesMaxMoney {
		t.Fatalf("money %d, packet % X", o.P.Char.Money, m)
	}
	if o.P.Char.LevelUpPoints != 305 || find(pk, 0xF3, 0xE0) == nil {
		t.Fatalf("points %d", o.P.Char.LevelUpPoints)
	}
	if b.countItems(o, 12*512+15, 0) != 1 || b.countItems(o, 14*512+13, -1) != 0 {
		t.Fatal("freebie items")
	}
	if !hasEffectIndex(o, 1) {
		t.Fatal("freebie effect")
	}
}

// enterWorld runs the freebies only when IsNewChar was set, and clears it.
func TestFreebiesOnlyForNewCharacters(t *testing.T) {
	b := newBareWorld(t)
	calls := 0
	old := newCharacterHook
	newCharacterHook = func(*World, *Object) { calls++ }
	defer func() { newCharacterHook = old }()
	for _, isNew := range []bool{true, false} {
		o := b.player(PlayerStart, "Enter", 0, 130, 130)
		ch := &store.Character{Name: "Enter", Class: 0x10, Level: 1, Map: 0, X: 130, Y: 130, IsNew: isNew}
		b.enterWorld(o, ch)
		if ch.IsNew {
			t.Fatal("IsNew not cleared")
		}
		b.leaveWorld(o)
	}
	if calls != 1 {
		t.Fatalf("hook ran %d times", calls)
	}
}

// --- CommandRequirement (FUN_00440660) ---------------------------------------------

func anyRequirement(index int) gamedata.CommandRequirement {
	return gamedata.CommandRequirement{Index: index, AccountLevel: -1, MinLevel: -1, MaxLevel: -1, MinReset: -1,
		MaxReset: -1, CheckItem: -1, ReqItem: -1, ReqItemCount: -1, ReqItemLevel: -1, ReqEffect: -1, ReqQuestIndex: -1,
		ReqQuestState: -1, ReqPKLevel: -1, ReqPKCount: -1, ReqGuildRnk: -1, Map: -1, StartX: -1, StartY: -1, EndX: -1,
		EndY: -1, DayOfWeek: -1, MinHour: -1, MaxHour: -1, FailMessage: 0x12E, Class: [5]int{1, 1, 1, 1, 1}}
}

func TestCommandRequirementChecks(t *testing.T) {
	b := newBareWorld(t)
	o := b.player(PlayerStart, "Req", 0, 130, 130)
	o.Class = 1
	check := func(name string, r gamedata.CommandRequirement, want bool) {
		t.Helper()
		b.dus().requirements = []gamedata.CommandRequirement{r}
		if got := b.commandRequirementFails(o, 5); got != want {
			t.Errorf("%s: fails = %v, want %v", name, got, want)
		}
		b.drain(o)
	}
	r := anyRequirement(5)
	check("no condition", r, false)
	r.Index = 6
	r.Map = 3
	check("other command", r, false)

	r = anyRequirement(5)
	r.Map = 3
	check("map", r, true)
	r.MinLevel = 401 // the row no longer applies to a level 400 character
	check("filtered by level", r, false)

	r = anyRequirement(5)
	r.Class = [5]int{1, 0, 1, 1, 1}
	r.Map = 3
	check("class column 0 skips", r, false)
	r.Class[1] = 2 // <= ChangeUp+1 = 1 fails the filter
	check("class column above ChangeUp+1", r, false)
	o.P.Char.Class = 0x11
	check("class column with ChangeUp", r, true)

	r = anyRequirement(5)
	r.CheckItem = 1
	check("no equipment", r, false)
	o.P.Inventory[2] = newItem(7*512+1, 0)
	check("equipment", r, true)
	o.P.Inventory[2] = item.Empty()

	r = anyRequirement(5)
	r.ReqItem, r.ReqItemCount, r.ReqItemLevel = 14*512+13, 2, 0
	give(t, b, o, 14*512+13, 0, 1)
	check("one of two items", r, true)
	give(t, b, o, 14*512+13, 0, 1)
	check("two items", r, false)

	r = anyRequirement(5)
	r.ReqEffect = 1
	check("effect missing", r, true)

	r = anyRequirement(5)
	r.ReqQuestIndex, r.ReqQuestState = 0, 2
	o.P.Char.Quest = []byte{0xFE} // quest 0 state 2
	check("quest state", r, false)
	r.ReqQuestState = 1
	check("quest other state", r, true)

	r = anyRequirement(5)
	r.ReqPKLevel = 3
	o.PKLevel = 3
	check("pk level", r, false)
	r.ReqPKCount = 2
	o.P.Char.PKCount = 1
	check("pk count", r, true)

	r = anyRequirement(5)
	r.ReqGuildRnk = 0x80
	check("guild rank", r, true)

	r = anyRequirement(5)
	r.StartX, r.StartY, r.EndX, r.EndY = 120, 120, 140, 140
	check("inside area", r, false)
	o.X = 141
	check("outside area", r, true)
	o.X = 130

	// 2026-01-01 00:00 UTC is a Thursday: wDayOfWeek 4 -> 5.
	r = anyRequirement(5)
	r.DayOfWeek = 5
	check("day of week", r, false)
	r.DayOfWeek = 1
	check("other day", r, true)
	r = anyRequirement(5)
	r.MinHour, r.MaxHour = 1, 23
	check("before MinHour", r, true)
	b.advance(23*time.Hour + 30*time.Minute)
	check("hour 23", r, false)

	// The first failing row of several sends its FailMessage.
	a, c := anyRequirement(5), anyRequirement(5)
	c.Map, c.FailMessage = 3, 0x12F
	b.dus().requirements = []gamedata.CommandRequirement{a, c}
	if !b.commandRequirementFails(o, 5) {
		t.Fatal("second row not checked")
	}
	hasText(t, b.drain(o), b.messageFor(o, 0x12F))
	c.FailMessage = -1
	b.dus().requirements = []gamedata.CommandRequirement{c}
	if !b.commandRequirementFails(o, 5) || len(b.drain(o)) != 0 {
		t.Fatal("FailMessage -1 sends nothing")
	}
}

// runCommand runs CommandRequirement after the CommandManager checks and
// before the command; a failure neither runs nor charges it.
func TestCommandRequirementBlocksCommand(t *testing.T) {
	b := newBareWorld(t)
	o := b.player(PlayerStart, "ReqCmd", 0, 130, 130)
	o.P.Char.LevelUpPoints, o.P.Char.Strength = 10, 20
	r := anyRequirement(5)
	r.Map = 3
	b.dus().requirements = []gamedata.CommandRequirement{r}
	b.runCommand(o, "/f 1")
	if o.P.Char.Strength != 20 {
		t.Fatal("/f ran")
	}
	hasText(t, b.drain(o), b.messageFor(o, 0x12E))
	b.dus().requirements = nil
	b.runCommand(o, "/f 1")
	if o.P.Char.Strength != 21 {
		t.Fatalf("/f did not run: %d", o.P.Char.Strength)
	}
}

// --- MonsterMove (FUN_004b1450) -------------------------------------------------------

func TestMonsterMove(t *testing.T) {
	b := newBareWorld(t)
	b.setMonsterMoves([]gamedata.MonsterMoveRow{{Monster: 229, Map: 0, X: 137, Y: 87, Dir: 1, KeepTime: 60}})
	b.monsterMoveSecond()
	n := b.dus().moves[0]
	if n.Index != -1 {
		t.Fatal("spawned before 1000 ms")
	}
	b.advance(time.Second)
	b.monsterMoveSecond()
	if !validIdx(n.Index) {
		t.Fatal("no spawn")
	}
	m := b.objects[n.Index]
	if m.Class != 229 || m.Map != 0 || m.X != 137 || m.Y != 87 || m.Dir != 1 {
		t.Fatalf("npc %d map %d (%d,%d) dir %d", m.Class, m.Map, m.X, m.Y, m.Dir)
	}
	first := n.Index
	b.advance(30 * time.Second)
	b.monsterMoveSecond()
	if n.Index != first || n.Remain != 30 {
		t.Fatalf("moved early: index %d remain %d", n.Index, n.Remain)
	}
	b.advance(30 * time.Second)
	b.monsterMoveSecond()
	if b.objects[first].State != StateEmpty && b.objects[first].Class == 229 && n.Index != first {
		t.Fatal("old NPC not deleted")
	}
	if !validIdx(n.Index) || b.objects[n.Index].Class != 229 || n.Until != b.now().Unix()+60 {
		t.Fatalf("no respawn: %+v", n)
	}
	count := 0
	for i := 0; i < SummonStart; i++ {
		if o := b.objects[i]; o.State != StateEmpty && o.Class == 229 {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("%d Marlons", count)
	}
}

// --- MonsterSchedule (FUN_004bb560 / FUN_004bb8d0) -----------------------------------------

func TestMonsterSchedule(t *testing.T) {
	b := newBareWorld(t)
	o := b.player(PlayerStart, "Sched", 0, 130, 130)
	npc := fakeNPC(b, 100, 229, 0, 131, 130)
	// Every day at 00:10:00, open for 120 s.
	b.setMonsterSchedule(map[int]*gamedata.MonsterScheduleGroup{
		3: {Schedule: []gamedata.Schedule{{Year: -1, Month: -1, Day: -1, DayOfWeek: -1, Hour: 0, Minute: 10, Second: 0}},
			EnableMsg: 0x12E, DisableMsg: 0x12F, NotTimeMsg: 0x130, NPCSec: 120,
			Monsters: []gamedata.ScheduledMonster{{Monster: 229, Map: -1, X: -1, Y: -1}}},
	})
	g := &b.dus().schedule[3]
	if g.State != scheduleClosed || g.Remain != 600 {
		t.Fatalf("state %d remain %d", g.State, g.Remain)
	}
	if !b.npcTalk(o, npc) {
		t.Fatal("closed NPC talked")
	}
	if p := find(b.drain(o), 0x01, -1); p == nil || cString(p[5:]) != b.messageFor(o, 0x130) {
		t.Fatalf("not-time chat % X", p)
	}
	other := fakeNPC(b, 101, 230, 0, 132, 130)
	if b.scheduleBlocksNpc(o, other) {
		t.Fatal("other class blocked")
	}
	b.advance(600 * time.Second)
	b.monsterScheduleSecond()
	if g.State != scheduleOpen || g.Remain != 120 {
		t.Fatalf("open: state %d remain %d", g.State, g.Remain)
	}
	hasText(t, b.drain(o), b.messageFor(o, 0x12E))
	if b.scheduleBlocksNpc(o, npc) {
		t.Fatal("open NPC blocked")
	}
	b.advance(119 * time.Second)
	b.monsterScheduleSecond()
	if g.State != scheduleOpen {
		t.Fatal("closed early")
	}
	b.advance(time.Second)
	b.monsterScheduleSecond()
	if g.State != scheduleClosed || g.Remain != 24*3600-120 {
		t.Fatalf("closed: state %d remain %d", g.State, g.Remain)
	}
	hasText(t, b.drain(o), b.messageFor(o, 0x12F))
}

// --- BlackList (FUN_00411f00 / FUN_00411fa0) ---------------------------------------------

func TestGameServerBlackList(t *testing.T) {
	b := newBareWorld(t)
	if !acceptFilterHook(b.World, "127.0.0.1") {
		t.Fatal("empty list refused")
	}
	b.dus().blackIPs["10.0.0.7"] = true
	if acceptFilterHook(b.World, "10.0.0.7") || !acceptFilterHook(b.World, "10.0.0.70") {
		t.Fatal("IP lookup")
	}
	hw := "ABCDEFGH-IJKLMNOP-QRSTUVWX-YZ012345-67890123"
	if len(hw) != 0x2C || b.blacklistedHWID(hw) {
		t.Fatal("hwid")
	}
	b.dus().blackHWIDs[hw] = true
	if !b.blacklistedHWID(hw) {
		t.Fatal("hwid not listed")
	}
}

// --- CustomEvolution (FUN_004ce0b0) ------------------------------------------------------------

func TestCustomEvolution(t *testing.T) {
	b := newBareWorld(t)
	o := b.player(PlayerStart, "Evo", 0, 130, 130)
	b.dus().evolution = []gamedata.EvolutionRow{{Index: 0, Target: 0*512 + 1, Result: 0*512 + 2}}
	src, dst := BagStart, BagStart+5
	o.P.Inventory[src] = newItem(evolutionItem, 0)
	tgt := newItem(0*512+1, 7)
	tgt.Luck, tgt.Option, tgt.Durability = true, 2, 30
	o.P.Inventory[dst] = tgt
	b.handleUseItem(o, []byte{0xC1, 6, 0x26, byte(src), byte(dst), 0})
	b.runAsync()
	pk := b.drain(o)
	if !o.P.Inventory[dst].IsEmpty() && o.P.Inventory[dst].Index == 0*512+1 {
		t.Fatal("target kept")
	}
	if !o.P.Inventory[src].IsEmpty() && o.P.Inventory[src].Index == evolutionItem {
		t.Fatal("evolution item kept")
	}
	got := -1
	for s := BagStart; s < BagEnd; s++ {
		if it := o.P.Inventory[s]; !it.IsEmpty() && it.Index == 0*512+2 {
			got = s
		}
	}
	if got < 0 {
		t.Fatal("no result item")
	}
	if r := o.P.Inventory[got]; r.Level != 7 || !r.Luck || r.Option != 2 || r.Durability != 30 {
		t.Fatalf("result %+v", r)
	}
	dels := 0
	for _, p := range pk {
		if len(p) > 2 && p[2] == 0x28 {
			dels++
		}
	}
	if dels != 3 { // source, target, then the caller deletes the source again
		t.Fatalf("%d deletes", dels)
	}

	// No row for the target: a weapon keeps its options and the item is used.
	o.P.Inventory[src] = newItem(evolutionItem, 0)
	o.P.Inventory[dst] = newItem(0*512+5, 3)
	b.handleUseItem(o, []byte{0xC1, 6, 0x26, byte(src), byte(dst), 0})
	if !o.P.Inventory[src].IsEmpty() || o.P.Inventory[dst].Index != 0*512+5 || o.P.Inventory[dst].Level != 3 {
		t.Fatal("no-row path")
	}
	// A jewel target (slot -1) leaves both items.
	o.P.Inventory[src] = newItem(evolutionItem, 0)
	o.P.Inventory[dst] = newItem(14*512+13, 0)
	b.handleUseItem(o, []byte{0xC1, 6, 0x26, byte(src), byte(dst), 0})
	if o.P.Inventory[src].IsEmpty() {
		t.Fatal("used on a jewel")
	}
}

func TestCustomEvolutionNoSpace(t *testing.T) {
	b := newBareWorld(t)
	o := b.player(PlayerStart, "EvoFull", 0, 130, 130)
	for s := BagStart; s < BagEnd; s++ {
		o.P.Inventory[s] = newItem(14*512+13, 0)
	}
	b.dus().evolution = []gamedata.EvolutionRow{{Index: 0, Target: 0*512 + 1, Result: 0*512 + 2}}
	src, dst := BagStart, BagStart+1
	o.P.Inventory[src] = newItem(evolutionItem, 0)
	o.P.Inventory[dst] = newItem(0*512+1, 0)
	b.handleUseItem(o, []byte{0xC1, 6, 0x26, byte(src), byte(dst), 0})
	pk := b.drain(o)
	hasText(t, pk, "You do not have space  to use the Item!")
	if o.P.Inventory[src].Index != evolutionItem || o.P.Inventory[dst].Index != 0*512+1 {
		t.Fatal("items changed")
	}
}

// --- ItemLevel (FUN_00494a40) ----------------------------------------------------------------

func TestItemLevelNames(t *testing.T) {
	b := newBareWorld(t)
	if len(b.dus().itemLevels) == 0 {
		t.Fatal("ItemLevel.txt not loaded")
	}
	r := b.dus().itemLevels[0]
	if got := b.itemName(r.Index, r.Level); got != r.Name {
		t.Fatalf("name %q, want %q", got, r.Name)
	}
	b.dus().itemLevels = []gamedata.ItemLevelName{{Index: 14*512 + 11, Level: 1, Name: "Star of Sacred Birth"}}
	if b.itemName(14*512+11, 1) != "Star of Sacred Birth" || b.itemName(14*512+11, 0) != b.data.Items[14*512+11].Name {
		t.Fatal("fallback to Item.txt")
	}
	if b.itemName(0x1FFF, 0) != "Unknown" {
		t.Fatal("unknown")
	}
	it := newItem(14*512+11, 1)
	if got := b.pickName(it); got != "star of sacred birth" {
		t.Fatalf("pick name %q", got)
	}
	it.Level = 2
	if got := b.pickName(it); got != toLower(b.data.Items[14*512+11].Name)+" +2" {
		t.Fatalf("pick name %q", got)
	}
}

// --- HackSkillSpeedCheck (FUN_00488550) -------------------------------------------------------

func TestHackSkillSpeedCheck(t *testing.T) {
	b := newBareWorld(t)
	o := b.player(PlayerStart, "Speedy", 0, 130, 130)
	o.Class = 1
	b.dus().speedRules = []gamedata.SkillSpeedRule{{Skill: 19, Class: -1, MinLevel: -1, MaxLevel: -1, MinReset: -1,
		MaxReset: -1, MinSpeed: -1, MaxSpeed: -1, MaxDelay: 1000, MinCount: 2, MaxCount: 3, Action: 1}}
	for i := 1; i <= 3; i++ {
		if !b.skillSpeedOK(o, 19) {
			t.Fatalf("use %d refused", i)
		}
	}
	b.sendSkillTo(o, 19, o.Index, true)
	pk := b.drain(o)
	if find(pk, 0x19, -1) != nil {
		t.Fatal("C3 08 19 sent on the 4th use")
	}
	hasText(t, pk, b.messageFor(o, 0x2CF))
	if !b.skillSpeedOK(o, 20) {
		t.Fatal("other skill refused")
	}
	b.advance(time.Second)
	if !b.skillSpeedOK(o, 19) {
		t.Fatal("count not reset after MaxDelay")
	}
	b.sendSkillTo(o, 19, o.Index, true)
	if find(b.drain(o), 0x19, -1) == nil {
		t.Fatal("packet dropped")
	}

	// Action 2 closes the game; the speed range filters the rule.
	b.dus().speedRules[0].Action = 2
	b.dus().speedRules[0].MaxDelay = -1
	for i := 0; i < 4; i++ {
		b.skillSpeedOK(o, 19)
	}
	if o.P.LogoutCountdown != 6 {
		t.Fatal("not disconnected")
	}
	b.dus().speedRules[0].MinSpeed = 100000
	if !b.skillSpeedOK(o, 19) {
		t.Fatal("speed filter")
	}
}

// --- GHRS (CastleSiege.exe) ----------------------------------------------------------------

func TestGHRSDay(t *testing.T) {
	g := gamedata.CustomGHRS{Week: [7]int{25, 20, 20, 20, 20, 20, 25}, Hour: 6}
	// 2026-01-01 22:00 UTC = 05:00 on Jan 2 in UTC+7, before 06:00: Jan 1.
	if d := ghrsDay(g, time.Date(2026, 1, 1, 22, 0, 0, 0, time.UTC)); ghrsDate(d) != 20260101 {
		t.Fatalf("day %d", ghrsDate(d))
	}
	if d := ghrsDay(g, time.Date(2026, 1, 1, 23, 30, 0, 0, time.UTC)); ghrsDate(d) != 20260102 {
		t.Fatalf("day %d", ghrsDate(d))
	}
}

func TestGHRSResetCap(t *testing.T) {
	b := newBareWorld(t)
	o := b.player(PlayerStart, "Cap", 0, 130, 130)
	if b.ghrsBlocks(o) {
		t.Fatal("GHRS on the GameServer")
	}
	// CastleSiege server before the first answer: no cap (the original
	// kept 0 and refused every reset, U-GHRS-01).
	b.dus().ghrs = &ghrsState{cfg: gamedata.CustomGHRS{Week: [7]int{25, 20, 20, 20, 20, 20, 25}}}
	o.P.Char.Resets = 3
	if b.ghrsBlocks(o) {
		t.Fatal("cap before the first answer")
	}
	// A reply: 2026-01-01 00:00 UTC is 07:00 Thursday in UTC+7 -> +20.
	b.ghrsReply(20260101, 10)
	g := b.dus().ghrs
	if g.Limit != 30 || g.Bonus != 20 || g.Top != 10 || g.Date != 20260101 {
		t.Fatalf("state %+v", g)
	}
	if b.ghrsBlocks(o) {
		t.Fatal("below the cap")
	}
	b.ghrsResetDone(o)
	hasText(t, b.drain(o), b.messageFor(o, msgGHRSDone, 3, 30))
	// At the cap the reset is refused with 0x3DF, which now has a text.
	o.P.Char.Resets = 30
	b.runCommand(o, "/reset")
	hasText(t, b.drain(o), b.messageFor(o, msgGHRSLimit, 30, 30))
	if cmdOf(o).ResetPending || b.messageFor(o, msgGHRSLimit, 30, 30) == "" {
		t.Fatal("reset at the cap")
	}
}

// The service answers the F8 request itself: the top-1 ResetCount plus the
// bonus of the GHRS weekday, asked every 10 s while the date is older.
func TestGHRSAnswer(t *testing.T) {
	b := newBareWorld(t)
	ctx := context.Background()
	acc, _ := b.store.CreateAccount(ctx, "ghrstop", "plain:x", "")
	b.store.CreateCharacter(ctx, acc.ID, "Topper", 0)
	c, _ := b.store.LoadCharacter(ctx, acc.ID, "Topper")
	c.Resets = 40
	b.store.SaveCharacter(ctx, c)
	b.clock = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) // Thursday 07:00 UTC+7
	b.dus().ghrs = &ghrsState{cfg: gamedata.CustomGHRS{Week: [7]int{25, 20, 20, 20, 20, 20, 25}}}
	b.ghrsSecond()
	b.runAsync()
	g := b.dus().ghrs
	if g.Date != 20260101 || g.Top != 40 || g.Limit != 60 {
		t.Fatalf("state %+v", g)
	}
	// Same day: no new request even after 10 s.
	c.Resets = 50
	b.store.SaveCharacter(ctx, c)
	b.advance(11 * time.Second)
	b.ghrsSecond()
	b.runAsync()
	if g.Top != 40 {
		t.Fatal("asked again on the same day")
	}
}

func toLower(s string) string {
	out := []byte(s)
	for i, c := range out {
		if c >= 'A' && c <= 'Z' {
			out[i] = c + 32
		}
	}
	return string(out)
}
