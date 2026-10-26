# 16 — Cobertura dos arquivos de dados

Quais arquivos de `MuServer/Data/**`, `4.GameServer/DATA/*` e `5.GameServerCS/DATA/*` o servidor original lê, e qual loader Go faz o parse de cada um. O teste `TestDataFileCoverage` (`apps/server/internal/game/datacoverage_test.go`) percorre `apps/server/config/` (os arquivos de `Data/`, das pastas `DATA` das instâncias e dos processos convertidos para JSON, [11](11-arquivos-de-configuracao.md) seção 0) e `apps/server/data/Data` (os arquivos mantidos no formato original). As entradas do teste usam os caminhos novos (`config/data/Item/Item.json`, `config/gameservers/4.GameServer.json`, `data/Data/Terrain/Terrain1.att`); nesta página os nomes são os originais. Cada arquivo encontrado cai em um de três casos:

- **carregado**: o original lê o arquivo e o loader Go listado roda sem erro sobre ele;
- **pendente**: o original lê, mas ainda não existe parser Go (nenhum arquivo hoje);
- **ignorado**: o original nunca lê, e o motivo fica registrado.

Um arquivo novo que não está em nenhuma das listas faz o teste falhar. Uma entrada que aponta para um arquivo inexistente também. O teste fica no pacote `game` porque vários loaders são privados desse pacote. Ele também monta um `World` com os dados reais (`New` executa todos os `startHooks`).

## Como o original encontra os arquivos

