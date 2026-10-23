# 06 — Banco de dados: schema original e proposta PostgreSQL

Fonte: `MuServer/DB/MuOnline.sql` (script SQL Server em UTF-16, gerado em 16/02/2022), `MuServer/DB/Query/*.sql`, e as queries embutidas no DataServer/JoinServer. Os backups `.bak` (formato nativo do SQL Server) e o `.rar` **não** foram analisados.

## 1. Tabelas originais (observado)

`!` = `NOT NULL`. Tamanhos em bytes.

### Contas

| Tabela | Colunas |
|---|---|
| `MEMB_INFO` | `memb_guid int IDENTITY!`, `memb___id varchar(10)!` (PK lógica), `memb__pwd varchar(10)!`, `memb_name varchar(10)!`, `sno__numb char(18)!`, `post_code`, `addr_info`, `addr_deta`, `tel__numb`, `phon_numb`, `mail_addr varchar(50)`, `fpas_ques`, `fpas_answ`, `job__code`, `appl_days`, `modi_days`, `out__days`, `true_days`, `mail_chek`, `bloc_code char(1)!`, `ctl1_code char(1)!`, `AccountLevel int!`, `AccountExpireDate smalldatetime!` |
| `MEMB_STAT` | `memb___id!` PK, `ConnectStat tinyint`, `ServerName varchar(50)`, `IP varchar(15)`, `ConnectTM`, `DisConnectTM`, `OnlineHours int` |
| `AccountCharacter` | `Number IDENTITY!`, `Id varchar(10)!` PK, `GameID1..GameID5 varchar(10)`, `GameIDC varchar(10)` (último usado), `MoveCnt tinyint`, `ExtClass int!` (bits de classes liberadas), `Locked int!`, `Password varchar(50)` |
| `warehouse` | `AccountID!` PK, `Items varbinary(1920)` (= 120 itens × 16), `Money int`, `EndUseDate`, `DbVersion tinyint`, `pw smallint` |
| `ExtWarehouse` | `AccountID!`, `Items varbinary(1920)`, `Money int`, `Number int` |

### Personagens

| Tabela | Colunas |
|---|---|
| `Character` | `AccountID!`, `Name varchar(10)!` PK, `cLevel int`, `LevelUpPoint int`, `Class tinyint`, `Experience int`, `Strength/Dexterity/Vitality/Energy/Leadership int`, `Inventory varbinary(1728)` (= 108 × 16), `MagicList varbinary(180)` (= 60 × 3), `Money int`, `Life/MaxLife/Mana/MaxMana/BP/MaxBP/Shield/MaxShield real`, `MapNumber/MapPosX/MapPosY smallint`, `MapDir tinyint`, `PkCount/PkLevel/PkTime int`, `MDate/LDate smalldatetime`, `CtlCode tinyint`, `DbVersion tinyint`, `Quest varbinary(50)`, `ChatLimitTime int`, `FruitPoint int`, `EffectList varbinary(208)` (= 16 × 13), `FruitAddPoint int!`, `FruitSubPoint int!`, `Married int!`, `MarryCouple varchar(10)`, `ResetCount int!`, `MasterResetCount int!`, `IsNewChar int!` |
| `DefaultClassType` | `Class tinyint!` PK, `Level`, `LevelUpPoint`, stats, `Inventory varbinary(1728)`, `MagicList varbinary(180)`, `Life/MaxLife/Mana/MaxMana real`, `MapNumber/MapPosX/MapPosY`, `Quest varbinary(50)`, `DbVersion`, `EffectList varbinary(208)` |
| `OptionData` | `Name!` PK, `SkillKey binary(10)`, `GameOption tinyint`, `Qkey/Wkey/Ekey tinyint`, `ChatWindow tinyint` |
| `ResetData` | `Account!`, `Name!`, `ResetDay/Wek/Mon int!`, `ResetDateDay/Wek/Mon!`, `MasterResetDay/Wek/Mon!`, `MasterResetDateDay/Wek/Mon!` |
| `EventEntryCount` | `Account!`, `Name!`, `BCCount, CCCount, DSCount, DGCount, ITCount, IGCount int!`, `LastDate!` |
| `ReconnectData` | `Name!`, `ServerCode int!` (PK composta), `Data binary(400)!`, `Time int!` |
| `CustomPick` | `Name!` PK, `CustomPickList varbinary(400)`, `Money!`, `Jewel!`, `ItemExc!`, `ItemSet!` |
| `GiftData`, `MonsterKillCount`, `DailyRewardData` | (Name, Index/MonsterClass, Count/result) |

