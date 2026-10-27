// Package loadbot is a headless client of the original protocol used by
// cmd/muload to put fake players on the server
// (docs/server-plan/18-teste-de-carga.md). The packets mirror the ones the
// internal/game tests send for the original client.
package loadbot

import (
	"encoding/binary"
	"fmt"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/crypto"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/frame"
)

// Account returns the login and character name of load test account i
// (1-based), as muserver -seed-load creates them.
func Account(i int) (login, char string) {
	return fmt.Sprintf("load%04d", i), fmt.Sprintf("ld%04d", i)
}

// seedClasses is the -seed-load rotation of character creation codes:
// Dark Wizard, Dark Knight, Elf.
var seedClasses = []byte{0x00, 0x10, 0x20}

// AccountClass returns the creation code (F3 01 class byte) of load test
// account i; the class itself is the code >> 4.
func AccountClass(i int) byte { return seedClasses[(i-1)%len(seedClasses)] }

// Password is the password of every load test account.
const Password = "load"

// HWID is the hardware id sent in F3 09 (any 44-character dashed id).
const HWID = "ABCDEFGH-ABCDEFGH-ABCDEFGH-ABCDEFGH-ABCDEFGH"

// ActionAttack is the D7 action byte (the one monsters use, combat.go).
const ActionAttack = 0x78

func bux(s string, n int) []byte {
	b := make([]byte, n)
	copy(b, s)
	crypto.Bux(b)
	return b
}

func name10(s string) []byte {
	b := make([]byte, 10)
	copy(b, s)
	return b
}

// ServerList is C1 F4 06 (ConnectServer server list request).
func ServerList() []byte { return frame.Build(frame.C1, 0xF4, 0x06) }

// ServerInfo is C1 F4 03 code (ConnectServer address request).
func ServerInfo(code byte) []byte { return frame.Build(frame.C1, 0xF4, 0x03, code) }

// Verify is C1 F3 09: Bux HWID[45] and the client language.
func Verify(hwid string, lang byte) []byte {
	body := append([]byte{0x09}, bux(hwid, 45)...)
	return frame.Build(frame.C1, 0xF3, append(body, lang)...)
}

// Login is C3 F1 01: Bux account[10] and password[10], tick, version[5]
// and serial[16].
func Login(account, password string, tick uint32, version [5]byte, serial string) []byte {
	body := []byte{0x01}
	body = append(body, bux(account, 10)...)
	body = append(body, bux(password, 10)...)
	body = binary.LittleEndian.AppendUint32(body, tick)
	body = append(body, version[:]...)
	s := make([]byte, 16)
	copy(s, serial)
	body = append(body, s...)
	return frame.Build(frame.C3, 0xF1, body...)
}

// CharList is C1 F3 00.
func CharList() []byte { return frame.Build(frame.C1, 0xF3, 0x00) }

// SelectChar is C1 F3 03 name[10].
func SelectChar(name string) []byte {
	return frame.Build(frame.C1, 0xF3, append([]byte{0x03}, name10(name)...)...)
}

// Walk is C1 D3: start X Y, dir<<4|count, then the step directions two per
// byte, high nibble first.
func Walk(x, y int, dir byte, dirs ...byte) []byte {
	body := []byte{byte(x), byte(y), dir<<4 | byte(len(dirs))}
	for i := 0; i < len(dirs); i += 2 {
		v := dirs[i] << 4
		if i+1 < len(dirs) {
			v |= dirs[i+1]
		}
		body = append(body, v)
	}
	return frame.Build(frame.C1, 0xD3, body...)
}

// Chat is C1 00 name[10] message NUL.
func Chat(name, msg string) []byte {
	body := append(name10(name), msg...)
	return frame.Build(frame.C1, 0x00, append(body, 0)...)
}

// Attack is C1 D7 target(BE) action dir.
func Attack(target int, action, dir byte) []byte {
	return frame.Build(frame.C1, 0xD7, byte(target>>8), byte(target), action, dir)
}

// LiveClient is C3 0E: tick and the attack/magic speeds.
func LiveClient(tick uint32, attackSpeed, magicSpeed uint16) []byte {
	body := binary.LittleEndian.AppendUint32([]byte{0}, tick)
	body = binary.LittleEndian.AppendUint16(body, attackSpeed)
	body = binary.LittleEndian.AppendUint16(body, magicSpeed)
	return frame.Build(frame.C3, 0x0E, body...)
}

// dirSteps is the direction table of the walk handler (move.go dirStep).
var dirSteps = [8][2]int{{-1, -1}, {0, -1}, {1, -1}, {1, 0}, {1, 1}, {0, 1}, {-1, 1}, {-1, 0}}

// DirTo returns the direction of one step from (x, y) towards (tx, ty).
func DirTo(x, y, tx, ty int) byte {
	dx, dy := sign(tx-x), sign(ty-y)
	for d, s := range dirSteps {
		if s[0] == dx && s[1] == dy {
			return byte(d)
		}
	}
	return 0
}

func sign(v int) int {
	switch {
	case v > 0:
		return 1
	case v < 0:
		return -1
	}
	return 0
}

// Step moves (x, y) one cell in direction d.
func Step(x, y int, d byte) (int, int) {
	s := dirSteps[d&7]
	return x + s[0], y + s[1]
}

// index16 reads a big-endian object index without the flag bits.
func index16(p []byte) int { return int(p[0]&0x7F)<<8 | int(p[1]) }
