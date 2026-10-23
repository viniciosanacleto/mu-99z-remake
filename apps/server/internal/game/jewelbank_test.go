package game

import (
	"bytes"
	"context"
	"encoding/binary"
	"testing"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/store"
)

func bankPacket(sub byte, vals ...uint32) []byte {
	p := []byte{0xC1, byte(4 + 4*len(vals)), 0xF3, sub}
	for _, v := range vals {
		p = binary.LittleEndian.AppendUint32(p, v)
	}
	return p
}

func storedBank(t *testing.T, b *bareWorld, login string) [store.JewelBankTypes]int {
	t.Helper()
	b.runAsync()
	v, err := b.store.(store.JewelBankStore).JewelBank(context.Background(), login)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestJewelBankPacketLayout(t *testing.T) {
	p := packetJewelBank([store.JewelBankTypes]int{1, 2, 3, 4, 5, 6, 7, 8, 9, 0x01020304})
	if len(p) != 0x2C || !bytes.Equal(p[:8], []byte{0xC1, 0x2C, 0xF3, 0xF5, 1, 0, 0, 0}) ||
		!bytes.Equal(p[40:], []byte{4, 3, 2, 1}) {
		t.Fatalf("% X", p)
	}
}

func TestJewelBankDeposit(t *testing.T) {
	b := newBareWorld(t)
	o := b.player(PlayerStart, "Banker", 0, 130, 130)
	give(t, b, o, 14*512+13, 0, 2) // two Bless
	give(t, b, o, 14*512+14, 1, 1) // a Soul +1 is not taken
	b.jewelBankDeposit(o, bankPacket(0xF5, 3))
	if p := find(b.drain(o), 0xF3, 0xF5); !bytes.Equal(p, packetJewelBank([store.JewelBankTypes]int{3: 1})) {
		t.Fatalf("bank % X", p)
	}
	if b.countItems(o, 14*512+13, 0) != 1 {
		t.Fatal("bless not taken")
	}
	b.jewelBankDeposit(o, bankPacket(0xF5, 2))
	if len(b.drain(o)) != 0 || cash(o).Bank[2] != 0 {
		t.Fatal("soul +1 deposited")
	}
	// Type 10 and above select no jewel; an open interface refuses.
	b.jewelBankDeposit(o, bankPacket(0xF5, 10))
	items(o).If.Use = true
	b.jewelBankDeposit(o, bankPacket(0xF5, 3))
	items(o).If.Use = false
	if len(b.drain(o)) != 0 || cash(o).Bank[3] != 1 {
		t.Fatal("refused deposits changed the bank")
	}
	if v := storedBank(t, b, o.P.Account.Login); v != ([store.JewelBankTypes]int{3: 1}) {
		t.Fatalf("stored %v", v)
	}
}

func TestJewelBankWithdraw(t *testing.T) {
	b := newBareWorld(t)
	o := b.player(PlayerStart, "Banker", 0, 130, 130)
	login := o.P.Account.Login
	s := cash(o)
	s.Bank[3], s.Bank[0], s.Bank[5] = 3, 25, 4
	// 99 = as many as free cells, at most the counter: all 3 Bless.
	b.jewelBankWithdraw(o, bankPacket(0xF6, 3, 99))
	b.runAsync()
	if b.countItems(o, 14*512+13, 0) != 3 || s.Bank[3] != 0 {
		t.Fatalf("bless %d bank %d", b.countItems(o, 14*512+13, 0), s.Bank[3])
	}
	// More than the counter is refused.
	b.drain(o)
	b.jewelBankWithdraw(o, bankPacket(0xF6, 5, 5))
	if len(b.drain(o)) != 0 || s.Bank[5] != 4 {
		t.Fatal("over-withdraw")
	}
	// 2 Guardian of 4.
	b.jewelBankWithdraw(o, bankPacket(0xF6, 5, 2))
	b.runAsync()
	if b.countItems(o, 14*512+31, 0) != 2 || s.Bank[5] != 2 {
		t.Fatal("guardian")
	}
	// Chaos x10 comes out as one Chaos pack (12,141) +0 and Life x20 as a
	// Life pack (12,136) +1; any other count gives that many jewels.
	s.Bank[1] = 20
	b.jewelBankWithdraw(o, bankPacket(0xF6, 0, 10))
	b.jewelBankWithdraw(o, bankPacket(0xF6, 1, 20))
	b.jewelBankWithdraw(o, bankPacket(0xF6, 0, 5))
	b.runAsync()
	if b.countItems(o, 12*512+141, 0) != 1 || b.countItems(o, 12*512+136, 1) != 1 ||
		b.countItems(o, 12*512+15, 0) != 5 || s.Bank[0] != 10 || s.Bank[1] != 0 {
		t.Fatalf("chaos pack %d life pack %d chaos %d bank %d/%d", b.countItems(o, 12*512+141, 0),
			b.countItems(o, 12*512+136, 1), b.countItems(o, 12*512+15, 0), s.Bank[0], s.Bank[1])
	}
	if b.countItems(o, 12*512+30, -1) != 0 || b.countItems(o, 12*512+31, -1) != 0 {
		t.Fatal("bless or soul pack handed out")
	}
	// Count 0 is refused: no free jewel.
	b.jewelBankWithdraw(o, bankPacket(0xF6, 0, 0))
	b.runAsync()
	if b.countItems(o, 12*512+15, 0) != 5 || s.Bank[0] != 10 {
		t.Fatal("count 0 of chaos gave a jewel")
	}
	if p := find(b.drain(o), 0xF3, 0xF5); p == nil {
		t.Fatal("no F3 F5")
	}
	if v := storedBank(t, b, login); v[3] != -3 || v[5] != -2 || v[0] != -15 || v[1] != -20 {
		t.Fatalf("stored deltas %v", v)
	}
	// More jewels than free cells are refused before anything is taken.
	free := b.freeBagCells(o)
	s.Bank[5] = free + 1
	b.jewelBankWithdraw(o, bankPacket(0xF6, 5, uint32(free+1)))
	if s.Bank[5] != free+1 {
		t.Fatal("withdraw beyond the free cells")
	}
	s.Bank[5] = 2
	// No free cell: nothing.
	for i := BagStart; i < BagEnd; i++ {
		if o.P.Inventory[i].IsEmpty() {
			o.P.Inventory[i] = newItem(14*512+13, 0)
		}
	}
	b.runAsync()
	b.jewelBankWithdraw(o, bankPacket(0xF6, 5, 1))
	if s.Bank[5] != 2 {
		t.Fatal("withdraw without room")
	}
}

func TestJewelBankLoadOnEnter(t *testing.T) {
	b := newBareWorld(t)
	o := b.player(PlayerStart, "Banker", 0, 130, 130)
	bs := b.store.(store.JewelBankStore)
	if err := bs.AddJewelBank(context.Background(), o.P.Account.Login, 6, 9); err != nil {
		t.Fatal(err)
	}
	b.jewelBankLoad(o)
	b.runAsync()
	want := [store.JewelBankTypes]int{6: 9}
	if p := find(b.drain(o), 0xF3, 0xF5); !bytes.Equal(p, packetJewelBank(want)) || cash(o).Bank != want {
		t.Fatalf("load % X", p)
	}
}
