package game

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"path/filepath"
	"sync"
	"time"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/auth"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/config/gsconfig"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/config/jsonconf"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/config/mutext"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/charset"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/crypto"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/gscodec"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/store"
)

// Reporter receives the user counters the original sent to the
// ConnectServer by UDP (0xA1).
type Reporter interface {
	Report(code, users, maxUsers int)
	MarkDown(code int)
}

// Options configure an instance.
type Options struct {
	DataDir  string // config/data: the JSON tables of MuServer/Data
	AssetDir string // MuServer/Data files kept in their original format (terrain, keys, scripts)
	Config   *gsconfig.Config
	Store    store.Store
	Auth     *auth.Service
	Reporter Reporter
	Log      *slog.Logger
	Now      func() time.Time
	// PublicIP, when set, replaces the MapServerInfo.txt address sent in
	// server move packets.
	PublicIP string
	// Hub links the instances of the process (nil = private hub).
	Hub *Hub
}

// Data is the static game data shared by the instance.
type Data struct {
	Maps       map[int]gamedata.MapInfo
	Gates      map[int]gamedata.Gate
	MapServer  *gamedata.MapServer
	Skills     map[int]gamedata.SkillInfo
	Items      map[int]gamedata.ItemInfo
	Experience *gamedata.Experience
	Custom     charset.Custom
}

// World is one game server instance.
type World struct {
	cfg      *gsconfig.Config
	data     *Data
	store    store.Store
	auth     *auth.Service
	reporter Reporter
	log      *slog.Logger
	now      func() time.Time
	keys     gscodec.Keys

	objects  [MaxObjects]*Object
	terrains [gamedata.MaxMaps]*gamedata.Terrain
	users    int
	nextUser int
	closing  bool

	notices        [3]noticeList
	publicIP       string
	hub            *Hub
	dataDir        string    // Options.DataDir (modules load their tables from it)
	assetDir       string    // Options.AssetDir
	moveData       *moveData // move.go / chat.go / teleport.go tables
	hackRules      *hackRules
	messages       messageTable
	connectMembers []string
	badWords       []string

	mon *monsterWorld // monster.go: monster data, index allocation, spawns

	itemData *itemData // inventory.go: item tables, shops, map items

	statsData *statsData       // stats.go: class info, item options, experience tables
	quests    *gamedata.Quests // quest.go: Quest/QuestObjective/QuestReward.txt
	eventData *eventData       // event.go: event managers
	parties   *partyManager    // party.go: CParty
	guilds    *guildWorld      // guild_war.go: per-instance guild war state
	skills    *skillWorld      // skill.go: EffectList, SkillDamage, ComboSkill, SkillEnabler, SkillUseArea
	castle    *castleSiege     // castlesiege.go: CCastleSiege (CastleSiege server only)
	cmd       *cmdWorld        // command_player2.go: ResetTable/ResetLimit, CustomQuiz
	custom    *customWorld     // command_custom.go: CustomAttack/Gift/Marriage/BuyVip/Pick, /superdrop
	cevt      *customEvents    // customevent.go: WizardTeam custom events (Data/Custom/Event)
	cash      *cashWorld       // xshop.go: CustomXShop, CustomJewel, LuckyWheel (block 12)
	rq        *roundQWorld     // round_q.go: raven table, push-back, MoveSummon, battle soccer
	rcn       *reconnectWorld  // reconnect.go: CReconnect cache (ReconnectTime, Offline Reconnect)

	expTable gamedata.ExperienceTable // datafiles.go: Util/ExperienceTable.txt
	du       *dataUses                // datauses.go: freebies, CommandRequirement, MonsterMove/Schedule, BlackList...
	lua      *luaScripts              // lua.go: ScriptMain.lua / CalcCharacter.lua states and the bridge

	chaos      *chaosData                   // chaos.go: EventItemBag, ItemOptionRate, ExcellentOptionRate, SetItemType
	petExp     [petMaxTableLevel + 1]uint32 // pet.go: DAT_00770de8
	petExpOnce sync.Once
	csLocal    *castleSync // castlesiege_tax.go: castle sync of a World without hub

	events chan func()
	ctx    context.Context
	wg     sync.WaitGroup
}

