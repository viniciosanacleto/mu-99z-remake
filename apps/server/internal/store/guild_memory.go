package store

import (
	"bytes"
	"context"
	"sort"
	"strings"
)

type memGuilds struct {
	next   int
	byNum  map[int]*Guild
	member map[string]int // lower(name) -> guild number
	seq    map[string]int // lower(name) -> insertion order
	n      int
}

func (m *Memory) guilds() *memGuilds {
	if m.gd == nil {
		m.gd = &memGuilds{byNum: map[int]*Guild{}, member: map[string]int{}, seq: map[string]int{}}
	}
	return m.gd
}

func (g *memGuilds) byName(name string) *Guild {
	for _, x := range g.byNum {
		if strings.EqualFold(x.Name, name) {
			return x
		}
	}
	return nil
}

func cloneGuild(g *Guild) Guild {
	c := *g
	c.Mark = bytes.Clone(g.Mark)
	c.Members = append([]GuildMember(nil), g.Members...)
	return c
}

func (m *Memory) LoadGuilds(context.Context) ([]Guild, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	g := m.guilds()
	var out []Guild
	for _, x := range g.byNum {
		c := cloneGuild(x)
		sort.SliceStable(c.Members, func(i, j int) bool {
			mi, mj := strings.EqualFold(c.Members[i].Name, c.Master), strings.EqualFold(c.Members[j].Name, c.Master)
			if mi != mj {
				return mi
			}
			return g.seq[strings.ToLower(c.Members[i].Name)] < g.seq[strings.ToLower(c.Members[j].Name)]
		})
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Number < out[j].Number })
	return out, nil
}

func (m *Memory) GuildCreate(_ context.Context, name, master string, mark []byte, status int) (int, int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	g := m.guilds()
	if g.byName(name) != nil {
		return 0, 1, nil
	}
	if _, ok := g.member[strings.ToLower(master)]; ok {
		return 0, 2, nil
	}
	g.next++
	x := &Guild{Number: g.next, Name: name, Master: master, Mark: append(append([]byte(nil), mark...), make([]byte, 32)...)[:32]}
	g.byNum[x.Number] = x
	m.addGuildMember(x, master, status)
	return x.Number, 0, nil
}

func (m *Memory) addGuildMember(x *Guild, name string, status int) {
	g := m.guilds()
	x.Members = append(x.Members, GuildMember{Name: name, Status: status})
	key := strings.ToLower(name)
	g.member[key] = x.Number
	g.n++
	g.seq[key] = g.n
	m.guildStat[key] = byte(status)
}

func (m *Memory) GuildDelete(_ context.Context, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	g := m.guilds()
	x := g.byName(name)
	if x == nil {
		return nil
	}
	for _, mb := range x.Members {
		delete(g.member, strings.ToLower(mb.Name))
		delete(m.guildStat, strings.ToLower(mb.Name))
	}
	delete(g.byNum, x.Number)
	return nil
}

func (m *Memory) GuildMemberAdd(_ context.Context, guild, name string, status int) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	g := m.guilds()
	if _, ok := g.member[strings.ToLower(name)]; ok {
		return false, nil
	}
	x := g.byName(guild)
	if x == nil {
		return false, ErrNotFound
	}
	m.addGuildMember(x, name, status)
	return true, nil
}

func (m *Memory) GuildMemberDelete(_ context.Context, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.removeGuildMember(name)
	return nil
}

func (m *Memory) removeGuildMember(name string) {
	g := m.guilds()
	key := strings.ToLower(name)
	num, ok := g.member[key]
	if !ok {
		return
	}
	delete(g.member, key)
	delete(m.guildStat, key)
	if x := g.byNum[num]; x != nil {
		for i, mb := range x.Members {
			if strings.EqualFold(mb.Name, name) {
				x.Members = append(x.Members[:i], x.Members[i+1:]...)
				break
			}
		}
	}
}

func (m *Memory) SetGuildMemberStatus(_ context.Context, name string, status int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	g := m.guilds()
	key := strings.ToLower(name)
	if x := g.byNum[g.member[key]]; x != nil {
		for i := range x.Members {
			if strings.EqualFold(x.Members[i].Name, name) {
				x.Members[i].Status = status
				m.guildStat[key] = byte(status)
			}
		}
	}
	return nil
}

func (m *Memory) updateGuild(name string, f func(*Guild)) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if x := m.guilds().byName(name); x != nil {
		f(x)
	}
	return nil
}

func (m *Memory) SetGuildType(_ context.Context, guild string, typ int) error {
	return m.updateGuild(guild, func(g *Guild) { g.Type = typ })
}

func (m *Memory) SetGuildNotice(_ context.Context, guild, notice string) error {
	return m.updateGuild(guild, func(g *Guild) { g.Notice = notice })
}

func (m *Memory) SetGuildScore(_ context.Context, guild string, score int) error {
	return m.updateGuild(guild, func(g *Guild) { g.Score = score })
}

func (m *Memory) SetGuildUnion(_ context.Context, union int, numbers ...int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, n := range numbers {
		if x := m.guilds().byNum[n]; x != nil {
			x.Union = union
		}
	}
	return nil
}

func (m *Memory) SetGuildRival(_ context.Context, rival int, numbers ...int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, n := range numbers {
		if x := m.guilds().byNum[n]; x != nil {
			x.Rival = rival
		}
	}
	return nil
}
