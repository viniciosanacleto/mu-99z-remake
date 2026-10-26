package game

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/config/jsonconf"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/config/mutext"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/frame"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/gscodec"
)

// HackPacketCheck (Data/Hack/HackPacketCheck.txt, CHackPacketCheck):
// every packet a player sends must have a rule for its head (and sub), the
// C1/C3 kind must match the Encrypt column, C3/C4 serials must increase by
// one, and MaxDelay/MinCount/MaxCount limit the rate per head.

// hackRule is one row: Index Value Encrypt MaxDelay MinCount MaxCount.
type hackRule struct {
	head, value int // value -1 = "*" (any sub)
	encrypt     int // -1 = "*"
	maxDelay    int // ms window (0 = none)
	minCount    int // count above which a "count error" is logged (0 = no limit)
	maxCount    int // count above which the client is closed (0 = never)
}

type hackRules struct {
	head [256]*hackRule      // the head row (FUN_00487f80 overwrites it per row)
	sub  map[[2]int]hackRule // rows with a specific Value
}

func loadHackRules(dataDir string) (*hackRules, error) {
	lines, err := jsonconf.LoadLines(filepath.Join(dataDir, "Hack", "HackPacketCheck.json"))
	if err != nil {
		return nil, err
	}
	r := &hackRules{sub: map[[2]int]hackRule{}}
	for _, l := range mutext.Table(lines) {
		if len(l.Tokens) != 6 {
			return nil, fmt.Errorf("HackPacketCheck.txt line %d: %d columns", l.No, len(l.Tokens))
		}
		v := [6]int{}
		for i := range v {
			if v[i], err = l.Int(i); err != nil {
				return nil, err
			}
		}
		r.add(hackRule{head: v[0], value: v[1], encrypt: v[2], maxDelay: v[3], minCount: v[4], maxCount: v[5]})
	}
	return r, nil
}

// add is FUN_00487f80.
func (r *hackRules) add(rule hackRule) {
	if rule.head < 0 || rule.head > 0xFF {
		return
	}
	h := rule
	r.head[rule.head] = &h
	if rule.value != -1 {
		r.sub[[2]int{rule.head, rule.value}] = rule
	}
}

// find is FUN_00488000.
func (r *hackRules) find(head, sub int) (hackRule, bool) {
	h := r.head[head]
	if h == nil {
		return hackRule{}, false
	}
	if h.value == -1 {
		return *h, true
	}
	s, ok := r.sub[[2]int{head, sub}]
	return s, ok
}

// hackCounter is the per-rule state of obj+0x604 (window start) and
// obj+0x608 (count).
type hackCounter struct {
	start time.Time
	count int
}

// hackPacketCheck is FUN_00488070. It returns false when the packet must be
// dropped; the client is closed on errors as in the original.
func (w *World) hackPacketCheck(o *Object, pkt gscodec.Packet) bool {
	if w.hackRules == nil {
		return true
	}
	p := pkt.Data
	head := int(frame.Head(p))
	sub := 0
	if s, ok := frame.Sub(p); ok {
		sub = int(s)
	}
	rule, ok := w.hackRules.find(head, sub)
	if !ok {
		w.log.Info("HackPacketCheck: packet unknown error", "index", o.Index, "head", head, "value", sub)
		w.closeClient(o, "HackPacketCheck unknown")
		return false
	}
	enc := 0
	if pkt.Encrypted {
		enc = 1
	}
	if rule.encrypt >= 0 && rule.encrypt != enc {
		w.log.Info("HackPacketCheck: packet encryption error", "index", o.Index, "head", head, "value", sub)
		w.closeClient(o, "HackPacketCheck encryption")
		return false
	}
	if pkt.Encrypted {
		if o.P.recvSerial+1 != pkt.Serial {
			w.log.Info("HackPacketCheck: packet serial error", "index", o.Index, "head", head,
				"want", o.P.recvSerial+1, "got", pkt.Serial)
			w.closeClient(o, "HackPacketCheck serial")
			return false
		}
		o.P.recvSerial++
	}
	if o.P.hackCounters == nil {
		o.P.hackCounters = map[[2]int]*hackCounter{}
	}
	key := [2]int{rule.head, rule.value}
	c := o.P.hackCounters[key]
	if c == nil {
		c = &hackCounter{}
		o.P.hackCounters[key] = c
	}
	now := w.now()
	if rule.maxDelay != 0 && now.Sub(c.start) >= time.Duration(rule.maxDelay)*time.Millisecond {
		c.start, c.count = now, 0
	}
	if rule.minCount == 0 {
		return true
	}
	c.count++
	if c.count <= rule.minCount {
		return true
	}
	w.log.Info("HackPacketCheck: packet count error", "index", o.Index, "head", head, "count", c.count)
	if rule.maxCount == 0 || c.count <= rule.maxCount {
		return false
	}
	w.closeClient(o, "HackPacketCheck count")
	return false
}