// New loads configuration-dependent data and builds an instance.
func New(o Options) (*World, error) {
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.Log == nil {
		o.Log = slog.Default()
	}
	cfg := o.Config
	stream, err := crypto.DeriveStreamKeys(cfg.CustomerName, cfg.ServerSerial)
	if err != nil {
		return nil, err
	}
	enc, err := crypto.LoadKeyFile(filepath.Join(o.AssetDir, "Hack", "Enc2.dat"))
	if err != nil {
		return nil, err
	}
	dec, err := crypto.LoadKeyFile(filepath.Join(o.AssetDir, "Hack", "Dec1.dat"))
	if err != nil {
		return nil, err
	}
	data, err := loadData(o.DataDir, cfg)
	if err != nil {
		return nil, err
	}
	w := &World{cfg: cfg, data: data, store: o.Store, auth: o.Auth, reporter: o.Reporter,
		log: o.Log.With("server", cfg.ServerName, "code", cfg.ServerCode), now: o.Now,
		keys:   gscodec.Keys{Stream: stream, Encrypt: enc, Decrypt: dec},
		events: make(chan func(), 4096), nextUser: PlayerStart, publicIP: o.PublicIP,
		dataDir: o.DataDir, assetDir: o.AssetDir}
	if w.terrains, err = gamedata.LoadTerrains(o.AssetDir); err != nil {
		return nil, err
	}
	if w.hackRules, err = loadHackRules(o.DataDir); err != nil {
		return nil, err
	}
	if w.messages, err = loadMessages(o.DataDir); err != nil {
		return nil, err
	}
	if w.connectMembers, err = loadNameList(filepath.Join(o.DataDir, "ConnectMember.json")); err != nil {
		return nil, err
	}
	if w.badWords, err = loadBadSyntax(filepath.Join(o.DataDir, "..", "server.json")); err != nil {
		return nil, err
	}
	for i := range w.objects {
		w.objects[i] = newObject(i)
	}
	w.hub = o.Hub
	if w.hub == nil {
		w.hub = NewHub()
	}
	w.hub.register(w)
	for _, h := range startHooks {
		if err := h(w); err != nil {
			return nil, err
		}
	}
	return w, nil
}

func loadData(dir string, cfg *gsconfig.Config) (*Data, error) {
	d := &Data{Custom: charset.NoCustom{}}
	var err error
	if d.Maps, err = gamedata.LoadMapManager(dir); err != nil {
		return nil, err
	}
	if d.Gates, err = gamedata.LoadGates(dir); err != nil {
		return nil, err
	}
	if d.MapServer, err = gamedata.LoadMapServer(dir); err != nil {
		return nil, err
	}
	if d.Skills, err = gamedata.LoadSkills(dir); err != nil {
		return nil, err
	}
	if d.Items, err = gamedata.LoadItems(dir); err != nil {
		return nil, err
	}
	if d.Custom, err = loadCustomTables(dir); err != nil {
		return nil, err
	}
	d.Experience = gamedata.NewExperience(int32(cfg.Int("Common", "ExperienceMultiplierConstA", 0)),
		int32(cfg.Int("Common", "ExperienceMultiplierConstB", 0)),
		int32(cfg.Int("Common", "PetExperienceMultiplierConstA", 0)))
	return d, nil
}

// Serve accepts clients on ln and runs the world until ctx is done.
func (w *World) Serve(ctx context.Context, ln net.Listener) error {
	w.ctx = ctx
	go func() {
		<-ctx.Done()
		ln.Close()
	}()
	if err := w.store.ResetSessions(ctx); err != nil {
		return err
	}
	loopDone := make(chan struct{})
	go func() {
		defer close(loopDone)
		w.loop(ctx)
	}()
	var err error
	for {
		nc, aerr := ln.Accept()
		if aerr != nil {
			if ctx.Err() == nil {
				err = aerr
			}
			break
		}
		c := newConn(nc, w.keys)
		w.post(func() { w.accept(c) })
	}
	<-loopDone
	w.wg.Wait()
	if w.reporter != nil {
		w.reporter.MarkDown(w.cfg.ServerCode)
	}
	return err
}

// post schedules f on the world goroutine.
func (w *World) post(f func()) {
	select {
	case w.events <- f:
	case <-w.ctxDone():
	}
}

func (w *World) ctxDone() <-chan struct{} {
	if w.ctx == nil {
		return nil
	}
	return w.ctx.Done()
}

// async runs work off the world goroutine (database calls, as the original
// delegated them to the DataServer/JoinServer) and posts its continuation.
func (w *World) async(work func(ctx context.Context) func()) {
	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		ctx := w.ctx
		if ctx == nil {
			ctx = context.Background()
		}
		if cont := work(ctx); cont != nil {
			w.post(cont)
		}
	}()
}

func (w *World) loop(ctx context.Context) {
	fast := time.NewTicker(100 * time.Millisecond)
	slow := time.NewTicker(time.Second)
	defer fast.Stop()
	defer slow.Stop()
	w.report()
	for {
		select {
		case <-ctx.Done():
			w.shutdown()
			return
		case f := <-w.events:
			f()
		case <-fast.C:
			w.tick100()
		case <-slow.C:
			w.tick1000()
		}
	}
}

func (w *World) shutdown() {
	w.closing = true
	for i := PlayerStart; i < MaxObjects; i++ {
		if o := w.objects[i]; o.conn != nil {
			w.closeClient(o, "server shutdown")
		}
	}
}

func (w *World) report() {
	if w.reporter != nil {
		w.reporter.Report(w.cfg.ServerCode, w.users, w.cfg.MaxUsers)
	}
}

