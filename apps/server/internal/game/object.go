// Package game is one game server instance (the original GameServer /
// CastleSiege): client sessions, the object table and all gameplay. All game
// state is owned by a single goroutine (World.loop), mirroring the single
// logic thread of the original (docs/server-plan/01-arquitetura-alvo.md).
package game

import (
	"time"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/auth"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/effect"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/item"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/store"
)

// Object table layout of the original (DAT_0107a7dc).
const (
	MaxObjects   = 0x57E4 // 22500
	PlayerStart  = 0x53FC // 21500: players use 21500..22499
	SummonStart  = 0x5014 // 20500: monsters/NPCs scan from here when looking for players
	ViewportSize = 75
)

// ConnState is obj+0x04.
type ConnState int

const (
	StateEmpty     ConnState = 0
	StateConnected ConnState = 1 // before login
	StateLogged    ConnState = 2 // character selection
	StatePlaying   ConnState = 3
)

// ObjType is obj+0x6c.
type ObjType int16

const (
	TypeNone    ObjType = 0
	TypePlayer  ObjType = 1
	TypeMonster ObjType = 2
	TypeNPC     ObjType = 3
)

// Live states of obj+0x26c.
const (
	LiveNew      = 1 // just appeared (bit 7 of the index in viewport packets)
	LiveNormal   = 2
	LiveDying    = 4
	LiveRemoved  = 8
	LiveTeleport = 0x20
)

// Viewport entry states.
const (
	VPEmpty   byte = 0
	VPNew     byte = 1
	VPVisible byte = 2
	VPRemove  byte = 3
)

// VPEntry is one 4-byte viewport slot {state, index u16, type}.
type VPEntry struct {
	State byte
	Index int
	Type  byte
}

// Inventory layout (108 slots): 12 equipment slots, the 8x8 bag and the
// 32-slot personal shop area.
const (
	InventorySlots = 108
	EquipSlots     = 12
	BagStart       = 12
	BagEnd         = 76 // exclusive
	ShopStart      = 76
	MagicSlots     = 60
)

// Skill is one MagicList entry (CSkill: +4 level, +8 index).
type Skill struct {
	Index int // -1 = empty
	Level byte
}

// Object is the common part of every object.
type Object struct {
	Index int
	State ConnState
	Type  ObjType
	Class int // monster class (obj+0x70) / character class

	Live        int
	Alive       bool
	Teleporting bool // obj+0x284

	Map     int
	X, Y    int
	TX, TY  int
	Dir     int
	PKLevel int

	ActionState    byte // obj+0x271 (merged into CharSet[0] when sent)
	PoseState      byte // obj+0x25c
	ViewSkillState uint32
	CharSet        [18]byte

	VP     [ViewportSize]VPEntry // objects I see (obj+0x3b8)
	VP2    [ViewportSize]VPEntry // objects that see me (obj+0x3bc)
	VPItem [ViewportSize]VPEntry // map items I see (obj+0x3c0)

	conn *conn
	P    *Player

	// Extension state owned by the gameplay modules (one file each).
	Move  *moveState    // move.go     (path walking, D3/DF)
	Mon   *monsterState // monster*.go (monsters and NPCs)
	Eff   *effect.State // effect.go (CEffect slots obj+0x5fc)
	Skill *skillState   // skill.go (skill delays, combo, Nova, delayed hits)
	Duel  *duelState    // duel.go (obj+0x43c..0x450)
	CS    *csObjState   // castlesiege.go (obj+0x974..0x99c)
}

// Player holds the per-session data of a connected client.
type Player struct {
	IP          string
	HWID        string
	ClientType  byte // obj+0x56 (F3 09 [0x31])
	Verified    bool // obj+0x18
	ConnectedAt time.Time
	LastPacket  time.Time

	friendCount  int                     // obj+0x24b0
	questSent    bool                    // obj+0x5c0: A0 list already sent
	entries      store.EventEntry        // obj+0x2098..: today's event entries (event_entry.go)
	archer       *goldenArcherState      // obj+0x6a8 / 0x6ac (goldenarcher.go)
	hasGate      bool                    // obj+0x2ac: owns a Kalima gate (kalima.go)
	gate         int                     // obj+0x2b0: index of that gate
	autoPoints   *autoPointState         // obj+0x63c..0x650 (command_player.go)
	cmd          *cmdState               // obj+0x654..0x69c, 0x24ac, 0x24ec (command_player2.go)
	cust         *customState            // offline modes, commands 50..65, hard-coded ones (command_custom.go)
	gm           *gmState                // obj+0x314 skin, custom event flags (command_gm.go, customevent*.go)
	cash         *cashState              // jewel bank, XShop / lucky wheel requests (xshop.go)
	recvSerial   byte                    // DAT_01020344[index*8], starts at 0xFF
	hackCounters map[[2]int]*hackCounter // obj+0x604 / obj+0x608

	LoginPending  bool // obj+0x08
	LoginAttempts int  // obj+0x09
	ClientTick    uint32

	Session        *auth.Session
	Moving         bool
	Account        store.Account
	CharListLoaded bool // obj+0x0c
	ClassFlag      byte // highest creatable class shown in F3 00 (2/3/4)

	LogoutCountdown int // obj+0x0a
	LogoutType      byte

	Char      *store.Character
	Inventory [InventorySlots]item.Item
	Skills    [MagicSlots]Skill
	NextExp   uint32
	AddLife   int
	AddMana   int
	AddBP     int
	LastMove  time.Time
	LastSave  time.Time

	// Extension state owned by the gameplay modules (one file each).
	Stats *statsState // stats.go     (CalcCharacter results, level up)
	Items *itemsState // inventory*.go (open shop/warehouse, trade box...)
	Party *partyState // party.go
	Trade *tradeState // trade.go
	Guild *guildState // guild.go     (obj+0x2f8..0x30c, pending request)
	Chaos *chaosState // chaos.go     (chaos box, mix, inventory transaction)
	PShop *pshopState // pshop.go     (personal shop)
	Pet   *petState   // pet.go       (Dark Raven)
}

func newObject(index int) *Object {
	o := &Object{Index: index}
	o.reset()
	return o
}

func (o *Object) reset() {
	idx := o.Index
	*o = Object{Index: idx}
	for i := range o.VP {
		o.VP[i].Index, o.VP2[i].Index, o.VPItem[i].Index = -1, -1, -1
	}
}

// IsPlayer reports whether the object is a connected player.
func (o *Object) IsPlayer() bool { return o.Type == TypePlayer && o.P != nil }
