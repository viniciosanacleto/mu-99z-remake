# 05 — Módulo `persistence` (substitui o DataServer)

Fonte: `DataServer.exe` (SHA-256 `bcdc46f32ab9…`, 326 funções de usuário). Dispatcher GS→DS: `FUN_0040a5a0`. Configuração: `DataServer.ini` (`DataServerODBC = MuOnline`, `DataServerPort = 55960`), `AllowableIpList.txt`, `BadSyntax.txt` (palavras proibidas em nomes).

O DataServer é um tradutor "pacote → SQL". Toda a tabela abaixo foi **observada**: o handler de cada opcode foi associado às strings SQL que ele (ou seus callees diretos) referencia. O head de resposta é a constante de cabeçalho encontrada no handler (`HDR`), quando existe.

## 1. Tabela de opcodes GS → DS

### 1.1 Conta e personagens

| Head/Sub | Handler | SQL | Resposta |
|---|---|---|---|
| `00` | 40b120 | `SELECT 1 FROM GameServerInfo WHERE Number = 0`; cria linha se faltar; `SELECT * FROM ReconnectData WHERE ServerCode='%d'` | registro do GS + lista de reconexões pendentes |
| `01` | 40b310 | `SELECT Id FROM AccountCharacter WHERE Id='%s'` / `INSERT INTO AccountCharacter (Id)`; `SELECT * FROM AccountCharacter` (`GameID1..5`, `MoveCnt`, `ExtClass`); por personagem `SELECT cLevel,Class,Inventory,CtlCode FROM Character WHERE AccountID AND Name`; `SELECT G_Status FROM GuildMember WHERE Name` | lista de personagens (slot, nome, nível, classe, equipamentos p/ CharSet, CtlCode, status de guild) |
| `02` | 40bcf0 | `EXEC WZ_CreateCharacter '%s','%s','%d'`; `UPDATE AccountCharacter SET GameID%d='%s'` | `C1 3A 02` |
| `03` | 40c170 | `EXEC WZ_DeleteCharacter '%s','%s'`; `UPDATE AccountCharacter SET GameID%d=NULL` | `C1 12 03` |
| `04` | 40c4c0 | `SELECT * FROM Character WHERE AccountID AND Name`; `UPDATE Character SET IsNewChar='0'`; `EXEC WZ_GetResetInfo`; `EXEC WZ_GetMasterResetInfo`; `EXEC WZ_GetEventEntryInfo`; `UPDATE AccountCharacter SET GameIDC` | dados completos do personagem para entrar no mundo |
| `30` | 40e9c0 | `UPDATE Character SET cLevel,Class,LevelUpPoint,Experience,Strength,Dexterity,Vitality,Energy,Leadership,Inventory=?,MagicList=?,Money,Life,MaxLife,Mana,MaxMana,BP,MaxBP,MapNumber,MapPosX,MapPosY,MapDir,PkCount,PkLevel,PkTime,Quest=?,ChatLimitTime,EffectList=?,FruitAddPoint,FruitSubPoint,Married,MarryCouple WHERE AccountID AND Name`; `EXEC WZ_SetEventEntryInfo` | — (save periódico/logout) |
| `31` | 40ed60 | `UPDATE Character SET Inventory=? WHERE AccountID AND Name` | — |
| `42` | 40fd30 | `UPDATE AccountCharacter SET ExtClass=%d WHERE Id` | — |
| `41` | 4108f0 | `UPDATE Character SET %s = %s + %d WHERE Name='%s'` (**coluna dinâmica** — usado pelos rankings custom `TopDeath`, `Duelsm`…) | — |
| `F3` | 410ae0 | `UPDATE Character SET Patente = 0 WHERE Patente = %d`; `UPDATE Character SET Patente = %d WHERE Name` | — |

### 1.2 Itens, pets e baú

