package game

import (
	"bytes"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/frame"
)

// Chat (00, FUN_004d7720) and whisper (02, FUN_004d7c10). Both requests
// are "C1 size head name[10] msg"; the server terminates the message at
// packet offset 0x48 (59 characters).

const (
	chatNameOff = 3
	chatMsgOff  = 0x0D
	chatMsgEnd  = 0x48 // forced NUL
	chatPktSize = 0x49 // C1 49 00 name[10] msg[60]
)

// moveData holds the tables of move.go, teleport.go, chat.go and
// command.go, loaded at start-up.
type moveData struct {
	filter   []string // Util/Filter.txt (CFilter)
	commands map[string]command
	moves    []moveEntry // Move/Move.txt (CMove)
}

func loadMoveData(w *World) error {
	d := &moveData{commands: map[string]command{}}
	w.moveData = d
	if w.configDir == "" {
		return nil
	}
	f, err := gamedata.LoadUtilConfig(w.configDir)
	if err != nil {
		return err
	}
	for _, word := range f.Filter.Words {
		if word != "" {
			d.filter = append(d.filter, word)
		}
	}
	if d.commands, err = loadCommands(w.configDir); err != nil {
		return err
	}
	d.moves, err = loadMoves(w.configDir)
	return err
}

// gmLevel is FUN_0047db10: the GameMaster.txt level of the account and
// character, 0 otherwise. The rewrite keeps it in characters.gm_level
// (store.GMStore), loaded with the character.
func (w *World) gmLevel(o *Object) int {
	if o.P == nil || o.P.Char == nil {
		return 0
	}
	return o.P.Char.GMLevel
}

// filterText is CFilter (FUN_004601e0): every occurrence of a filtered
// word (case-sensitive strstr) is overwritten with '*'.
func filterText(msg []byte, words []string) {
	n := bytes.IndexByte(msg, 0)
	if n < 0 {
		n = len(msg)
	}
	s := msg[:n]
	for _, word := range words {
		wb := []byte(word)
		for i := 0; i+len(wb) <= len(s); {
			j := bytes.Index(s[i:], wb)
			if j < 0 {
				break
			}
			for k := 0; k < len(wb); k++ {
				s[i+j+k] = '*'
			}
			i += j + len(wb)
		}
	}
}

// cstr returns the bytes up to the first NUL.
func cstr(b []byte) []byte {
	if i := bytes.IndexByte(b, 0); i >= 0 {
		return b[:i]
	}
	return b
}

// chatBuffer copies a 00/02 request into a 0x49-byte buffer (the original
// reads the receive buffer directly) and terminates the message.
func chatBuffer(pkt []byte) []byte {
	b := make([]byte, chatPktSize+1)
	copy(b, pkt)
	b[chatMsgEnd] = 0
	return b
}

// packetChat is C1 49 00 name[10] msg[60] (FUN_004d7720).
func packetChat(name []byte, msg []byte) []byte {
	p := newPacket(frame.C1, 0x00, -1, chatPktSize)
	copy(p[3:13], name)
	copy(p[13:], msg[:min(len(msg), 60)])
	return p
}

// packetWhisper is C1 size 02 name[10] msg\0 with the message cut at 59
// characters (FUN_004d9ce0).
func packetWhisper(from string, msg []byte) []byte {
	msg = cstr(msg)
	if len(msg) > 0x3B {
		msg = msg[:0x3B]
	}
	p := newPacket(frame.C1, 0x02, -1, len(msg)+0x0E)
	putName(p[3:], from)
	copy(p[13:], msg)
	return p
}

// packetWhisperNotFound is C1 04 0C 00 (FUN_004d9fb0).
func packetWhisperNotFound() []byte { return []byte{0xC1, 0x04, 0x0C, 0x00} }

// chatBlocked reports the ChatLimitTime ban (obj+0x16c) and sends message
// 269 with the remaining minutes. The other two bans of the original
// (obj+0x254 bit 0, obj+0x20af) are not identified.
func (w *World) chatBlocked(o *Object) bool {
	if n := o.P.Char.ChatLimitTime; n != 0 {
		w.systemMessage(o, w.messageFor(o, 0x10D, n))
		return true
	}
	return false
}

