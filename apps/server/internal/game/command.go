package game

import (
	"fmt"
	"strings"
	"time"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"
)

// CommandManager (FUN_00435400): "/" chat lines. Data/CommandManager.txt
// rows are Index, Switch AL0..AL3, Syntax, Delay (s), GMLevel, MinLevel,
// MaxLevel, MinReset, MaxReset, Money AL0..AL3. The record of the
// original also has three more per-account-level costs (Coin1..Coin3,
// messages 307..309) that this file never fills.

type command struct {
	Index    int
	Switch   [4]int
	Syntax   string
	Delay    int
	GMLevel  int
	MinLevel int
	MaxLevel int
	MinReset int
	MaxReset int
	Money    [4]int
}

// moveEntry is a Move/Move.txt row (CMove, 19 ints in the original).
type moveEntry struct {
	Index    int
	Name     string
	Money    int
	MinLevel int
	MaxLevel int
	MinReset int
	MaxReset int
	Switch   [4]int
	Gate     int
}

// loadCommands reads the CommandManager table of config/data/common.toml.
func loadCommands(configDir string) (map[string]command, error) {
	f, err := gamedata.LoadCommonConfig(configDir)
	if err != nil {
		return nil, err
	}
	out := map[string]command{}
	for _, r := range f.CommandManager.Rows {
		out[r.Syntax] = command{Index: r.Index, Switch: [4]int{r.AL0, r.AL1, r.AL2, r.AL3}, Syntax: r.Syntax,
			Delay: r.Delay, GMLevel: r.GMLevel, MinLevel: r.MinLevel, MaxLevel: r.MaxLevel, MinReset: r.MinReset,
			MaxReset: r.MaxReset, Money: [4]int{r.Money_AL0, r.Money_AL1, r.Money_AL2, r.Money_AL3}}
	}
	return out, nil
}

// loadMoves reads the Move table of config/data/move.toml (the /move list).
func loadMoves(configDir string) ([]moveEntry, error) {
	f, err := gamedata.LoadMoveConfig(configDir)
	if err != nil {
		return nil, err
	}
	var out []moveEntry
	for _, r := range f.Move.Rows {
		out = append(out, moveEntry{Index: r.Index, Name: r.Name, Money: r.RequireMoney, MinLevel: r.MinLevel,
			MaxLevel: r.MaxLevel, MinReset: r.MinReset, MaxReset: r.MaxReset, Switch: [4]int{r.AL0, r.AL1, r.AL2, r.AL3},
			Gate: r.GateNumber})
	}
	return out, nil
}

// commandToken is FUN_004352a0: word n of s (words split by single
// spaces; consecutive spaces count as empty words), at most 59 input
// characters, truncated to size-1 bytes.
func commandToken(s string, n, size int) string {
	if len(s) > 0x3B {
		s = s[:0x3B]
	}
	k := 0
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == ' ' {
			k++
		} else if k == n {
			b.WriteByte(s[i])
		}
	}
	out := b.String()
	if len(out) > size-1 {
		out = out[:size-1]
	}
	return out
}

// hardCodedCommands are compared before the table in FUN_00435400
// (/vendo, /compro, /attack with "off", /minhainfo, /superdrop, /combo,
// /offline, /bau with the extended warehouse); the ones without an entry
// in hardCodedHandlers are not implemented yet.
var hardCodedCommands = []string{"/vendo", "/compro", "/attack", "/minhainfo", "/superdrop", "/combo", "/offline", "/bau"}

