package game

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	lua "github.com/Shopify/go-lua"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/item"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/frame"
)

// luaDo runs code in the ScriptMain state.
func luaDo(t *testing.T, b *bareWorld, code string) {
	t.Helper()
	l := b.scripts().main
	if l == nil {
		t.Fatal("ScriptMain.lua state not loaded")
	}
	if err := lua.DoString(l, code); err != nil {
		msg, _ := l.ToString(-1)
		l.Pop(1)
		t.Fatalf("lua: %v %s", err, msg)
	}
}

// luaGet evaluates expr and returns it with tostring (nil -> "nil").
func luaGet(t *testing.T, b *bareWorld, expr string) string {
	t.Helper()
	luaDo(t, b, "R__ = tostring(("+expr+"))")
	l := b.scripts().main
	l.Global("R__")
	s, _ := l.ToString(-1)
	l.Pop(1)
	return s
}

// luaErr runs code expecting a Lua error and returns its message.
func luaErr(t *testing.T, b *bareWorld, code string) string {
	t.Helper()
	l := b.scripts().main
	if err := lua.DoString(l, code); err == nil {
		t.Fatalf("no error for %q", code)
	}
	msg, _ := l.ToString(-1)
	l.Pop(1)
	return msg
}

// noticesOf returns the texts of the C1 0D notices and F3 E4 messages.
func noticesOf(pkts [][]byte) []string {
	var out []string
	for _, p := range pkts {
		switch {
		case len(p) > 13 && p[0] == 0xC1 && p[2] == 0x0D:
			out = append(out, fmt.Sprintf("0D %d %s", p[3], luaCStr(p[13:])))
		case len(p) == 0x86 && p[2] == 0xF3 && p[3] == 0xE4:
			out = append(out, fmt.Sprintf("E4 %d %d %s", p[4], p[5], luaCStr(p[6:])))
		}
	}
	return out
}

func luaCStr(b []byte) string {
	if i := bytes.IndexByte(b, 0); i >= 0 {
		b = b[:i]
	}
	return string(b)
}

func TestLuaScriptsLoadAtStart(t *testing.T) {
	b := newBareWorld(t)
	s := b.scripts()
	if s.main == nil || s.calc == nil {
		t.Fatalf("states main=%v calc=%v", s.main, s.calc)
	}
	// ScriptMain.lua required System\ScriptCore, WelcomeMessage and
	// CustomWindowName (FUN_004a5220 keys _LOADED by the full path).
	if got := luaGet(t, b, `#BridgeFunctionTable["OnCharacterEntry"]`); got != "1" {
		t.Fatalf("OnCharacterEntry bridges %s", got)
	}
	if got := luaGet(t, b, `BridgeFunctionTable["OnTimerThread"][1].Function`); got != "CustomWindowName_OnTimerThread" {
		t.Fatalf("timer bridge %s", got)
	}
	if got := luaGet(t, b, `package.loaded["..\\Data\\\\Script\\System\\ScriptCore.lua"]`); got != "true" {
		t.Fatalf("_LOADED[ScriptCore] = %s", got)
	}
	// CalcCharacter.lua lives in its own state and is never called.
	if got := luaGet(t, b, `type(DWPhysiDamageCalculate)`); got != "nil" {
		t.Fatalf("main state has CalcCharacter functions: %s", got)
	}
	s.calc.Global("DWPhysiDamageCalculate")
	if s.calc.TypeOf(-1) != lua.TypeFunction {
		t.Fatal("CalcCharacter.lua not run in its state")
	}
	s.calc.Pop(1)
}

