package capture

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/frame"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/gscodec"
)

// FormatVersion is the version of the golden-vector file format.
const FormatVersion = 1

// Session is one proxied TCP connection; saved as a golden-vector file.
type Session struct {
	Version  int       `json:"version"`
	ID       int       `json:"id"`
	Kind     Kind      `json:"kind"`
	Client   string    `json:"client"`
	Upstream string    `json:"upstream"`
	Start    time.Time `json:"start"`
	// StreamAdd/StreamXor are the GameServer stream cipher keys used.
	StreamAdd byte     `json:"stream_add,omitempty"`
	StreamXor byte     `json:"stream_xor,omitempty"`
	Records   []Record `json:"records"`

	mu sync.Mutex
}

func (s *Session) add(r Record) {
	s.mu.Lock()
	s.Records = append(s.Records, r)
	s.mu.Unlock()
}

// Save writes the session as indented JSON into dir and returns the path.
func (s *Session) Save(dir string) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	b, err := json.MarshalIndent(s, "", " ")
	if err != nil {
		return "", err
	}
	name := fmt.Sprintf("%s-%04d-%s.json", s.Start.UTC().Format("20060102T150405"), s.ID, s.Kind)
	path := filepath.Join(dir, name)
	return path, os.WriteFile(path, b, 0o644)
}

// LoadSession reads a golden-vector file.
func LoadSession(path string) (*Session, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	s := &Session{}
	if err := json.Unmarshal(b, s); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if s.Version != FormatVersion {
		return nil, fmt.Errorf("%s: format version %d, want %d", path, s.Version, FormatVersion)
	}
	return s, nil
}

// Replay checks every record of a session against the codecs, in order and
// in both roles:
//
//   - client -> server: gscodec.Server decodes Wire into Plain, and
//     gscodec.Client (Enc1.dat, serial from the record) encodes Plain back
//     into Wire;
//   - server -> client: gscodec.Client decodes Wire into Plain, and
//     gscodec.Server (Enc2.dat, serial from the record) encodes Plain back
//     into Wire.
//
// ConnectServer sessions only check the framing (Wire == Plain, one frame).
// Records that failed to decode at capture time are skipped.
func Replay(s *Session, k Keys) error {
	if s.Kind == Game {
		k.Stream.Add, k.Stream.Xor = s.StreamAdd, s.StreamXor
	}
	srvDec := gscodec.NewServer(gscodec.Keys{Stream: k.Stream, Decrypt: k.ServerDec})
	cliDec := gscodec.NewClient(gscodec.Keys{Stream: k.Stream, Decrypt: k.ClientDec})
	srvEnc := gscodec.NewServer(gscodec.Keys{Stream: k.Stream, Encrypt: k.ServerEnc})
	cliEnc := gscodec.NewClient(gscodec.Keys{Stream: k.Stream, Encrypt: k.ClientEnc})
	for i := range s.Records {
		r := &s.Records[i]
		if r.Err != "" || len(r.Plain) == 0 {
			continue
		}
		if s.Kind != Game {
			n, err := frame.Next(r.Wire, frame.MaxSizeConnect)
			if err != nil || n != len(r.Wire) || !bytes.Equal(r.Wire, r.Plain) {
				return fmt.Errorf("record %d: bad ConnectServer frame %x", i, r.Wire)
			}
			continue
		}
		var pkts []gscodec.Packet
		var err error
		if r.Dir == ClientToServer {
			pkts, err = srvDec.Feed(r.Wire)
		} else {
			pkts, err = cliDec.Feed(r.Wire)
		}
		if err != nil {
			return fmt.Errorf("record %d (%s): decode: %w", i, r.Dir, err)
		}
		if len(pkts) != 1 || !bytes.Equal(pkts[0].Data, r.Plain) || pkts[0].Encrypted != r.Encrypted ||
			(r.Encrypted && pkts[0].Serial != r.Serial) {
			return fmt.Errorf("record %d (%s): decoded %+v, want %x", i, r.Dir, pkts, []byte(r.Plain))
		}
		p := append([]byte(nil), r.Plain...)
		p[0] = r.WireKind()
		var wire []byte
		if r.Dir == ClientToServer {
			cliEnc.SetSerial(r.Serial)
			wire, err = cliEnc.Encode(p)
		} else {
			srvEnc.SetSerial(r.Serial)
			wire, err = srvEnc.Encode(p)
		}
		if err != nil {
			return fmt.Errorf("record %d (%s): encode: %w", i, r.Dir, err)
		}
		if !bytes.Equal(wire, r.Wire) {
			return fmt.Errorf("record %d (%s): encoded %x, want %x", i, r.Dir, wire, []byte(r.Wire))
		}
	}
	return nil
}
