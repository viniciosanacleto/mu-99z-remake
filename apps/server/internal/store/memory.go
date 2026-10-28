package store

import (
	"bytes"
	"context"
	"strings"
	"sync"
	"time"
)

// Memory is an in-memory Store for tests and database-less development.
type Memory struct {
	mu         sync.Mutex
	templates  map[byte]ClassTemplate
	accounts   map[int64]*Account
	chars      map[int64]*Character
	guildStat  map[string]byte
	warehouses map[[2]int64]*Warehouse
	options    map[int64]*Options
	online     map[int64]bool
	nextID     int64
	serial     uint32
	fr         *memFriends
	gd         *memGuilds                // guild_memory.go
	pets       map[uint32]PetInfo        // pet_memory.go
	entries    map[string]*eventEntryRow // event_entry.go
	renas      map[string]int            // golden_archer.go
	castles    map[int]*memCastle        // castle.go
	resets     map[string]*resetRow      // command.go
	duelRank   map[string][2]int         // ranking.go
	eventRank  map[rankKey]int           // ranking.go
	customRank map[customRankKey]int     // customevent.go
	periodic   map[uint32]int64          // periodic.go
	custom     *memCustom                // custom.go
	cworld     *memCustomWorld           // customworld.go
	reconnect  memReconnect              // reconnect.go
}

func NewMemory() (*Memory, error) {
	t, err := ClassTemplates()
	if err != nil {
		return nil, err
	}
	return &Memory{
		templates: t, accounts: map[int64]*Account{}, chars: map[int64]*Character{},
		guildStat: map[string]byte{}, warehouses: map[[2]int64]*Warehouse{},
		options: map[int64]*Options{}, online: map[int64]bool{},
	}, nil
}

func (m *Memory) Close() {}

func (m *Memory) id() int64 { m.nextID++; return m.nextID }

func cloneAccount(a *Account) *Account { c := *a; return &c }

func cloneChar(c *Character) *Character {
	d := *c
	d.Inventory = bytes.Clone(c.Inventory)
	d.MagicList = bytes.Clone(c.MagicList)
	d.EffectList = bytes.Clone(c.EffectList)
	d.Quest = bytes.Clone(c.Quest)
	return &d
}

func (m *Memory) AccountByLogin(_ context.Context, login string) (*Account, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, a := range m.accounts {
		if strings.EqualFold(a.Login, login) {
			return cloneAccount(a), nil
		}
	}
	return nil, ErrNotFound
}

func (m *Memory) CreateAccount(_ context.Context, login, hash, personalID string) (*Account, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, a := range m.accounts {
		if strings.EqualFold(a.Login, login) {
			return nil, ErrDuplicate
		}
	}
	a := &Account{ID: m.id(), Login: login, PasswordHash: hash, PersonalID: personalID}
	m.accounts[a.ID] = a
	return cloneAccount(a), nil
}

func (m *Memory) withAccount(id int64, f func(*Account)) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	a := m.accounts[id]
	if a == nil {
		return ErrNotFound
	}
	f(a)
	return nil
}

func (m *Memory) SetPasswordHash(_ context.Context, id int64, hash string) error {
	return m.withAccount(id, func(a *Account) { a.PasswordHash = hash })
}

func (m *Memory) SetAccountLevel(_ context.Context, id int64, level int, expires time.Time) error {
	return m.withAccount(id, func(a *Account) { a.Level, a.LevelExpiresAt = level, expires })
}

func (m *Memory) SetAccountLock(_ context.Context, id int64, lock int) error {
	return m.withAccount(id, func(a *Account) { a.Lock = lock })
}

func (m *Memory) SetExtClass(_ context.Context, id int64, ext int) error {
	return m.withAccount(id, func(a *Account) { a.ExtClass = ext })
}

func (m *Memory) ConnectAccount(_ context.Context, id int64, _, _ string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.online[id] = true
	return nil
}

func (m *Memory) DisconnectAccount(_ context.Context, id int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.online, id)
	return nil
}

func (m *Memory) ResetSessions(context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.online = map[int64]bool{}
	return nil
}

// Online reports whether ConnectAccount was called without a matching disconnect.
func (m *Memory) Online(id int64) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.online[id]
}

func (m *Memory) ListCharacters(_ context.Context, accountID int64) ([]CharacterSummary, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []CharacterSummary
	for slot := 0; slot < MaxCharacters; slot++ {
		for _, c := range m.chars {
			if c.AccountID != accountID || c.Slot != slot {
				continue
			}
			gs, ok := m.guildStat[strings.ToLower(c.Name)]
			if !ok {
				gs = GuildStatusNone
			}
			out = append(out, CharacterSummary{
				Slot: c.Slot, Name: c.Name, Level: c.Level, Class: c.Class, CtlCode: c.CtlCode,
				Inventory: bytes.Clone(c.Inventory[:SummaryItems*ItemDBSize]), GuildStatus: gs,
			})
		}
	}
	return out, nil
}

