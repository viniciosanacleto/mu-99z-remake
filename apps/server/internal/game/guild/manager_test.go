package guild

import (
	"context"
	"testing"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/store"
)

func TestManagerPersistsAcrossLoad(t *testing.T) {
	mem, _ := store.NewMemory()
	ctx := context.Background()
	acc, _ := mem.CreateAccount(ctx, "a", "x", "")
	mem.CreateCharacter(ctx, acc.ID, "Mas", 0)
	mem.CreateCharacter(ctx, acc.ID, "Mem", 0)
	num, _, _ := mem.GuildCreate(ctx, "Owls", "Mas", nil, store.GuildMasterStatus)
	mem.GuildMemberAdd(ctx, "Owls", "Mem", store.GuildBattleMasterStatus)
	m := &Manager{byNum: map[int]*Record{}, byName: map[string]*Record{}, owner: map[string]*Record{}}
	if err := m.Load(ctx, mem); err != nil {
		t.Fatal(err)
	}
	g, slot, ok := m.Of("Mem")
	if !ok || g.Number != num || slot != 1 || g.Members[1].Status != 0x20 || g.Members[0].Name != "Mas" || g.Members[1].Server != -1 {
		t.Fatalf("loaded = %+v slot %d", g.Members[:2], slot)
	}
	if r := m.AddMember(ctx, "Owls", "Mem", 0); r != 3 {
		t.Fatalf("re-add = %d", r)
	}
	if r := m.DelMember(ctx, "Owls", "Nobody"); r != 3 {
		t.Fatalf("del missing = %d", r)
	}
}

// RivalCount is the rival plus every guild of the rival's union
// (FUN_00417470).
func TestManagerRivalCount(t *testing.T) {
	ctx := t.Context()
	m := NewManager()
	var n [3]int
	for i, name := range []string{"Alpha", "Beta", "Gamma"} {
		r, num := m.Create(ctx, name, name+"M", [32]byte{}, 0, -1)
		if r != 1 {
			t.Fatalf("create %s = %d", name, r)
		}
		n[i] = num
	}
	alpha, beta, gamma := n[0], n[1], n[2]
	if r := m.Join(ctx, 1, gamma, beta); r != 1 {
		t.Fatalf("Gamma joins Beta's union = %d", r)
	}
	if r := m.Join(ctx, 2, alpha, beta); r != 1 {
		t.Fatalf("Alpha and Beta rivals = %d", r)
	}
	tests := []struct {
		name  string
		guild int
		want  int
	}{
		{name: "rival with a union", guild: alpha, want: 2},
		{name: "rival without a union", guild: beta, want: 1},
		{name: "no rival", guild: gamma, want: 0},
		{name: "unknown guild", guild: 999, want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := m.RivalCount(tt.guild); got != tt.want {
				t.Fatalf("RivalCount = %d, want %d", got, tt.want)
			}
		})
	}
}
