package game

// The functions GameServer.exe registers in its Lua states (FUN_004a3f40:
// lua_pushcclosure + lua_setglobal for 194 names, the C functions at
// 0x4a5410..0x4ac9b0). Every entry checks the argument count first
// (lua_gettop) and raises "[%d argument's expected]", "[No argument's
// allowed]" or "[Minimum %d argument's expected]" with luaL_error. Object
// arguments outside 0..0x57E3 (and, for the "user" functions, objects that
// are not players, obj+0x6c != 1) return no value (nil). A C function
// that returns 1 without pushing returns its last argument to Lua; the Go
// functions keep that by returning 1 too.
//
// docs/server-plan/17-lua.md lists every function with its address.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	lua "github.com/Shopify/go-lua"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/config/ini"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/guild"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/item"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/packet"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/store"
)

// luaArgCount is the argument rule of one API function.
type luaArgCount struct {
	n   int
	min bool
}

// luaAPIEntry is one FUN_004a3f40 registration.
type luaAPIEntry struct {
	name string
	argc luaArgCount
	fn   func(w *World, l *lua.State) int
}

func eq(n int) luaArgCount  { return luaArgCount{n: n} }
func atl(n int) luaArgCount { return luaArgCount{n: n, min: true} }

func (c luaArgCount) check(l *lua.State) {
	top := l.Top()
	switch {
	case c.min && top < c.n:
		lua.Errorf(l, "[Minimum %d argument's expected]", c.n)
	case !c.min && top != c.n && c.n == 0:
		lua.Errorf(l, "[No argument's allowed]")
	case !c.min && top != c.n:
		lua.Errorf(l, "[%d argument's expected]", c.n)
	}
}

// registerLuaAPI is FUN_004a3f40.
func (w *World) registerLuaAPI(l *lua.State) {
	for _, e := range luaAPI {
		e := e
		l.Register(e.name, func(l *lua.State) int {
			e.argc.check(l)
			return e.fn(w, l)
		})
	}
}

// --- argument helpers ------------------------------------------------------------

func luaStr(l *lua.State, i int) string {
	s, _ := l.ToString(i)
	return s
}

// luaObj is arg i as an object index inside the table.
func (w *World) luaObj(l *lua.State, i int) *Object {
	n := luaInt(l, i)
	if n < 0 || n >= MaxObjects {
		return nil
	}
	return w.objects[n]
}

// luaUser is luaObj for objects whose obj+0x6c is 1.
func (w *World) luaUser(l *lua.State, i int) *Object {
	o := w.luaObj(l, i)
	if o == nil || o.Type != TypePlayer || o.P == nil {
		return nil
	}
	return o
}

// noChar stands for the zeroed character data of a player that has not
// entered the world yet.
var noChar store.Character

func chr(o *Object) *store.Character {
	if o.P != nil && o.P.Char != nil {
		return o.P.Char
	}
	return &noChar
}

func pushInt(l *lua.State, v int) int    { l.PushInteger(v); return 1 }
func pushStr(l *lua.State, s string) int { l.PushString(s); return 1 }

// constant returns a no-argument function pushing v.
func constant(v int) func(*World, *lua.State) int {
	return func(_ *World, l *lua.State) int { return pushInt(l, v) }
}

// objGet is a getter that works for any object index.
func objGet(f func(w *World, o *Object) int) func(*World, *lua.State) int {
	return func(w *World, l *lua.State) int {
		o := w.luaObj(l, 1)
		if o == nil {
			return 0
		}
		return pushInt(l, f(w, o))
	}
}

// userGet is a getter for players only.
func userGet(f func(w *World, o *Object) int) func(*World, *lua.State) int {
	return func(w *World, l *lua.State) int {
		o := w.luaUser(l, 1)
		if o == nil {
			return 0
		}
		return pushInt(l, f(w, o))
	}
}

func userGetStr(f func(w *World, o *Object) string) func(*World, *lua.State) int {
	return func(w *World, l *lua.State) int {
		o := w.luaUser(l, 1)
		if o == nil {
			return 0
		}
		return pushStr(l, f(w, o))
	}
}

// userSet is a player setter (arg 2 already converted); it returns its
// last argument like the original.
func userSet(f func(w *World, o *Object, v int)) func(*World, *lua.State) int {
	return func(w *World, l *lua.State) int {
		o := w.luaUser(l, 1)
		v := luaInt(l, 2)
		if o == nil {
			return 0
		}
		f(w, o, v)
		return 1
	}
}

func objSet(f func(w *World, o *Object, v int)) func(*World, *lua.State) int {
	return func(w *World, l *lua.State) int {
		o := w.luaObj(l, 1)
		v := luaInt(l, 2)
		if o == nil {
			return 0
		}
		f(w, o, v)
		return 1
	}
}

// userOnly returns its last argument for players (MasterLevelUpSend,
// MasterSkillTreeRebuild, ObjectAddCoin/SubCoin: the build has no master
// system nor coin store, the C functions only test the object type).
func userOnly(w *World, l *lua.State) int {
	if w.luaUser(l, 1) == nil {
		return 0
	}
	return 1
}

// classDefault is DefaultClassInfo (FUN_00434520 + 8 + class*0x30): 0 for
// classes above 4.
func (w *World) classDefault(o *Object, f func(c *luaClassDefaults) int) int {
	if o.Class > 4 || o.Class < 0 || w.statsData == nil {
		return 0
	}
	ci, ok := w.statsData.classes[o.Class]
	if !ok {
		return 0
	}
	return f(&luaClassDefaults{ci.Strength, ci.Dexterity, ci.Vitality, ci.Energy, ci.Lead})
}

type luaClassDefaults struct{ Str, Dex, Vit, Ene, Lead int }

// setStat is SetObjectStrength..Energy (FUN_004a7b90..FUN_004a7e30): the
// value is kept between the class default and MaxStatPoint_AL<account
// level> (CServerInfo+0x3d0).
func (w *World) setStat(o *Object, v int, def func(c *luaClassDefaults) int, set func(c *store.Character, v int)) {
	lo := w.classDefault(o, def)
	hi := w.cfg.ALInt("Common", "MaxStatPoint", o.P.Account.Level, 0)
	if v < lo {
		v = lo
	} else if v > hi {
		v = hi
	}
	set(chr(o), v)
}

func luaObjName(o *Object) string {
	if o.Mon != nil && o.Mon.Info != nil {
		return o.Mon.Info.Name
	}
	return chr(o).Name
}

func objMana(o *Object) int {
	if o.P != nil {
		return int(chr(o).Mana)
	}
	return 0
}

func objMaxBase(o *Object) (life, mana int) {
	if o.Mon != nil {
		return int(o.Mon.MaxLife), 0
	}
	c := chr(o)
	return int(c.MaxLife), int(c.MaxMana)
}

func (w *World) objDefense(o *Object) int {
	if o.Mon != nil {
		return o.Mon.Defense
	}
	if o.P != nil {
		return w.statsOf(o).Defense
	}
	return 0
}

func (w *World) objMaxBP(o *Object) int {
	if o.P != nil {
		return w.statsOf(o).MaxBP
	}
	return 0
}

