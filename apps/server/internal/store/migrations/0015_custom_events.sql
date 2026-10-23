-- WizardTeam custom events (docs/server-plan/10f-gm-eventos-custom.md).
--
-- Character.Patente (obj+0x2500, sent in F3 03 [112]): the rank badge of
-- the last winner of each custom event (/patente and the event rewards).
-- DS F3 E7 runs "UPDATE Character SET Patente = 0 WHERE Patente = N" and
-- then sets N on the winner, so each value has one holder.
ALTER TABLE characters ADD COLUMN IF NOT EXISTS patente integer NOT NULL DEFAULT 0;

-- The custom ranking columns of Character (TopDeath, TopTheft, TopRoleta,
-- TopDupla, topsurvivor, Duelsm..Duelall) raised by DS 0x41
-- ("UPDATE Character SET %s = %s + %d WHERE Name='%s'"), kept as rows.
CREATE TABLE custom_rank (
  name  citext      NOT NULL,
  rank  varchar(16) NOT NULL,
  score integer     NOT NULL DEFAULT 0,
  PRIMARY KEY (name, rank)
);
