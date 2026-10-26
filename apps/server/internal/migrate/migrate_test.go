package migrate

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
	"unicode/utf16"

	"github.com/jackc/pgx/v5"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/store"
)

func dataFile(t *testing.T) string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..", "data", "DB", "MuOnline.sql")
}

func TestLoadOriginalScript(t *testing.T) {
	s, err := LoadScript(dataFile(t))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	m, _ := s.Rows(ctx, "MEMB_INFO")
	if len(m) != 2 || m[0].str("memb___id") != "test" || m[0].str("memb__pwd") != "test" ||
		m[0].num("memb_guid") != 2 || m[0].str("bloc_code") != "0" || !m[0].null("addr_info") {
		t.Fatalf("MEMB_INFO %v", m)
	}
	if d, ok := m[0]["AccountExpireDate"].(time.Time); !ok || !d.Equal(sqlEpoch) {
		t.Fatalf("expire %v", m[0]["AccountExpireDate"])
	}
	dc, _ := s.Rows(ctx, "defaultclasstype")
	if len(dc) != 5 || len(dc[1].bytes("Inventory")) != 1728 || dc[1].num("Class") != 16 {
		t.Fatalf("DefaultClassType %d rows", len(dc))
	}
}

func utf16le(s string) []byte {
	u := utf16.Encode([]rune(s))
	b := []byte{0xFF, 0xFE}
	for _, c := range u {
		b = append(b, byte(c), byte(c>>8))
	}
	return b
}

