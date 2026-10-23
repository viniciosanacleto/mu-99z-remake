package game

import (
	"bytes"
	"encoding/binary"
	"testing"
	"time"
)

func TestLuckyWheelPackets(t *testing.T) {
	if p := packetWheelCost(0x0A0B0C0D); !bytes.Equal(p, []byte{0xC1, 8, 0xFB, 0xFC, 0x0D, 0x0C, 0x0B, 0x0A}) {
		t.Fatalf("FB FC % X", p)
	}
	if p := packetWheelResult(11); !bytes.Equal(p, []byte{0xC1, 8, 0xFB, 0x16, 11, 0, 0, 0}) {
		t.Fatalf("FB 16 % X", p)
	}
	b := newBareWorld(t)
	p := b.packetWheelList()
	// CustomLuckyWheel.txt has 12 rows.
	if len(p) != 6+32*12 || !bytes.Equal(p[:6], []byte{0xC2, 0x01, 0x86, 0xFB, 0xFB, 12}) {
		t.Fatalf("FB FB header % X", p[:6])
	}
	row := func(i int) []uint32 {
		v := make([]uint32, 8)
		for k := range v {
			v[k] = binary.LittleEndian.Uint32(p[6+32*i+4*k:])
		}
		return v
	}
	if r := row(1); r[0] != 1 || r[1] != 14 || r[2] != 11 || r[3] != 10 || r[4] != 0 || r[7] != 0 {
		t.Fatalf("row 1 %v", r)
	}
	if r := row(11); r[0] != 11 || r[2] != 11 || r[3] != 12 {
		t.Fatalf("row 11 %v", r)
	}
}

func TestLuckyWheelSpin(t *testing.T) {
	b := newBareWorld(t)
	o := b.player(PlayerStart, "Lucky", 0, 130, 130)
	fixRand(t, 13, 1) // positions 1 then 1
	b.wheelSpinRequest(o, []byte{0xC1, 4, 0xFB, 0x16})
	b.wheelSpinRequest(o, []byte{0xC1, 4, 0xFB, 0x16})
	b.advance(2999 * time.Millisecond)
	b.wheelTick()
	if len(b.drain(o)) != 0 {
		t.Fatal("reward before 3 s")
	}
	b.advance(time.Millisecond)
	b.wheelTick()
	b.runAsync()
	pk := b.drain(o)
	if p := find(pk, 0xFB, 0x16); !bytes.Equal(p, packetWheelResult(1)) {
		t.Fatalf("result % X", p)
	}
	// Position 1 is a Box of Luck +10 (14,11).
	if b.countItems(o, 14*512+11, 10) != 1 {
		t.Fatal("reward not given")
	}
	// The second spin waits for the first one: 3 s more.
	b.advance(2 * time.Second)
	b.wheelTick()
	if len(b.drain(o)) != 0 {
		t.Fatal("second spin early")
	}
	b.advance(time.Second)
	b.wheelTick()
	b.runAsync()
	if find(b.drain(o), 0xFB, 0x16) == nil || b.countItems(o, 14*512+11, 10) != 2 {
		t.Fatal("second spin")
	}
}

func TestLuckyWheelPrice(t *testing.T) {
	b := newBareWorld(t)
	withConfig(t, b, "Custom", map[string]string{"LuckyWheelCustValue": "5"})
	o := b.player(PlayerStart, "Lucky", 0, 130, 130)
	spin := func() {
		b.wheelSpinRequest(o, []byte{0xC1, 4, 0xFB, 0x16})
		b.runAsync()
	}
	// The price is debited from the account credits (U-LW-01 fixed): 12
	// credits pay two spins, the third is refused with 0x38D.
	shopAccount(t, b, o, 12)
	spin()
	spin()
	if len(cash(o).Spins) != 2 || credits(t, b, o.P.Account.Login) != 2 {
		t.Fatalf("spins %d credits %d", len(cash(o).Spins), credits(t, b, o.P.Account.Login))
	}
	b.drain(o)
	spin()
	hasText(t, b.drain(o), b.messageFor(o, 0x38D, 5))
	if len(cash(o).Spins) != 2 || credits(t, b, o.P.Account.Login) != 2 {
		t.Fatal("third spin charged or queued")
	}
	if b.messageFor(o, 0x38D, 5) == "" {
		t.Fatal("message 0x38D missing")
	}
}
