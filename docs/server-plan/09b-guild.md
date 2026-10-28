# 09b — Guild, aliança (union), hostilidade (rival) e guerra de guild (achados do REA)

Complementa o [10](10-sistemas-de-jogo.md). Observado no `GameServer.exe` e no `DataServer.exe` (endereços citados). Implementado em `apps/server/internal/game/{guild,guild_manager,guild_union,guild_war}.go`, no pacote `apps/server/internal/game/guild` (registro e pacotes) e `internal/store/guild*.go` (migração `0005_guild.sql`).

## 1. Arquitetura original

- O **DataServer** mantém todas as guilds em memória (`CGuildManager`, `0x4158a0`; struct de `0x6c4` bytes) e grava direto no SQL. Carrega na partida com `SELECT * FROM Guild` e `SELECT * FROM GuildMember` (`0x415b90`): o mestre (G_Master) é o slot 0, os demais membros entram nos slots 1..79 na ordem da tabela, todos offline (servidor −1).
- Cada **GameServer** tem um cache próprio (`CGuild`, ponteiro em `obj+0x2f8`) alimentado pelos broadcasts do DataServer (`FUN_00415760` envia a todos os GS vivos). O estado de guerra vive só nesse cache (por instância).
- Struct do DS: `+0 Number`, `+4 Score`, `+8 Name[9]`, `+0x11 Master[11]`, `+0x1c Notice[60]`, `+0x58 Mark[32]`, `+0x78` 80 membros de `0x14` (`name[11]`, `+0xc` servidor, `+0x10` G_Status), `+0x6b8 Type`, `+0x6bc Union`, `+0x6c0 Rival`.
- Struct do GS: igual até `+0x78`; membros de `0x18` (`name[11]`, `+0xc` índice do objeto, `+0x10` servidor, `+0x14` status); `+0x7f8` guild inimiga, `+0x7fc` pedido de guerra, `+0x7fd` guerra ativa, `+0x7fe` tipo (0 guerra, 1 battle soccer), `+0x7ff` time, `+0x800` placar, `+0x801` tipo da guild, `+0x804` union, `+0x808` rival.
- Objeto: `obj+0x2fc` nome da guild, `+0x308` número, `+0x30c` status (`0x80` mestre, `0x40` assistente, `0x20` battle master, `0` membro, `−1` sem guild), `+0x318` o outro lado de um pedido, `+0x3d4` interface (tipo 4 = pedido de entrada `0x101`, 5 = janela de criação `0x140`, 14 = pedido de aliança `0x381`).

Na reconstrução, `guild.Manager` (um por `Hub`, compartilhado pelas instâncias) faz o papel do DataServer; `World.guildAll` substitui o broadcast. A tabela `guilds.id` é o `Number`.

## 2. Protocolo GameServer ↔ DataServer (C1/C2 A0 xx)

| Sub | GS → DS | DS → GS (resposta) | DS (`ds.c`) | GS (handler da resposta) |
|---|---|---|---|---|
| 00 | `C1 40`: nome[9] `[4]`, mestre[11] `[13]`, marca[32] `[24]`, tipo `[56]`, servidor u32 `[60]` | `C1 40 A0 00` (resultado em `[0x3d]`, tipo `[0x3c]`): 3 nome existe, 2 `WZ_GuildCreate` falhou, 0 erro, 1 ok (broadcast) | `0x413f40` → `0x4166b0` | `0x481660` |
| 01 | `C1 18`: guild, mestre | `C1 19` resultado `[0x18]`: 3 não existe, 2 inscrita no cerco, 1 ok (broadcast) | `0x4140d0` → `0x4169c0` | `0x4817d0` |
| 02 | `C1 1C`: guild, membro, servidor | `C1 20` `[0x1c]`: 0 sem guild, 3 já está, 4 já tem linha em GuildMember, 2 cheia, 1 ok | `0x4141f0` → `0x416af0` | `0x481a60` |
| 03 | `C1 18`: guild, membro | `C1 19` `[0x18]`: 3 não é membro, 1 ok | `0x414320` → `0x416cd0` | `0x481c20` |
| 04 | `C1 1C`: guild, membro, status u16, tipo | `C1 1C A0 05` | `0x414440` → `0x4179a0` | `0x481fe0` |
| 05 | `C1 0E`: guild, tipo | `C1 0F A0 06` | `0x414510` → `0x4178c0` | `0x482140` |
| 06 | `C1 49`: guild, aviso[60] (`@>`) | `C1 49 A0 07` | `0x4145c0` → `0x417790` | `0x482320` |
| 07 | `C1 55`: guild, nome, tipo (0 guild / 1 aliança), msg[60] (`FUN_00483500`) | `C1 55 A0 08` a todos | `0x414680` | `0x482430` |
| 08 | `C1 14`: guild, score (`FUN_00483490`) | `C1 14 A0 09` | `0x4146f0` → `0x4176b0` | `0x482630` |
| 09 | `C1 14`: nome, union | `C2 A0 0D` lista | `0x4147a0` | `0x482c40` |
| 0A | `C1 14`: idx1, idx2, relação, guild1, guild2 | `C1 14 A0 0E` | `0x414b60` → `0x416d70` | `0x482db0` |
| 0B | idem (rompimento) | `C1 14 A0 0F` | `0x414e00` → `0x417010` | `0x482fd0` |
| 0C | `C1 1A`: idx, 1, guild[9], alvo[8] | `C1 1C A0 10` | `0x414f90` → `0x417200` | `0x4831f0` |

