package devilsquare

import (
	"bytes"
	"testing"
	"time"
)

func TestSquareUser(t *testing.T) {
	var s Square
	for i := range s.Users {
		s.Users[i].Index = -1
	}
	s.Users[3] = User{Index: 8000, Score: 7}
	if u := s.User(8000); u == nil || u.Score != 7 {
		t.Fatalf("User(8000) = %+v, want the slot with score 7", u)
	}
	if u := s.User(8001); u != nil {
		t.Fatalf("User(8001) = %+v, want nil", u)
	}
	s.User(8000).Score = 9
	if s.Users[3].Score != 9 {
		t.Fatal("User does not return the slot itself")
	}
}

func TestSquareSetRemain(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	var s Square
	s.SetRemain(now, 90)
	if s.Remain != 90 || !s.Target.Equal(now.Add(90*time.Second)) {
		t.Fatalf("Remain %d Target %v", s.Remain, s.Target)
	}
}

func TestPackets(t *testing.T) {
	tests := []struct {
		name string
		got  []byte
		want []byte
	}{
		{name: "90 result", got: PacketResult(3), want: []byte{0xC1, 0x04, 0x90, 3}},
		{name: "92 state", got: PacketState(2), want: []byte{0xC1, 0x04, 0x92, 2}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !bytes.Equal(tt.got, tt.want) {
				t.Fatalf("got % X, want % X", tt.got, tt.want)
			}
		})
	}
}
