package game

import (
	"context"
	"log/slog"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/store"
)

// Ranking reports to the DataServer (C1 24 3C duel, FUN_00476bc0; C1 20
// 3D/3E/3F Blood Castle, Chaos Castle and Devil Square scores,
// FUN_00476c50, FUN_00476ce0 and the tail of FUN_004705d0). The packets
// carry the account and character names; the DataServer adds the values
// to the RankingDuel / Ranking* row of the character.

// reportDuelRanking is FUN_00476bc0: win/lose added to RankingDuel.
func (w *World) reportDuelRanking(o *Object, win, lose int) {
	rs, _ := w.store.(store.RankingStore)
	name := bcName(o)
	if rs == nil || name == "" {
		return
	}
	w.async(func(ctx context.Context) func() {
		if err := rs.AddDuelRanking(ctx, name, win, lose); err != nil {
			slog.Warn("duel ranking", "name", name, "err", err)
		}
		return nil
	})
}

// reportEventRanking adds score to the event's ranking row (event is the
// DS head, store.RankingBloodCastle and so on).
func (w *World) reportEventRanking(event int, o *Object, score int) {
	rs, _ := w.store.(store.RankingStore)
	name := bcName(o)
	if rs == nil || name == "" {
		return
	}
	w.async(func(ctx context.Context) func() {
		if err := rs.AddEventRanking(ctx, event, name, score); err != nil {
			slog.Warn("event ranking", "event", event, "name", name, "err", err)
		}
		return nil
	})
}