Valores iniciais de `DefaultClassType` (observado):

| Class | Str | Dex | Vit | Ene | Lead | Life | Mana | Mapa (X,Y) |
|---|---|---|---|---|---|---|---|---|
| 0 (DW) | 18 | 18 | 15 | 30 | 0 | 60 | 60 | 0 (182,128) |
| 16 (DK) | 28 | 20 | 25 | 10 | 0 | 110 | 20 | 0 (182,128) |
| 32 (Elf) | 22 | 25 | 20 | 15 | 0 | 80 | 30 | 3 (172,97) |
| 48 (MG) | 26 | 26 | 26 | 26 | 0 | 110 | 60 | 0 (182,128) |
| 64 (DL) | 26 | 20 | 20 | 15 | 25 | 90 | 40 | 0 (182,128) |

### Itens

| Tabela | Colunas |
|---|---|
| `GameServerInfo` | `Number int!` PK, `ItemCount int!` (próximo serial), `ZenCount`, `AceItemCount` |
| `T_PetItem_Info` | `ItemSerial!` PK, `Pet_Level smallint`, `Pet_Exp int` |
| `PeriodicItem` | `ItemSerial!` PK, `Time int` |

### Guild

| Tabela | Colunas |
|---|---|
| `Guild` | `G_Name varchar(8)!` PK, `G_Mark varbinary(32)` (logo 8×8, 4 bits/pixel), `G_Score int`, `G_Master varchar(10)`, `G_Count int`, `G_Notice varchar(60)`, `Number IDENTITY!`, `G_Type int!`, `G_Rival int!`, `G_Union int!`, `MemberCount int`, `Vault varbinary(3840)` |
| `GuildMember` | `Name!` PK, `G_Name varchar(8)!`, `G_Level tinyint`, `G_Status tinyint!` |

### Amigos / mail

`T_CGuid (GUID IDENTITY, Name)`, `T_FriendMain (GUID PK, Name, FriendCount, MemoCount, MemoTotal)`, `T_FriendList (GUID, FriendGuid, FriendName, Del)`, `T_WaitFriend (GUID, FriendGuid, FriendName)`, `T_FriendMail (MemoIndex, GUID, FriendName, wDate, Subject varchar(50), bRead bit, Memo varbinary(1000), Photo binary(18), Dir, Act)`.

### Rankings e eventos

`RankingDuel (Name, WinScore, LoseScore)`, `RankingBloodCastle/ChaosCastle/DevilSquare (Name, Score)`, `EventGoldenArcher (Account, RenaCount, StoneCount)`.

### Castle Siege

`MuCastle_DATA (MAP_SVR_GROUP PK, SIEGE_START_DATE, SIEGE_END_DATE, SIEGE_GUILDLIST_SETTED bit, SIEGE_ENDED bit, CASTLE_OCCUPY bit, OWNER_GUILD varchar(8), MONEY money, TAX_RATE_CHAOS, TAX_RATE_STORE, TAX_HUNT_ZONE)`, `MuCastle_NPC (MAP_SVR_GROUP, NPC_NUMBER, NPC_INDEX, NPC_DF_LEVEL, NPC_RG_LEVEL, NPC_MAXHP, NPC_HP, NPC_X, NPC_Y, NPC_DIR, NPC_CREATEDATE)`, `MuCastle_REG_SIEGE (MAP_SVR_GROUP, REG_SIEGE_GUILD, REG_MARKS, IS_GIVEUP, SEQ_NUM)`, `MuCastle_SIEGE_GUILDLIST (MAP_SVR_GROUP, GUILD_NAME, GUILD_ID, GUILD_INVOLVED bit, GUILD_SCORE)`, `MuCastle_MONEY_STATISTICS (MAP_SVR_GROUP, LOG_DATE, MONEY_CHANGE)`.

