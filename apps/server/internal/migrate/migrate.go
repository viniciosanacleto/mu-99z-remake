package migrate

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// Report counts what was copied and what was skipped.
type Report struct {
	Accounts, Characters, Warehouses, Options int
	Friends, FriendLinks, Mails               int
	Guilds, GuildMembers, Pets                int
	EventEntries, Renas, Resets, Rankings     int
	PeriodicItems, Gifts, Picks, CustomRanks  int
	CastleRows                                int // MuCastle_* rows (castle.go)
	JewelBanks                                int // CustomJewelBank rows
	Skipped                                   []string
}

func (r *Report) skip(format string, args ...any) {
	r.Skipped = append(r.Skipped, fmt.Sprintf(format, args...))
}

// Sizes of the original blobs (internal/store/migrations/0001_init.sql).
const (
	inventorySize = 1728
	magicSize     = 180
	effectSize    = 208
	questSize     = 50
	vaultSize     = 1920
	markSize      = 32
)

// Run copies the original tables into an empty target schema inside tx.
func Run(ctx context.Context, src Source, tx pgx.Tx) (*Report, error) {
	rep := &Report{}
	var n int
	if err := tx.QueryRow(ctx, "SELECT COUNT(*) FROM accounts").Scan(&n); err != nil {
		return nil, err
	}
	if n > 0 {
		return nil, fmt.Errorf("target database already has %d accounts", n)
	}
	accounts, err := importAccounts(ctx, src, tx, rep)
	if err != nil {
		return nil, err
	}
	chars, err := importCharacters(ctx, src, tx, accounts, rep)
	if err != nil {
		return nil, err
	}
	steps := []func() error{
		func() error { return importWarehouses(ctx, src, tx, accounts, rep) },
		func() error { return importOptions(ctx, src, tx, chars, rep) },
		func() error { return importFriends(ctx, src, tx, chars, rep) },
		func() error { return importGuilds(ctx, src, tx, chars, rep) },
		func() error { return importPets(ctx, src, tx, rep) },
		func() error { return importEventEntries(ctx, src, tx, chars, rep) },
		func() error { return importGoldenArcher(ctx, src, tx, accounts, rep) },
		func() error { return importResetData(ctx, src, tx, chars, rep) },
		func() error { return importRankings(ctx, src, tx, rep) },
		func() error { return importPeriodicItems(ctx, src, tx, rep) },
		func() error { return importCustomCommands(ctx, src, tx, chars, rep) },
		func() error { return importJewelBank(ctx, src, tx, accounts, rep) },
		func() error { return importCastle(ctx, src, tx, rep) },
		func() error { return importMonsterKillCount(ctx, src, tx, chars, rep) },
		func() error { return importDailyReward(ctx, src, tx, accounts, rep) },
		func() error { return importSerial(ctx, src, tx) },
	}
	for _, f := range steps {
		if err := f(); err != nil {
			return nil, err
		}
	}
	return rep, nil
}

func rows(ctx context.Context, src Source, table string) ([]Row, error) {
	r, err := src.Rows(ctx, table)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", table, err)
	}
	return r, nil
}

func key(s string) string { return strings.ToLower(s) }

