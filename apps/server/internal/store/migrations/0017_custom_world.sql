-- WizardTeam world customs (docs/server-plan/10f-gm-eventos-custom.md,
-- block 12).
--
-- MonsterKillCount (CustomMonsterKillCount.txt): DS 0x50 (DataServer
-- FUN_0040e6d0) reads the row of (Name, MonsterClass), adds one and
-- writes it back, or inserts it with 1; the new count goes back to the
-- GameServer in C1 20 50.
CREATE TABLE monster_kill_count (
  name          citext  NOT NULL,
  monster_class integer NOT NULL,
  kill_count    integer NOT NULL DEFAULT 0,
  PRIMARY KEY (name, monster_class)
);

-- DailyReward (DailyReward.txt): the days of the month an account has
-- claimed. The DataServer reads "SELECT * FROM DailyReward WHERE Acc='%s'
-- AND month = MONTH(GETDATE())" with the character (FUN_0040c4c0) and DS
-- 0x68 (FUN_00410980) inserts (acc, name, date) for each claimed day that
-- is not there yet. Only the month number is kept, as in the original.
CREATE TABLE daily_reward (
  account citext      NOT NULL,
  name    varchar(10) NOT NULL,
  day     smallint    NOT NULL CHECK (day BETWEEN 1 AND 31),
  month   smallint    NOT NULL CHECK (month BETWEEN 1 AND 12),
  PRIMARY KEY (account, month, day)
);
