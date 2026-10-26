-- CustomJewelBank (docs/server-plan/10e-customs.md):
-- the jewels each account keeps in the bank (DS 0xF7 04 / 05). One column
-- per type in the order of the F3 F5 / F3 F6 packets; the DataServer adds
-- the deltas the GameServer sends (UPDATE ... += or INSERT).
CREATE TABLE custom_jewel_bank (
  account    citext  PRIMARY KEY,
  chaos      integer NOT NULL DEFAULT 0,
  life       integer NOT NULL DEFAULT 0,
  soul       integer NOT NULL DEFAULT 0,
  bless      integer NOT NULL DEFAULT 0,
  creation   integer NOT NULL DEFAULT 0,
  guardian   integer NOT NULL DEFAULT 0,
  harmony    integer NOT NULL DEFAULT 0,
  gem_stone  integer NOT NULL DEFAULT 0,
  low_stone  integer NOT NULL DEFAULT 0,
  high_stone integer NOT NULL DEFAULT 0
);
