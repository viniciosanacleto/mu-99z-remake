-- EventEntryCount (WZ_GetEventEntryInfo / WZ_SetEventEntryInfo): daily
-- entries per event, reset on the first read of a new day.
CREATE TABLE event_entry_count (
  account   citext NOT NULL,
  name      citext NOT NULL,
  bc        integer NOT NULL DEFAULT 0,
  cc        integer NOT NULL DEFAULT 0,
  ds        integer NOT NULL DEFAULT 0,
  dg        integer NOT NULL DEFAULT 0,
  it        integer NOT NULL DEFAULT 0,
  ig        integer NOT NULL DEFAULT 0,
  last_date timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (account, name)
);
