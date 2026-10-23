package game

import (
	"bytes"
	"testing"
)

func TestWhisperRelayAcrossInstances(t *testing.T) {
	a, b := newBareWorld(t), newBareWorld(t)
	b.cfg.ServerCode = a.cfg.ServerCode + 19
	h := NewHub()
	for _, w := range []*World{a.World, b.World} {
		w.hub = h
		h.register(w)
	}
	ann := a.player(PlayerStart, "Ann", 0, 130, 130)
	bob := b.player(PlayerStart, "Bob", 0, 130, 130)
	h.Enter(OnlineChar{Name: "Ann", ServerCode: a.cfg.ServerCode, Index: ann.Index})
	h.Enter(OnlineChar{Name: "Bob", ServerCode: b.cfg.ServerCode, Index: bob.Index})

	if whisperRelay(a.World, ann, "bob", []byte("hi")) {
		t.Fatal("relayed with a different-case name")
	}
	if whisperRelay(a.World, ann, "Ann", []byte("hi")) {
		t.Fatal("relayed to the own instance")
	}
	if !whisperRelay(a.World, ann, "Bob", []byte("hi")) {
		t.Fatal("not relayed")
	}
	(<-b.events)()
	p := find(b.drain(bob), 0x02, -1)
	if !bytes.Equal(p, packetWhisper("Ann", []byte("hi"))) {
		t.Fatalf("whisper = % X", p)
	}
}
