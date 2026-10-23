package game

import (
	"context"
	"errors"
	"strings"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/charset"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/item"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/store"
)

// classByte is the CharSet byte 0 formula applied to a DB class
// (FUN_00473770 / FUN_00472d00).
func classByte(dbClass byte) byte {
	return (dbClass>>4)*32 - (dbClass&0xF)>>1 + dbClass*16
}

// summaryPieces converts the DataServer 5-byte equipment records (DB item
// bytes 0, 1, 7, 8, 9) into CharSet pieces.
func summaryPieces(inv []byte) [charset.Slots]charset.Piece {
	var ps [charset.Slots]charset.Piece
	for k := 0; k < charset.Slots; k++ {
		b := inv[k*store.ItemDBSize:]
		it, ok := item.DecodeDB(b)
		if !ok {
			ps[k] = charset.Piece{Index: -1}
			continue
		}
		ps[k] = charset.Piece{Index: it.Index, Level: it.Level, Excellent: it.Excellent, SetOption: it.SetOption}
	}
	return ps
}

// summaryRecords builds the 9x5-byte block of C1 32 F3 F9.
func summaryRecords(inv []byte) []byte {
	out := make([]byte, 45)
	for k := 0; k < 9; k++ {
		b := inv[k*store.ItemDBSize:]
		r := out[k*5:]
		if b[0] == 0xFF && b[7]&0x80 != 0 && b[9]&0xF0 == 0xF0 {
			for i := 0; i < 5; i++ {
				r[i] = 0xFF
			}
			continue
		}
		r[0], r[1], r[2], r[3], r[4] = b[0], b[1], b[7], b[8], b[9]
	}
	return out
}

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
	entries := make([]charListEntry, 0, len(list))
	for _, s := range list {
		maxLevel = max(maxLevel, s.Level)
		cs := charset.Build(charset.Input{DBClass: s.Class, Items: summaryPieces(s.Inventory)}, w.data.Custom)
		entries = append(entries, charListEntry{Slot: byte(s.Slot), Name: s.Name, Level: s.Level,
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
	w.send(o, packetCharList(flag, byte(o.P.Account.MoveCount), entries))
	for i, s := range list {
		if i > 4 {
			break
		}
		if s.Slot < 5 {
			w.send(o, packetCharEquipment(byte(s.Slot), summaryRecords(s.Inventory)))
		}
	}
	w.send(o, packetCharListInfo(flag, w.cfg.Int("Common", "ExperienceMultiplierConstA", 0),
		w.cfg.Int("Common", "ExperienceMultiplierConstB", 0), w.cfg.Int("Common", "MaxLevel", 0),
		w.cfg.CharDeleteMaxLevel))
}

// validName is CheckTextSymbol (FUN_00513850).
func validName(name string) bool {
	return name != "" && !strings.ContainsAny(name, " \"%'/\\_")
}

// handleCharCreate is F3 01 (FUN_004d95c0): [4..13] name, [14] class.
func (w *World) handleCharCreate(o *Object, p []byte) {
	if o.State != StateLogged || len(p) < 15 {
		return
	}
	name := cString(p[4:14])
	fail := func() { w.send(o, packetCreateResult(0, name, 0, 0, 0)) }
	if !w.cfg.CharCreateSwitch || !validName(name) {
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
				w.send(o, packetCreateResult(byte(result), name, 0, 0, 0))
				return
			}
			w.send(o, packetCreateResult(1, name, byte(slot), 1, classByte(cls)))
		}
	})
}

// badSyntax is the DataServer BadSyntax.txt check (strstr on the name).
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
		w.send(o, packetDeleteResult(2))
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
			w.send(o, packetDeleteResult(2))
			return
		}
	}
	if o.P.Account.Lock > 0 {
		w.systemMessage(o, w.messageFor(o, 0x6F2))
		w.send(o, packetDeleteResult(2))
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
				w.send(o, packetDeleteResult(1))
			} else {
				w.send(o, packetDeleteResult(0))
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

// validDBClass is the class whitelist of FUN_004cc730.
func validDBClass(c byte) bool {
	switch c {
	case 0x00, 0x01, 0x02, 0x10, 0x11, 0x12, 0x20, 0x21, 0x22, 0x30, 0x32, 0x40, 0x42,
		0x50, 0x51, 0x52, 0x60, 0x62:
		return true
	}
	return false
}
