package game

import (
	"context"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/store"
)

// Golden Archer (CGoldenArcher, 0x461610..0x4621f0; Event/GoldenArcher.dat):
// the NPC 0xEC keeps a per-account count of Renas (EventGoldenArcher, a
// DataServer table) and trades 10/20/30/50/100 of them for a reward of the
// matching group.

const (
	gaNPC      = 0xEC
	ifArcher   = 9
	itemRena   = 0x1C15 // 14,21
	gaGroups   = 5
	gaFirework = 0
)

// gaNeeded is DAT_00547920: Renas needed per reward group.
var gaNeeded = [gaGroups]int{10, 20, 30, 50, 100}

type goldenArcherState struct {
	CountPending, RewardPending bool // obj+0x6a8 / obj+0x6ac
}

func init() {
	startHooks = append(startHooks, func(w *World) error {
		w.ev().archer = nil
		if w.configDir == "" {
			return nil
		}
		r, err := gamedata.LoadGoldenArcher(w.configDir)
		w.ev().archer = r
		return err
	})
	npcTalkHooks[gaNPC] = func(w *World, o, npc *Object) bool {
		// FUN_004c22f0: interface type 9, then the count request.
		items(o).If = ifState{Use: true, Type: ifArcher}
		w.gaRequestCount(o)
		return true
	}
	worldHandlers[0x95] = (*World).handleGiveRena
	worldHandlers[0x96] = (*World).handleArcherReward
	worldHandlers[0x97] = (*World).handleArcherClose
}

func gaState(o *Object) *goldenArcherState {
	if o.P.archer == nil {
		o.P.archer = &goldenArcherState{}
	}
	return o.P.archer
}

func (w *World) gaStore() store.GoldenArcherStore {
	s, _ := w.store.(store.GoldenArcherStore)
	return s
}

// packetRenaCount is C1 10 94 00 count(u32) + 8 zero bytes (FUN_00461b60).
func packetRenaCount(n int) []byte {
	p := make([]byte, 0x10)
	p[0], p[1], p[2] = 0xC1, 0x10, 0x94
	u32le(p[4:], uint32(n))
	return p
}

// gaRequestCount is FUN_00461870 (DataServer C1 12 94 00) and its reply
// FUN_00461b60. before runs first in the same DataServer turn (the
// original DataServer answers one connection in order).
func (w *World) gaRequestCount(o *Object, before ...func(ctx context.Context, gs store.GoldenArcherStore) error) {
	st, gs := gaState(o), w.gaStore()
	if gs == nil || !questPlayerOK(o) {
		return
	}
	pending := st.CountPending
	st.CountPending = true
	account, c := o.P.Account.Login, o.conn
	w.async(func(ctx context.Context) func() {
		for _, f := range before {
			if err := f(ctx, gs); err != nil {
				w.log.Error("golden archer", "account", account, "err", err)
			}
		}
		if pending {
			return nil // a count request is already on its way
		}
		n, err := gs.RenaCount(ctx, account)
		return func() {
			o := w.objectOf(c)
			if o == nil || o.P == nil {
				return
			}
			gaState(o).CountPending = false
			if err != nil {
				w.log.Error("golden archer count", "account", account, "err", err)
				return
			}
			w.send(o, packetRenaCount(n))
		}
	})
}

// handleGiveRena is 95 (FUN_00461920): one Rena from the inventory adds 1
// to the count (DataServer FUN_00413c80), then the count is asked again.
func (w *World) handleGiveRena(o *Object, p []byte) {
	if !questPlayerOK(o) || w.gaStore() == nil || w.countItems(o, itemRena, 0) == 0 {
		return
	}
	w.deleteItems(o, itemRena, 0, 1)
	account := o.P.Account.Login
	w.gaRequestCount(o, func(ctx context.Context, gs store.GoldenArcherStore) error {
		return gs.AddRena(ctx, account, 1)
	})
}

// handleArcherReward is 96 (FUN_00461a40): [3] reward group. The
// DataServer (FUN_004139a0) takes the Renas when there are enough and the
// reply (FUN_00461c90) gives the reward.
func (w *World) handleArcherReward(o *Object, p []byte) {
	gs := w.gaStore()
	if len(p) < 4 || !questPlayerOK(o) || gs == nil {
		return
	}
	st := gaState(o)
	if st.RewardPending {
		return
	}
	group := int(p[3])
	if group >= gaGroups {
		return
	}
	st.RewardPending = true
	account, c := o.P.Account.Login, o.conn
	w.async(func(ctx context.Context) func() {
		ok, err := gs.TakeRena(ctx, account, gaNeeded[group])
		return func() {
			o := w.objectOf(c)
			if o == nil || o.P == nil {
				return
			}
			gaState(o).RewardPending = false
			if err != nil {
				w.log.Error("golden archer reward", "account", account, "err", err)
				return
			}
			if ok {
				w.gaGiveReward(o, group)
			}
			w.gaRequestCount(o)
		}
	})
}

// gaGiveReward is the success branch of FUN_00461c90: a weighted draw
// (weight = DropRate) among the group's rows, ItemOptionRate options, the
// item into the inventory when it fits (else on the player's cell) and the
// C1 07 F3 40 00 X Y firework to the player and its viewers.
func (w *World) gaGiveReward(o *Object, group int) {
	var cand []gamedata.GoldenArcherReward
	total := 0
	for _, r := range w.ev().archer {
		if r.Group == group && r.Rate != 0 {
			cand = append(cand, r)
			total += r.Rate
		}
	}
	if total <= 0 {
		return
	}
	x := monsterRand() % total
	for _, r := range cand {
		if x >= r.Rate {
			x -= r.Rate
			continue
		}
		opt := w.optionsFrom(r.Option, 0)
		s := dropSpec{Index: r.Index, Level: opt[0], Skill: opt[1], Luck: opt[2], Option: opt[3], Duration: r.Duration}
		s.Exc = w.makeNewOption(r.Index, 1, opt[4])
		s.Set = w.makeSetOption(r.Index, opt[5])
		s.ToInventory = w.findBagSlot(&o.P.Inventory, r.Index) >= 0 // FUN_004952a0
		w.createItem(o, o.Map, o.X, o.Y, s)
		fw := []byte{0xC1, 0x07, 0xF3, 0x40, gaFirework, byte(o.X), byte(o.Y)}
		w.send(o, fw)
		w.broadcast(o, fw)
		return
	}
}

// handleArcherClose is 97 (FUN_00461b00): closes interface type 9.
func (w *World) handleArcherClose(o *Object, p []byte) {
	if !questPlayerOK(o) {
		return
	}
	if st := items(o); st.If.Use && st.If.Type == ifArcher {
		st.If = ifState{}
	}
}
