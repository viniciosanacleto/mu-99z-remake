# 17 — Scripts Lua do GameServer

O `GameServer.exe` embute o Lua 5.2.0 (`$LuaVersion: Lua 5.2.0` em `0x5446a8`) e carrega os scripts de `Data/Script`. Este documento descreve a ponte entre o C++ e o Lua: quais funções o servidor registra, quais funções Lua ele chama, quando e com quais argumentos. A reimplementação usa a VM Lua 5.2 em Go puro `github.com/Shopify/go-lua` (`v0.0.0-20261007132305-3f823f845ff9`; o projeto não publica tags, então a versão fica fixada nesse pseudo-version) e fica em `apps/server/internal/game/lua.go` (carga e ganchos) e `lua_api.go` (API). Os testes estão em `lua_test.go`.

## Estados e carga

Existem dois estados Lua:

| Estado | Onde fica | Arquivo | Uso |
|---|---|---|---|
| CScriptLoader | objeto global `0x10d60c8`, estado em `+0x58` | `Script\ScriptMain.lua` e tudo o que ele faz `require` | todas as pontes `BridgeFunction_*` |
| CScript de cálculo | `CServerInfo+0x1b70` (`DAT_010be218`), estado em `DAT_010be21c` | `Script\Character\CalcCharacter.lua` | nenhum (ver abaixo) |

A carga é `FUN_004ebe20`, chamada na partida (lista de loaders de [16](16-cobertura-dados.md)) e pelo item de menu "Reload Script":

1. `FUN_004e6a60`: chama `BridgeFunction_OnShutScript` no estado antigo, se ele existir.
2. `FUN_004e6860("..\Data\Script\ScriptMain.lua")`: `luaL_newstate`, `luaL_openlibs`, troca o `require` global por `FUN_004a5220`, `lua_gc`, registra a API (`FUN_004a3f40`), `luaL_loadfilex` e `lua_pcall(0, 0)`. Se a carga falhar, loga `[ScriptLoader] Could not load '%s'. %s`. Se a execução falhar, loga `[ScriptLoader] Error in Lua-file. %s` e `Error: %s`. Nos dois casos o estado é fechado e `+0x58` fica 0, e então todas as pontes devolvem o valor padrão.
3. `FUN_004e69c0`: chama `BridgeFunction_OnReadScript`.
4. `FUN_004e65e0("..\Data\Script\Character\CalcCharacter.lua")`: o estado de cálculo é criado sob demanda por `FUN_004e8390`, com `luaL_openlibs` e a mesma API. Em seguida vêm `luaL_loadfilex` e `lua_pcall(0, -1)`. As mensagens de erro usam o prefixo `[Script]`.

Em Go, `loadScripts` reproduz essa sequência num `startHook`, e `reloadScripts` repete a carga. O servidor Go não tem o menu da janela, então a recarga só existe como função.

### `require` (`FUN_004a5220`)

- O caminho é `"..\Data\" + "\Script\%s.lua"` formatado com o argumento 1, ou seja `..\Data\\Script\System\ScriptCore.lua` (com a barra dupla). Esse caminho é a chave de `_LOADED` e também o nome do chunk. O segundo argumento (`require('arquivo','chave')`, citado no comentário do `ScriptMain.lua` como "scripts criptografados") é ignorado: o binário não tem decriptação.
- Se `_LOADED[caminho]` for a sentinela `DAT_00772d80` (arquivo em execução, ou seja, um require circular), loga `[ScriptLoader] Error in Lua-file` e devolve nada.
- Um arquivo que **já terminou** é carregado e executado de novo a cada `require`. O código só testa a sentinela e não devolve o valor guardado.
- Falha de leitura: loga `[ScriptLoader] Could not load '%s'. %s` e devolve nada, sem erro Lua.
- Execução: grava a sentinela, chama o chunk com o caminho como único argumento (`lua_callk(1, 1)`, sem proteção, então o erro sobe para quem chamou) e guarda o resultado (ou `true`) em `_LOADED[caminho]`.

Em Go, a sentinela é um userdata completo, porque o go-lua não tem o tipo light userdata.

## CalcCharacter.lua

O `CalcCharacter.lua` é carregado e executado (o que só define as funções `DWPhysiDamageCalculate`, `CalcAttackSuccessRate` e as outras), mas **nunca é chamado**:

