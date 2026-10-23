package game

import (
	"fmt"
	"path/filepath"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/config/mutext"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/frame"
)

// langDirs are the three message tables the original loads; the client
// language (F3 09 [0x31], obj+0x56) selects one.
var langDirs = [3]string{"Eng", "Por", "Spn"}

type messageTable [3]map[int]string

func loadMessages(dataDir string) (messageTable, error) {
	var t messageTable
	for i, lang := range langDirs {
		lines, err := mutext.LoadFile(filepath.Join(dataDir, "Lang", lang, "Message.txt"))
		if err != nil {
			return t, err
		}
		t[i] = map[int]string{}
		for _, l := range mutext.Table(lines) {
			n, err := l.Int(0)
			if err != nil || len(l.Tokens) < 2 {
				return t, fmt.Errorf("Lang/%s/Message.txt line %d: bad row", lang, l.No)
			}
			t[i][n] = l.Tokens[1]
		}
	}
	return t, nil
}

// messageFor returns message n in the player's language.
func (w *World) messageFor(o *Object, n int, args ...any) string {
	lang := 0
	if o != nil && o.P != nil && int(o.P.ClientType) < len(langDirs) {
		lang = int(o.P.ClientType)
	}
	s, ok := w.messages[lang][n]
	if !ok {
		s = w.messages[0][n]
	}
	if len(args) > 0 {
		s = fmt.Sprintf(cFormat(s), args...)
	}
	return s
}

// cFormat keeps printf verbs compatible (the files use %d / %s only).
func cFormat(s string) string { return s }

// Notice types of C1 0D (FUN_004c07f0 param_2).
const (
	NoticeGold  = 0 // golden center text
	NoticeBlue  = 1 // blue system text
	NoticeGuild = 2
)

// packetNotice is C1 size 0D type count opacity delay(u16) color(u32) speed
// text\0 with size = len(text)+0x11 (FUN_004c07f0).
func packetNotice(typ, count, opacity byte, delay uint16, color uint32, speed byte, text string) []byte {
	if len(text) > 0xFF {
		text = text[:0xFF]
	}
	n := len(text) + 0x11
	if n > 0xFF {
		n = 0xFF
		text = text[:n-0x11]
	}
	p := newPacket(frame.C1, 0x0D, -1, n)
	p[3], p[4], p[5] = typ, count, opacity
	u16le(p[6:], int(delay))
	u32le(p[8:], color)
	p[12] = speed
	copy(p[13:], text)
	return p
}

// systemMessage sends a blue notice (type 1), as most server messages do.
func (w *World) systemMessage(o *Object, text string) {
	w.send(o, packetNotice(NoticeBlue, 0, 0, 0, 0, 0, text))
}

// goldMessage sends a type 0 notice.
func (w *World) goldMessage(o *Object, text string) {
	w.send(o, packetNotice(NoticeGold, 0, 0, 0, 0, 0, text))
}
