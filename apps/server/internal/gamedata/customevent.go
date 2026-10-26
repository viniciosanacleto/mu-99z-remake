package gamedata

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/config/jsonconf"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/config/mutext"
)

// WizardTeam custom events of Data/Custom/Event (loaders called from
// FUN_004e8390's start-up list: DuelEvent FUN_0045d1f0, DeathEvent
// FUN_0045b890, RoletaEvent FUN_004e42e0, RaffleEvent FUN_004e0350,
// PairEvent FUN_004cebf0, SobreEvent FUN_00502c70, TheftEvent
// FUN_00505fa0). The files are read by the WizardTeam script reader
// (FUN_00409600 / FUN_00451390): numbered sections closed by "end", ';'
// and "//" comments, quoted strings. A missing cell reads as -1 for the
// integer fields and 0xFF for the byte fields (map, x, y).

// EventSection is one numbered section of a script file: rows of cells.
type EventSection struct {
	Index int
	Rows  [][]string
}

// LoadEventScript reads a WizardTeam script file (converted with
// mutext.TokenizeScript).
func LoadEventScript(path string) ([]EventSection, error) {
	lines, err := jsonconf.LoadLines(path)
	if err != nil {
		return nil, err
	}
	secs, err := parseEventScript(lines)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return secs, nil
}

// parseEventScript groups script lines into numbered sections closed by
// "end".
func parseEventScript(lines []mutext.Line) ([]EventSection, error) {
	var out []EventSection
	var cur *EventSection
	for _, l := range lines {
		toks := l.Tokens
		if cur == nil {
			n, err := strconv.Atoi(toks[0])
			if err != nil || len(toks) != 1 {
				return nil, fmt.Errorf("line %d: expected section number, got %q", l.No, toks)
			}
			out = append(out, EventSection{Index: n})
			cur = &out[len(out)-1]
			continue
		}
		if len(toks) == 1 && strings.EqualFold(toks[0], "end") {
			cur = nil
			continue
		}
		cur.Rows = append(cur.Rows, toks)
	}
	if cur != nil {
		return nil, fmt.Errorf("section %d without end", cur.Index)
	}
	return out, nil
}

// eventCells gives typed access to the cells of a section with the
// defaults of FUN_004524e0 (missing row or column).
type eventCells struct{ secs []EventSection }

func (c eventCells) section(n int) *EventSection {
	for i := range c.secs {
		if c.secs[i].Index == n {
			return &c.secs[i]
		}
	}
	return nil
}

func (c eventCells) str(sec, row, col int) (string, bool) {
	s := c.section(sec)
	if s == nil || row >= len(s.Rows) || col >= len(s.Rows[row]) {
		return "", false
	}
	return s.Rows[row][col], true
}

// int is atoi of a cell, -1 when it is missing.
func (c eventCells) int(sec, row, col int) int {
	v, ok := c.str(sec, row, col)
	if !ok {
		return -1
	}
	return cAtoi(v)
}

// byte is a byte cell (map, x, y), 0xFF when it is missing.
func (c eventCells) byte(sec, row, col int) int {
	v, ok := c.str(sec, row, col)
	if !ok {
		return 0xFF
	}
	return cAtoi(v) & 0xFF
}

// rows returns the rows of a section as ints (atoi; a missing cell -1).
func (c eventCells) rows(sec, cols int) [][]int {
	s := c.section(sec)
	if s == nil {
		return nil
	}
	out := make([][]int, len(s.Rows))
	for i, r := range s.Rows {
		v := make([]int, cols)
		for k := range v {
			v[k] = -1
			if k < len(r) {
				v[k] = cAtoi(r[k])
			}
		}
		out[i] = v
	}
	return out
}

// cAtoi is the C atoi (leading blanks, sign, digits).
func cAtoi(s string) int {
	s = strings.TrimLeft(s, " \t")
	neg := false
	if s != "" && (s[0] == '-' || s[0] == '+') {
		neg = s[0] == '-'
		s = s[1:]
	}
	n := 0
	for i := 0; i < len(s) && s[i] >= '0' && s[i] <= '9'; i++ {
		n = n*10 + int(s[i]-'0')
	}
	if neg {
		return -n
	}
	return n
}

// syntax is the strcpy_s(dst, 0x19, cell) of the player command.
func syntax(s string) string {
	if len(s) > 0x18 {
		s = s[:0x18]
	}
	return s
}

func loadEventCells(dataDir, name string) (eventCells, error) {
	secs, err := LoadEventScript(filepath.Join(dataDir, "Custom", "Event", name))
	return eventCells{secs}, err
}

// EventPlace is a map and cell (byte fields of the event records).
type EventPlace struct{ Map, X, Y int }

// EventStart is a schedule row: weekday (0 = Sunday), hour, minute and
// the registration time in minutes; Extra is the fifth column (DuelEvent
// class, Death/Theft duration in minutes).
type EventStart struct {
	Day, Hour, Minute, Time, Extra int
}

func eventStarts(c eventCells, cols int) []EventStart {
	var out []EventStart
	for _, v := range c.rows(1, cols) {
		s := EventStart{Day: v[0], Hour: v[1], Minute: v[2], Extra: -1}
		if cols > 3 {
			s.Time = v[3]
		}
		if cols > 4 {
			s.Extra = v[4]
		}
		out = append(out, s)
	}
	return out
}

// KillEvent is DeathEvent.txt / TheftEvent.txt: section 0 row 0 Active
// Syntax Patente, row 1 Level Map X Y; section 1 Day Hour Minute Time
// Duration (the loader keeps Duration x 60000 ms; here it stays in
// minutes).
type KillEvent struct {
	Active  bool
	Syntax  string
	Patente bool
	Level   int
	Place   EventPlace
	Starts  []EventStart
}

