package game

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/frame"
)

// A clean stop (ctx cancelled) saves every playing character, like a
// disconnect does: the readers' disconnect events are not delivered once
// the world loop has stopped, so shutdown must remove the players itself.
func TestServeShutdownSavesPlayingCharacters(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	acc, err := env.store.AccountByLogin(ctx, "tester")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := env.store.CreateCharacter(ctx, acc.ID, "Saver", 0x10); err != nil {
		t.Fatal(err)
	}
	c := env.dial(t)
	c.next(0xF1, 0x00)
	c.verify()
	if r := c.login("tester", "pass"); r != 1 {
		t.Fatalf("login = %d", r)
	}
	name := make([]byte, 10)
	copy(name, "Saver")
	c.send(frame.Build(frame.C1, 0xF3, append([]byte{0x03}, name...)...))
	c.next(0xF3, 0x03)

	const money = 123456
	onWorld(env, func(w *World) {
		for i := PlayerStart; i < MaxObjects; i++ {
			if o := w.objects[i]; o.IsPlayer() && o.P.Char != nil && o.P.Char.Name == "Saver" {
				o.P.Char.Money = money
				return
			}
		}
		t.Error("Saver is not in the world")
	})

	env.stop()
	got, err := env.store.LoadCharacter(ctx, acc.ID, "Saver")
	if err != nil {
		t.Fatal(err)
	}
	if got.Money != money {
		t.Fatalf("saved money = %d, want %d", got.Money, money)
	}
	if env.auth.Online("tester") {
		t.Fatal("account session still open after shutdown")
	}
}

// failingListener fails every Accept, like a listener whose socket broke.
type failingListener struct{ net.Listener }

var errListenerBroken = errors.New("listener broken")

func (failingListener) Accept() (net.Conn, error) { return nil, errListenerBroken }
func (failingListener) Close() error              { return nil }
func (failingListener) Addr() net.Addr            { return &net.TCPAddr{} }

// A listener failure (ctx still live) stops the world and is returned;
// Serve used to wait forever for a context nobody cancelled.
func TestServeReturnsListenerError(t *testing.T) {
	b := newBareWorld(t)
	done := make(chan error, 1)
	go func() { done <- b.Serve(t.Context(), failingListener{}) }()
	select {
	case err := <-done:
		if !errors.Is(err, errListenerBroken) {
			t.Fatalf("Serve = %v, want %v", err, errListenerBroken)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Serve did not return after the listener failed")
	}
}
