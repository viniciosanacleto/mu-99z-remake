package game

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/store"
)

// CreationGold (FA 12, FUN_0049a690; Custom/CreationGold.txt read with
// GetPrivateProfileInt on every use): a right click on a Jewel of Creation
// (14,22) arms the bonus, a second click on the same slot between 100 ms
// and 1.5 s later consumes the jewel and adds GoldValue (at most
// 2,000,000,000) to MEMB_INFO.Gold (DS C1 20 F7 06 {index, account[11],
// name[11], gold u32}, DataServer FUN_00410c50).

const (
	creationJewel      = 14*512 + 22
	creationGoldWindow = 1500 * time.Millisecond
	creationGoldMin    = 100 * time.Millisecond
	msgCreationOff     = "O bonus da Jewel of Creation esta desativado."
	msgCreationArm     = "Clique novamente com o botao direito para usar a Creation e receber +%d Gold."
	msgCreationDone    = "Voce recebeu +%d Gold de bonus usando a Creation!"
)

type creationClick struct {
	at   time.Time // DAT_012a4e30[index]
	slot int       // DAT_012badc0[index]
}

func init() {
	faHandlers[0x12] = sized(5, func(w *World, o *Object, p []byte) { w.creationGold(o, int(p[4])) })
}

func (w *World) creationClick(idx int) *creationClick {
	x := w.wcx()
	if x.creation == nil {
		x.creation = map[int]*creationClick{}
	}
	c := x.creation[idx]
	if c == nil {
		c = &creationClick{}
		x.creation[idx] = c
	}
	return c
}

// creationGold is FUN_0049a690 (slot = [4]).
func (w *World) creationGold(o *Object, slot int) {
	if o.State != StatePlaying || !o.IsPlayer() || o.P.LogoutCountdown > 0 || o.P.Moving ||
		w.commandUnsettled(o) || ifBusy(o) || chaosTransactionOpen(o) || slot >= InventorySlots {
		return
	}
	jewel := func() bool { it := o.P.Inventory[slot]; return !it.IsEmpty() && it.Index == creationJewel }
	if !jewel() {
		return
	}
	gold := 0x32
	if w.dataDir != "" {
		gold = gamedata.CreationGoldValue(w.dataDir)
	}
	if gold < 1 {
		w.eventText(o, NoticeBlue, msgCreationOff)
		return
	}
	gold = min(gold, 2000000000)
	c := w.creationClick(o.Index)
	now := w.now()
	diff := now.Sub(c.at)
	if c.at.IsZero() || c.slot != slot || diff > creationGoldWindow {
		c.at, c.slot = now, slot
		w.eventText(o, NoticeBlue, fmt.Sprintf(msgCreationArm, gold))
		return
	}
	if diff < creationGoldMin {
		return
	}
	c.at, c.slot = time.Time{}, 0
	if !jewel() {
		return
	}
	w.deleteInventory(o, slot)
	w.send(o, packetItemDelete(slot, 1))
	if cs, _ := w.store.(store.CustomWorldStore); cs != nil {
		login := o.P.Account.Login
		w.async(func(ctx context.Context) func() {
			if err := cs.AddGold(ctx, login, gold); err != nil {
				slog.Warn("creation gold", "account", login, "err", err)
			}
			return nil
		})
	}
	w.log.Info("Jewel of Creation Bonus", "account", o.P.Account.Login, "name", charName(o), "gold", gold)
	w.eventText(o, NoticeBlue, fmt.Sprintf(msgCreationDone, gold))
}
