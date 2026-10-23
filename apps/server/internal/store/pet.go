package store

import "context"

// PetInfo is a T_PetItem_Info row: the level and experience of a Dark Horse
// or Dark Raven, keyed by the item serial (CItem+0xc4 / +0xc8 in the
// GameServer).
type PetInfo struct {
	Serial uint32
	Level  int
	Exp    uint32
}

// PetStore replaces the pet queries of the original DataServer:
//
//   - LoadPetInfo is DS 0x09 (FUN_0040dfc0): "SELECT Pet_Level,Pet_Exp FROM
//     T_PetItem_Info WHERE ItemSerial=%d" per requested serial; a missing row
//     is inserted ("INSERT ... VALUES (%d,%d,%d)") and answered as level 1,
//     experience 0.
//   - SavePetInfo is DS 0x34 (FUN_0040f0b0): UPDATE the row, or INSERT it
//     when the SELECT finds nothing.
//   - CreatePetInfo is the insert DS 0x07 (FUN_0040d8c0, item serial
//     creation) does for new 0x1A04/0x1A05 items when no row exists.
type PetStore interface {
	LoadPetInfo(ctx context.Context, serials []uint32) ([]PetInfo, error)
	SavePetInfo(ctx context.Context, pets []PetInfo) error
	CreatePetInfo(ctx context.Context, p PetInfo) error
}

// DefaultPetInfo is the answer DS 0x09 gives for a serial without a row.
func DefaultPetInfo(serial uint32) PetInfo { return PetInfo{Serial: serial, Level: 1} }