func loadKillEvent(dataDir, name string) (*KillEvent, error) {
	c, err := loadEventCells(dataDir, name)
	if err != nil {
		return nil, err
	}
	e := &KillEvent{Active: c.int(0, 0, 0) > 0, Patente: c.int(0, 0, 2) > 0, Level: c.int(0, 1, 0),
		Place: EventPlace{c.byte(0, 1, 1), c.byte(0, 1, 2), c.byte(0, 1, 3)}, Starts: eventStarts(c, 5)}
	e.Syntax, _ = c.str(0, 0, 1)
	e.Syntax = syntax(e.Syntax)
	return e, nil
}

// LoadDeathEvent reads Custom/Event/DeathEvent.txt (FUN_0045b890).
func LoadDeathEvent(dataDir string) (*KillEvent, error) {
	return loadKillEvent(dataDir, "DeathEvent.json")
}

// LoadTheftEvent reads Custom/Event/TheftEvent.txt (FUN_00505fa0).
func LoadTheftEvent(dataDir string) (*KillEvent, error) {
	return loadKillEvent(dataDir, "TheftEvent.json")
}

// FightEvent is DuelEvent.txt / PairEvent.txt: section 0 row 0 Active
// Syntax Patente, row 1 Level WaitMap WaitX WaitY FightMap FightX FightY;
// section 1 Day Hour Minute Time (and Class for the duel).
type FightEvent struct {
	Active      bool
	Syntax      string
	Patente     bool
	Level       int
	Wait, Fight EventPlace
	Starts      []EventStart
}

func loadFightEvent(dataDir, name string, cols int) (*FightEvent, error) {
	c, err := loadEventCells(dataDir, name)
	if err != nil {
		return nil, err
	}
	e := &FightEvent{Active: c.int(0, 0, 0) > 0, Patente: c.int(0, 0, 2) > 0, Level: c.int(0, 1, 0),
		Wait:   EventPlace{c.byte(0, 1, 1), c.byte(0, 1, 2), c.byte(0, 1, 3)},
		Fight:  EventPlace{c.byte(0, 1, 4), c.byte(0, 1, 5), c.byte(0, 1, 6)},
		Starts: eventStarts(c, cols)}
	e.Syntax, _ = c.str(0, 0, 1)
	e.Syntax = syntax(e.Syntax)
	return e, nil
}

// LoadDuelEvent reads Custom/Event/DuelEvent.txt (FUN_0045d1f0).
func LoadDuelEvent(dataDir string) (*FightEvent, error) {
	return loadFightEvent(dataDir, "DuelEvent.json", 5)
}

// LoadPairEvent reads Custom/Event/PairEvent.txt (FUN_004cebf0).
func LoadPairEvent(dataDir string) (*FightEvent, error) {
	return loadFightEvent(dataDir, "PairEvent.json", 4)
}

// SobreEvent is SobreEvent.txt (FUN_00502c70): section 0 row 0 Active
// Syntax Patente, row 1 Level Map X Y Players; section 1 Day Hour Minute
// Time.
type SobreEvent struct {
	Active     bool
	Syntax     string
	Patente    bool
	Level      int
	Place      EventPlace
	MinPlayers int
	Starts     []EventStart
}

// LoadSobreEvent reads Custom/Event/SobreEvent.txt.
func LoadSobreEvent(dataDir string) (*SobreEvent, error) {
	c, err := loadEventCells(dataDir, "SobreEvent.json")
	if err != nil {
		return nil, err
	}
	e := &SobreEvent{Active: c.int(0, 0, 0) > 0, Patente: c.int(0, 0, 2) > 0, Level: c.int(0, 1, 0),
		Place: EventPlace{c.byte(0, 1, 1), c.byte(0, 1, 2), c.byte(0, 1, 3)}, MinPlayers: c.byte(0, 1, 4),
		Starts: eventStarts(c, 4)}
	e.Syntax, _ = c.str(0, 0, 1)
	e.Syntax = syntax(e.Syntax)
	return e, nil
}

// RoletaEvent is RoletaEvent.txt (FUN_004e42e0): section 0 row 0 Active
// Syntax, row 1 Level Map X Y. It has no schedule: only /rodar starts it.
type RoletaEvent struct {
	Active bool
	Syntax string
	Level  int
	Place  EventPlace
}

// LoadRoletaEvent reads Custom/Event/RoletaEvent.txt.
func LoadRoletaEvent(dataDir string) (*RoletaEvent, error) {
	c, err := loadEventCells(dataDir, "RoletaEvent.json")
	if err != nil {
		return nil, err
	}
	e := &RoletaEvent{Active: c.int(0, 0, 0) > 0, Level: c.int(0, 1, 0),
		Place: EventPlace{c.byte(0, 1, 1), c.byte(0, 1, 2), c.byte(0, 1, 3)}}
	e.Syntax, _ = c.str(0, 0, 1)
	e.Syntax = syntax(e.Syntax)
	return e, nil
}

// RaffleEvent is RaffleEvent.txt (FUN_004e0350): section 0 Active,
// section 1 Day Hour Minute.
type RaffleEvent struct {
	Active bool
	Starts []EventStart
}

// LoadRaffleEvent reads Custom/Event/RaffleEvent.txt.
func LoadRaffleEvent(dataDir string) (*RaffleEvent, error) {
	c, err := loadEventCells(dataDir, "RaffleEvent.json")
	if err != nil {
		return nil, err
	}
	return &RaffleEvent{Active: c.int(0, 0, 0) > 0, Starts: eventStarts(c, 3)}, nil
}
