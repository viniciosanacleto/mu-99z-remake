package game

import (
	"bytes"
	"testing"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/castlesiege"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/effect"
)

// csPair places a caster and another player one cell apart on map 30.
func csPair(t *testing.T) (*bareWorld, *Object, *Object) {
	t.Helper()
	b := newBareWorld(t)
	x, y := b.openRow(castlesiege.Map, 4)
	o := b.player(PlayerStart, "Lord", castlesiege.Map, x, y)
	v := b.player(PlayerStart+1, "Mate", castlesiege.Map, x+1, y)
	o.Class, v.Class = 4, 0
	b.see(o, v)
	return b, o, v
}

// noticeOf is the text of a 0D notice (empty when none).
func noticeOf(p []byte) string {
	if len(p) < 14 {
		return ""
	}
	return noticeText(p)
}

func TestSternSector(t *testing.T) {
	// Byte = (dy + 8) << 4 | (dx + 8) of the target cell; degrees are
	// atan2(8 - hi, 8 - lo) + 90.
	cases := map[int]int{0x88: 9, 0x87: 9, 0x78: 18, 0x89: 27, 0x98: 0}
	for in, want := range cases {
		if got := sternSector(in); got != want {
			t.Errorf("sternSector(%#x) = %d, want %d", in, got, want)
		}
	}
}

func TestGuildSkillsNeedCastleMaps(t *testing.T) {
	b := newBareWorld(t)
	o := b.player(PlayerStart, "Lord", 0, 130, 130)
	for _, n := range []int{67, 68, 69, 70, 71, 72, 75} {
		if b.guildSkill(o, b.skillRefOf(n, 0), o.Index, 0) {
			t.Errorf("skill %d cast outside maps 30/31", n)
		}
	}
}

func TestSkillStern(t *testing.T) {
	b, o, v := csPair(t)
	angle := -1
	for a := 0; a < 256; a++ {
		if s := sternSector(a); s <= 35 && b.hitboxHas(s, v.X-o.X+10, v.Y-o.Y+10) {
			angle = a
			break
		}
	}
	if angle < 0 {
		t.Skip("no sector holds the cell")
	}
	sk(o).KillCount = 3
	if !b.guildSkill(o, b.skillRefOf(67, 0), -1, angle) {
		t.Fatal("stern refused")
	}
	if s := effectSlotOf(v, effect.Stern); s == nil || s.Count != b.cfg.Int("Skill", "SternTimeConstA", 0) {
		t.Fatalf("stern = %+v", s)
	}
	out := b.drain(o)
	if r := find(out, 0xB8, -1); !bytes.Equal(r, []byte{0xC1, 5, 0xB8, 1, 0}) || sk(o).KillCount != 0 {
		t.Fatalf("kill count = % X", r)
	}
	if r := find(out, 0x19, -1); r == nil || r[3] != 67 || int(r[6]&0x7F)<<8|int(r[7]) != v.Index {
		t.Fatalf("19 = % X", r)
	}
	// Remove Stern (68) clears it for the players within Radio.
	b.guildSkill(o, b.skillRefOf(68, 0), -1, 0)
	if hasEffectIndex(v, effect.Stern) {
		t.Fatal("stern not removed")
	}
}

func TestSkillGreaterManaGuild(t *testing.T) {
	b, o, v := csPair(t)
	other := b.player(PlayerStart+2, "Other", castlesiege.Map, o.X+2, o.Y)
	b.see(o, other)
	guildOf(o).Number, guildOf(v).Number, guildOf(other).Number = 7, 7, 8
	if !b.guildSkill(o, b.skillRefOf(69, 0), -1, 0) {
		t.Fatal("greater mana refused")
	}
	val, secs := b.cfg.Int("Skill", "GreaterManaConstA", 0), b.cfg.Int("Skill", "GreaterManaTimeConstA", 0)
	for _, p := range []*Object{o, v} {
		if s := effectSlotOf(p, effect.GreaterMana); s == nil || s.Value[0] != val || s.Count != secs {
			t.Fatalf("%s greater mana = %+v", p.P.Char.Name, s)
		}
	}
	if hasEffectIndex(other, effect.GreaterMana) {
		t.Fatal("other guild buffed")
	}
}

func TestSkillInvisibilityParty(t *testing.T) {
	b, o, v := csPair(t)
	secs := b.cfg.Int("Skill", "InvisibilityTimeConstA", 0)
	// Not in the caster's party: the caster gets it.
	b.guildSkill(o, b.skillRefOf(70, 0), v.Index, 0)
	if !hasEffectIndex(o, effect.Invisibility) || hasEffectIndex(v, effect.Invisibility) {
		t.Fatal("non-party target")
	}
	b.removeEffect(o, effect.Invisibility)
	b.partyCreate(o)
	b.partyAdd(b.partyOf(o), v)
	b.guildSkill(o, b.skillRefOf(70, 0), v.Index, 0)
	if s := effectSlotOf(v, effect.Invisibility); s == nil || s.Count != secs {
		t.Fatalf("party target = %+v", s)
	}
	// Remove Invisibility (71).
	b.guildSkill(o, b.skillRefOf(71, 0), -1, 0)
	if hasEffectIndex(v, effect.Invisibility) {
		t.Fatal("not removed")
	}
}

