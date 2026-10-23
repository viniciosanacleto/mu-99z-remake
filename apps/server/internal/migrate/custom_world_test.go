package migrate

import (
	"context"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/store"
)

// customWorldFixture: MonsterKillCount and DailyReward rows (one of an
// unknown character / account each, one day out of range).
const customWorldFixture = `
INSERT [dbo].[MEMB_INFO] ([memb_guid], [memb___id], [memb__pwd], [sno__numb], [bloc_code], [AccountLevel]) VALUES (1, N'Hero', N'pw1', N'1', N'0', 0)
INSERT [dbo].[Character] ([AccountID], [Name], [cLevel], [Class], [FruitAddPoint], [FruitSubPoint], [Married], [ResetCount], [MasterResetCount], [IsNewChar]) VALUES (N'Hero', N'Knight', 1, 16, 0, 0, 0, 0, 0, 0)
INSERT [dbo].[MonsterKillCount] ([Name], [MonsterClass], [MonsterKillCount]) VALUES (N'Knight', 3, 7)
INSERT [dbo].[MonsterKillCount] ([Name], [MonsterClass], [MonsterKillCount]) VALUES (N'Ghost', 3, 1)
INSERT [dbo].[DailyReward] ([Acc], [Name], [Date], [Month]) VALUES (N'Hero', N'Knight', 4, 10)
INSERT [dbo].[DailyReward] ([Acc], [Name], [Date], [Month]) VALUES (N'Hero', N'Knight', 32, 10)
INSERT [dbo].[DailyReward] ([Acc], [Name], [Date], [Month]) VALUES (N'Nobody', N'X', 1, 10)
`

func TestImportCustomWorld(t *testing.T) {
	base := os.Getenv("MU_TEST_DATABASE_URL")
	if base == "" {
		t.Skip("MU_TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	admin, err := pgx.Connect(ctx, base)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close(ctx)
	db := fmt.Sprintf("mu_migrate_cw_%d", time.Now().UnixNano())
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+db); err != nil {
		t.Fatal(err)
	}
	defer admin.Exec(ctx, "DROP DATABASE "+db+" WITH (FORCE)")
	url := strings.Replace(base, "/mu?", "/"+db+"?", 1)
	pg, err := store.OpenPostgres(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pg.Close()
	src, err := ParseScript(customWorldFixture)
	if err != nil {
		t.Fatal(err)
	}
	conn, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	tx, _ := conn.Begin(ctx)
	rep, err := Run(ctx, src, tx)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	skipped := strings.Join(rep.Skipped, "\n")
	for _, want := range []string{`MonsterKillCount "Ghost"`, `DailyReward "Nobody"`, `DailyReward "Hero": day 32`} {
		if !strings.Contains(skipped, want) {
			t.Fatalf("skipped %v, want %s", rep.Skipped, want)
		}
	}
	if n, err := pg.AddMonsterKill(ctx, "knight", 3); err != nil || n != 8 {
		t.Fatalf("kill count %d %v", n, err)
	}
	if days, err := pg.DailyRewardDays(ctx, "hero", 10); err != nil || !slices.Equal(days, []int{4}) {
		t.Fatalf("daily reward %v %v", days, err)
	}
}
