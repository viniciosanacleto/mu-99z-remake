package loadbot

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"sync/atomic"
	"time"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/frame"
)

// Profile is the behaviour of a bot once it is in the world.
type Profile struct {
	WalkEvery   time.Duration // random walk when no monster is around (jittered up to 2x)
	AttackEvery time.Duration // D7 cadence while a monster is in reach
	ChatEvery   time.Duration // 0 = never
	LiveEvery   time.Duration // C3 0E keepalive
	Home        int           // cells the random walk may stray from the spawn point
	Hunt        int           // cells within which monsters are chased
}

// DefaultProfile is the "light + combat" profile of the load test.
var DefaultProfile = Profile{
	WalkEvery:   time.Second,
	AttackEvery: time.Second,
	ChatEvery:   20 * time.Second,
	LiveEvery:   20 * time.Second,
	Home:        4,
	Hunt:        8,
}

// Stats are shared by every bot of a run.
type Stats struct {
	LoginOK, LoginFail    atomic.Int64
	Disconnects           atomic.Int64
	Walks, Attacks, Chats atomic.Int64
	Hits, Kills, Deaths   atomic.Int64
	PacketsIn             atomic.Int64
	BytesIn, BytesOut     atomic.Int64
	Online                atomic.Int64
}

// Bot is one fake player.
type Bot struct {
	Client    Client
	Addr      string // GameServer address
	Account   string
	Password  string
	Char      string
	Class     byte // 0 DW, 1 DK, 2 Elf: the Elf attacks from farther away
	Profile   Profile
	Terrain   *gamedata.Terrain // optional: avoids walking into blocked cells
	Stats     *Stats
	Timeout   time.Duration
	Rand      *rand.Rand
	LoginTime time.Duration // set once the character is in the world
	Index     int           // own object index (F1 00)
	X, Y, Map int
	homeX     int
	homeY     int
	dead      bool
	monsters  map[int]*monster
	conn      *Conn
	start     time.Time
}

type monster struct{ x, y int }

// ErrLogin is returned when the server refuses the account or character.
var ErrLogin = errors.New("login refused")

func (b *Bot) timeout() time.Duration {
	if b.Timeout > 0 {
		return b.Timeout
	}
	return 10 * time.Second
}

func (b *Bot) tick() uint32 { return uint32(time.Since(b.start).Milliseconds()) }

// Run logs in, enters the world and plays until ctx is done. It returns
// nil when ctx ends the session and an error when the server does.
func (b *Bot) Run(ctx context.Context) error {
	if b.Stats == nil {
		b.Stats = &Stats{}
	}
	if b.Rand == nil {
		b.Rand = rand.New(rand.NewPCG(rand.Uint64(), rand.Uint64()))
	}
	if b.Profile == (Profile{}) {
		b.Profile = DefaultProfile
	}
	b.monsters = map[int]*monster{}
	b.start = time.Now()
	if err := b.enter(); err != nil {
		if b.conn != nil {
			b.conn.Close()
		}
		b.Stats.LoginFail.Add(1)
		return err
	}
	b.LoginTime = time.Since(b.start)
	b.Stats.LoginOK.Add(1)
	b.Stats.Online.Add(1)
	defer b.Stats.Online.Add(-1)
	defer b.conn.Close()
	err := b.play(ctx)
	b.Stats.BytesIn.Add(b.conn.BytesIn.Load())
	b.Stats.BytesOut.Add(b.conn.BytesOut.Load())
	if err != nil && ctx.Err() == nil {
		b.Stats.Disconnects.Add(1)
		return err
	}
	return nil
}