- os únicos `lua_getglobal` do binário (`0x51a090`, 18 chamadas) são as 17 pontes `BridgeFunction_*` e um dentro da própria biblioteca Lua (`0x52c6bf`);
- os nomes das funções do script não existem nas strings do executável;
- o estado `DAT_010be21c` só é usado pelos inicializadores preguiçosos do `CServerInfo` (`new`, `openlibs`, `gc`, API) e pelo destrutor;
- `CalcLuaScriptSwitch` é lido por `FUN_004eda70` para `CServerInfo+0xd3c`, e nenhum código lê esse campo. Os outros acessos a `+0xd3c` são de outra estrutura, o objeto (`0x4661ee`).

Por isso o cálculo nativo `FUN_004c9790` (`stats.go`) é o único em vigor. O Go repete a carga, que mostra no log os erros do arquivo, e não faz comparação de resultados, porque o Lua não tem efeito no original. As fórmulas do script (constantes fixas como `Strength / 8`) não foram comparadas com as do `GameServerInfo - Character.dat`, porque não têm efeito.

## Pontes chamadas pelo servidor

Cada ponte (`0x4e69c0..0x4e7860`) entra na seção crítica do loader, faz `lua_getglobal("BridgeFunction_<Nome>")` e confere `lua_type == LUA_TFUNCTION`. Se não for função, loga `Error: Function '<Nome>' is not defined.`. Depois empilha os argumentos (`lua_pushinteger`, `lua_pushstring` para texto) e faz `lua_pcall`; em caso de erro, loga `Error: %s`. As pontes que devolvem valor exigem `lua_isnumber`, senão logam `Error: '<Nome>' must return a value.`, e convertem com `lua_tointegerx`. Se o estado não existir, se a função faltar ou se ocorrer erro, cada ponte devolve o seu padrão.

| Ponte | Função | Chamada em | Argumentos | Retorno (padrão) | Go |
|---|---|---|---|---|---|
| OnReadScript | `0x4e69c0` | fim da carga (`FUN_004ebe20`) | — | — | `loadScripts` |
| OnShutScript | `0x4e6a60` | início da recarga e destrutor `FUN_004e67b0` | — | — | `loadScripts` (só na recarga) |
| OnTimerThread | `0x4e6b00` | timer 9 de `FUN_0047d810` (`CreateTimerQueueTimer`, 1000 ms de espera e período, `FUN_004e0120`) | — | — | `tick1000Hooks` |
| OnCommandManager | `0x4e6ba0` | `FUN_00435400`, depois das checagens de delay, GM, nível, reset, zen e Coin1..3, antes do `CommandRequirement` | índice, código do comando, texto dos argumentos | ≠ 0: o comando foi tratado, cobra e chama `FUN_00436c30` (0) | `commandScriptHook` |
| OnCommandDone | `0x4e6cc0` | fim de `FUN_00436c30` (cobrança e delay) | índice, código | — | `commandDone` |
| OnCharacterEntry | `0x4e6d90` | `FUN_00473a10`, depois que o personagem entra no mundo | índice | — | `enterWorldHooks` |
| OnCharacterClose | `0x4e6e50` | `DelCharacterInfo` `FUN_004c4b40` (objeto jogador em estado 3) | índice | — | `leaveWorldHooks` |
| OnNpcTalk | `0x4e6f10` | `FUN_004c2eb0`, depois do `CMonsterSchedule` (`FUN_004bb8d0`) e antes da quest e do NpcTalk | índice do NPC, índice do jogador | ≠ 0 encerra a conversa (0) | `npcTalk` (`npc.go`) |
| OnMonsterDie | `0x4e7020` | morte em `FUN_004cda80`, vítima tipo 2 | monstro, matador (o dono, no caso de summon) | — | `deathHooks` |
| OnUserDie | `0x4e70f0` | morte em `FUN_004cda80`, vítima tipo 1, depois de PK, guerra e duelo | vítima, matador | — | `deathHooks` |
| OnUserRespawn | `0x4e71c0` | `FUN_004c3b10`, depois de escolher o ponto de respawn | índice, `obj+0x285` | — | `respawnPlayer` (`combat.go`) |
| OnCheckUserTarget | `0x4e7290` | `FUN_0040dca0` (PvP), só para alvo jogador, antes do teste de Authority 8 | atacante, alvo | 0 recusa o alvo (1) | `pvpAllowed` (`pvp.go`) |
| OnCheckUserKiller | `0x4e73b0` | `FUN_0050f330`, com os dois objetos jogadores | matador, vítima | 0: sem PK (1) | `playerKill` (`pk.go`) |
| OnUserItemPick | `0x4e74d0` | `FUN_004982e0`, depois da checagem de loot (`FUN_004adcc0`) | índice, número do item no mapa | 0 recusa (1) | `handlePickup` (`mapitem.go`) |
| OnUserItemDrop | `0x4e75f0` | `FUN_004988f0`, depois da trava de conta (0x6f2), só sem `/superdrop` | índice, slot, x, y | 0 recusa com `C1 05 23 00 slot` (1) | `handleDrop` (`mapitem.go`) |
| OnUserItemMove | `0x4e7720` | `FUN_00499410`, depois das checagens de interface | índice, flag origem, slot origem, flag destino, slot destino | 0 recusa com `C3 0C 24 FF ...` (1) | `handleItemMove` (`itemmove.go`) |
| OnSQLAsyncResult | `0x4e7860` | timer 10 de `FUN_0047d810` (o último timer criado tem período de 100 ms; o id passa em registrador e foi deduzido pela ordem), quando a fila do `SQLAsyncQuery` tem resultado | label, parâmetro, resultado 0/1 | — | não ligada (sem ODBC, a fila nunca anda) |

