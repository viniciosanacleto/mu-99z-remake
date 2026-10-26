# 11 — Arquivos de configuração e de dados

O servidor original lê arquivos texto (`.ini`, `.dat`, `.txt`, Latin-1 com `\r\n`) espalhados pelas pastas dos processos. O serviço Go lê o mesmo conteúdo, convertido para JSON numa pasta só, `apps/server/config/` (seção 0). As seções 1 a 4 descrevem os arquivos originais, que continuam sendo a referência de comportamento.

Licenças dos arquivos originais são ignoradas. Quando uma tarefa precisar de um arquivo original ainda não convertido, copiar de `MuServer/**` e converter com `cmd/configconv` (seção 0.5). Arquivos que não são texto de configuração ficam no formato original em `apps/server/data/`, com os caminhos originais.

## 0. Configuração em JSON (`apps/server/config/`)

### 0.1 Layout

| Arquivo JSON | Arquivos originais |
|---|---|
| `config/server.json` | membros `ConnectServer` (`1.ConnectServer/ConnectServer.ini`), `ServerList` (`1.ConnectServer/ServerList.dat`), `BlackList` (`1.ConnectServer/BlackList.txt`), `JoinServer` (`3.JoinServer/JoinServer.ini`), `BadSyntax` (`2.DataServer/BadSyntax.txt`) |
| `config/gameservers/<instância>.json` | os 8 `<instância>/DATA/GameServerInfo - <Nome>.dat`; cada arquivo vira uma seção (`Common`, `Character`, `ChaosMix`, `Command`, `Custom`, `Event`, `Item`, `Skill`) com as chaves do seu `[GameServerInfo]` |
| `config/data/<caminho>.json` | `MuServer/Data/<caminho>.txt` / `.dat` / `.ini`, mesmo caminho relativo, extensão trocada |

Ficam em `apps/server/data/Data/` (formato original): `Terrain/*.att`, `Hack/Enc2.dat` e `Dec1.dat` (chaves SimpleModulus), `Common/Protect/*.dat`, `Skill/SkillElect.hit`, `Script/**.lua` e as imagens `.bmp` do XShopRecharge (`Custom/`). O `data/DB/MuOnline.sql` continua lá para o `cmd/migrate`.

Flags: `muserver -config config -data data` (`-config` = JSON, `-data` = arquivos no formato original); `-instances` nomeia os arquivos de `config/gameservers/`; `-blacklist` aceita uma tabela JSON (`{"rows": []}` libera `127.0.0.1`). `mueditor -config config`; `muproxy -gs-config config/gameservers/4.GameServer.json`.

### 0.2 Tabelas (arquivos tokenizados)

```json
{
  "sections": [
    {"index": 0, "rows": [
      {"WarningTime": 5, "NotifyTime": 1, "EventTime": 15, "CloseTime": 4}
    ]},
    {"index": 1, "rows": [
      {"StartYear": "*", "StartMonth": "*", "StartDay": "*", "StartDoW": "*", "StartHour": 0, "StartMin": 0, "StartSec": 0}
    ]}
  ]
}
```

- Um bloco por seção numerada (`index`); arquivo sem seções numeradas: `{"rows": [...]}`. `"end": false` marca um bloco que o original não fechava com `end`.
- Cada linha é um objeto: chave = nome da coluna, **na ordem das colunas** (os leitores são posicionais; a ordem das chaves é a ordem dos tokens).
- Os nomes vêm do comentário de cabeçalho do arquivo original (`// Index   Type   Name ...`, `; #  LEVEL  WAITMAP ...`). Uma linha usa o último cabeçalho acima dela com a mesma quantidade de colunas; senão o cabeçalho em vigor ou um anterior dessa largura; senão um cabeçalho mais largo. Colunas sem nome no original ficam `Col1`, `Col2`, ... Exceções do conversor: `ACTIVE SYNTAX PLAYER PATENTE` (3 colunas, `SYNTAX_PLAYER`), `Item/ItemDrop.txt` (sem cabeçalho: nomes da struct `gamedata.ItemDropRule`) e a seção 0 de `Custom/CustomXShop.txt` (`Type`, `Preco`).
- Valores: número JSON quando o token é um inteiro que se relê igual; o resto é string (`"*"`, `"12,015"`, `"010"`, `"1.5"`, textos).
- `"encoding": "windows-1252"`: o original não era UTF-8 (`Lang/*/Message`, `Monster/NpcTalk`, `Custom/CustomQuiz`). O JSON guarda UTF-8 e o leitor devolve os bytes Windows-1252 que o cliente espera. Em arquivos sem esse campo, use só ASCII nos textos.
- Comentários do original não são preservados.

