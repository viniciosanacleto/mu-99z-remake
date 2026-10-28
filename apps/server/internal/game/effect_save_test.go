package game

import (
	"bytes"
	"testing"
	"time"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/store"
)

func TestEffectListEncoding(t *testing.T) {
	b := newBareWorld(t)
	o := b.player(PlayerStart, "Buff", 0, 130, 130)
	// Greater Damage (1, Save 1) is kept; Disable Magic (12, Save 0) is not.
	b.addEffect(o, effGreaterDamage, 60, [4]int{25, 0x1234}, false)
	b.addEffect(o, 12, 30, [4]int{}, false)
	enc := encodeEffectList(o)
	if len(enc) != store.EffectListSize {
		t.Fatalf("size %d", len(enc))
	}
	want := []byte{1, 0, 0, 0, 60, 0, 25, 0x12, 0x34, 0, 0, 0, 0}
	if !bytes.Equal(enc[:13], want) {
		t.Fatalf("record 0 = % X", enc[:13])
	}
	if !bytes.Equal(enc[13:26], bytes.Repeat([]byte{0xFF}, 13)) {
		t.Fatalf("unsaved effect stored: % X", enc[13:26])
	}
	for _, c := range enc[26:] {
		if c != 0xFF {
			t.Fatal("free slots must be 0xFF")
		}
	}
}

func TestEffectListLoadAndLogoutSave(t *testing.T) {
	b := newBareWorld(t)
	o := b.player(PlayerStart, "Buff", 0, 130, 130)
	list := bytes.Repeat([]byte{0xFF}, store.EffectListSize)
	copy(list, []byte{effGreaterDefense, 0, 0, 0, 45, 0, 30, 0, 0, 0, 0, 0, 0})
	copy(list[13:], []byte{0xEE, 0, 0, 0, 10, 0, 1, 0, 0, 0, 0, 0, 0}) // unknown effect: dropped
	o.P.Char.EffectList = list
	b.loadEffectList(o)
	s := effectSlotOf(o, effGreaterDefense)
	if s == nil || s.Count != 45 || s.Value[0] != 30 || effOpt(o).AddDefense != 30 {
		t.Fatalf("loaded = %+v opt %+v", s, effOpt(o))
	}
	if o.ViewSkillState&effectStateBit(effGreaterDefense) == 0 {
		t.Fatal("state bit not set")
	}
	if len(b.drain(o)) != 0 {
		t.Fatal("the load sends packets")
	}
	if !bytes.Equal(o.P.Char.EffectList[13:26], bytes.Repeat([]byte{0xFF}, 13)) {
		t.Fatal("refused record kept")
	}
	// The effect counts down, then the logout (leave world, then save)
	// stores what is left.
	b.effectSecond(o)
	b.leaveWorld(o)
	if o.Eff != nil {
		t.Fatal("effects not dropped on leave")
	}
	var saved *store.Character
	b.hooks.characterSave = append(b.hooks.characterSave, func(w *World, x *Object, c *store.Character) { saved = c })
	b.saveCharacter(o, nil)
	if saved == nil || !bytes.Equal(saved.EffectList[:13], []byte{effGreaterDefense, 0, 0, 0, 44, 0, 30, 0, 0, 0, 0, 0, 0}) {
		t.Fatalf("saved = % X", saved.EffectList[:13])
	}
}

func TestEffectType2Expiry(t *testing.T) {
	b := newBareWorld(t)
	o := b.player(PlayerStart, "Buff", 0, 130, 130)
	b.skills.effects[0x70] = effectInfo{Index: 0x70, Group: 0x70, Save: 1, Type: 2, Duration: 100, Power: [4]int{-1, -1, -1, -1}}
	now := b.now().Unix()
	if !b.addEffect(o, 0x70, 0, [4]int{}, false) {
		t.Fatal("type 2 refused")
	}
	s := effectSlotOf(o, 0x70)
	if s.Count != 100 || s.Expire != now+100 {
		t.Fatalf("type 2 = %+v", s)
	}
	enc := encodeEffectList(o)
	if r := enc[:5]; r[0] != 0x70 || uint32(r[1])<<24|uint32(r[2])<<16|uint32(r[3])<<8|uint32(r[4]) != uint32(now+100) {
		t.Fatalf("type 2 record = % X", enc[:13])
	}
	// Loaded after the end time: refused (FUN_004785e0).
	p := b.player(PlayerStart+1, "Late", 0, 131, 130)
	p.P.Char.EffectList = enc
	b.advance(200 * time.Second)
	b.loadEffectList(p)
	if effectSlotOf(p, 0x70) != nil {
		t.Fatal("expired type 2 loaded")
	}
}

func TestEnterWorldLoadsSavedEffects(t *testing.T) {
	b := newBareWorld(t)
	o := b.player(PlayerStart, "Buff", 0, 130, 130)
	b.occupy(o, false)
	o.State = StateLogged
	list := bytes.Repeat([]byte{0xFF}, store.EffectListSize)
	copy(list, []byte{effGreaterDamage, 0, 0, 0, 30, 0, 12, 0, 0, 0, 0, 0, 0})
	ch := &store.Character{Name: "Buff", Class: 0x40, Level: 10, Map: 0, X: 130, Y: 130, Life: 100, MaxLife: 100,
		EffectList: list}
	b.enterWorld(o, ch)
	if s := effectSlotOf(o, effGreaterDamage); s == nil || s.Count != 30 || s.Value[0] != 12 {
		t.Fatalf("effect after enter world = %+v", s)
	}
}
