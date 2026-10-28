package game

// Illusion Temple (CIllusionTemple). In this GameServer build the class is
// an empty shell: the singleton at 0x01296100 (getter FUN_0047dbe0, also
// the menu item 0x7d3, which only constructs it) holds one vector at
// +0xd70 that is never filled, its vftable (0x54cac0) has only the
// destructor FUN_00488b80, no code reads the singleton's fields, and
// Event/IllusionTemple.dat is never opened (the path is not among the
// binary's strings). No packet, timer or spawn exists for it.
//
// The one observable behaviour is the type 5 branch of 91 (FUN_004d8430
// at 0x4d868d): the client's [4] is set to 0xFF (the "no level" result of
// the inlined entry level lookup), the player gets the F3 E4 notice with
// Message.txt 45 ("not strong enough to enter the Illusion Temple") in
// the player's language (obj+0x56), and no 91 answer is sent.

const itMsgNotStrong = 45 // 0x2d

func registerIllusiontemple(h *hooks) {
	h.eventTimeHandlers[5] = (*World).itRemainTime
}

// itRemainTime is the type 5 branch of 91.
func (w *World) itRemainTime(o *Object, p []byte) []byte {
	w.send(o, packetCustomNotice(0, 0, w.messageFor(o, itMsgNotStrong)))
	return nil
}
