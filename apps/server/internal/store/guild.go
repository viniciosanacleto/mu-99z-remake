package store

import "context"

// GuildStore replaces the guild SQL of the original DataServer (CGuildManager,
// 0x4158a0..0x417a70; docs/server-plan/09b-guild.md). The DataServer kept
// every guild in memory and wrote through to the database; the game module
// does the same with this interface as the write-through layer.
type GuildStore interface {
	// LoadGuilds is the start-up load ("SELECT * FROM Guild" then
	// "SELECT * FROM GuildMember"): the master is member 0, the other
	// members follow in table order.
	LoadGuilds(ctx context.Context) ([]Guild, error)
	// GuildCreate is WZ_GuildCreate followed by the G_Mark and G_Status
	// updates of FUN_004166b0. code is the procedure's @ErrorCode: 0 ok
	// (number is the new Guild.Number), 1 the Guild insert failed (name
	// taken), 2 the GuildMember insert failed (master already in a guild).
	GuildCreate(ctx context.Context, name, master string, mark []byte, status int) (number, code int, err error)
	// GuildDelete is "DELETE FROM Guild" + "DELETE FROM GuildMember" by
	// G_Name (FUN_004169c0).
	GuildDelete(ctx context.Context, name string) error
	// GuildMemberAdd is the "SELECT * FROM GuildMember WHERE Name" check and
	// the INSERT (G_Level 1) of FUN_00416af0; false when the name already
	// has a GuildMember row.
	GuildMemberAdd(ctx context.Context, guild, name string, status int) (bool, error)
	// GuildMemberDelete is "DELETE FROM GuildMember WHERE Name" (FUN_00416cd0).
	GuildMemberDelete(ctx context.Context, name string) error
	// SetGuildMemberStatus is "UPDATE GuildMember SET G_Status" (FUN_004179a0).
	SetGuildMemberStatus(ctx context.Context, name string, status int) error
	// SetGuildType / SetGuildNotice / SetGuildScore are the G_Type, G_Notice
	// and G_Score updates by G_Name (FUN_004178c0, FUN_00417790, FUN_004176b0).
	SetGuildType(ctx context.Context, guild string, typ int) error
	SetGuildNotice(ctx context.Context, guild, notice string) error
	SetGuildScore(ctx context.Context, guild string, score int) error
	// SetGuildUnion / SetGuildRival are "UPDATE Guild SET G_Union|G_Rival=%d
	// WHERE Number IN (...)" (FUN_00416d70, FUN_00417010, FUN_00417200).
	SetGuildUnion(ctx context.Context, union int, numbers ...int) error
	SetGuildRival(ctx context.Context, rival int, numbers ...int) error
}

// Guild is a Guild row with its GuildMember rows.
type Guild struct {
	Number  int
	Name    string // G_Name (8 characters)
	Master  string // G_Master
	Mark    []byte // G_Mark (32 bytes)
	Score   int    // G_Score
	Notice  string // G_Notice (60 bytes)
	Type    int    // G_Type
	Union   int    // G_Union (number of the union master guild, 0 = none)
	Rival   int    // G_Rival
	Members []GuildMember
}

// GuildMember is a GuildMember row (G_Level is always 1).
type GuildMember struct {
	Name   string
	Status int // G_Status: 0 member, 0x20 battle master, 0x40 assistant, 0x80 master
}

// Guild statuses (G_Status / obj+0x30c).
const (
	GuildMemberStatus       = 0x00
	GuildBattleMasterStatus = 0x20
	GuildAssistantStatus    = 0x40
	GuildMasterStatus       = 0x80
)