### Divergências entre o script e o que o código usa (observado)

O código referencia objetos **ausentes** em `MuOnline.sql`:

| Objeto | Onde é usado | Origem conhecida |
|---|---|---|
| `MEMB_INFO.Lock` | JoinServer (`SELECT top 1 Lock …`, `UPDATE memb_info SET Lock`) | `DB/Query/Rodar.sql` (`ADD Lock int NOT NULL DEFAULT 0`) |
| `MEMB_INFO.Credits`, `MEMB_INFO.Gold` | DataServer (CustomXShop, Gold) | **nenhum script** (U-DB-01) |
| `Character.Patente`, `TopDeath`, `Duel*`, `TopDupla`, `TopRoleta`, `topsurvivor`, `TopTheft`, `top*` | DataServer `0x41`, `0xF3` | `DB/Query/Rodar.sql`, `Query Survivor.sql` |
| `CustomJewelBank` | DataServer `0xF7` | `DB/Query/Create Table CustomJewelBank.sql` |
| `DailyReward (acc, name, date, month, Created_At)` | DataServer `0x68` | `DB/Query/DailyReward.sql` |
| `QuestKillCount` | DataServer `0x0C` | **nenhum script** (U-DB-02) |
| `RankingIllusionTemple` | DataServer `0x40` | **nenhum script** (U-DB-03) |
| `WZ_GetMasterResetInfo`/`WZ_SetMasterResetInfo` | DataServer `0x0F/01`, `0x3A` | presentes no script |

Contas de exemplo no dump (`admin/admin`, `test/test`) são de fábrica do emulador — não migrar.

## 2. Formatos binários dentro das colunas (observado)

| Coluna | Formato |
|---|---|
| `Character.Inventory` | 108 slots × 16 bytes; slot vazio = 16 × `0xFF`. Codificação de item em [09 §2](09-itens-e-inventario.md). Slots 0–11 = equipamento, 12–75 = grade 8×8, restante = loja pessoal (**inferido** pela contagem; validar U-ITEM-02). |
| `warehouse.Items` / `ExtWarehouse.Items` | 120 slots × 16 bytes (grade 8×15). |
| `Character.MagicList` | 60 × 3 bytes. O GS grava, por skill, 3 bytes vindos de `+8`, `+4`, `+9` do registro de skill de 20 bytes (`0x475c30`). Significado exato dos 3 bytes **a confirmar** (U-DB-04). |
| `Character.EffectList` | 16 × 13 bytes (buffs persistentes, `CEffectManager`). |
| `Character.Quest` | 50 bytes (estado das quests; 2 bits por quest é o esperado, **não verificado**). |
| `OptionData.SkillKey` | 10 bytes (atalhos de skill 1–0). |
| `Guild.G_Mark` | 32 bytes (logo 8×8, 4 bits por pixel — **inferido** pelo tamanho). |
| `ReconnectData.Data` | 400 bytes de estado para reconexão (formato a confirmar, U-DB-05). |

## 3. Proposta de schema PostgreSQL

Diretrizes:

