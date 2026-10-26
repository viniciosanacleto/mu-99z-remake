package game

import (
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/item"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"
)

// NewItemDurability is the durability CreateItem gives a new item
// (drop.go createItem with Dur 0): FUN_00494500 on its level, excellent and
// set options. Used by tools outside the World (cmd/mueditor).
func NewItemDurability(info gamedata.ItemInfo, it item.Item) int {
	return maxDurability(info, it.Index, it.Level, it.Excellent != 0, it.SetOption != 0)
}
