package game

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/devilsquare"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/item"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/store"
)

// gmPlayer is a player whose character has GM level lv (characters.gm_level).
func gmPlayer(b *bareWorld, index int, name string, lv, mp, x, y int) *Object {
	o := b.player(index, name, mp, x, y)
	o.P.Char.GMLevel = lv
	mv(o).Authority = lv
	return o
}

func TestGMCommandNeedsLevel(t *testing.T) {
	b := newBareWorld(t)
	o := b.player(PlayerStart, "Plain", 0, 130, 130)
	b.runCommand(o, "/setzen Plain 5")
	if o.P.Char.Money != 1000000 {
		t.Fatal("non-GM used /setzen")
	}
	hasText(t, b.drain(o), b.messageFor(o, 0x12D, "/setzen"))
	// /make needs level 32.
	g := gmPlayer(b, PlayerStart+1, "Gm8", 8, 0, 131, 130)
	b.runCommand(g, "/make 0 0 0 0 0 0 0 0 0 0 0 0")
	hasText(t, b.drain(g), b.messageFor(g, 0x12D, "/make"))
}

func TestGMMoveTraceTrack(t *testing.T) {
	b := newBareWorld(t)
	g := gmPlayer(b, PlayerStart, "Gm", 32, 0, 130, 130)
	p := b.player(PlayerStart+1, "Target", 0, 140, 140)
	b.runCommand(g, "/gmove Nobody 0 10 10")
	hasText(t, b.drain(g), b.messageFor(g, msgGMTargetNotFound))
	b.runCommand(g, "/gmove Target 0 135 135")
	if p.X != 135 || p.Y != 135 {
		t.Fatalf("gmove %d,%d", p.X, p.Y)
	}
	b.runCommand(g, "/trace Target")
	if g.X != 135 || g.Y != 135 {
		t.Fatalf("trace %d,%d", g.X, g.Y)
	}
	b.runCommand(g, "/gmove Gm 0 120 120")
	b.runCommand(g, "/track Target")
	if p.X != 120 || p.Y != 120 {
		t.Fatalf("track %d,%d", p.X, p.Y)
	}
	b.runCommand(g, "/dc Target")
	if p.P.LogoutCountdown != 6 {
		t.Fatal("dc countdown")
	}
}

func TestGMFirework(t *testing.T) {
	b := newBareWorld(t)
	g := gmPlayer(b, PlayerStart, "Gm", 32, 0, 130, 130)
	v := b.player(PlayerStart+1, "Viewer", 0, 131, 130)
	b.see(g, v)
	fixRand(t, 0, 4) // y offset (0%5-2)*2 = -4, x offset (4%5-2)*2 = +4
	b.runCommand(g, "/firework")
	var n int
	for _, p := range b.drain(g) {
		if bytes.Equal(p, packetFirework(0, 134, 126)) {
			n++
		}
	}
	if n != 10 {
		t.Fatalf("%d fireworks", n)
	}
	if find(b.drain(v), 0xF3, 0x40) == nil {
		t.Fatal("viewport got no firework")
	}
}

func TestGMMakeDropMakeSet(t *testing.T) {
	b := newBareWorld(t)
	g := gmPlayer(b, PlayerStart, "Gm", 32, 0, 130, 130)
	// Unknown index: nothing, no message.
	b.runCommand(g, "/make 15 500 0 0 0 0 0 0 0 0 0 0")
	if len(b.drain(g)) != 0 {
		t.Fatal("unknown item answered")
	}
	b.runCommand(g, "/make 0 1 7 1 1 2 0 0 0 0 0 0")
	b.runAsync()
	pk := b.drain(g)
	hasText(t, pk, b.messageFor(g, msgGMItemCreated))
	got := 0
	for _, it := range g.P.Inventory[12:] {
		if it.Index == 1 && it.Level == 7 && it.Luck && it.Option == 2 {
			got++
		}
	}
	if got != 1 {
		t.Fatalf("made %d", got)
	}
	// A negative first token is a count.
	b.runCommand(g, "/make -2 0 1 0 0 0 0 0 0 0 0 0 0")
	b.runAsync()
	got = 0
	for _, it := range g.P.Inventory[12:] {
		if it.Index == 1 {
			got++
		}
	}
	if got != 3 {
		t.Fatalf("count make %d", got)
	}
	b.clearMapItems(0)
	b.runCommand(g, "/drop 0 1 3 0 0 0 0 0 0 0")
	b.runAsync()
	if items := b.mapItemsAt(0); len(items) != 1 || items[0].Item.Level != 3 {
		t.Fatalf("drop %+v", items)
	}
	b.runCommand(g, "/drop -3 0 1 0 0 0 0 0 0 0 0")
	b.runAsync()
	if items := b.mapItemsAt(0); len(items) != 4 {
		t.Fatalf("count drop %d", len(items))
	}
	for i := range g.P.Inventory {
		g.P.Inventory[i] = item.Empty()
	}
	b.runCommand(g, "/makeset 0 5 0 0 0 0 0 0 0 0")
	b.runAsync()
	secs := map[int]bool{}
	for _, it := range g.P.Inventory[12:] {
		if !it.IsEmpty() {
			secs[it.Index/512] = true
			if it.Level != 5 || it.Skill {
				t.Fatalf("set piece %+v", it)
			}
		}
	}
	for s := 7; s <= 11; s++ {
		if !secs[s] {
			t.Fatalf("set has no section %d: %v", s, secs)
		}
	}
}

