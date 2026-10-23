-- Rankings written by the DataServer (DS 0x3C..0x40, FUN_0040f470..
-- FUN_0040fb80): duel wins/losses and the accumulated scores of Blood
-- Castle, Chaos Castle, Devil Square and Illusion Temple, one row per
-- character name. A report adds to the row or creates it.
CREATE TABLE ranking_duel (
  name       citext PRIMARY KEY,
  win_score  integer NOT NULL DEFAULT 0,
  lose_score integer NOT NULL DEFAULT 0
);

CREATE TABLE ranking_event (
  event  smallint NOT NULL, -- 0x3D BC, 0x3E CC, 0x3F DS, 0x40 IT (the DS head)
  name   citext NOT NULL,
  score  integer NOT NULL DEFAULT 0,
  PRIMARY KEY (event, name)
);
