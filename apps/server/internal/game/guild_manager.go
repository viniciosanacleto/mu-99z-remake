package game

import (
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/guild"
)

// The DataServer half of the guild system (CGuildManager of DataServer.exe,
// 0x4158a0..0x417a70, reached by the GameServer with C1 A0 xx). The
// original DataServer kept every guild in memory, wrote through to SQL and
// broadcast the results to every GameServer (FUN_00415760); here one
// guild.Manager per Hub plays that role for every instance of the process
// and store.GuildStore is the SQL. docs/server-plan/09b-guild.md.

// guildMgr returns the process-wide guild manager of the hub.
func (w *World) guildMgr() *guild.Manager { return w.hub.guilds }
