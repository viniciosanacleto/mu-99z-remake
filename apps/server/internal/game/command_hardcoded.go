package game

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/frame"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/store"
)

// Hard-coded commands of FUN_00435400 compared before the CommandManager
// table, without its delay, level or zen checks: /vendo and /compro
// (FUN_0043f710), /minhainfo (FUN_004047e0), /superdrop and /combo. The
// /attack, /offline and /bau ones are in customattack.go, offline.go and
// command_player2.go.

func init() {
	hardCodedHandlers["/vendo"] = func(w *World, o *Object, arg string) { w.commandAd(o, arg, true) }
	hardCodedHandlers["/compro"] = func(w *World, o *Object, arg string) { w.commandAd(o, arg, false) }
	hardCodedHandlers["/minhainfo"] = (*World).commandMyInfo
	hardCodedHandlers["/superdrop"] = (*World).commandSuperDrop
	hardCodedHandlers["/combo"] = (*World).commandCombo
	tick1000Hooks = append(tick1000Hooks, (*World).adSecond)
}

// --- /vendo, /compro -----------------------------------------------------------------

// adState is obj+0x125 (0 off, 1 /compro, 2 /vendo), obj+0x128 (last
// send tick) and obj+0x130 (the text, 60 bytes).
type adState struct {
	Type byte
	Text string
	At   time.Time
}

// adInterval is the 3-second repeat of FUN_0043f810.
const adInterval = 3000 * time.Millisecond

// packetAd is C1 size 01 index(BE) text\0 (FUN_004d9c60): the banner over
// a character, at most 59 characters.
func packetAd(index int, text string) []byte {
	if len(text) > 0x3B {
		text = text[:0x3B]
	}
	p := newPacket(frame.C1, 0x01, -1, len(text)+6)
	p[3], p[4] = byte(index>>8), byte(index)
	copy(p[5:], text)
	return p
}

// commandAd is /vendo and /compro (FUN_0043f710): "off" clears the banner
// (sent empty) with "Anuncio acima do personagem desativado."; a text of
// up to 49 (/vendo) or 51 (/compro) characters becomes "[ VENDO %s ]" or
// "COMPRO: %s" and is shown at once; otherwise the usage notice.
func (w *World) commandAd(o *Object, arg string, sell bool) {
	ad := &custom(o).Ad
	if strings.EqualFold(arg, "off") {
		ad.Type, ad.Text = 0, ""
		w.sendAd(o)
		w.customNotice(o, 1, 0, "Anuncio acima do personagem desativado.")
		return
	}
	limit, name := 51, "/compro"
	if sell {
		limit, name = 49, "/vendo"
	}
	if arg == "" || len(arg) > limit {
		w.customNotice(o, 1, 2, "Use %s mensagem (ate %d caracteres), ou %s off.", name, limit, name)
		return
	}
	if sell {
		ad.Type, ad.Text = 2, fmt.Sprintf("[ VENDO %s ]", arg)
	} else {
		ad.Type, ad.Text = 1, fmt.Sprintf("COMPRO: %s", arg)
	}
	ad.At = w.now().Add(-adInterval)
	w.adRefresh(o)
}

// sendAd is FUN_0043f690: the banner to the owner and to the players of
// its viewport (VP entries of type 1 in state 1 or 2).
func (w *World) sendAd(o *Object) {
	p := packetAd(o.Index, custom(o).Ad.Text)
	w.send(o, p)
	for _, e := range o.VP {
		if e.Type != byte(TypePlayer) || (e.State != VPNew && e.State != VPVisible) || e.Index < 0 || e.Index >= MaxObjects {
			continue
		}
		if t := w.objects[e.Index]; t.State == StatePlaying && t.IsPlayer() && t.P.LogoutCountdown < 1 {
			w.send(t, p)
		}
	}
}

// adRefresh is FUN_0043f810: with a banner, every 3 seconds.
func (w *World) adRefresh(o *Object) {
	ad := &custom(o).Ad
	if ad.Type == 0 || o.State != StatePlaying || o.P.LogoutCountdown > 0 {
		return
	}
	if now := w.now(); now.Sub(ad.At) >= adInterval {
		ad.At = now
		w.sendAd(o)
	}
}