O leitor (`jsonconf.LoadLines`) devolve as mesmas `mutext.Line` que o tokenizador dava ao arquivo original (números de seção, linhas e `end`), então os parsers posicionais de `internal/gamedata` e `internal/game` não mudaram. Os scripts WizardTeam (`Custom/Event/*.txt`, `Custom/CustomPatente.txt`) são convertidos com o tokenizador deles (`mutext.TokenizeScript`: `;` também comenta).

### 0.3 Settings (arquivos `.ini`)

```json
{
  "ConnectServerInfo": {
    "ConnectServerPortTCP": 44405,
    "MaxConnectionIdle": 60
  }
}
```

Seção → chave → valor, na ordem e com a grafia da primeira ocorrência no original. O leitor (`jsonconf.LoadINI`) reproduz `GetPrivateProfileIntA/StringA` (nomes sem diferença de maiúsculas, inteiro lido até o primeiro não dígito, truncamento pelo tamanho do buffer). Vale para `CustomGHRS.ini`, `CreationGold.txt`, `CustomXShopRecharge.txt`, os membros `ConnectServer`/`JoinServer` de `server.json` e as seções dos arquivos de instância.

### 0.4 Escritas em tempo de execução

| Original | JSON |
|---|---|
| `/xshop` de GM reescreve a linha `Type Index Preco` da seção 1 do `CustomXShop.txt` (`FUN_00403920`) | `gamedata.WriteXShopPrice` altera/acrescenta a linha em `Custom/CustomXShop.json` (sem o comentário `// alterado/criado ingame por GM`) |
| upload do GM grava `CustomXShopRecharge.txt` e `pix*.tmp.bmp` (`FUN_00405350`) | `Custom/CustomXShopRecharge.json`; o BMP vai para `data/Data/Custom/` |
| `CCastleSiege::LoadData` zera as chaves de ciclo/estado no `GameServerInfo - Event.dat` (`FUN_0041ab60`) | `gsconfig.Config.WriteKeys` na seção `Event` do arquivo da instância |
| `ConfigSaveString` (Lua, `WritePrivateProfileStringA`) | inalterado: `.ini` do script em `data/<instância>/` (a pasta é criada se faltar) |

### 0.5 Conversão

`go run ./cmd/configconv -from <MuServer> -to config [-report]` converte uma pasta MuServer original. Cada tabela é conferida: os tokens do JSON precisam ser os mesmos do arquivo original, senão a conversão falha. `-report` lista as linhas com colunas sem nome no cabeçalho. Ficam de fora os binários (seção 0.1) e as notas do administrador (`Item/test_item.txt`, `Monster/BALANCE_CHANGELOG.txt`, `Shop/_ALTERACOES.txt`, `EventItemBag/AJUSTE_JOIAS_20_PORCENTO.txt`).

## 1. Convenções de formato (observado)

- Comentários `//` até o fim da linha.
- Arquivos com várias tabelas usam **seções numeradas**: uma linha só com o número da seção (`0`, `1`, …) seguida das linhas de dados e terminada por `end`.
- `*` = "qualquer"/sem limite (ex. `MinLevel *`).
- Strings entre aspas (`"Kris"`). Índices de item como `cc,iii` (categoria, índice).
- Colunas separadas por espaços/tabs.
- `.ini`/`.dat` de servidor: formato `GetPrivateProfile` (`[Seção]` + `Chave = Valor`, comentários `;`).

O tokenizador `internal/config/mutext` trata seções, `*`, aspas e `cc,iii`; o conversor da seção 0 usa o mesmo tokenizador, e `TestDataFileCoverage` carrega todos os arquivos convertidos.

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
