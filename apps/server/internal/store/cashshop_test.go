package store

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"
)

var (
	_ CreditStore    = (*Memory)(nil)
	_ CreditStore    = (*Postgres)(nil)
	_ JewelBankStore = (*Memory)(nil)
	_ JewelBankStore = (*Postgres)(nil)
)

func testCashShopStore(t *testing.T, s Store, setCredits func(id int64, n int64)) {
	ctx := context.Background()
	cs, bs := s.(CreditStore), s.(JewelBankStore)
	suffix := fmt.Sprintf("%07d", time.Now().UnixNano()/1000%10000000)
	login := "csh" + suffix
	acc, err := s.CreateAccount(ctx, login, "plain:x", "")
	if err != nil {
		t.Fatal(err)
	}

	// Credits: read, debit with and without balance, refund.
	setCredits(acc.ID, 100)
	if n, err := cs.Credits(ctx, login); err != nil || n != 100 {
		t.Fatalf("credits %d %v", n, err)
	}
	if ok, n, err := cs.DebitCredits(ctx, login, 30); err != nil || !ok || n != 70 {
		t.Fatalf("debit %v %d %v", ok, n, err)
	}
	if ok, n, err := cs.DebitCredits(ctx, login, 71); err != nil || ok || n != 70 {
		t.Fatalf("debit over balance %v %d %v", ok, n, err)
	}
	if ok, n, _ := cs.DebitCredits(ctx, login, 0); ok || n != 0 {
		t.Fatalf("debit 0 %v %d", ok, n)
	}
	if err := cs.RefundCredits(ctx, login, 5); err != nil {
		t.Fatal(err)
	}
	if n, _ := cs.Credits(ctx, login); n != 75 {
		t.Fatalf("after refund %d", n)
	}
	if _, err := cs.Credits(ctx, "nobody"+suffix); err != ErrNotFound {
		t.Fatalf("missing account %v", err)
	}
	if ok, n, err := cs.DebitCredits(ctx, "nobody"+suffix, 5); ok || n != 0 || err != nil {
		t.Fatalf("debit missing %v %d %v", ok, n, err)
	}

	// Jewel bank: missing row is zero, insert then update, negative deltas.
	if v, err := bs.JewelBank(ctx, login); err != nil || v != ([JewelBankTypes]int{}) {
		t.Fatalf("empty bank %v %v", v, err)
	}
	if err := bs.AddJewelBank(ctx, login, 3, 1); err != nil {
		t.Fatal(err)
	}
	if err := bs.AddJewelBank(ctx, login, 3, 4); err != nil {
		t.Fatal(err)
	}
	if err := bs.AddJewelBank(ctx, login, 9, 7); err != nil {
		t.Fatal(err)
	}
	if err := bs.AddJewelBank(ctx, login, 3, -2); err != nil {
		t.Fatal(err)
	}
	// The account key ignores case like MEMB_INFO.memb___id.
	v, err := bs.JewelBank(ctx, "CSH"+suffix)
	if err != nil || v != ([JewelBankTypes]int{3: 3, 9: 7}) {
		t.Fatalf("bank %v %v", v, err)
	}
	if err := bs.AddJewelBank(ctx, login, 10, 1); err == nil {
		t.Fatal("type 10 accepted")
	}
}

func TestMemoryCashShopStore(t *testing.T) {
	m, _ := NewMemory()
	testCashShopStore(t, m, m.SetCredits)
}

func TestPostgresCashShopStore(t *testing.T) {
	url := os.Getenv("MU_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("MU_TEST_DATABASE_URL not set")
	}
	p, err := OpenPostgres(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	testCashShopStore(t, p, func(id, n int64) {
		if _, err := p.pool.Exec(context.Background(), `UPDATE accounts SET credits=$2 WHERE id=$1`, id, n); err != nil {
			t.Fatal(err)
		}
	})
}