Broadcasts sem pedido: `C2 A0 0A` (guild completa, `0x415130`; GS `0x4826b0` reenvia `C2 65`/`C2 67` de todos os membros online), `C2 A0 0B` (listas de aliança/rival, `0x415470`; GS `0x482a20` = `CUnionManager`), `C1 08 A0 0C` (aliança dissolvida, `0x415600`; GS `0x482b80`, mensagem 201), `C1 20 A0 04` (membro entrou/saiu, `0x415620`, disparado pelo `0x70`/`0x71` de personagem online; GS `0x481d50`).

SQL usado: `WZ_GuildCreate` (insere Guild e o mestre em GuildMember; `@ErrorCode` 1 = falha no Guild, 2 = falha no membro), `UPDATE Guild SET G_Mark`, `UPDATE GuildMember SET G_Status`, `DELETE FROM Guild/GuildMember WHERE G_Name`, `INSERT INTO GuildMember (Name,G_Name,G_Level,G_Status) VALUES (…,1,%d)`, `DELETE FROM GuildMember WHERE Name`, `UPDATE Guild SET G_Union|G_Rival|G_Score|G_Notice|G_Type`. **O tipo da guild na criação fica só em memória** (só G_Mark e G_Status são gravados) — reproduzido.

## 3. Pacotes do cliente

