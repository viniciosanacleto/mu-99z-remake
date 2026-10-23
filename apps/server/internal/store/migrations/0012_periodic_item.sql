-- PeriodicItem (CPeriodicItem, DataServer FUN_004189b0 load / FUN_00418d30
-- save): the expiry time (Unix seconds) of each periodic item by serial.
-- A serial without a row is inserted with time 0 when loaded, so the item
-- expires on the next check.
CREATE TABLE periodic_item (
  item_serial bigint PRIMARY KEY,
  time        bigint NOT NULL DEFAULT 0
);