func TestLuaWelcomeMessage(t *testing.T) {
	b := newBareWorld(t)
	o := b.player(PlayerStart, "Hero", 0, 130, 130)
	gm := b.player(PlayerStart+1, "Admin", 0, 140, 140)
	b.moveData.gameMasters = append(b.moveData.gameMasters, gameMaster{gm.P.Account.Login, "Admin", 8})
	gm.P.Account.Level = 2
	gm.P.Account.LevelExpiresAt = time.Date(2026, 12, 31, 23, 0, 0, 0, time.Local)

	b.scriptCharacterEntry(o)
	got := noticesOf(b.drain(o))
	want := []string{
		"0D 0 " + b.messageFor(nil, 990),
		"0D 0 Bem-vindo de volta, Hero!",
		"0D 1 " + b.messageFor(nil, 155),
		"E4 2 2 Compartilhe o MuJogatina.com.br",
		"E4 2 2 Ajude-nos a crescer",
		"E4 2 2 /cemiterio /bar /pista /offline /bau1 ..20",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("welcome\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if len(b.drain(gm)) != 0 {
		t.Fatal("other players got the welcome")
	}

	// A GameMaster (level 8) is announced to every player (NoticeSendToAll).
	b.scriptCall("OnCharacterEntry", gm.Index)
	if got := noticesOf(b.drain(o)); len(got) != 1 || got[0] != "0D 0 GameMaster Admin Online!" {
		t.Fatalf("GM online %q", got)
	}
	gmGot := noticesOf(b.drain(gm))
	if gmGot[2] != "0D 1 "+fmt.Sprintf(b.messageFor(nil, 157), "2026-12-31 23:00:00") {
		t.Fatalf("account type %q", gmGot[2])
	}
	b.scriptCall("OnCharacterClose", gm.Index)
	if got := noticesOf(b.drain(o)); len(got) != 1 || got[0] != "0D 0 GameMaster Admin Offline!" {
		t.Fatalf("GM offline %q", got)
	}
	b.drain(gm)
	b.scriptCall("OnCharacterClose", o.Index)
	if len(b.drain(gm)) != 0 {
		t.Fatal("a normal player leaving announced")
	}
}

func TestLuaWelcomeMessageNoticeBytes(t *testing.T) {
	b := newBareWorld(t)
	o := b.player(PlayerStart, "Hero", 0, 130, 130)
	b.scriptCall("OnCharacterEntry", o.Index)
	p := b.drain(o)[1]
	// C1 size 0D type count opacity delay(u16) color(u32) speed text\0, size
	// = len(text)+0x11 (FUN_004c07f0).
	text := "Bem-vindo de volta, Hero!"
	want := append([]byte{0xC1, byte(len(text) + 0x11), 0x0D, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}, text...)
	want = append(want, 0, 0, 0, 0)
	if !bytes.Equal(p, want) {
		t.Fatalf("% X\nwant % X", p, want)
	}
}

func TestLuaCustomWindowName(t *testing.T) {
	b := newBareWorld(t)
	o := b.player(PlayerStart+3, "Hero", 0, 130, 130)
	o.P.Char.Level, o.P.Char.Resets = 250, 7
	v := b.player(PlayerStart+9, "Vip", 0, 131, 131)
	v.P.Account.Level = 3
	b.tick1000()
	p := find(b.drain(o), 0xF3, 0xE8)
	title := "MuJogatina 97D - [Hero] [Free] - Resets: 7 - Level: 250"
	want := make([]byte, 0x84)
	copy(want, []byte{0xC1, 0x84, 0xF3, 0xE8})
	copy(want[4:], title)
	if !bytes.Equal(p, want) {
		t.Fatalf("title % X\nwant % X", p, want)
	}
	if p := find(b.drain(v), 0xF3, 0xE8); luaCStr(p[4:]) != "MuJogatina 97D - [Vip] [VIP] - Resets: 0 - Level: 400" {
		t.Fatalf("vip title %q", luaCStr(p[4:]))
	}
}

func TestPacketWindowTitle(t *testing.T) {
	p := packetWindowTitle("100%% " + strings.Repeat("x", 200))
	if len(p) != 0x84 || p[0] != 0xC1 || p[1] != 0x84 || p[2] != 0xF3 || p[3] != 0xE8 {
		t.Fatalf("header % X", p[:4])
	}
	if s := luaCStr(p[4:]); !strings.HasPrefix(s, "100% x") || len(s) != 0x7F {
		t.Fatalf("text %q (%d)", s, len(s))
	}
}

func TestLuaAPIBasics(t *testing.T) {
	b := newBareWorld(t)
	o := b.player(PlayerStart, "Hero", 2, 100, 120)
	o.P.Char.Resets, o.P.Char.Strength = 3, 500
	idx := fmt.Sprint(o.Index)
	cases := map[string]string{
		`GET_ITEM(14, 13)`:                      fmt.Sprint(14*512 + 13),
		`GetMinUserIndex()`:                     fmt.Sprint(PlayerStart),
		`GetMaxUserIndex()`:                     fmt.Sprint(MaxObjects),
		`GetMaxMonsterIndex()`:                  fmt.Sprint(SummonStart),
		`GetGameServerVersion()`:                "101",
		`GetLicenseCustomerName()`:              "MuOnline",
		`GetObjectName(` + idx + `)`:            "Hero",
		`GetObjectName(-1)`:                     "nil",
		`GetObjectName(GetMaxIndex())`:          "nil",
		`GetObjectName(` + idx + `+1)`:          "",
		`GetObjectReset(` + idx + `+1)`:         "nil",
		`GetObjectLevel(` + idx + `+1)`:         "0",
		`GetObjectReset(` + idx + `)`:           "3",
		`GetObjectMap(` + idx + `)`:             "2",
		`GetObjectMapY(` + idx + `)`:            "120",
		`GetObjectType(` + idx + `)`:            "1",
		`GetObjectAccountLevel(` + idx + `)`:    "0",
		`GetObjectIndexByName("Hero")`:          idx,
		`GetObjectIndexByName("Nobody")`:        "-1",
		`SetObjectLevelUpPoint(` + idx + `,-5)`: "-5",
		`GetObjectLevelUpPoint(` + idx + `)`:    "0",
		`MessageGet(154)`:                       "Bem-vindo de volta, %s!",
		`MessageGet(993)`:                       "Could not find message: 993, lang: 0!",
		`InventoryGetMainSize()`:                "76",
		`MasterLevelUpSend(` + idx + `)`:        idx,
		`CommandGetArgNumber("10 20 30", 1)`:    "20",
		`CommandGetArgString("ab cd", 1)`:       "cd",
		`RandomGetNumber(0)`:                    "0",
		`string.format("%d", 7.9)`:              "7",
		`os.date("*t").wday`:                    fmt.Sprint(int(b.clock.Local().Weekday()) + 1),
		`MapCheckAttr(0, -1, 0, 4)`:             "1",
		`PermissionCheck(` + idx + `, 50)`:      "nil",
	}
	for expr, want := range cases {
		if got := luaGet(t, b, expr); got != want {
			t.Errorf("%s = %q, want %q", expr, got, want)
		}
	}
	for code, want := range map[string]string{
		`GetObjectName()`:     "[1 argument's expected]",
		`GetMaxIndex(1)`:      "[No argument's allowed]",
		`ItemGiveEx(1, 2, 3)`: "[Minimum 8 argument's expected]",
	} {
		if msg := luaErr(t, b, code); !strings.Contains(msg, want) {
			t.Errorf("%s: %q", code, msg)
		}
	}
}

func TestLuaAPIStatSetterClamp(t *testing.T) {
	b := newBareWorld(t)
	o := b.player(PlayerStart, "Hero", 0, 130, 130)
	o.Class = 1 // Dark Knight
	idx := fmt.Sprint(o.Index)
	def := b.statsData.classes[1].Strength
	maxStat := b.cfg.ALInt("Common", "MaxStatPoint", 0, 0)
	luaDo(t, b, `SetObjectStrength(`+idx+`, 1)`)
	if o.P.Char.Strength != def {
		t.Fatalf("min %d want %d", o.P.Char.Strength, def)
	}
	luaDo(t, b, `SetObjectStrength(`+idx+`, 999999999)`)
	if o.P.Char.Strength != maxStat {
		t.Fatalf("max %d want %d", o.P.Char.Strength, maxStat)
	}
	if got := luaGet(t, b, `SetObjectLeadership(`+idx+`, 100)`); got != "nil" {
		t.Fatalf("leadership on a DK: %s", got)
	}
	luaDo(t, b, `SetObjectMoney(`+idx+`, 2100000000)`)
	if o.P.Char.Money != 2000000000 {
		t.Fatalf("money %d", o.P.Char.Money)
	}
	// Beyond 32 bits fistp stores the "integer indefinite" 0x80000000,
	// which the setter turns into 0.
	luaDo(t, b, `SetObjectMoney(`+idx+`, 3000000000)`)
	if o.P.Char.Money != 0 {
		t.Fatalf("money %d", o.P.Char.Money)
	}
	// lua_tointegerx rounds to nearest even (MSVC x87 fistp).
	luaDo(t, b, `SetObjectPKCount(`+idx+`, 2.5)`)
	if o.P.Char.PKCount != 2 {
		t.Fatalf("PK count %d", o.P.Char.PKCount)
	}
}

func TestLuaAPIPackets(t *testing.T) {
	b := newBareWorld(t)
	o := b.player(PlayerStart, "Hero", 0, 130, 130)
	c := b.player(PlayerStart+1, "Friend", 0, 131, 131)
	b.see(o, c)
	idx := fmt.Sprint(o.Index)

	luaDo(t, b, `FireworksSend(`+idx+`, -1, 77)`)
	want := packetFirework(0, 130, 77)
	if p := find(b.drain(o), 0xF3, 0x40); !bytes.Equal(p, want) || !bytes.Equal(find(b.drain(c), 0xF3, 0x40), want) {
		t.Fatalf("firework % X", p)
	}
	luaDo(t, b, `ChatTargetSend(`+idx+`, -1, "hi")`)
	if p := find(b.drain(c), 0x01, -1); !bytes.Equal(p, packetObjectChat(o.Index, "hi")) {
		t.Fatalf("chat % X", p)
	}
	luaDo(t, b, `MessageSendToAll(1, 3, "all")`)
	for _, x := range []*Object{o, c} {
		if p := find(b.drain(x), 0xF3, 0xE4); !bytes.Equal(p, packetCustomNotice(1, 3, "all")) {
			t.Fatalf("message to all % X", p)
		}
	}
	luaDo(t, b, `PKLevelSend(`+idx+`, 5)`)
	if p := find(b.drain(o), 0xF3, 0x08); !bytes.Equal(p, packetPKLevel(o.Index, 5)) {
		t.Fatalf("PK level % X", p)
	}
	b.drain(c)

	o.P.Inventory[12] = item.Item{Index: 14*512 + 13, Level: 0, Durability: 1, Serial: 77}
	if got := luaGet(t, b, `InventoryGetItemTable(`+idx+`, 12).Serial`); got != "77" {
		t.Fatalf("item table serial %s", got)
	}
	if got := luaGet(t, b, `InventoryGetItemIndex(`+idx+`, 13)`); got != "nil" {
		t.Fatalf("empty slot %s", got)
	}
	luaDo(t, b, `InventoryDelItemIndex(`+idx+`, 12)`)
	if !o.P.Inventory[12].IsEmpty() {
		t.Fatal("item not deleted")
	}
	if p := find(b.drain(o), 0x28, -1); !bytes.Equal(p, []byte{0xC1, 0x05, 0x28, 12, 1}) {
		t.Fatalf("delete % X", p)
	}
}

func TestLuaBridgeHooks(t *testing.T) {
	b := newBareWorld(t)
	o := b.player(PlayerStart, "Hero", 0, 130, 130)
	c := b.player(PlayerStart+1, "Other", 0, 131, 131)
	luaDo(t, b, `
		Log = {}
		BridgeFunctionAttach('OnNpcTalk','T_Npc')
		function T_Npc(a, b) table.insert(Log, 'npc '..a..' '..b) return 1 end
		BridgeFunctionAttach('OnCheckUserTarget','T_Target')
		function T_Target(a, b) return 0 end
		BridgeFunctionAttach('OnCheckUserKiller','T_Killer')
		function T_Killer(a, b) table.insert(Log, 'killer '..a..' '..b) return 0 end
		BridgeFunctionAttach('OnCommandManager','T_Cmd')
		function T_Cmd(a, code, arg) table.insert(Log, 'cmd '..code..' '..arg) return 1 end
		BridgeFunctionAttach('OnCommandDone','T_Done')
		function T_Done(a, code) table.insert(Log, 'done '..code) end
		BridgeFunctionAttach('OnUserItemMove','T_Move')
		function T_Move(a, sf, ss, tf, ts) table.insert(Log, 'move '..sf..' '..ss..' '..tf..' '..ts) return 0 end
	`)
	npc := &Object{Index: 100, Type: TypeNPC}
	if !b.scriptNpcTalk(npc, o) {
		t.Fatal("OnNpcTalk 1 must end the talk")
	}
	if b.pvpAllowed(o, c) {
		t.Fatal("OnCheckUserTarget 0 must refuse the target")
	}
	pk := c.P.Char.PKCount
	b.playerKill(o, c)
	if c.P.Char.PKCount != pk || o.PKLevel != 0 {
		t.Fatal("OnCheckUserKiller 0 must skip the PK")
	}
	if !commandScriptHook(b.World, o, 42, "x y") {
		t.Fatal("OnCommandManager 1 must take the command")
	}
	b.commandFinish(o, command{Index: 42})
	b.handleItemMove(o, []byte{0xC1, 14, 0x24, 0, 12, 0, 0, 0, 0, 0, 0, 0, 0, 13})
	if p := find(b.drain(o), 0x24, -1); p == nil || p[3] != 0xFF {
		t.Fatalf("item move refused % X", p)
	}
	want := fmt.Sprintf("npc 100 %d|killer %d %d|cmd 42 x y|done 42|move 0 12 0 13", o.Index, o.Index, c.Index)
	if got := luaGet(t, b, `table.concat(Log, '|')`); got != want {
		t.Fatalf("log %q want %q", got, want)
	}
}

func TestLuaPermissionBlocksDrop(t *testing.T) {
	b := newBareWorld(t)
	o := b.player(PlayerStart, "Hero", 0, 130, 130)
	o.P.Inventory[12] = item.Item{Index: 14*512 + 13, Durability: 1}
	luaDo(t, b, fmt.Sprintf(`PermissionInsert(%d, 5)`, o.Index))
	b.handleDrop(o, []byte{0xC1, 6, 0x23, 130, 130, 12})
	if o.P.Inventory[12].IsEmpty() {
		t.Fatal("drop allowed with permission 5")
	}
	if p := find(b.drain(o), 0x23, -1); !bytes.Equal(p, packetDropResult(0, 12)) {
		t.Fatalf("drop result % X", p)
	}
	if got := luaGet(t, b, fmt.Sprintf(`PermissionCheck(%d, 5)`, o.Index)); got != "1" {
		t.Fatalf("check %s", got)
	}
	// Character load clears the flags (FUN_0050ae50).
	b.drain(o)
	b.scriptCharacterEntry(o)
	if b.scriptPermission(o, 5) {
		t.Fatal("permissions kept after the character load")
	}
}

func TestLuaDeathBridge(t *testing.T) {
	b := newBareWorld(t)
	o := b.player(PlayerStart, "Hero", 3, 50, 60)
	luaDo(t, b, `
		Deaths = {}
		BridgeFunctionAttach('OnUserDie','T_Die')
		function T_Die(a, b) table.insert(Deaths, a..'<'..b) end
	`)
	killer := b.player(PlayerStart+1, "Killer", 3, 51, 61)
	b.scriptObjectDeath(killer, o)
	if got := luaGet(t, b, `table.concat(Deaths, ',')`); got != fmt.Sprintf("%d<%d", o.Index, killer.Index) {
		t.Fatalf("deaths %q", got)
	}
	if got := luaGet(t, b, fmt.Sprintf(`GetObjectDeathMapX(%d)`, o.Index)); got != "51" {
		t.Fatalf("death x %s", got)
	}
}

func TestLuaRequireAndReload(t *testing.T) {
	b := newBareWorld(t)
	dir := t.TempDir()
	// A private data folder: the shipped ScriptMain plus a script that
	// counts its runs.
	for _, f := range []string{"ScriptMain.lua", "System/ScriptCore.lua", "Script/WelcomeMessage.lua", "Script/CustomWindowName.lua", "Character/CalcCharacter.lua"} {
		src, err := os.ReadFile(filepath.Join(b.dataDir, "Script", f))
		if err != nil {
			t.Fatal(err)
		}
		dst := filepath.Join(dir, "Script", f)
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(dst, src, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "Script", "Script", "Count.lua"), []byte(
		"Runs = (Runs or 0) + 1\nPathArg = ...\nBridgeFunctionAttach('OnShutScript','Count_Shut')\nfunction Count_Shut() ShutCalled = true end\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	b.dataDir = dir
	b.reloadScripts()
	luaDo(t, b, `require('Script\\Count') require('Script\\Count')`)
	if got := luaGet(t, b, `Runs`); got != "2" {
		t.Fatalf("a finished require runs the file again: %s", got)
	}
	if got := luaGet(t, b, `PathArg`); got != `..\Data\\Script\Script\Count.lua` {
		t.Fatalf("chunk argument %q", got)
	}
	if got := luaGet(t, b, `require('Script\\Missing')`); got != "nil" {
		t.Fatalf("missing require %s", got)
	}
	old := b.scripts().main
	b.reloadScripts()
	old.Global("ShutCalled")
	if !old.ToBoolean(-1) {
		t.Fatal("OnShutScript not called on the old state")
	}
	if b.scripts().main == old {
		t.Fatal("reload kept the old state")
	}

	// A broken ScriptMain leaves no state: the bridges return their defaults.
	if err := os.WriteFile(filepath.Join(dir, "Script", "ScriptMain.lua"), []byte("this is not lua"), 0o644); err != nil {
		t.Fatal(err)
	}
	b.reloadScripts()
	if b.scripts().main != nil {
		t.Fatal("broken ScriptMain kept a state")
	}
	o := b.player(PlayerStart, "Hero", 0, 130, 130)
	if !b.scriptCheckUserTarget(o, o) || b.scriptNpcTalk(o, o) {
		t.Fatal("defaults without a state")
	}
}

func TestLuaConfigProfile(t *testing.T) {
	b := newBareWorld(t)
	path := filepath.Join(t.TempDir(), "x.ini")
	luaDo(t, b, fmt.Sprintf(`ConfigSaveString("Sec", "Key", "12", %q)`, path))
	luaDo(t, b, fmt.Sprintf(`ConfigSaveString("Sec", "Name", "abc", %q)`, path))
	luaDo(t, b, fmt.Sprintf(`ConfigSaveString("Sec", "Key", "13", %q)`, path))
	if got := luaGet(t, b, fmt.Sprintf(`ConfigReadNumber("Sec", "Key", %q)`, path)); got != "13" {
		t.Fatalf("number %s", got)
	}
	if got := luaGet(t, b, fmt.Sprintf(`ConfigReadString("Sec", "Name", %q)`, path)); got != "abc" {
		t.Fatalf("string %s", got)
	}
	if got := luaGet(t, b, `ConfigReadNumber("Sec", "Key", "missing.ini")`); got != "0" {
		t.Fatalf("missing file %s", got)
	}
}

func TestStrftime(t *testing.T) {
	tm := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
	if got := strftime("%Y-%m-%d %H:%M:%S %a %j %%", tm); got != "2026-03-04 05:06:07 Wed 063 %" {
		t.Fatal(got)
	}
}

// luaCoverageLoad runs the shipped scripts like FUN_004ebe20 and checks
// that ScriptMain.lua required its three files (TestDataFileCoverage).
func luaCoverageLoad(c *covEnv) error {
	w := c.w
	main := w.newScriptState()
	if !w.runScriptFile(main, luaMainPath, "ScriptLoader") {
		return fmt.Errorf("ScriptMain.lua failed")
	}
	main.Field(lua.RegistryIndex, "_LOADED")
	for _, name := range []string{`System\ScriptCore`, `Script\WelcomeMessage`, `Script\CustomWindowName`} {
		main.Field(-1, fmt.Sprintf(luaRequireFmt, name))
		ok := main.ToBoolean(-1) && main.ToUserData(-1) == nil
		main.Pop(1)
		if !ok {
			return fmt.Errorf("%s.lua was not required", name)
		}
	}
	main.Pop(1)
	if !w.runScriptFile(w.newScriptState(), luaCalcPath, "Script") {
		return fmt.Errorf("CalcCharacter.lua failed")
	}
	return nil
}

// TestLuaWelcomeAndTitleEndToEnd enters the world with the original client
// protocol: the welcome notices of WelcomeMessage.lua follow the world
// entry and CustomWindowName.lua sets the window title within a second.
func TestLuaWelcomeAndTitleEndToEnd(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	acc, _ := env.store.AccountByLogin(ctx, "tester")
	if _, _, err := env.store.CreateCharacter(ctx, acc.ID, "Hero", 0x10); err != nil {
		t.Fatal(err)
	}
	c := env.dial(t)
	c.next(0xF1, 0x00)
	c.verify()
	if r := c.login("tester", "pass"); r != 1 {
		t.Fatalf("login %d", r)
	}
	name := make([]byte, 10)
	copy(name, "Hero")
	c.send(frame.Build(frame.C1, 0xF3, append([]byte{0x03}, name...)...))
	c.next(0xF3, 0x03)
	msgs := env.world.messages[0]
	for _, want := range []string{msgs[990], "Bem-vindo de volta, Hero!", msgs[155]} {
		if p := c.next(0x0D, -1); luaCStr(p[13:]) != want {
			t.Fatalf("notice %q want %q", luaCStr(p[13:]), want)
		}
	}
	if p := c.next(0xF3, 0xE4); p[4] != 2 || p[5] != 2 || luaCStr(p[6:]) != "Compartilhe o MuJogatina.com.br" {
		t.Fatalf("F3 E4 % X", p)
	}
	title := c.next(0xF3, 0xE8)
	if len(title) != 0x84 || luaCStr(title[4:]) != "MuJogatina 97D - [Hero] [Free] - Resets: 0 - Level: 1" {
		t.Fatalf("title %q", luaCStr(title[4:]))
	}
}
