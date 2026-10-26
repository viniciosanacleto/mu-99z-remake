package game

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net"
	"testing"
	"time"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/auth"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/config/gsconfig"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/item"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/frame"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/store"
)

// bareWorld is a World without sockets or goroutines: tests drive the
// handlers directly and read what each player was sent.
type bareWorld struct {
	*World
	t     *testing.T
	clock time.Time
}

func newBareWorld(t *testing.T) *bareWorld {
	t.Helper()
	cfg, err := gsconfig.Load(repoConfig("gameserver", "gameserver.toml"))
	if err != nil {
		t.Fatal(err)
	}
	mem, err := store.NewMemory()
	if err != nil {
		t.Fatal(err)
	}
	b := &bareWorld{t: t, clock: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	w, err := New(Options{DataDir: repoData("Data"), ConfigDir: repoConfig(), Config: cfg, Store: mem, Auth: auth.New(mem, auth.Config{}),
		Log: slog.New(slog.NewTextHandler(io.Discard, nil)), Now: func() time.Time { return b.clock }})
	if err != nil {
		t.Fatal(err)
	}
	b.World = w
	return b
}

func (b *bareWorld) advance(d time.Duration) { b.clock = b.clock.Add(d) }

// player places a fake connected character at (mp, x, y).
func (b *bareWorld) player(index int, name string, mp, x, y int) *Object {
	o := b.objects[index]
	o.reset()
	o.State, o.Type = StatePlaying, TypePlayer
	nc, peer := net.Pipe()
	b.t.Cleanup(func() { nc.Close(); peer.Close() })
	o.conn = &conn{nc: nc, out: make(chan []byte, 4096), done: make(chan struct{}), index: index}
	o.P = &Player{Char: &store.Character{Name: name, Level: 400, Money: 1000000, Mana: 1000, BP: 1000}}
	o.P.Account.Login = "acc" + name
	o.P.Verified = true
	o.P.ConnectedAt, o.P.LastPacket, o.P.LastSave = b.clock, b.clock, b.clock
	for i := range o.P.Inventory {
		o.P.Inventory[i] = item.Empty()
	}
	for i := range o.P.Skills {
		o.P.Skills[i].Index = -1
	}
	o.Map, o.X, o.Y, o.TX, o.TY = mp, x, y, x, y
	o.Live, o.Alive = LiveNormal, true
	b.occupy(o, true)
	o.Move = nil
	mv(o).Authority = b.gmLevel(o)
	b.drain(o)
	return o
}

// see makes a and c see each other (VP/VP2 entries in both directions).
func (b *bareWorld) see(a, c *Object) {
	vpAdd(&a.VP, c.Index, byte(c.Type))
	vpAdd(&c.VP2, a.Index, byte(a.Type))
	vpAdd(&c.VP, a.Index, byte(a.Type))
	vpAdd(&a.VP2, c.Index, byte(c.Type))
}

// drain returns every packet queued for o.
func (b *bareWorld) drain(o *Object) [][]byte {
	var out [][]byte
	for {
		select {
		case p := <-o.conn.out:
			out = append(out, p)
		default:
			return out
		}
	}
}

// find returns the first queued packet with head (and sub >= 0).
func find(pkts [][]byte, head byte, sub int) []byte {
	for _, p := range pkts {
		if p == nil || frame.Head(p) != head {
			continue
		}
		if s, ok := frame.Sub(p); sub >= 0 && (!ok || int(s) != sub) {
			continue
		}
		return p
	}
	return nil
}

// openRow finds n+1 walkable cells in a row (x..x+n, y) on map mp.
func (b *bareWorld) openRow(mp, n int) (int, int) {
	t := b.terrain(mp)
	for y := 20; y < 230; y++ {
		for x := 20; x < 230-n; x++ {
			ok := true
			for k := 0; k <= n && ok; k++ {
				ok = t.Get(x+k, y)&(gamedata.AttrBlock|gamedata.AttrNoGround|gamedata.AttrCharacter|gamedata.AttrSafeZone|gamedata.AttrWall) == 0
			}
			if ok {
				return x, y
			}
		}
	}
	b.t.Fatal("no open row")
	return 0, 0
}

func walkPacket(x, y int, dir byte, dirs ...byte) []byte {
	body := []byte{byte(x), byte(y), dir<<4 | byte(len(dirs))}
	for i := 0; i < len(dirs); i += 2 {
		v := dirs[i] << 4
		if i+1 < len(dirs) {
			v |= dirs[i+1]
		}
		body = append(body, v)
	}
	return frame.Build(frame.C1, 0xD3, body...)
}

func TestMovePacketLayouts(t *testing.T) {
	if got := packetWalk(0x53FC, 130, 131, 3); !bytes.Equal(got, []byte{0xC1, 0x08, 0xD3, 0x53, 0xFC, 130, 131, 0x30}) {
		t.Fatalf("D3 = % X", got)
	}
	if got := packetPosition(0x53FD, 10, 20); !bytes.Equal(got, []byte{0xC1, 0x07, 0xDF, 0x53, 0xFD, 10, 20}) {
		t.Fatalf("DF = % X", got)
	}
	if got := packetAction(0x53FC, 2, 0x7A, 0, 0); !bytes.Equal(got, []byte{0xC1, 0x09, 0x18, 0x53, 0xFC, 2, 0x7A, 0, 0}) {
		t.Fatalf("18 = % X", got)
	}
}

func TestDirTable(t *testing.T) {
	want := [8][2]int{{-1, -1}, {0, -1}, {1, -1}, {1, 0}, {1, 1}, {0, 1}, {-1, 1}, {-1, 0}}
	for d := byte(0); d < 8; d++ {
		if x, y := dirStep(d); x != want[d][0] || y != want[d][1] {
			t.Fatalf("dir %d = %d,%d", d, x, y)
		}
	}
}

func TestStepDelay(t *testing.T) {
	cases := []struct {
		dir  byte
		ms   int
		slow bool
		want time.Duration
	}{
		{3, 400, false, 400 * time.Millisecond},
		{2, 400, false, 520 * time.Millisecond},
		{4, 401, false, 521 * time.Millisecond}, // 521.3 truncated (FISTP, RC=chop)
		{0, 400, true, 910 * time.Millisecond},
		{7, 400, true, 700 * time.Millisecond},
	}
	for _, c := range cases {
		if got := stepDelay(c.dir, c.ms, c.slow); got != c.want {
			t.Fatalf("stepDelay(%d,%d,%v) = %v, want %v", c.dir, c.ms, c.slow, got, c.want)
		}
	}
}

func TestWalkAcceptAndBroadcast(t *testing.T) {
	b := newBareWorld(t)
	x, y := b.openRow(0, 4)
	a := b.player(PlayerStart, "Alpha", 0, x, y)
	c := b.player(PlayerStart+1, "Beta", 0, x, y+20)
	b.see(a, c)
	b.handleWalk(a, walkPacket(x, y, 3, 3, 3))
	want := []byte{0xC1, 0x08, 0xD3, 0x53, 0xFC, byte(x + 2), byte(y), 0x30}
	if p := find(b.drain(a), 0xD3, -1); !bytes.Equal(p, want) {
		t.Fatalf("self D3 = % X", p)
	}
	if p := find(b.drain(c), 0xD3, -1); !bytes.Equal(p, want) {
		t.Fatalf("viewer D3 = % X", p)
	}
	m := a.Move
	if a.X != x || a.TX != x+2 || m.PathCount != 3 || m.PathCur != 1 || m.OldX != x+2 {
		t.Fatalf("state X=%d TX=%d count=%d cur=%d old=%d", a.X, a.TX, m.PathCount, m.PathCur, m.OldX)
	}
	tr := b.terrain(0)
	if tr.Get(x, y)&gamedata.AttrCharacter != 0 || tr.Get(x+2, y)&gamedata.AttrCharacter == 0 {
		t.Fatal("occupancy bit not moved to the target cell")
	}
	// obj+0x24c (last step) is not reset by D3: the first step is taken
	// on the next tick, the following ones strictly after the delay
	// (400 ms for direction 3).
	b.moveTick()
	if a.X != x+1 || a.Dir != 3 || m.PathCur != 2 {
		t.Fatalf("after 1st step X=%d cur=%d", a.X, m.PathCur)
	}
	b.advance(400 * time.Millisecond)
	b.moveTick()
	if a.X != x+1 {
		t.Fatal("step taken at exactly the delay")
	}
	b.advance(time.Millisecond)
	b.moveTick()
	if a.X != x+2 || m.PathCount != 0 || m.PathCur != 0 || m.PathStartEnd != 1 {
		t.Fatalf("after 2nd step X=%d count=%d start=%d", a.X, m.PathCount, m.PathStartEnd)
	}
}

func TestWalkThrottleAndPreconditions(t *testing.T) {
	b := newBareWorld(t)
	x, y := b.openRow(0, 4)
	a := b.player(PlayerStart, "Alpha", 0, x, y)
	b.handleWalk(a, walkPacket(x, y, 3, 3))
	b.drain(a)
	b.advance(99 * time.Millisecond)
	b.handleWalk(a, walkPacket(x, y, 3, 3, 3))
	if len(b.drain(a)) != 0 || a.TX != x+1 {
		t.Fatal("D3 within 100 ms must be ignored")
	}
	b.advance(time.Millisecond)
	a.P.LogoutCountdown = 3
	b.handleWalk(a, walkPacket(x, y, 3, 3, 3))
	if a.TX != x+1 {
		t.Fatal("D3 during logout countdown")
	}
	a.P.LogoutCountdown = 0
	a.Move.Regen = 1
	b.handleWalk(a, walkPacket(x, y, 3, 3, 3))
	if a.TX != x+1 {
		t.Fatal("D3 while obj+0x287 > 0")
	}
}

func TestWalkRejectResendsPosition(t *testing.T) {
	b := newBareWorld(t)
	x, y := b.openRow(0, 4)
	a := b.player(PlayerStart, "Alpha", 0, x, y)
	// 16 cells away: outside +-15.
	dirs := bytes.Repeat([]byte{3}, 15)
	b.handleWalk(a, walkPacket(x+1, y, 3, dirs...))
	pk := b.drain(a)
	if find(pk, 0xD3, -1) != nil {
		t.Fatal("rejected walk broadcast")
	}
	if p := find(pk, 0xDF, -1); !bytes.Equal(p, []byte{0xC1, 0x07, 0xDF, 0x53, 0xFC, byte(x), byte(y)}) {
		t.Fatalf("resend = % X", p)
	}
	if a.Move.PathCount != 0 {
		t.Fatal("path not cleared")
	}
	// Target cell blocked.
	b.advance(time.Second)
	tr := b.terrain(0)
	tr.Set(x+1, y, tr.Get(x+1, y)|gamedata.AttrBlock)
	b.handleWalk(a, walkPacket(x, y, 3, 3))
	if find(b.drain(a), 0xDF, -1) == nil || a.TX != x {
		t.Fatalf("blocked target accepted (TX=%d)", a.TX)
	}
}

func TestMoveTickBlockedCellStops(t *testing.T) {
	b := newBareWorld(t)
	x, y := b.openRow(0, 4)
	a := b.player(PlayerStart, "Alpha", 0, x, y)
	b.handleWalk(a, walkPacket(x, y, 3, 3, 3, 3))
	b.drain(a)
	tr := b.terrain(0)
	tr.Set(x+1, y, tr.Get(x+1, y)|gamedata.AttrNoGround)
	b.advance(500 * time.Millisecond)
	b.moveTick()
	if a.X != x || a.Move.PathCount != 0 {
		t.Fatalf("walked into a blocked cell: X=%d", a.X)
	}
	if p := find(b.drain(a), 0xDF, -1); p == nil || p[5] != byte(x) {
		t.Fatalf("no DF after blocked step: % X", p)
	}
}

func TestSetPathHookMovesMonsters(t *testing.T) {
	b := newBareWorld(t)
	x, y := b.openRow(0, 4)
	viewer := b.player(PlayerStart, "Alpha", 0, x, y+3)
	mon := b.objects[100]
	mon.reset()
	mon.State, mon.Type, mon.Class, mon.Map, mon.X, mon.Y = StatePlaying, TypeMonster, 3, 0, x, y
	mon.Live, mon.Alive = LiveNormal, true
	b.setMoveSpeed(mon, 400)
	vpAdd(&mon.VP2, viewer.Index, byte(TypePlayer))
	setPathHook(b.World, mon, x, y, []byte{3, 3})
	if p := find(b.drain(viewer), 0xD3, -1); p == nil || p[3] != 0 || p[4] != 100 || p[5] != byte(x+2) {
		t.Fatalf("monster D3 = % X", p)
	}
	b.advance(401 * time.Millisecond)
	b.moveTick()
	if mon.X != x+1 {
		t.Fatal("monster did not step")
	}
	// Excluded classes never walk (FUN_004c4480).
	mon.Class = 0x83
	b.advance(time.Second)
	b.moveTick()
	if mon.X != x+1 {
		t.Fatal("castle statue walked")
	}
}

func TestPositionFix(t *testing.T) {
	b := newBareWorld(t)
	x, y := b.openRow(0, 4)
	a := b.player(PlayerStart, "Alpha", 0, x, y)
	c := b.player(PlayerStart+1, "Beta", 0, x, y+5)
	b.see(a, c)
	b.handlePosition(a, frame.Build(frame.C1, 0xDF, byte(x+3), byte(y)))
	want := []byte{0xC1, 0x07, 0xDF, 0x53, 0xFC, byte(x + 3), byte(y)}
	if p := find(b.drain(a), 0xDF, -1); !bytes.Equal(p, want) {
		t.Fatalf("self DF = % X", p)
	}
	if p := find(b.drain(c), 0xDF, -1); !bytes.Equal(p, want) {
		t.Fatalf("viewer DF = % X", p)
	}
	if a.X != x+3 || a.TX != x+3 || a.Move.OldX != x+3 {
		t.Fatal("position not applied")
	}
	b.handlePosition(a, frame.Build(frame.C1, 0xDF, byte(x+3+16), byte(y)))
	if len(b.drain(a)) != 0 {
		t.Fatal("DF beyond 15 cells accepted")
	}
	// Chaos Castle: 1 s after a knock-back.
	a.Map = 18
	a.Move.CCPushTick = b.clock
	b.advance(999 * time.Millisecond)
	b.handlePosition(a, frame.Build(frame.C1, 0xDF, byte(x+3), byte(y)))
	if len(b.drain(a)) != 0 {
		t.Fatal("DF inside the Chaos Castle cooldown")
	}
}

func TestActionBroadcast(t *testing.T) {
	b := newBareWorld(t)
	x, y := b.openRow(0, 4)
	a := b.player(PlayerStart, "Alpha", 0, x, y)
	c := b.player(PlayerStart+1, "Beta", 0, x, y+5)
	b.see(a, c)
	b.handleAction(a, frame.Build(frame.C1, 0x18, 5, 0x80, 0x12, 0x34))
	if len(b.drain(a)) != 0 {
		t.Fatal("action echoed to the sender")
	}
	if p := find(b.drain(c), 0x18, -1); !bytes.Equal(p, []byte{0xC1, 0x09, 0x18, 0x53, 0xFC, 5, 0x80, 0x12, 0x34}) {
		t.Fatalf("viewer 18 = % X", p)
	}
	if a.ActionState != 2 || a.PoseState != 0x80 || a.Move.Flag270 != 0x80 || a.Dir != 5 {
		t.Fatalf("action state %d pose %X", a.ActionState, a.PoseState)
	}
}

func TestCastleGateBlocksWalk(t *testing.T) {
	b := newBareWorld(t)
	x, y := b.openRow(30, 4)
	a := b.player(PlayerStart, "Alpha", 30, x, y)
	tr := b.terrain(30)
	tr.Set(x+2, y, tr.Get(x+2, y)|gamedata.AttrWall)
	b.handleWalk(a, walkPacket(x, y, 3, 3, 3, 3))
	pk := b.drain(a)
	if find(pk, 0xD3, -1) != nil {
		t.Fatal("walk through a castle gate accepted")
	}
	if p := find(pk, 0x18, -1); !bytes.Equal(p, []byte{0xC1, 0x09, 0x18, 0x53, 0xFC, 3, 0x7A, 0, 0}) {
		t.Fatalf("gate action = % X", p)
	}
	if a.Move.Flag270 != 0 {
		t.Fatal("obj+0x270 must end at 0 (FUN_00511cb0 clears it)")
	}
}

func TestImmobilizedByEffect(t *testing.T) {
	b := newBareWorld(t)
	x, y := b.openRow(0, 4)
	a := b.player(PlayerStart, "Alpha", 0, x, y)
	old := moveEffectHook
	defer func() { moveEffectHook = old }()
	moveEffectHook = func(_ *World, _ *Object, e int) bool { return e == 0x3D }
	b.handleWalk(a, walkPacket(x, y, 3, 3))
	if len(b.drain(a)) != 0 {
		t.Fatal("stunned player walked")
	}
}

// --- end-to-end ---

// enterAt logs a character in through the real protocol after placing it
// at (mp, x, y) with the given level.
func enterAt(t *testing.T, env *testEnv, account, char string, class byte, mp, x, y, level int) *testClient {
	t.Helper()
	ctx := context.Background()
	acc, err := env.store.AccountByLogin(ctx, account)
	if err != nil {
		h, _ := auth.HashPassword("pass")
		if acc, err = env.store.CreateAccount(ctx, account, h, ""); err != nil {
			t.Fatal(err)
		}
	}
	if r, _, err := env.store.CreateCharacter(ctx, acc.ID, char, class); err != nil || r != store.CreateOK {
		t.Fatalf("create %s: %v %v", char, r, err)
	}
	ch, _ := env.store.LoadCharacter(ctx, acc.ID, char)
	ch.Map, ch.X, ch.Y, ch.Level = mp, x, y, level
	ch.Money = 100000
	if err := env.store.SaveCharacter(ctx, ch); err != nil {
		t.Fatal(err)
	}
	c := env.dial(t)
	c.next(0xF1, 0x00)
	c.verify()
	if r := c.login(account, "pass"); r != 1 {
		t.Fatalf("login %s = %d", account, r)
	}
	n := make([]byte, 10)
	copy(n, char)
	c.send(frame.Build(frame.C1, 0xF3, append([]byte{0x03}, n...)...))
	c.next(0xF3, 0x03)
	return c
}

// waitSees waits for the C2 12 entry of index on c.
func waitSees(t *testing.T, c *testClient, index int) {
	t.Helper()
	for {
		p := c.next(0x12, -1)
		for i := 0; i < int(p[4]); i++ {
			e := p[5+44*i:]
			if int(e[0]&0x7F)<<8|int(e[1]) == index {
				return
			}
		}
	}
}

func TestEndToEndPlayersSeeEachOtherWalk(t *testing.T) {
	env := newTestEnv(t)
	a := enterAt(t, env, "tester", "Alpha", 0x00, 0, 135, 125, 20)
	c := enterAt(t, env, "second", "Beta", 0x00, 0, 137, 126, 20)
	waitSees(t, a, PlayerStart+1)
	waitSees(t, c, PlayerStart)

	a.send(walkPacket(135, 125, 3, 3))
	p := c.next(0xD3, -1)
	if int(p[3])<<8|int(p[4]) != PlayerStart || p[5] != 136 || p[6] != 125 || p[7] != 0x30 {
		t.Fatalf("Beta got D3 % X", p)
	}
	if p := a.next(0xD3, -1); p[5] != 136 {
		t.Fatalf("Alpha got D3 % X", p)
	}

	a.send(frame.Build(frame.C1, 0xDF, 136, 125))
	if p := c.next(0xDF, -1); int(p[3])<<8|int(p[4]) != PlayerStart || p[5] != 136 || p[6] != 125 {
		t.Fatalf("Beta got DF % X", p)
	}
}
