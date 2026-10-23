# 11 — Arquivos de configuração e de dados

O serviço Go deve ler os mesmos arquivos (diretório `Data/` configurável) para reaproveitar o conteúdo existente. Todos são texto Latin-1 com `\r\n`.

Licenças dos arquivos originais são ignoradas. Quando uma tarefa precisar deles, copiar de `MuServer/Data/**` (e dos `.dat`/`.ini` dos processos) para `apps/server/data/`, mantendo caminhos e nomes originais; o diretório padrão de dados do servidor é essa pasta do repositório.

## 1. Convenções de formato (observado)

- Comentários `//` até o fim da linha.
- Arquivos com várias tabelas usam **seções numeradas**: uma linha só com o número da seção (`0`, `1`, …) seguida das linhas de dados e terminada por `end`.
- `*` = "qualquer"/sem limite (ex. `MinLevel *`).
- Strings entre aspas (`"Kris"`). Índices de item como `cc,iii` (categoria, índice).
- Colunas separadas por espaços/tabs.
- `.ini`/`.dat` de servidor: formato `GetPrivateProfile` (`[Seção]` + `Chave = Valor`, comentários `;`).

Recomendação: um único tokenizer (`internal/config/mutext`) com suporte a seções, `*`, aspas e `cc,iii`, e structs por arquivo com tags de coluna; testes que carregam todos os arquivos reais.

## 2. Configuração dos processos

| Arquivo | Seção | Chaves |
|---|---|---|
| `1.ConnectServer/ConnectServer.ini` | `ConnectServerInfo` | ver [03](03-connect-server.md) |
| `1.ConnectServer/ServerList.dat` | — | `ServerCode ServerName IpAddress Port Visible` |
| `2.DataServer/DataServer.ini` | `DataServerInfo` | `DataServerODBC`, `DataServerPort` (+ `DataServerUSER/PASS`) |
| `3.JoinServer/JoinServer.ini` | `JoinServerInfo` | ver [04](04-auth-join-server.md) |
| `*/AllowableIpList.txt` | — | IPs de GS permitidos |
| `2.DataServer/BadSyntax.txt` | — | palavras proibidas em nomes |
| `StartUp Auto Reinicio/StartUp.xml`, `StartUP/` | — | utilitários de start/restart dos processos (substituídos por systemd/containers) |

## 3. `DATA/GameServerInfo - *.dat` (seção `[GameServerInfo]`)

Famílias de chaves (sufixo `_AL0..3` = por nível de conta; `N` = índice numérico):

