package store

import (
	"context"
	"os"
	"testing"
	"time"
)

// testCastleStore checks the semantics of the WZ_CS_* procedures; it runs
// on the memory store and, with MU_TEST_DATABASE_URL, on PostgreSQL.
func testCastleStore(t *testing.T, s CastleStore, group int) {
	ctx := context.Background()

	// Without MuCastle_DATA the updates report 0.
	if r, _, _ := s.ModifyCastleMoney(ctx, group, 10); r != 0 {
		t.Fatalf("money without row = %d", r)
	}
	if r, _, _ := s.ModifyCastleTaxRate(ctx, group, 1, 2); r != 0 {
		t.Fatalf("tax without row = %d", r)
	}

	d, first, err := s.CastleTotalInfo(ctx, group, 7)
	if err != nil || !first {
		t.Fatalf("first total info %v %v", first, err)
	}
	today := castleDate(time.Now())
	if !d.StartDate.Equal(today) || d.EndDate.Sub(d.StartDate) != 7*24*time.Hour || d.Occupied || d.Owner != "" {
		t.Fatalf("created row %+v (today %v)", d, today)
	}
	if _, first, _ := s.CastleTotalInfo(ctx, group, 7); first {
		t.Fatal("second call created again")
	}

	// Taxes and money.
	if r, rate, _ := s.ModifyCastleTaxRate(ctx, group, 2, 3); r != 1 || rate != 3 {
		t.Fatalf("store tax %d %d", r, rate)
	}
	if r, _, _ := s.ModifyCastleTaxRate(ctx, group, 4, 3); r != 0 {
		t.Fatalf("bad tax kind %d", r)
	}
	s.ModifyCastleTaxRate(ctx, group, 1, 1)
	s.ModifyCastleTaxRate(ctx, group, 3, 300000)
	if r, m, _ := s.ModifyCastleMoney(ctx, group, 5000); r != 1 || m != 5000 {
		t.Fatalf("money +5000 = %d %d", r, m)
	}
	if r, m, _ := s.ModifyCastleMoney(ctx, group, -6000); r != 2 || m != 5000 {
		t.Fatalf("money -6000 = %d %d", r, m)
	}
	d, ok, _ := s.CastleTaxInfo(ctx, group)
	if !ok || d.Money != 5000 || d.TaxChaos != 1 || d.TaxStore != 3 || d.TaxHunt != 300000 {
		t.Fatalf("tax info %+v", d)
	}
	if r, _ := s.ResetCastleTaxInfo(ctx, group); r != 1 {
		t.Fatalf("reset tax %d", r)
	}
	if d, _, _ := s.CastleTaxInfo(ctx, group); d.Money != 0 || d.TaxChaos != 0 || d.TaxStore != 0 || d.TaxHunt != 0 {
		t.Fatalf("after reset %+v", d)
	}

	// Owner, schedule, siege end.
	start := time.Date(2026, 3, 1, 15, 0, 0, 0, time.Local)
	s.SetCastleSchedule(ctx, group, start, start.AddDate(0, 0, 7))
	s.SetCastleOwner(ctx, group, true, "Lords")
	s.SetCastleSiegeEnded(ctx, group, true)
	d, _, _ = s.CastleTaxInfo(ctx, group)
	if !d.StartDate.Equal(castleDate(start)) || !d.EndDate.Equal(castleDate(start.AddDate(0, 0, 7))) ||
		!d.Occupied || d.Owner != "Lords" || !d.SiegeEnded {
		t.Fatalf("owner/schedule %+v", d)
	}

	// Registration (WZ_CS_ReqRegAttackGuild and the marks).
	ok1 := CastleGuildCheck{Exists: true, MasterLevel: 400, Members: 20}
	cases := []struct {
		guild string
		chk   CastleGuildCheck
		want  int
	}{
		{"lords", ok1, CastleRegOwner},
		{"Ghost", CastleGuildCheck{}, CastleRegNoGuild},
		{"Low", CastleGuildCheck{Exists: true}, CastleRegMasterLevel},
		{"Empty", CastleGuildCheck{Exists: true, MasterLevel: 1}, CastleRegNoMembers},
		{"Alpha", ok1, CastleRegOK},
		{"ALPHA", ok1, CastleRegAlready},
		{"Beta", ok1, CastleRegOK},
		{"Gamma", ok1, CastleRegOK},
	}
	for _, c := range cases {
		if r, err := s.RegisterSiegeGuild(ctx, group, c.guild, c.chk); err != nil || r != c.want {
			t.Fatalf("register %s = %d %v, want %d", c.guild, r, err, c.want)
		}
	}
	if r, m, _ := s.RegisterGuildMark(ctx, group, "Alpha"); r != 1 || m != 1 {
		t.Fatalf("mark %d %d", r, m)
	}
	s.RegisterGuildMark(ctx, group, "Alpha")
	if r, _, _ := s.RegisterGuildMark(ctx, group, "Nobody"); r != 0 {
		t.Fatalf("mark unknown %d", r)
	}
	if r, m, _ := s.RegisterGuildMark(ctx, group, "Beta"); r != 1 || m != 1 {
		t.Fatalf("beta mark %d %d", r, m)
	}
	if r, del, _ := s.GiveUpSiegeGuild(ctx, group, "Beta", true); r != 1 || del != 1 {
		t.Fatalf("give up %d %d", r, del)
	}
	if r, _, _ := s.RegisterGuildMark(ctx, group, "Beta"); r != 0 {
		t.Fatalf("mark after give up %d", r)
	}
	if r, _, _ := s.GiveUpSiegeGuild(ctx, group, "Nobody", true); r != 2 {
		t.Fatalf("give up unknown %d", r)
	}
	g, ok, _ := s.GuildMarkRegInfo(ctx, group, "Alpha")
	if !ok || g.Marks != 2 || g.GiveUp || g.Seq != 1 {
		t.Fatalf("reg info %+v %v", g, ok)
	}
	all, _ := s.RegisteredSiegeGuilds(ctx, group)
	if len(all) != 3 || all[0].Guild != "Alpha" || all[2].Guild != "Gamma" || !all[1].GiveUp {
		t.Fatalf("registered %+v", all)
	}
	calc, _ := s.CalcRegGuildList(ctx, group)
	if len(calc) != 2 || calc[0].Guild != "Gamma" || calc[1].Guild != "Alpha" {
		t.Fatalf("calc list %+v", calc)
	}

	// Siege guild list.
	s.SetCastleSiegeEnded(ctx, group, false)
	list := []CastleSiegeGuild{{"Lords", 1, true, 0}, {"Gamma", 2, true, 0}, {"Friend", 2, false, 0}}
	if r, err := s.SetSiegeGuildList(ctx, group, list); err != nil || r != 0 {
		t.Fatalf("set siege list %d %v", r, err)
	}
	got, _ := s.SiegeGuildList(ctx, group)
	if len(got) != 3 {
		t.Fatalf("siege list %+v", got)
	}
	if in, _ := s.CheckSiegeGuildList(ctx, "friend"); !in {
		t.Fatal("friend should be in the siege")
	}
	if in, _ := s.CheckSiegeGuildList(ctx, "Beta"); in {
		t.Fatal("given up guild reported")
	}
	if in, _ := s.CheckSiegeGuildList(ctx, "Alpha"); !in {
		t.Fatal("registered guild not reported")
	}
	s.ResetCastleSiegeGuilds(ctx, group)
	if got, _ := s.SiegeGuildList(ctx, group); len(got) != 0 {
		t.Fatalf("reset siege list %+v", got)
	}
	s.SetSiegeGuildList(ctx, group, list)
	if r, _ := s.ResetCastleSiege(ctx, group); r != 1 {
		t.Fatalf("reset siege %d", r)
	}
	if got, _ := s.SiegeGuildList(ctx, group); len(got) != 0 {
		t.Fatalf("siege list after reset %+v", got)
	}
	s.ResetCastleRegSiege(ctx, group)
	if all, _ := s.RegisteredSiegeGuilds(ctx, group); len(all) != 0 {
		t.Fatalf("reg after reset %+v", all)
	}

	// NPCs.
	gate := CastleNPCData{Number: 277, Index: 1, MaxHP: 500000, HP: 1000, X: 93, Y: 204, Dir: 1}
	statue := CastleNPCData{Number: 283, Index: 1, MaxHP: 400000, HP: 400000, X: 94, Y: 227, Dir: 1}
	if err := s.ReplaceCastleNPCs(ctx, group, []CastleNPCData{gate, statue}); err != nil {
		t.Fatal(err)
	}
	if r, _ := s.BuyCastleNPC(ctx, group, gate); r != 4 {
		t.Fatalf("buy existing %d", r)
	}
	if r, hp, max, _ := s.RepairCastleNPC(ctx, group, 277, 1); r != 1 || hp != 500000 || max != 500000 {
		t.Fatalf("repair %d %d %d", r, hp, max)
	}
	if r, _, _, _ := s.RepairCastleNPC(ctx, group, 277, 9); r != 2 {
		t.Fatalf("repair missing %d", r)
	}
	if r, _ := s.UpgradeCastleNPC(ctx, group, 283, 1, 2, 3); r != 1 {
		t.Fatalf("upgrade rg %d", r)
	}
	if r, _ := s.UpgradeCastleNPC(ctx, group, 277, 1, 3, 800000); r != 1 {
		t.Fatalf("upgrade hp %d", r)
	}
	if r, _ := s.UpgradeCastleNPC(ctx, group, 277, 1, 9, 1); r != 2 {
		t.Fatalf("upgrade bad type %d", r)
	}
	if r, _ := s.UpgradeCastleNPC(ctx, group, 277, 7, 1, 1); r != 0 {
		t.Fatalf("upgrade missing %d", r)
	}
	s.RemoveCastleNPC(ctx, group, 283, 1)
	if r, _ := s.BuyCastleNPC(ctx, group, statue); r != 1 {
		t.Fatalf("buy %d", r)
	}
	npcs, _ := s.CastleNPCs(ctx, group)
	if len(npcs) != 2 || npcs[0].Number != 277 || npcs[0].MaxHP != 800000 || npcs[0].HP != 800000 ||
		npcs[1].Number != 283 || npcs[1].RGLevel != 0 {
		t.Fatalf("npcs %+v", npcs)
	}
}

func TestMemoryCastleStore(t *testing.T) {
	m, _ := NewMemory()
	testCastleStore(t, m, 0)
}

func TestPostgresCastleStore(t *testing.T) {
	url := os.Getenv("MU_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("MU_TEST_DATABASE_URL not set")
	}
	p, err := OpenPostgres(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	ctx := context.Background()
	group := 900 + time.Now().Nanosecond()%90
	for _, tbl := range []string{"mu_castle_data", "mu_castle_npc", "mu_castle_reg_siege", "mu_castle_siege_guildlist"} {
		p.pool.Exec(ctx, `DELETE FROM `+tbl+` WHERE map_svr_group = $1`, group)
	}
	// CheckSiegeGuildList reads every group: keep only this one.
	p.pool.Exec(ctx, `DELETE FROM mu_castle_data WHERE map_svr_group <> $1`, group)
	testCastleStore(t, p, group)
}