O `ScriptCore.lua` também define `BridgeFunction_OnUserMove`, e o `TemplateScript.lua` documenta essa ponte, mas o binário nunca a chama (o nome não aparece nas strings).

Os flags de permissão (`obj+0x20a4 + n`, escritos por `PermissionInsert`/`PermissionRemove` e zerados na carga do personagem por `FUN_0050ae50`) também bloqueiam ações no original. Em Go estão ligados os índices 1 (`0x20a5`, mover item), 5 (`0x20a9`, dropar) e 6 (`0x20aa`, pegar item). Os demais são lidos por: 2 `0x49adb0`, 3 `0x49a910`, 4 `0x49a690`, 7 `0x507cf0`/`0x507f80` (trade), 9 `0x42ebd0`/`0x42ee40` (Chaos Machine), 0xb `0x437280`/`0x4d7720`/`0x4d7c10` (chat), 0xc `0x436dd0`/`0x4bff20` (move) e 0xd. Esses ainda não estão ligados.

## Scripts entregues

- **ScriptCore.lua**: define a `BridgeFunctionTable`, a `BridgeFunctionAttach(ponte, função)` e uma `BridgeFunction_*` por ponte. Cada uma percorre as funções anexadas. Nas pontes com retorno, OnCommandManager e OnNpcTalk devolvem 1 no primeiro resultado ≠ 0, e as checagens devolvem 0 no primeiro resultado 0.
- **WelcomeMessage.lua** (OnCharacterEntry / OnCharacterClose). Na entrada envia, nesta ordem:
  1. `NoticeSend(i, 0, format(MessageGet(990), MaxReset))`, onde MaxReset é 25 no sábado e no domingo e 20 nos outros dias, por `os.date("*t").wday`;
  2. `MessageGet(991)` (tipo 1), se resets de hoje ≥ MaxReset;
  3. `MessageGet(992)` (tipo 1), com bônus GHRS. As tabelas do script começam vazias, então os dois avisos condicionais nunca saem;
  4. `NoticeSend(i, 0, MessageGet(154) com o nome)`;
  5. `NoticeSend(i, 1, MessageGet(155 + AccountLevel) com a data de expiração)`;
  6. se `CommandCheckGameMasterLevel(i, 8)`, `NoticeSendToAll(0, "GameMaster %s Online!")`;
  7. três `MessageSend(i, 2, 2, texto)`.

  Na saída, um GM nível 8 gera `NoticeSendToAll(0, "GameMaster %s Offline!")`. As mensagens 991 e 992 não existem nos `Message.txt`. Se aparecessem, viriam como `Could not find message: 991, lang: 0!`.
