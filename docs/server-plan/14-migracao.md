# 14 — Migração do banco original (SQL Server → PostgreSQL)

Ferramenta: `apps/server/cmd/migrate` (pacote `internal/migrate`).

```
go run ./cmd/migrate -from 'sqlserver://sa:senha@host:1433?database=MuOnline' -to postgres://...
go run ./cmd/migrate -script data/DB/MuOnline.sql -to postgres://...
```

- `-from` lê um banco vivo (driver `github.com/microsoft/go-mssqldb`, `SELECT *` de cada tabela `dbo`; tabela ausente = nenhuma linha).
- `-script` lê um script "Generate Scripts" do SSMS (UTF-16 ou UTF-8): só os `INSERT [dbo].[T] (...) VALUES (...)` no início da linha. Entende `N'...'`, `NULL`, `0x...`, números e `CAST(0x.. AS SmallDateTime/DateTime)`.
- O destino recebe o schema de `internal/store` (`OpenPostgres`) e precisa estar sem contas. Tudo roda numa transação.

| Origem | Destino | Regras |
|---|---|---|
| `MEMB_INFO` + `AccountCharacter` | `accounts` | Senha vira `plain:<senha>` (`JoinServer.ini` `MD5Encryption = 0`; o `auth` regrava em argon2id no próximo login). `bloc_code`, `AccountLevel`, `AccountExpireDate` (> 1900), `ExtClass`, `Locked`, `MoveCnt`. `Credits` (coluna WizardTeam do XShop e do `/minhainfo`, ausente do `MuOnline.sql` distribuído; `NULL`/ausente = 0). |
| `Character` | `characters` | Slot = posição em `GameID1..5`; sem posição, primeiro slot livre. Blobs completados com `0xFF` até 1728 / 180 / 208 / 50 bytes. Personagem sem conta é ignorado (listado no relatório). |
| `Character.Patente`, `TopDeath`, `TopTheft`, `TopRoleta`, `TopDupla`, `topsurvivor`, `Duelsm`..`Duelall` | `characters.patente`, `custom_rank` | Colunas WizardTeam ausentes do `MuOnline.sql` distribuído; lidas quando existem no banco (nome sem diferenciar maiúsculas). Ranking 0 não gera linha. |
| `warehouse`, `ExtWarehouse` | `warehouses` | Número 0 e `Number`; itens completados até 1920 bytes. |
| `OptionData` | `character_options` | |
| `T_FriendMain`, `T_FriendList`, `T_WaitFriend`, `T_FriendMail` | `friend_*` | GUID mantido; sequência ajustada. |
| `Guild`, `GuildMember` | `guilds`, `guild_members` | `id = Number`, para `G_Rival`/`G_Union` continuarem válidos; marca completada até 32 bytes. |
| `T_PetItem_Info` | `pet_item_info` | |
| `EventEntryCount` | `event_entry_count` | |
| `EventGoldenArcher` | `golden_archer` | |
| `MuCastle_DATA`, `_NPC`, `_REG_SIEGE`, `_SIEGE_GUILDLIST`, `_MONEY_STATISTICS` | `mu_castle_*` | Ver [10d](10d-castle-siege.md) (`internal/migrate/castle.go`). |
| `ResetData` | `reset_data` | Só personagens importados; data ausente = agora (como a linha nova de `WZ_GetResetInfo`). |
| `RankingDuel` | `ranking_duel` | Mantém linhas de personagens apagados, como as tabelas originais; `NULL` = 0. |
| `RankingBloodCastle`, `RankingChaosCastle`, `RankingDevilSquare`, `RankingIllusionTemple` | `ranking_event` | `event` = head do DS que alimenta a tabela (0x3D, 0x3E, 0x3F, 0x40). |
| `PeriodicItem` | `periodic_item` | `Time` `NULL` = 0 (o item expira na próxima checagem). |
| `GiftData` | `gift_data` | Só personagens importados. |
| `CustomPick` | `custom_pick` | Só personagens importados; `CustomPickList` `NULL` = vazio. |
| `CustomJewelBank` | `custom_jewel_bank` | Uma linha por conta (`AccountID`); só contas importadas; colunas `NULL` = 0. Tabela ausente do `MuOnline.sql` distribuído (só aparece nas consultas do DataServer). |
| `MonsterKillCount` | `monster_kill_count` | Só personagens importados; `(Name, MonsterClass)` repetido mantém a primeira linha (`internal/migrate/custom_world.go`). |
| `DailyReward` (`acc`, `name`, `date`, `month`) | `daily_reward` | A tabela que o DataServer desta build lê e grava (`FUN_0040c4c0`, `FUN_00410980`); ausente do `MuOnline.sql` distribuído (colunas lidas sem diferenciar maiúsculas). Só contas importadas, dia 1..31 e mês 1..12. `DailyRewardData` (`Name`, `index`, `result`) só aparece em `WZ_RenameCharacter`/`WZ_DeleteCharacter`; nenhum binário a lê, então não é importada. |
| `GameServerInfo.ItemCount` | `item_serial_seq` | Os seriais continuam depois do contador do DataServer. |

Fora do escopo por enquanto: `MEMB_STAT`, `ReconnectData`, `T_CGuid`, `DailyRewardData` (não usada pelos binários), `Guild.Vault`.

Testes: `internal/migrate/migrate_test.go` lê o `MuOnline.sql` original (`apps/server/data/DB/`, 2 contas) e importa um script de exemplo com todas as tabelas num banco PostgreSQL temporário (quando `MU_TEST_DATABASE_URL` está definido); `custom_world_test.go` faz o mesmo para `MonsterKillCount` e `DailyReward`. Pendente: rodar contra o `MuOnline.bak` restaurado num SQL Server em contêiner.