// enter is the client login sequence of TestLoginCreateEnterWorld.
func (b *Bot) enter() error {
	c, err := Dial(b.Addr, b.Client.Keys, b.timeout())
	if err != nil {
		return err
	}
	b.conn = c
	join, err := c.Next(0xF1, 0x00, b.timeout(), nil)
	if err != nil {
		return fmt.Errorf("join: %w", err)
	}
	if len(join) < 7 || join[4] != 1 {
		return fmt.Errorf("%w: join % X", ErrLogin, join)
	}
	b.Index = int(join[5])<<8 | int(join[6])
	steps := []struct {
		send      []byte
		head      byte
		sub       int
		check     func(p []byte) error
		noRespond bool
	}{
		{send: Verify(HWID, 1), noRespond: true},
		{send: Login(b.Account, b.Password, b.tick(), b.Client.Version, b.Client.Serial), head: 0xF1, sub: 0x01,
			check: func(p []byte) error {
				if len(p) < 5 || p[4] != 1 {
					return fmt.Errorf("%w: account %s result %d", ErrLogin, b.Account, p[4])
				}
				return nil
			}},
		{send: CharList(), head: 0xF3, sub: 0x00},
		{send: SelectChar(b.Char), head: 0xF3, sub: 0x03, check: func(p []byte) error {
			if len(p) < 7 {
				return fmt.Errorf("%w: short F3 03", ErrLogin)
			}
			b.X, b.Y, b.Map = int(p[4]), int(p[5]), int(p[6])
			b.homeX, b.homeY = b.X, b.Y
			return nil
		}},
	}
	for _, s := range steps {
		if err := c.Send(s.send); err != nil {
			return err
		}
		if s.noRespond {
			continue
		}
		p, err := c.Next(s.head, s.sub, b.timeout(), nil)
		if err != nil {
			return err
		}
		if s.check != nil {
			if err := s.check(p); err != nil {
				return err
			}
		}
	}
	return nil
}

// play is the in-world loop: every packet updates the bot's view, the
// ticker decides the next action.
func (b *Bot) play(ctx context.Context) error {
	tk := time.NewTicker(100 * time.Millisecond)
	defer tk.Stop()
	now := time.Now()
	nextWalk := now.Add(b.jitter(b.Profile.WalkEvery))
	nextAttack := now.Add(b.jitter(b.Profile.AttackEvery))
	nextChat := now.Add(b.jitter(b.Profile.ChatEvery))
	nextLive := now.Add(b.Profile.LiveEvery)
	for {
		select {
		case <-ctx.Done():
			return nil
		case p, ok := <-b.conn.In:
			if !ok {
				if err := b.conn.Err(); err != nil {
					return err
				}
				return errors.New("closed by server")
			}
			b.Stats.PacketsIn.Add(1)
			b.handle(p)
		case now := <-tk.C:
			var err error
			switch {
			case b.dead:
			case now.After(nextAttack) && b.attackOrChase():
				nextAttack = now.Add(b.Profile.AttackEvery)
				nextWalk = now.Add(b.jitter(b.Profile.WalkEvery))
			case now.After(nextWalk):
				err = b.wander()
				nextWalk = now.Add(b.jitter(b.Profile.WalkEvery))
			}
			if err == nil && b.Profile.ChatEvery > 0 && now.After(nextChat) {
				err = b.conn.Send(Chat(b.Char, "load test"))
				b.Stats.Chats.Add(1)
				nextChat = now.Add(b.jitter(b.Profile.ChatEvery))
			}
			if err == nil && now.After(nextLive) {
				err = b.conn.Send(LiveClient(b.tick(), 0, 0))
				nextLive = now.Add(b.Profile.LiveEvery)
			}
			if err != nil {
				return err
			}
		}
	}
}

// jitter returns d plus up to d of random delay.
func (b *Bot) jitter(d time.Duration) time.Duration {
	if d <= 0 {
		return time.Hour
	}
	return d + time.Duration(b.Rand.Int64N(int64(d)))
}