// adSecond is the FUN_0043f810 call of the 1-second client pass.
func (w *World) adSecond() {
	for i := PlayerStart; i < MaxObjects; i++ {
		if o := w.objects[i]; o.IsPlayer() && o.P.cust != nil {
			w.adRefresh(o)
		}
	}
}

// --- /minhainfo ----------------------------------------------------------------------

// myInfoInterval is the 3-second limit of FUN_004047e0.
const myInfoInterval = 3000 * time.Millisecond

// validAccountName is the character test of the DS handler FUN_00401740:
// digits, letters, '_' and '-'.
func validAccountName(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s) && i < 10; i++ {
		c := s[i]
		if !(c >= '0' && c <= '9' || c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c == '_' || c == '-') {
			return false
		}
	}
	return true
}

// commandMyInfo is /minhainfo (FUN_004047e0): at most once every 3
// seconds, C1 12 0F 23 index account to the DataServer; the reply
// (FUN_004048f0) shows the credits, the account type (Message.txt
// 0x2EA + level), the VIP expire date and the accumulated online time, or
// "Minha info: banco indisponivel. Tente novamente." when it failed.
func (w *World) commandMyInfo(o *Object, arg string) {
	c := custom(o)
	now := w.now()
	if !c.InfoAt.IsZero() && now.Sub(c.InfoAt) < myInfoInterval {
		return
	}
	c.InfoAt = now
	cs := w.customStore()
	login, conn := o.P.Account.Login, o.conn
	if cs == nil || !validAccountName(login) {
		w.myInfoReply(o, store.AccountInfo{}, false)
		return
	}
	w.async(func(ctx context.Context) func() {
		info, err := cs.AccountInfo(ctx, login)
		return func() {
			if o := w.objectOf(conn); o != nil && o.State == StatePlaying {
				w.myInfoReply(o, info, err == nil)
			}
		}
	})
}

// myInfoReply is FUN_004048f0.
func (w *World) myInfoReply(o *Object, info store.AccountInfo, ok bool) {
	if !ok {
		w.customNotice(o, 1, 2, "Minha info: banco indisponivel. Tente novamente.")
		return
	}
	w.customNotice(o, 1, 0, "[Minha info] %s | Saldo: %d creditos", o.P.Char.Name, uint32(info.Credits))
	al := o.P.Account.Level
	if al < 0 || al > 3 {
		al = 0
	}
	w.customNotice(o, 1, 0, "Assinatura: %s", w.messageFor(o, 0x2EA+al))
	if al > 0 {
		w.customNotice(o, 1, 0, "Validade VIP: %.19s", expireText(o.P.Account.LevelExpiresAt))
	}
	hours := uint64(info.OnlineHours) + uint64(info.SessionSeconds/3600)
	w.customNotice(o, 1, 0, "Online acumulado: %d h %d min", hours, info.SessionSeconds/60%60)
}

// --- /superdrop ----------------------------------------------------------------------

// packetSuperDrop is C1 05 F3 FA flag (FUN_00434bc0).
func packetSuperDrop(on bool) []byte {
	p := newPacket(frame.C1, 0xF3, 0xFA, 5)
	if on {
		p[4] = 1
	}
	return p
}

// superDropOn reports DAT_00772d7c: drops (23) and pick ups (22) skip
// their item and ownership restrictions.
func (w *World) superDropOn() bool { return w.custom != nil && w.custom.superDrop }

// commandSuperDrop is /superdrop: only Authority 4 or 8 (obj+0x250),
// otherwise "/superdrop: comando exclusivo para Game Master."; toggles
// the flag, sends F3 FA to every playing player (FUN_00434c30) and
// "SuperDrop ATIVADO." / "SuperDrop DESATIVADO.".
func (w *World) commandSuperDrop(o *Object, arg string) {
	if a := mv(o).Authority; a != 4 && a != 8 {
		w.customNotice(o, 1, 2, "/superdrop: comando exclusivo para Game Master.")
		return
	}
	c := w.cw()
	c.superDrop = !c.superDrop
	p := packetSuperDrop(c.superDrop)
	for i := PlayerStart; i < MaxObjects; i++ {
		if t := w.objects[i]; t.State == StatePlaying && t.IsPlayer() && t.P.LogoutCountdown < 1 {
			w.send(t, p)
		}
	}
	state := "ATIVADO"
	if !c.superDrop {
		state = "DESATIVADO"
	}
	w.customNotice(o, 1, 0, "SuperDrop %s.", state)
}

