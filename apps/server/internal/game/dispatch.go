package game

import (
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/frame"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/gscodec"
)

// handlePacket is ProtocolCore (GameServer 0x4d6690). Packets arrive here
// already in plain C1/C2 form.
func (w *World) handlePacket(c *conn, pkt gscodec.Packet) {
	o := w.objectOf(c)
	if o == nil {
		return
	}
	p := pkt.Data
	if len(p) < frame.HeaderLen(p[0])+1 {
		return
	}
	o.P.LastPacket = w.now()
	head := frame.Head(p)
	sub, hasSub := frame.Sub(p)
	if o.Type == TypePlayer && head != 0xFA && !w.hackPacketCheck(o, pkt) {
		return
	}

	switch head {
	case 0x0E:
		w.handleLiveClient(o, p)
	case 0xF1:
		if !hasSub {
			return
		}
		switch sub {
		case 0x01:
			w.handleLogin(o, p)
		case 0x02:
			w.handleLogout(o, p)
		}
	case 0xB1:
		if hasSub && sub == 0x01 {
			w.handleMoveAuth(o, p)
			return
		}
		if h := worldHandlers[head]; h != nil && o.State == StatePlaying {
			h(w, o, p)
		}
	case 0xF3:
		if !hasSub {
			return
		}
		switch sub {
		case 0x00:
			w.handleCharList(o)
		case 0x01:
			w.handleCharCreate(o, p)
		case 0x02:
			w.handleCharDelete(o, p)
		case 0x03:
			w.handleCharSelect(o, p)
		case 0x09:
			w.handleVerify(o, p)
		default:
			if h := f3Handlers[sub]; h != nil && o.State == StatePlaying {
				h(w, o, p)
			}
		}
	default:
		if h := worldHandlers[head]; h != nil && o.State == StatePlaying {
			if packetBlockHook != nil && packetBlockHook(w, o, head) {
				return
			}
			h(w, o, p)
		}
	}
}

// packetBlockHook refuses an in-game head before its handler (the
// WizardTeam custom event checks of 36, 40 and AA in FUN_004d6690;
// customevent.go).
var packetBlockHook func(w *World, o *Object, head byte) bool

// worldHandlers maps in-game heads to handlers; gameplay files register
// themselves in init functions.
var worldHandlers = map[byte]func(*World, *Object, []byte){}

// faHandlers and fbHandlers map the sub-codes of the WizardTeam heads FA
// and FB (FUN_004d6690: FA only for C1 packets longer than 3 bytes, the
// sub-code at [3]); xshop.go registers the head handlers.
var (
	faHandlers = map[byte]func(*World, *Object, []byte){}
	fbHandlers = map[byte]func(*World, *Object, []byte){}
)

// f3Handlers maps F3 sub-codes (other than 00-03 and 09) to in-game
// handlers; modules register them in init functions.
var f3Handlers = map[byte]func(*World, *Object, []byte){}
