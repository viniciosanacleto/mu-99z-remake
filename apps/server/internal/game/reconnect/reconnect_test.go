package reconnect

import (
	"encoding/binary"
	"testing"
)

// The record keeps the FUN_004e1350 offsets.
func TestRecordLayout(t *testing.T) {
	r := &Record{Account: "acc", Name: "Hero", IP: "10.0.0.1", HWID: "HW-1", Time: 300, Party: 7,
		AutoCount: 2, AutoAmount: [5]int{1, 2, 3, 4, 5}, AutoReset: true, AutoPoints: [5]int{6, 7, 8, 9, 10},
		PartyFlags: 5, PartyPassword: "pw", AttackActive: true, AttackOffline: true, AttackBuff: 1,
		AttackSkill: 0x1234, AttackMap: 3, AttackX: 100, AttackY: 101, PickOn: true, Helper: true, PShopOpen: true,
		StoreOffline: 1, StoreType: 4}
	r.Prices[0], r.Prices[31] = 1000, 2000
	b := r.Encode()
	le := binary.LittleEndian
	if len(b) != 400 || string(b[0:3]) != "acc" || string(b[0x16:0x1a]) != "Hero" || string(b[0x21:0x29]) != "10.0.0.1" ||
		string(b[0x31:0x35]) != "HW-1" || le.Uint32(b[0x60:]) != 300 || le.Uint32(b[0x64:]) != 7 ||
		le.Uint32(b[0x68:]) != 2 || le.Uint32(b[0x7c:]) != 5 || b[0x80] != 1 || le.Uint32(b[0x94:]) != 10 ||
		le.Uint32(b[0x98:]) != 5 || string(b[0x9c:0x9e]) != "pw" || b[0xa7] != 1 || b[0xa8] != 1 ||
		le.Uint16(b[0xb0:]) != 0x1234 || b[0xb2] != 3 || b[0xb3] != 100 || b[0xb4] != 101 || b[0xb5] != 1 ||
		b[0xb6] != 0 || b[0xb7] != 1 || b[0xb8] != 1 || b[0xb9] != 1 || b[0xba] != 4 ||
		le.Uint32(b[0xbc:]) != 1000 || le.Uint32(b[0x138:]) != 2000 {
		t.Fatalf("layout % X", b[:RecordSize])
	}
	back, ok := Decode(b)
	if !ok || *back != *r {
		t.Fatalf("round trip %+v", back)
	}
	if _, ok := Decode(b[:0x100]); ok {
		t.Fatal("short data accepted")
	}
}
