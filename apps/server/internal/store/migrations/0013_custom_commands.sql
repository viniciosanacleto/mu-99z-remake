-- WizardTeam custom commands (docs/server-plan/10e-customs-wizardteam.md).
--
-- GiftData (DS 0x0F 03, /gift): how many times each character took each
-- CustomGift.txt gift. The original row is created with Count 0 on the
-- first request and Count+1 is written only when Count < MaxCount.
CREATE TABLE gift_data (
  name  citext  NOT NULL,
  idx   integer NOT NULL,
  count integer NOT NULL DEFAULT 0,
  PRIMARY KEY (name, idx)
);

-- CustomPick (DS 0x13, /pick, /pickset): the 20 x 20-byte text filters
-- and the four switches of the auto pick of each character.
CREATE TABLE custom_pick (
  name     citext PRIMARY KEY,
  list     bytea   NOT NULL,
  money    integer NOT NULL DEFAULT 0,
  jewel    integer NOT NULL DEFAULT 0,
  item_exc integer NOT NULL DEFAULT 0,
  item_set integer NOT NULL DEFAULT 0
);
