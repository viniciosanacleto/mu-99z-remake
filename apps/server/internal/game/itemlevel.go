package game

// CItemLevel (Item/ItemLevel.txt, DAT_01241a4c, reader FUN_00493010): the
// names of items at a given level (14,11 +1 "Star of Sacred Birth"...).
// FUN_00494a40 returns the first row with the same index and level, then
// the Item.txt name of the index, then "Unknown". Users: the marriage
// messages 0x290/0x298 (FUN_00465e70, FUN_00466170, FUN_00466490, with the
// item level of the CustomMarriage row), the CustomPick filters
// (FUN_00455c80), and the Chaos Machine +13 announcement (FUN_005120b0,
// chaos_announce.go).

// itemLevelName is the CItemLevel part of FUN_00494a40.
func (w *World) itemLevelName(index, level int) (string, bool) {
	for _, r := range w.dus().itemLevels {
		if r.Index == index && r.Level == level {
			return r.Name, true
		}
	}
	return "", false
}

// itemName is FUN_00494a40.
func (w *World) itemName(index, level int) string {
	if s, ok := w.itemLevelName(index, level); ok {
		return s
	}
	if info, ok := w.data.Items[index]; ok && info.Name != "" {
		return info.Name
	}
	return "Unknown"
}
