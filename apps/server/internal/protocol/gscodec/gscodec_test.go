package gscodec

import (
	"bytes"
	"errors"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/crypto"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/frame"
)

func load(t *testing.T, parts ...string) crypto.ModulusKeys {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	k, err := crypto.LoadKeyFile(filepath.Join(append([]string{filepath.Dir(file)}, parts...)...))
	if err != nil {
		t.Fatal(err)
	}
	return k
}

func pair(t *testing.T) (*Server, *Client) {
	stream, err := crypto.DeriveStreamKeys("WizardTeam", "A9kT3wX5rBz7fQ1u")
	if err != nil {
		t.Fatal(err)
	}
	srv := NewServer(Keys{
		Stream:  stream,
		Encrypt: load(t, "..", "..", "..", "data", "Data", "Hack", "Enc2.dat"),
		Decrypt: load(t, "..", "..", "..", "data", "Data", "Hack", "Dec1.dat"),
	})
	cli := NewClient(Keys{
		Stream:  stream,
		Encrypt: load(t, "..", "crypto", "testdata", "Enc1.dat"),
		Decrypt: load(t, "..", "crypto", "testdata", "Dec2.dat"),
	})
	return srv, cli
}

func TestClientToServer(t *testing.T) {
	srv, cli := pair(t)
	login := frame.Build(frame.C3, 0xF1, append([]byte{0x01}, bytes.Repeat([]byte{'a'}, 45)...)...)
	walk := frame.Build(frame.C1, 0xD3, 0x80, 0x80, 0x24, 0x00)
	big := frame.Build(frame.C4, 0xF3, make([]byte, 300)...)

	var wire []byte
	for _, p := range [][]byte{login, walk, big} {
		enc, err := cli.Encode(p)
		if err != nil {
			t.Fatal(err)
		}
		wire = append(wire, enc...)
	}

	// Deliver in awkward chunks to exercise the incremental parser.
	var got []Packet
	for len(wire) > 0 {
		n := min(7, len(wire))
		pkts, err := srv.Feed(wire[:n])
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, pkts...)
		wire = wire[n:]
	}
	if len(got) != 3 {
		t.Fatalf("got %d packets", len(got))
	}

	wantLogin := bytes.Clone(login)
	wantLogin[0] = frame.C1
	if !got[0].Encrypted || got[0].Serial != 0 || !bytes.Equal(got[0].Data, wantLogin) {
		t.Fatalf("login = %+v", got[0])
	}
	if got[1].Encrypted || !bytes.Equal(got[1].Data, walk) {
		t.Fatalf("walk = % X", got[1].Data)
	}
	wantBig := bytes.Clone(big)
	wantBig[0] = frame.C2
	if !got[2].Encrypted || got[2].Serial != 1 || !bytes.Equal(got[2].Data, wantBig) {
		t.Fatalf("big: serial %d, % X", got[2].Serial, got[2].Data[:8])
	}
}

func TestServerToClient(t *testing.T) {
	srv, cli := pair(t)
	hello := frame.Build(frame.C1, 0xF1, 0x00, 0x01, 0x53, 0xFC, '1', '0', '0', '0', '0')
	charInfo := frame.Build(frame.C3, 0xF3, append([]byte{0x03}, make([]byte, 111)...)...)

	var wire []byte
	for _, p := range [][]byte{hello, charInfo, charInfo} {
		enc, err := srv.Encode(p)
		if err != nil {
			t.Fatal(err)
		}
		wire = append(wire, enc...)
	}
	if wire[0] == frame.C1 {
		t.Fatal("stream cipher not applied to outgoing bytes")
	}
	got, err := cli.Feed(wire)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || !bytes.Equal(got[0].Data, hello) {
		t.Fatalf("got %d packets, first % X", len(got), got[0].Data)
	}
	if got[1].Serial != 0 || got[2].Serial != 1 || got[1].Data[0] != frame.C1 ||
		!bytes.Equal(got[1].Data[2:], charInfo[2:]) {
		t.Fatalf("charInfo = %+v", got[1])
	}
}

func TestServerRejectsGarbage(t *testing.T) {
	srv, _ := pair(t)
	raw := []byte{0x00, 0x01, 0x02}
	srv.keys.Stream.Encode(raw) // so it decodes back to 0x00...
	if _, err := srv.Feed(raw); !errors.Is(err, frame.ErrHeader) {
		t.Fatalf("err = %v", err)
	}
}

func TestServerRejectsBadChecksum(t *testing.T) {
	srv, cli := pair(t)
	enc, err := cli.Encode(frame.Build(frame.C3, 0xF1, 0x01, 0x02))
	if err != nil {
		t.Fatal(err)
	}
	enc[len(enc)-1] ^= 0xFF
	if _, err := srv.Feed(enc); !errors.Is(err, crypto.ErrBlock) {
		t.Fatalf("err = %v", err)
	}
}

func TestEncodeLimits(t *testing.T) {
	srv, _ := pair(t)
	// 200 plain bytes -> 26 blocks -> 286 bytes: does not fit a C3.
	if _, err := srv.Encode(frame.Build(frame.C3, 0x00, make([]byte, 197)...)); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("err = %v", err)
	}
	if _, err := srv.Encode([]byte{0x01, 0x03, 0x00}); !errors.Is(err, ErrNotPacket) {
		t.Fatalf("err = %v", err)
	}
}

func TestSetSerial(t *testing.T) {
	srv, cli := pair(t)
	p := frame.Build(frame.C3, 0xF1, 0x01, 0x02)
	cli.SetSerial(0x41)
	srv.SetSerial(0x42)
	wire, err := cli.Encode(p)
	if err != nil {
		t.Fatal(err)
	}
	got, err := srv.Feed(wire)
	if err != nil || len(got) != 1 || got[0].Serial != 0x41 {
		t.Fatalf("client serial: %+v %v", got, err)
	}
	out, err := srv.Encode(p)
	if err != nil {
		t.Fatal(err)
	}
	back, err := cli.Feed(out)
	if err != nil || len(back) != 1 || back[0].Serial != 0x42 {
		t.Fatalf("server serial: %+v %v", back, err)
	}
}