1. **Chaves substitutas** (`bigserial id`) + `UNIQUE` nos nomes; FKs reais (`characters.account_id → accounts.id`). Renomear personagem passa a ser um `UPDATE` em uma tabela.
2. Nomes `snake_case`; manter um dicionário de equivalência para a ferramenta de migração.
3. **Blobs**: manter `bytea` com exatamente o mesmo layout do original nas colunas de inventário/baú/magias/efeitos na **fase 1** (migração trivial, compatibilidade com ferramentas existentes). Em fase posterior, normalizar itens em `items(id, owner_kind, owner_id, slot, item_index, level, …, serial)`.
4. `citext` para nomes de conta/personagem (o SQL Server original é case-insensitive por collation).
5. `item_serial_seq` (`SEQUENCE`) no lugar de `GameServerInfo.ItemCount`.
6. Datas em `timestamptz`.

```sql
CREATE EXTENSION IF NOT EXISTS citext;

CREATE TABLE accounts (
  id               bigserial PRIMARY KEY,
  login            citext UNIQUE NOT NULL CHECK (length(login) <= 10),
  password_hash    text   NOT NULL,          -- argon2id; aceita 'plain:'/'md5:' na migração
  personal_id      varchar(18) NOT NULL DEFAULT '',   -- sno__numb
  email            varchar(50),
  block_code       smallint NOT NULL DEFAULT 0,       -- bloc_code
  account_level    smallint NOT NULL DEFAULT 0,       -- AL0..AL3
  level_expires_at timestamptz,
  lock_code        integer NOT NULL DEFAULT 0,        -- MEMB_INFO.Lock
  ext_class        integer NOT NULL DEFAULT 0,        -- AccountCharacter.ExtClass
  credits          bigint  NOT NULL DEFAULT 0 CHECK (credits >= 0),
  gold             bigint  NOT NULL DEFAULT 0,
  created_at       timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE account_sessions (            -- MEMB_STAT
  account_id     bigint PRIMARY KEY REFERENCES accounts(id) ON DELETE CASCADE,
  online         boolean NOT NULL DEFAULT false,
  server_name    varchar(50),
  ip             inet,
  connected_at   timestamptz,
  disconnected_at timestamptz,
  online_hours   integer NOT NULL DEFAULT 0
);

CREATE TABLE characters (
  id              bigserial PRIMARY KEY,
  account_id      bigint NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  slot            smallint NOT NULL CHECK (slot BETWEEN 0 AND 4),
  name            citext UNIQUE NOT NULL CHECK (length(name) <= 10),
  class           smallint NOT NULL,
  level           integer NOT NULL DEFAULT 1,
  level_up_points integer NOT NULL DEFAULT 0,
  experience      bigint  NOT NULL DEFAULT 0,
  strength integer NOT NULL, dexterity integer NOT NULL, vitality integer NOT NULL,
  energy   integer NOT NULL, leadership integer NOT NULL DEFAULT 0,
  money           bigint  NOT NULL DEFAULT 0,
  life real NOT NULL, max_life real NOT NULL, mana real NOT NULL, max_mana real NOT NULL,
  bp real NOT NULL DEFAULT 0, max_bp real NOT NULL DEFAULT 0,
  map smallint NOT NULL, pos_x smallint NOT NULL, pos_y smallint NOT NULL, dir smallint NOT NULL DEFAULT 0,
  pk_count integer NOT NULL DEFAULT 0, pk_level integer NOT NULL DEFAULT 3, pk_time integer NOT NULL DEFAULT 0,
  ctl_code smallint NOT NULL DEFAULT 0,
  inventory  bytea NOT NULL CHECK (length(inventory) = 1728),
  magic_list bytea NOT NULL CHECK (length(magic_list) = 180),
  effect_list bytea NOT NULL CHECK (length(effect_list) = 208),
  quest       bytea NOT NULL CHECK (length(quest) = 50),
  chat_limit_time integer NOT NULL DEFAULT 0,
  fruit_add_points integer NOT NULL DEFAULT 0, fruit_sub_points integer NOT NULL DEFAULT 0,
  married_to_id   bigint REFERENCES characters(id),
  resets integer NOT NULL DEFAULT 0, master_resets integer NOT NULL DEFAULT 0,
  is_new boolean NOT NULL DEFAULT true,
  rank_counters jsonb NOT NULL DEFAULT '{}',      -- Patente, TopDeath, Duel* ... (enum validado no código)
  created_at timestamptz NOT NULL DEFAULT now(), last_login_at timestamptz,
  UNIQUE (account_id, slot)
);
CREATE TABLE account_last_character (account_id bigint PRIMARY KEY REFERENCES accounts(id), character_id bigint REFERENCES characters(id));

CREATE TABLE default_class_templates (class smallint PRIMARY KEY, level integer, level_up_points integer,
  strength integer, dexterity integer, vitality integer, energy integer, leadership integer,
  inventory bytea, magic_list bytea, life real, max_life real, mana real, max_mana real,
  map smallint, pos_x smallint, pos_y smallint, quest bytea, effect_list bytea);

CREATE TABLE warehouses (account_id bigint REFERENCES accounts(id) ON DELETE CASCADE, number smallint NOT NULL DEFAULT 0,
  items bytea NOT NULL CHECK (length(items) = 1920), money bigint NOT NULL DEFAULT 0, password smallint,
  PRIMARY KEY (account_id, number));

CREATE SEQUENCE item_serial_seq;
CREATE TABLE pet_items (item_serial bigint PRIMARY KEY, pet_level smallint, pet_exp integer);
CREATE TABLE periodic_items (item_serial bigint PRIMARY KEY, expires_at integer);

CREATE TABLE character_options (character_id bigint PRIMARY KEY REFERENCES characters(id) ON DELETE CASCADE,
  skill_keys bytea, game_option smallint, q_key smallint, w_key smallint, e_key smallint, chat_window smallint);
CREATE TABLE character_reset_counters (character_id bigint PRIMARY KEY REFERENCES characters(id) ON DELETE CASCADE,
  reset_day int, reset_week int, reset_month int, reset_day_at timestamptz, reset_week_at timestamptz, reset_month_at timestamptz,
  mreset_day int, mreset_week int, mreset_month int, mreset_day_at timestamptz, mreset_week_at timestamptz, mreset_month_at timestamptz);
CREATE TABLE character_event_entries (character_id bigint PRIMARY KEY REFERENCES characters(id) ON DELETE CASCADE,
  bc int, cc int, ds int, dg int, it int, ig int, last_reset_at timestamptz);
CREATE TABLE reconnect_states (character_id bigint REFERENCES characters(id) ON DELETE CASCADE, server_code smallint,
  data bytea, time integer, PRIMARY KEY (character_id, server_code));

CREATE TABLE guilds (id bigserial PRIMARY KEY, name citext UNIQUE NOT NULL CHECK (length(name) <= 8),
  mark bytea CHECK (length(mark) = 32), score integer NOT NULL DEFAULT 0, master_id bigint REFERENCES characters(id),
  notice varchar(60), type integer NOT NULL DEFAULT 0, rival_id bigint REFERENCES guilds(id), union_id bigint REFERENCES guilds(id),
  vault bytea);
CREATE TABLE guild_members (character_id bigint PRIMARY KEY REFERENCES characters(id) ON DELETE CASCADE,
  guild_id bigint NOT NULL REFERENCES guilds(id) ON DELETE CASCADE, level smallint, status smallint NOT NULL);

-- friends/mail, rankings, castle siege, cash shop: tabelas 1:1 com o original, com FKs para characters/guilds.
```

## 4. Migração de dados (MSSQL → PostgreSQL)

1. Restaurar o `.bak` em um SQL Server temporário (fora do escopo deste repositório).
2. Ferramenta Go `cmd/migrate-mssql` que lê tabela a tabela e grava no novo schema, convertendo nomes em IDs e preservando blobs.
3. Senhas: `memb__pwd` em texto (com `MD5Encryption=0`) ⇒ gravar `plain:<senha>` e re-hash no primeiro login.
4. Validar: contagem de linhas por tabela, checksum dos blobs, personagens por conta ≤ 5.
