-- Castle Siege (MuCastle_* tables of the original database, written by the
-- DataServer for the CastleSiege GameServer; docs/server-plan/10d-castle-siege.md).
-- Names compare case-insensitively like the SQL Server collation.

-- MuCastle_DATA: schedule, owner, money and tax rates per map server group.
CREATE TABLE mu_castle_data (
  map_svr_group          integer PRIMARY KEY,
  siege_start_date       date    NOT NULL,
  siege_end_date         date    NOT NULL,
  siege_guildlist_setted boolean NOT NULL DEFAULT false,
  siege_ended            boolean NOT NULL DEFAULT false,
  castle_occupy          boolean NOT NULL DEFAULT false,
  owner_guild            citext  NOT NULL DEFAULT '' CHECK (length(owner_guild) <= 8),
  money                  bigint  NOT NULL DEFAULT 0,
  tax_rate_chaos         integer NOT NULL DEFAULT 0,
  tax_rate_store         integer NOT NULL DEFAULT 0,
  tax_hunt_zone          integer NOT NULL DEFAULT 0
);

-- MuCastle_NPC: the gates and guardian statues kept between sieges.
CREATE TABLE mu_castle_npc (
  map_svr_group  integer  NOT NULL,
  npc_number     integer  NOT NULL,
  npc_index      integer  NOT NULL,
  npc_df_level   integer  NOT NULL,
  npc_rg_level   integer  NOT NULL,
  npc_maxhp      integer  NOT NULL,
  npc_hp         integer  NOT NULL,
  npc_x          smallint NOT NULL,
  npc_y          smallint NOT NULL,
  npc_dir        smallint NOT NULL,
  npc_createdate timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (map_svr_group, npc_number, npc_index)
);

-- MuCastle_REG_SIEGE: guilds registered for the next siege and their
-- Sign of Lord marks.
CREATE TABLE mu_castle_reg_siege (
  map_svr_group   integer  NOT NULL,
  reg_siege_guild citext   NOT NULL CHECK (length(reg_siege_guild) BETWEEN 1 AND 8),
  reg_marks       integer  NOT NULL DEFAULT 0,
  is_giveup       smallint NOT NULL DEFAULT 0,
  seq_num         integer  NOT NULL,
  PRIMARY KEY (map_svr_group, reg_siege_guild)
);

-- MuCastle_SIEGE_GUILDLIST: the guilds of the current siege (guild_id is
-- the side: 1 defenders, 2..4 attackers).
CREATE TABLE mu_castle_siege_guildlist (
  map_svr_group  integer NOT NULL,
  guild_name     citext  NOT NULL CHECK (length(guild_name) BETWEEN 1 AND 8),
  guild_id       integer NOT NULL,
  guild_involved boolean NOT NULL,
  guild_score    integer NOT NULL
);
CREATE INDEX mu_castle_siege_guildlist_group_idx ON mu_castle_siege_guildlist (map_svr_group);

-- MuCastle_MONEY_STATISTICS: every WZ_CS_ModifyMoney call.
CREATE TABLE mu_castle_money_statistics (
  map_svr_group integer     NOT NULL,
  log_date      timestamptz NOT NULL DEFAULT now(),
  money_change  bigint      NOT NULL
);