| Head/Sub | Handler | SQL | Resposta |
|---|---|---|---|
| `05/00` | 41c8a0 | baú: `SELECT AccountID FROM warehouse`; `INSERT INTO warehouse (AccountID,Money,EndUseDate,DbVersion) VALUES (…,0,getdate(),3)`; `UPDATE warehouse SET Items=CONVERT(varbinary(%d),REPLICATE(char(0xFF),%d))` (inicializa vazio); `SELECT Items,Money,pw FROM warehouse`; baús extras: mesmas operações em `ExtWarehouse … AND Number=%d` | conteúdo do baú |
| `05/30` | 41cf60 | `UPDATE warehouse SET Items=?,Money=%d`; `UPDATE ExtWarehouse SET Items=?,Money=%d … Number`; `UPDATE warehouse SET pw=%d` | — |
| `06` | 40d6c0 | `EXEC WZ_GetItemSerial` | `C1 24 06` (serial novo) |
| `07` | 40d8c0 | `EXEC WZ_GetItemSerial` + `INSERT INTO T_PetItem_Info` | `C1 30 07` |
| `09` | 40dfc0 | `SELECT Pet_Level,Pet_Exp FROM T_PetItem_Info WHERE ItemSerial` | — |
| `34` | 40f0b0 | upsert `T_PetItem_Info (ItemSerial, Pet_Level, Pet_Exp)` | — |
| `2A/00`, `2A/30` | 4189b0, 418d30 | `PeriodicItem (ItemSerial, Time)` — leitura e upsert | — |
| `F7/04`, `F7/05` | 410280, 410510 | `CustomJewelBank` (Chaos, Life, Soul, Bless, Creation, Guardian, Harmony, GemStone, LowStone, HighStone) — `UPDATE … SET Chaos+=%d …` / `SELECT *` | `C1 30 F7 05` |
| `F7/06` | 410c50 | `MEMB_INFO.Gold`: `UPDATE MEMB_INFO SET Gold = ISNULL(Gold,0) + %d` (cria linha se não existir) | — |
| `0F/20` | 4010d0 | `SELECT ISNULL(Credits,0) AS Credits FROM MEMB_INFO` | `C1 18 0F 20` |
| `0F/21` | 401300 | `UPDATE MEMB_INFO SET Credits=ISNULL(Credits,0)-%u OUTPUT inserted.Credits … AND ISNULL(Credits,0)>=%u` (débito atômico) | `C1 24 0F 21` |
| `0F/22` | 401610 | `UPDATE MEMB_INFO SET Credits=ISNULL(Credits,0)+%u` (estorno; log `[CustomXShop] Refund`) | — |
| `0F/23` | 401740 | créditos + `OnlineHours` + segundos da sessão atual (join com `MEMB_STAT`) | `C1 20 0F 23` |

### 1.3 Opções, reset, eventos e rankings

| Head/Sub | Handler | SQL |
|---|---|---|
| `08` | 40dc90 | `SELECT * FROM OptionData WHERE Name` → `C1 2C 08` |
| `33` | 40ee30 | upsert `OptionData (SkillKey=?, GameOption, Qkey, Wkey, Ekey, ChatWindow)` |
| `0C/00`, `0C/30` | 419670, 419ab0 | `QuestKillCount` (QuestIndex + até 5 pares MonsterClass/KillCount) |
| `0F/00`, `0F/01` | 4085a0, 4087c0 | `EXEC WZ_GetResetInfo`, `EXEC WZ_GetMasterResetInfo` |
| `0F/02` | 4089e0 | `UPDATE Character SET Married=0, MarryCouple=NULL` |
| `0F/03` | 408a70 | `GiftData (Name, [Index], Count)` |
| `0F/04` | 408d60 | `EXEC WZ_RenameCharacter '%s','%s',?` |
| `0F/05`, `0F/06` | 409040, 409220 | `EXEC WZ_BanAccount`, `EXEC WZ_BanCharacter` |
| `13/00`, `13/30` | 409480, 4097b0 | `CustomPick (CustomPickList=?, Money, Jewel, ItemExc, ItemSet)` (filtro de auto-pick) |
| `39`, `3A` | 40f2e0, 40f3a0 | `EXEC WZ_SetResetInfo`, `EXEC WZ_SetMasterResetInfo` |
| `3C` | 40f470 | `RankingDuel (WinScore, LoseScore)` += |
| `3D` / `3E` / `3F` / `40` | 40f670 / 40f820 / 40f9d0 / 40fb80 | `RankingBloodCastle` / `RankingChaosCastle` / `RankingDevilSquare` / `RankingIllusionTemple` `Score += %d` |
| `50` | 40e6d0 | `MonsterKillCount (Name, MonsterClass, MonsterKillCount)` |
| `68` | 410980 | `DailyReward (acc, name, date, month)` |
| `94/00`, `94/01`, `94/30` | 413750, 4139a0, 413c80 | `EventGoldenArcher (RenaCount, StoneCount)` |
| `C0/00`, `C0/01` | 41a4e0, 41a760 | `ReconnectData (Name, ServerCode, Data=?, Time)` — upsert/delete |
| `20`, `21`, `2B` | 40e3a0, 40e4a0, 40e5d0 | sem SQL direto (respostas `C1 52 20`, `C1 94 21`, `C1 88 2B`) — a confirmar |

### 1.4 Guild (head `A0`)