func (w *World) luaGuildRecord(number int) *guild.Record {
	m := w.guildMgr()
	if m == nil || number == 0 {
		return nil
	}
	if g, ok := m.ByNumber(number); ok {
		return &g
	}
	return nil
}

// luaItemTable pushes the item table of InventoryGetItemTable /
// MapGetItemTable (FUN_004a8f80 / FUN_004aa340).
func luaItemTable(l *lua.State, it item.Item) {
	l.CreateTable(0, 18)
	set := func(k string, v float64) { l.PushNumber(v); l.SetField(-2, k) }
	set("Serial", float64(it.Serial))
	set("Index", float64(it.Index))
	set("Level", float64(it.Level))
	set("Durability", float64(it.Durability))
	b := func(v bool) float64 {
		if v {
			return 1
		}
		return 0
	}
	set("Option1", b(it.Skill))
	set("Option2", b(it.Luck))
	set("Option3", float64(it.Option))
	set("NewOption", float64(it.Excellent))
	set("SetOption", float64(it.SetOption))
	set("JoHOption", float64(it.JoH))
	set("380Option", float64(it.OptionEx))
	// The build has no socket items: CItem+0xd3..0xd8 stay 0xFF.
	for i := 1; i <= 5; i++ {
		set(fmt.Sprintf("SocketOption%d", i), 0xFF)
	}
	set("SocketOptionBonus", 0xFF)
}

