package game

import (
	"context"
	"testing"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/devilsquare"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/store"
)

// eventRank waits for the async reports and reads a ranking row back.
func (b *bareWorld) eventRank(t *testing.T, event int, name string) int {
	t.Helper()
	b.runAsync()
	n, err := b.store.(store.RankingStore).EventRanking(context.Background(), event, name)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func TestRankingReports(t *testing.T) {
	b := newBareWorld(t)
	o := b.player(PlayerStart, "Ranker", 0, 130, 130)
	b.reportEventRanking(store.RankingBloodCastle, o, 40)
	b.reportEventRanking(store.RankingBloodCastle, o, 2)
	b.reportDuelRanking(o, 1, 0)
	if n := b.eventRank(t, store.RankingBloodCastle, "Ranker"); n != 42 {
		t.Fatalf("bc %d", n)
	}
	if w, l, _ := b.store.(store.RankingStore).DuelRanking(context.Background(), "Ranker"); w != 1 || l != 0 {
		t.Fatalf("duel %d %d", w, l)
	}
	// FUN_004705d0: each Devil Square user's score with the ranking window.
	s := &b.ds().sq[0]
	s.Users[0] = devilsquare.User{Index: o.Index, Score: 30}
	b.dsSendRanking(s)
	if n := b.eventRank(t, store.RankingDevilSquare, "Ranker"); n != 30 || find(b.drain(o), 0x93, -1) == nil {
		t.Fatalf("ds %d", n)
	}
	// Monsters and objects without a character report nothing.
	b.reportEventRanking(store.RankingChaosCastle, &Object{}, 5)
	if n := b.eventRank(t, store.RankingChaosCastle, ""); n != 0 {
		t.Fatalf("empty name %d", n)
	}
}