// runCommand is the CommandManager part of FUN_00435400.
func (w *World) runCommand(o *Object, msg string) {
	name := commandToken(msg, 0, 0x20)
	arg := msg[min(len(name), len(msg)):]
	if strings.HasPrefix(arg, " ") {
		arg = arg[1:]
	}
	// The custom event join commands are compared first (strcmp with the
	// Syntax of each event file; customevent.go).
	if w.hooks.commandEvent != nil && w.hooks.commandEvent(w, o, name) {
		return
	}
	for _, h := range hardCodedCommands {
		if strings.EqualFold(name, h) {
			if f := w.hooks.hardCodedHandlers[h]; f != nil {
				f(w, o, arg)
				return
			}
			w.log.Debug("command pending", "command", name)
			return
		}
	}
	c, ok := w.moveData.commands[name]
	al := o.P.Account.Level
	if !ok || al < 0 || al > 3 || c.Switch[al] == 0 {
		return
	}
	m := mv(o)
	if m.CommandTicks == nil {
		m.CommandTicks = map[int]time.Time{}
	}
	ch := o.P.Char
	elapsed := int(w.now().Sub(m.CommandTicks[c.Index]) / time.Second)
	var msgID, val int
	switch {
	case elapsed < c.Delay:
		msgID, val = 300, c.Delay-elapsed
	case c.GMLevel != -1 && w.gmLevel(o) < c.GMLevel:
		w.systemMessage(o, w.messageFor(o, 0x12D, name))
		return
	case c.MinLevel != -1 && ch.Level < c.MinLevel:
		msgID, val = 0x12E, c.MinLevel
	case c.MaxLevel != -1 && ch.Level > c.MaxLevel:
		msgID, val = 0x12F, c.MaxLevel
	case c.MinReset != -1 && ch.Resets < c.MinReset:
		msgID, val = 0x130, c.MinReset
	case c.MaxReset != -1 && ch.Resets > c.MaxReset:
		msgID, val = 0x131, c.MaxReset
	case ch.Money < int64(c.Money[al]):
		msgID, val = 0x132, c.Money[al]
	}
	if msgID != 0 {
		w.systemMessage(o, w.messageFor(o, msgID, val, name))
		return
	}
	if w.hooks.commandScript != nil && w.hooks.commandScript(w, o, c.Index, arg) {
		w.commandDone(o, c)
		return
	}
	// CommandRequirement.txt (FUN_00440660, cmdrequirement.go): true
	// means a matching row failed and the command does not run.
	if w.hooks.commandRequirement != nil && w.hooks.commandRequirement(w, o, c.Index) {
		return
	}
	h := w.hooks.commandHandlers[c.Index]
	if h == nil {
		w.log.Debug("command pending", "command", name, "index", c.Index)
		return
	}
	h(w, o, c, arg)
}

// commandDone is FUN_00436c30: charge Money_AL<n> (and send the zen
// update), then remember the use for the Delay check.
func (w *World) commandDone(o *Object, c command) {
	al := o.P.Account.Level
	if cost := c.Money[al]; cost != 0 {
		o.P.Char.Money -= int64(cost)
		w.sendMoney(o)
	}
	mv(o).CommandTicks[c.Index] = w.now()
	w.scriptCommandDone(o, c.Index) // BridgeFunction_OnCommandDone (FUN_004e6cc0, lua.go)
}

// commandMove is /move (case 0, FUN_00436dd0 -> FUN_004beba0 + FUN_004bec30).
func (w *World) commandMove(o *Object, c command, arg string) {
	var entry *moveEntry
	for i := range w.moveData.moves {
		if strings.EqualFold(w.moveData.moves[i].Name, arg) {
			entry = &w.moveData.moves[i]
			break
		}
	}
	if entry == nil {
		return
	}
	w.moveTo(o, *entry)
	w.commandDone(o, c)
}

