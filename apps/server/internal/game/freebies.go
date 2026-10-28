package game

import "github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"

// CDefaultClassFreebies (Character/DefaultClassFreebies.txt, DAT_01252578,
// reader FUN_0046cb40): what a character receives the first time it
// enters the game. DGCharacterInfoRecv (FUN_00473a10) calls FUN_0046d0b0
// after the world entry packets when the DataServer 04 reply has
// IsNewChar set (packet +0x8f0); the DataServer clears the column at once
// (UPDATE Character SET IsNewChar='0').

func registerFreebies(h *hooks) {
	h.newCharacter = (*World).giveFreebies
}

const freebiesMaxMoney = 2000000000

// giveFreebies is FUN_0046d0b0, indexed by the class (obj+0xe0):
//   - InitialMoney > 0: zen += money, capped at 2,000,000,000 (also on
//     overflow), then C3 10 22 FE;
//   - InitialPoint > 0: LevelUpPoint (obj+0xe8) += points, F3 E0
//     (FUN_004dba70) and the character save (FUN_00475c30);
//   - each section 1 item of the class is created in the inventory
//     (FUN_004753e0, map 0xEB, CL = 0xEB at 0x46d274) with its options and
//     Duration seconds (0 = permanent);
//   - each section 2 effect present in EffectList.txt is added
//     (FUN_00478ae0): EffectType 0 uses Duration as is, otherwise the
//     time it ends (now + Duration) with the "load" flag set.
//
// The save is also done when no point is given: it stands for the
// DataServer clearing IsNewChar when the character is loaded.
func (w *World) giveFreebies(o *Object) {
	if o.P == nil || o.P.Char == nil {
		return
	}
	f := w.dus().freebies
	ch := o.P.Char
	if money := f.Money[o.Class]; money > 0 {
		if ch.Money+int64(money) > freebiesMaxMoney {
			ch.Money = freebiesMaxMoney
		} else {
			ch.Money += int64(money)
		}
		w.sendMoney(o)
	}
	if pts := f.Points[o.Class]; pts > 0 {
		ch.LevelUpPoints += pts
		w.send(o, w.packetCharRefresh(o))
	}
	w.saveCharacter(o, nil)
	for _, it := range f.Items {
		if it.Class != o.Class {
			continue
		}
		// Bytes, as pushed by 0x46d23a..0x46d26e.
		w.createItem(o, 0xEB, 0, 0, dropSpec{Index: it.Index, Level: it.Level & 0xFF, Dur: it.Durability & 0xFF,
			Skill: it.Skill & 0xFF, Luck: it.Luck & 0xFF, Option: it.Option & 0xFF, Exc: byte(it.Excellent),
			Set: byte(it.SetOption), Duration: max(it.Duration, 0), ToInventory: true})
	}
	for _, e := range f.Effects {
		if e.Class != o.Class {
			continue
		}
		w.giveFreebieEffect(o, e)
	}
}

func (w *World) giveFreebieEffect(o *Object, e gamedata.FreebieEffect) {
	if e.Effect < 0 || e.Effect >= 0x80 {
		return
	}
	if _, ok := w.effectTable()[e.Effect]; !ok {
		return
	}
	dur := e.Duration
	if e.Type != 0 {
		dur = int(w.now().Unix()) + e.Duration
	}
	var v [4]int
	for i := range v {
		v[i] = int(int16(e.Power[i]))
	}
	w.addEffectEx(o, e.Effect, dur, v, true, e.Type != 0)
}
