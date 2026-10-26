package game

import (
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/frame"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/gscodec"
)

func TestHackRulesLookup(t *testing.T) {
	r := &hackRules{sub: map[[2]int]hackRule{}}
	r.add(hackRule{head: 0xF3, value: -1, encrypt: -1})
	r.add(hackRule{head: 0x4D, value: 0, encrypt: 1})
	r.add(hackRule{head: 0x4D, value: 15, encrypt: 0})
	if _, ok := r.find(0xF3, 0x55); !ok {
		t.Fatal("'*' rule must match every sub")
	}
	if rule, ok := r.find(0x4D, 15); !ok || rule.encrypt != 0 {
		t.Fatalf("sub rule = %+v %v", rule, ok)
	}
	if _, ok := r.find(0x4D, 3); ok {
		t.Fatal("missing sub rule must not match")
	}
	if _, ok := r.find(0x99, 0); ok {
		t.Fatal("missing head must not match")
	}
}

func TestLoadOriginalHackRules(t *testing.T) {
	r, err := loadHackRules(repoConfig())
	if err != nil {
		t.Fatal(err)
	}
	if rule, ok := r.find(0xF1, 1); !ok || rule.encrypt != 1 {
		t.Fatalf("F1 rule = %+v", rule)
	}
	if rule, ok := r.find(0x18, 0); !ok || rule.maxDelay != 1000 || rule.minCount != 40 || rule.maxCount != 50 {
		t.Fatalf("18 rule = %+v", rule)
	}
}

func newHackWorld(t *testing.T) (*World, *Object) {
	w := &World{hackRules: &hackRules{sub: map[[2]int]hackRule{}}, now: time.Now, log: discardLogger()}
	w.hackRules.add(hackRule{head: 0xF1, value: -1, encrypt: 1})
	w.hackRules.add(hackRule{head: 0x18, value: -1, encrypt: 0, maxDelay: 1000, minCount: 2, maxCount: 3})
	o := newObject(PlayerStart)
	o.Type = TypePlayer
	o.P = &Player{recvSerial: 0xFF}
	return w, o
}

func TestHackPacketCheckSerialAndEncryption(t *testing.T) {
	w, o := newHackWorld(t)
	login := gscodec.Packet{Data: frame.Build(frame.C1, 0xF1, 0x01), Encrypted: true, Serial: 0}
	if !w.hackPacketCheck(o, login) {
		t.Fatal("first serial 0 must pass")
	}
	login.Serial = 5
	if w.hackPacketCheck(o, login) {
		t.Fatal("wrong serial must fail")
	}
	w2, o2 := newHackWorld(t)
	plain := gscodec.Packet{Data: frame.Build(frame.C1, 0xF1, 0x01)}
	if w2.hackPacketCheck(o2, plain) {
		t.Fatal("F1 must be encrypted")
	}
	if w2.hackPacketCheck(o2, gscodec.Packet{Data: frame.Build(frame.C1, 0x77, 0x00)}) {
		t.Fatal("unknown head must fail")
	}
}

func TestHackPacketCheckCount(t *testing.T) {
	w, o := newHackWorld(t)
	now := time.Unix(1000, 0)
	w.now = func() time.Time { return now }
	act := gscodec.Packet{Data: frame.Build(frame.C1, 0x18, 0, 0, 0, 0)}
	for i := 1; i <= 2; i++ {
		if !w.hackPacketCheck(o, act) {
			t.Fatalf("packet %d within MinCount dropped", i)
		}
	}
	if w.hackPacketCheck(o, act) {
		t.Fatal("packet above MinCount must be dropped")
	}
	now = now.Add(1100 * time.Millisecond)
	if !w.hackPacketCheck(o, act) {
		t.Fatal("window must reset after MaxDelay")
	}
}

func TestHackPacketCheckEndToEnd(t *testing.T) {
	env := newTestEnv(t)
	c := env.dial(t)
	c.next(0xF1, 0x00)
	c.verify()
	// F1 01 sent as C1 violates the Encrypt column: the server closes.
	body := []byte{0x01}
	body = append(body, make([]byte, 45)...)
	c.send(frame.Build(frame.C1, 0xF1, body...))
	c.nc.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 64)
	for {
		if _, err := c.nc.Read(buf); err != nil {
			if ne, ok := err.(interface{ Timeout() bool }); ok && ne.Timeout() {
				t.Fatal("server did not close the connection")
			}
			return
		}
	}
}

func discardLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }
