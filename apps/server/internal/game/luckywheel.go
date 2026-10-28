package game

import (
	"context"
	"sort"
	"time"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/frame"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/store"
)

// CLuckyWheel (0x44e080..0x44eac0): the custom roulette. When the
// character enters the game (FUN_00473a10) the client gets the wheel
// (C2 FB FB, FUN_0044e860) and the price (C1 08 FB FC, FUN_0044ea00). A spin
// (FB 16, FUN_0044e570) draws rand() % 12, waits 3 s, gives the reward
// of that wheel position (Custom/CustomLuckyWheel.txt) and answers C1 08
// FB 16 position.
//
// The original compares the price with Coin1 (obj+0xd20), which this build
// never loads or deducts, so a positive LuckyWheelCustValue refused every
// spin and 0 made it free (U-LW-01). Fixed here: a positive price is
// debited from the account credits (MEMB_INFO.Credits, the XShop
// currency) before the draw; without enough credits the spin is refused
// with message 0x38D (missing from the shipped Message.txt, added to the
// copy in data/).
//
// Deliberate deviation: the original calls Sleep(3000) on the server
// thread between the draw and the reward, freezing the whole GameServer;
// here the spin waits in a per-player queue checked every 100 ms
// (U-LW-02).

func registerLuckywheel(h *hooks) {
	h.fbHandlers[0x16] = (*World).wheelSpinRequest
	h.enterWorld = append(h.enterWorld, func(w *World, o *Object) {
		w.send(o, w.packetWheelList())
		w.send(o, packetWheelCost(uint32(w.wheelCost())))
	})
	h.tick100 = append(h.tick100, (*World).wheelTick)
}

// wheelPositions is the rand() % 12 of FUN_0044e570.
const (
	wheelPositions = 12
	wheelDelay     = 3000 * time.Millisecond
)

// wheelSpin is a spin waiting for its reward.
type wheelSpin struct {
	Due time.Time
	Key int
}

// wheelCost is LuckyWheelCustValue (GameServerInfo - Custom.dat,
// CServerInfo+0x1c7c).
func (w *World) wheelCost() int { return w.cfg.Int("Custom", "LuckyWheelCustValue", 0) }

// packetWheelCost is C1 08 FB FC price(u32).
func packetWheelCost(cost uint32) []byte {
	p := newPacket(frame.C1, 0xFB, 0xFC, 8)
	u32le(p[4:], cost)
	return p
}

// packetWheelResult is C1 08 FB 16 position(u32).
func packetWheelResult(key int) []byte {
	p := newPacket(frame.C1, 0xFB, 0x16, 8)
	u32le(p[4:], uint32(key))
	return p
}

// packetWheelList is C2 size FB FB count + 32 bytes per wheel row in key
// order: IndexWheel, Type, Index, Level, Luck, Skill, Option, Excellent
// (u32 each; the record keeps Luck before Skill, FUN_0044e3a0).
func (w *World) packetWheelList() []byte {
	var rows []gamedata.WheelReward
	if w.cash != nil {
		for _, r := range w.cash.wheel {
			rows = append(rows, r)
		}
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Index < rows[j].Index })
	p := newPacket(frame.C2, 0xFB, 0xFB, 6+32*len(rows))
	p[5] = byte(len(rows))
	for i, r := range rows {
		b := p[6+32*i:]
		for k, v := range []int{r.Index, r.Type, r.Item, r.Level, r.Luck, r.Skill, r.Option, r.Excellent} {
			u32le(b[4*k:], uint32(int32(v)))
		}
	}
	return p
}

// wheelSpinRequest is the first half of FUN_0044e570: the price test and
// the draw.
func (w *World) wheelSpinRequest(o *Object, p []byte) {
	cost := w.wheelCost()
	if cost <= 0 {
		w.wheelQueue(o)
		return
	}
	cs, _ := w.store.(store.CreditStore)
	if cs == nil || o.P == nil {
		w.customMsg(o, 0x38D, cost)
		return
	}
	login := o.P.Account.Login
	w.async(func(ctx context.Context) func() {
		ok, _, err := cs.DebitCredits(ctx, login, int64(cost))
		return func() {
			if o.State != StatePlaying || o.P == nil || o.P.Account.Login != login {
				if ok && err == nil {
					w.async(func(ctx context.Context) func() { cs.RefundCredits(ctx, login, int64(cost)); return nil })
				}
				return
			}
			if err != nil || !ok {
				w.customMsg(o, 0x38D, cost)
				return
			}
			w.wheelQueue(o)
		}
	})
}

// wheelQueue draws the position and queues the reward 3 s later.
func (w *World) wheelQueue(o *Object) {
	s, now := cash(o), w.now()
	due := now.Add(wheelDelay)
	if n := len(s.Spins); n > 0 && s.Spins[n-1].Due.After(now) {
		due = s.Spins[n-1].Due.Add(wheelDelay)
	}
	s.Spins = append(s.Spins, wheelSpin{Due: due, Key: monsterRand() % wheelPositions})
}

// wheelTick finishes the spins whose 3 s are over.
func (w *World) wheelTick() {
	now := w.now()
	for i := PlayerStart; i < MaxObjects; i++ {
		o := w.objects[i]
		if !o.IsPlayer() || o.P == nil || o.P.cash == nil || len(o.P.cash.Spins) == 0 {
			continue
		}
		s := o.P.cash
		for len(s.Spins) > 0 && !s.Spins[0].Due.After(now) {
			key := s.Spins[0].Key
			s.Spins = s.Spins[1:]
			if o.State == StatePlaying && o.P.Char != nil {
				w.wheelReward(o, key)
			}
		}
	}
}

// wheelReward is the second half of FUN_0044e570: the reward of the
// position (a missing position reads as a zero row, item 0,0, through the
// std::map operator[] of FUN_0044eac0) goes to the inventory (CreateItem
// map 0xEB), then FB 16.
func (w *World) wheelReward(o *Object, key int) {
	var r gamedata.WheelReward
	if w.cash != nil {
		r = w.cash.wheel[key]
	}
	w.createItem(o, 0xEB, 0, 0, dropSpec{Index: r.Type*512 + r.Item, Level: r.Level, Skill: r.Skill,
		Luck: r.Luck, Option: r.Option, Exc: byte(r.Excellent), ToInventory: true})
	w.send(o, packetWheelResult(key))
}