// accept reproduces FUN_0050c260 (index allocation) and FUN_0050c3c0
// (AddClient), then sends F1 00 (FUN_004db050).
func (w *World) accept(c *conn) {
	if acceptFilterHook != nil && !acceptFilterHook(w, c.ip) { // Data/BlackList.txt (blacklist.go)
		c.close()
		return
	}
	if w.closing {
		c.sendAndClose(packetLoginResult(2))
		go c.writer()
		return
	}
	limit := min(w.cfg.MaxUsers, 1000)
	if w.users >= limit {
		c.sendAndClose(packetLoginResult(4))
		go c.writer()
		return
	}
	idx := -1
	for n := 0; n < MaxObjects-PlayerStart; n++ {
		i := w.nextUser
		w.nextUser++
		if w.nextUser >= MaxObjects {
			w.nextUser = PlayerStart
		}
		if w.objects[i].State == StateEmpty {
			idx = i
			break
		}
	}
	if idx < 0 {
		c.close()
		return
	}
	o := w.objects[idx]
	o.reset()
	now := w.now()
	o.State, o.Type, o.conn = StateConnected, TypePlayer, c
	o.P = &Player{IP: c.ip, ConnectedAt: now, LastPacket: now, CharListLoaded: true, recvSerial: 0xFF}
	c.index = idx
	w.users++
	w.report()
	w.log.Debug("AddClient", "index", idx, "ip", c.ip)

	go c.writer()
	go c.reader(
		func(p gscodec.Packet) { w.post(func() { w.handlePacket(c, p) }) },
		func() { w.post(func() { w.disconnected(c) }) },
	)
	c.send(packetJoinResult(idx, w.cfg.ServerVersion, w.cfg.ServerCode))
}

// disconnected runs when the socket is gone (FUN_00505150 / gObjDel).
func (w *World) disconnected(c *conn) {
	if c.index < 0 {
		return
	}
	o := w.objects[c.index]
	if o.conn != c {
		return
	}
	w.removePlayer(o)
}

// closeClient closes the socket; the reader's onClose cleans the object.
func (w *World) closeClient(o *Object, reason string) {
	if o.conn == nil {
		return
	}
	w.log.Debug("CloseClient", "index", o.Index, "reason", reason)
	o.conn.close()
}

// removePlayer frees the slot, saving the character and closing the
// account session.
func (w *World) removePlayer(o *Object) {
	p := o.P
	if o.State == StatePlaying {
		w.leaveWorld(o)
		w.saveCharacter(o, nil)
	}
	if p != nil && p.Session != nil {
		account := p.Account.Login
		w.async(func(ctx context.Context) func() {
			w.auth.LogoutUnlessMoving(ctx, account)
			return nil
		})
	}
	o.reset()
	w.users--
	w.report()
}

// send queues a packet for one object.
func (w *World) send(o *Object, p []byte) {
	if o.conn != nil {
		o.conn.send(p)
	}
}

// sendIndex sends to an object index (FUN_00512570).
func (w *World) sendIndex(index int, p []byte) {
	if index >= 0 && index < MaxObjects {
		w.send(w.objects[index], p)
	}
}

// broadcast sends to every player that sees o (FUN_00512730: VP2 entries
// of type 1).
func (w *World) broadcast(o *Object, p []byte) {
	for _, e := range o.VP2 {
		if e.State != VPEmpty && e.Type == byte(TypePlayer) {
			w.sendIndex(e.Index, p)
		}
	}
}

// KickAccount implements auth.Kicker (JoinServer 0x30, FUN_004a2490).
func (w *World) KickAccount(account string) {
	w.post(func() {
		for i := PlayerStart; i < MaxObjects; i++ {
			o := w.objects[i]
			if o.P != nil && o.P.Session != nil && o.P.Account.Login == account {
				if w.offlineKick(o) { // offline modes (offline.go)
					continue
				}
				w.closeClient(o, "already connected")
			}
		}
	})
}

var errNoSession = errors.New("game: no session")

// mustPlayer returns the object bound to c, or nil if the slot was reused.
func (w *World) objectOf(c *conn) *Object {
	if c.index < 0 || c.index >= MaxObjects {
		return nil
	}
	o := w.objects[c.index]
	if o.conn != c {
		return nil
	}
	return o
}

func (w *World) String() string { return fmt.Sprintf("World(%s)", w.cfg.ServerName) }

// loadNameList reads a one-column table (ConnectMember.txt).
func loadNameList(path string) ([]string, error) {
	lines, err := jsonconf.LoadLines(path)
	if err != nil {
		return nil, err
	}
	return firstColumn(lines), nil
}

// loadBadSyntax reads the DataServer BadSyntax.txt list of server.json.
func loadBadSyntax(path string) ([]string, error) {
	f, err := jsonconf.LoadServer(path)
	if err != nil {
		return nil, err
	}
	lines, err := f.BadSyntax.Lines()
	if err != nil {
		return nil, fmt.Errorf("%s: BadSyntax: %w", path, err)
	}
	return firstColumn(lines), nil
}

func firstColumn(lines []mutext.Line) []string {
	var out []string
	for _, l := range mutext.Table(lines) {
		out = append(out, l.Tokens[0])
	}
	return out
}