// handleChat is 00 (FUN_004d7720).
func (w *World) handleChat(o *Object, pkt []byte) {
	if len(pkt) < chatMsgOff || !o.IsPlayer() || o.P.Char == nil {
		return
	}
	b := chatBuffer(pkt)
	msg := b[chatMsgOff:chatPktSize]
	if msg[0] == '@' && msg[1] == '>' {
		for i := range cstr(msg) {
			if msg[i] == '\'' {
				msg[i] = '"'
			}
		}
	}
	if info, ok := w.data.Maps[o.Map]; !ok || info.ChatEnable == 0 {
		return
	}
	if !canPlayerAct(o) {
		return
	}
	name := b[chatNameOff : chatNameOff+10]
	if string(cstr(name)) != o.P.Char.Name {
		return
	}
	w.log.Info("[General]", "account", o.P.Account.Login, "name", o.P.Char.Name, "message", string(cstr(msg)))
	if w.hooks.customMove != nil { // CustomMove.txt words (FUN_0044f430, custom_world.go)
		w.hooks.customMove(w, o, string(cstr(msg)))
	}
	if w.hooks.chatAnswer != nil {
		w.hooks.chatAnswer(w, o, string(cstr(msg)))
	}
	filterText(msg, w.moveData.filter)
	if msg[0] == '/' {
		w.runCommand(o, string(cstr(msg)))
		return
	}
	if w.chatBlocked(o) {
		return
	}
	out := packetChat(name, msg)
	switch {
	case msg[0] == '!' && mv(o).Authority > 3:
		w.noticeAll(NoticeGold, string(cstr(msg[1:])))
	case msg[0] == '~':
		if w.hooks.partyMembers != nil {
			for _, idx := range w.hooks.partyMembers(w, o) {
				w.sendIndex(idx, out)
			}
		}
	case msg[0] == '@':
		if w.hooks.guildChat == nil {
			return
		}
		switch msg[1] {
		case '>':
			w.hooks.guildChat(w, o, 2, cstr(msg[2:]))
		case '@':
			w.hooks.guildChat(w, o, 1, cstr(msg[2:]))
		default:
			w.hooks.guildChat(w, o, 0, cstr(msg[1:]))
		}
	default:
		w.send(o, out)
		w.broadcast(o, out)
	}
}

// noticeAll is FUN_004c0ab0: a C1 0D notice to every playing user.
func (w *World) noticeAll(typ byte, text string) {
	p := packetNotice(typ, 0, 0, 0, 0, 0, text)
	for i := PlayerStart; i < MaxObjects; i++ {
		if t := w.objects[i]; t.State == StatePlaying && t.IsPlayer() && canPlayerAct(t) {
			w.send(t, p)
		}
	}
}

// findPlayer is FUN_0050c6d0: a playing user by exact name.
func (w *World) findPlayer(name string) *Object {
	for i := PlayerStart; i < MaxObjects; i++ {
		t := w.objects[i]
		if t.State == StatePlaying && t.IsPlayer() && canPlayerAct(t) && t.P.Char != nil && t.P.Char.Name == name {
			return t
		}
	}
	return nil
}

// handleWhisper is 02 (FUN_004d7c10).
func (w *World) handleWhisper(o *Object, pkt []byte) {
	if len(pkt) < chatMsgOff || !o.IsPlayer() || !canPlayerAct(o) || o.P.Char == nil {
		return
	}
	b := chatBuffer(pkt)
	target := string(cstr(b[chatNameOff : chatNameOff+10]))
	if w.chatBlocked(o) {
		return
	}
	msg := b[chatMsgOff:chatPktSize]
	filterText(msg, w.moveData.filter)
	t := w.findPlayer(target)
	switch {
	case t == nil:
		if w.hooks.globalWhisper == nil || !w.hooks.globalWhisper(w, o, target, cstr(msg)) {
			w.send(o, packetWhisperNotFound())
		}
	case t == o:
		w.systemMessage(o, w.messageFor(o, 0x10E))
	default:
		w.log.Info("[Whisper]", "account", o.P.Account.Login, "name", o.P.Char.Name, "target", target,
			"message", string(cstr(msg)))
		w.deliverWhisper(t, o.P.Char.Name, msg)
	}
}

// deliverWhisper sends a whisper to t (FUN_004d9ce0); the Hub uses it for
// whispers relayed from another instance (FUN_00474f70).
func (w *World) deliverWhisper(t *Object, from string, msg []byte) {
	w.send(t, packetWhisper(from, msg))
}

// chatLimitSecond is the ChatLimitTime part of FUN_00510810: minutes are
// counted in obj+0x170 seconds; message 283 when the ban ends.
func (w *World) chatLimitSecond() {
	for i := PlayerStart; i < MaxObjects; i++ {
		o := w.objects[i]
		if o.State != StatePlaying || o.P == nil || o.P.Char == nil || o.P.Char.ChatLimitTime == 0 {
			continue
		}
		m := mv(o)
		m.ChatSeconds++
		if m.ChatSeconds > 60 {
			m.ChatSeconds = 0
			o.P.Char.ChatLimitTime--
			if o.P.Char.ChatLimitTime == 0 {
				w.systemMessage(o, w.messageFor(o, 0x11B))
			}
		}
	}
}

func registerChat(h *hooks) {
	h.worldHandlers[0x00] = (*World).handleChat
	h.worldHandlers[0x02] = (*World).handleWhisper
	h.start = append(h.start, loadMoveData)
	h.tick1000 = append(h.tick1000, (*World).chatLimitSecond)
}