func TestSkillBrandAndRemoveAll(t *testing.T) {
	b, o, v := csPair(t)
	far := b.player(PlayerStart+2, "Far", castlesiege.Map, o.X+4, o.Y)
	far.X = o.X + 20
	b.see(o, far)
	b.guildSkill(o, b.skillRefOf(75, 0), -1, 0)
	if s := effectSlotOf(o, effect.Brand); s == nil || s.Count != 0 {
		t.Fatalf("caster brand = %+v", s)
	}
	if !hasEffectIndex(v, effect.Brand) || hasEffectIndex(far, effect.Brand) {
		t.Fatal("brand radio")
	}
	// Remove All Effect (72) is FUN_0047a540 on everyone within Radio.
	b.guildSkill(o, b.skillRefOf(72, 0), -1, 0)
	if hasEffectIndex(v, effect.Brand) || !hasEffectIndex(o, effect.Brand) {
		t.Fatal("remove all")
	}
}

func TestSummonParty(t *testing.T) {
	b := newBareWorld(t)
	x, y := b.openRow(0, 8)
	o := b.player(PlayerStart, "Lord", 0, x, y)
	near := b.player(PlayerStart+1, "Near", 0, x+30, y)
	away := b.player(PlayerStart+2, "Away", 2, 50, 50)
	b.partyCreate(o)
	b.partyAdd(b.partyOf(o), near)
	b.partyAdd(b.partyOf(o), away)
	b.drain(near)
	b.drain(away)
	if !b.guildSkill(o, b.skillRefOf(63, 0), -1, 0) {
		t.Fatal("summon party refused")
	}
	if r := find(b.drain(o), 0x19, -1); r == nil || r[3] != 63 {
		t.Fatalf("19 = % X", r)
	}
	if got, want := noticeOf(find(b.drain(near), 0x0D, -1)), b.messageFor(near, 0x111, 5); got != want {
		t.Fatalf("countdown = %q, want %q", got, want)
	}
	for i := 4; i > 0; i-- {
		b.summonSecond()
		if got, want := noticeOf(find(b.drain(near), 0x0D, -1)), b.messageFor(near, 0x111, i); got != want {
			t.Fatalf("countdown %d = %q", i, got)
		}
	}
	b.drain(away)
	b.summonSecond()
	// Same map: the Teleport magic next to the caster (+-4 cells).
	if mv(near).TeleportState != 1 || near.X < x-4 || near.X > x+4 || near.Y < y-4 || near.Y > y+4 {
		t.Fatalf("near at %d,%d state %d", near.X, near.Y, mv(near).TeleportState)
	}
	if r := find(b.drain(near), 0x19, -1); r == nil || r[3] != teleportSkill {
		t.Fatalf("teleport 19 = % X", r)
	}
	// Other map: C3 08 1C FF map x y dir.
	if away.Map != 0 {
		t.Fatalf("away map %d", away.Map)
	}
	if r := find(b.drain(away), 0x1C, -1); r == nil || r[3] != 0xFF || r[4] != 0 {
		t.Fatalf("1C = % X", r)
	}
	// Walking cancels a countdown with message 272.
	mv(near).TeleportState, near.Teleporting = 0, false
	b.guildSkill(o, b.skillRefOf(63, 0), -1, 0)
	if s := b.q().summons[near.Index]; s == nil || !s.pending {
		t.Fatal("no second countdown")
	}
	b.drain(near)
	b.handleWalk(near, []byte{0xC1, 6, 0xD3, byte(near.X), byte(near.Y), 0})
	if got := noticeOf(find(b.drain(near), 0x0D, -1)); got != b.messageFor(near, 0x110) {
		t.Fatalf("cancel = %q", got)
	}
	if s := b.q().summons[near.Index]; s != nil {
		t.Fatal("countdown kept")
	}
}

