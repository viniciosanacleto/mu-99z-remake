-- EventGoldenArcher: Renas handed to the Golden Archer per account.
CREATE TABLE golden_archer (
  account     citext PRIMARY KEY,
  rena_count  integer NOT NULL DEFAULT 0,
  stone_count integer NOT NULL DEFAULT 0
);
