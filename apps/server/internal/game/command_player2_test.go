package game

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/config/gsconfig"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/item"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/store"
)

// withConfig reloads the instance configuration with some keys of one
// file replaced (or appended).
func withConfig(t *testing.T, b *bareWorld, file string, kv map[string]string) {
	t.Helper()
	dir := t.TempDir()
	for _, name := range gsconfig.Files {
		fn := "GameServerInfo - " + name + ".dat"
		data, err := os.ReadFile(repoData("4.GameServer", "DATA", fn))
		if err != nil {
			t.Fatal(err)
		}
		if name == file {
			lines := strings.Split(string(data), "\n")
			done := map[string]bool{}
			for i, l := range lines {
				k := strings.TrimSpace(strings.SplitN(l, "=", 2)[0])
				if v, ok := kv[k]; ok {
					lines[i] = k + " = " + v
					done[k] = true
				}
			}
			for k, v := range kv {
				if !done[k] {
					lines = append(lines, k+" = "+v)
				}
			}
			data = []byte(strings.Join(lines, "\n"))
		}
		if err := os.WriteFile(filepath.Join(dir, fn), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	cfg, err := gsconfig.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	b.cfg = cfg
}

// texts returns the texts of the C1 0D notices and C1 86 F3 E4 notices.
func texts(pkts [][]byte) []string {
	var out []string
	for _, p := range pkts {
		switch {
		case len(p) > 13 && p[2] == 0x0D:
			out = append(out, strings.TrimRight(string(p[13:]), "\x00"))
		case len(p) > 6 && p[2] == 0xF3 && p[3] == 0xE4:
			out = append(out, strings.TrimRight(string(p[6:]), "\x00"))
		}
	}
	return out
}

func hasText(t *testing.T, pkts [][]byte, want string) {
	t.Helper()
	for _, s := range texts(pkts) {
		if s == want {
			return
		}
	}
	t.Fatalf("no %q in %q", want, texts(pkts))
}

// safeCell finds a walkable safe-zone cell of map mp.
func (b *bareWorld) safeCell(mp int) (int, int) {
	t := b.terrain(mp)
	for y := 0; y < 256; y++ {
		for x := 0; x < 256; x++ {
			if a := t.Get(x, y); a&1 != 0 && a&(4|8) == 0 {
				return x, y
			}
		}
	}
	b.t.Fatal("no safe cell")
	return 0, 0
}

func TestCommandRequest(t *testing.T) {
	b := newBareWorld(t)
	o := b.player(PlayerStart, "Req", 0, 130, 130)
	ps := b.partyStateOf(o)
	b.runCommand(o, "/request off")
	if ps.Flags != 0 {
		t.Fatalf("off flags %x", ps.Flags)
	}
	hasText(t, b.drain(o), b.messageFor(o, 0x141))
	b.runCommand(o, "/request auto senha123 x")
	if ps.Flags != 7 || ps.Password != "senha123" {
		t.Fatalf("auto %x %q", ps.Flags, ps.Password)
	}
	hasText(t, b.drain(o), b.messageFor(o, 0x142))
	b.runCommand(o, "/request on")
	if ps.Flags != 3 || ps.Password != "" {
		t.Fatalf("on %x %q", ps.Flags, ps.Password)
	}
	mv(o).CommandTicks = nil
	b.runCommand(o, "/request maybe")
	if _, ok := mv(o).CommandTicks[3]; ok || len(b.drain(o)) > 1 {
		t.Fatal("unknown word handled")
	}
}

func TestCommandChange(t *testing.T) {
	b := newBareWorld(t)
	o := b.player(PlayerStart, "Evo", 0, 130, 130)
	o.Class, o.P.Char.Class = 1, 0x10
	o.P.Char.Quest = bytes.Repeat([]byte{0xFF}, 50)
	b.runCommand(o, "/evo")
	for q := 0; q <= 3; q++ {
		if questState(o, q) != 2 {
			t.Fatalf("quest %d state %d", q, questState(o, q))
		}
	}
	if questState(o, 4) != 3 {
		t.Fatal("quest 4 touched")
	}
	pk := b.drain(o)
	hasText(t, pk, b.messageFor(o, 0x14C))
	if find(pk, 0xA0, -1) == nil {
		t.Fatal("no quest list")
	}
	// A Magic Gladiator counts one evolution more: with
	// CommandChangeLimit_AL0 = 1 it is refused at once.
	m := b.player(PlayerStart+1, "Glad", 0, 131, 130)
	m.Class, m.P.Char.Class = 3, 0x30
	m.P.Char.Quest = bytes.Repeat([]byte{0xFF}, 50)
	b.runCommand(m, "/evo")
	hasText(t, b.drain(m), b.messageFor(m, 0x14B))
	if questState(m, 0) != 3 {
		t.Fatal("refused /evo changed quests")
	}
}

func TestCommandBau(t *testing.T) {
	b := newBareWorld(t)
	x, y := b.safeCell(0)
	o := b.player(PlayerStart, "Vault", 0, x, y)
	b.runCommand(o, "/bau 21")
	hasText(t, b.drain(o), "Uso correto: /bau 1 ate /bau 20")
	b.runCommand(o, "/bau 5")
	st := items(o)
	if st.WarehouseNumber != 4 {
		t.Fatalf("number %d", st.WarehouseNumber)
	}
	hasText(t, b.drain(o), "Bau 5 selecionado.")
	// In a safe zone the vault opens at once.
	b.runAsync()
	if !st.WarehouseLoaded {
		t.Fatal("vault not opened")
	}
	b.drain(o)
	b.runCommand(o, "/bau 2")
	hasText(t, b.drain(o), "Feche a interface atual antes de trocar de bau.")
	if st.WarehouseNumber != 4 {
		t.Fatal("changed while open")
	}

	// The table entry (FUN_00438110) uses 0..19.
	p := b.player(PlayerStart+1, "Vault2", 0, 130, 130)
	c := b.commandByIndex(13)
	mv(p).CommandTicks = map[int]time.Time{}
	b.commandWare(p, c, "20")
	hasText(t, b.drain(p), b.messageFor(p, 0x14E))
	b.commandWare(p, c, "0")
	if items(p).WarehouseNumber != 0 {
		t.Fatal("number")
	}
	hasText(t, b.drain(p), b.messageFor(p, 0x14F, 0))
}

func resetPlayer(b *bareWorld, index int, name string, class int) *Object {
	o := b.player(index, name, 0, 130, 130)
	o.Class, o.P.Char.Class = class, byte(class<<4)
	o.P.Char.Quest = bytes.Repeat([]byte{0xFF}, 50)
	return o
}

func TestCommandReset(t *testing.T) {
	b := newBareWorld(t)
	o := resetPlayer(b, PlayerStart, "Resetter", 1)
	ch := o.P.Char
	ch.Level, ch.Money, ch.Experience = 399, 1000000, 12345
	b.runCommand(o, "/reset")
	hasText(t, b.drain(o), b.messageFor(o, 0x151, 400))
	ch.Level = 400
	b.runCommand(o, "/reset")
	if !cmdOf(o).ResetPending {
		t.Fatal("no DS request")
	}
	b.runAsync()
	// CommandResetType 0: level 400 - 400 -> 1, zen -5000, points kept.
	if ch.Resets != 1 || ch.Level != 1 || ch.Money != 995000 || ch.Experience != 0 || cmdOf(o).ResetPending {
		t.Fatalf("reset %d level %d money %d exp %d", ch.Resets, ch.Level, ch.Money, ch.Experience)
	}
	pk := b.drain(o)
	hasText(t, pk, b.messageFor(o, 0x15E, 1))
	if find(pk, 0xF3, 0xE0) == nil {
		t.Fatal("no F3 E0")
	}
	if _, ok := mv(o).CommandTicks[14]; !ok {
		t.Fatal("not charged")
	}
	// CommandResetMove_AL0 = 1: the DK goes to gate 17.
	if g := b.data.Gates[17]; o.Map != g.Map {
		t.Fatalf("map %d", o.Map)
	}
	cs := b.commandStore()
	if c, _ := cs.ResetInfo(context.Background(), o.P.Account.Login, ch.Name, false); c != (store.ResetCounts{Day: 1, Week: 1, Month: 1}) {
		t.Fatalf("counts %+v", c)
	}

	// CommandResetLimitDay_AL0 = 50 (the gate move left the player
	// teleporting: settle it first).
	o.Live, o.Teleporting = LiveNormal, false
	cs.SetResetInfo(context.Background(), o.P.Account.Login, ch.Name, false, 1, 0, store.ResetCounts{Day: 50, Week: 50, Month: 50})
	ch.Level = 400
	b.runCommand(o, "/reset")
	b.runAsync()
	hasText(t, b.drain(o), b.messageFor(o, 0x15B, 50))
	if ch.Resets != 1 {
		t.Fatal("reset past the daily limit")
	}

	// Equipment with CheckItem, and a busy player.
	withConfig(t, b, "Command", map[string]string{"CommandResetCheckItem_AL0": "1"})
	o.P.Inventory[0] = item.Item{Index: 0, Durability: 10}
	b.runCommand(o, "/reset")
	hasText(t, b.drain(o), b.messageFor(o, 0x157))
	o.P.Inventory[0] = item.Empty()
	items(o).If.Use = true
	b.runCommand(o, "/reset")
	hasText(t, b.drain(o), b.messageFor(o, 0x150))
	// A Summon Party countdown (obj+0x5d2) also counts as busy.
	items(o).If.Use = false
	b.partySummonOf(o).count = 3
	b.runCommand(o, "/reset")
	hasText(t, b.drain(o), b.messageFor(o, 0x150))
}

func TestCommandResetPoints(t *testing.T) {
	b := newBareWorld(t)
	withConfig(t, b, "Command", map[string]string{
		"CommandResetType": "1", "CommandResetClearSkill_AL0": "1", "CommandResetClearQuest_AL0": "1",
		"CommandResetMove_AL0": "0",
	})
	o := resetPlayer(b, PlayerStart, "Points", 1)
	ch := o.P.Char
	ch.Class = 0x11 // evolved
	ch.Level, ch.Resets, ch.LevelUpPoints = 400, 2, 9999
	ch.Strength, ch.Dexterity = 500, 600
	o.P.Skills[5] = Skill{Index: 0x13}
	b.runCommand(o, "/reset")
	b.runAsync()
	// 3 resets x CommandResetPoint_AL0 (263) x RateDK (100%) + (1-1) x
	// DKLevelUpPoint + no quest/fruit points.
	if ch.LevelUpPoints != 3*263 {
		t.Fatalf("points %d", ch.LevelUpPoints)
	}
	ci := b.statsData.classes[1]
	if ch.Strength != ci.Strength || ch.Dexterity != ci.Dexterity {
		t.Fatalf("stats %d %d", ch.Strength, ch.Dexterity)
	}
	if ch.Class != 0x10 {
		t.Fatalf("evolution kept %x", ch.Class)
	}
	if p := find(b.drain(o), 0xA3, -1); p == nil || p[5] != 0xC9 || p[6] != evolutionByteOf(1, 0) {
		t.Fatalf("A3 % X", p)
	}
	for _, s := range o.P.Skills {
		if s.Index == 0x13 {
			t.Fatal("skill not cleared")
		}
	}
	if o.P.Skills[0].Index < 0 {
		t.Fatal("class skills not learned again")
	}
}

func TestResetTableLookups(t *testing.T) {
	b := newBareWorld(t)
	o := resetPlayer(b, PlayerStart, "Table", 0)
	o.P.Char.Resets = 4
	b.cmdWorld().resetTable = []resetTableRow{{MinReset: 1, MaxReset: 3, Level: [4]int{100, -1, -1, -1},
		Point: [4]int{10, 0, 0, 0}, Money: [4]int{-1, -1, -1, -1}}, {MinReset: 5, MaxReset: 9, Level: [4]int{350, 0, 0, 0},
		Money: [4]int{7, 0, 0, 0}, Point: [4]int{-1, 0, 0, 0}}}
	b.cmdWorld().resetLimit = []resetLimitRow{{MinReset: 5, MaxReset: 5, Day: [4]int{2, 0, 0, 0}, Week: [4]int{-1, 0, 0, 0}, Month: [4]int{9, 0, 0, 0}}}
	if b.resetLevel(o) != 350 || b.resetMoney(o) != 7 {
		t.Fatalf("level %d money %d", b.resetLevel(o), b.resetMoney(o))
	}
	// Resets 1..3 from the table (10 each), 4 from Command.dat (263).
	if p := b.resetPoints(o); p != 30+263 {
		t.Fatalf("points %d", p)
	}
	if d, wk, m := b.resetLimits(o); d != 2 || wk != 10000 || m != 9 {
		t.Fatalf("limits %d %d %d", d, wk, m)
	}
	o.P.Char.Resets = 0
	if b.resetLevel(o) != 100 || b.resetMoney(o) != 5000 {
		t.Fatalf("level %d money %d", b.resetLevel(o), b.resetMoney(o))
	}
}

func TestCommandResetAuto(t *testing.T) {
	b := newBareWorld(t)
	o := resetPlayer(b, PlayerStart, "AutoRes", 1)
	b.runCommand(o, "/reset auto 1 2 3 4")
	hasText(t, b.drain(o), b.messageFor(o, 0x158))
	withConfig(t, b, "Command", map[string]string{"CommandResetAutoEnable_AL0": "1", "CommandResetMove_AL0": "0"})
	b.runCommand(o, "/reset auto 10 20 30 40 50")
	s := cmdOf(o)
	if !s.AutoReset || s.AutoPoints != [5]int{10, 20, 30, 40, 0} {
		t.Fatalf("auto %+v", s)
	}
	hasText(t, b.drain(o), b.messageFor(o, 0x159))
	ch := o.P.Char
	ch.Level, ch.LevelUpPoints = 400, 100
	ch.Strength, ch.Dexterity, ch.Vitality, ch.Energy = 10, 10, 10, 10
	b.autoResetSecond()
	b.runAsync()
	if ch.Resets != 1 {
		t.Fatal("no auto reset")
	}
	// The 100 kept points are spread with the auto amounts.
	if ch.Strength != 20 || ch.Dexterity != 30 || ch.Vitality != 40 || ch.Energy != 50 || ch.LevelUpPoints != 0 {
		t.Fatalf("stats %d %d %d %d points %d", ch.Strength, ch.Dexterity, ch.Vitality, ch.Energy, ch.LevelUpPoints)
	}
	b.runCommand(o, "/reset auto")
	if s.AutoReset || s.AutoPoints != [5]int{} {
		t.Fatal("auto reset not turned off")
	}
}

func TestCommandMasterReset(t *testing.T) {
	b := newBareWorld(t)
	o := resetPlayer(b, PlayerStart, "Master", 0)
	ch := o.P.Char
	ch.Level, ch.Resets, ch.Money = 400, 10, 100000
	b.runCommand(o, "/mreset")
	hasText(t, b.drain(o), b.messageFor(o, 0x161, 400))
	ch.Resets = 450
	b.runCommand(o, "/mreset")
	b.runAsync()
	// StartLevel_AL0 20, StartReset -1: resets 450-400.
	if ch.MasterResets != 1 || ch.Level != 20 || ch.Resets != 50 || ch.Money != 50000 {
		t.Fatalf("mreset %d level %d resets %d money %d", ch.MasterResets, ch.Level, ch.Resets, ch.Money)
	}
	hasText(t, b.drain(o), b.messageFor(o, 0x16B, 1))
	if _, ok := mv(o).CommandTicks[15]; !ok {
		t.Fatal("not charged")
	}
	c, _ := b.commandStore().ResetInfo(context.Background(), o.P.Account.Login, ch.Name, true)
	if c != (store.ResetCounts{Day: 1, Week: 1, Month: 1}) {
		t.Fatalf("counts %+v", c)
	}
}

func TestCommandRename(t *testing.T) {
	b := newBareWorld(t)
	ctx := context.Background()
	acc, _ := b.store.CreateAccount(ctx, "renacc", "plain:x", "")
	b.store.CreateCharacter(ctx, acc.ID, "OldName", 0x10)
	b.store.CreateCharacter(ctx, acc.ID, "Taken", 0x10)
	row, _ := b.store.LoadCharacter(ctx, acc.ID, "OldName")
	x, y := b.safeCell(0)
	ox, oy := b.openRow(0, 1)
	o := b.player(PlayerStart, "OldName", 0, ox, oy)
	o.P.Char, o.P.Account = row, *acc
	b.runCommand(o, "/rename ab_cd")
	hasText(t, b.drain(o), b.messageFor(o, 0x2D3))
	b.runCommand(o, "/rename abc")
	hasText(t, b.drain(o), b.messageFor(o, 0x2B1))
	b.runCommand(o, "/rename NewName")
	hasText(t, b.drain(o), b.messageFor(o, 0x2B2))
	o.X, o.Y = x, y
	guildOf(o).Number = 3
	b.runCommand(o, "/rename NewName")
	hasText(t, b.drain(o), b.messageFor(o, 0x2C8))
	guildOf(o).Number = 0
	b.runCommand(o, "/rename Taken")
	b.runAsync()
	hasText(t, b.drain(o), b.messageFor(o, 0x2B5))
	b.runCommand(o, "/rename NewName")
	b.runAsync()
	hasText(t, b.drain(o), b.messageFor(o, 0x2B6))
	if o.P.Char.Name != "NewName" || o.P.LogoutCountdown != 6 || o.P.LogoutType != 1 {
		t.Fatalf("name %q countdown %d type %d", o.P.Char.Name, o.P.LogoutCountdown, o.P.LogoutType)
	}
	if _, err := b.store.LoadCharacter(ctx, acc.ID, "NewName"); err != nil {
		t.Fatal("store not renamed")
	}
	if _, ok := mv(o).CommandTicks[19]; !ok {
		t.Fatal("not charged")
	}
}

func TestCommandDCFriend(t *testing.T) {
	b := newBareWorld(t)
	o := b.player(PlayerStart, "Caller", 0, 130, 130)
	f := b.player(PlayerStart+1, "Friend", 0, 140, 140)
	f.P.Account.PasswordHash = "plain:segredo"
	b.runCommand(o, "/dcfriend Nobody segredo")
	hasText(t, b.drain(o), b.messageFor(o, 0x2B8))
	b.runCommand(o, "/dcfriend Friend errado")
	hasText(t, b.drain(o), b.messageFor(o, 0x2B9))
	if f.P.LogoutCountdown != 0 {
		t.Fatal("disconnected with a wrong password")
	}
	b.runCommand(o, "/dcfriend Friend segredo")
	hasText(t, b.drain(o), b.messageFor(o, 0x2BA))
	if f.P.LogoutCountdown != 6 || f.P.LogoutType != 0 {
		t.Fatalf("countdown %d type %d", f.P.LogoutCountdown, f.P.LogoutType)
	}
}

func TestCommandOpenWare(t *testing.T) {
	b := newBareWorld(t)
	x, y := b.openRow(0, 1)
	o := b.player(PlayerStart, "Opener", 0, x, y)
	b.runCommand(o, "/open")
	hasText(t, b.drain(o), "Utilize o Comando em Zona Segura")
	o.X, o.Y = b.safeCell(0)
	b.runCommand(o, "/open")
	b.runAsync()
	if !items(o).WarehouseLoaded {
		t.Fatal("vault not opened")
	}
	if _, ok := mv(o).CommandTicks[22]; !ok {
		t.Fatal("not charged")
	}
}

func TestCommandChangeClass(t *testing.T) {
	b := newBareWorld(t)
	o := resetPlayer(b, PlayerStart, "Switch", 0)
	b.runCommand(o, "/classe")
	hasText(t, b.drain(o), b.messageFor(o, 0x38A))
	b.runCommand(o, "/classe dw")
	hasText(t, b.drain(o), b.messageFor(o, 0x38B))
	o.P.Inventory[2] = item.Item{Index: 7*512 + 1, Durability: 10}
	b.runCommand(o, "/classe dl")
	hasText(t, b.drain(o), b.messageFor(o, 0x38C))
	if o.P.Char.Leadership != 15 || o.Class != 0 {
		t.Fatal("dl word sets the leadership before the item check")
	}
	o.P.Inventory[2] = item.Empty()
	o.P.Skills[0] = Skill{Index: 0x11}
	b.runCommand(o, "/classe elf")
	if o.Class != 2 || o.P.Char.Class != 0x20 || o.P.Skills[0].Index != -1 {
		t.Fatalf("class %d %x skill %d", o.Class, o.P.Char.Class, o.P.Skills[0].Index)
	}
	if o.P.LogoutCountdown != 1 || o.P.LogoutType != 1 {
		t.Fatal("no return to the character list")
	}
	if questState(o, 0) != 3 {
		t.Fatal("quests not reset")
	}
}

func TestCommandEZen(t *testing.T) {
	b := newBareWorld(t)
	o := b.player(PlayerStart, "EZen", 0, 130, 130)
	ch := o.P.Char
	ch.Money = 1099999999
	b.runCommand(o, "/ezen")
	hasText(t, b.drain(o), b.messageFor(o, 0x386))
	ch.Money = 2000000000
	b.runCommand(o, "/ezen")
	b.runAsync()
	if ch.Money != 900000000 {
		t.Fatalf("money %d", ch.Money)
	}
	found := false
	for s := BagStart; s < BagEnd; s++ {
		found = found || o.P.Inventory[s].Index == ezenItem
	}
	if !found {
		t.Fatal("no (14,12) created")
	}
	hasText(t, b.drain(o), b.messageFor(o, 0x387))
	for s := BagStart; s < BagEnd; s++ {
		o.P.Inventory[s] = item.Item{Index: 14*512 + 13, Durability: 1}
	}
	b.runCommand(o, "/ezen")
	hasText(t, b.drain(o), b.messageFor(o, 0x385))
}

func TestCommandAutoPotion(t *testing.T) {
	b := newBareWorld(t)
	o := b.player(PlayerStart, "Potion", 0, 130, 130)
	ch := o.P.Char
	ch.MaxLife, ch.Life, ch.MaxMana, ch.Mana = 1000, 900, 100, 100
	b.runCommand(o, "/autopotion")
	hasText(t, b.drain(o), "[AutoPotion] Ativado - HP 50% / Mana 30%")
	o.P.Inventory[BagStart+3] = item.Item{Index: 14*512 + 1, Durability: 5}
	o.P.Inventory[BagStart+7] = item.Item{Index: 14*512 + 3, Durability: 5}
	b.autoPotionSecond()
	if o.P.Inventory[BagStart+7].Durability != 5 {
		t.Fatal("potion used above 50%")
	}
	ch.Life = 500
	b.autoPotionSecond()
	// The large potion (14,3) comes first.
	if o.P.Inventory[BagStart+7].Durability != 4 || o.P.Inventory[BagStart+3].Durability != 5 {
		t.Fatalf("durability %v %v", o.P.Inventory[BagStart+7].Durability, o.P.Inventory[BagStart+3].Durability)
	}
	ch.Life = 100
	b.advance(500 * time.Millisecond)
	b.autoPotionSecond()
	if o.P.Inventory[BagStart+7].Durability != 4 {
		t.Fatal("potion used within 600 ms")
	}
	b.advance(100 * time.Millisecond)
	b.autoPotionSecond()
	if o.P.Inventory[BagStart+7].Durability != 3 {
		t.Fatal("potion not used after 600 ms")
	}
	b.runCommand(o, "/autopotion")
	hasText(t, b.drain(o), "[AutoPotion] Desativado")
	if cmdOf(o).AutoPotion {
		t.Fatal("still on")
	}
}

func TestCommandLock(t *testing.T) {
	b := newBareWorld(t)
	o := b.player(PlayerStart, "Locker", 0, 130, 130)
	b.runCommand(o, "/unlock 5")
	if len(b.drain(o)) != 0 {
		t.Fatal("unlock without a lock answered")
	}
	b.runCommand(o, "/lock 100000000")
	hasText(t, b.drain(o), b.messageFor(o, 0x6EE))
	b.runCommand(o, "/lock 1234")
	if o.P.Account.Lock != 1234 {
		t.Fatal("not locked")
	}
	hasText(t, b.drain(o), b.messageFor(o, 0x6ED))
	b.runCommand(o, "/lock 99")
	hasText(t, b.drain(o), b.messageFor(o, 0x6EF))
	b.runCommand(o, "/unlock 99")
	hasText(t, b.drain(o), b.messageFor(o, 0x6F1))
	b.runCommand(o, "/unlock 1234")
	if o.P.Account.Lock != 0 {
		t.Fatal("not unlocked")
	}
	hasText(t, b.drain(o), b.messageFor(o, 0x6F0))
	b.runAsync()
}

func TestCustomQuiz(t *testing.T) {
	b := newBareWorld(t)
	q := b.cmdWorld().quiz
	if q == nil {
		t.Fatal("quiz not loaded")
	}
	s := q.slots[0]
	if s.State != quizEmpty || s.Target.Hour() != 18 || s.Target.Minute() != 15 {
		t.Fatalf("slot %+v", s)
	}
	o := b.player(PlayerStart, "Quizzer", 0, 130, 130)
	o.P.Char.Money = 100
	fixRand(t, 3)
	b.clock = s.Target.Add(-3 * time.Minute)
	b.quizTick()
	hasText(t, b.drain(o), b.messageFor(nil, quizMsgAlarm, "Quiz", 3))
	b.advance(3 * time.Minute)
	b.quizTick()
	pk := b.drain(o)
	hasText(t, pk, b.messageFor(nil, quizMsgText, "Quiz", q.data.Questions[3].Question))
	if !q.Active || q.Question != 3 || s.State != quizStart {
		t.Fatalf("quiz %+v slot %+v", q, s)
	}
	b.runCommand(o, "/resp newton")
	hasText(t, b.drain(o), b.messageFor(nil, quizMsgWrong, "Quiz"))
	b.runCommand(o, "/resp ALBERT einstein")
	pk = b.drain(o)
	hasText(t, pk, b.messageFor(nil, quizMsgWinner, "Quiz", "Quizzer"))
	hasText(t, pk, b.messageFor(nil, quizMsgAnswer, "Quiz", "albert einstein"))
	if o.P.Char.Money != 103 || q.Active {
		t.Fatalf("money %d active %v", o.P.Char.Money, q.Active)
	}
	if find(pk, 0xF3, 0x40) == nil {
		t.Fatal("no firework")
	}
	b.advance(time.Second)
	b.quizTick()
	if s.State != quizEmpty || !s.Target.After(b.clock) {
		t.Fatalf("not rescheduled %+v", s)
	}
}

func TestQuizNoWinner(t *testing.T) {
	b := newBareWorld(t)
	q := b.cmdWorld().quiz
	s := q.slots[0]
	o := b.player(PlayerStart, "Late", 0, 130, 130)
	fixRand(t, 0)
	b.clock = s.Target
	b.quizTick()
	b.drain(o)
	b.advance(time.Minute)
	b.quizTick()
	hasText(t, b.drain(o), b.messageFor(nil, quizMsgText, "Quiz", q.data.Questions[0].Question))
	b.advance(2 * time.Minute)
	b.quizTick()
	hasText(t, b.drain(o), b.messageFor(nil, quizMsgNoWinner, "Quiz"))
	if q.Active || s.State != quizEmpty {
		t.Fatal("quiz still running")
	}
}

func TestCommandGuildWarShortName(t *testing.T) {
	b := newBareWorld(t)
	o := b.player(PlayerStart, "Warrior", 0, 130, 130)
	b.runCommand(o, "/guildwar abc")
	if p := find(b.drain(o), 0x60, -1); !bytes.Equal(p, []byte{0xC1, 0x04, 0x60, 0x00}) {
		t.Fatalf("60 = % X", p)
	}
}

func TestEvolutionByteOf(t *testing.T) {
	o := &Object{Class: 2, P: &Player{Char: &store.Character{Class: 0x21}}}
	if evolutionByteOf(2, 1) != evolutionByte(o) {
		t.Fatal("evolution byte")
	}
	if !validRename("Abcd") || validRename("ab cd") || validRename("a%b") {
		t.Fatal("validRename")
	}
}