| Sub | Handler | SQL |
|---|---|---|
| 00 | 413f40 | carga: `SELECT * FROM Guild`, `SELECT * FROM GuildMember`; `UPDATE Guild SET G_Mark=?`; `UPDATE GuildMember SET G_Status`; `SELECT Number FROM Guild WHERE G_Name` |
| 01 | 4140d0 | apagar guild: `DELETE FROM Guild`, `DELETE FROM GuildMember WHERE G_Name` |
| 02 | 4141f0 | adicionar membro: `INSERT INTO GuildMember (Name,G_Name,G_Level,G_Status) VALUES (…,1,%d)` |
| 03 | 414320 | remover membro |
| 04 | 414440 | `UPDATE GuildMember SET G_Status` |
| 05 | 414510 | `UPDATE Guild SET G_Type` |
| 06 | 4145c0 | `UPDATE Guild SET G_Notice=?` |
| 07 | 414680 | (resposta `C1 55 A0 08`) a confirmar |
| 08 | 4146f0 | `UPDATE Guild SET G_Score` |
| 09 | 4147a0 | a confirmar |
| 0A | 414b60 | `UPDATE Guild SET G_Rival` / `G_Union` (criar relação) |
| 0B, 0C | 414e00, 414f90 | desfazer união/rivalidade (`… = 0 WHERE Number IN (%d,%d)`) |

Criação de guild usa a SP `WZ_GuildCreate` (presente no schema; não referenciada nas strings do DataServer — incógnita U-DS-01).

### 1.5 Amigos e mail (head `B0`)

Subs `00..08` (`410ed0`…`412bc0`): `T_FriendMain (GUID, Name, FriendCount, MemoCount, MemoTotal)`, `T_FriendList (GUID, FriendGuid, FriendName, Del)`, `T_WaitFriend`, `T_FriendMail (MemoIndex, GUID, FriendName, wDate, Subject, bRead, Memo varbinary(1000), Photo binary(18), Dir, Act)`, `T_CGuid`. SPs: `WZ_UserGuidCreate`, `WZ_FriendAdd`, `WZ_FriendDel`, `WZ_WaitFriendAdd`, `WZ_WaitFriendDel`, `WZ_WriteMail` (retornos 2–6 de erro), `WZ_DelMail`.

### 1.6 Castle Siege (heads `80`–`89`)

| Head/Sub | SQL |
|---|---|
| `80/01` | `EXEC WZ_CS_GetOwnerGuildMaster %d` |
| `80/03` | `EXEC WZ_CS_ReqNpcBuy` (10 params) |
| `80/04` | `EXEC WZ_CS_ReqNpcRepair` |
| `80/05` | `EXEC WZ_CS_ReqNpcUpgrade` |
| `80/06` | `EXEC WZ_CS_GetCastleTaxInfo` |
| `80/07` | `EXEC WZ_CS_ModifyTaxRate` |
| `80/08`, `80/18` | `EXEC WZ_CS_ModifyMoney` |
| `80/09` | `EXEC WZ_CS_ModifyCastleSchedule %d, 'Y-M-D 00:00:00', 'Y-M-D 00:00:00'` |
| `80/0A` | `EXEC WZ_CS_GetGuildMarkRegInfo` |
| `80/0B` | `EXEC WZ_CS_ModifySiegeEnd` |
| `80/0C` | `EXEC WZ_CS_ModifyCastleOwnerInfo` |
| `80/0D` | `EXEC WZ_CS_ReqRegAttackGuild` |
| `80/0E` | `EXEC WZ_CS_ResetCastleSiege` |
| `80/10` | `EXEC WZ_CS_ReqRegGuildMark` |
| `80/12` | `EXEC WZ_CS_ModifyGuildGiveUp` |
| `80/16` | `DELETE MuCastle_NPC WHERE MAP_SVR_GROUP AND NPC_NUMBER AND NPC_INDEX` |
| `80/17` | (resposta `C1 24 80 17`) a confirmar |
| `80/19`, `80/1A`, `80/1B` | `WZ_CS_ResetCastleTaxInfo`, `WZ_CS_ResetSiegeGuildInfo`, `WZ_CS_ResetRegSiegeInfo` |
| `81` | `WZ_CS_GetCastleTotalInfo` + `WZ_CS_GetCastleNpcInfo` (carga inicial, `C2 .. 81`) |
| `83` | `SELECT TOP 100 * FROM MuCastle_REG_SIEGE … ORDER BY SEQ_NUM` |
| `84`, `89` | regravar NPCs: `DELETE MuCastle_NPC WHERE MAP_SVR_GROUP` + `INSERT INTO MuCastle_NPC (…)` |
| `85` | `WZ_CS_GetCalcRegGuildList` |
| `86` | `WZ_CS_GetCsGuildUnionInfo` |
| `87` | `DELETE MuCastle_SIEGE_GUILDLIST` + `WZ_CS_SetSiegeGuildInfo` |
| `88` | `WZ_CS_GetSiegeGuildInfo` |

