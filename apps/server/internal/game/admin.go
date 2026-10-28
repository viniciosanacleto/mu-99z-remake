package game

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"time"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/item"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/store"
)

// Live editing for the web editor running in the muserver process. A
// playing character lives in its World (Object.P) and is written over the
// store on every save, so the editor changes it on the world goroutine and
// the client gets the packets the server already sends for those changes.
// The original had no such path (its editors needed the account offline).

// Errors of the live editing API.
var (
	// ErrCharLeaving: the character left its session between the lookup
	// and the edit; its logout save may still be in flight.
	ErrCharLeaving = errors.New("game: character is leaving the game")
	// ErrVaultBusy: the account vault is being loaded or another vault
	// number is open.
	ErrVaultBusy = errors.New("game: vault is busy")
)

// liveEditTimeout bounds the wait for the world goroutine.
const liveEditTimeout = 5 * time.Second

// LiveEdit is the in-memory state of a playing character given to an
// editor callback, in the store encoding. Changes the callback makes are
// applied back to the character and sent to its client.
type LiveEdit struct {
	// Char is a copy of the character with the position of the object.
	// Only Level, LevelUpPoints, Experience, Strength, Dexterity, Vitality,
	// Energy, Leadership, Money and Resets are applied back.
	Char store.Character
	// Inventory is store.InventorySize bytes (item.DBSize per slot).
	Inventory []byte
	// VaultLoaded is set when the character has its vault window open;
	// Vault (store.WarehouseSize bytes), VaultNumber and VaultMoney are
	// then the open vault.
	VaultLoaded bool
	VaultNumber int
	Vault       []byte
	VaultMoney  int64
}

// AccountCharacter returns the playing character of an account login.
func (h *Hub) AccountCharacter(login string) (string, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, c := range h.online {
		if strings.EqualFold(c.Account, login) {
			return c.Name, true
		}
	}
	return "", false
}

// onChar runs f on the world goroutine of an online character and waits
// for its result. online is false when the name is not playing; f gets a
// nil object when the session changed after the lookup.
func (h *Hub) onChar(ctx context.Context, name string, f func(w *World, o *Object) error) (online bool, err error) {
	c, ok := h.Lookup(name)
	if !ok {
		return false, nil
	}
	h.mu.Lock()
	w := h.worlds[c.ServerCode]
	h.mu.Unlock()
	if w == nil {
		return false, nil
	}
	ctx, cancel := context.WithTimeout(ctx, liveEditTimeout)
	defer cancel()
	done := make(chan error, 1)
	run := func() {
		o := w.objects[c.Index]
		if o.State != StatePlaying || o.P == nil || o.P.Char == nil || !strings.EqualFold(o.P.Char.Name, c.Name) {
			o = nil
		}
		done <- f(w, o)
	}
	select {
	case w.events <- run:
	case <-w.ctxDone():
		return true, ErrCharLeaving
	case <-ctx.Done():
		return true, ctx.Err()
	}
	select {
	case err := <-done:
		return true, err
	case <-ctx.Done():
		return true, ctx.Err()
	}
}

// EditCharacter runs f on the in-memory state of a playing character and
// applies what it changed. It returns online=false, without calling f,
// when the character is not playing on any instance of the process.
func (h *Hub) EditCharacter(ctx context.Context, name string, f func(*LiveEdit) error) (bool, error) {
	return h.onChar(ctx, name, func(w *World, o *Object) error {
		if o == nil {
			return ErrCharLeaving
		}
		return w.liveEdit(o, f)
	})
}

// LockVault prepares a vault edit of the account of a playing character.
// When the vault window is open it returns loaded=true and the edit goes
// through EditCharacter. Otherwise the vault is marked as loading until
// release runs, so the player cannot open it while the editor writes it
// to the store.
func (h *Hub) LockVault(ctx context.Context, name string) (loaded bool, release func(), err error) {
	release = func() {}
	online, err := h.onChar(ctx, name, func(w *World, o *Object) error {
		if o == nil {
			return ErrCharLeaving
		}
		st := items(o)
		switch {
		case st.warehouseLoading:
			return ErrVaultBusy
		case st.WarehouseLoaded:
			loaded = true
			return nil
		}
		st.warehouseLoading = true
		session := o.conn
		release = func() {
			h.onChar(context.Background(), name, func(w *World, o *Object) error {
				if o != nil && o.conn == session {
					items(o).warehouseLoading = false
				}
				return nil
			})
		}
		return nil
	})
	if err == nil && !online {
		err = ErrCharLeaving
	}
	return loaded, release, err
}

