package game

import (
	"math/rand/v2"
	"time"
)

// Timers of the original (FUN_0047ddb0): ids 0, 1, 4 and 10 every 100 ms
// (monster AI and movement), 5, 6, 7 and 9 every second (object processing,
// viewport), 8 every 60 seconds; plus the 1-second client thread
// FUN_00510810 (logout countdown, save, timeouts).

const (
	saveInterval      = 600 * time.Second // FUN_00510810: 600000 ms
	verifyTimeout     = 5 * time.Second   // "Not verified"
	clientIdleTimeout = 60 * time.Second  // "Timeout" (59999 < elapsed)
)

func (w *World) rand(n int) int {
	if n <= 0 {
		return 0
	}
	return rand.IntN(n)
}

func (w *World) tick100() {
	for _, f := range tick100Hooks {
		f(w)
	}
}

// tick100Hooks are registered by the movement and monster modules.
var tick100Hooks []func(*World)

func (w *World) tick1000() {
	now := w.now()
	for i := PlayerStart; i < MaxObjects; i++ {
		o := w.objects[i]
		if o.State == StateEmpty || o.P == nil {
			continue
		}
		w.playerSecond(o, now)
	}
	for _, f := range tick1000Hooks {
		f(w)
	}
	w.viewportProc()
	for _, f := range viewportEndHooks {
		f(w)
	}
}

var tick1000Hooks []func(*World)

// viewportEndHooks run after the 1-second viewport cycle (the tail of
// FUN_0050a020: FUN_004c3b10 state transitions).
var viewportEndHooks []func(*World)

// playerSecond is the per-client part of FUN_00510810.
func (w *World) playerSecond(o *Object, now time.Time) {
	p := o.P
	if o.State >= StateLogged && p.LogoutCountdown > 0 {
		if p.LogoutCountdown == 1 {
			switch p.LogoutType {
			case 1:
				if w.returnToSelect(o) {
					w.send(o, packetLogoutResult(1))
				}
			case 0:
				w.send(o, packetLogoutResult(0))
			default:
				w.send(o, packetLogoutResult(2))
			}
		} else {
			w.systemMessage(o, w.messageFor(o, 0x121, p.LogoutCountdown-1))
		}
		p.LogoutCountdown--
	}
	if o.State == StatePlaying && now.Sub(p.LastSave) > saveInterval {
		w.saveCharacter(o, nil)
		p.LastSave = now
	}
	if !p.Verified && now.Sub(p.ConnectedAt) > verifyTimeout {
		w.closeClient(o, "Not verified")
		return
	}
	if now.Sub(p.LastPacket) > clientIdleTimeout {
		w.closeClient(o, "Timeout")
	}
}

// returnToSelect is FUN_004c4b40: save, leave the map, back to state 2.
func (w *World) returnToSelect(o *Object) bool {
	if o.State != StatePlaying {
		return false
	}
	w.leaveWorld(o)
	w.saveCharacter(o, nil)
	o.State = StateLogged
	o.P.CharListLoaded = true
	o.P.Char = nil
	return true
}