- **CustomWindowName.lua** (OnTimerThread, a cada segundo): para `i = GetMinUserIndex() .. GetMaxUserIndex()` (0x53FC..0x57E4, inclusive, porque o `for` do Lua inclui o limite e o último índice está fora da tabela), chama `SetObjectWindowTitle(i, "MuOrix 97D - [nome] [Free|VIP] - Resets: %d - Level: %d")`. Todo índice com socket recebe o pacote, mesmo na seleção de personagem.

## Pacotes enviados pela API

| Função | Pacote | Original |
|---|---|---|
| `SetObjectWindowTitle` | `C1 84 F3 E8 texto[128]` (título da janela do cliente) | `FUN_004dcad0`: `vsprintf_s(buf, 0x80, texto, …)`. O texto do script é usado como formato, então `%%` vira `%` |
| `MessageSend` / `MessageSendToAll` | `C1 86 F3 E4 tipo cor texto[128]` | `FUN_004dc890` |
| `NoticeSend` / `NoticeSendToAll` | `C1 (len+0x11) 0D tipo 0 0 0(u16) 0(u32) 0 texto\0` | `FUN_004c07f0` / `FUN_004c0ab0` |
| `NoticeGlobalSend` | `C1 94 21` para o DataServer (aviso do grupo de servidores) | `FUN_00475b50` (`groupNotice`) |
| `FireworksSend` | `C1 07 F3 40 00 x y` para o jogador e o VP2 (−1 usa a posição do objeto) | `0x4a8dd0` |
| `ChatTargetSend` | `C1 len 01 idx(BE) texto` (`FUN_004d9c60`) para o alvo, ou para cada jogador do VP2 quando o alvo é −1 | `0x4a8550` |
| `InventoryDelItemIndex` | `C1 05 28 slot 01` | `0x4a95a0` |
| `PKLevelSend` | `C1 07 F3 08 idx(BE) nível` para o jogador e o VP2 | `FUN_004db9d0` |
| `LevelUpSend` | `C1 30 F3 05` | `FUN_004db550` |

`MessageSendToAll` e `NoticeSendToAll` vão para os jogadores em estado 3, tipo 1, sem contagem de logout (`obj+0x0a < 1`) e sem troca de servidor (`obj+0x964`).

## API registrada (`FUN_004a3f40`)

São 194 nomes (`lua_pushcclosure` + `lua_setglobal`), com as funções em `0x4a5410..0x4ac9b0`. Regras comuns a todas:

- `lua_gettop` é conferido primeiro: `[%d argument's expected]`, `[No argument's allowed]` ou, em `ItemDropEx` (≥ 11), `ItemGiveEx` (≥ 8) e `SQLAsyncQuery` (≥ 1), `[Minimum %d argument's expected]`, sempre via `luaL_error`;
- um índice fora de 0..0x57E3 devolve nada (nil); as funções "de usuário" também devolvem nil quando `obj+0x6c ≠ 1`;
- uma função C que devolve 1 sem empilhar nada devolve o último argumento ao Lua (setters, Send, Delete...). O Go faz o mesmo;
- `lua_tointegerx` foi compilado com `LUA_MSASMTRICK` (MSVC x86): `fistp` com arredondamento ao par mais próximo para int de 32 bits (2.5 → 2; fora da faixa dá 0x80000000). O Go reproduz isso em `luaInt`;
- strings saem de `lua_tolstring`; números são empilhados com `lua_pushinteger`, e os campos de tabela com `lua_pushnumber`.

