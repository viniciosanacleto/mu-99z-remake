# 11 — Arquivos de configuração e de dados

O emulador original lê dezenas de `.ini`, `.dat` e `.txt` (texto Latin-1/UTF-8 com `\r\n`; formatos nas seções 1 a 4). O serviço Go não lê esses arquivos: o conteúdo deles foi convertido para TOML em `apps/server/config/` (seção 0) pelo `cmd/configconv`, que aplica uma vez as regras do leitor original. Em `apps/server/data/` ficam só os arquivos binários (terreno, scripts Lua, chaves, imagens) e o `MuOnline.sql`, com os caminhos originais.

Licenças dos arquivos originais são ignoradas. Quando uma tarefa precisar de um arquivo original novo: um texto de configuração entra no `configconv` (um grupo novo ou uma tabela num grupo existente) e o TOML gerado vai para `apps/server/config/`; um binário é copiado para `apps/server/data/` com caminho e nome originais.

## 0. Configuração do serviço Go (TOML, `apps/server/config`)

```
config/
  server.toml                 [ConnectServer] ConnectServer.ini + BlackList.txt + ServerList.dat,
                              [JoinServer] JoinServer.ini, [DataServer] BadSyntax.txt
  gameserver/gameserver.toml  4.GameServer/DATA: [Common] [Character] [ChaosMix] [Command]
  gameserver/gameserver-cs.toml 5.GameServerCS/DATA  [Custom] [Event] [Item] [Skill]
  data/<grupo>.toml           Data/**: um arquivo por pasta original
```

| Grupo (`config/data`) | Arquivos originais |
|---|---|
| `common.toml` | `MapManager.txt`, `MapServerInfo.txt`, `MapRateInfo.txt`, `BlackList.txt`, `CommandManager.txt`, `CommandRequirement.txt`, `ConnectMember.txt` (raiz de `Data/`) |
| `character.toml` | `Character/*.txt` |
| `custom.toml` | `Custom/*` menos o XShop (`CreationGold.txt` e `CustomGHRS.ini` como tabelas) |
| `custom-event.toml` | `Custom/Event/*.txt` (células ausentes já com os valores do leitor: -1, mapa/x/y 255) |
| `event.toml` | `Event/*.dat` |
| `eventitembag.toml` | `EventItemBagManager.txt` e as bags que ele cita (`Bag."<caminho com />"`) |
| `hack.toml` | `Hack/*.txt` (`Enc2.dat`/`Dec1.dat` ficam em `data/`) |
| `item.toml` | `Item/*.txt` (`Item.txt` com um array por categoria: `Swords`=0 … `Scrolls`=15) |
| `lang.toml` | `Lang/{Eng,Por,Spn}/{Message,Notice}.txt` |
| `monster.toml` | `Monster/*.txt` e `Monster/Skill/*.txt` |
| `move.toml`, `quest.toml`, `skill.toml`, `util.toml` | `Move/`, `Quest/`, `Skill/` (o `SkillElect.hit` fica em `data/`), `Util/` |
| `shop.toml` | `ShopManager.txt` e as lojas que ele cita (`Shop."<ShopPath>"`) |
| `spawn.toml` | `Monster/Spawn/NNN - Nome.txt` (`Spawn."NNN - Nome"`, linhas na ordem do arquivo com a coluna `Section`) |
| `xshop.toml` | `Custom/CustomXShop.txt` e `Custom/CustomXShopRecharge.txt` (o servidor regrava este arquivo) |

Regras:

- Cada arquivo original vira uma tabela com o nome dele (`[ItemOption]`); uma tabela simples vira `Rows = [...]`, uma linha por inline table. Seções numeradas viram arrays nomeados (o número fica no comentário, ex. `# section 1`). As colunas usam os nomes do cabeçalho do original; a coluna `Comment` traz o comentário `//` da linha.
- Os `.ini`/`.dat` chave=valor viram tabelas com as mesmas chaves, na ordem e com os comentários do original. Um valor é inteiro TOML só quando o valor inteiro é um número (um comentário no fim vira comentário TOML); o resto fica string, e o `gsconfig` lê número de uma string com as regras de `GetPrivateProfileIntA`. As chaves continuam sem distinção de maiúsculas.
- `*` vira `-1`. Item é sempre `"cc,iii"` (texto), inclusive nas tabelas que o original lê como dois números; o loader Go repete a checagem de faixa do original.
- Peculiaridades do leitor original resolvidas na conversão: arquivos lidos como fluxo de números (registros cortados a cada N números, sem respeitar quebras de linha, vírgula como separador, `N` repetindo o número anterior) viram as linhas que o original montaria; linhas que o original descarta pela ordem do arquivo (drop de bag antes da bag, item de gift antes do gift) não entram.
- Os arquivos são UTF-8. Os originais em Windows-1252 (`Lang/*/Message.txt`, `CustomQuiz.txt`, `NpcTalk.txt`, eventos custom) são convertidos, e o servidor converte as strings de volta para Windows-1252 ao carregar, como o cliente espera. A única diferença de bytes em relação ao original é `Item.txt` 12,203 ("Capa del Señor Oscuro"), que estava em UTF-8 no arquivo e agora vai em Windows-1252.
- Arquivos que o original não lê (ver [16](16-cobertura-dados.md)) não são convertidos.
- Escritas em tempo de execução: o reinício do ciclo do Castle Siege grava `Chave = 0` em `[Event]` do arquivo da instância (edição de linha, mantém os comentários); o preço por GM e a recarga do XShop regravam `config/data/xshop.toml` (comentários de cabeçalho mantidos, comentários soltos não). As imagens da recarga (`pix*.tmp.bmp`) ficam em `data/Data/Custom`.
- `-config` (padrão `config`) aponta a pasta para `muserver` e `mueditor`; `-data` aponta os binários. Os arquivos de perfil que os scripts Lua leem e gravam (`GetPrivateProfile*` da API Lua) continuam `.ini`.

Pacotes: `internal/config/tomlconf` (leitura estrita, conversão Windows-1252, codificador com uma linha por registro), `servercfg` (`server.toml`), `gsconfig` (instâncias), `gamedata` (tabelas, com o arquivo decodificado em cache enquanto não muda), `legacy` + `mutext` + `ini` (leitores dos formatos originais, usados só pelo `configconv`).

## 1. Convenções de formato (observado)

- Comentários `//` até o fim da linha.
- Arquivos com várias tabelas usam **seções numeradas**: uma linha só com o número da seção (`0`, `1`, …) seguida das linhas de dados e terminada por `end`.
- `*` = "qualquer"/sem limite (ex. `MinLevel *`).
- Strings entre aspas (`"Kris"`). Índices de item como `cc,iii` (categoria, índice).
- Colunas separadas por espaços/tabs.
- `.ini`/`.dat` de servidor: formato `GetPrivateProfile` (`[Seção]` + `Chave = Valor`, comentários `;`).

O tokenizer desses formatos é `internal/config/mutext`, usado pelo conversor (`internal/config/legacy`).

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
| `Util/ExperienceTable.txt`, `ResetTable.txt`, `ResetLimit.txt`, `DamageTable.txt`, `PetDamageTable.txt`, `Filter.txt` (o `GameMaster.txt` não é convertido: o nível de GM fica em `characters.gm_level`, importado por `cmd/migrate -gamemasters`) | utilitários | [08](08-combate-e-formulas.md), [10](10-sistemas-de-jogo.md) |
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