func TestGMSetMoneySkinHide(t *testing.T) {
	b := newBareWorld(t)
	g := gmPlayer(b, PlayerStart, "Gm", 8, 0, 130, 130) // Authority 8: /hide hides
	p := b.player(PlayerStart+1, "Rich", 0, 131, 130)
	b.see(g, p)
	b.runCommand(g, "/setzen Rich 1999999999")
	if p.P.Char.Money != gmMaxMoney {
		t.Fatalf("money %d", p.P.Char.Money)
	}
	if !bytes.Equal(find(b.drain(p), 0x22, 0xFE), packetMoney(gmMaxMoney)) {
		t.Fatal("no zen packet")
	}
	b.runCommand(g, "/setzen Rich 0")
	if p.P.Char.Money != 0 {
		t.Fatal("zen not cleared")
	}
	b.runCommand(g, "/skin Rich 25")
	if gmOf(p).Skin != 25 {
		t.Fatal("skin")
	}
	b.drain(p)
	b.runCommand(g, "/hide")
	if !hasEffectIndex(g, effGMHide) || !gmHidden(g) {
		t.Fatal("not hidden")
	}
	if !bytes.Equal(find(b.drain(p), 0x14, -1), packetDestroy([]int{g.Index})) {
		t.Fatal("viewer got no destroy")
	}
	// A hidden GM is left out of the C2 12 list.
	p.VP[0] = VPEntry{Index: g.Index, Type: byte(TypePlayer), State: VPNew}
	b.sendNewPlayers(p)
	if find(b.drain(p), 0x12, -1) != nil {
		t.Fatal("hidden GM shown")
	}
	b.runCommand(g, "/hide")
	if hasEffectIndex(g, effGMHide) {
		t.Fatal("still hidden")
	}
	if find(b.drain(p), 0x12, -1) == nil {
		t.Fatal("GM not shown again")
	}
}

func TestGMEdit(t *testing.T) {
	b := newBareWorld(t)
	g := gmPlayer(b, PlayerStart, "Gm", 32, 0, 130, 130)
	p := b.player(PlayerStart+1, "Edited", 0, 131, 130)
	p.Class, p.P.Char.Class = 1, 0x10
	ch := p.P.Char
	ch.Level, ch.LevelUpPoints, ch.Strength, ch.Dexterity, ch.Vitality, ch.Energy = 10, 5, 100, 100, 100, 100
	b.runCommand(g, "/edit Edited 5 10 -1 20 0 0 7")
	base := b.statsData.classes[1]
	if ch.Level != 15 || ch.LevelUpPoints != 15 || ch.Strength != base.Strength || ch.Dexterity != 120 {
		t.Fatalf("edit %+v", ch)
	}
	if ch.Leadership != 0 {
		t.Fatal("leadership edited on a Dark Knight")
	}
	if ch.Experience < int64(b.data.Experience.Floor(15)) {
		t.Fatal("experience below the level floor")
	}
	hasText(t, b.drain(g), b.messageFor(g, msgGMEdited))
	pk := b.drain(p)
	hasText(t, pk, b.messageFor(p, msgGMEditedBy, "Gm"))
	if find(pk, 0xF3, 0xE0) == nil {
		t.Fatal("no F3 E0")
	}
	maxStat := b.cfg.ALInt("Common", "MaxStatPoint", 0, 0)
	b.runCommand(g, "/edit Edited -1 0 99999999 0 0 0 0")
	if ch.Level != 1 || ch.Strength != maxStat {
		t.Fatalf("reset level %d str %d (max %d)", ch.Level, ch.Strength, maxStat)
	}
}