// importAccounts: MEMB_INFO (+ AccountCharacter's ExtClass, Locked and
// MoveCnt). Passwords stay in clear text ("plain:", JoinServer.ini
// MD5Encryption = 0); the auth module rehashes them on the next login.
func importAccounts(ctx context.Context, src Source, tx pgx.Tx, rep *Report) (map[string]int64, error) {
	members, err := rows(ctx, src, "MEMB_INFO")
	if err != nil {
		return nil, err
	}
	ac, err := rows(ctx, src, "AccountCharacter")
	if err != nil {
		return nil, err
	}
	extra := map[string]Row{}
	for _, r := range ac {
		extra[key(r.str("Id"))] = r
	}
	ids := map[string]int64{}
	for _, r := range members {
		login := r.str("memb___id")
		if login == "" || ids[key(login)] != 0 {
			rep.skip("MEMB_INFO %q: empty or duplicate login", login)
			continue
		}
		var expires *time.Time
		if t, ok := r.time("AccountExpireDate"); ok && t.After(sqlEpoch) {
			expires = &t
		}
		created := time.Now()
		if t, ok := r.time("appl_days"); ok {
			created = t
		}
		x := extra[key(login)]
		var id int64
		// Credits is the WizardTeam column of CCustomXShop and /minhainfo
		// (ISNULL(Credits,0) in the DataServer); absent in the schema dump.
		err := tx.QueryRow(ctx, `INSERT INTO accounts (login, password_hash, personal_id, email, block_code,
			account_level, level_expires_at, lock_code, ext_class, move_count, created_at, credits)
			VALUES ($1,$2,$3,NULLIF($4,''),$5,$6,$7,$8,$9,$10,$11,$12) RETURNING id`,
			login, "plain:"+r.str("memb__pwd"), r.str("sno__numb"), r.str("mail_addr"), r.num("bloc_code"),
			r.num("AccountLevel"), expires, x.num("Locked"), x.num("ExtClass"), x.num("MoveCnt"), created,
			max(r.num("Credits"), 0)).Scan(&id)
		if err != nil {
			return nil, fmt.Errorf("account %s: %w", login, err)
		}
		ids[key(login)] = id
		rep.Accounts++
	}
	return ids, nil
}

type charRef struct {
	ID      int64
	Account int64
}