| Grupo | Funções (endereço) | Comportamento / Go |
|---|---|---|
| Constantes | `GET_ITEM` `0x4a5410` (sec*0x200+idx), `GetMaxIndex`/`GetMaxUserIndex` `0x4a54b0` (0x57E4), `GetMinUserIndex` `0x4a5470` (0x53FC), `GetMinMonsterIndex`/`GetGameServerProtocol`/`GetLicenseId` `0x4a5640` (0), `GetMaxMonsterIndex` `0x4a54f0` (0x5014), `GetGameServerCode` `0x4a5530` (+0x68), `GetGameServerVersion` `0x4a5570` (0x65), `GetGameServerCurUser` `0x4a55b0` (`DAT_00772d44`), `GetGameServerMaxUser` `0x4a55f0` (+0xbc), `GetLicenseCustomerName` `0x4a5680` ("MuOnline") | todas implementadas |
| Getters de qualquer objeto | `GetObjectType` (+0x6c), `GetObjectName` (+0x7a), `GetObjectClass` (+0xe0), `GetObjectLevel` (+0xe6), `GetObjectLive` (+0x6e), `GetObjectLife`/`MaxLife`/`Mana`/`MaxMana` (`_ftol` de +0x110/+0x114/+0x11c/+0x120), `GetObjectBP` (+0x188), `GetObjectMaxBP` (+0x18c), `GetObjectDefense` (+0x368), `GetObjectShield`/`MaxShield` (+0x1bc/+0x1c0), `GetObjectMap`/`MapX`/`MapY`, `GetObjectDeathMap`/`X`/`Y` (+0x1ae/+0x1b0/+0x1b2: mapa e posição do matador, gravados por `FUN_004cda80`) | implementadas |
| Getters de jogador | `GetObjectConnected` (+4), `IpAddress` (+0x19), `HardwareId` (+0x29), `Account` (+0x6f), `Password` (+0x85), `PersonalCode` (+0xb0), `ChangeUp` e `Change` (os dois em `0x4a5ad0`, que lê +0x314, o skin do `/skin`), `LevelUpPoint`, `Money`, atributos base/`Extra` (+0x174..)/`Total` (base + extra)/`Default` (DefaultClassInfo, 0 para classe > 4), `PKCount`/`PKLevel`/`PKTimer`, `Authority` (+0x250), `PartyNumber`, `GuildNumber`/`Status`/`Name`, `GuildRelationship` (`FUN_005091a0`), `GuildUnionNumber`/`UnionName` (guild+0x804; sem guild loga `Usuario sem guild`), `Interface` (+0x3d4>>6), `AccountLevel` (+0x618), `AccountExpireDate` (+0x61c), `Reset` (+0x6dc), `MasterReset` (+0x6e0), `CSGuildSide` (+0x97e), `OfflineFlag` (+0x20d9/+0x2110/+0x249e), `GetObjectIndexByName` (`FUN_0050c6d0`, −1 se não achar) | implementadas. `GetObjectPassword` devolve "" porque o Go só guarda o hash. `MasterLevel`, `MasterPoint` e os quatro `Gens*` são uma única função (`0x4a77d0`) que devolve 0 |
| Setters | `SetObjectLevel` (qualquer objeto, short), `LevelUpPoint` (< 0 → 0), `Money` (0..2e9), `Strength`..`Energy` (entre o padrão da classe e `MaxStatPoint_AL<nível da conta>`), `Leadership` (só Dark Lord), `ChatLimitTime`, `PKCount`/`PKLevel`/`PKTimer`, `Map`/`MapX`/`MapY` (qualquer objeto), `MasterLevel`/`MasterPoint` (+0x5e4/+0x5e8, nada lê), `SetObjectWindowTitle` | só memória, sem pacote (exceto o título); implementados |
| Jogo | `CommandCheckGameMasterLevel` (`FUN_0047db10` ≥ nível), `CommandGetArgNumber`/`String` (`FUN_00435210`/`FUN_004352a0`), `CommandSend` (`FUN_00435400`), `Config{ReadNumber,ReadString,SaveString}` (`GetPrivateProfile*`/`WritePrivateProfileString`, com caminho relativo à pasta da instância), `EffectAdd` (tipo ≠ 0: duração = agora + segundos)/`Del`/`Check`/`Clear`, `Inventory{Get{Wear,Main,Full}Size}` (12/76/108), `InventoryGetItemTable`/`MapGetItemTable` (tabela `Serial Index Level Durability Option1..3 NewOption SetOption JoHOption 380Option SocketOption1..5 SocketOptionBonus`), `InventoryGetItemIndex`/`ItemCount`/`DelItemIndex`/`DelItemCount`/`GetFreeSlotCount`/`CheckSpaceByItem`/`CheckSpaceBySize`, `ItemDropEx`/`ItemGiveEx` (`FUN_004753e0`), `LevelUpSend`, `Log{Print,Color,Console}`, `MapCheckAttr` (fora do mapa = 4), `MessageGet` (idioma `CServerInfo+0x70`, que nada escreve, ou seja Eng), `MoneySend`, `MonsterCreate` (`FUN_004b4b00` + `FUN_004b4fe0`; dir −1 = rand&7)/`MonsterDelete`, `MonsterSummonDelete`, `MoveUser` (gate)/`MoveUserEx` (teleporte), `Message*`/`Notice*`, `PartyGetMemberCount`/`Index` (o argumento é o **número da party**), `ObjectGetCoin` (Coin1..3 = 0, nunca carregados), `Permission{Check,Insert,Remove}` (n < 50), `PKLevelSend`, `QuestStateCheck` (estado 2), `RandomGetNumber` (rand % max(n,1)), `SkinChangeSend`, `UserDisconnect`, `UserGameLogout` (tipo 0..2, contagem 6), `UserCalcAttribute` (`FUN_004c9790`), `UserSetAccountLevel` (JS 0x11 + 0x05, em dias) | implementadas sobre o código do World |
| Sem efeito no original | `MasterLevelUpSend`, `MasterSkillTreeRebuild`, `ObjectAddCoin`, `ObjectSubCoin` | só testam o tipo do objeto e devolvem o último argumento |
| Pendentes | `ItemDrop`/`ItemGive` (item de EventItemBag: `FUN_0048ac60`/`FUN_004912f0`, a escolha da bag não foi reconstruída), `MonsterSummonCreate` (`0x4aad10`: summon com percentuais de vida, dano, defesa e taxa), `CharacterGetRespawnLocation` (`FUN_004c53c0`), `PostSend` (`FUN_005127f0`/`00512920`/`00512a50`/`00512b80` por tipo), `UserActionSend` (`FUN_004da300`), `UserInfoSend` (`FUN_0050be30` + `FUN_004dba70`; o Go só salva o personagem) | registradas com a contagem de argumentos e o retorno do original; logam em nível debug |
| LuaSQL | `SQLConnect`/`SQLAsyncConnect` (DSN, usuário, senha), `SQLDisconnect`, `SQLCheck`, `SQLQuery`, `SQLClose`, `SQLFetch`, `SQLGetResult`/`Number`/`Single`/`String` e as versões `SQLAsync*` (`0x4ac260..0x4ac9b0`, ODBC via `Ordinal_*`) | sem ODBC: a conexão falha com a mensagem `[LuaFunction][SQLConnect] Could not connect to database [%s][%s][%s]` e o resto devolve 0 ou "". Uma ponte para o PostgreSQL fica pendente |