// --- /combo --------------------------------------------------------------------------

// comboState is obj+0xcc (on), +0xce (sequence length), +0xd0 (step),
// +0xd2 (the skill sequence) and +0xdc (step deadline).
type comboState struct {
	On       bool
	Len      int
	Step     int
	Skills   [3]int
	Deadline time.Time
}

// comboStepTime is the 3-second window between two skills of the custom
// combo (FUN_004f8d10).
const comboStepTime = 3000 * time.Millisecond

// packetComboOn is C1 04 F3 F8, sent when /combo turns on.
func packetComboOn() []byte { return newPacket(frame.C1, 0xF3, 0xF8, 4) }

// commandCombo is /combo [off]: a second word, or a first word other than
// "off", gives "Use apenas /combo ou /combo off."; "off" clears the combo
// ("Combo DESATIVADO."). Without words the player needs Twisting Slash
// (41) and Death Stab (43); Cyclone (22) is learned when missing (F3 11),
// then the sequence Cyclone, Twisting Slash, Death Stab is armed, C1 04
// F3 F8 is sent and "Combo ATIVADO: 1-Cyclone 2-Twisting Slash 3-Death
// Stab. Uso manual.".
func (w *World) commandCombo(o *Object, arg string) {
	first, second := commandToken(arg, 0, 0x10), commandToken(arg, 1, 0x10)
	if second != "" || (first != "" && !strings.EqualFold(first, "off")) {
		w.customNotice(o, 1, 2, "Use apenas /combo ou /combo off.")
		return
	}
	cb := &custom(o).Combo
	if first != "" {
		*cb = comboState{Skills: [3]int{-1, -1, -1}}
		w.customNotice(o, 1, 0, "Combo DESATIVADO.")
		return
	}
	if w.findSkill(o, 0x29) == nil || w.findSkill(o, 0x2B) == nil {
		w.customNotice(o, 1, 2, "Aprenda Twisting Slash e Death Stab para ativar /combo.")
		return
	}
	if w.findSkill(o, 0x16) == nil {
		slot := w.addSkill(o, 0x16, 0)
		if slot < 0 {
			w.customNotice(o, 1, 2, "Nao foi possivel adicionar Cyclone: confira requisitos e espaco de magias.")
			return
		}
		w.send(o, packetSkillChange(0xFE, slot, o.P.Skills[slot]))
	}
	*cb = comboState{On: true, Len: 3, Skills: [3]int{0x16, 0x29, 0x2B}}
	w.send(o, packetComboOn())
	w.customNotice(o, 1, 0, "Combo ATIVADO: 1-Cyclone 2-Twisting Slash 3-Death Stab. Uso manual.")
}

// customComboCheck is FUN_004f8d10, called for every skill a player
// uses: an expired step restarts; the expected skill advances (3 seconds
// for the next one) and the last one completes the combo (true); the
// first skill of the sequence restarts at step 1; any other skill resets.
func (w *World) customComboCheck(o *Object, num int) bool {
	if o.P == nil || o.P.cust == nil {
		return false
	}
	cb := &o.P.cust.Combo
	if !cb.On || cb.Len != 3 {
		return false
	}
	now := w.now()
	if cb.Step != 0 && now.After(cb.Deadline) {
		cb.Step = 0
	}
	if cb.Step < len(cb.Skills) && num == cb.Skills[cb.Step] {
		cb.Step++
		cb.Deadline = now.Add(comboStepTime)
		if cb.Step >= cb.Len {
			cb.Step, cb.Deadline = 0, time.Time{}
			return true
		}
		return false
	}
	if num == cb.Skills[0] {
		cb.Step, cb.Deadline = 1, now.Add(comboStepTime)
		return false
	}
	cb.Step, cb.Deadline = 0, time.Time{}
	return false
}

// customWeaponSkillBypass is the WizardTeam prefix of FUN_004f8800: with
// /combo on, Cyclone needs no weapon; the manual auto attack's skill
// neither.
func customWeaponSkillBypass(o *Object, num int) bool {
	if o.Type != TypePlayer || o.P == nil || o.P.cust == nil {
		return false
	}
	c := o.P.cust
	return (c.Combo.On && num == 0x16) || (c.Attack.Manual && c.Attack.Active && c.Attack.Skill == num)
}