func TestGMSetPK(t *testing.T) {
	b := newBareWorld(t)
	g := gmPlayer(b, PlayerStart, "Gm", 32, 0, 130, 130)
	p := b.player(PlayerStart+1, "Killer", 0, 131, 130)
	b.runCommand(g, "/setpk Killer 9")
	if p.PKLevel != 6 || p.P.Char.PKCount != 3 || p.P.Char.PKTime != b.cfg.Int("Common", "PKDownRequirePoint3", 0) {
		t.Fatalf("pk %d %d %d", p.PKLevel, p.P.Char.PKCount, p.P.Char.PKTime)
	}
	if !bytes.Equal(find(b.drain(p), 0xF3, 0x08), packetPKLevel(p.Index, 6)) {
		t.Fatal("no F3 08")
	}
	hasText(t, b.drain(g), b.messageFor(g, msgGMPKSet))
	b.runCommand(g, "/setpk Killer 3")
	if p.PKLevel != 3 || p.P.Char.PKCount != 0 || p.P.Char.PKTime != 0 {
		t.Fatal("commoner")
	}
}

func TestGMSetVip(t *testing.T) {
	b := newBareWorld(t)
	g := gmPlayer(b, PlayerStart, "Gm", 32, 0, 130, 130)
	p := b.player(PlayerStart+1, "Vip", 0, 131, 130)
	acc, err := b.store.CreateAccount(context.Background(), p.P.Account.Login, "plain:x", "")
	if err != nil {
		t.Fatal(err)
	}
	p.P.Account = *acc
	b.runCommand(g, "/setvip Vip 2 3")
	hasText(t, b.drain(g), b.messageFor(g, msgGMVipSet))
	b.runAsync()
	if p.P.Account.Level != 2 {
		t.Fatalf("level %d", p.P.Account.Level)
	}
	pk := b.drain(p)
	hasText(t, pk, b.messageFor(p, msgGMVipChanged))
	hasText(t, pk, b.messageFor(p, msgAccountTypeBase+2, expireText(p.P.Account.LevelExpiresAt)))
	a, _ := b.store.AccountByLogin(context.Background(), p.P.Account.Login)
	if a.Level != 2 || a.LevelExpiresAt.IsZero() {
		t.Fatalf("stored %+v", a)
	}
}

func TestGMMoveAllAndMob(t *testing.T) {
	b := newBareWorld(t)
	g := gmPlayer(b, PlayerStart, "Gm", 32, 0, 130, 130)
	p := b.player(PlayerStart+1, "Moved", 0, 140, 140)
	b.runCommand(g, "/moveall 0 125 125")
	if p.X != 125 || g.X != 125 {
		t.Fatalf("moveall %d %d", p.X, g.X)
	}
	x, y := b.safeCell(0)
	b.gmWarp(g, 0, x, y)
	b.runCommand(g, "/mob 3 2")
	hasText(t, b.drain(g), b.messageFor(g, msgGMMobSafeZone))
	// Lorencia field (outside the town).
	b.gmWarp(g, 0, 180, 180)
	before := countMonsters(b, 3)
	b.runCommand(g, "/mob 3 2")
	if n := countMonsters(b, 3) - before; n != 2 {
		t.Fatalf("%d monsters", n)
	}
	b.runCommand(g, "/mob 99999 1")
	hasText(t, b.drain(g), b.messageFor(g, msgGMMobFailed))
}

func countMonsters(b *bareWorld, class int) int {
	n := 0
	for i := 0; i < SummonStart; i++ {
		if o := b.objects[i]; o.State == StatePlaying && o.Type == TypeMonster && o.Class == class {
			n++
		}
	}
	return n
}

func TestGMBans(t *testing.T) {
	b := newBareWorld(t)
	ctx := context.Background()
	g := gmPlayer(b, PlayerStart, "Gm", 32, 0, 130, 130)
	p := b.player(PlayerStart+1, "Badman", 0, 131, 130)
	acc, _ := b.store.CreateAccount(ctx, p.P.Account.Login, "plain:x", "")
	if r, _, err := b.store.CreateCharacter(ctx, acc.ID, "Badman", 0x10); err != nil || r != store.CreateOK {
		t.Fatal(err)
	}
	b.runCommand(g, "/banchar Badman")
	b.runAsync()
	if p.P.LogoutCountdown != 6 || p.P.Char.CtlCode&1 == 0 {
		t.Fatal("character not closed / flagged")
	}
	hasText(t, b.drain(p), b.messageFor(p, msgGMBanned))
	hasText(t, b.drain(g), b.messageFor(g, msgGMBlocked, "Badman"))
	if c, _ := b.store.LoadCharacter(ctx, acc.ID, "Badman"); c.CtlCode != 1 {
		t.Fatal("ctl code not stored")
	}
	p.P.LogoutCountdown = 0 // FUN_0050c6d0 skips closing characters
	b.runCommand(g, "/banacc Badman")
	b.runAsync()
	if a, _ := b.store.AccountByLogin(ctx, p.P.Account.Login); a.BlockCode != 1 {
		t.Fatal("bloc code not stored")
	}
	b.drain(g)
	b.runCommand(g, "/unbanchar Bad")
	hasText(t, b.drain(g), b.messageFor(g, msgGMNameShort))
	b.runCommand(g, "/unbanchar Nobody")
	b.runAsync()
	hasText(t, b.drain(g), b.messageFor(g, msgGMNameMissing))
	b.runCommand(g, "/unbanchar Badman")
	b.runAsync()
	hasText(t, b.drain(g), b.messageFor(g, msgGMUnblocked, "Badman"))
	if c, _ := b.store.LoadCharacter(ctx, acc.ID, "Badman"); c.CtlCode != 0 {
		t.Fatal("ban not lifted")
	}
	b.runCommand(g, "/unbanacc "+p.P.Account.Login)
	b.runAsync()
	if a, _ := b.store.AccountByLogin(ctx, p.P.Account.Login); a.BlockCode != 0 {
		t.Fatal("account ban not lifted")
	}
}