- **Common**: identidade (`ServerName`, `ServerCode`, `ServerLang`, `ServerType`, `ServerLock`, `ServerPort`, `ServerVersion`, `ServerSerial`, `ServerMaxUserNumber`, limites de nível/reset/master reset, `ServerEncDecKeyN`), endereços DS/JS/CS, `MaxConnectionPerIP/HID`, `Write*Log`, anti-flood e anti-hack (`AntiFlood*`, `CheckSpeedHack*`, `CheckLatencyHack*`, `CheckAutoPotionHack*`, `CheckAutoComboHack*`, `CheckMoveHack*`), anúncios de chaos mix, `MaxItemLevel`, `MaxItemOption`, `PersonalCodeCheck`, `ConnectMemberCheck`, `TeleportAttackCheck`, `EffectOverwriteMode`, `SelfDefenseTime`, modificadores de monstro (`MonsterMaxLifeRate`, `MonsterDefenseRate`, …, `MonsterHealthBarSwitch`), reconexão (`ReconnectTime`, `Reconnect*Switch`), experiência/drop (`ExperienceDeduceRateN`, `DropItemRateN`, `DropItem*`, `MoneyDeduceRate`, `ItemDropTime/Rate`, `MoneyDropTime`, `MoneyAmountDropRate`, `AddExperienceRate`, `AddEventExperienceRate`, `ExperienceMultiplierConstA/B`, `PetExperience*`), PK (`NonPK`, `PKLimit*`, `PKDeathAnnounce`, `PKDown*`), trade/loja/duelo/guild (ver [10](10-sistemas-de-jogo.md)), Elf buffer (`ElfBuffer*`), durabilidade (`Weapon/Armor/Wing/Guardian/Pendant/Ring/PetDurabilityRate`), `TradeItemBlock*`, níveis (`MaxLevel`, `MaxPetLevel`, `MaxLevelUp`, `MaxLevelUpEvent`, `MaxStatPoint`, `<Classe>LevelUpPoint`, `PlusStatPoint`, `PlusStatMinLevel`), criação/exclusão (`CharacterCreateSwitch`, `MGCreateLevel`, `DLCreateLevel`, `CharacterDeleteSwitch`, `CharacterDeleteMaxLevel`), party, joias (`SoulSuccessRate`, `LifeSuccessRate`, `AddLuckSuccessRateN`), frutas, quest em party.
- **Character**: fórmulas e multiplicadores (ver [08](08-combate-e-formulas.md)); `CalcLuaScriptSwitch`.
- **Skill**: `SkillRequiresSwitch`, efeitos (veneno, gelo, decay, ice storm, flechas), Mana Shield, Defense, Heal, Greater Defense/Damage/Life/Mana/Critical, Fire Slash, Stern, Invisibility, imunidades, `SummonMonsterN`, `InfinityArrowSwitch`, `EnableCSSkillsAllMaps`.
- **Item**: anéis de transformação, constantes de asas/pets (Satan, Dinorant, Angel, Dark Horse), taxas de poções por classe, Ale/Olive/Remedy of Love, Potion of Bless/Soul.
- **Event**: ver [10 §4](10-sistemas-de-jogo.md#4-eventos-observado-arquivos-e-chaves).
- **Custom**: Arena, Attack/Offline, EventDrop, Gift, LukeHelper, Marriage, Store/Offline, JewelPack, Pick, eZen, `EnableKillLevelN`, `InfinityPotion`, `BlockSellItemExc`.
- **Command**: `/post`, `/pk`, pontos automáticos, `/bau`, reset e master reset (todas as opções de manter atributos, limites diário/semanal/mensal, pontos por classe).
- **ChaosMix**: taxas por receita e AL.

Valores divergentes entre `4.GameServer/DATA` e `5.GameServerCS/DATA` não foram comparados (U-CFG-01).

## 4. `Data/` (conteúdo de jogo)

| Pasta/arquivo | Descrição | Doc |
|---|---|---|
| `MapManager.txt`, `MapServerInfo.txt`, `MapRateInfo.txt` | mapas, servidores por mapa, taxas | [07](07-mundo-e-objetos.md) |
| `Terrain/TerrainN.att` | atributos de célula 256×256 | [07](07-mundo-e-objetos.md) |
| `Move/Gate.txt`, `Move.txt`, `MoveSummon.txt` | gates e teleporte | [07](07-mundo-e-objetos.md) |
| `Monster/MonsterList.txt`, `Spawn/*.txt`, `MonsterMove.txt`, `MonsterSchedule.txt`, `NpcTalk.txt`, `Skill/*` | monstros | [07](07-mundo-e-objetos.md) |
| `Item/*.txt` | itens, opções, drops | [09](09-itens-e-inventario.md) |
| `EventItemBagManager.txt`, `EventItemBag/**` | bags | [09](09-itens-e-inventario.md) |
| `Shop/*.txt`, `ShopManager.txt` | lojas | [09](09-itens-e-inventario.md) |
| `Skill/*.txt` | skills | [08](08-combate-e-formulas.md) |
| `Character/DefaultClassInfo.txt`, `DefaultClassFreebies.txt` | classes | [08](08-combate-e-formulas.md) |
| `Util/ExperienceTable.txt`, `ResetTable.txt`, `ResetLimit.txt`, `DamageTable.txt`, `PetDamageTable.txt`, `Filter.txt`, `GameMaster.txt` | utilitários | [08](08-combate-e-formulas.md), [10](10-sistemas-de-jogo.md) |
| `Quest/*.txt` | quests | [10](10-sistemas-de-jogo.md) |
| `Event/*.dat` | eventos | [10](10-sistemas-de-jogo.md) |
| `Custom/**` | sistemas custom | [10](10-sistemas-de-jogo.md) |
| `Hack/HackPacketCheck.txt`, `HackSkillSpeedCheck.txt`, `Enc2.dat`, `Dec1.dat` | anti-hack e chaves SimpleModulus | [02](02-protocolo-de-rede.md) |
| `Common/Protect/Encrypt.dat`, `Decrypt.dat` | proteção (uso desconhecido) | [12](12-roadmap-e-incognitas.md) |
| `CommandManager.txt`, `CommandRequirement.txt` | comandos | [10](10-sistemas-de-jogo.md) |
| `BlackList.txt`, `ConnectMember.txt` | acesso | [10](10-sistemas-de-jogo.md) |
| `Lang/{Eng,Por,Spn}/Message.txt`, `Notice.txt` | textos | [10](10-sistemas-de-jogo.md) |
| `Script/**.lua` | Lua | [01](01-arquitetura-alvo.md) |

`Data/Item/BMD_Tool.exe`, `Data/Custom/pix*.bmp`, `_ALTERACOES.txt`, `BALANCE_CHANGELOG.txt`, `_Original_Backup/` e `backup/` são artefatos do administrador, não dados de runtime.
