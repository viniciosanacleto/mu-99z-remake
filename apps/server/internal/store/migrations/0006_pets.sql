-- Dark Horse / Dark Raven level and experience (T_PetItem_Info of the
-- original database, written by DataServer heads 0x07, 0x09 and 0x34).
-- Rows are keyed by the item serial, like the original.
CREATE TABLE pet_item_info (
  item_serial bigint PRIMARY KEY,
  pet_level   smallint NOT NULL DEFAULT 0,
  pet_exp     bigint   NOT NULL DEFAULT 0
);
