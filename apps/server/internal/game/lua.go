package game

// Lua scripting: the embedded Lua 5.2.0 of GameServer.exe ($LuaVersion at
// 0x5446a8) and its bridge (docs/server-plan/17-lua.md).
//
// Two states exist in the original:
//   - CScriptLoader (0x10d60c8, state at +0x58): Script\ScriptMain.lua,
//     loaded by FUN_004e6860 (luaL_newstate, luaL_openlibs, the custom
//     require FUN_004a5220, the API table FUN_004a3f40, luaL_loadfilex and
//     lua_pcall). Every game event calls a global BridgeFunction_<Name>
//     through FUN_004e69c0..FUN_004e7860;
//   - the CScript at CServerInfo+0x1b70 (state DAT_010be21c, created by
//     FUN_004e8390 with the same API): Script\Character\CalcCharacter.lua,
//     loaded and run once by FUN_004e65e0. No code ever calls a function of
//     that state (the only lua_getglobal calls are the 17 bridges) and the
//     CalcLuaScriptSwitch read by FUN_004eda70 into CServerInfo+0xd3c is
//     never tested, so the native CalcCharacter FUN_004c9790 (stats.go) is
//     the only one in effect.
//
// Both loads happen in FUN_004ebe20, the start-up loader that the "Reload
// Script" menu also runs: OnShutScript on the old state, ScriptMain.lua,
// OnReadScript, then CalcCharacter.lua.

import (
	"bytes"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	lua "github.com/Shopify/go-lua"
)

