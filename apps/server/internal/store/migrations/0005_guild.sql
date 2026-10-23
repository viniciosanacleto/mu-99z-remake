-- Guilds (Guild / GuildMember of the original database, written by the
-- DataServer CGuildManager; docs/server-plan/09b-guild.md). 0001 created the
-- tables; this migration adds what the DataServer semantics need.

-- GuildMember rows are read back in table order after the master
-- (FUN_00415b90 fills the member slots in "SELECT * FROM GuildMember" order).
ALTER TABLE guild_members ADD COLUMN seq bigserial;

-- G_Name is char(8) in the original; names of 3..8 characters reach it.
ALTER TABLE guilds DROP CONSTRAINT IF EXISTS guilds_name_check;
ALTER TABLE guilds ADD CONSTRAINT guilds_name_check CHECK (length(name) BETWEEN 1 AND 8);

-- G_Mark defaults to 32 zero bytes when a row is inserted without it.
ALTER TABLE guilds ALTER COLUMN mark SET DEFAULT decode(repeat('00', 32), 'hex');

CREATE INDEX guild_members_guild_idx ON guild_members (guild_id, seq);
CREATE INDEX guilds_union_idx ON guilds (union_id) WHERE union_id <> 0;
CREATE INDEX guilds_rival_idx ON guilds (rival) WHERE rival <> 0;
