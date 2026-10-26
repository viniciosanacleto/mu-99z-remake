package game

import (
	"testing"
	"time"
)

func TestLoadNotices(t *testing.T) {
	now := time.Unix(0, 0)
	ns, err := loadNotices(repoConfig(), now)
	if err != nil {
		t.Fatal(err)
	}
	por := ns[1]
	if len(por.items) == 0 || por.items[0].repeat != 360*time.Second || por.items[0].color != 100<<8 {
		t.Fatalf("por notices = %+v", por.items)
	}
}

func TestNoticeRotation(t *testing.T) {
	w := &World{now: time.Now}
	for i := range w.objects {
		w.objects[i] = newObject(i)
	}
	start := time.Unix(1000, 0)
	w.notices[0] = noticeList{items: []notice{{text: "a", repeat: time.Second}, {text: "b", repeat: time.Second}}, last: start}
	w.now = func() time.Time { return start.Add(500 * time.Millisecond) }
	w.noticeTick()
	if w.notices[0].cur != 0 {
		t.Fatal("rotated before RepeatTime")
	}
	w.now = func() time.Time { return start.Add(time.Second) }
	w.noticeTick()
	if w.notices[0].cur != 1 {
		t.Fatal("did not rotate")
	}
	w.now = func() time.Time { return start.Add(2 * time.Second) }
	w.noticeTick()
	if w.notices[0].cur != 0 {
		t.Fatal("did not wrap")
	}
}