// moveTo is FUN_004bec30 (CMove::Move).
func (w *World) moveTo(o *Object, e moveEntry) {
	ch := o.P.Char
	al := o.P.Account.Level
	msg := func(id int, args ...any) { w.systemMessage(o, w.messageFor(o, id, args...)) }
	if e.MinLevel != -1 {
		req := e.MinLevel
		if o.Class == 3 || o.Class == 4 || o.Class == 6 {
			req = req * 2 / 3
		}
		if ch.Level < req {
			msg(0xE0, req)
			return
		}
	}
	switch {
	case e.MaxLevel != -1 && ch.Level > e.MaxLevel:
		msg(0xE6, e.MaxLevel)
		return
	case e.MinReset != -1 && ch.Resets < e.MinReset:
		msg(0xE7, e.MinReset)
		return
	case e.MaxReset != -1 && ch.Resets > e.MaxReset:
		msg(0xE8, e.MaxReset)
		return
	case ch.Money < int64(e.Money):
		// FUN_00478e90: an effect (not identified) makes the move free.
		msg(0xE1, e.Money)
		return
	case al < 0 || al > 3 || e.Switch[al] == 0:
		msg(0xE2)
		return
	case w.cfg.Int("Common", "PKLimitFree", 0) == 0 && o.PKLevel > 4 && w.cfg.Int("Common", "PKLimitMove", 0) != 0:
		msg(0xE3)
		return
	case !o.Alive || mv(o).TeleportState != 0:
		msg(0xE2)
		return
	}
	g, ok := w.data.Gates[e.Gate]
	if ok {
		pet := w.slotItem(o, 8)
		switch g.Map {
		case 7:
			if !pet.IsEmpty() && (pet.Index == 13*512+2 || pet.Index == 13*512+3) {
				msg(0xE4)
				return
			}
		case 10:
			if w.slotItem(o, 7).IsEmpty() && (pet.IsEmpty() || (pet.Index != 13*512+3 && pet.Index != 13*512+37)) {
				msg(0xE4)
				return
			}
		}
	}
	// TODO(command): the self-defense timers (obj+0x2d4.., message 265)
	// belong to the PvP module.
	if w.hooks.arenaMove != nil && !w.hooks.arenaMove(w, o, e.Gate) { // CustomArena join FUN_00443c50 (customarena.go)
		return
	}
	if w.gateMove(o, e.Gate) {
		ch.Money -= int64(e.Money)
		w.sendMoney(o)
	}
}

// commandPost is /post (case 4, FUN_00437280).
func (w *World) commandPost(o *Object, c command, arg string) {
	if w.chatBlocked(o) {
		return
	}
	name := o.P.Char.Name
	if w.cfg.Int("Command", "CommandPostGlobal", 0) != 0 {
		// The original sends C1 52 20 to the ExDB for every server.
		if w.hooks.globalPost != nil {
			w.hooks.globalPost(w, o, arg)
		} else {
			w.postLocal(name, arg)
		}
	} else {
		w.postLocal(name, arg)
	}
	w.log.Info("[CommandPost]", "account", o.P.Account.Login, "name", name, "message", arg)
	w.commandDone(o, c)
}

// postLocal broadcasts a /post line on this instance according to
// CommandPostType: 0 whisper-style C1 02 with the formatted message
// CommandPostMessage (FUN_00512cb0), 1..3 chat C1 00 prefixed with '~',
// '@' or '$' (FUN_00512f10 / FUN_00513170 / FUN_005133d0), 4..9 and
// above the custom C1 86 F3 E4 2 color "%s: %s" (colors 0..5, 6 above).
func (w *World) postLocal(name, text string) {
	typ := w.cfg.Int("Command", "CommandPostType", 0)
	msgID := w.cfg.Int("Command", "CommandPostMessage", 0)
	for i := PlayerStart; i < MaxObjects; i++ {
		t := w.objects[i]
		if t.State != StatePlaying || !t.IsPlayer() || !canPlayerAct(t) {
			continue
		}
		line := fmt.Sprintf(cFormat(w.messageFor(t, msgID)), text)
		switch typ {
		case 0:
			w.send(t, packetWhisper(name, []byte(line)))
		case 1, 2, 3:
			prefix := [...]byte{'~', '@', '$'}[typ-1]
			w.send(t, packetPostChat(name, append([]byte{prefix}, line...)))
		default:
			color := byte(6)
			if typ <= 9 {
				color = byte(typ - 4)
			}
			w.send(t, packetCustomNotice(2, color, name+": "+text))
		}
	}
}

// packetPostChat is C1 size 00 name[10] msg\0 (size len+0x0E, msg cut at
// 59 bytes), the variable-size chat of FUN_00512f10.
func packetPostChat(name string, msg []byte) []byte {
	p := packetWhisper(name, msg)
	p[2] = 0x00
	return p
}

// packetCustomNotice is the custom C1 86 F3 E4 type color text[128]
// (FUN_004dc890).
func packetCustomNotice(typ, color byte, text string) []byte {
	p := newPacket(0xC1, 0xF3, 0xE4, 0x86)
	p[4], p[5] = typ, color
	if len(text) > 0x7F {
		text = text[:0x7F]
	}
	copy(p[6:], text)
	return p
}