func (m *Memory) CharacterExists(_ context.Context, name string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.nameTaken(name), nil
}

func (m *Memory) nameTaken(name string) bool {
	for _, c := range m.chars {
		if strings.EqualFold(c.Name, name) {
			return true
		}
	}
	return false
}

func (m *Memory) CreateCharacter(_ context.Context, accountID int64, name string, class byte) (CreateResult, int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.nameTaken(name) {
		return CreateNameExists, 0, nil
	}
	used := map[int]bool{}
	for _, c := range m.chars {
		if c.AccountID == accountID {
			used[c.Slot] = true
		}
	}
	slot := -1
	for s := 0; s < MaxCharacters; s++ {
		if !used[s] {
			slot = s
			break
		}
	}
	if slot < 0 {
		return CreateNoSlot, 0, nil
	}
	t, ok := m.templates[class]
	if !ok {
		return CreateError, 0, nil
	}
	c := newFromTemplate(t, accountID, slot, name, class)
	c.ID = m.id()
	m.chars[c.ID] = c
	return CreateOK, slot, nil
}

func newFromTemplate(t ClassTemplate, accountID int64, slot int, name string, class byte) *Character {
	return &Character{
		AccountID: accountID, Slot: slot, Name: name, Class: class,
		Level: t.Level, LevelUpPoints: t.LevelUpPoints,
		Strength: t.Strength, Dexterity: t.Dexterity, Vitality: t.Vitality, Energy: t.Energy, Leadership: t.Leadership,
		Inventory: bytes.Clone(t.Inventory), MagicList: bytes.Clone(t.MagicList),
		Life: t.Life, MaxLife: t.MaxLife, Mana: t.Mana, MaxMana: t.MaxMana,
		Map: t.Map, X: t.X, Y: t.Y, PKLevel: 3,
		Quest: bytes.Clone(t.Quest), EffectList: bytes.Clone(t.EffectList), IsNew: true,
	}
}

func (m *Memory) DeleteCharacter(_ context.Context, accountID int64, name string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, c := range m.chars {
		if c.AccountID == accountID && strings.EqualFold(c.Name, name) {
			delete(m.chars, id)
			delete(m.options, id)
			m.friends().remove(name)
			m.removeGuildMember(name)
			m.deleteCustom(name) // custom.go
			return true, nil
		}
	}
	return false, nil
}

func (m *Memory) LoadCharacter(_ context.Context, accountID int64, name string) (*Character, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, c := range m.chars {
		if c.AccountID == accountID && strings.EqualFold(c.Name, name) {
			return cloneChar(c), nil
		}
	}
	return nil, ErrNotFound
}

func (m *Memory) SaveCharacter(_ context.Context, c *Character) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	old := m.chars[c.ID]
	if old == nil {
		return ErrNotFound
	}
	n := cloneChar(c)
	n.GMLevel = old.GMLevel // like gm_level, only SetGMLevel writes it
	m.chars[c.ID] = n
	return nil
}

func (m *Memory) SaveInventory(_ context.Context, id int64, inv []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	c := m.chars[id]
	if c == nil {
		return ErrNotFound
	}
	c.Inventory = bytes.Clone(inv)
	return nil
}

// SetGuildStatus is a test helper for the G_Status shown in the character list.
func (m *Memory) SetGuildStatus(name string, status byte) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.guildStat[strings.ToLower(name)] = status
}

func (m *Memory) LoadWarehouse(_ context.Context, accountID int64, number int) (*Warehouse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := [2]int64{accountID, int64(number)}
	w := m.warehouses[key]
	if w == nil {
		w = &Warehouse{AccountID: accountID, Number: number, Items: EmptyItems(WarehouseSize / ItemDBSize)}
		m.warehouses[key] = w
	}
	c := *w
	c.Items = bytes.Clone(w.Items)
	return &c, nil
}

func (m *Memory) SaveWarehouse(_ context.Context, w *Warehouse) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	c := *w
	c.Items = bytes.Clone(w.Items)
	m.warehouses[[2]int64{w.AccountID, int64(w.Number)}] = &c
	return nil
}

func (m *Memory) LoadOptions(_ context.Context, id int64) (*Options, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	o := m.options[id]
	if o == nil {
		return nil, ErrNotFound
	}
	c := *o
	c.SkillKeys = bytes.Clone(o.SkillKeys)
	return &c, nil
}

func (m *Memory) SaveOptions(_ context.Context, id int64, o *Options) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	c := *o
	c.SkillKeys = bytes.Clone(o.SkillKeys)
	m.options[id] = &c
	return nil
}

func (m *Memory) NextItemSerial(context.Context) (uint32, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.serial++
	return m.serial, nil
}