// importCharacters: Character, with the slot taken from AccountCharacter
// GameID1..5 (first free slot otherwise).
func importCharacters(ctx context.Context, src Source, tx pgx.Tx, accounts map[string]int64, rep *Report) (map[string]charRef, error) {
	list, err := rows(ctx, src, "Character")
	if err != nil {
		return nil, err
	}
	ac, err := rows(ctx, src, "AccountCharacter")
	if err != nil {
		return nil, err
	}
	slotOf := map[string]int{}
	for _, r := range ac {
		for i := 1; i <= 5; i++ {
			if n := r.str(fmt.Sprintf("GameID%d", i)); n != "" {
				slotOf[key(r.str("Id"))+"/"+key(n)] = i - 1
			}
		}
	}
	used := map[int64]map[int]bool{}
	out := map[string]charRef{}
	for _, r := range list {
		name, login := r.str("Name"), r.str("AccountID")
		acc, ok := accounts[key(login)]
		if !ok {
			rep.skip("Character %q: account %q not found", name, login)
			continue
		}
		if used[acc] == nil {
			used[acc] = map[int]bool{}
		}
		slot, ok := slotOf[key(login)+"/"+key(name)]
		if !ok || used[acc][slot] {
			slot = -1
			for s := 0; s < 5; s++ {
				if !used[acc][s] {
					slot = s
					break
				}
			}
		}
		if slot < 0 {
			rep.skip("Character %q: account %q has no free slot", name, login)
			continue
		}
		used[acc][slot] = true
		var created, last *time.Time
		if t, ok := r.time("MDate"); ok {
			created = &t
		}
		if t, ok := r.time("LDate"); ok {
			last = &t
		}
		var id int64
		err := tx.QueryRow(ctx, `INSERT INTO characters (account_id, slot, name, class, level, level_up_points,
			experience, strength, dexterity, vitality, energy, leadership, money, life, max_life, mana, max_mana,
			bp, max_bp, shield, max_shield, map, pos_x, pos_y, dir, pk_count, pk_level, pk_time, ctl_code,
			inventory, magic_list, effect_list, quest, chat_limit_time, fruit_add_points, fruit_sub_points,
			married, marry_couple, resets, master_resets, is_new, created_at, last_login_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,
			$26,$27,$28,$29,$30,$31,$32,$33,$34,$35,$36,$37,NULLIF($38,''),$39,$40,$41,COALESCE($42,now()),$43)
			RETURNING id`,
			acc, slot, name, r.num("Class"), r.num("cLevel"), r.num("LevelUpPoint"), r.num("Experience"),
			r.num("Strength"), r.num("Dexterity"), r.num("Vitality"), r.num("Energy"), r.num("Leadership"),
			r.num("Money"), r.float("Life"), r.float("MaxLife"), r.float("Mana"), r.float("MaxMana"),
			r.float("BP"), r.float("MaxBP"), r.float("Shield"), r.float("MaxShield"),
			r.num("MapNumber"), r.num("MapPosX"), r.num("MapPosY"), r.num("MapDir"),
			r.num("PkCount"), r.num("PkLevel"), r.num("PkTime"), r.num("CtlCode"),
			fixed(r.bytes("Inventory"), inventorySize, 0xFF), fixed(r.bytes("MagicList"), magicSize, 0xFF),
			fixed(r.bytes("EffectList"), effectSize, 0xFF), fixed(r.bytes("Quest"), questSize, 0xFF),
			r.num("ChatLimitTime"), r.num("FruitAddPoint"), r.num("FruitSubPoint"), r.num("Married") != 0,
			r.str("MarryCouple"), r.num("ResetCount"), r.num("MasterResetCount"), r.num("IsNewChar") != 0,
			created, last).Scan(&id)
		if err != nil {
			return nil, fmt.Errorf("character %s: %w", name, err)
		}
		out[key(name)] = charRef{ID: id, Account: acc}
		rep.Characters++
		if err := importCustomCharacterColumns(ctx, tx, id, name, r, rep); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// importWarehouses: warehouse (number 0) and ExtWarehouse (its Number).
func importWarehouses(ctx context.Context, src Source, tx pgx.Tx, accounts map[string]int64, rep *Report) error {
	for _, table := range []string{"warehouse", "ExtWarehouse"} {
		list, err := rows(ctx, src, table)
		if err != nil {
			return err
		}
		for _, r := range list {
			acc, ok := accounts[key(r.str("AccountID"))]
			if !ok {
				rep.skip("%s of %q: account not found", table, r.str("AccountID"))
				continue
			}
			number := int64(0)
			if table == "ExtWarehouse" {
				number = r.num("Number")
			}
			if _, err := tx.Exec(ctx, `INSERT INTO warehouses (account_id, number, items, money, password)
				VALUES ($1,$2,$3,$4,$5) ON CONFLICT (account_id, number) DO NOTHING`,
				acc, number, fixed(r.bytes("Items"), vaultSize, 0xFF), r.num("Money"), r.num("pw")); err != nil {
				return fmt.Errorf("%s %s: %w", table, r.str("AccountID"), err)
			}
			rep.Warehouses++
		}
	}
	return nil
}

// importOptions: OptionData (F3 30 skill keys and options).
func importOptions(ctx context.Context, src Source, tx pgx.Tx, chars map[string]charRef, rep *Report) error {
	list, err := rows(ctx, src, "OptionData")
	if err != nil {
		return err
	}
	for _, r := range list {
		c, ok := chars[key(r.str("Name"))]
		if !ok {
			rep.skip("OptionData %q: character not found", r.str("Name"))
			continue
		}
		if _, err := tx.Exec(ctx, `INSERT INTO character_options (character_id, skill_keys, game_option,
			q_key, w_key, e_key, chat_window) VALUES ($1,$2,$3,$4,$5,$6,$7)`,
			c.ID, fixed(r.bytes("SkillKey"), 10, 0xFF), r.num("GameOption"), r.num("Qkey"), r.num("Wkey"),
			r.num("Ekey"), r.num("ChatWindow")); err != nil {
			return fmt.Errorf("OptionData %s: %w", r.str("Name"), err)
		}
		rep.Options++
	}
	return nil
}

// importFriends: T_FriendMain (GUID kept), T_FriendList, T_WaitFriend,
// T_FriendMail.
func importFriends(ctx context.Context, src Source, tx pgx.Tx, chars map[string]charRef, rep *Report) error {
	mains, err := rows(ctx, src, "T_FriendMain")
	if err != nil {
		return err
	}
	guids := map[int64]bool{}
	for _, r := range mains {
		if _, ok := chars[key(r.str("Name"))]; !ok {
			rep.skip("T_FriendMain %q: character not found", r.str("Name"))
			continue
		}
		if _, err := tx.Exec(ctx, `INSERT INTO friend_main (guid, name, friend_count, memo_count, memo_total)
			VALUES ($1,$2,$3,$4,$5)`, r.num("GUID"), r.str("Name"), r.num("FriendCount"), r.num("MemoCount"),
			r.num("MemoTotal")); err != nil {
			return fmt.Errorf("T_FriendMain %s: %w", r.str("Name"), err)
		}
		guids[r.num("GUID")] = true
		rep.Friends++
	}
	for _, table := range []string{"T_FriendList", "T_WaitFriend"} {
		list, err := rows(ctx, src, table)
		if err != nil {
			return err
		}
		for _, r := range list {
			g, f := r.num("GUID"), r.num("FriendGuid")
			if !guids[g] || !guids[f] {
				rep.skip("%s %d -> %d: unknown GUID", table, g, f)
				continue
			}
			sql := `INSERT INTO wait_friend (guid, friend_guid, friend_name) VALUES ($1,$2,$3) ON CONFLICT DO NOTHING`
			args := []any{g, f, r.str("FriendName")}
			if table == "T_FriendList" {
				sql = `INSERT INTO friend_list (guid, friend_guid, friend_name, del) VALUES ($1,$2,$3,$4) ON CONFLICT DO NOTHING`
				args = append(args, r.num("Del"))
			}
			if _, err := tx.Exec(ctx, sql, args...); err != nil {
				return fmt.Errorf("%s: %w", table, err)
			}
			rep.FriendLinks++
		}
	}
	mails, err := rows(ctx, src, "T_FriendMail")
	if err != nil {
		return err
	}
	for _, r := range mails {
		if !guids[r.num("GUID")] {
			rep.skip("T_FriendMail %d: unknown GUID %d", r.num("MemoIndex"), r.num("GUID"))
			continue
		}
		date := time.Now()
		if t, ok := r.time("wDate"); ok {
			date = t
		}
		subject := r.str("Subject")
		if len(subject) > 32 {
			subject = subject[:32]
		}
		if _, err := tx.Exec(ctx, `INSERT INTO friend_mail (memo_index, guid, friend_name, w_date, subject,
			b_read, memo, photo, dir, act) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
			r.num("MemoIndex"), r.num("GUID"), r.str("FriendName"), date, subject, r.num("bRead"),
			nonNil(r.bytes("Memo")), nonNil(r.bytes("Photo")), r.num("Dir"), r.num("Act")); err != nil {
			return fmt.Errorf("T_FriendMail %d: %w", r.num("MemoIndex"), err)
		}
		rep.Mails++
	}
	_, err = tx.Exec(ctx, `SELECT setval('friend_main_guid_seq', GREATEST((SELECT COALESCE(MAX(guid),0) FROM friend_main),1))`)
	return err
}

func nonNil(b []byte) []byte {
	if b == nil {
		return []byte{}
	}
	return b
}

// importGuilds: Guild (id = Number so G_Rival / G_Union keep pointing at
// the right guild) and GuildMember.
func importGuilds(ctx context.Context, src Source, tx pgx.Tx, chars map[string]charRef, rep *Report) error {
	list, err := rows(ctx, src, "Guild")
	if err != nil {
		return err
	}
	ids := map[string]int64{}
	for _, r := range list {
		name := r.str("G_Name")
		if _, err := tx.Exec(ctx, `INSERT INTO guilds (id, name, mark, score, master, notice, type, rival, union_id)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`, r.num("Number"), name, fixed(r.bytes("G_Mark"), markSize, 0),
			r.num("G_Score"), r.str("G_Master"), r.str("G_Notice"), r.num("G_Type"), r.num("G_Rival"),
			r.num("G_Union")); err != nil {
			return fmt.Errorf("Guild %s: %w", name, err)
		}
		ids[key(name)] = r.num("Number")
		rep.Guilds++
	}
	members, err := rows(ctx, src, "GuildMember")
	if err != nil {
		return err
	}
	for _, r := range members {
		g, ok := ids[key(r.str("G_Name"))]
		if !ok {
			rep.skip("GuildMember %q: guild %q not found", r.str("Name"), r.str("G_Name"))
			continue
		}
		if _, ok := chars[key(r.str("Name"))]; !ok {
			rep.skip("GuildMember %q: character not found", r.str("Name"))
			continue
		}
		if _, err := tx.Exec(ctx, `INSERT INTO guild_members (name, guild_id, level, status) VALUES ($1,$2,$3,$4)`,
			r.str("Name"), g, r.num("G_Level"), r.num("G_Status")); err != nil {
			return fmt.Errorf("GuildMember %s: %w", r.str("Name"), err)
		}
		rep.GuildMembers++
	}
	_, err = tx.Exec(ctx, `SELECT setval('guilds_id_seq', GREATEST((SELECT COALESCE(MAX(id),0) FROM guilds),1))`)
	return err
}

// importPets: T_PetItem_Info (Dark Horse / Dark Raven level and
// experience by item serial).
func importPets(ctx context.Context, src Source, tx pgx.Tx, rep *Report) error {
	list, err := rows(ctx, src, "T_PetItem_Info")
	if err != nil {
		return err
	}
	for _, r := range list {
		if _, err := tx.Exec(ctx, `INSERT INTO pet_item_info (item_serial, pet_level, pet_exp) VALUES ($1,$2,$3)
			ON CONFLICT (item_serial) DO NOTHING`, r.num("ItemSerial"), r.num("Pet_Level"), r.num("Pet_Exp")); err != nil {
			return fmt.Errorf("T_PetItem_Info %d: %w", r.num("ItemSerial"), err)
		}
		rep.Pets++
	}
	return nil
}

// importEventEntries: EventEntryCount (today's BC/CC/DS/DG/IT/IG entries).
func importEventEntries(ctx context.Context, src Source, tx pgx.Tx, chars map[string]charRef, rep *Report) error {
	list, err := rows(ctx, src, "EventEntryCount")
	if err != nil {
		return err
	}
	for _, r := range list {
		if _, ok := chars[key(r.str("Name"))]; !ok {
			rep.skip("EventEntryCount %q: character not found", r.str("Name"))
			continue
		}
		last := time.Now()
		if t, ok := r.time("LastDate"); ok {
			last = t
		}
		if _, err := tx.Exec(ctx, `INSERT INTO event_entry_count (account, name, bc, cc, ds, dg, it, ig, last_date)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9) ON CONFLICT DO NOTHING`, r.str("Account"), r.str("Name"),
			r.num("BCCount"), r.num("CCCount"), r.num("DSCount"), r.num("DGCount"), r.num("ITCount"),
			r.num("IGCount"), last); err != nil {
			return fmt.Errorf("EventEntryCount %s: %w", r.str("Name"), err)
		}
		rep.EventEntries++
	}
	return nil
}

// importGoldenArcher: EventGoldenArcher (Renas per account).
func importGoldenArcher(ctx context.Context, src Source, tx pgx.Tx, accounts map[string]int64, rep *Report) error {
	list, err := rows(ctx, src, "EventGoldenArcher")
	if err != nil {
		return err
	}
	for _, r := range list {
		if _, ok := accounts[key(r.str("Account"))]; !ok {
			rep.skip("EventGoldenArcher %q: account not found", r.str("Account"))
			continue
		}
		if _, err := tx.Exec(ctx, `INSERT INTO golden_archer (account, rena_count, stone_count) VALUES ($1,$2,$3)
			ON CONFLICT DO NOTHING`, r.str("Account"), r.num("RenaCount"), r.num("StoneCount")); err != nil {
			return fmt.Errorf("EventGoldenArcher %s: %w", r.str("Account"), err)
		}
		rep.Renas++
	}
	return nil
}

// importResetData: ResetData (reset and master reset counters per day,
// week and month; migration 0010_reset_data.sql). A missing date becomes
// now, as a new row of WZ_GetResetInfo.
func importResetData(ctx context.Context, src Source, tx pgx.Tx, chars map[string]charRef, rep *Report) error {
	list, err := rows(ctx, src, "ResetData")
	if err != nil {
		return err
	}
	now := time.Now()
	date := func(r Row, col string) time.Time {
		if t, ok := r.time(col); ok {
			return t
		}
		return now
	}
	for _, r := range list {
		if _, ok := chars[key(r.str("Name"))]; !ok {
			rep.skip("ResetData %q: character not found", r.str("Name"))
			continue
		}
		if _, err := tx.Exec(ctx, `INSERT INTO reset_data (account, name, reset_day, reset_wek, reset_mon,
			reset_date_day, reset_date_wek, reset_date_mon, master_reset_day, master_reset_wek, master_reset_mon,
			master_date_day, master_date_wek, master_date_mon)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14) ON CONFLICT DO NOTHING`,
			r.str("Account"), r.str("Name"), r.num("ResetDay"), r.num("ResetWek"), r.num("ResetMon"),
			date(r, "ResetDateDay"), date(r, "ResetDateWek"), date(r, "ResetDateMon"),
			r.num("MasterResetDay"), r.num("MasterResetWek"), r.num("MasterResetMon"),
			date(r, "MasterResetDateDay"), date(r, "MasterResetDateWek"), date(r, "MasterResetDateMon")); err != nil {
			return fmt.Errorf("ResetData %s: %w", r.str("Name"), err)
		}
		rep.Resets++
	}
	return nil
}

// importRankings: RankingDuel and the event rankings (migration
// 0011_ranking.sql, event = the DS head that feeds the table). Rows are kept
// even when the character no longer exists, as in the original tables.
func importRankings(ctx context.Context, src Source, tx pgx.Tx, rep *Report) error {
	duel, err := rows(ctx, src, "RankingDuel")
	if err != nil {
		return err
	}
	for _, r := range duel {
		if r.str("Name") == "" {
			continue
		}
		if _, err := tx.Exec(ctx, `INSERT INTO ranking_duel (name, win_score, lose_score) VALUES ($1,$2,$3)
			ON CONFLICT (name) DO UPDATE SET win_score = ranking_duel.win_score + $2,
			lose_score = ranking_duel.lose_score + $3`, r.str("Name"), r.num("WinScore"), r.num("LoseScore")); err != nil {
			return fmt.Errorf("RankingDuel %s: %w", r.str("Name"), err)
		}
		rep.Rankings++
	}
	for _, t := range []struct {
		table string
		event int
	}{
		{"RankingBloodCastle", 0x3D}, {"RankingChaosCastle", 0x3E}, {"RankingDevilSquare", 0x3F},
		{"RankingIllusionTemple", 0x40},
	} {
		list, err := rows(ctx, src, t.table)
		if err != nil {
			return err
		}
		for _, r := range list {
			if r.str("Name") == "" {
				continue
			}
			if _, err := tx.Exec(ctx, `INSERT INTO ranking_event (event, name, score) VALUES ($1,$2,$3)
				ON CONFLICT (event, name) DO UPDATE SET score = ranking_event.score + $3`,
				t.event, r.str("Name"), r.num("Score")); err != nil {
				return fmt.Errorf("%s %s: %w", t.table, r.str("Name"), err)
			}
			rep.Rankings++
		}
	}
	return nil
}

// importPeriodicItems: PeriodicItem (expiry time by item serial, migration
// 0012_periodic_item.sql). A NULL time is 0, which expires the item.
func importPeriodicItems(ctx context.Context, src Source, tx pgx.Tx, rep *Report) error {
	list, err := rows(ctx, src, "PeriodicItem")
	if err != nil {
		return err
	}
	for _, r := range list {
		if _, err := tx.Exec(ctx, `INSERT INTO periodic_item (item_serial, time) VALUES ($1,$2)
			ON CONFLICT (item_serial) DO NOTHING`, r.num("ItemSerial"), r.num("Time")); err != nil {
			return fmt.Errorf("PeriodicItem %d: %w", r.num("ItemSerial"), err)
		}
		rep.PeriodicItems++
	}
	return nil
}

// importCustomCommands: GiftData (/gift counts) and CustomPick (/pick
// filters and switches), migration 0013_custom_commands.sql. Only rows of
// imported characters are kept; a NULL CustomPickList is empty.
func importCustomCommands(ctx context.Context, src Source, tx pgx.Tx, chars map[string]charRef, rep *Report) error {
	gifts, err := rows(ctx, src, "GiftData")
	if err != nil {
		return err
	}
	for _, r := range gifts {
		if _, ok := chars[key(r.str("Name"))]; !ok {
			rep.skip("GiftData %q: character not found", r.str("Name"))
			continue
		}
		if _, err := tx.Exec(ctx, `INSERT INTO gift_data (name, idx, count) VALUES ($1,$2,$3) ON CONFLICT DO NOTHING`,
			r.str("Name"), r.num("Index"), r.num("Count")); err != nil {
			return fmt.Errorf("GiftData %s: %w", r.str("Name"), err)
		}
		rep.Gifts++
	}
	picks, err := rows(ctx, src, "CustomPick")
	if err != nil {
		return err
	}
	for _, r := range picks {
		if _, ok := chars[key(r.str("Name"))]; !ok {
			rep.skip("CustomPick %q: character not found", r.str("Name"))
			continue
		}
		if _, err := tx.Exec(ctx, `INSERT INTO custom_pick (name, list, money, jewel, item_exc, item_set)
			VALUES ($1,$2,$3,$4,$5,$6) ON CONFLICT DO NOTHING`, r.str("Name"), nonNil(r.bytes("CustomPickList")),
			r.num("Money"), r.num("Jewel"), r.num("ItemExc"), r.num("ItemSet")); err != nil {
			return fmt.Errorf("CustomPick %s: %w", r.str("Name"), err)
		}
		rep.Picks++
	}
	return nil
}

// customRankColumns are the WizardTeam ranking columns of Character raised
// by DS 0x41 (custom_rank.rank keeps the column name the events use).
var customRankColumns = []string{"TopDeath", "TopTheft", "TopRoleta", "TopDupla", "topsurvivor",
	"Duelsm", "Duelbk", "Duelelf", "Duelmg", "Dueldl", "Duelall"}

// importCustomCharacterColumns copies the custom columns a WizardTeam
// database adds to Character (absent from the shipped MuOnline.sql):
// Patente and the ranking columns, matched without case.
func importCustomCharacterColumns(ctx context.Context, tx pgx.Tx, id int64, name string, r Row, rep *Report) error {
	col := func(want string) (any, bool) {
		for k, v := range r {
			if strings.EqualFold(k, want) {
				return v, v != nil
			}
		}
		return nil, false
	}
	if v, ok := col("Patente"); ok {
		if _, err := tx.Exec(ctx, `UPDATE characters SET patente=$1 WHERE id=$2`, Row{"v": v}.num("v"), id); err != nil {
			return fmt.Errorf("character %s patente: %w", name, err)
		}
	}
	for _, rank := range customRankColumns {
		v, ok := col(rank)
		if !ok {
			continue
		}
		n := Row{"v": v}.num("v")
		if n == 0 {
			continue
		}
		if _, err := tx.Exec(ctx, `INSERT INTO custom_rank (name, rank, score) VALUES ($1,$2,$3) ON CONFLICT DO NOTHING`,
			name, rank, n); err != nil {
			return fmt.Errorf("character %s %s: %w", name, rank, err)
		}
		rep.CustomRanks++
	}
	return nil
}

// importJewelBank: CustomJewelBank (the jewels of each account in the
// bank, DS 0xF7, migration 0016_custom_shop.sql). Only rows of imported
// accounts are kept; NULL columns are 0.
func importJewelBank(ctx context.Context, src Source, tx pgx.Tx, accounts map[string]int64, rep *Report) error {
	list, err := rows(ctx, src, "CustomJewelBank")
	if err != nil {
		return err
	}
	for _, r := range list {
		if _, ok := accounts[key(r.str("AccountID"))]; !ok {
			rep.skip("CustomJewelBank %q: account not found", r.str("AccountID"))
			continue
		}
		if _, err := tx.Exec(ctx, `INSERT INTO custom_jewel_bank (account, chaos, life, soul, bless, creation,
			guardian, harmony, gem_stone, low_stone, high_stone) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
			ON CONFLICT DO NOTHING`, r.str("AccountID"), r.num("Chaos"), r.num("Life"), r.num("Soul"),
			r.num("Bless"), r.num("Creation"), r.num("Guardian"), r.num("Harmony"), r.num("GemStone"),
			r.num("LowStone"), r.num("HighStone")); err != nil {
			return fmt.Errorf("CustomJewelBank %s: %w", r.str("AccountID"), err)
		}
		rep.JewelBanks++
	}
	return nil
}

// importSerial continues the item serials after GameServerInfo.ItemCount
// (the DataServer counter).
func importSerial(ctx context.Context, src Source, tx pgx.Tx) error {
	list, err := rows(ctx, src, "GameServerInfo")
	if err != nil {
		return err
	}
	var max int64
	for _, r := range list {
		if v := r.num("ItemCount"); v > max {
			max = v
		}
	}
	if max < 1 {
		return nil
	}
	_, err = tx.Exec(ctx, `SELECT setval('item_serial_seq', $1)`, max)
	return err
}