func TestParseScriptValues(t *testing.T) {
	text := decodeText(utf16le("SET X\r\n" +
		"INSERT [dbo].[T] ([a], [b], [c], [d], [e], [f]) VALUES (N'it''s', -12, 0x0AFF, NULL, " +
		"CAST(0x00010002 AS SmallDateTime), CAST(0x0000000100000000 AS DateTime))\r\n" +
		"  INSERT INTO T (a) VALUES (1)\r\n"))
	s, err := ParseScript(text)
	if err != nil {
		t.Fatal(err)
	}
	r, _ := s.Rows(context.Background(), "t")
	if len(r) != 1 {
		t.Fatalf("rows %v", r)
	}
	row := r[0]
	if row.str("a") != "it's" || row.num("b") != -12 || !bytes.Equal(row.bytes("c"), []byte{0x0A, 0xFF}) || !row.null("d") {
		t.Fatalf("row %v", row)
	}
	if d := row["e"].(time.Time); !d.Equal(time.Date(1900, 1, 2, 0, 2, 0, 0, time.UTC)) {
		t.Fatalf("smalldatetime %v", d)
	}
	if d := row["f"].(time.Time); !d.Equal(time.Date(1900, 1, 2, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("datetime %v", d)
	}
}

func TestFixed(t *testing.T) {
	if b := fixed([]byte{1, 2}, 4, 0xFF); !bytes.Equal(b, []byte{1, 2, 0xFF, 0xFF}) {
		t.Fatalf("% X", b)
	}
	if b := fixed([]byte{1, 2, 3}, 2, 0); !bytes.Equal(b, []byte{1, 2}) {
		t.Fatalf("% X", b)
	}
}

// fixture is a full original database in script form.
const fixture = `
INSERT [dbo].[MEMB_INFO] ([memb_guid], [memb___id], [memb__pwd], [sno__numb], [mail_addr], [bloc_code], [AccountLevel], [AccountExpireDate], [Credits]) VALUES (1, N'Hero', N'pw1', N'111111111111111111', N'h@x', N'0', 2, CAST(0xB0000000 AS SmallDateTime), 321)
INSERT [dbo].[MEMB_INFO] ([memb_guid], [memb___id], [memb__pwd], [sno__numb], [mail_addr], [bloc_code], [AccountLevel], [AccountExpireDate]) VALUES (2, N'Mage', N'pw2', N'2', NULL, N'1', 0, CAST(0x00000000 AS SmallDateTime))
INSERT [dbo].[AccountCharacter] ([Number], [Id], [GameID1], [GameID2], [GameID3], [MoveCnt], [ExtClass], [Locked]) VALUES (1, N'Hero', NULL, N'Knight', NULL, 3, 7, 0)
INSERT [dbo].[Character] ([AccountID], [Name], [cLevel], [LevelUpPoint], [Class], [Experience], [Strength], [Dexterity], [Vitality], [Energy], [Leadership], [Inventory], [MagicList], [Money], [Life], [MaxLife], [Mana], [MaxMana], [MapNumber], [MapPosX], [MapPosY], [MapDir], [PkCount], [PkLevel], [PkTime], [CtlCode], [Quest], [EffectList], [FruitAddPoint], [FruitSubPoint], [Married], [ResetCount], [MasterResetCount], [IsNewChar]) VALUES (N'Hero', N'Knight', 150, 5, 17, 123456, 100, 50, 60, 20, 0, 0x000102, 0x1100, 99999, 200, 300, 40, 50, 2, 180, 90, 3, 0, 3, 0, 0, 0xFD, NULL, 0, 0, 0, 2, 0, 0)
INSERT [dbo].[Character] ([AccountID], [Name], [cLevel], [Class], [Inventory], [Life], [MaxLife], [Mana], [MaxMana], [MapNumber], [MapPosX], [MapPosY], [FruitAddPoint], [FruitSubPoint], [Married], [ResetCount], [MasterResetCount], [IsNewChar], [Patente], [TopDeath], [duelall]) VALUES (N'Mage', N'Wiz', 1, 0, NULL, 60, 60, 60, 60, 0, 130, 130, 0, 0, 0, 0, 0, 1, 9, 4, 0)
INSERT [dbo].[Character] ([AccountID], [Name], [cLevel], [Class], [FruitAddPoint], [FruitSubPoint], [Married], [ResetCount], [MasterResetCount], [IsNewChar]) VALUES (N'Ghost', N'Orphan', 1, 0, 0, 0, 0, 0, 0, 0)
INSERT [dbo].[warehouse] ([AccountID], [Items], [Money], [pw]) VALUES (N'Hero', 0x0001, 5000, 1234)
INSERT [dbo].[ExtWarehouse] ([AccountID], [Items], [Money], [Number]) VALUES (N'Hero', NULL, 1, 1)
INSERT [dbo].[OptionData] ([Name], [SkillKey], [GameOption], [Qkey], [Wkey], [Ekey], [ChatWindow]) VALUES (N'Knight', 0x01020304050607080900, 1, 2, 3, 4, 5)
INSERT [dbo].[T_FriendMain] ([GUID], [Name], [FriendCount], [MemoCount], [MemoTotal]) VALUES (10, N'Knight', 1, 11, 1)
INSERT [dbo].[T_FriendMain] ([GUID], [Name], [FriendCount], [MemoCount], [MemoTotal]) VALUES (11, N'Wiz', 1, 10, 0)
INSERT [dbo].[T_FriendList] ([GUID], [FriendGuid], [FriendName], [Del]) VALUES (10, 11, N'Wiz', 0)
INSERT [dbo].[T_FriendMail] ([MemoIndex], [GUID], [FriendName], [wDate], [Subject], [bRead], [Memo], [Photo], [Dir], [Act]) VALUES (11, 10, N'Wiz', CAST(0xB0000000 AS SmallDateTime), N'Hi', 1, 0x6869, 0x00, 1, 2)
INSERT [dbo].[Guild] ([G_Name], [G_Mark], [G_Score], [G_Master], [G_Notice], [Number], [G_Type], [G_Rival], [G_Union]) VALUES (N'Lions', 0x11, 30, N'Knight', N'hello', 7, 0, 0, 7)
INSERT [dbo].[GuildMember] ([Name], [G_Name], [G_Level], [G_Status]) VALUES (N'Knight', N'Lions', 1, 128)
INSERT [dbo].[T_PetItem_Info] ([ItemSerial], [Pet_Level], [Pet_Exp]) VALUES (777, 5, 1000)
INSERT [dbo].[EventEntryCount] ([Account], [Name], [BCCount], [CCCount], [DSCount], [DGCount], [ITCount], [IGCount], [LastDate]) VALUES (N'Hero', N'Knight', 1, 0, 2, 0, 0, 0, CAST(0xB0000000 AS SmallDateTime))
INSERT [dbo].[ResetData] ([Account], [Name], [ResetDay], [ResetWek], [ResetMon], [ResetDateDay], [ResetDateWek], [ResetDateMon], [MasterResetDay], [MasterResetWek], [MasterResetMon], [MasterResetDateDay], [MasterResetDateWek], [MasterResetDateMon]) VALUES (N'Hero', N'Knight', 1, 2, 3, CAST(0xFF000000 AS SmallDateTime), CAST(0xFF000000 AS SmallDateTime), CAST(0xFF000000 AS SmallDateTime), 4, 5, 6, CAST(0xFF000000 AS SmallDateTime), CAST(0xFF000000 AS SmallDateTime), CAST(0xFF000000 AS SmallDateTime))
INSERT [dbo].[ResetData] ([Account], [Name], [ResetDay], [ResetWek], [ResetMon], [ResetDateDay], [ResetDateWek], [ResetDateMon], [MasterResetDay], [MasterResetWek], [MasterResetMon], [MasterResetDateDay], [MasterResetDateWek], [MasterResetDateMon]) VALUES (N'Hero', N'Nobody', 1, 1, 1, CAST(0xFF000000 AS SmallDateTime), CAST(0xFF000000 AS SmallDateTime), CAST(0xFF000000 AS SmallDateTime), 0, 0, 0, CAST(0xFF000000 AS SmallDateTime), CAST(0xFF000000 AS SmallDateTime), CAST(0xFF000000 AS SmallDateTime))
INSERT [dbo].[RankingDuel] ([Name], [WinScore], [LoseScore]) VALUES (N'Knight', 3, NULL)
INSERT [dbo].[RankingBloodCastle] ([Name], [Score]) VALUES (N'Knight', 900)
INSERT [dbo].[RankingChaosCastle] ([Name], [Score]) VALUES (N'Gone', 12)
INSERT [dbo].[PeriodicItem] ([ItemSerial], [Time]) VALUES (4242, 1900000000)
INSERT [dbo].[GiftData] ([Name], [Index], [Count]) VALUES (N'Knight', 2, 1)
INSERT [dbo].[CustomPick] ([Name], [CustomPickList], [Money], [Jewel], [ItemExc], [ItemSet]) VALUES (N'Knight', 0x6A6F, 1, 1, 0, 1)
INSERT [dbo].[CustomJewelBank] ([AccountID], [Chaos], [Life], [Soul], [Bless], [Creation], [Guardian], [Harmony], [GemStone], [LowStone], [HighStone]) VALUES (N'Hero', 1, 2, 3, 4, 5, 6, 7, 8, 9, NULL)
INSERT [dbo].[EventGoldenArcher] ([Account], [RenaCount], [StoneCount]) VALUES (N'Hero', 7, 0)
INSERT [dbo].[GameServerInfo] ([Number], [ItemCount], [ZenCount], [AceItemCount]) VALUES (0, 5000, 0, 0)
INSERT [dbo].[MuCastle_DATA] ([MAP_SVR_GROUP], [SIEGE_START_DATE], [SIEGE_END_DATE], [SIEGE_GUILDLIST_SETTED], [SIEGE_ENDED], [CASTLE_OCCUPY], [OWNER_GUILD], [MONEY], [TAX_RATE_CHAOS], [TAX_RATE_STORE], [TAX_HUNT_ZONE]) VALUES (0, CAST(0x0000B0D800000000 AS DateTime), CAST(0x0000B0DF00000000 AS DateTime), 0, 1, 1, N'Lions', CAST(123456.0000 AS Decimal(19, 4)), 3, 2, 1000)
INSERT [dbo].[MuCastle_NPC] ([MAP_SVR_GROUP], [NPC_NUMBER], [NPC_INDEX], [NPC_DF_LEVEL], [NPC_RG_LEVEL], [NPC_MAXHP], [NPC_HP], [NPC_X], [NPC_Y], [NPC_DIR], [NPC_CREATEDATE]) VALUES (0, 277, 1, 1, 0, 800000, 500000, 93, 204, 1, CAST(0x0000B0D800000000 AS DateTime))
INSERT [dbo].[MuCastle_REG_SIEGE] ([MAP_SVR_GROUP], [REG_SIEGE_GUILD], [REG_MARKS], [IS_GIVEUP], [SEQ_NUM]) VALUES (0, N'Lions', 4, 0, 1)
`

func TestRunOnPostgres(t *testing.T) {
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
	db := fmt.Sprintf("mu_migrate_%d", time.Now().UnixNano())
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+db); err != nil {
		t.Fatal(err)
	}
	defer admin.Exec(ctx, "DROP DATABASE "+db+" WITH (FORCE)")
	url := strings.Replace(base, "/mu?", "/"+db+"?", 1)
	pg, err := store.OpenPostgres(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	src, err := ParseScript(fixture)
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
	if rep.Accounts != 2 || rep.Characters != 2 || rep.Warehouses != 2 || rep.Options != 1 || rep.Friends != 2 ||
		rep.FriendLinks != 1 || rep.Mails != 1 || rep.Guilds != 1 || rep.GuildMembers != 1 || rep.Pets != 1 || rep.EventEntries != 1 || rep.Renas != 1 || rep.CastleRows != 3 ||
		rep.Resets != 1 || rep.Rankings != 3 || rep.PeriodicItems != 1 || rep.Gifts != 1 || rep.Picks != 1 || rep.CustomRanks != 1 || rep.JewelBanks != 1 || len(rep.Skipped) != 2 {
		t.Fatalf("report %+v", rep)
	}
	// Read back through the store.
	acc, err := pg.AccountByLogin(ctx, "hero")
	if err != nil {
		t.Fatal(err)
	}
	ch, err := pg.LoadCharacter(ctx, acc.ID, "Knight")
	if err != nil {
		t.Fatal(err)
	}
	if ch.Level != 150 || ch.Class != 17 || ch.Money != 99999 || ch.Map != 2 || ch.Resets != 2 ||
		!bytes.Equal(ch.Inventory[:4], []byte{0, 1, 2, 0xFF}) || len(ch.Inventory) != 1728 || ch.Quest[0] != 0xFD {
		t.Fatalf("character %+v", ch)
	}
	list, _ := pg.ListCharacters(ctx, acc.ID)
	if len(list) != 1 || list[0].Slot != 1 {
		t.Fatalf("slots %+v", list)
	}
	castle, ok, err := pg.CastleTaxInfo(ctx, 0)
	if err != nil || !ok || !castle.Occupied || castle.Owner != "Lions" || castle.Money != 123456 || castle.TaxChaos != 3 ||
		castle.TaxHunt != 1000 || !castle.SiegeEnded || castle.EndDate.Sub(castle.StartDate) != 7*24*time.Hour {
		t.Fatalf("castle %+v %v %v", castle, ok, err)
	}
	if npcs, _ := pg.CastleNPCs(ctx, 0); len(npcs) != 1 || npcs[0].MaxHP != 800000 || npcs[0].DFLevel != 1 {
		t.Fatalf("castle npcs %+v", npcs)
	}
	if reg, ok, _ := pg.GuildMarkRegInfo(ctx, 0, "Lions"); !ok || reg.Marks != 4 || reg.Seq != 1 {
		t.Fatalf("castle registration %+v", reg)
	}
	// ResetData dates in 2078 are not in an older period: counters stay.
	// Credits and the jewel bank (CCustomXShop / CCustomJewelBank).
	if n, err := pg.Credits(ctx, "Hero"); err != nil || n != 321 {
		t.Fatalf("credits %d %v", n, err)
	}
	if v, err := pg.JewelBank(ctx, "hero"); err != nil || v != ([10]int{1, 2, 3, 4, 5, 6, 7, 8, 9, 0}) {
		t.Fatalf("jewel bank %v %v", v, err)
	}
	if c, err := pg.ResetInfo(ctx, "Hero", "Knight", false); err != nil || c != (store.ResetCounts{Day: 1, Week: 2, Month: 3}) {
		t.Fatalf("reset info %+v %v", c, err)
	}
	if c, _ := pg.ResetInfo(ctx, "Hero", "Knight", true); c != (store.ResetCounts{Day: 4, Week: 5, Month: 6}) {
		t.Fatalf("master reset info %+v", c)
	}
	if w, l, _ := pg.DuelRanking(ctx, "knight"); w != 3 || l != 0 {
		t.Fatalf("duel ranking %d %d", w, l)
	}
	if n, _ := pg.EventRanking(ctx, store.RankingBloodCastle, "Knight"); n != 900 {
		t.Fatalf("bc ranking %d", n)
	}
	if n, _ := pg.EventRanking(ctx, store.RankingChaosCastle, "Gone"); n != 12 {
		t.Fatalf("cc ranking %d", n)
	}
	if times, _ := pg.PeriodicTimes(ctx, []uint32{4242}); times[4242] != 1_900_000_000 {
		t.Fatalf("periodic %v", times)
	}
	if d, err := pg.LoadPick(ctx, "Knight"); err != nil || string(d.List[:2]) != "jo" || d.Money != 1 || d.Set != 1 || d.Exc != 0 {
		t.Fatalf("pick %+v %v", d, err)
	}
	// Custom columns of Character: Patente and the custom rankings.
	mage, _ := pg.AccountByLogin(ctx, "mage")
	if wiz, err := pg.LoadCharacter(ctx, mage.ID, "Wiz"); err != nil || wiz.Patente != 9 {
		t.Fatalf("patente %+v %v", wiz, err)
	}
	if n, _ := pg.CustomRank(ctx, "Wiz", "TopDeath"); n != 4 {
		t.Fatalf("TopDeath %d", n)
	}
	var serial int64
	conn.QueryRow(ctx, "SELECT last_value FROM item_serial_seq").Scan(&serial)
	if serial != 5000 {
		t.Fatalf("serial %d", serial)
	}
	// A second run refuses a non-empty target.
	tx2, _ := conn.Begin(ctx)
	defer tx2.Rollback(ctx)
	if _, err := Run(ctx, src, tx2); err == nil {
		t.Fatal("second run accepted")
	}
	pg.Close()
}
