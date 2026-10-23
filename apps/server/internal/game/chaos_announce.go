package game

import "github.com/viniciosanacleto/mu-remake/apps/server/internal/game/item"

// chaosAnnounceMix is the FUN_0042b790 type that announces: 3, the +13
// mix (86 16 with the +12 item; +10..+12 and +14/+15 never announce).
const chaosAnnounceMix = 3

// chaosAnnounceLevel is the level FUN_005120b0 pushes as a constant for
// both the CItemLevel lookup and the "+%d" of the text.
const chaosAnnounceLevel = 13

// chaosAnnounce is FUN_005120b0, called by FUN_0042b790 after a
// successful +item mix with EAX = the CItem +4 (the item *index*, read
// from the upgraded copy with movswl -0xf4(%ebp)), ECX = the player and
// the mix type on the stack. When the type is 3 and
// ChaosMixPlusItemAnnounce (CServerInfo+0x178) is set:
//   - the item index is compared with AnnounceMinLevelChaosMix
//     (CServerInfo+0x17c) — an original bug: the "level" limit is
//     checked against the index, so with the shipped 11 only the items
//     0,0..0,10 (Kris..) are never announced;
//   - the text is Message.txt 500 ("%s colocou o item %s +%d") with the
//     character name, FUN_00494a40(index, 13) (ItemLevel name for level
//     13, else the Item.txt name) and the constant 13;
//   - mode 1: a gold notice to every player of this server, each in his
//     language (FUN_004c0c70, type 0);
//   - mode 2: a gold notice to the server group through the DataServer
//     (FUN_00475b50 C1 94 21 with the group of this server's
//     MapServerInfo entry), text in the server language (CServerInfo+0x70,
//     never written: 0);
//   - any other mode sends nothing.
func (w *World) chaosAnnounce(o *Object, mixType int, it item.Item) {
	if mixType != chaosAnnounceMix {
		return
	}
	mode := w.cfg.Int("Common", "ChaosMixPlusItemAnnounce", 0)
	if mode == 0 || it.Index < w.cfg.Int("Common", "AnnounceMinLevelChaosMix", 0xd) {
		return
	}
	name := w.itemName(it.Index, chaosAnnounceLevel)
	switch mode {
	case 1:
		w.noticeAllMsg(NoticeGold, 500, o.P.Char.Name, name, chaosAnnounceLevel)
	case 2:
		w.groupNotice(NoticeGold, w.messageFor(nil, 500, o.P.Char.Name, name, chaosAnnounceLevel))
	}
}
