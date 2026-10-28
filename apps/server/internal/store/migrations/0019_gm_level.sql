-- GM level of the character (obj+0x250 Authority). The original reads it
-- from Util/GameMaster.txt (FUN_0047db10, account + character name); the
-- rewrite keeps it here so a change applies on the next login without a
-- restart. Only SetGMLevel writes it (the character save leaves it alone).
ALTER TABLE characters ADD COLUMN gm_level smallint NOT NULL DEFAULT 0;
