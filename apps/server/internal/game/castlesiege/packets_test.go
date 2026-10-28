package castlesiege

import (
	"bytes"
	"encoding/binary"
	"testing"
	"time"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/store"
)

func TestPacketLayouts(t *testing.T) {
	start := time.Date(2026, 3, 4, 19, 30, 0, 0, time.Local)
	s := PacketState(StateInfo{Result: 1, State: 6, Start: start, End: start.Add(time.Hour),
		Siege: start.Add(48 * time.Hour), Owner: "Lords", Master: "King", Remain: 0x01020304})
	if len(s) != 0x2E || s[0] != 0xC1 || s[2] != 0xB2 || s[3] != 0 || s[4] != 1 || s[5] != 6 ||
		!bytes.Equal(s[6:12], []byte{0x07, 0xEA, 3, 4, 19, 30}) || !bytes.Equal(s[12:18], []byte{0x07, 0xEA, 3, 4, 20, 30}) ||
		!bytes.Equal(s[18:24], []byte{0x07, 0xEA, 3, 6, 19, 30}) || string(s[24:29]) != "Lords" ||
		string(s[32:36]) != "King" || !bytes.Equal(s[42:46], []byte{1, 2, 3, 4}) {
		t.Fatalf("B2 00 = % X", s)
	}
	cases := []struct {
		got, want []byte
	}{
		{PacketRegister(1, "Alpha"), []byte{0xC1, 0x0D, 0xB2, 0x01, 1, 'A', 'l', 'p', 'h', 'a', 0, 0, 0}},
		{PacketGiveUp(1, 1, "Ab"), []byte{0xC1, 0x0E, 0xB2, 0x02, 1, 1, 'A', 'b', 0, 0, 0, 0, 0, 0}},
		{PacketRegInfo(1, "Ab", 0x0102, true, 7), []byte{0xC1, 0x13, 0xB2, 0x03, 1, 'A', 'b', 0, 0, 0, 0, 0, 0, 0, 0, 1, 2, 1, 7}},
		{PacketRegMark(3, "", 0), []byte{0xC1, 0x11, 0xB2, 0x04, 3, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}},
		{PacketNPCBuy(1, 277, 2), []byte{0xC1, 0x10, 0xB2, 0x05, 1, 0, 0, 0, 0x15, 1, 0, 0, 2, 0, 0, 0}},
		{PacketNPCRepair(1, 283, 1, 10, 20), []byte{0xC1, 0x18, 0xB2, 0x06, 1, 0, 0, 0, 0x1B, 1, 0, 0, 1, 0, 0, 0, 10, 0, 0, 0, 20, 0, 0, 0}},
		{PacketNPCUpgrade(4, 277, 3, 1, 2), []byte{0xC1, 0x18, 0xB2, 0x07, 4, 0, 0, 0, 0x15, 1, 0, 0, 3, 0, 0, 0, 1, 0, 0, 0, 2, 0, 0, 0}},
		{PacketTaxInfo(1, 2, 3, 0x0102030405), []byte{0xC1, 0x0F, 0xB2, 0x08, 1, 2, 3, 0, 0, 0, 1, 2, 3, 4, 5}},
		{PacketTaxRate(1, 3, 300000), []byte{0xC1, 0x0A, 0xB2, 0x09, 1, 3, 0x00, 0x04, 0x93, 0xE0}},
		{PacketMoneyOut(1, 0x0A0B), []byte{0xC1, 0x0D, 0xB2, 0x10, 1, 0, 0, 0, 0, 0, 0, 0x0A, 0x0B}},
		{PacketLever(1, 0x1234), []byte{0xC1, 0x07, 0xB2, 0x11, 1, 0x12, 0x34}},
		{PacketGateOperate(1, 0, 0x0102), []byte{0xC1, 0x08, 0xB2, 0x12, 1, 0, 1, 2}},
		{PacketGateState(1, 0x0102), []byte{0xC1, 0x07, 0xB2, 0x13, 1, 1, 2}},
		{PacketSwitchUser(0x0102, -1, 1), []byte{0xC1, 0x09, 0xB2, 0x14, 1, 2, 0xFF, 0xFF, 1}},
		{PacketCrownAccess(2, 1500), []byte{0xC1, 0x0C, 0xB2, 0x15, 2, 0, 0, 0, 0xDC, 0x05, 0, 0}},
		{PacketCrownState(2), []byte{0xC1, 0x05, 0xB2, 0x16, 2}},
		{PacketCrownGuild(1, "Win"), []byte{0xC1, 0x0D, 0xB2, 0x18, 1, 'W', 'i', 'n', 0, 0, 0, 0, 0}},
		{PacketMinimapResult(1), []byte{0xC1, 0x05, 0xB2, 0x1B, 1}},
		{PacketCommand(1, 2, 3, 4), []byte{0xC1, 0x08, 0xB2, 0x1D, 1, 2, 3, 4}},
		{PacketRemainTime(2*3600000 + 5*60000 + 59999), []byte{0xC1, 0x06, 0xB2, 0x1E, 2, 5}},
		{PacketHuntZone(1, 1), []byte{0xC1, 0x06, 0xB2, 0x1F, 1, 1}},
		{PacketWeaponWindow(2, 0x0102), []byte{0xC1, 0x08, 0xB7, 0x00, 1, 2, 1, 2}},
		{PacketWeaponUse(1, 0x0102, 1, 70, 105), []byte{0xC1, 0x0A, 0xB7, 0x01, 1, 1, 2, 1, 70, 105}},
		{PacketWeaponShot(2, 9, 8), []byte{0xC1, 0x07, 0xB7, 0x02, 2, 9, 8}},
		{PacketStoneState(0x0102, 5), []byte{0xC1, 0x07, 0xB9, 0x01, 1, 2, 5}},
		{PacketHuntInfo(3, 1, 7), []byte{0xC1, 0x14, 0xB9, 0x03, 3, 1, 0, 0, 7, 0, 0, 0, 0xE0, 0x93, 0x04, 0, 0x10, 0x27, 0, 0}},
		{PacketHuntEnter(1), []byte{0xC1, 0x05, 0xB9, 0x05, 1}},
	}
	for i, c := range cases {
		if !bytes.Equal(c.got, c.want) {
			t.Errorf("case %d: % X, want % X", i, c.got, c.want)
		}
	}
	sw := PacketSwitchInfo(0x0102, true, 3, "Union", "Hero")
	if len(sw) != 0x1A || sw[3] != 0x20 || sw[4] != 1 || sw[5] != 2 || sw[6] != 1 || sw[7] != 3 ||
		string(sw[8:13]) != "Union" || string(sw[16:20]) != "Hero" {
		t.Fatalf("B2 20 = % X", sw)
	}
	var mark [32]byte
	mark[0], mark[31] = 0x11, 0x22
	if m := PacketOwnerMark(mark); len(m) != 0x24 || m[3] != 2 || m[4] != 0x11 || m[35] != 0x22 {
		t.Fatalf("B9 02 = % X", m)
	}
	l := PacketNPCList(1, []NPCListEntry{{Class: 277, Index: 1, DF: 2, RG: 0, MaxLife: 500000, Life: 400000, X: 93, Y: 204, Live: true}})
	if len(l) != 0x0C+0x1C || l[0] != 0xC2 || l[3] != 0xB3 || l[4] != 1 || l[8] != 1 ||
		binary.LittleEndian.Uint32(l[12:]) != 277 || binary.LittleEndian.Uint32(l[20:]) != 2 ||
		binary.LittleEndian.Uint32(l[28:]) != 500000 || binary.LittleEndian.Uint32(l[32:]) != 400000 ||
		l[36] != 93 || l[37] != 204 || l[38] != 1 {
		t.Fatalf("B3 = % X", l)
	}
	if l := PacketNPCList(2, nil); len(l) != 0x0C || l[4] != 2 {
		t.Fatalf("B3 fail = % X", l)
	}
	r := PacketRegList(1, []store.CastleRegGuild{{Guild: "Alpha", Marks: 0x0102, GiveUp: true, Seq: 9}})
	if len(r) != 0x0C+0x0E || r[3] != 0xB4 || r[8] != 1 || string(r[12:17]) != "Alpha" ||
		!bytes.Equal(r[20:26], []byte{0, 0, 1, 2, 1, 9}) {
		t.Fatalf("B4 = % X", r)
	}
	sl := PacketSiegeList(1, []SiegeEntry{{Side: 2, Involved: true, Guild: "Alpha"}})
	if len(sl) != 0x0C+0x0A || sl[3] != 0xB5 || sl[12] != 2 || sl[13] != 1 || string(sl[14:19]) != "Alpha" {
		t.Fatalf("B5 = % X", sl)
	}
	if p := PacketMinimapPlayers([][2]byte{{1, 2}, {3, 4}}); !bytes.Equal(p, []byte{0xC2, 0, 12, 0xB6, 2, 0, 0, 0, 1, 2, 3, 4}) {
		t.Fatalf("B6 = % X", p)
	}
	if p := PacketMinimapNPCs([]MinimapNPC{{1, 94, 227}}); !bytes.Equal(p, []byte{0xC2, 0, 11, 0xBB, 1, 0, 0, 0, 1, 94, 227}) {
		t.Fatalf("BB = % X", p)
	}
}
