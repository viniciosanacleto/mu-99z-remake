// Package character holds the login and character-select rules that do
// not need the world actor: the name and class checks, the CharSet
// summaries of the character list and the F1 / F3 client packets (join,
// login, logout, list, create, delete, character info). The game package
// runs the account and character handlers on the store.
package character

import (
	"strings"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/charset"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/item"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/store"
)

// ClassByte is the CharSet byte 0 formula applied to a DB class
// (FUN_00473770 / FUN_00472d00).
func ClassByte(dbClass byte) byte {
	return (dbClass>>4)*32 - (dbClass&0xF)>>1 + dbClass*16
}

// SummaryPieces converts the DataServer 5-byte equipment records (DB item
// bytes 0, 1, 7, 8, 9) into CharSet pieces.
func SummaryPieces(inv []byte) [charset.Slots]charset.Piece {
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

// SummaryRecords builds the 9x5-byte block of C1 32 F3 F9.
func SummaryRecords(inv []byte) []byte {
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

// ValidName is CheckTextSymbol (FUN_00513850).
func ValidName(name string) bool {
	return name != "" && !strings.ContainsAny(name, " \"%'/\\_")
}

// ValidDBClass is the class whitelist of FUN_004cc730.
func ValidDBClass(c byte) bool {
	switch c {
	case 0x00, 0x01, 0x02, 0x10, 0x11, 0x12, 0x20, 0x21, 0x22, 0x30, 0x32, 0x40, 0x42,
		0x50, 0x51, 0x52, 0x60, 0x62:
		return true
	}
	return false
}