| Head | Handler | Conteúdo / regras |
|---|---|---|
| NPC 241 | `0x4c2520` | já tem guild ⇒ msg 245; nível < `GuildCreateMinLevel_AL` ⇒ 246; resets < `GuildCreateMinReset_AL` ⇒ 247; senão `C1 03 54` (`0x4da9a0`) se nenhuma interface aberta |
| 54 | `0x484210` | `[3]=1` e nível/resets ok ⇒ `C1 03 55` (abre a janela); senão fecha interface tipo 5 |
| 55 | `0x484310` | `[3]` tipo, `[4..11]` nome, `[12..43]` marca. Caractere proibido (`0x513850`) ⇒ `C1 05 56 05 00`; nome fora de 3..8 ⇒ `56 02`; senão A0 00. Resposta `C1 05 56 result type` (`0x485780`), depois `C2 65`/`C2 67` |
| 57 | `0x4844b0` | fecha a janela de criação |
| 50 | `0x483580` | `[3..4]` índice do mestre. Ordem: alvo não aceita pedidos (`obj+0x434` bit 0) ⇒ `C1 04 51 00`; alvo não é mestre ⇒ 4; já tem guild ⇒ 5; interface ocupada ou guerra ⇒ 6; cerco (estado 7) ⇒ msg 740; nível < 6 ⇒ 7; membros ≥ capacidade ⇒ 2; senão `C1 05 50 idxHi idxLo` ao mestre e os dois entram na interface 4 |
| 51 | `0x483810` | `[3]` aceita, `[4..5]` solicitante; repete as checagens e manda A0 02; resposta `C1 04 51 result` ao novo membro |
| 52 | `0x483a30` | lista (abaixo) |
| 53 | `0x483ed0` | `[3..12]` nome, `[13..22]` código pessoal; intervalo de 300 ms; `GuildDeleteSwitch` 0 ou sem guild ⇒ `C1 04 53 00`; mestre com o próprio nome ⇒ dissolve (A0 01; `GuildOwnerDestroyLimit` + dono do castelo bloqueia); mestre com outro nome ⇒ expulsa (A0 03); membro só com o próprio nome ⇒ sai. Na saída: `C1 04 53 result` e `C1 05 5D idxHi idxLo` (`0x5145e0`) a si e ao viewport |
| 61 | `0x484520` | resposta à guerra (§5) |
| 66 | `0x484890` | `[4..7]` número ⇒ `C1 3C 66`: `[4]` número u32, `[8]` tipo, `[9..16]` nome da aliança, `[17..24]` nome, `[25..56]` marca |
| E1 | `0x484a40` | `[3]` tipo 1..3, `[4]` status, `[5..14]` nome. Sem guild ⇒ `0x10`; a si mesmo, não-membro ou não-mestre ⇒ `0x11`; BM: já há `(nível+nívelMaster)/200+1` ⇒ `0x12`; assistente: já há 1 ⇒ `0x12`; outro status ⇒ `0x12`. Resposta `C1 0F E1 tipo result nome[10]`; sucesso ⇒ `C2 65` do membro e mensagem 190/191/192 (dourada) à guild |
| E2 | `0x484ca0` | `[3]` tipo; sem guild `0x10`, não-mestre `0x11`; `C1 05 E2 tipo result`; mensagem 193 e `C2 65` de todos |
| E5 | `0x484da0` | `[3]` relação (1 aliança, 2 rival), `[4]` pedido (1 unir, 2 romper), `[5..6]` mestre alvo. Códigos em `guild_union.go` (0x10 cerco/msg 742, 0x11, 0x15..0x1E). Rompimento de aliança vai direto ao DS (A0 0B) sem resposta do outro lado. Pedido: `C1 07 E5 rel req idx` e interface 14 nos dois |
| E6 | `0x485280` | `[3]` rel, `[4]` req, `[5]` 1 = sim, `[6..7]` solicitante; não ⇒ `C1 08 E6 rel req 0x20 idx`; sim ⇒ A0 0A / A0 0B; resultados `C1 08 E6` aos dois mestres |
| E9 | `0x485410` | sem aliança ⇒ `C1 04 51 03`; senão `C2 E9 count result nUnion nRival` + 41 bytes `{membros, marca[32], nome[8]}` |
| EB | `0x4854e0` | `C1 0C EB 01 nome[8]`: mestre da aliança expulsa uma guild; `C1 07 EB 01 result 02 01`; mensagens 198/197 |
| chat | `0x4d7720` | `@msg` (guild) e `@@msg` (aliança, só com union) vão como `C1 49 00 nome msg[60]` **com o prefixo** a todos os membros online de todas as instâncias; `@>texto` (mestre) troca `'` por `"` e vira o aviso: `C1 0D` tipo 2 a todos e é gravado em G_Notice; o aviso é reenviado na entrada do membro |

### Layouts de viewport

- `C2 65` (`0x5160a0` para entradas novas do VP, estado 1; `0x516f60` para si mesmo + VP2): `count` e 12 bytes por jogador: `número u32`, `status`, `tipo`, `relação`, `idxHi | 0x80 se mestre`, `idxLo`, 3 bytes de lixo. Na versão individual a relação é 0. Pula Authority 2 e efeito 0x12 (invisível).
- `C2 67` (`0x5162f0` para entradas **visíveis** do VP, estado 2 — reenviado a cada ciclo de 1 s; `0x517080` para si mesmo com relação 0 e para o VP2 com a relação de cada um): 20 bytes `idxHi idxLo ?? ?? número u32 relação u32 nomeDaAliança[8] ???`.
- Relação (`0x486c20` → `0x5091a0`): chave = `G_Union` da guild de quem vê, ou o próprio número; 1 se a guild alvo está na lista da aliança da chave (`G_Union == chave`), 2 se está na lista de rivais (o rival da guild-chave e as guilds da aliança dele).
- **Nenhuma informação de guild vai no `F3 03` nem na entrada `C2 12`**; a lista de personagens só leva o G_Status (`[33]`).

### Lista `C2 52`

`[4]` resultado 1, `[5]` count, `[8..11]` G_Score, `[12]` placar da guerra, `[13..21]` nome do rival, 24 bytes de cabeçalho; 13 bytes por membro: `nome[10]`, byte do servidor (0xFF offline), `0x80 | servidor&0x7F` online ou `servidor&0x7F` offline, status.

### Capacidade (`0x486200`)

DL: `(nível + nívelMaster)/10 + liderança/10`; outros: `(nível + nívelMaster)/10`; com resets > 0 vale direto `GuildInsertMaxUser2` (DL) / `GuildInsertMaxUser1`; teto nesses valores e em 80. Nível master = 0 nesta build.

## 4. Aliança e hostilidade (DS `0x416d70`, `0x417010`, `0x417200`)

