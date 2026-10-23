package store

import (
	"bytes"
	"context"
	"sort"
	"strings"
	"time"
)

type memFriend struct {
	guid      int
	name      string
	memoCount int
	memoTotal int
}

type memFriendRel struct {
	friendGuid int
	name       string
	del        int
}

type memFriends struct {
	nextGuid int
	main     map[string]*memFriend        // lower(name)
	byGuid   map[int]*memFriend           // guid
	list     map[int]map[int]memFriendRel // guid -> friendGuid -> row
	wait     map[int]map[int]string       // guid -> friendGuid -> name
	mail     map[int]map[int]Mail         // guid -> memo index -> mail
}

func (m *Memory) friends() *memFriends {
	if m.fr == nil {
		m.fr = &memFriends{main: map[string]*memFriend{}, byGuid: map[int]*memFriend{},
			list: map[int]map[int]memFriendRel{}, wait: map[int]map[int]string{}, mail: map[int]map[int]Mail{}}
	}
	return m.fr
}

func (f *memFriends) get(name string) *memFriend { return f.main[strings.ToLower(name)] }

func (f *memFriends) remove(name string) {
	r := f.get(name)
	if r == nil {
		return
	}
	delete(f.main, strings.ToLower(name))
	delete(f.byGuid, r.guid)
	delete(f.list, r.guid)
	delete(f.wait, r.guid)
	delete(f.mail, r.guid)
	for _, l := range f.list {
		delete(l, r.guid)
	}
	for _, l := range f.wait {
		delete(l, r.guid)
	}
}

func (m *Memory) charLevel(name string) (int, bool) {
	for _, c := range m.chars {
		if strings.EqualFold(c.Name, name) {
			return c.Level, true
		}
	}
	return 0, false
}

func (m *Memory) FriendGuidCreate(_ context.Context, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	f := m.friends()
	if f.get(name) != nil {
		return nil
	}
	if _, ok := m.charLevel(name); !ok {
		return ErrNotFound
	}
	f.nextGuid++
	r := &memFriend{guid: f.nextGuid, name: name, memoCount: 10}
	f.main[strings.ToLower(name)] = r
	f.byGuid[r.guid] = r
	return nil
}

func (m *Memory) FriendMain(_ context.Context, name string) (int, int, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r := m.friends().get(name)
	if r == nil {
		return 0, 0, false, nil
	}
	return r.guid, r.memoTotal, true, nil
}

func (m *Memory) FriendNames(_ context.Context, guid int) ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []string
	for _, rel := range m.friends().list[guid] {
		out = append(out, rel.name)
	}
	sort.Strings(out)
	return out, nil
}

func (m *Memory) WaitingFriends(_ context.Context, guid int) ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []string
	for _, n := range m.friends().wait[guid] {
		out = append(out, n)
	}
	sort.Strings(out)
	return out, nil
}

func (f *memFriends) setRel(guid, friendGuid int, name string, del int) {
	if f.list[guid] == nil {
		f.list[guid] = map[int]memFriendRel{}
	}
	f.list[guid][friendGuid] = memFriendRel{friendGuid: friendGuid, name: name, del: del}
}

// WaitFriendAdd follows WZ_WaitFriendAdd.
func (m *Memory) WaitFriendAdd(_ context.Context, name, friend string) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	f := m.friends()
	u, fr := f.get(name), f.get(friend)
	if u == nil || fr == nil {
		return 0, nil
	}
	if lvl, ok := m.charLevel(friend); !ok {
		return 7, nil
	} else if lvl < 6 {
		return 6, nil
	}
	if _, ok := f.list[u.guid][fr.guid]; ok {
		return 4, nil
	}
	if rel, ok := f.list[fr.guid][u.guid]; ok {
		rel.del = 0
		f.list[fr.guid][u.guid] = rel
		return 8, nil
	}
	f.setRel(u.guid, fr.guid, fr.name, 1)
	if f.wait[fr.guid] == nil {
		f.wait[fr.guid] = map[int]string{}
	}
	f.wait[fr.guid][u.guid] = u.name
	return 1, nil
}

