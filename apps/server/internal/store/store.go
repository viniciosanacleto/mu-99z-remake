// Package store is the persistence layer that replaces the original
// DataServer and the SQL used by the JoinServer
// (docs/server-plan/05-persistencia-data-server.md, 06-schema-postgresql.md).
// Blob columns keep the original byte layouts so the game code can use the
// same item/skill encodings as the original GameServer.
package store

import (
	"context"
	"errors"
	"time"
)

// Blob sizes of the original MuOnline database.
const (
	InventorySize   = 1728 // 108 items x 16 bytes
	MagicListSize   = 180  // 60 skills x 3 bytes
	EffectListSize  = 208
	QuestSize       = 50
	WarehouseSize   = 1920 // 120 items x 16 bytes
	MaxCharacters   = 5    // GameID1..5
	ItemDBSize      = 16
	SkillKeysSize   = 20
	SummaryItems    = 12 // DataServer FUN_0040b310 sends the first 12 inventory items
	GuildStatusNone = 0xFF
)

var (
	ErrNotFound  = errors.New("store: not found")
	ErrDuplicate = errors.New("store: duplicate")
)

// Account mirrors MEMB_INFO + AccountCharacter fields used by the game.
type Account struct {
	ID             int64
	Login          string
	PasswordHash   string
	PersonalID     string // sno__numb
	BlockCode      int    // bloc_code
	Level          int    // AccountLevel (AL0..AL3)
	LevelExpiresAt time.Time
	Lock           int // MEMB_INFO.Lock
	ExtClass       int // AccountCharacter.ExtClass
	MoveCount      int // AccountCharacter.MoveCnt
}

// CharacterSummary is one entry of the character selection list
// (DataServer 0x01 reply).
type CharacterSummary struct {
	Slot        int
	Name        string
	Level       int
	Class       byte
	CtlCode     byte
	Inventory   []byte // first SummaryItems*16 bytes of the inventory
	GuildStatus byte   // G_Status, GuildStatusNone when not in a guild
}

// Character is a full Character row.
type Character struct {
	ID             int64
	AccountID      int64
	Slot           int
	Name           string
	Class          byte
	Level          int
	LevelUpPoints  int
	Experience     int64
	Strength       int
	Dexterity      int
	Vitality       int
	Energy         int
	Leadership     int
	Money          int64
	Life, MaxLife  float32
	Mana, MaxMana  float32
	BP, MaxBP      float32
	Shield         float32
	MaxShield      float32
	Map            int
	X, Y, Dir      int
	PKCount        int
	PKLevel        int
	PKTime         int
	CtlCode        byte
	Inventory      []byte
	MagicList      []byte
	EffectList     []byte
	Quest          []byte
	ChatLimitTime  int
	FruitAddPoints int
	FruitSubPoints int
	Married        bool
	MarryCouple    string
	Resets         int
	MasterResets   int
	IsNew          bool
	Patente        int // Character.Patente (obj+0x2500), written only by SetPatente
	GMLevel        int // gm_level (Util/GameMaster.txt in the original), written only by SetGMLevel
}

// CreateResult values are the WZ_CreateCharacter results forwarded to the
// client in F3 01 (0 name exists, 1 ok, 2 error, 3 no free slot).
type CreateResult byte

const (
	CreateNameExists CreateResult = 0
	CreateOK         CreateResult = 1
	CreateError      CreateResult = 2
	CreateNoSlot     CreateResult = 3
)

// Warehouse is one vault (warehouse / ExtWarehouse.Number).
type Warehouse struct {
	AccountID int64
	Number    int
	Items     []byte
	Money     int64
	Password  int
}

// Options mirrors OptionData.
type Options struct {
	SkillKeys  []byte
	GameOption byte
	QKey, WKey byte
	EKey       byte
	ChatWindow byte
}

// Store is implemented by the PostgreSQL store and the in-memory store.
type Store interface {
	AccountByLogin(ctx context.Context, login string) (*Account, error)
	CreateAccount(ctx context.Context, login, passwordHash, personalID string) (*Account, error)
	SetPasswordHash(ctx context.Context, accountID int64, hash string) error
	SetAccountLevel(ctx context.Context, accountID int64, level int, expires time.Time) error
	SetAccountLock(ctx context.Context, accountID int64, lock int) error
	SetExtClass(ctx context.Context, accountID int64, extClass int) error
	// ConnectAccount / DisconnectAccount replace WZ_CONNECT_MEMB / WZ_DISCONNECT_MEMB.
	ConnectAccount(ctx context.Context, accountID int64, serverName, ip string) error
	DisconnectAccount(ctx context.Context, accountID int64) error
	ResetSessions(ctx context.Context) error

	ListCharacters(ctx context.Context, accountID int64) ([]CharacterSummary, error)
	CreateCharacter(ctx context.Context, accountID int64, name string, class byte) (CreateResult, int, error)
	DeleteCharacter(ctx context.Context, accountID int64, name string) (bool, error)
	LoadCharacter(ctx context.Context, accountID int64, name string) (*Character, error)
	SaveCharacter(ctx context.Context, c *Character) error
	SaveInventory(ctx context.Context, characterID int64, inventory []byte) error
	CharacterExists(ctx context.Context, name string) (bool, error)

	LoadWarehouse(ctx context.Context, accountID int64, number int) (*Warehouse, error)
	SaveWarehouse(ctx context.Context, w *Warehouse) error

	LoadOptions(ctx context.Context, characterID int64) (*Options, error)
	SaveOptions(ctx context.Context, characterID int64, o *Options) error

	NextItemSerial(ctx context.Context) (uint32, error)
	Close()
}

// EmptyItems returns n empty 16-byte item slots (0xFF filled), the encoding
// the original uses for an empty slot.
func EmptyItems(n int) []byte {
	b := make([]byte, n*ItemDBSize)
	for i := range b {
		b[i] = 0xFF
	}
	return b
}
