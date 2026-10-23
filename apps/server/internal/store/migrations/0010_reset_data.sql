-- ResetData (WZ_GetResetInfo / WZ_SetResetInfo and the master variants):
-- reset counters per day, week and month of each character. A counter
-- reads as 0 once its date is in an older day/week/month; the date moves
-- to now when the counter is written again. The original columns have no
-- visible defaults; new rows start at 0 with the dates set to now.
CREATE TABLE reset_data (
  account           citext NOT NULL,
  name              citext NOT NULL,
  reset_day         integer NOT NULL DEFAULT 0,
  reset_wek         integer NOT NULL DEFAULT 0,
  reset_mon         integer NOT NULL DEFAULT 0,
  reset_date_day    timestamptz NOT NULL DEFAULT now(),
  reset_date_wek    timestamptz NOT NULL DEFAULT now(),
  reset_date_mon    timestamptz NOT NULL DEFAULT now(),
  master_reset_day  integer NOT NULL DEFAULT 0,
  master_reset_wek  integer NOT NULL DEFAULT 0,
  master_reset_mon  integer NOT NULL DEFAULT 0,
  master_date_day   timestamptz NOT NULL DEFAULT now(),
  master_date_wek   timestamptz NOT NULL DEFAULT now(),
  master_date_mon   timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (account, name)
);