- Unir aliança: o solicitante não pode ter union nem rival e a lista da aliança alvo (mestre incluído) deve ter < 5 guilds; grava `G_Union = número do mestre da aliança` nos dois. Mensagens: 194 à guild que entrou, 196 à do mestre.
- Rival: nenhuma das duas pode ter rival e nenhuma pode ser membro (não-mestre) de aliança; `G_Rival` cruzado. Mensagem 195.
- Romper aliança: o mestre da aliança não pode sair; se só restam 2 guilds a aliança é dissolvida (`G_Union=0` nas duas, A0 0C, mensagem 201). Romper rivalidade zera os dois (mensagem 200).
- Mortes entre rivais não dão PK (`0x50f330`), nem mortes em guerra de guild.

## 5. Guerra de guild (`/guildwar`, `/guildsoccer`)

- Pedido `0x485f40` (comandos 1 e 2 do CommandManager): `GuildWarSwitch` e `GuildWarEnable` do mapa; sem guild ⇒ `C1 04 60 03`; em guerra/pedido ou PK > 4 sem `PKLimitFree` ⇒ 4; não-mestre ⇒ 5; mesma guild ou mesma aliança ⇒ nada; guild inexistente ⇒ 0; mestre alvo fora desta instância ⇒ 2; alvo não aceita pedidos ⇒ 4; senão `60 01` ao solicitante e `C1 0C 61 guild[8] tipo` ao mestre alvo. Nome com menos de 3 caracteres ⇒ `60 00`.
- Resposta `61` (`0x484520`): recusa ⇒ `60 06` ao solicitante; aceita (mestre inimigo fora de BC/CC/DS) ⇒ times 0 (quem aceitou) e 1, placar zerado, `C1 0D 62 inimigo[8] tipo time` e `C1 06 64 próprio inimigo 0` aos membros online (no soccer só o mestre e os membros da party dele).
- Morte (`0x4868f0`, chamada da morte do jogador `0x4cda80`): `GuildWarKillScore2` se a vítima é o mestre, senão `KillScore1`; abaixo de `GuildWarScoreMax1` (guerra) / `Max2` (soccer) manda `64` às duas guilds; no limite o vencedor ganha `GuildWarWinnerScore1` (inimigo com 0), `2` (inimigo ≤ metade) ou `3` em G_Score (A0 08 para as duas), `C1 0C 63 1 inimigo` aos vencedores e `63 0` aos perdedores, e o estado é limpo 2 s depois (`FUN_00510550(4)` → `0x4c4120` caso 4).
- Mestre dissolvendo a guild em guerra (`0x486a60`): `63 03` / `63 02` e limpeza imediata; num battle soccer também bola de volta e estado CLEAN (`FUN_00410fd0`, que manda de novo `63 1` ao time 0 e `63 0` ao time 1 porque as guerras ainda estão ativas).
- Regras no ataque (`FUN_0040b110` e o golpe do Dark Raven `FUN_0046b5a0`): atacante numa guerra ativa do tipo 1 (soccer) fora do estado START ⇒ o golpe é engolido (retorna 1 sem efeito); guerra do tipo 0 ⇒ não ataca monstros (retorna 0).

## 5b. Battle soccer (`CBattleSoccer`, `guild_soccer.go`)

Objeto em `DAT_0115f298` (`FUN_0040aa60`, construtor `FUN_00410c50`): `+4` índice da bola, `+8` estado, `+0xc` segundos restantes (`difftime(+0x10, agora)`), `+0x10` fim (time64), `+0x14` tick; dois times de 0x34 bytes em `+0x18` (`+4` party nunca gravada, `+8` guild, `+0xc` 5 × `{índice, mestre}`).