- A base é `..\Data\` (`strcpy_s(&DAT_012a4ccc, "..\\Data\\")` em `FUN_004e8460`). Cada loader monta `DAT_012a4d30 = base + caminho` e chama o leitor da classe.
- Os loaders de início ficam em `0x4e8520..0x4ec4c0` e também servem ao menu de reload (comandos `0x800f..0x801c` em `FUN_0047ec30`):

| Loader | Arquivos | Loader Go |
|---|---|---|
| `0x4e8520` | `BlackList.txt` (`FUN_00411ae0`: seção 0 IPs de 16 caracteres, seção 1 HWIDs de 44) | `gamedata.LoadBlackList`; usado em `game/blacklist.go` |
| `0x4e85f0` | `Character/DefaultClassInfo.txt`, `Character/DefaultClassFreebies.txt` (`FUN_0046cb40`) | `LoadClassInfo`, `LoadDefaultClassFreebies` (usado em `game/freebies.go`) |
| `0x4e86f0` | `CommandManager.txt`, `CommandRequirement.txt` (`FUN_004402a0`) | `game.loadCommands`, `LoadCommandRequirement` (usado em `game/cmdrequirement.go`) |
| `0x4e8860` | `ConnectMember.txt`, `MapManager.txt`, `MapRateInfo.txt` | `game.loadNameList`, `LoadMapManager`, `LoadMapRateInfo` |
| `0x4e8a50` | 35 arquivos de `Custom/` (incluindo `Custom/Event/*`, `CustomGuard`, `CustomMobCombo`, `DailyReward`, `CustomPatente`, `CustomEvolution`) | `gamedata.LoadCustom*` / `Load*Event`, `game.loadCustomTables`; `LoadCustomMobCombo` (mesmo leitor `FUN_0044ef50` do CustomGuard; lista do push-back, `pushback.go`) e `LoadCustomEvolution` (`FUN_0044aff0`; `game/customevo.go`) |
| `0x4e9d30` | 12 arquivos de `Event/*.dat` e `Custom/CustomQuiz.txt` | `LoadBloodCastle`, `LoadBonusManager`, `LoadCastleDeep`, `LoadCastleSiege`, `LoadChaosCastle`, `LoadDevilSquare`, `LoadDropEvent`, `LoadEventEntryLevel`, `LoadEventStageSpawn`, `LoadGoldenArcher`, `LoadInvasions`, `LoadKalima`, `LoadCustomQuiz` |
| `0x4ea4a0` | `EventItemBagManager.txt` e as bags que ele cita (`EventItemBag\%s.txt`, `FUN_00491220`) | `LoadEventItemBags` |
| `0x4ea5d0` | `Hack/HackPacketCheck.txt`, `Hack/HackSkillSpeedCheck.txt` (`FUN_00488340`), `Hack/Enc2.dat`, `Hack/Dec1.dat` | `game.loadHackRules`, `LoadHackSkillSpeedCheck` (`game/skillspeed.go`), `crypto.LoadKeyFile` |
| `0x4ea820` | 14 arquivos de `Item/` (`ItemLevel.txt` em `FUN_00493010`) | `LoadItems`, `LoadExcellentOptionRate`, `LoadItemDrop`, `LoadItemLevel` (`game/itemlevel.go`), `LoadItemLoot`, `LoadItemMoneyDrop`, `LoadItemMove`, `LoadItemOptions`, `LoadItemOptionRate`, `LoadItemStack`, `LoadItemValue`, `LoadSetItemOption`, `LoadSetItemType`, `LoadWingOptions` |
| `0x4eb000` | `Lang/{Eng,Por,Spn}/{Message,Notice}.txt` | `game.loadMessages`, `game.loadNotices` |
| `0x4eb3a0` | `Terrain\terrain%d.att` para os mapas 1..100, reaproveitando o último encontrado | `LoadTerrains` |
| `0x4eb520` | `MapServerInfo.txt` | `LoadMapServer` |
| `0x4eb5f0` | `Monster/MonsterList.txt`, `MonsterMove.txt` (`FUN_004b1110`), `MonsterSchedule.txt` (`FUN_004bb0c0`), `Skill/MonsterSkill*.txt`, `NpcTalk.txt` | `LoadMonsterList`, `LoadMonsterMove` (`game/monstermove.go`), `LoadMonsterSchedule` (`game/monsterschedule.go`), `game.loadMonsterSkills`, `LoadNpcTalk` |
| `0x4bbaa0` | `Monster\Spawn\NNN - *.txt`: só arquivos, sem subpastas, com o mapa aceito por `FUN_004ae750` | `LoadSpawns` |
| `0x4eba60` | `Move/Gate.txt`, `Move.txt`, `MoveSummon.txt` (`FUN_004bfd00`) | `LoadGates`, `game.loadMoves`, `LoadMoveSummon` (novo) |
| `0x4ebc50` | `Quest/Quest.txt`, `QuestObjective.txt`, `QuestReward.txt` | `LoadQuests` |
| `0x4ebe20` | `Script/ScriptMain.lua` (+ `require` → `\Script\%s.lua`, `0x4a5220`), `Script/Character/CalcCharacter.lua` | `game.loadScripts` (VM Lua embutida, `lua.go`; ponte em [17](17-lua.md)) |
| `0x4ebfc0` | `ShopManager.txt` e `Shop\%s.txt` citados (`FUN_004f55e0`) | `LoadShops` |
| `0x4ec0f0` | 7 arquivos de `Skill/` | `game.loadSkillWorld` (SkillDamage, ComboSkill, SkillEnabler, SkillUseArea, SkillElect.hit), `game.loadEffectList`, `LoadSkills` |
| `0x4ec4c0` | `Util/DamageTable.txt`, `ExperienceTable.txt` (`FUN_0047be80`), `Filter.txt`, `GameMaster.txt`, `PetDamageTable.txt` (`FUN_004d61d0`), `ResetLimit.txt`, `ResetTable.txt` | `game.loadDamageTable`, `LoadExperienceTable` (novo), `game.loadMoveData`, `LoadPetDamageTable` (novo), `game.loadCmdWorld` |
| sob demanda | `Custom/CreationGold.txt` (`FUN_0049a690`), `Custom/CustomXShopRecharge.txt` ao lado do CustomXShop (`FUN_00404a60`) | `CreationGoldValue`, `game.rechargeBlock` |
| `.\Data\GameServerInfo - *.dat` | os 8 `.dat` de cada instância (`GetPrivateProfile*`) | `gsconfig.Load` |
| CastleSiege.exe `FUN_00442c10` | `Custom/CustomGHRS.ini` (só no CastleSiege.exe): `[WEEK] Monday..Sunday` e `[TIMEUPDATE] Hour/Min/Sec`, `GetPrivateProfileIntA` com padrão 0 | `LoadCustomGHRS` (`game/ghrs.go`, só na instância com `ServerType = 1`) |

O CastleSiege.exe lê o mesmo conjunto que o GameServer.exe, com estas diferenças: não lê `CustomXShop.txt`, `CustomXShopRecharge.txt` nem `CreationGold.txt`, e lê também `CustomGHRS.ini`. A comparação foi feita por `strings` nos dois executáveis.

`Item\Item_{Eng,Por,Spn}.bmd` (`0x4ec8d0`) só é tratado pelo comando de menu `0x801c`, nunca na partida. Os arquivos não existem na instalação, e o uso que `FUN_004940b0` faz deles (ler ou gerar) não foi verificado.

## Números (`apps/server/config` + `apps/server/data/Data`, 279 arquivos)

| Situação | Arquivos |
|---|---|
| Lidos pelo original e carregados em Go | 261 (inclui `server.json`, os dois arquivos de instância e os 5 scripts Lua: `ScriptMain.lua`, `System/ScriptCore.lua`, `Script/WelcomeMessage.lua`, `Script/CustomWindowName.lua`, `Character/CalcCharacter.lua`) |
| Lidos pelo original, sem parser Go (pendentes) | 0 |
| Nunca lidos (ignorados) | 18 |
| Lido pelo original, mas ausente na instalação | 1 (`Custom/CustomPatente.txt`; `LoadCustomPatente` aceita a ausência) |

Antes da conversão para JSON eram 296 arquivos (274 carregados, 22 ignorados): os 16 `GameServerInfo - *.dat` viraram 2 arquivos de instância, os arquivos dos processos viraram `server.json`, e as 4 notas do administrador (`Item/test_item.txt`, `Monster/BALANCE_CHANGELOG.txt`, `Shop/_ALTERACOES.txt`, `EventItemBag/AJUSTE_JOIAS_20_PORCENTO.txt`) saíram do repositório.

Na pasta original existem 346 arquivos. Os 50 que não foram copiados são todos não lidos:

- 8 `.dat` em `4.GameServer/DATA/Backup/`;
- `Data/Monster/_Original_Backup/**` (37 arquivos);
- `Data/Monster/Spawn/backup/006 - Arena.txt` (subpasta, que `0x4bbaa0` pula);
- `Data/Item/BMD_Tool.exe`;
- os `.bmp` gerados pelo XShopRecharge (`RecargaPix.bmp`, `pixA274.tmp.bmp`, `pixqr/25.bmp`).

Nenhum arquivo lido pelo original estava faltando na cópia do repositório, então esta etapa não precisou copiar dados.

### Ignorados (motivo no teste)

- `Common/Protect/{Encrypt,Decrypt}.dat`, `Custom/CustomMix.txt`: nenhum executável de servidor referencia esses arquivos (`Item/test_item.txt` também não; saiu do repositório).
- `Custom/Crywolf.dat`, `Event/Crywolf.dat`, `Event/Kanturu.dat`, `Event/IllusionTemple.dat`: sistemas ausentes. `CIllusionTemple` existe, mas não carrega arquivo.
- `Custom/CustomDailyReward.txt`: o original lê `Custom\DailyReward.txt`.
- `Custom/pixFE31.tmp.bmp`: saída de `GetTempFileName("pix")` do XShopRecharge.
- `Shop/239 - Arena Guard.txt`: o `ShopManager.txt` não cita esse arquivo.
- `EventItemBag/Special/{HideAndSeek,Loteria,MataMata,Quiz,Roleta}.txt`: o `EventItemBagManager.txt` não cita esses arquivos.
- `Script/Script/TemplateScript.lua`, `CustomMonsterDie(Boss).lua` (o `require` está comentado) e `CustomMonsterDie(Boss) - Copia.lua`.

## Parsers novos (`internal/gamedata/datafiles.go`)

Os leitores originais consomem um fluxo de tokens (`FUN_004e0d70` lê o token e `_ftol` converte), sem olhar para as quebras de linha. A vírgula termina um número (`12,015` são dois números) e `*` vale -1. Os parsers Go repetem esse comportamento (`streamRows`).

- **DefaultClassFreebies**: a seção 0 tem `Class Money Points`. A seção 1 tem 11 números: `Class`, item `cc,iii` (fora de 0..0x1FFF vira -1), `Level Durability Skill Luck Option Excellent SetOption Duration`; linhas com `Class` fora de 0..4 são descartadas. A seção 2 tem 8 números: `Class Effect Type Power1..4 Duration`.
- **CommandRequirement** (0x78 bytes): `ReqItemIndex` igual a `*` ocupa um token só; nos outros casos são dois. A linha tem 17 campos depois do item e, por fim, as 5 classes.
- **ExperienceTable**: são lidos 7 números por registro: `MinLevel MaxLevel MinReset MaxReset MinMasterReset MaxMasterReset Rate`, guardados em +0/+4/+0x10/+0x14/+0x18/+0x1c/+0x20 de um registro de 0x24 bytes. O cabeçalho do arquivo documenta 4 colunas `ExperienceRate_AL0..3`, mas o binário lê uma só taxa; colunas a mais começariam o registro seguinte. A busca é `FUN_0047c040`, que usa o nível (`obj+0xe6`), resets (`obj+0x6dc`) e master resets (`obj+0x6e0`) e devolve 100 quando nenhum registro casa. **A busca está ligada em `expTableRate`** (EXP de party em `levelup.go` e EXP solo em `combat.go`, antes da taxa do CustomAttack). Como a tabela vem vazia, o resultado continua 100.
- **MonsterSchedule** (grupos de 0x44 bytes): a seção 0 tem `Group` e o horário em 7 campos; a seção 1 tem `Group EnableMsg DisableMsg NotTimeMsg EnableNpcTime`; a seção 2 tem `Group Monster Map X Y`.
- **CustomGHRS.ini**: `Sunday` vai para `0x8eb6bc` e `Monday..Saturday` para `0x8eb6c0..0x8eb6d4`; `Hour/Min/Sec` ficam em `0x8eb6d8/dc/e0`.
- Os demais parsers seguem a contagem de `_ftol` de cada leitor: MonsterMove usa 6 números, MoveSummon 13, HackSkillSpeedCheck 12, PetDamageTable 6, CustomEvolution 5 e ItemLevel 3 mais o nome (até 0x31 caracteres).

## Pendências

- **Lua** (`Script/*.lua`): os scripts rodam na VM Lua embutida (`lua.go`, `lua_api.go`). O teste de cobertura carrega o `ScriptMain.lua` como o `FUN_004e6860`, confere que os três `require` entraram em `_LOADED` e roda o `CalcCharacter.lua` no estado próprio. O que falta da API está em [17](17-lua.md).
- **Parsers sem uso no jogo**: nenhum. As ligações com o jogo e o que cada uso faz no original estão em [Usos no jogo](#usos-no-jogo).

## Usos no jogo

Cada arquivo foi procurado no `GameServer.exe` (e no `CastleSiege.exe`, para o GHRS) a partir do singleton que o loader preenche. O estado fica em `game/datauses.go` (`w.dus()`), carregado por um `startHook`.

| Arquivo | Uso no original | Go |
|---|---|---|
| `DefaultClassFreebies.txt` | `DGCharacterInfoRecv` (`0x473a10`) chama `FUN_0046d0b0` depois dos pacotes de entrada quando a resposta 04 do DataServer traz `IsNewChar` (pacote `+0x8f0`; o DS zera a coluna na hora). Pela classe: zen (+, teto de 2.000.000.000, `C3 10 22 FE`), pontos (`F3 E0` + save), itens no inventário (`FUN_004753e0` com CL = 0xEB, opções e `Duration`) e efeitos do `EffectList` (`FUN_00478ae0`; `EffectType` ≠ 0 grava o fim em hora Unix) | `freebies.go`, gancho `newCharacterHook` no fim de `enterWorld`; o save no fim substitui o `UPDATE IsNewChar='0'` |
| `CommandRequirement.txt` | `FUN_00440660`, chamado pelo CommandManager (`FUN_00435400`) depois das checagens de delay/GM/nível/reset/zen e do Lua, antes do `switch`. Linhas do comando cujo AL, faixas de nível/reset e coluna da classe (≠ 0 e ≤ ChangeUp+1) casam são checadas; as outras são puladas. Falha (equipamento nos slots 0..11 com `CheckItem`, item/quantidade, efeito, estado de quest, PK level/count, `G_Status`, mapa, retângulo, dia da semana e horas em UTC de `GetSystemTime`) manda `FailMessage` (azul, -1 = nada) e o comando não roda nem é cobrado | `cmdrequirement.go`, gancho `commandRequirementHook` em `command.go` |
| `MonsterMove.txt` | `FUN_004b1450` (timer de 1 s `0x50a140`): por classe, a cada 1000 ms, quando o tempo acaba apaga o NPC atual, sorteia uma linha, cria o NPC lá (`FUN_004b4b00`/`FUN_004b4fe0`) por `KeepTime` s. Falha de índice ou de `SetMonster` encerra a passada | `monstermove.go` (Marlon, 229) |
| `MonsterSchedule.txt` | Não cria monstros: 100 grupos abrem e fecham NPCs. `FUN_004bb560` (1 s): estado 1 → no horário manda `EnableMessage` a todos e abre por `EnableNpcTime` s; estado 2 → `DisableMessage` e volta a esperar. `FUN_004bb8d0`, primeira checagem da conversa com NPC (`FUN_004c2eb0`): NPC de grupo fechado responde `ItsNotTimeMessage` (`C1 01`) e não conversa | `monsterschedule.go`, gancho `npcScheduleHook` no início de `npcTalk` |
| `BlackList.txt` (Data) | IPs: condição do `WSAAccept` do GameServer (`FUN_005054f0` → `FUN_00411f00`), recusa antes de alocar índice. HWIDs: `F3 09` (`FUN_00441730` → `FUN_00411fa0`) fecha o cliente. Não é o `1.ConnectServer/BlackList.txt` (flag `-blacklist`), que só vale para a porta do ConnectServer | `blacklist.go`, gancho `acceptFilterHook` em `World.accept` e `blacklistedHWID` |
| `CustomEvolution.txt` | Só `FUN_004ce0b0`, chamado pelo uso de item (`FUN_00499a20`) para o item 14,211 (0x1cd3) solto sobre outro item. O `/evo` não usa a tabela (completa as quests de classe). Detalhes em [10e](10e-customs-wizardteam.md#customevolution) | `customevo.go`, `case` em `itemuse.go` |
| `CustomMobCombo.txt` | Lista de classes que o push-back não empurra (`FUN_005110e0`/`FUN_005114f0`) | `pushback.go` (agente Q) |
| `CustomGHRS.ini` | Teto diário de resets do `CastleSiege.exe`. Detalhes em [10e](10e-customs-wizardteam.md#ghrs-castlesiegeexe) | `ghrs.go`, ganchos em `/reset`, reset automático e resposta do reset |
| `ItemLevel.txt` | `FUN_00494a40`: nome da linha com o mesmo índice e nível, senão o do `Item.txt`, senão `"Unknown"`. Usado nas mensagens 0x290/0x298 do casamento (com o nível da linha do `CustomMarriage`), nos filtros do CustomPick (`FUN_00455c80`: " +nível" só quando o nível > 0 não tem linha) e no anúncio +13 da Chaos Machine (`FUN_005120b0`, não reconstruído) | `itemlevel.go` (`itemName(index, level)`), `custompick.go` |
| `HackSkillSpeedCheck.txt` | `FUN_00488550`, para jogadores, antes de `C3 08 19` (`FUN_00501570`) e `C3 09 1E` (`FUN_00501740`), que não são enviados se falhar, a cada passo da carga da Nova (`FUN_0050e640`, skill 0x28) e na skill 215 (`FUN_004fdbd0`, não reconstruída). Detalhes em [10](10-sistemas-de-jogo.md) | `skillspeed.go`, ganchos em `sendSkillTo`, `sendSkillUse`, `useSkillArea` e `novaTick` |
| `MoveSummon.txt`, `PetDamageTable.txt` | Ligados pelo agente Q (`round_q.go`, `pet_raven.go`) | — |