// FriendAdd follows WZ_FriendAdd.
func (m *Memory) FriendAdd(_ context.Context, name, friend string) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	f := m.friends()
	fr := f.get(friend)
	if fr == nil {
		return 5, nil
	}
	u := f.get(name)
	if u == nil {
		return 3, nil
	}
	if _, ok := f.list[u.guid][fr.guid]; ok {
		return 2, nil
	}
	f.setRel(u.guid, fr.guid, fr.name, 0)
	delete(f.wait[u.guid], fr.guid)
	return 1, nil
}

// WaitFriendDel follows WZ_WaitFriendDel.
func (m *Memory) WaitFriendDel(_ context.Context, name, friend string) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	f := m.friends()
	u, fr := f.get(name), f.get(friend)
	if u == nil || fr == nil {
		return 0, nil
	}
	if _, ok := f.wait[u.guid][fr.guid]; !ok {
		return 0, nil
	}
	delete(f.wait[u.guid], fr.guid)
	if rel, ok := f.list[fr.guid][u.guid]; ok {
		rel.del = 1
		f.list[fr.guid][u.guid] = rel
	}
	return 2, nil
}

// FriendDel follows WZ_FriendDel.
func (m *Memory) FriendDel(_ context.Context, name, friend string) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	f := m.friends()
	u := f.get(name)
	if u == nil {
		return 3, nil
	}
	fr := f.get(friend)
	if fr == nil {
		return 5, nil
	}
	if _, ok := f.list[u.guid][fr.guid]; !ok {
		return 2, nil
	}
	delete(f.list[u.guid], fr.guid)
	if rel, ok := f.list[fr.guid][u.guid]; ok {
		rel.del = 1
		f.list[fr.guid][u.guid] = rel
	}
	return 1, nil
}

func (m *Memory) SetFriendDel(_ context.Context, name string, del int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	f := m.friends()
	u := f.get(name)
	if u == nil {
		return nil
	}
	for k, rel := range f.list[u.guid] {
		rel.del = del
		f.list[u.guid][k] = rel
	}
	return nil
}

// WriteMail follows WZ_WriteMail plus the memo/photo update.
func (m *Memory) WriteMail(_ context.Context, ml Mail) (int, int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	f := m.friends()
	r := f.get(ml.To)
	if r == nil {
		return 2, 0, nil
	}
	if r.memoTotal > 49 {
		return 5, r.guid, nil
	}
	lvl, ok := m.charLevel(ml.To)
	if !ok {
		return 2, r.guid, nil
	}
	if lvl < 6 {
		return 6, r.guid, nil
	}
	r.memoCount++
	r.memoTotal++
	ml.Index = r.memoCount
	if ml.Date.IsZero() {
		ml.Date = time.Now()
	}
	ml.Memo = bytes.Clone(ml.Memo)
	ml.Photo = bytes.Clone(ml.Photo)
	if f.mail[r.guid] == nil {
		f.mail[r.guid] = map[int]Mail{}
	}
	f.mail[r.guid][ml.Index] = ml
	return ml.Index, r.guid, nil
}

func (m *Memory) MailList(_ context.Context, guid int) ([]Mail, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []Mail
	for _, ml := range m.friends().mail[guid] {
		out = append(out, ml)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Index < out[j].Index })
	return out, nil
}

func (m *Memory) ReadMail(_ context.Context, name string, index int) (*Mail, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	f := m.friends()
	r := f.get(name)
	if r == nil {
		return nil, ErrNotFound
	}
	ml, ok := f.mail[r.guid][index]
	if !ok {
		return nil, ErrNotFound
	}
	ml.Read = true
	f.mail[r.guid][index] = ml
	return &ml, nil
}

// DelMail follows WZ_DelMail.
func (m *Memory) DelMail(_ context.Context, name string, index int) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	f := m.friends()
	r := f.get(name)
	if r == nil {
		return 2, nil
	}
	if _, ok := f.mail[r.guid][index]; !ok {
		return 4, nil
	}
	delete(f.mail[r.guid], index)
	r.memoTotal--
	return 1, nil
}