- **Campo**: mapa 6 (Arena), x 54..70, y 141..180 (`FUN_00411590`). **Bola**: o monstro classe 200 "Soccer Ball" do `Spawn/006 - Arena.txt`; `FUN_004b4fe0` guarda o índice em `+4` ao criá-lo. O passo de IA da bola (`FUN_004b4090`) só chama a checagem de gol.
- **Início** (`0x484520`, aceite de `/guildsoccer`): campo ocupado ⇒ os dois pedidos caem e `60 04` a quem respondeu; senão placares zerados e `FUN_004110a0`: time 0 = guild que aceitou, time 1 = a que pediu; cada time recebe os 5 primeiros membros online da lista da guild (ordem dos 80 slots), mestre marcado por G_Status 0x80; bola de volta ao spawn (`FUN_004b4e40`); STAND por 10 s. A mensagem 5 posta 10 s depois para quem aceitou (`FUN_00510550(5, …, 10000)`) não tem caso no `FUN_004c4120` dos jogadores (sem efeito).
- **Máquina de estados** (`FUN_00410ce0`, a cada segundo): todo estado começa por `FUN_00411400`, que tira do time quem saiu do jogo ou do mapa 6 (o teste de retângulo no mapa 6 compara X com as duas pontas da área do gol e nunca vale). STAND (1): com os dois times não vazios espera o fim e passa a START (900 s, `FUN_00410f80`); com um time vazio ⇒ CLEAN. START (2): time vazio ou tempo esgotado ⇒ CLEAN. CLEAN (3, `FUN_00410fd0`): se as duas guerras ainda estão ativas `63 1` ao time 0 e `63 0` ao time 1 (`FUN_00486660`, sempre nessa ordem), limpa as duas guerras, 10 s; ao fim bola de volta e EMPTY (0, `FUN_00410f50` + `FUN_00411390`): todos os membros vão para o gate 17 (Lorencia) e os times esvaziam.
- **Gol** (`FUN_00411640`, só em START): bola dentro da área do time i (`DAT_005479c4`: time 0 x 60..64 y 181..182, time 1 x 60..64 y 139..140) ⇒ bola de volta e `FUN_004867c0(time i, time i^1, GuildWarKillScore3)` (20; `64` às duas guilds abaixo do limite `GuildWarScoreMax2`); no limite (G_Score do vencedor como numa guerra) ⇒ bola de volta, CLEAN — que reporta o **time 0** como vencedor qualquer que seja quem marcou — e depois `FUN_00486660`/`FUN_00486780` do marcador, já sem efeito.
- **Chutes**: todo golpe (CAttack ou Dark Raven) na bola manda a mensagem 6 na hora (`FUN_004b48f0` → `FUN_005114f0`): a bola rola 2 células para longe do atacante e o golpe erra.
- **Mortes** (`0x4868f0`): contam como na guerra (KillScore1/2); no limite de um soccer `FUN_00486660` + `FUN_00486780` imediatos, bola de volta e CLEAN. Uma morte que contou marca obj+0x285 = 2: no renascimento (`FUN_004c3b10` caso 2) a vítima de guild em soccer volta numa célula aleatória do campo (`FUN_004115e0`: x 54 + rand%16, y 141 + rand%39).
- **Intrusos** (`FUN_00510810`, a cada segundo): jogador no campo do mapa 6 sem partida (EMPTY) ou que não é membro de um time ⇒ gate 17.

## 6. Incógnitas

| ID | Descrição |
|---|---|
| U-GUILD-01 | ~~Battle soccer não reproduzido.~~ **Resolvida** (seção 5b). `FUN_0040abd0()+0x15c50` é o mapa da CustomArena (taxa de dano), não o do soccer; `0x50fd50` é o teleporte genérico. |
| U-GUILD-09 | Battle soccer: a party do time (`+0x1c`, iniciada em 0) é usada por `FUN_00411250` ao tirar um membro (`FUN_004d2650`), mas nada a grava; não reproduzido. A checagem de intrusos exige jogador vivo no Go (a condição exata do laço de `FUN_00510810` não foi isolada). |
| U-GUILD-02 | A resposta A0 0C (`C1 08`, sem dados visíveis) não diz qual aliança foi dissolvida; usamos o mestre da aliança. |
| U-GUILD-03 | Quem põe a interface tipo 5 (`0x140`) que 54/55/57 limpam não foi achado; usamos a fala do NPC 241. |
| U-GUILD-04 | Destinatário de `FUN_0050c6d0` nas respostas A0 03/05/06 (ponteiro em registrador): assumido mestre da guild para a lista 52 e o resultado E1/E2. |
| U-GUILD-05 | Ramo de aceitação automática do 50 para mestres em ataque offline (`obj+0x20f8/0x2110/0x2294`) não reproduzido. |
| U-GUILD-06 | Trava de senha custom (`obj+0x24fc`, mensagem 1778) no 53 não reproduzida. |
| U-GUILD-07 | Estado do Castle Siege (`FUN_0040ab00()+4 == 7` e `4 < estado < 8`) aproximado por `castleBattleHook`; dono do castelo por `castleOwnerHook`. |
| U-GUILD-08 | Bytes de lixo de pilha em `C2 65 [9..11]`, `C2 67 [2..3]/[17..19]`, `C1 3C 66 [3]/[57..59]`, `C2 52 [6..7]/[22..23]`: enviados como 0. |
