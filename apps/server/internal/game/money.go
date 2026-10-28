package game

import (
	"encoding/binary"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/packet"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/frame"
)

// packetMoney is C3 10 22 FE money(u32 BIG-endian) + 8 zero bytes
// (FUN_004da370; the money is passed in ECX and stored byte by byte from
// the most significant).
func packetMoney(money uint32) []byte {
	p := packet.New(frame.C3, 0x22, 0xFE, 0x10)
	binary.BigEndian.PutUint32(p[4:], money)
	return p
}

// sendMoney sends the player's current zen (obj+0xf8).
func (w *World) sendMoney(o *Object) {
	if o.P != nil && o.P.Char != nil {
		w.send(o, packetMoney(uint32(o.P.Char.Money)))
	}
}
