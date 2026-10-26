package gamedata

import ()

// WizardTeam custom events of Data/Custom/Event, now in
// config/data/custom-event.toml (loaders called from FUN_004e8390's
// start-up list: DuelEvent FUN_0045d1f0, DeathEvent FUN_0045b890,
// RoletaEvent FUN_004e42e0, RaffleEvent FUN_004e0350, PairEvent
// FUN_004cebf0, SobreEvent FUN_00502c70, TheftEvent FUN_00505fa0).

// EventPlace is a map and cell (byte fields of the event records).
type EventPlace struct{ Map, X, Y int }

// EventStart is a schedule row: weekday (0 = Sunday), hour, minute and
// the registration time in minutes; Extra is the fifth column (DuelEvent
// class, Death/Theft duration in minutes).
type EventStart struct {
	Day, Hour, Minute, Time, Extra int
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

func killEvent(f KillEventFile) *KillEvent {
	return &KillEvent{Active: f.Active > 0, Syntax: syntax(f.Syntax), Patente: f.Patente > 0, Level: f.Level,
		Place: EventPlace{f.Map, f.X, f.Y}, Starts: eventStartsOf(f.Start)}
}

// LoadDeathEvent reads the DeathEvent table of config/data/custom-event.toml
// (FUN_0045b890).
func LoadDeathEvent(configDir string) (*KillEvent, error) {
	f, err := LoadCustomEventConfig(configDir)
	if err != nil {
		return nil, err
	}
	return killEvent(f.DeathEvent), nil
}

// LoadTheftEvent reads the TheftEvent table of config/data/custom-event.toml
// (FUN_00505fa0).
func LoadTheftEvent(configDir string) (*KillEvent, error) {
	f, err := LoadCustomEventConfig(configDir)
	if err != nil {
		return nil, err
	}
	return killEvent(f.TheftEvent), nil
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

func fightEvent(f FightEventFile) *FightEvent {
	return &FightEvent{Active: f.Active > 0, Syntax: syntax(f.Syntax), Patente: f.Patente > 0, Level: f.Level,
		Wait: EventPlace{f.WaitMap, f.WaitX, f.WaitY}, Fight: EventPlace{f.FightMap, f.FightX, f.FightY},
		Starts: eventStartsOf(f.Start)}
}

// LoadDuelEvent reads the DuelEvent table of config/data/custom-event.toml
// (FUN_0045d1f0).
func LoadDuelEvent(configDir string) (*FightEvent, error) {
	f, err := LoadCustomEventConfig(configDir)
	if err != nil {
		return nil, err
	}
	return fightEvent(f.DuelEvent), nil
}

// LoadPairEvent reads the PairEvent table of config/data/custom-event.toml
// (FUN_004cebf0).
func LoadPairEvent(configDir string) (*FightEvent, error) {
	f, err := LoadCustomEventConfig(configDir)
	if err != nil {
		return nil, err
	}
	return fightEvent(f.PairEvent), nil
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

// LoadSobreEvent reads the SobreEvent table of config/data/custom-event.toml.
func LoadSobreEvent(configDir string) (*SobreEvent, error) {
	c, err := LoadCustomEventConfig(configDir)
	if err != nil {
		return nil, err
	}
	f := c.SobreEvent
	return &SobreEvent{Active: f.Active > 0, Syntax: syntax(f.Syntax), Patente: f.Patente > 0, Level: f.Level,
		Place: EventPlace{f.Map, f.X, f.Y}, MinPlayers: f.MinPlayers, Starts: eventStartsOf(f.Start)}, nil
}

// RoletaEvent is RoletaEvent.txt (FUN_004e42e0): section 0 row 0 Active
// Syntax, row 1 Level Map X Y. It has no schedule: only /rodar starts it.
type RoletaEvent struct {
	Active bool
	Syntax string
	Level  int
	Place  EventPlace
}

// LoadRoletaEvent reads the RoletaEvent table of config/data/custom-event.toml.
func LoadRoletaEvent(configDir string) (*RoletaEvent, error) {
	c, err := LoadCustomEventConfig(configDir)
	if err != nil {
		return nil, err
	}
	f := c.RoletaEvent
	return &RoletaEvent{Active: f.Active > 0, Syntax: syntax(f.Syntax), Level: f.Level,
		Place: EventPlace{f.Map, f.X, f.Y}}, nil
}

// RaffleEvent is RaffleEvent.txt (FUN_004e0350): section 0 Active,
// section 1 Day Hour Minute.
type RaffleEvent struct {
	Active bool
	Starts []EventStart
}

// LoadRaffleEvent reads the RaffleEvent table of config/data/custom-event.toml.
func LoadRaffleEvent(configDir string) (*RaffleEvent, error) {
	c, err := LoadCustomEventConfig(configDir)
	if err != nil {
		return nil, err
	}
	return &RaffleEvent{Active: c.RaffleEvent.Active > 0, Starts: eventStartsOf(c.RaffleEvent.Start)}, nil
}

// syntax is the strcpy_s(dst, 0x19, cell) of the player command.
func syntax(s string) string {
	if len(s) > 0x18 {
		s = s[:0x18]
	}
	return s
}