// Paths as FUN_004ebe20 / FUN_004a5220 build them from the data base
// "..\Data\" (FUN_004e8460).
const (
	luaDataBase   = `..\Data\`
	luaMainPath   = luaDataBase + `Script\ScriptMain.lua`
	luaCalcPath   = luaDataBase + `Script\Character\CalcCharacter.lua`
	luaRequireFmt = luaDataBase + `\Script\%s.lua`
)

// luaScripts is the scripting state of one instance.
type luaScripts struct {
	main *lua.State // CScriptLoader+0x58 (nil when ScriptMain.lua failed)
	calc *lua.State // CServerInfo+0x1b70 (CalcCharacter.lua; never called)

	// sentinel is DAT_00772d80, the light userdata FUN_004a5220 stores in
	// _LOADED while a required file runs.
	sentinel *byte

	objs map[int]*luaObject // per-object fields only the API touches
}

// luaObject keeps object fields the Go model has nowhere else.
type luaObject struct {
	perm                     [50]byte // obj+0x20a4: PermissionInsert/Remove (cleared on load, FUN_0050ae50)
	deathMap, deathX, deathY int      // obj+0x1ae / 0x1b0 / 0x1b2: the killer's position (FUN_004cda80)
	masterLevel, masterPoint int      // obj+0x5e4 / 0x5e8: written by the API, read by nothing
}

func (w *World) scripts() *luaScripts {
	if w.lua == nil {
		w.lua = &luaScripts{sentinel: new(byte), objs: map[int]*luaObject{}}
	}
	return w.lua
}

func (s *luaScripts) obj(index int) *luaObject {
	o := s.objs[index]
	if o == nil {
		o = &luaObject{}
		s.objs[index] = o
	}
	return o
}

func registerLua(h *hooks) {
	h.start = append(h.start, func(w *World) error {
		w.loadScripts()
		return nil
	})
	// BridgeFunction_OnTimerThread: timer 9 of FUN_0047d810, created by
	// FUN_004e0120 with 1000 ms due time and period.
	h.tick1000 = append(h.tick1000, func(w *World) { w.scriptCall("OnTimerThread") })
	// BridgeFunction_OnCharacterEntry: FUN_00473a10 after the character is
	// placed in the world.
	h.enterWorld = append(h.enterWorld, (*World).scriptCharacterEntry)
	// BridgeFunction_OnCharacterClose: DelCharacterInfo FUN_004c4b40.
	h.leaveWorld = append(h.leaveWorld, func(w *World, o *Object) {
		w.scriptCall("OnCharacterClose", o.Index)
		delete(w.scripts().objs, o.Index)
	})
	// BridgeFunction_OnCommandManager (FUN_00435400 -> FUN_004e6ba0): a
	// non-zero result means the script ran the command (FUN_00436c30).
	h.commandScript = func(w *World, o *Object, index int, arg string) bool {
		return w.scriptCallInt("OnCommandManager", 0, o.Index, index, arg) != 0
	}
	// FUN_004cda80: the victim keeps the killer's map and position
	// (obj+0x1ae/0x1b0/0x1b2), then BridgeFunction_OnMonsterDie (monster
	// victims) or BridgeFunction_OnUserDie (player victims) runs with
	// (victim, killer).
	h.death = append(h.death, (*World).scriptObjectDeath)
}

// scriptCharacterEntry clears the per-character script fields (FUN_0050ae50
// clears obj+0x20a4.. on load) and runs BridgeFunction_OnCharacterEntry.
func (w *World) scriptCharacterEntry(o *Object) {
	delete(w.scripts().objs, o.Index)
	w.scriptCall("OnCharacterEntry", o.Index)
}

// scriptObjectDeath is the death part of FUN_004cda80 for the scripts.
func (w *World) scriptObjectDeath(killer, victim *Object) {
	lo := w.scripts().obj(victim.Index)
	lo.deathMap, lo.deathX, lo.deathY = killer.Map, killer.X, killer.Y
	switch victim.Type {
	case TypeMonster:
		w.scriptCall("OnMonsterDie", victim.Index, killer.Index)
	case TypePlayer:
		w.scriptCall("OnUserDie", victim.Index, killer.Index)
	}
}

// loadScripts is FUN_004ebe20.
func (w *World) loadScripts() {
	s := w.scripts()
	if s.main != nil {
		w.scriptCall("OnShutScript") // FUN_004e6a60
	}
	s.main = w.newScriptState()
	if !w.runScriptFile(s.main, luaMainPath, "ScriptLoader") {
		s.main = nil // FUN_004e6860 closes the state (+0x58 stays 0)
	}
	w.scriptCall("OnReadScript") // FUN_004e69c0
	s.calc = w.newScriptState()
	if !w.runScriptFile(s.calc, luaCalcPath, "Script") {
		s.calc = nil // FUN_004e65e0 closes the state
	}
}

// reloadScripts is the "Reload Script" menu (FUN_004ebe20 again).
func (w *World) reloadScripts() { w.loadScripts() }

// newScriptState is luaL_newstate + luaL_openlibs + the API table
// (FUN_004e6860 / FUN_004e8390), with the require of FUN_004a5220.
func (w *World) newScriptState() *lua.State {
	l := lua.NewState()
	lua.OpenLibraries(l)
	w.luaCompat(l)
	l.Register("require", w.luaRequire)
	w.registerLuaAPI(l)
	return l
}

// runScriptFile is luaL_loadfilex + lua_pcall(0, 0) of FUN_004e6860 (tag
// ScriptLoader) and FUN_004e65e0 (tag Script).
func (w *World) runScriptFile(l *lua.State, path, tag string) bool {
	if err := w.luaLoadFile(l, path); err != nil {
		w.log.Error(fmt.Sprintf("[%s] Could not load '%s'. %s", tag, path, luaErrText(l)))
		return false
	}
	if err := l.ProtectedCall(0, 0, 0); err != nil {
		w.log.Error(fmt.Sprintf("[%s] Error in Lua-file. %s", tag, luaErrText(l)))
		return false
	}
	return true
}

// luaFilePath maps an original "..\Data\..." path to the data directory.
func (w *World) luaFilePath(path string) string {
	rel := strings.TrimPrefix(path, luaDataBase)
	rel = strings.TrimLeft(strings.ReplaceAll(rel, `\`, "/"), "/")
	return filepath.Join(w.dataDir, filepath.FromSlash(rel))
}

// luaLoadFile is luaL_loadfilex with the original file name as the chunk
// name; on failure the message is left on the stack.
func (w *World) luaLoadFile(l *lua.State, path string) error {
	b, err := os.ReadFile(w.luaFilePath(path))
	if err != nil {
		l.PushString("cannot open " + path)
		return lua.FileError
	}
	if len(b) > 0 && b[0] == '#' { // skipcomment: the first line is ignored
		if i := bytes.IndexByte(b, '\n'); i >= 0 {
			b = b[i:]
		} else {
			b = nil
		}
	}
	return l.Load(bytes.NewReader(b), "@"+path, "")
}

func luaErrText(l *lua.State) string {
	s, _ := l.ToString(-1)
	l.Pop(1)
	return s
}

// luaRequire is FUN_004a5220: require(name) runs "..\Data\\Script\<name>.lua"
// once per call. The file is loaded again on every require of a finished
// file (only an in-progress one, marked by the sentinel, is refused), the
// chunk gets the path as its argument and its result (or true) is stored in
// _LOADED[path]. Load failures are only logged and return nothing.
func (w *World) luaRequire(l *lua.State) int {
	s := w.scripts()
	name, _ := l.ToString(1)
	path := fmt.Sprintf(luaRequireFmt, name)
	l.SetTop(1)
	l.Field(lua.RegistryIndex, "_LOADED") // 2
	l.Field(2, path)                      // 3
	if l.ToBoolean(-1) && l.ToUserData(-1) == s.sentinel {
		w.log.Error(fmt.Sprintf("[ScriptLoader] Error in Lua-file. %s", luaToString(l, -1)))
		return 0
	}
	if err := w.luaLoadFile(l, path); err != nil { // 4
		w.log.Error(fmt.Sprintf("[ScriptLoader] Could not load '%s'. %s", path, luaToString(l, -1)))
		return 0
	}
	l.PushUserData(s.sentinel) // go-lua has no light userdata type
	l.SetField(2, path)
	l.PushString(path)
	l.Call(1, 1)
	if !l.IsNil(-1) {
		l.SetField(2, path)
	}
	l.Field(2, path)
	if l.ToUserData(-1) == s.sentinel {
		l.PushBoolean(true)
		l.PushValue(-1)
		l.SetField(2, path)
	}
	return 1
}

func luaToString(l *lua.State, i int) string {
	s, _ := l.ToString(i)
	return s
}

// --- bridges -----------------------------------------------------------------

// scriptPush pushes bridge arguments: ints with lua_pushinteger, strings
// with lua_pushstring.
func scriptPush(l *lua.State, args []any) {
	for _, a := range args {
		switch v := a.(type) {
		case int:
			l.PushInteger(v)
		case string:
			l.PushString(v)
		default:
			l.PushNil()
		}
	}
}

// scriptBegin gets BridgeFunction_<name>; false (with the original log line)
// when the state is missing or the global is not a function. base is the
// stack top to restore: a bridge may run inside an API function of an
// outer bridge (CommandSend -> OnCommandManager), whose arguments stay.
func (w *World) scriptBegin(name string) (l *lua.State, base int, ok bool) {
	l = w.scripts().main
	if l == nil {
		return nil, 0, false
	}
	base = l.Top()
	l.Global("BridgeFunction_" + name)
	if l.TypeOf(-1) != lua.TypeFunction {
		w.log.Warn(fmt.Sprintf("Error: Function '%s' is not defined.", name))
		l.SetTop(base)
		return nil, 0, false
	}
	return l, base, true
}

// scriptCall runs a bridge without result (FUN_004e69c0, FUN_004e6a60,
// FUN_004e6b00, FUN_004e6cc0, FUN_004e6d90, FUN_004e6e50, FUN_004e7020,
// FUN_004e70f0, FUN_004e71c0).
func (w *World) scriptCall(name string, args ...any) {
	l, base, ok := w.scriptBegin(name)
	if !ok {
		return
	}
	scriptPush(l, args)
	if err := l.ProtectedCall(len(args), 0, 0); err != nil {
		w.log.Warn(fmt.Sprintf("Error: %s", luaErrText(l)))
	}
	l.SetTop(base)
}

// scriptCallInt runs a bridge that must return a number (FUN_004e6ba0,
// FUN_004e6f10, FUN_004e7290, FUN_004e73b0, FUN_004e74d0, FUN_004e75f0,
// FUN_004e7720): def when the state or the function is missing, the call
// fails or no number comes back.
func (w *World) scriptCallInt(name string, def int, args ...any) int {
	l, base, ok := w.scriptBegin(name)
	if !ok {
		return def
	}
	defer l.SetTop(base)
	scriptPush(l, args)
	if err := l.ProtectedCall(len(args), 1, 0); err != nil {
		w.log.Warn(fmt.Sprintf("Error: %s", luaErrText(l)))
		return def
	}
	if !l.IsNumber(-1) {
		w.log.Warn(fmt.Sprintf("Error: '%s' must return a value.", name))
		return def
	}
	return luaInt(l, -1)
}

// scriptNpcTalk is BridgeFunction_OnNpcTalk(npc, user) of FUN_004c2eb0:
// true ends the talk.
func (w *World) scriptNpcTalk(npc, o *Object) bool {
	return w.scriptCallInt("OnNpcTalk", 0, npc.Index, o.Index) != 0
}

// scriptCheckUserTarget is BridgeFunction_OnCheckUserTarget(attacker,
// target) of FUN_0040dca0 (player targets only).
func (w *World) scriptCheckUserTarget(a, t *Object) bool {
	return w.scriptCallInt("OnCheckUserTarget", 1, a.Index, t.Index) != 0
}

// scriptCheckUserKiller is BridgeFunction_OnCheckUserKiller(killer, victim)
// of FUN_0050f330: 0 gives no PK status.
func (w *World) scriptCheckUserKiller(killer, victim *Object) bool {
	return w.scriptCallInt("OnCheckUserKiller", 1, killer.Index, victim.Index) != 0
}

// scriptUserItemPick is the permission flag obj+0x20aa and
// BridgeFunction_OnUserItemPick(user, map item) of FUN_004982e0.
func (w *World) scriptUserItemPick(o *Object, num int) bool {
	if w.scriptPermission(o, 6) {
		return false
	}
	return w.scriptCallInt("OnUserItemPick", 1, o.Index, num) != 0
}

// scriptUserItemDrop is the permission flag obj+0x20a9 and
// BridgeFunction_OnUserItemDrop(user, slot, x, y) of FUN_004988f0.
func (w *World) scriptUserItemDrop(o *Object, slot, x, y int) bool {
	if w.scriptPermission(o, 5) {
		return false
	}
	return w.scriptCallInt("OnUserItemDrop", 1, o.Index, slot, x, y) != 0
}

// scriptUserItemMove is the permission flag obj+0x20a5 and
// BridgeFunction_OnUserItemMove(user, source flag, source slot, target
// flag, target slot) of FUN_00499410.
func (w *World) scriptUserItemMove(o *Object, sf, ss, tf, ts int) bool {
	if w.scriptPermission(o, 1) {
		return false
	}
	return w.scriptCallInt("OnUserItemMove", 1, o.Index, sf, ss, tf, ts) != 0
}

// scriptCommandDone is BridgeFunction_OnCommandDone(user, command) at the
// end of FUN_00436c30.
func (w *World) scriptCommandDone(o *Object, index int) {
	w.scriptCall("OnCommandDone", o.Index, index)
}

// scriptUserRespawn is BridgeFunction_OnUserRespawn(user, obj+0x285) of
// FUN_004c3b10, after the respawn position is chosen.
func (w *World) scriptUserRespawn(o *Object) {
	kind := 0
	if d := w.mw().deaths[o.Index]; d != nil {
		kind = d.Respawn
	}
	w.scriptCall("OnUserRespawn", o.Index, kind)
}

// scriptPermission reads obj+0x20a4+n (PermissionInsert / Remove).
func (w *World) scriptPermission(o *Object, n int) bool {
	if w.lua == nil {
		return false
	}
	lo := w.lua.objs[o.Index]
	return lo != nil && lo.perm[n] != 0
}

// --- Lua 5.2.0 compatibility of the Go VM ------------------------------------

// luaInt is lua_tointegerx as built by MSVC for x86 (LUA_MSASMTRICK: fistp
// with the default round-to-nearest-even mode into a 32-bit int); 0 when
// the value is not a number.
func luaInt(l *lua.State, i int) int {
	n, ok := l.ToNumber(i)
	if !ok {
		return 0
	}
	return luaNumberToInt(n)
}

func luaNumberToInt(n float64) int {
	r := math.RoundToEven(n)
	if math.IsNaN(r) || r < math.MinInt32 || r > math.MaxInt32 {
		return math.MinInt32 // the x87 "integer indefinite"
	}
	return int(r)
}

// luaCompat adds what go-lua lacks from the Lua 5.2.0 standard library the
// original links: os.date, and string.format converting a fractional
// argument of an integer conversion with a C cast (go-lua raises "number
// has no integer representation", a Lua 5.3 rule).
func (w *World) luaCompat(l *lua.State) {
	l.Global("os")
	l.PushGoFunction(w.luaOsDate)
	l.SetField(-2, "date")
	l.Pop(1)

	l.Global("string")
	l.Field(-1, "format")
	l.PushGoClosure(luaFormat, 1)
	l.SetField(-2, "format")
	l.Pop(1)
}

// luaFormat truncates the numbers used by %d %i %c %o %u %x %X towards zero
// and calls the original string.format (upvalue 1).
func luaFormat(l *lua.State) int {
	f := lua.CheckString(l, 1)
	arg := 1
	for i := 0; i < len(f); i++ {
		if f[i] != '%' {
			continue
		}
		i++
		if i < len(f) && f[i] == '%' {
			continue
		}
		for i < len(f) && strings.IndexByte("-+ #0123456789.", f[i]) >= 0 {
			i++
		}
		arg++
		if i >= len(f) {
			break
		}
		if strings.IndexByte("dicouxX", f[i]) >= 0 && l.TypeOf(arg) == lua.TypeNumber {
			if n, _ := l.ToNumber(arg); n != math.Trunc(n) {
				l.PushNumber(math.Trunc(n))
				l.Replace(arg)
			}
		}
	}
	n := l.Top()
	l.PushValue(lua.UpValueIndex(1))
	l.Insert(1)
	l.Call(n, 1)
	return 1
}

// luaOsDate is os.date([format [, time]]) of Lua 5.2 (loslib.c): "*t" /
// "!*t" give a table, other formats go through strftime; a leading "!"
// means UTC. The default time is the world clock.
func (w *World) luaOsDate(l *lua.State) int {
	format := lua.OptString(l, 1, "%c")
	t := w.now()
	if !l.IsNoneOrNil(2) {
		t = time.Unix(int64(lua.CheckNumber(l, 2)), 0)
	}
	t = t.Local()
	if strings.HasPrefix(format, "!") {
		t = t.UTC()
		format = format[1:]
	}
	if strings.HasPrefix(format, "*t") {
		l.CreateTable(0, 9)
		set := func(k string, v int) { l.PushInteger(v); l.SetField(-2, k) }
		set("sec", t.Second())
		set("min", t.Minute())
		set("hour", t.Hour())
		set("day", t.Day())
		set("month", int(t.Month()))
		set("year", t.Year())
		set("wday", int(t.Weekday())+1)
		set("yday", t.YearDay())
		l.PushBoolean(t.IsDST())
		l.SetField(-2, "isdst")
		return 1
	}
	l.PushString(strftime(format, t))
	return 1
}

// strftime covers the C89 conversions of the "C" locale.
func strftime(f string, t time.Time) string {
	var b strings.Builder
	for i := 0; i < len(f); i++ {
		if f[i] != '%' || i+1 >= len(f) {
			b.WriteByte(f[i])
			continue
		}
		i++
		switch f[i] {
		case 'a':
			b.WriteString(t.Format("Mon"))
		case 'A':
			b.WriteString(t.Format("Monday"))
		case 'b', 'h':
			b.WriteString(t.Format("Jan"))
		case 'B':
			b.WriteString(t.Format("January"))
		case 'c':
			b.WriteString(t.Format("01/02/06 15:04:05"))
		case 'd':
			fmt.Fprintf(&b, "%02d", t.Day())
		case 'H':
			fmt.Fprintf(&b, "%02d", t.Hour())
		case 'I':
			h := t.Hour() % 12
			if h == 0 {
				h = 12
			}
			fmt.Fprintf(&b, "%02d", h)
		case 'j':
			fmt.Fprintf(&b, "%03d", t.YearDay())
		case 'm':
			fmt.Fprintf(&b, "%02d", int(t.Month()))
		case 'M':
			fmt.Fprintf(&b, "%02d", t.Minute())
		case 'p':
			b.WriteString(t.Format("PM"))
		case 'S':
			fmt.Fprintf(&b, "%02d", t.Second())
		case 'U':
			fmt.Fprintf(&b, "%02d", (t.YearDay()+6-int(t.Weekday()))/7)
		case 'w':
			fmt.Fprintf(&b, "%d", int(t.Weekday()))
		case 'W':
			fmt.Fprintf(&b, "%02d", (t.YearDay()+6-(int(t.Weekday())+6)%7)/7)
		case 'x':
			b.WriteString(t.Format("01/02/06"))
		case 'X':
			b.WriteString(t.Format("15:04:05"))
		case 'y':
			fmt.Fprintf(&b, "%02d", t.Year()%100)
		case 'Y':
			fmt.Fprintf(&b, "%d", t.Year())
		case 'Z':
			z, _ := t.Zone()
			b.WriteString(z)
		case '%':
			b.WriteByte('%')
		default:
			b.WriteByte('%')
			b.WriteByte(f[i])
		}
	}
	return b.String()
}
