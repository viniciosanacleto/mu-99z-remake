package store

import (
	"context"
	"time"
)

// FriendStore replaces the friend and mail stored procedures of the
// original database (WZ_UserGuidCreate, WZ_WaitFriendAdd, WZ_FriendAdd,
// WZ_WaitFriendDel, WZ_FriendDel, WZ_WriteMail, WZ_DelMail) and the
// DataServer queries around them (CFriend, 0x410ed0..0x413c00). Result codes
// are those of the procedures.
type FriendStore interface {
	// FriendGuidCreate is WZ_UserGuidCreate (idempotent).
	FriendGuidCreate(ctx context.Context, name string) error
	// FriendMain returns GUID and MemoTotal (ok false when there is no row).
	FriendMain(ctx context.Context, name string) (guid, memoTotal int, ok bool, err error)
	// FriendNames lists T_FriendList.FriendName of a GUID.
	FriendNames(ctx context.Context, guid int) ([]string, error)
	// WaitingFriends lists T_WaitFriend.FriendName of a GUID.
	WaitingFriends(ctx context.Context, guid int) ([]string, error)
	WaitFriendAdd(ctx context.Context, name, friend string) (int, error)
	FriendAdd(ctx context.Context, name, friend string) (int, error)
	WaitFriendDel(ctx context.Context, name, friend string) (int, error)
	FriendDel(ctx context.Context, name, friend string) (int, error)
	// SetFriendDel is "UPDATE T_FriendList SET Del=%d WHERE GUID=%d".
	SetFriendDel(ctx context.Context, name string, del int) error
	// WriteMail is WZ_WriteMail followed by the Memo/Photo update. result is
	// the procedure return: < 10 an error code, otherwise the memo index.
	WriteMail(ctx context.Context, m Mail) (result, recvGuid int, err error)
	MailList(ctx context.Context, guid int) ([]Mail, error)
	// ReadMail returns the memo and marks it read (bRead=1).
	ReadMail(ctx context.Context, name string, index int) (*Mail, error)
	DelMail(ctx context.Context, name string, index int) (int, error)
}

// Mail is a T_FriendMail row.
type Mail struct {
	Index   int
	From    string // sender (FriendName)
	To      string // receiver name (input of WriteMail)
	Date    time.Time
	Subject string
	Read    bool
	Memo    []byte
	Photo   []byte // 18 bytes (CharSet of the sender)
	Dir     byte
	Act     byte
}

// MailDate formats wDate the way the DataServer reads it into a 30-byte
// field (SQL Server smalldatetime as text).
func MailDate(t time.Time) string { return t.Format("2006-01-02 15:04:05") }
