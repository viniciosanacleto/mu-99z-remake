package game

import (
	"bytes"
	"context"
	"errors"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/frame"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/store"
)

// Client options (OptionData): skill hot keys, game option flags, Q/W/E
// potion keys and the chat window state. Loaded at world entry (DataServer
// 0x08 -> FUN_00474990) and saved by the client with F3 30
// (FUN_004d9c10 -> FUN_00476370 -> DataServer 0x33).

// defaultOptions is what DataServer FUN_0040dc90 answers when the
// character has no OptionData row.
func defaultOptions() *store.Options {
	return &store.Options{SkillKeys: bytes.Repeat([]byte{0xFF}, 10),
		GameOption: 0xFF, QKey: 0xFF, WKey: 0xFF, EKey: 0xFF, ChatWindow: 6}
}

// packetOptions is C1 13 F3 30 keys[10] gameOption Q W E chatWindow.
func packetOptions(o *store.Options) []byte {
	p := newPacket(frame.C1, 0xF3, 0x30, 0x13)
	copy(p[4:14], o.SkillKeys)
	p[14], p[15], p[16], p[17], p[18] = o.GameOption, o.QKey, o.WKey, o.EKey, o.ChatWindow
	return p
}

// parseOptions reads the client's F3 30 (same layout).
func parseOptions(p []byte) (*store.Options, bool) {
	if len(p) < 0x13 {
		return nil, false
	}
	return &store.Options{SkillKeys: bytes.Clone(p[4:14]), GameOption: p[14], QKey: p[15],
		WKey: p[16], EKey: p[17], ChatWindow: p[18]}, true
}

func init() {
	enterWorldHooks = append(enterWorldHooks, func(w *World, o *Object) {
		id := o.P.Char.ID
		c := o.conn
		w.async(func(ctx context.Context) func() {
			opt, err := w.store.LoadOptions(ctx, id)
			if errors.Is(err, store.ErrNotFound) {
				opt, err = defaultOptions(), nil
			}
			return func() {
				o := w.objectOf(c)
				if o == nil || o.State != StatePlaying {
					return
				}
				if err != nil {
					w.log.Error("load options", "err", err)
					return
				}
				w.send(o, packetOptions(opt))
			}
		})
	})
	f3Handlers[0x30] = func(w *World, o *Object, p []byte) {
		opt, ok := parseOptions(p)
		if !ok || o.P.Char == nil {
			return
		}
		id := o.P.Char.ID
		w.async(func(ctx context.Context) func() {
			if err := w.store.SaveOptions(ctx, id, opt); err != nil {
				w.log.Error("save options", "err", err)
			}
			return nil
		})
	}
}