## 2. Semântica das stored procedures principais (observado em `MuOnline.sql`)

- **`WZ_CreateCharacter(account, name, class)`**: nome já existe ⇒ retorna `0`; sem `AccountCharacter` cria; ocupa o primeiro slot vazio `GameID1..5`; sem slot ⇒ `3`; copia stats/inventário/magias/mapa/posição de `DefaultClassType WHERE Class = @Class`; sucesso ⇒ `1`; outro erro ⇒ `2`. Transação.
- **`WZ_DeleteCharacter(account, name)`**: apaga de `Character`, `CustomPick`, `DailyRewardData`, `EventEntryCount`, `GiftData`, `MonsterKillCount`, `OptionData`, rankings, `ReconnectData`, `ResetData`, `T_CGuid`, `T_Friend*`, `T_WaitFriend`. Retorna 1/0. **Sem transação** no original.
- **`WZ_RenameCharacter(account, old, new)`**: atualiza o nome em ~20 tabelas (incluindo `Guild.G_Master`, `GuildMember`). Retorna 0 se o novo nome existir.
- **`WZ_GetAccountLevel`**: se nível ≠ 0 e expirado ⇒ nível vira 0 (persistido). **`WZ_SetAccountLevel(account, level, seconds)`**: mesmo nível ⇒ estende a expiração atual; nível diferente ⇒ expiração = agora + segundos.
- **`WZ_GetResetInfo` / `WZ_SetResetInfo`** (e versões *Master*): contadores diário/semanal/mensal em `ResetData`, zerados por `datediff(day|week|month)`; `Character.ResetCount` é o total.
- **`WZ_GetEventEntryInfo`**: contadores diários de entrada (BC, CC, DS, DG, IT, IG) zerados após 1 dia. **Bug observado**: o `UPDATE` usa `@ITCount=@ITCount` em vez de `ITCount=@ITCount`, então `ITCount` nunca é zerado no banco. A reescrita deve corrigir.
- **`WZ_GetItemSerial`**: `UPDATE GameServerInfo SET @ItemSerial = ItemCount = ItemCount+1` (contador global de serial de item). Em PostgreSQL usar `SEQUENCE`.
- **`WZ_CONNECT_MEMB` / `WZ_DISCONNECT_MEMB`**: upsert de `MEMB_STAT` (ConnectStat, ServerName, IP, ConnectTM / DisConnectTM, OnlineHours += horas).
- **`WZ_GuildCreate(name, master)`**: insere `Guild` + `GuildMember (G_Level=1)`; 0 ok, 1/2 erro.
- **`WZ_SetGuildDelete`**, **`WZ_BanAccount` (`bloc_code`)**, **`WZ_BanCharacter` (`CtlCode`)**.

## 3. Interfaces Go propostas

```go
type CharacterRepo interface {
    ListByAccount(ctx, account string) ([]CharacterSummary, AccountChars, error)
    Create(ctx, account, name string, class uint8) (CreateResult, error)  // 0=existe,1=ok,2=erro,3=sem slot
    Delete(ctx, account, name string) error                                // transacional
    Load(ctx, account, name string) (*CharacterData, error)
    Save(ctx, *CharacterData) error                                        // equivale a DS 0x30
    SaveInventory(ctx, account, name string, inv []byte) error             // DS 0x31
    Rename(ctx, account, old, new string) (bool, error)
}
type WarehouseRepo interface { Load(ctx, account string, number int) (*Warehouse, error); Save(ctx, *Warehouse) error; SetPassword(...) }
type ItemSerialRepo interface { Next(ctx) (uint32, error) }
type GuildRepo interface { LoadAll(ctx) ([]Guild, []GuildMember, error); Create; Delete; AddMember; RemoveMember; SetStatus; SetType; SetNotice; SetScore; SetRelation(...) }
type FriendRepo interface { ... }   // T_Friend*, mail
type CastleRepo interface { ... }   // todas as WZ_CS_*
type RankingRepo interface { Add(ctx, kind RankingKind, name string, delta ...int) error }
type CashRepo interface { Credits(ctx, account) (uint32, error); Debit(ctx, account string, v uint32) (newBalance uint32, ok bool, err error); Refund(...) }
```

Regras: toda operação multi-tabela em transação; nada de SQL com nome de coluna vindo do pacote (substituir o `0x41` dinâmico por enum de colunas permitidas).