func encodeItems(its []item.Item, size int) []byte {
	b := make([]byte, size)
	for i, it := range its {
		it.EncodeDB(b[i*store.ItemDBSize:])
	}
	return b
}

// liveEdit builds the LiveEdit of o, runs f and applies the changes.
func (w *World) liveEdit(o *Object, f func(*LiveEdit) error) error {
	p := o.P
	e := &LiveEdit{Char: *p.Char, Inventory: encodeItems(p.Inventory[:], store.InventorySize)}
	e.Char.Inventory = nil
	e.Char.Map, e.Char.X, e.Char.Y = o.Map, o.X, o.Y
	st := items(o)
	if st.WarehouseLoaded {
		e.VaultLoaded, e.VaultNumber, e.VaultMoney = true, st.WarehouseNumber, st.WarehouseMoney
		e.Vault = encodeItems(st.Warehouse[:], store.WarehouseSize)
	}
	before := *e
	before.Inventory, before.Vault = bytes.Clone(e.Inventory), bytes.Clone(e.Vault)
	if err := f(e); err != nil {
		return err
	}
	changed := w.liveApplyChar(o, &before.Char, &e.Char)
	changed = w.liveApplyInventory(o, before.Inventory, e.Inventory) || changed
	if e.VaultLoaded && (!bytes.Equal(before.Vault, e.Vault) || before.VaultMoney != e.VaultMoney) {
		w.liveApplyVault(o, e.Vault, e.VaultMoney)
		changed = true
	}
	if changed {
		w.saveCharacter(o, nil)
	}
	return nil
}

// liveApplyChar copies the editable fields and refreshes the client: the
// C1 A4 F3 E0 refresh of the GM /edit (commandEdit) after CalcCharacter,
// and the zen.
func (w *World) liveApplyChar(o *Object, old, nw *store.Character) bool {
	c := o.P.Char
	stats := old.Level != nw.Level || old.LevelUpPoints != nw.LevelUpPoints || old.Experience != nw.Experience ||
		old.Strength != nw.Strength || old.Dexterity != nw.Dexterity || old.Vitality != nw.Vitality ||
		old.Energy != nw.Energy || old.Leadership != nw.Leadership || old.Resets != nw.Resets
	if stats {
		c.Level, c.LevelUpPoints, c.Experience, c.Resets = nw.Level, nw.LevelUpPoints, nw.Experience, nw.Resets
		c.Strength, c.Dexterity, c.Vitality, c.Energy, c.Leadership = nw.Strength, nw.Dexterity, nw.Vitality, nw.Energy, nw.Leadership
		w.calcCharacter(o)
		w.send(o, w.packetCharRefresh(o))
	}
	money := old.Money != nw.Money
	if money {
		c.Money = nw.Money
		w.sendMoney(o)
	}
	return stats || money
}

// liveApplyInventory replaces the changed slots: C1 05 28 for a removed
// item, C1 0C F3 14 for a new one (the 0xEB insertion of
// giveInventoryItem), and the equipment broadcast for slots 0..11.
func (w *World) liveApplyInventory(o *Object, old, nw []byte) bool {
	changed := false
	for s := range o.P.Inventory {
		a, b := old[s*store.ItemDBSize:(s+1)*store.ItemDBSize], nw[s*store.ItemDBSize:(s+1)*store.ItemDBSize]
		if bytes.Equal(a, b) {
			continue
		}
		changed = true
		it, ok := item.DecodeDB(b)
		if !ok {
			it = item.Empty()
		}
		o.P.Inventory[s] = it
		if ok {
			w.send(o, packetItemModify(s, it))
		} else {
			w.send(o, packetItemDelete(s, 1))
		}
		w.equipmentChanged(o, s)
	}
	return changed
}

// liveApplyVault replaces the open vault, sends it again as on opening
// (warehouseLoaded) and saves it.
func (w *World) liveApplyVault(o *Object, blob []byte, money int64) {
	st := items(o)
	for i := range st.Warehouse {
		st.Warehouse[i] = item.Empty()
		if it, ok := item.DecodeDB(blob[i*store.ItemDBSize:]); ok {
			st.Warehouse[i] = it
		}
	}
	st.WarehouseMoney = money
	w.send(o, packetItemList(st.Warehouse[:]))
	w.send(o, packetWarehouseMoney(1, st.WarehouseMoney, o.P.Char.Money))
	wh := w.warehouseRecord(o)
	w.async(func(ctx context.Context) func() {
		if err := w.store.SaveWarehouse(ctx, wh); err != nil {
			w.log.Error("save warehouse", "err", err)
		}
		return nil
	})
}