// handle updates position, monsters and counters from a server packet.
func (b *Bot) handle(p []byte) {
	switch {
	case Match(p, 0x13, -1) && p[0] == frame.C2: // monster viewport
		n := int(p[4])
		for i := 0; i < n && 5+16*(i+1) <= len(p); i++ {
			e := p[5+16*i:]
			b.monsters[index16(e)] = &monster{x: int(e[10]), y: int(e[11])}
		}
	case Match(p, 0x14, -1) && len(p) >= 4: // destroy
		for i := 0; i < int(p[3]) && 6+2*i <= len(p); i++ {
			delete(b.monsters, index16(p[4+2*i:]))
		}
	case Match(p, 0xD3, -1) && len(p) >= 7: // walk
		b.moved(index16(p[3:]), int(p[5]), int(p[6]))
	case Match(p, 0xDF, -1) && len(p) >= 7: // position
		b.moved(index16(p[3:]), int(p[5]), int(p[6]))
	case Match(p, 0xD7, -1) && len(p) >= 7: // damage
		if _, ok := b.monsters[index16(p[3:])]; ok {
			b.Stats.Hits.Add(1)
		}
	case Match(p, 0x17, -1) && len(p) >= 8: // die
		t := index16(p[3:])
		if t == b.Index {
			b.dead = true
			b.Stats.Deaths.Add(1)
		} else if _, ok := b.monsters[t]; ok {
			delete(b.monsters, t)
			if index16(p[6:]) == b.Index {
				b.Stats.Kills.Add(1)
			}
		}
	case Match(p, 0xF3, 0x04) && len(p) >= 7: // respawn
		b.X, b.Y, b.Map = int(p[4]), int(p[5]), int(p[6])
		b.homeX, b.homeY = b.X, b.Y
		b.dead = false
		clear(b.monsters)
	}
}

func (b *Bot) moved(index, x, y int) {
	if index == b.Index {
		b.X, b.Y = x, y
		return
	}
	if m, ok := b.monsters[index]; ok {
		m.x, m.y = x, y
	}
}

// reach is the attack distance the server accepts (combat.go), minus one
// cell for monsters that move between the bot's view and the server's.
func (b *Bot) reach() int {
	if b.Class == 2 {
		return 5
	}
	return 2
}

// attackOrChase attacks the nearest monster in reach or walks towards one
// within Hunt cells. It returns false when no monster is around.
func (b *Bot) attackOrChase() bool {
	best, bd := -1, 1<<30
	for i, m := range b.monsters {
		dx, dy := m.x-b.X, m.y-b.Y
		if d := max(abs(dx), abs(dy)); d < bd && d <= b.Profile.Hunt {
			best, bd = i, d
		}
	}
	if best < 0 {
		return false
	}
	m := b.monsters[best]
	if bd <= b.reach() {
		b.Stats.Attacks.Add(1)
		return b.conn.Send(Attack(best, ActionAttack, DirTo(b.X, b.Y, m.x, m.y))) == nil
	}
	dirs := b.path(m.x, m.y, min(bd-1, 3))
	if len(dirs) == 0 {
		return false
	}
	b.Stats.Walks.Add(1)
	return b.conn.Send(Walk(b.X, b.Y, dirs[0], dirs...)) == nil
}

// wander walks 1-3 random steps, back towards home when too far.
func (b *Bot) wander() error {
	tx := b.X + b.Rand.IntN(5) - 2
	ty := b.Y + b.Rand.IntN(5) - 2
	if abs(b.X-b.homeX) > b.Profile.Home || abs(b.Y-b.homeY) > b.Profile.Home {
		tx, ty = b.homeX, b.homeY
	}
	dirs := b.path(tx, ty, 1+b.Rand.IntN(3))
	if len(dirs) == 0 {
		return nil
	}
	b.Stats.Walks.Add(1)
	return b.conn.Send(Walk(b.X, b.Y, dirs[0], dirs...))
}

// path returns up to n straight steps towards (tx, ty), stopping before a
// cell the terrain blocks or marks as safe zone.
func (b *Bot) path(tx, ty, n int) []byte {
	var dirs []byte
	x, y := b.X, b.Y
	for len(dirs) < n && (x != tx || y != ty) {
		d := DirTo(x, y, tx, ty)
		nx, ny := Step(x, y, d)
		if b.Terrain != nil && b.Terrain.Get(nx, ny)&(gamedata.AttrBlock|gamedata.AttrNoGround|gamedata.AttrSafeZone) != 0 {
			break
		}
		dirs = append(dirs, d)
		x, y = nx, ny
	}
	return dirs
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