## Compatibilidade do go-lua

- `os.date` não existe no go-lua e foi implementado (`*t`, `!*t` e strftime do locale C), usando o relógio do World.
- O `string.format` do go-lua recusa `%d` com número fracionário (regra do Lua 5.3). O Lua 5.2.0 faz cast. Um invólucro trunca esses argumentos antes de chamar o original.
- Nos dois casos (`luaL_loadfilex`) o arquivo é lido com o nome original (`@..\Data\...`) como nome do chunk.
- A pilha de cada ponte é restaurada ao topo de entrada. Assim uma ponte chamada de dentro de uma função da API (por exemplo `CommandSend` → OnCommandManager) não apaga os argumentos da chamada externa.

## Incógnitas

| Item | Situação |
|---|---|
| Títulos ou mensagens com ≥ 128 bytes (`vsprintf_s` em buffer de 0x80) | o handler de parâmetro inválido do processo não foi identificado; o Go corta em 127 |
| `%` no título da janela (o texto é usado como formato sem argumentos) | o Go só trata `%%` |
| `GetObjectName` e os outros campos de índices vazios ou de jogadores em seleção | o original lê a memória do objeto, que pode ter dados antigos; o Go devolve "" e 0 |
| Ordem exata de OnCharacterEntry em relação aos outros envios de `FUN_00473a10` | o Go chama no fim de `enterWorld`, com os outros `enterWorldHooks` |
| Mapa da `ItemDropEx`/`ItemGiveEx` | passado em registrador para `FUN_004753e0` (o Ghidra perde o valor). O Go usa o argumento 2 e o inventário (0xEB) |
| `UserInfoSend`, `UserActionSend`, `PostSend`, `CharacterGetRespawnLocation`, `MonsterSummonCreate`, `ItemDrop`/`ItemGive` | ver Pendentes |
| Flags de permissão 2, 3, 4, 7, 9, 0xb, 0xc, 0xd | lidos pelo original, ainda não ligados em Go |
| OnSQLAsyncResult e a fila `CLuaQueue` (`FUN_004acdf0`, até 0x400 entradas) | sem banco ODBC; não reproduzido |