func TestMoveSummonRules(t *testing.T) {
	b := newBareWorld(t)
	o := b.player(PlayerStart, "Mage", 0, 130, 130)
	if !b.moveSummonOK(o, 0, 130, 130) {
		t.Fatal("Lorencia refused")
	}
	if b.moveSummonOK(o, 5, 100, 100) {
		t.Fatal("map without a MoveSummon row accepted")
	}
	// PKLimitMoveSummon = 1, PKLimitFree = 0: outlaws stay.
	o.PKLevel = 5
	if b.moveSummonOK(o, 0, 130, 130) {
		t.Fatal("outlaw summoned")
	}
	o.PKLevel = 3
	// Level 2/3 for MG (3), DL (4) and class 6.
	b.q().moveSummon = []moveSummonRow{{Map: 0, X1: 1, Y1: 1, X2: 255, Y2: 255, MinLevel: 30, MaxLevel: -1,
		MinReset: -1, MaxReset: -1, AccountLevel: [4]int{1, 0, 1, 1}}}
	o.P.Char.Level = 20
	o.Class = 3
	if !b.moveSummonOK(o, 0, 130, 130) {
		t.Fatal("MG at 2/3 of the level")
	}
	o.Class = 1
	if b.moveSummonOK(o, 0, 130, 130) {
		t.Fatal("DK below the level")
	}
	o.Class, o.P.Account.Level = 3, 1
	if b.moveSummonOK(o, 0, 130, 130) {
		t.Fatal("account level switch off")
	}
	// Icarus needs wings or a Dinorant / Fenrir.
	b.q().moveSummon = []moveSummonRow{{Map: 10, X1: 1, Y1: 1, X2: 255, Y2: 255, MinLevel: -1, MaxLevel: -1,
		MinReset: -1, MaxReset: -1, AccountLevel: [4]int{1, 1, 1, 1}}}
	if b.moveSummonOK(o, 10, 100, 100) {
		t.Fatal("Icarus without wings")
	}
}

func TestTeleportAlly(t *testing.T) {
	b := newBareWorld(t)
	x, y := b.openRow(0, 8)
	o := b.player(PlayerStart, "Wiz", 0, x, y)
	v := b.player(PlayerStart+1, "Ally", 0, x+2, y)
	o.P.Skills[0].Index = 15
	o.P.Char.Mana, o.P.Char.BP = 1000, 1000
	pkt := func(dx int) []byte {
		return []byte{0xC1, 7, 0xB0, byte(v.Index), byte(v.Index >> 8), byte(x + dx), byte(y)}
	}
	// Not in the same party: nothing.
	b.handleTeleportAlly(o, pkt(4))
	if mv(v).TeleportState != 0 {
		t.Fatal("teleported a stranger")
	}
	b.partyCreate(o)
	b.partyAdd(b.partyOf(o), v)
	b.drain(o)
	// More than 8 cells from the target: refused (FUN_0050d0a0).
	b.handleTeleportAlly(o, pkt(11))
	if mv(v).TeleportState != 0 {
		t.Fatal("destination too far")
	}
	b.handleTeleportAlly(o, pkt(4))
	if mv(v).TeleportState != 1 || v.X != x+4 {
		t.Fatalf("ally at %d state %d", v.X, mv(v).TeleportState)
	}
	info := b.data.Skills[15]
	if o.P.Char.Mana != float32(1000-info.Mana) || o.P.Char.BP != float32(1000-info.BP) {
		t.Fatalf("mana %v bp %v", o.P.Char.Mana, o.P.Char.BP)
	}
	out := b.drain(o)
	if r := find(out, 0x19, -1); r == nil || r[3] != 15 || int(r[6]&0x7F)<<8|int(r[7]) != v.Index {
		t.Fatalf("19 = % X", r)
	}
	if find(out, 0x27, -1) == nil {
		t.Fatal("no mana packet")
	}
	// A skill delay failure answers C3 08 1C 00 to the caster.
	si := b.data.Skills[15]
	si.Delay = 5000
	b.data.Skills[15] = si
	mv(v).TeleportState, v.Teleporting = 0, false
	b.handleTeleportAlly(o, pkt(3))
	b.handleTeleportAlly(o, pkt(3))
	if r := find(b.drain(o), 0x1C, -1); r == nil || r[3] != 0 {
		t.Fatalf("delay answer = % X", r)
	}
}

func TestCastleTeleportClamp(t *testing.T) {
	if y := csTeleportGateY(0x6E, 0x80); y != 0x70 {
		t.Fatalf("gate line = %#x", y)
	}
	if y := csTeleportGateY(0x76, 0x70); y != 0x74 {
		t.Fatalf("inner side = %#x", y)
	}
	crown := &Object{X: 0xB0, Y: 0xC8}
	if x, y := csCrownClamp(crown, 0xA0, 0xB0); x != 0xA4 || y != 0xC0 {
		t.Fatalf("crown clamp = %#x,%#x", x, y)
	}
	if x, y := csCrownClamp(&Object{X: 10, Y: 10}, 0xA0, 0xB0); x != 0xA0 || y != 0xB0 {
		t.Fatal("outside the crown room")
	}
}