func TestGMChatBan(t *testing.T) {
	b := newBareWorld(t)
	g := gmPlayer(b, PlayerStart, "Gm", 32, 0, 130, 130)
	p := b.player(PlayerStart+1, "Loud", 0, 131, 130)
	b.runCommand(g, "/banchat Loud 0")
	hasText(t, b.drain(g), b.messageFor(g, msgGMChatMinutes))
	b.runCommand(g, "/banchat Loud 5")
	if p.P.Char.ChatLimitTime != 5 {
		t.Fatal("chat ban")
	}
	hasText(t, b.drain(g), b.messageFor(g, msgGMChatBanned, "Loud", 5))
	if !b.chatBlocked(p) {
		t.Fatal("chat not blocked")
	}
	b.runCommand(g, "/unbanchat Loud")
	if p.P.Char.ChatLimitTime != 0 {
		t.Fatal("chat unban")
	}
	hasText(t, b.drain(g), b.messageFor(g, msgGMChatUnbanned, "Loud"))
}

func TestGMEventForceStart(t *testing.T) {
	b := newBareWorld(t)
	g := gmPlayer(b, PlayerStart, "Gm", 32, 0, 130, 130)
	b.clock = time.Date(2026, 10, 22, 10, 20, 30, 0, time.UTC)
	r := forceStartRow(b.clock)
	if r.Year != 2026 || r.Month != 10 || r.Day != 22 || r.DayOfWeek != -1 || r.Hour != 10 || r.Minute != 21 || r.Second != 30 {
		t.Fatalf("row %+v", r)
	}
	b.runCommand(g, "/bcstart")
	b.runCommand(g, "/dsstart")
	b.runCommand(g, "/ccstart")
	want := b.clock.Add(time.Minute)
	if b.bcOn() {
		if c := b.bc().c[0]; c.State != devilsquare.StateEmpty || !c.Target.Equal(want) {
			t.Fatalf("bc %d %v", c.State, c.Target)
		}
	}
	if b.dsOn() {
		if s := b.ds().sq[0]; !s.Target.Equal(want) {
			t.Fatalf("ds %v", s.Target)
		}
	}
	if b.ccOn() {
		if c := b.cc().c[0]; !c.Target.Equal(want) {
			t.Fatalf("cc %v", c.Target)
		}
	}
}

func TestGMSetBuff(t *testing.T) {
	b := newBareWorld(t)
	g := gmPlayer(b, PlayerStart, "Gm", 32, 0, 130, 130)
	p := b.player(PlayerStart+1, "Buffed", 0, 131, 130)
	b.runCommand(g, "/setbuff Buffed 200 0 60 0 0 0 0")
	hasText(t, b.drain(g), b.messageFor(g, msgGMBuffMissing))
	b.runCommand(g, "/setbuff Buffed 10 0 60 0 0 0 0")
	if !hasEffectIndex(p, 10) {
		t.Fatal("buff not added")
	}
	b.runCommand(g, "/setbuff Buffed 10 0 60 0 0 0 0")
	if hasEffectIndex(p, 10) {
		t.Fatal("buff not toggled off")
	}
	b.runCommand(g, "/setbuff Buffed 10 0 60 0 0 0 0")
	b.runCommand(g, "/setbuff Buffed -1")
	if hasEffectIndex(p, 10) {
		t.Fatal("buffs not cleared")
	}
}

func TestGMPost(t *testing.T) {
	b := newBareWorld(t)
	g := gmPlayer(b, PlayerStart, "Gm", 32, 0, 130, 130)
	p := b.player(PlayerStart+1, "Reader", 0, 131, 130)
	b.runCommand(g, "/gg hello all")
	if b.mapServerGroup() >= 0 {
		hasText(t, b.drain(p), b.messageFor(nil, msgGMPost, "Gm", "hello all"))
	}
}
