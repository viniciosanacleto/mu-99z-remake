package game

import (
	"context"
	"errors"
	"strings"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/character"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/charset"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/store"
)

// handleCharList is F3 00 (FUN_00475120 -> DataServer 0x01 ->
// FUN_00472d00).
func (w *World) handleCharList(o *Object) {
	if o.State != StateLogged {
		return
	}
	accID := o.P.Account.ID
	c := o.conn
	w.async(func(ctx context.Context) func() {
		list, err := w.store.ListCharacters(ctx, accID)
		return func() {
			o := w.objectOf(c)
			if o == nil || o.State != StateLogged {
				return
			}
			if err != nil {
				w.log.Error("character list", "err", err)
				w.closeClient(o, "character list failed")
				return
			}
			w.sendCharList(o, list)
		}
	})
}

func (w *World) sendCharList(o *Object, list []store.CharacterSummary) {
	maxLevel := 0
	entries := make([]character.ListEntry, 0, len(list))
	for _, s := range list {
		maxLevel = max(maxLevel, s.Level)
		cs := charset.Build(charset.Input{DBClass: s.Class, Items: character.SummaryPieces(s.Inventory)}, w.data.Custom)
		entries = append(entries, character.ListEntry{Slot: byte(s.Slot), Name: s.Name, Level: s.Level,
			CtlCode: s.CtlCode, CharSet: cs, GuildStatus: s.GuildStatus})
	}
	al := o.P.Account.Level
	if al < 0 || al > 3 {
		al = 0
	}
	flag := byte(2)
	o.P.Account.ExtClass = 0
	if w.cfg.MGCreateLevel[al] <= maxLevel {
		o.P.Account.ExtClass |= 4
		flag = 3
	}
	if w.cfg.DLCreateLevel[al] <= maxLevel {
		o.P.Account.ExtClass |= 2
		flag = 4
	}
	o.P.ClassFlag = flag
	w.send(o, character.PacketList(flag, byte(o.P.Account.MoveCount), entries))
	for i, s := range list {
		if i > 4 {
			break
		}
		if s.Slot < 5 {
			w.send(o, character.PacketEquipment(byte(s.Slot), character.SummaryRecords(s.Inventory)))
		}
	}
	w.send(o, character.PacketListInfo(flag, w.cfg.Int("Common", "ExperienceMultiplierConstA", 0),
		w.cfg.Int("Common", "ExperienceMultiplierConstB", 0), w.cfg.Int("Common", "MaxLevel", 0),
		w.cfg.CharDeleteMaxLevel))
}

// handleCharCreate is F3 01 (FUN_004d95c0): [4..13] name, [14] class.
func (w *World) handleCharCreate(o *Object, p []byte) {
	if o.State != StateLogged || len(p) < 15 {
		return
	}
	name := cString(p[4:14])
	fail := func() { w.send(o, character.PacketCreateResult(0, name, 0, 0, 0)) }
	if !w.cfg.CharCreateSwitch || !character.ValidName(name) {
		fail()
		return
	}
	cls := p[14]
	ext := o.P.Account.ExtClass
	switch cls {
	case 0x00, 0x10, 0x20:
	case 0x30:
		if ext&4 == 0 {
			fail()
			return
		}
	case 0x40:
		if ext&2 == 0 {
			fail()
			return
		}
	case 0x50:
		if ext&1 == 0 {
			fail()
			return
		}
	case 0x60:
		if ext&8 == 0 {
			fail()
			return
		}
	default:
		fail()
		return
	}
	accID := o.P.Account.ID
	c := o.conn
	w.async(func(ctx context.Context) func() {
		result, slot := store.CreateNameExists, 0
		var err error
		if !w.badSyntax(name) {
			result, slot, err = w.store.CreateCharacter(ctx, accID, name, cls)
		}
		return func() {
			o := w.objectOf(c)
			if o == nil {
				return
			}
			if err != nil {
				w.log.Error("create character", "err", err)
				result = store.CreateError
			}
			if result != store.CreateOK {
				w.send(o, character.PacketCreateResult(byte(result), name, 0, 0, 0))
				return
			}
			w.send(o, character.PacketCreateResult(1, name, byte(slot), 1, character.ClassByte(cls)))
		}
	})
}

// badSyntax is the DataServer BadSyntax.txt check (strstr on the name),
// list from config/server.toml [DataServer] BadSyntax.
func (w *World) badSyntax(name string) bool {
	for _, bad := range w.badWords {
		if bad != "" && strings.Contains(name, bad) {
			return true
		}
	}
	return false
}

// handleCharDelete is F3 02 (FUN_004d9780): [4..13] name, [14..] personal code.
func (w *World) handleCharDelete(o *Object, p []byte) {
	if o.State != StateLogged || !o.P.CharListLoaded || len(p) < 14 {
		return
	}
	if !w.cfg.CharDeleteSwitch {
		w.send(o, character.PacketDeleteResult(2))
		return
	}
	if w.cfg.PersonalCodeCheck {
		code := ""
		if len(p) > 14 {
			code = cString(p[14:min(len(p), 14+20)])
		}
		pid := o.P.Account.PersonalID
		if len(pid) > 6 {
			pid = pid[6:]
		} else {
			pid = ""
		}
		if len(code) < 7 || len(pid) < 7 || code[:7] != pid[:7] {
			w.send(o, character.PacketDeleteResult(2))
			return
		}
	}
	if o.P.Account.Lock > 0 {
		w.systemMessage(o, w.messageFor(o, 0x6F2))
		w.send(o, character.PacketDeleteResult(2))
		return
	}
	name := cString(p[4:14])
	accID := o.P.Account.ID
	c := o.conn
	w.async(func(ctx context.Context) func() {
		ok, err := w.store.DeleteCharacter(ctx, accID, name)
		return func() {
			o := w.objectOf(c)
			if o == nil {
				return
			}
			if err != nil {
				w.log.Error("delete character", "err", err)
			}
			if ok && err == nil {
				w.send(o, character.PacketDeleteResult(1))
			} else {
				w.send(o, character.PacketDeleteResult(0))
			}
		}
	})
}

// handleCharSelect is F3 03 (FUN_004d98a0 -> DataServer 0x04 ->
// FUN_00473a10).
func (w *World) handleCharSelect(o *Object, p []byte) {
	if o.State != StateLogged || len(p) < 14 {
		return
	}
	o.P.CharListLoaded = false
	name := cString(p[4:14])
	accID := o.P.Account.ID
	c := o.conn
	w.async(func(ctx context.Context) func() {
		ch, err := w.store.LoadCharacter(ctx, accID, name)
		return func() {
			o := w.objectOf(c)
			if o == nil || o.State != StateLogged {
				return
			}
			if errors.Is(err, store.ErrNotFound) || (err == nil && ch.CtlCode&1 != 0) {
				w.closeClient(o, "character not found or blocked")
				return
			}
			if err != nil {
				w.log.Error("load character", "err", err)
				w.closeClient(o, "character load failed")
				return
			}
			w.enterWorld(o, ch)
		}
	})
}