// luaProfilePath resolves a GetPrivateProfile* file name: relative paths
// start in the data root (the parent of Data, standing in for the process
// directory of the original).
func (w *World) luaProfilePath(name string) string {
	p := filepath.FromSlash(strings.ReplaceAll(name, `\`, "/"))
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(filepath.Dir(w.dataDir), p)
}

// --- the table -------------------------------------------------------------------

var luaAPI = []luaAPIEntry{
	// 0x4a5410: ItemIndex = section*0x200 + index.
	{"GET_ITEM", eq(2), func(w *World, l *lua.State) int { return pushInt(l, luaInt(l, 1)*0x200+luaInt(l, 2)) }},
	{"GetMaxIndex", eq(0), constant(MaxObjects)},                                                            // 0x4a54b0
	{"GetMinUserIndex", eq(0), constant(PlayerStart)},                                                       // 0x4a5470
	{"GetMaxUserIndex", eq(0), constant(MaxObjects)},                                                        // 0x4a54b0 (the same function as GetMaxIndex)
	{"GetMinMonsterIndex", eq(0), constant(0)},                                                              // 0x4a5640
	{"GetMaxMonsterIndex", eq(0), constant(SummonStart)},                                                    // 0x4a54f0
	{"GetGameServerCode", eq(0), func(w *World, l *lua.State) int { return pushInt(l, w.cfg.ServerCode) }},  // 0x4a5530 (CServerInfo+0x68)
	{"GetGameServerVersion", eq(0), constant(0x65)},                                                         // 0x4a5570
	{"GetGameServerProtocol", eq(0), constant(0)},                                                           // 0x4a5640
	{"GetGameServerCurUser", eq(0), func(w *World, l *lua.State) int { return pushInt(l, w.users) }},        // 0x4a55b0 (DAT_00772d44)
	{"GetGameServerMaxUser", eq(0), func(w *World, l *lua.State) int { return pushInt(l, w.cfg.MaxUsers) }}, // 0x4a55f0 (+0xbc)
	{"GetLicenseId", eq(0), constant(0)},                                                                    // 0x4a5640
	{"GetLicenseCustomerName", eq(0), func(w *World, l *lua.State) int { return pushStr(l, "MuOnline") }},   // 0x4a5680

	// Object getters (0x4a56c0..0x4a7960).
	{"GetObjectConnected", eq(1), userGet(func(w *World, o *Object) int { return int(o.State) })},          // +0x04
	{"GetObjectIpAddress", eq(1), userGetStr(func(w *World, o *Object) string { return o.P.IP })},          // +0x19
	{"GetObjectHardwareId", eq(1), userGetStr(func(w *World, o *Object) string { return o.P.HWID })},       // +0x29
	{"GetObjectType", eq(1), objGet(func(w *World, o *Object) int { return int(o.Type) })},                 // +0x6c
	{"GetObjectAccount", eq(1), userGetStr(func(w *World, o *Object) string { return o.P.Account.Login })}, // +0x6f
	// +0x85 is the login password, which the Go server only keeps hashed.
	{"GetObjectPassword", eq(1), userGetStr(func(w *World, o *Object) string { return "" })},
	{"GetObjectName", eq(1), func(w *World, l *lua.State) int { // 0x4a5970: +0x7a, any object
		o := w.luaObj(l, 1)
		if o == nil {
			return 0
		}
		return pushStr(l, luaObjName(o))
	}},
	{"GetObjectPersonalCode", eq(1), userGetStr(func(w *World, o *Object) string { return o.P.Account.PersonalID })}, // +0xb0
	{"GetObjectClass", eq(1), objGet(func(w *World, o *Object) int { return o.Class })},                              // +0xe0
	// 0x4a5ad0 reads obj+0x314, the Change (skin) of /skin, for both names.
	{"GetObjectChangeUp", eq(1), userGet(func(w *World, o *Object) int { return gmOf(o).Skin })},
	{"GetObjectLevel", eq(1), objGet(func(w *World, o *Object) int { return objLevel(o) })},                  // +0xe6
	{"GetObjectLevelUpPoint", eq(1), userGet(func(w *World, o *Object) int { return chr(o).LevelUpPoints })}, // +0xe8
	{"GetObjectMoney", eq(1), userGet(func(w *World, o *Object) int { return int(chr(o).Money) })},           // +0xf8
	{"GetObjectStrength", eq(1), userGet(func(w *World, o *Object) int { return chr(o).Strength })},          // +0xfc
	{"GetObjectDexterity", eq(1), userGet(func(w *World, o *Object) int { return chr(o).Dexterity })},        // +0x100
	{"GetObjectVitality", eq(1), userGet(func(w *World, o *Object) int { return chr(o).Vitality })},          // +0x104
	{"GetObjectEnergy", eq(1), userGet(func(w *World, o *Object) int { return chr(o).Energy })},              // +0x108
	{"GetObjectLeadership", eq(1), userGet(func(w *World, o *Object) int { return chr(o).Leadership })},      // +0x10c
	{"GetObjectExtraStrength", eq(1), userGet(func(w *World, o *Object) int { return w.statsOf(o).AddStr })}, // +0x174
	{"GetObjectExtraDexterity", eq(1), userGet(func(w *World, o *Object) int { return w.statsOf(o).AddDex })},
	{"GetObjectExtraVitality", eq(1), userGet(func(w *World, o *Object) int { return w.statsOf(o).AddVit })},
	{"GetObjectExtraEnergy", eq(1), userGet(func(w *World, o *Object) int { return w.statsOf(o).AddEne })},
	{"GetObjectExtraLeadership", eq(1), userGet(func(w *World, o *Object) int { return w.statsOf(o).AddLead })},
	{"GetObjectTotalLevel", eq(1), userGet(func(w *World, o *Object) int { return chr(o).Level })}, // 0x4a61c0: +0xe6
	{"GetObjectTotalStrength", eq(1), userGet(func(w *World, o *Object) int { return chr(o).Strength + w.statsOf(o).AddStr })},
	{"GetObjectTotalDexterity", eq(1), userGet(func(w *World, o *Object) int { return chr(o).Dexterity + w.statsOf(o).AddDex })},
	{"GetObjectTotalVitality", eq(1), userGet(func(w *World, o *Object) int { return chr(o).Vitality + w.statsOf(o).AddVit })},
	{"GetObjectTotalEnergy", eq(1), userGet(func(w *World, o *Object) int { return chr(o).Energy + w.statsOf(o).AddEne })},
	{"GetObjectTotalLeadership", eq(1), userGet(func(w *World, o *Object) int { return chr(o).Leadership + w.statsOf(o).AddLead })},
	{"GetObjectDefaultStrength", eq(1), userGet(func(w *World, o *Object) int {
		return w.classDefault(o, func(c *luaClassDefaults) int { return c.Str })
	})},
	{"GetObjectDefaultDexterity", eq(1), userGet(func(w *World, o *Object) int {
		return w.classDefault(o, func(c *luaClassDefaults) int { return c.Dex })
	})},
	{"GetObjectDefaultVitality", eq(1), userGet(func(w *World, o *Object) int {
		return w.classDefault(o, func(c *luaClassDefaults) int { return c.Vit })
	})},
	{"GetObjectDefaultEnergy", eq(1), userGet(func(w *World, o *Object) int {
		return w.classDefault(o, func(c *luaClassDefaults) int { return c.Ene })
	})},
	{"GetObjectDefaultLeadership", eq(1), userGet(func(w *World, o *Object) int {
		return w.classDefault(o, func(c *luaClassDefaults) int { return c.Lead })
	})},
	{"GetObjectLive", eq(1), objGet(func(w *World, o *Object) int { // +0x6e
		if o.Alive {
			return 1
		}
		return 0
	})},
	// _ftol of the floats +0x110 / +0x114 / +0x11c / +0x120.
	{"GetObjectLife", eq(1), objGet(func(w *World, o *Object) int { return int(objLife(o)) })},
	{"GetObjectMaxLife", eq(1), objGet(func(w *World, o *Object) int { l, _ := objMaxBase(o); return l })},
	{"GetObjectMana", eq(1), objGet(func(w *World, o *Object) int { return objMana(o) })},
	{"GetObjectMaxMana", eq(1), objGet(func(w *World, o *Object) int { _, m := objMaxBase(o); return m })},
	{"GetObjectBP", eq(1), objGet(func(w *World, o *Object) int { return int(chr(o).BP) })},               // +0x188
	{"GetObjectMaxBP", eq(1), objGet(func(w *World, o *Object) int { return w.objMaxBP(o) })},             // +0x18c
	{"GetObjectDefense", eq(1), objGet(func(w *World, o *Object) int { return w.objDefense(o) })},         // +0x368
	{"GetObjectShield", eq(1), objGet(func(w *World, o *Object) int { return int(chr(o).Shield) })},       // +0x1bc
	{"GetObjectMaxShield", eq(1), objGet(func(w *World, o *Object) int { return int(chr(o).MaxShield) })}, // +0x1c0
	{"GetObjectPKCount", eq(1), userGet(func(w *World, o *Object) int { return chr(o).PKCount })},         // +0x1a0
	{"GetObjectPKLevel", eq(1), userGet(func(w *World, o *Object) int { return o.PKLevel })},              // +0x19c
	{"GetObjectPKTimer", eq(1), userGet(func(w *World, o *Object) int { return chr(o).PKTime })},          // +0x1a4
	{"GetObjectMap", eq(1), objGet(func(w *World, o *Object) int { return o.Map })},                       // +0x1ad
	{"GetObjectMapX", eq(1), objGet(func(w *World, o *Object) int { return o.X })},                        // +0x1a8
	{"GetObjectMapY", eq(1), objGet(func(w *World, o *Object) int { return o.Y })},                        // +0x1aa
	{"GetObjectDeathMap", eq(1), objGet(func(w *World, o *Object) int { return w.scripts().obj(o.Index).deathMap })},
	{"GetObjectDeathMapX", eq(1), objGet(func(w *World, o *Object) int { return w.scripts().obj(o.Index).deathX })},
	{"GetObjectDeathMapY", eq(1), objGet(func(w *World, o *Object) int { return w.scripts().obj(o.Index).deathY })},
	{"GetObjectAuthority", eq(1), userGet(func(w *World, o *Object) int { return mv(o).Authority })},       // +0x250
	{"GetObjectPartyNumber", eq(1), userGet(func(w *World, o *Object) int { return w.partyNumber(o) })},    // +0x2f0
	{"GetObjectGuildNumber", eq(1), userGet(func(w *World, o *Object) int { return guildOf(o).Number })},   // +0x308
	{"GetObjectGuildStatus", eq(1), userGet(func(w *World, o *Object) int { return guildOf(o).Status })},   // +0x30c
	{"GetObjectGuildName", eq(1), userGetStr(func(w *World, o *Object) string { return guildOf(o).Name })}, // +0x2fc
	{"GetObjectGuildRelationship", eq(2), func(w *World, l *lua.State) int { // 0x4a7330 -> FUN_005091a0
		a, b := w.luaUser(l, 1), w.luaUser(l, 2)
		if a == nil || b == nil {
			return 0
		}
		rel := 0
		if m := w.guildMgr(); m != nil {
			rel = m.Relation(guildOf(b).Number, guildOf(a).Number)
		}
		return pushInt(l, rel)
	}},
	{"GetObjectGuildUnionNumber", eq(1), func(w *World, l *lua.State) int { // 0x4a7400: guild+0x804
		o := w.luaUser(l, 1)
		if o == nil {
			return 0
		}
		g := w.luaGuildRecord(guildOf(o).Number)
		if g == nil {
			w.log.Info("Usuario sem guild")
			return 0
		}
		return pushInt(l, g.Union)
	}},
	{"GetObjectGuildUnionName", eq(1), func(w *World, l *lua.State) int { // 0x4a7490
		o := w.luaUser(l, 1)
		if o == nil {
			return 0
		}
		g := w.luaGuildRecord(guildOf(o).Number)
		if g == nil {
			w.log.Info("Usuario sem guild")
			return 0
		}
		if u := w.luaGuildRecord(g.Union); u != nil {
			return pushStr(l, u.Name)
		}
		l.PushNil() // lua_pushstring(NULL)
		return 1
	}},
	{"GetObjectChange", eq(1), userGet(func(w *World, o *Object) int { return gmOf(o).Skin })},            // 0x4a5ad0
	{"GetObjectInterface", eq(1), userGet(func(w *World, o *Object) int { return items(o).If.Type })},     // +0x3d4 >> 6 & 0x3ff
	{"GetObjectMasterLevel", eq(1), userGet(func(w *World, o *Object) int { return 0 })},                  // 0x4a77d0
	{"GetObjectMasterPoint", eq(1), userGet(func(w *World, o *Object) int { return 0 })},                  // 0x4a77d0
	{"GetObjectAccountLevel", eq(1), userGet(func(w *World, o *Object) int { return o.P.Account.Level })}, // +0x618
	{"GetObjectAccountExpireDate", eq(1), userGetStr(func(w *World, o *Object) string { // +0x61c
		return expireText(o.P.Account.LevelExpiresAt)
	})},
	{"GetObjectReset", eq(1), userGet(func(w *World, o *Object) int { return chr(o).Resets })},             // +0x6dc
	{"GetObjectMasterReset", eq(1), userGet(func(w *World, o *Object) int { return chr(o).MasterResets })}, // +0x6e0
	{"GetObjectGensRank", eq(1), userGet(func(w *World, o *Object) int { return 0 })},                      // 0x4a77d0
	{"GetObjectGensSymbol", eq(1), userGet(func(w *World, o *Object) int { return 0 })},
	{"GetObjectGensFamily", eq(1), userGet(func(w *World, o *Object) int { return 0 })},
	{"GetObjectGensContribution", eq(1), userGet(func(w *World, o *Object) int { return 0 })},
	{"GetObjectCSGuildSide", eq(1), userGet(func(w *World, o *Object) int { // +0x97e
		if o.CS != nil {
			return int(o.CS.Side)
		}
		return 0
	})},
	{"GetObjectOfflineFlag", eq(1), userGet(func(w *World, o *Object) int { // +0x20d9 / 0x2110 / 0x249e
		if isOffline(o) {
			return 1
		}
		return 0
	})},
	{"GetObjectIndexByName", eq(1), func(w *World, l *lua.State) int { // 0x4a7960 -> FUN_0050c6d0
		if t := w.findPlayer(luaStr(l, 1)); t != nil {
			return pushInt(l, t.Index)
		}
		return pushInt(l, -1)
	}},

	// Setters (0x4a79d0..0x4a8470): memory only, no packet.
	{"SetObjectLevel", eq(2), objSet(func(w *World, o *Object, v int) { // +0xe6 (short), any object
		if o.Mon != nil {
			o.Mon.Level = int(int16(v))
		} else if o.P != nil && o.P.Char != nil {
			o.P.Char.Level = int(int16(v))
		}
	})},
	{"SetObjectLevelUpPoint", eq(2), userSet(func(w *World, o *Object, v int) { chr(o).LevelUpPoints = max(v, 0) })},
	{"SetObjectMoney", eq(2), userSet(func(w *World, o *Object, v int) { chr(o).Money = int64(min(max(v, 0), 2000000000)) })},
	{"SetObjectStrength", eq(2), userSet(func(w *World, o *Object, v int) {
		w.setStat(o, v, func(c *luaClassDefaults) int { return c.Str }, func(c *store.Character, v int) { c.Strength = v })
	})},
	{"SetObjectDexterity", eq(2), userSet(func(w *World, o *Object, v int) {
		w.setStat(o, v, func(c *luaClassDefaults) int { return c.Dex }, func(c *store.Character, v int) { c.Dexterity = v })
	})},
	{"SetObjectVitality", eq(2), userSet(func(w *World, o *Object, v int) {
		w.setStat(o, v, func(c *luaClassDefaults) int { return c.Vit }, func(c *store.Character, v int) { c.Vitality = v })
	})},
	{"SetObjectEnergy", eq(2), userSet(func(w *World, o *Object, v int) {
		w.setStat(o, v, func(c *luaClassDefaults) int { return c.Ene }, func(c *store.Character, v int) { c.Energy = v })
	})},
	{"SetObjectLeadership", eq(2), func(w *World, l *lua.State) int { // 0x4a7f10: Dark Lord (class 4) only
		o := w.luaUser(l, 1)
		v := luaInt(l, 2)
		if o == nil || o.Class != 4 {
			return 0
		}
		w.setStat(o, v, func(c *luaClassDefaults) int { return c.Lead }, func(c *store.Character, v int) { c.Leadership = v })
		return 1
	}},
	{"SetObjectChatLimitTime", eq(2), userSet(func(w *World, o *Object, v int) { chr(o).ChatLimitTime = v })}, // +0x16c (+0x170 = 0)
	{"SetObjectPKCount", eq(2), userSet(func(w *World, o *Object, v int) { chr(o).PKCount = v })},             // +0x1a0
	{"SetObjectPKLevel", eq(2), userSet(func(w *World, o *Object, v int) { // +0x19c (byte)
		o.PKLevel = int(byte(v))
		chr(o).PKLevel = o.PKLevel
	})},
	{"SetObjectPKTimer", eq(2), userSet(func(w *World, o *Object, v int) { chr(o).PKTime = v })}, // +0x1a4
	{"SetObjectMap", eq(2), objSet(func(w *World, o *Object, v int) { o.Map = int(byte(v)) })},   // +0x1ad
	{"SetObjectMapX", eq(2), objSet(func(w *World, o *Object, v int) { o.X = int(int16(v)) })},   // +0x1a8
	{"SetObjectMapY", eq(2), objSet(func(w *World, o *Object, v int) { o.Y = int(int16(v)) })},   // +0x1aa
	{"SetObjectMasterLevel", eq(2), userSet(func(w *World, o *Object, v int) { w.scripts().obj(o.Index).masterLevel = v })},
	{"SetObjectMasterPoint", eq(2), userSet(func(w *World, o *Object, v int) { w.scripts().obj(o.Index).masterPoint = v })},
	{"SetObjectWindowTitle", eq(2), func(w *World, l *lua.State) int { // 0x4a8470 -> FUN_004dcad0
		o := w.luaObj(l, 1)
		text := luaStr(l, 2)
		if o == nil {
			return 0
		}
		w.send(o, packetWindowTitle(text))
		return 1
	}},

	// Game functions (0x4a84e0..0x4ac1c0).
	{"CharacterGetRespawnLocation", eq(1), userOnly}, // 0x4a84e0 -> FUN_004c53c0: not reproduced
	{"ChatTargetSend", eq(3), func(w *World, l *lua.State) int { // 0x4a8550 -> FUN_004d9c60
		o := w.luaObj(l, 1)
		target := luaInt(l, 2)
		text := luaStr(l, 3)
		if o == nil {
			return 0
		}
		p := packetObjectChat(o.Index, text)
		if target == -1 {
			for _, e := range o.VP2 {
				if e.State != VPEmpty && e.Type == byte(TypePlayer) {
					w.sendIndex(e.Index, p)
				}
			}
			return 1
		}
		if target < 0 || target >= MaxObjects || w.objects[target].Type != TypePlayer {
			return 0
		}
		w.sendIndex(target, p)
		return 1
	}},
	{"CommandCheckGameMasterLevel", eq(2), func(w *World, l *lua.State) int { // 0x4a8650 -> FUN_0047db10
		o := w.luaUser(l, 1)
		lvl := luaInt(l, 2)
		if o == nil {
			return 0
		}
		if w.gmLevel(o) < lvl {
			return pushInt(l, 0)
		}
		return pushInt(l, 1)
	}},
	{"CommandGetArgNumber", eq(2), func(w *World, l *lua.State) int { // 0x4a8700 -> FUN_00435210
		return pushInt(l, cmdInt(luaStr(l, 1), luaInt(l, 2)))
	}},
	{"CommandGetArgString", eq(2), func(w *World, l *lua.State) int { // 0x4a87d0 -> FUN_004352a0
		return pushStr(l, commandToken(luaStr(l, 1), luaInt(l, 2), 0x100))
	}},
	{"CommandSend", eq(2), func(w *World, l *lua.State) int { // 0x4a88d0 -> FUN_00435400
		o := w.luaUser(l, 1)
		text := luaStr(l, 2)
		if o == nil || o.P.Char == nil {
			return 0
		}
		w.runCommand(o, text)
		return 1
	}},
	{"ConfigReadNumber", eq(3), func(w *World, l *lua.State) int { // GetPrivateProfileIntA(sec, key, 0, file)
		f, err := ini.Load(w.luaProfilePath(luaStr(l, 3)))
		if err != nil {
			return pushInt(l, 0)
		}
		return pushInt(l, f.Int(luaStr(l, 1), luaStr(l, 2), 0))
	}},
	{"ConfigReadString", eq(3), func(w *World, l *lua.State) int { // GetPrivateProfileStringA(sec, key, "", buf, 0x100, file)
		f, err := ini.Load(w.luaProfilePath(luaStr(l, 3)))
		if err != nil {
			return pushStr(l, "")
		}
		return pushStr(l, f.String(luaStr(l, 1), luaStr(l, 2), "", 0x100))
	}},
	{"ConfigSaveString", eq(4), func(w *World, l *lua.State) int { // WritePrivateProfileStringA
		if err := writeProfileString(w.luaProfilePath(luaStr(l, 4)), luaStr(l, 1), luaStr(l, 2), luaStr(l, 3)); err != nil {
			w.log.Warn("ConfigSaveString", "err", err)
		}
		return 1
	}},
	{"EffectAdd", eq(8), func(w *World, l *lua.State) int { // 0x4a8b30 -> FUN_00478ae0
		o := w.luaObj(l, 1)
		typ, index, dur := luaInt(l, 2), luaInt(l, 3), luaInt(l, 4)
		v := [4]int{luaInt(l, 5), luaInt(l, 6), luaInt(l, 7), luaInt(l, 8)}
		if o == nil {
			return 0
		}
		if typ != 0 { // type 1: the duration becomes an end time (time64 + seconds)
			dur = int(w.now().Unix()) + dur
		}
		w.addEffect(o, index, dur, v, true)
		return 1
	}},
	{"EffectDel", eq(2), func(w *World, l *lua.State) int { // 0x4a8c40 -> FUN_00478d30
		o := w.luaObj(l, 1)
		index := luaInt(l, 2)
		if o == nil {
			return 0
		}
		w.removeEffect(o, index)
		return 1
	}},
	{"EffectCheck", eq(2), func(w *World, l *lua.State) int { // 0x4a8cc0 -> FUN_00478ec0
		o := w.luaObj(l, 1)
		index := luaInt(l, 2)
		if o == nil {
			return 0
		}
		if effectSlotOf(o, index) != nil {
			return pushInt(l, 1)
		}
		return pushInt(l, 0)
	}},
	{"EffectClear", eq(1), func(w *World, l *lua.State) int { // 0x4a8d60 -> FUN_0047a540
		o := w.luaObj(l, 1)
		if o == nil {
			return 0
		}
		w.clearEffects(o)
		return 1
	}},
	{"FireworksSend", eq(3), func(w *World, l *lua.State) int { // 0x4a8dd0: C1 07 F3 40 00 x y
		o := w.luaObj(l, 1)
		x, y := luaInt(l, 2), luaInt(l, 3)
		if o == nil {
			return 0
		}
		if y == -1 {
			y = o.Y
		}
		if x == -1 {
			x = o.X
		}
		p := packetFirework(0, byte(x), byte(y))
		w.send(o, p)
		w.broadcast(o, p)
		return 1
	}},
	{"InventoryGetWearSize", eq(0), constant(EquipSlots)},     // 0x4a8ec0
	{"InventoryGetMainSize", eq(0), constant(BagEnd)},         // 0x4a8f00
	{"InventoryGetFullSize", eq(0), constant(InventorySlots)}, // 0x4a8f40
	{"InventoryGetItemTable", eq(2), func(w *World, l *lua.State) int { // 0x4a8f80
		o := w.luaUser(l, 1)
		slot := luaInt(l, 2)
		if o == nil || slot < 0 || slot >= InventorySlots {
			return 0
		}
		it := o.P.Inventory[slot]
		if it.Index < 0 || it.Index >= 0x2000 {
			return 0
		}
		luaItemTable(l, it)
		return 1
	}},
	{"InventoryGetItemIndex", eq(2), func(w *World, l *lua.State) int { // 0x4a9430
		o := w.luaUser(l, 1)
		slot := luaInt(l, 2)
		if o == nil || slot < 0 || slot >= InventorySlots {
			return 0
		}
		it := o.P.Inventory[slot]
		if it.Index < 0 || it.Index >= 0x2000 {
			return 0
		}
		return pushInt(l, it.Index)
	}},
	{"InventoryGetItemCount", eq(3), func(w *World, l *lua.State) int { // 0x4a94f0 -> FUN_004948f0
		o := w.luaUser(l, 1)
		index := luaInt(l, 2)
		level := luaLevelArg(l, 3)
		if o == nil {
			return 0
		}
		return pushInt(l, w.inventoryItemCount(o, index, level))
	}},
	{"InventoryDelItemIndex", eq(2), func(w *World, l *lua.State) int { // 0x4a95a0: delete + C1 05 28 slot 01
		o := w.luaUser(l, 1)
		slot := luaInt(l, 2)
		if o == nil {
			return 0
		}
		w.deleteInventory(o, slot)
		w.send(o, packetItemDelete(slot, 1))
		return 1
	}},
	{"InventoryDelItemCount", eq(4), func(w *World, l *lua.State) int { // 0x4a9660 -> FUN_00496ee0
		o := w.luaUser(l, 1)
		index := luaInt(l, 2)
		level := luaLevelArg(l, 3)
		count := luaInt(l, 4)
		if o == nil {
			return 0
		}
		w.deleteItems(o, index, level, count)
		return 1
	}},
	{"InventoryGetFreeSlotCount", eq(1), func(w *World, l *lua.State) int { // 0x4a9710: free cells of the 8x8 map
		o := w.luaUser(l, 1)
		if o == nil {
			return 0
		}
		g := w.occupancy(&o.P.Inventory, -1)
		n := 0
		for y := 0; y < 8; y++ {
			for x := 0; x < 8; x++ {
				if !g[y][x] {
					n++
				}
			}
		}
		return pushInt(l, n)
	}},
	{"InventoryCheckSpaceByItem", eq(2), func(w *World, l *lua.State) int { // 0x4a97b0 -> FUN_004952a0
		o := w.luaUser(l, 1)
		index := luaInt(l, 2)
		if o == nil {
			return 0
		}
		if w.findBagSlot(&o.P.Inventory, index) < 0 {
			return pushInt(l, 0)
		}
		return pushInt(l, 1)
	}},
	{"InventoryCheckSpaceBySize", eq(3), func(w *World, l *lua.State) int { // 0x4a9860 -> FUN_00495340
		o := w.luaUser(l, 1)
		wd, ht := luaInt(l, 2), luaInt(l, 3)
		if o == nil {
			return 0
		}
		g := w.occupancy(&o.P.Inventory, -1)
		for s := BagStart; s < BagEnd; s++ {
			if fitsAt(&g, s, wd, ht) {
				return pushInt(l, 1)
			}
		}
		return pushInt(l, 0)
	}},
	// 0x4a9910 / 0x4a9cf0: an EventItemBag item (FUN_0048ac60, FUN_004912f0)
	// dropped or given; the bag selection is not reconstructed.
	{"ItemDrop", eq(5), func(w *World, l *lua.State) int { w.log.Debug("lua ItemDrop not reproduced"); return 0 }},
	{"ItemDropEx", atl(11), func(w *World, l *lua.State) int { // 0x4a9ac0 -> FUN_004753e0
		o := w.luaObj(l, 1)
		s := luaItemSpec(l, 5)
		mp, x, y := luaInt(l, 2), luaInt(l, 3), luaInt(l, 4)
		loot := luaInt(l, 22)
		if o == nil {
			return 0
		}
		owner := o
		if loot != 0 && loot >= 0 && loot < MaxObjects {
			owner = w.objects[loot]
		}
		w.createItem(owner, mp, x, y, s)
		return 1
	}},
	{"ItemGive", eq(2), func(w *World, l *lua.State) int { w.log.Debug("lua ItemGive not reproduced"); return 0 }},
	{"ItemGiveEx", atl(8), func(w *World, l *lua.State) int { // 0x4a9e60 -> FUN_004753e0 (map 0xEB)
		o := w.luaObj(l, 1)
		s := luaItemSpec(l, 2)
		s.ToInventory = true
		if o == nil {
			return 0
		}
		w.createItem(o, 0xEB, 0, 0, s)
		return 1
	}},
	{"LevelUpSend", eq(1), func(w *World, l *lua.State) int { // 0x4aa050 -> FUN_004db550
		o := w.luaUser(l, 1)
		if o == nil || o.P.Char == nil {
			return 0
		}
		w.send(o, w.packetLevelUp(o))
		return 1
	}},
	{"LogPrint", eq(1), func(w *World, l *lua.State) int { // 0x4aa0c0: SCRIPT_LOG
		w.log.Info(luaStr(l, 1), "log", "script")
		return 1
	}},
	{"LogColor", eq(2), func(w *World, l *lua.State) int { // 0x4aa150: console, colour 0..3
		w.log.Info(luaStr(l, 2), "log", "console", "color", luaInt(l, 1))
		return 1
	}},
	{"LogConsole", eq(2), func(w *World, l *lua.State) int { // 0x4aa220 -> FUN_00442ab0
		w.log.Info(luaStr(l, 2), "log", "console")
		return 1
	}},
	{"MapCheckAttr", eq(4), func(w *World, l *lua.State) int { // 0x4aa280: outside the map = 4
		attr := w.attrAt(luaInt(l, 1), luaInt(l, 2), luaInt(l, 3))
		if attr&byte(luaInt(l, 4)) != 0 {
			return pushInt(l, 1)
		}
		return pushInt(l, 0)
	}},
	{"MapGetItemTable", eq(2), func(w *World, l *lua.State) int { // 0x4aa340
		o := w.luaUser(l, 1)
		n := luaInt(l, 2)
		if o == nil {
			return 0
		}
		t := w.mapItems(o.Map)
		if t == nil || n < 0 || n >= len(t.Items) {
			return 0
		}
		it := t.Items[n].Item
		if it.Index < 0 || it.Index >= 0x2000 {
			return 0
		}
		luaItemTable(l, it)
		return 1
	}},
	{"MasterLevelUpSend", eq(1), userOnly},      // 0x4aa890
	{"MasterSkillTreeRebuild", eq(2), userOnly}, // 0x4aa900
	{"MessageGet", eq(1), func(w *World, l *lua.State) int { // 0x4aa980 -> FUN_004b3470
		return pushStr(l, w.luaMessage(luaInt(l, 1)))
	}},
	{"MoneySend", eq(2), func(w *World, l *lua.State) int { // 0x4aaa80 -> FUN_004da370
		o := w.luaUser(l, 1)
		if o == nil || o.P.Char == nil {
			return 0
		}
		w.sendMoney(o)
		return 1
	}},
	{"MonsterCreate", eq(5), func(w *World, l *lua.State) int { // 0x4aab00
		class, mp, x, y, dir := luaInt(l, 1), luaInt(l, 2), luaInt(l, 3), luaInt(l, 4), luaInt(l, 5)
		idx := w.allocMonsterIndex()
		if idx < 0 {
			return 0
		}
		m := w.objects[idx]
		m.reset()
		m.State = StateConnected
		m.X, m.Y, m.TX, m.TY = int(int16(x)), int(int16(y)), int(int16(x)), int(int16(y))
		if dir == -1 {
			dir = monsterRand() & 7
		}
		m.Dir = int(byte(dir))
		m.Map = int(byte(mp))
		if !w.setMonster(m, class) {
			w.deleteMonster(m)
			return 0
		}
		return pushInt(l, idx)
	}},
	{"MonsterDelete", eq(1), func(w *World, l *lua.State) int { // 0x4aacb0 -> FUN_0050c4a0
		o := w.luaObj(l, 1)
		if o == nil {
			return 0
		}
		if o.Type == TypeMonster || o.Type == TypeNPC {
			w.deleteMonster(o)
		}
		return 1
	}},
	// 0x4aad10 / 0x4ab0e0: the summon of a monster for a player with stat
	// percentages; not reproduced.
	{"MonsterSummonCreate", eq(7), func(w *World, l *lua.State) int { w.log.Debug("lua MonsterSummonCreate not reproduced"); return 0 }},
	{"MonsterSummonDelete", eq(1), func(w *World, l *lua.State) int {
		o := w.luaUser(l, 1)
		if o == nil || w.summonOf(o) == nil {
			return 0
		}
		w.summonCancel(o)
		return 1
	}},
	{"MoveUser", eq(2), func(w *World, l *lua.State) int { // 0x4ab190 -> FUN_0050f6d0 (gate move)
		o := w.luaUser(l, 1)
		gate := luaInt(l, 2)
		if o == nil || o.P.Char == nil {
			return 0
		}
		w.gateMove(o, gate)
		return 1
	}},
	{"MoveUserEx", eq(4), func(w *World, l *lua.State) int { // 0x4ab250 -> FUN_0050fd50
		o := w.luaUser(l, 1)
		mp, x, y := luaInt(l, 2), luaInt(l, 3), luaInt(l, 4)
		if o == nil || o.P.Char == nil {
			return 0
		}
		w.teleportTo(o, mp, x, y, o.Dir)
		return 1
	}},
	{"MessageSend", eq(4), func(w *World, l *lua.State) int { // 0x4ab300 -> FUN_004dc890 (C1 86 F3 E4)
		o := w.luaUser(l, 1)
		typ, color, text := luaInt(l, 2), luaInt(l, 3), luaStr(l, 4)
		if o == nil {
			return 0
		}
		w.send(o, packetCustomNotice(byte(typ), byte(color), text))
		return 1
	}},
	{"MessageSendToAll", eq(3), func(w *World, l *lua.State) int { // 0x4ab3b0
		p := packetCustomNotice(byte(luaInt(l, 1)), byte(luaInt(l, 2)), luaStr(l, 3))
		w.luaEachUser(func(o *Object) { w.send(o, p) })
		return 1
	}},
	{"NoticeSend", eq(3), func(w *World, l *lua.State) int { // 0x4ab470 -> FUN_004c07f0
		o := w.luaUser(l, 1)
		typ, text := luaInt(l, 2), luaStr(l, 3)
		if o == nil {
			return 0
		}
		w.send(o, packetNotice(byte(typ), 0, 0, 0, 0, 0, text))
		return 1
	}},
	{"NoticeSendToAll", eq(2), func(w *World, l *lua.State) int { // 0x4ab520 -> FUN_004c0ab0
		p := packetNotice(byte(luaInt(l, 1)), 0, 0, 0, 0, 0, luaStr(l, 2))
		w.luaEachUser(func(o *Object) { w.send(o, p) })
		return 1
	}},
	{"NoticeGlobalSend", eq(2), func(w *World, l *lua.State) int { // 0x4ab5f0 -> FUN_00475b50
		w.groupNotice(byte(luaInt(l, 1)), luaStr(l, 2))
		return 1
	}},
	{"PartyGetMemberCount", eq(1), func(w *World, l *lua.State) int { // 0x4ab6d0: by party number
		if pt := w.luaParty(luaInt(l, 1)); pt != nil {
			return pushInt(l, pt.count)
		}
		return pushInt(l, 0)
	}},
	{"PartyGetMemberIndex", eq(2), func(w *World, l *lua.State) int { // 0x4ab770
		pt, n := w.luaParty(luaInt(l, 1)), luaInt(l, 2)
		if pt != nil && n >= 0 && n < partyMaxMembers {
			return pushInt(l, pt.members[n])
		}
		return pushInt(l, -1)
	}},
	{"ObjectGetCoin", eq(1), func(w *World, l *lua.State) int { // 0x4ab850: obj+0xd20..0xd28, never loaded
		if w.luaUser(l, 1) == nil {
			return 0
		}
		l.CreateTable(0, 3)
		for _, k := range []string{"Coin1", "Coin2", "Coin3"} {
			l.PushNumber(0)
			l.SetField(-2, k)
		}
		return 1
	}},
	{"ObjectAddCoin", eq(4), userOnly}, // 0x4ab990
	{"ObjectSubCoin", eq(4), userOnly}, // 0x4ab990
	{"PermissionCheck", eq(2), func(w *World, l *lua.State) int { // 0x4aba20: obj+0x20a4+n, n < 50
		o, n := w.luaUser(l, 1), luaInt(l, 2)
		if o == nil || n < 0 || n > 0x31 {
			return 0
		}
		return pushInt(l, int(w.scripts().obj(o.Index).perm[n]))
	}},
	{"PermissionInsert", eq(2), func(w *World, l *lua.State) int { // 0x4abab0
		o, n := w.luaUser(l, 1), luaInt(l, 2)
		if o == nil || n < 0 || n > 0x31 {
			return 0
		}
		w.scripts().obj(o.Index).perm[n] = 1
		return 1
	}},
	{"PermissionRemove", eq(2), func(w *World, l *lua.State) int { // 0x4abb30
		o, n := w.luaUser(l, 1), luaInt(l, 2)
		if o == nil || n < 0 || n > 0x31 {
			return 0
		}
		w.scripts().obj(o.Index).perm[n] = 0
		return 1
	}},
	{"PKLevelSend", eq(2), func(w *World, l *lua.State) int { // 0x4abbb0 -> FUN_004db9d0
		o, lvl := w.luaUser(l, 1), luaInt(l, 2)
		if o == nil {
			return 0
		}
		p := packetPKLevel(o.Index, lvl)
		w.send(o, p)
		w.broadcast(o, p)
		return 1
	}},
	// 0x4abc30: a type 0..3 post through FUN_005127f0/00512920/00512a50/
	// 00512b80; the packet is not reconstructed.
	{"PostSend", eq(4), func(w *World, l *lua.State) int { w.log.Debug("lua PostSend not reproduced"); return 1 }},
	{"QuestStateCheck", eq(2), func(w *World, l *lua.State) int { // 0x4abd50 -> FUN_004dd740(obj, quest) == 2
		o, q := w.luaUser(l, 1), luaInt(l, 2)
		if o == nil {
			return 0
		}
		if q >= 0 && o.P.Char != nil && questState(o, q) == 2 {
			return pushInt(l, 1)
		}
		return pushInt(l, 0)
	}},
	{"RandomGetNumber", eq(1), func(w *World, l *lua.State) int { // 0x4abe00: rand % max(n, 1)
		n := luaInt(l, 1)
		if n < 1 {
			n = 1
		}
		return pushInt(l, monsterRand()%n)
	}},
	{"SkinChangeSend", eq(2), func(w *World, l *lua.State) int { // 0x4abe70: obj+0x314 and the re-show
		o := w.luaUser(l, 1)
		skin := luaInt(l, 2)
		if o == nil || o.P.Char == nil {
			return 0
		}
		gmOf(o).Skin = skin
		w.reshow(o)
		return 1
	}},
	{"UserDisconnect", eq(1), func(w *World, l *lua.State) int { // 0x4abf10 -> FUN_00512780
		o := w.luaObj(l, 1)
		if o == nil {
			return 0
		}
		if o.conn != nil {
			w.closeClient(o, "script")
		}
		return 1
	}},
	{"UserGameLogout", eq(2), func(w *World, l *lua.State) int { // 0x4abf70: obj+0x0b type 0..2, obj+0x0a = 6
		o, typ := w.luaObj(l, 1), luaInt(l, 2)
		if o == nil {
			return 0
		}
		if o.P != nil {
			o.P.LogoutType, o.P.LogoutCountdown = byte(min(max(typ, 0), 2)), 6
		}
		return 1
	}},
	{"UserCalcAttribute", eq(1), func(w *World, l *lua.State) int { // 0x4ac000 -> FUN_004c9790
		o := w.luaUser(l, 1)
		if o == nil || o.P.Char == nil {
			return 0
		}
		w.calcCharacter(o)
		return 1
	}},
	// 0x4ac070: FUN_0050be30, FUN_004dba70 and the save FUN_00475c30.
	{"UserInfoSend", eq(1), func(w *World, l *lua.State) int {
		o := w.luaUser(l, 1)
		if o == nil || o.P.Char == nil {
			return 0
		}
		w.saveCharacter(o, nil)
		return 1
	}},
	{"UserActionSend", eq(3), func(w *World, l *lua.State) int { // 0x4ac100 -> FUN_004da300: not reproduced
		if w.luaUser(l, 1) == nil || w.luaUser(l, 2) == nil {
			return 0
		}
		return 1
	}},
	{"UserSetAccountLevel", eq(3), func(w *World, l *lua.State) int { // 0x4ac1c0 -> JS 0x11 + 0x05
		o, level, days := w.luaUser(l, 1), luaInt(l, 2), luaInt(l, 3)
		if o == nil {
			return 0
		}
		w.setAccountLevel(o, level, time.Duration(days)*24*time.Hour)
		return 1
	}},

	// LuaSQL (0x4ac260..0x4ac9b0): ODBC to the MuOnline database through
	// SQLConnect(dsn, user, password). There is no ODBC here, so every
	// connection fails as when the DSN is missing.
	{"SQLConnect", eq(3), luaSQLConnect("SQLConnect")},
	{"SQLDisconnect", eq(0), func(w *World, l *lua.State) int { return 1 }},
	{"SQLCheck", eq(0), constant(0)},
	{"SQLQuery", eq(1), constant(0)},
	{"SQLClose", eq(0), func(w *World, l *lua.State) int { return 1 }},
	{"SQLFetch", eq(0), constant(0)},
	{"SQLGetResult", eq(1), constant(0)},
	{"SQLGetNumber", eq(1), constant(0)},
	{"SQLGetSingle", eq(1), func(w *World, l *lua.State) int { l.PushNumber(0); return 1 }},
	{"SQLGetString", eq(1), func(w *World, l *lua.State) int { return pushStr(l, "") }},
	{"SQLAsyncConnect", eq(3), luaSQLConnect("SQLAsyncConnect")},
	{"SQLAsyncDisconnect", eq(0), func(w *World, l *lua.State) int { return 1 }},
	{"SQLAsyncCheck", eq(0), constant(0)},
	{"SQLAsyncQuery", atl(1), func(w *World, l *lua.State) int { return 1 }}, // queued only while connected
	{"SQLAsyncGetResult", eq(1), constant(0)},
	{"SQLAsyncGetNumber", eq(1), constant(0)},
	{"SQLAsyncGetSingle", eq(1), func(w *World, l *lua.State) int { l.PushNumber(0); return 1 }},
	{"SQLAsyncGetString", eq(1), func(w *World, l *lua.State) int { return pushStr(l, "") }},
}

func luaSQLConnect(name string) func(*World, *lua.State) int {
	return func(w *World, l *lua.State) int {
		w.log.Error(fmt.Sprintf("[LuaFunction][%s] Could not connect to database [%s][%s][%s]",
			name, luaStr(l, 1), luaStr(l, 2), luaStr(l, 3)))
		return pushInt(l, 0)
	}
}

// luaLevelArg reads an item level argument (lua_tonumberx + _ftol).
func luaLevelArg(l *lua.State, i int) int {
	n, _ := l.ToNumber(i)
	return int(n)
}

// luaItemSpec reads the item columns of ItemDropEx (from arg 5) and
// ItemGiveEx (from arg 2): index, level, durability, skill, luck, option,
// excellent, set, JoH, 380 option, 5 sockets, socket bonus, duration.
func luaItemSpec(l *lua.State, first int) dropSpec {
	a := func(k int) int { return luaInt(l, first+k) }
	return dropSpec{
		Index: a(0), Level: a(1), Dur: a(2), Skill: a(3), Luck: a(4), Option: a(5),
		Exc: byte(a(6)), Set: byte(a(7)), OptionEx: byte(a(9)), Duration: max(a(16), 0),
	}
}

// luaEachUser is the loop of FUN_004ab3b0 / FUN_004c0ab0: playing players
// without a logout countdown (obj+0x0a < 1) or server move (obj+0x964).
func (w *World) luaEachUser(f func(o *Object)) {
	for i := PlayerStart; i < MaxObjects; i++ {
		if o := w.objects[i]; o.State == StatePlaying && o.IsPlayer() && canPlayerAct(o) {
			f(o)
		}
	}
}

func (w *World) luaParty(number int) *party {
	if number < 0 || number >= MaxObjects || w.parties == nil {
		return nil
	}
	pt := w.parties.parties[number]
	if pt == nil || pt.count == 0 {
		return nil
	}
	return pt
}

// luaMessage is FUN_004b3470 with the server language CServerInfo+0x70,
// which nothing in the binary writes (0, Eng).
func (w *World) luaMessage(n int) string {
	if s, ok := w.messages[0][n]; ok {
		return s
	}
	return fmt.Sprintf("Could not find message: %d, lang: %d!", n, 0)
}

// inventoryItemCount is FUN_004948f0: items of index (and level, -1 any);
// a stackable item counts its durability.
func (w *World) inventoryItemCount(o *Object, index, level int) int {
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

// packetWindowTitle is C1 84 F3 E8 text[128] (FUN_004dcad0): the client
// window caption. The original passes the script text to vsprintf_s as
// the format (only "%%" is meaningful without arguments) into 128 bytes.
func packetWindowTitle(text string) []byte {
	text = strings.ReplaceAll(text, "%%", "%")
	if len(text) > 0x7F {
		text = text[:0x7F]
	}
	p := packet.New(0xC1, 0xF3, 0xE8, 0x84)
	copy(p[4:], text)
	return p
}

// writeProfileString is WritePrivateProfileStringA: replace or add key in
// [section], creating the section and the file when missing.
func writeProfileString(path, section, key, value string) error {
	b, _ := readFileIfExists(path)
	lines := strings.Split(strings.ReplaceAll(string(b), "\r\n", "\n"), "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	entry := key + "=" + value
	sec := -1
	for i, ln := range lines {
		t := strings.TrimSpace(ln)
		if strings.HasPrefix(t, "[") && strings.HasSuffix(t, "]") {
			if sec >= 0 { // end of the section without the key
				lines = append(lines[:i], append([]string{entry}, lines[i:]...)...)
				return writeFileLines(path, lines)
			}
			if strings.EqualFold(strings.TrimSpace(t[1:len(t)-1]), section) {
				sec = i
			}
			continue
		}
		if sec >= 0 {
			if k, _, ok := strings.Cut(t, "="); ok && strings.EqualFold(strings.TrimSpace(k), key) {
				lines[i] = entry
				return writeFileLines(path, lines)
			}
		}
	}
	if sec < 0 {
		lines = append(lines, "["+section+"]")
	}
	lines = append(lines, entry)
	return writeFileLines(path, lines)
}

func readFileIfExists(path string) ([]byte, error) {
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	return b, err
}

func writeFileLines(path string, lines []string) error {
	return os.WriteFile(path, []byte(strings.Join(lines, "\r\n")+"\r\n"), 0o644)
}
