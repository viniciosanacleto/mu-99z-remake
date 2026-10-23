# 10d — Castle Siege (achados do REA)

Implementação: `apps/server/internal/game/castlesiege*.go`, `internal/gamedata/castlesiege.go`, `internal/store/castle*.go` (migração `0008_castle.sql`) e importação em `internal/migrate/castle.go`. Dados: `apps/server/data/Data/Event/CastleSiege.dat`.

## 1. Binários

- O código do cerco (`CCastleSiege`, `CCastleSiegeSync`, `CCastleSiegeCrown`, `CCastleSiegeCrownSwitch`, `CCastleSiegeWeapon`, `CLifeStone`, `CMercenary`, `CGuardianStatue`) existe **no `GameServer.exe` e no `CastleSiege.exe`**. Os dois têm a mesma lógica; o `CastleSiege.exe` (sha256 `8fff317f87…`, ago/2025) é um build anterior com layout de objeto deslocado (ex.: nome da guild `obj+0x2a0` em vez de `+0x2fc`, zen `+0xe4` em vez de `+0xf8`). Pseudocódigo do intervalo 0x412800–0x425000 do `CastleSiege.exe` em `.re/gscs.c`; os endereços citados abaixo e no código são os do `GameServer.exe` (`.re/gs.c`), salvo indicação.
- Tudo é condicionado a `ServerType = 1` (`CServerInfo+0x6c`, `DAT_010bc714`): só a instância `5.GameServerCS` (código 19) roda o cerco. `CastleSiegeEvent` (`GameServerInfo - Event.dat`) liga a máquina de estados; vem **0** nesta instalação.
- Registradores perdidos pelo Ghidra foram conferidos no assembly do `CastleSiege.exe`: `B2 05/06/07/09/14/15/1F`, `B9 02/05`, `B2 04` sem Sign of Lord, `B2 12` (operação da porta), criação de portão (`DL = 1`), fechamento no início (`DL = 0`), `FUN_0041d070` (cálculo da lista).

## 2. CastleSiege.dat (`FUN_0041ab60`)

| Seção | Conteúdo |
|---|---|
| 0 | `CycleDays` (7) |
| 1 | `State StartDay StartHour StartMinute`; ordenado por estado, cada linha dura até a seguinte: `((ΔDia·24 − h0 + h1)·60 − m0 + m1)·60` s (negativo → log "Date Order is wrong", fica 0) |
| 2 | NPCs: `Class Index DBSave Side DFLevel RGLevel MaxLife X Y Dir` (entrada de 0x54 bytes com valores atuais [7..13] e padrões [14..20]; `Side` não é usado) |
| 3 / 4 | upgrade de defesa do portão / estátua: `Index Valor Joias Zen` |
| 5 / 6 | upgrade de vida do portão / estátua (valor = nova vida) |
| 7 | upgrade de regeneração da estátua |
| 8 / 9 | defesa do portão / estátua por nível DF 0..3 (`obj+0x368/0x36c`) |

Chaves lidas de `GameServerInfo - Event.dat`: `CastleSiegeEvent`, `CastleSiegeCycleStart{Year,Month,Day}` (força o início do ciclo enquanto hoje < início + CycleDays), `CastleSiegeSpecificState` + `CastleSiegeStateStart{Year,Month,Day,Hour,Minute}` (o estado S começa nessa data; até lá fica S−1); depois as nove chaves são regravadas com `0` (na reconstrução só quando alguma não era 0). Outras chaves: `CastleSiegeOffensive/DefensiveWeaponDamage`, `CastleSiegeLower/DecayAccumulatedTimeValue`, `CastleSiegeRegisterMinMasterLevel`, `CastleSiegeRegisterMinGuildMember`, `CastleSiegeDamageRate1/2/3` (Character.dat).

## 3. Máquina de estados (`+0x1c`, processo `FUN_0041b780` a cada 100 ms)

| Estado | Nome | Entrada | Durante |
|---|---|---|---|
| 0 | IDLE1 | restante até 00:00 da data de início | → 1 |
| 1 | SIGN1 (inscrição) | `WZ_CS_ResetCastleSiege` (0x80 0x0E) | aviso 111 a cada 30 min |
| 2 | IDLE2 | — | → 3 |
| 3 | SIGN2 (Sign of Lord) | — | aviso 112 a cada 30 min |
| 4 | IDLE3 | — | aviso 113 a cada 2 h |
| 5 | NOTIFY | lista de guilds do cerco (`FUN_0041e670`) | aviso 114 a cada 30 min |
| 6 | READY | idem | aviso 115 a cada 30 min; nos últimos 5 min aviso 116 (minutos) por minuto |
| 7 | START | §6 | §6 |
| 8 | END | restante até a data final; NPCs da batalha removidos, lados zerados, aviso 110 | → 9 |
| 9 | END_CYCLE | novo início = momento do END_CYCLE do ciclo anterior (`+0x108`), fim = início + CycleDays; `WZ_CS_ModifyCastleSchedule`, `ResetRegSiegeInfo`, `ResetSiegeGuildInfo`; SetState(1) | — |

- O "restante" (`+0x20`, ms) é recalculado a cada 1 s a partir do alvo `+0x24` (`time_t`).
- Avisos: `FUN_00475b50` (DataServer 0x21, notice tipo 0 a todos os servidores do grupo) com o texto do `Message.txt` no idioma padrão.
- A cada 300 s fora do estado 7 o estado dos NPCs é gravado (0x89); a cada 10 s calcula-se a data da próxima batalha (`+0x148`, `FUN_0041d760`).
- **CheckSync** (`FUN_0041d020`, após a resposta 0x81 do DataServer): ciclo vencido → novo ciclo começa hoje; estado fixo → S−1; `SIEGE_ENDED` → END; início no futuro → IDLE1; senão a linha da agenda em curso. Reinício durante o START ou nos últimos 15 min do READY volta para READY com 15 min.
- `CCastleSiegeSync` (`FUN_00425c90` → DataServer 0x80 0x17 → `FUN_00424640` em todos os GS): estado, taxas e dono, a cada 100 ms. Na reconstrução é um objeto por `Hub` (`castleSync`); `castleBattleHook` = Sync estado 7.

## 4. Inscrição

| Pacote | Handler | Regras e resposta |
|---|---|---|
| B2 00 | `0x4203b0` / `0x423280` | `C1 2E B2 00`: [4] resultado `WZ_CS_GetOwnerGuildMaster` (1 dono, 2 sem dono, 3 nome vazio, 4 guild inexistente, 0 sem dados), [5] estado (0xFF parado), [6..11] início do estado, [12..17] fim do estado, [18..23] próxima batalha (ano BE, mês, dia, hora, min), [24..31] guild dona, [32..41] mestre, [42..45] segundos restantes (BE) |
| B2 01 | `0x4204a0` | estado ≠ 1 ou evento desligado → 7; sem guild → 6; não é mestre (`FUN_0041f370`: 0x80 e sem aliança ou mestre dela) → 0; dono/aliança do dono → 3; nível < MinMasterLevel → 5; membros < MinGuildMember → 8; senão `WZ_CS_ReqRegAttackGuild` (6 cheio (100), 2 já inscrita, 3 dona, 4/5/8, 1 ok). `C1 0D B2 01 r guild[8]` |
| B2 02 | `0x420620` / `0x424510` | só estados 1..3 (senão `B2 02 3 0 ""`); mestre; `[4]` = desistir; `WZ_CS_ModifyGuildGiveUp` zera as marcas e devolve **3000 zen por marca**; `C1 0E B2 02 r giveUp guild[8]` |
| B2 03 | `0x420730` / `0x424020` | `C1 13 B2 03 r guild[8] marcas(BE) giveUp seq` (r 1 ou 2) |
| B2 04 | `0x420800` / `0x4243a0` | estado 3, guild, Sign of Lord (14,21)+3; sem ele `C1 11 B2 04 03`; trava `obj+0x6b8`; `WZ_CS_ReqRegGuildMark`; sucesso apaga um Sign; `C1 11 B2 04 r guild[8] marcas(BE)` |
| B4 | `0x421ce0` / `0x424d50` | `TOP 100 ... ORDER BY SEQ_NUM ASC`: `C2 B4 r pad count(u32)` + 14 bytes `{guild[8], marcas BE, giveUp, seq}` |
| B5 | `0x421dd0` | estados 5/6 e lista pronta: `C2 B5 1 pad count` + `{lado, envolvida, guild[8]}`; 2 fora de estado, 3 sem lista |

**Escolha das guilds** (`FUN_00425020`, resposta 0x85 de `WZ_CS_GetCalcRegGuildList`): as três linhas **de maior `SEQ_NUM`** (as inscritas por último que não desistiram) viram os lados 2, 3 e 4 — o predicado de ordenação `FUN_004202d0` compara `+0x1c`, que guarda o `SEQ_NUM`; as marcas não entram no critério. O dono entra como lado 1. Depois `WZ_CS_GetCsGuildUnionInfo` (0x86) acrescenta as guilds das alianças com o mesmo lado e "não envolvidas"; a lista é gravada (0x87, `MuCastle_SIEGE_GUILDLIST`). `WZ_CS_SetSiegeGuildInfo` não devolve linha, então `SIEGE_GUILDLIST_SETTED` nunca fica 1 e a lista é sempre recalculada em NOTIFY/READY. Sem inscritos a lista fica vazia e o START termina na hora ("No Attack Guild": o dono mantém o castelo).

**Lados** (`obj+0x97e`, `+0x980`): `FUN_0041e860` (a cada 5 s no START) copia o lado da guild do jogador; `FUN_00422e00` mostra os efeitos 14..17 ("Guild State 1..4") em quem está no mapa 30 durante START/END.

## 5. NPCs do castelo

- Classes: 215 escudo, 216 coroa, 217/218 interruptores, 219 alavanca, 220 porteiro da Land of Trial, 221/222 estilingues de ataque/defesa, 223 Senior, 224 guarda, 277 portão, 278 pedra da vida, 283 estátua, 286/287 mercenários, 288 canhão. A coroa, os interruptores e os NPCs de serviço vêm do arquivo de spawn do mapa 30.
- Portões e estátuas (`DBSave = 1`) vivem em `MuCastle_NPC`. Na primeira carga (`FIRST_CREATE`) a lista padrão é gravada (0x84). Criação (`FUN_0041dab0`): vida/vida máxima da linha, `obj+0x974 = 1`, defesa da tabela; portões abrem (`FUN_0041e5a0` com DL = 1 tira o atributo 0x10 de x−2..x+3, y..y+2; efeito 13 "Castle Gate State"; `+0x975 = 1`) e ganham uma alavanca (219, vida 10000) 4 células ao sul.
- Morte (`FUN_0041e180`): a linha volta aos padrões; portão destruído fica aberto; última estátua no START abre a sala da coroa (remove o escudo 215, `+0x78 = 1`). Fora do START a linha é apagada do banco (0x80 0x16). No START a verificação a cada 5 s (`FUN_0041df10`) também detecta objetos sumidos.

| Pacote | Handler | Regras |
|---|---|---|
| B3 | `0x4219b0` | dono, mestre/assistente; [3] 1 portões, 2 estátuas; `C2 B3 1 pad count` + 28 bytes `{classe, índice, DF, RG, vidaMáx, vida (u32), X, Y, vivo, pad}`; vida 0 e vida máx padrão se morto; não dono → `B3 2` |
| B2 05 | `0x420930` / `0x423500` | compra: portão = vida padrão × 5, estátua × 3 zen; vivo → 0; sem zen → 3; `WZ_CS_ReqNpcBuy` (4 existe); `C1 10 B2 05 r pad classe índice` |
| B2 06 | `0x420b20` / `0x423910` | reparo: portão `(vida perdida + DF·200000)·5`, estátua `(DF+RG)·1000000 + vida perdida·3`; `C1 18 B2 06 r pad classe índice vida vidaMáx` |
| B2 07 | `0x420d80` / `0x423ab0` | upgrade tipo 1 DF, 2 RG (estátua), 3 vida; nível = atual+1; joias de Guardian (14,31) e zen são tirados **antes** da resposta do banco; erros 7 NPC desconhecido, 6 nível, 5 tipo, 3 zen, 4 joias. O upgrade de DF usa a tabela de defesa **do portão** também para estátuas (`FUN_0041e350`) |
| NPC 219 | `0x426cb0` | no START só defensores, fora dele dono/aliança: `C1 07 B2 11 r portão(BE)` (1 ok com interface 12, 2 sem portão, 4 negado) |
| B2 12 | `0x4221f0` | [4] 1 abrir / 0 fechar, [5..6] portão: bloqueio conforme a operação, efeito 13 acrescentado nos dois casos (o viewport `FUN_00515510` sincroniza com `+0x975`); `C1 08 B2 12 r op portão` e `C1 07 B2 13 op portão` ao viewport do portão |

## 6. A batalha

- **Início** (`FUN_0041cc10`): grava NPCs, apaga e recria os NPCs de batalha (`DBSave = 0`: canhões, escudo, estilingues), remove o escudo se não houver estátua, fecha os portões (DL = 0, remove efeito 13), atualiza lados, tira do castelo quem não é defensor (`FUN_0041f620`: célula aleatória das áreas (35,17)-(144,48) atacantes ou (74,144)-(115,154) lado 1, pacote `C3 08 1C FF mapa x y dir`) e envia `C1 05 B2 17 01` ao mapa 30.
- **Regras** (via ganchos): portões, estátuas e canhões só levam dano no START de atacantes (`FUN_0040b110`); PvP no mapa 30 entre jogadores com lado (proibido no mesmo lado com `CastleSiegeDamageRate2 = 0`, `FUN_0040dca0`); dano PvP × Rate1/100 entre lados e × Rate2/100 no mesmo lado; sem PK para quem tem lado no START, para a aliança do dono dentro do castelo (Y > 0x71) e para vítimas com PK > 5 (`FUN_0050f330`); gate 97 (sala da coroa): no START defensores, e atacantes depois da sala aberta; fora dele dono e aliança (`FUN_0050f6d0`).
- **Interruptores** (`FUN_00426bb0`, proc `FUN_00429750`): atacante a menos de 4 células, com as estátuas destruídas (senão mensagem 121): `C1 09 B2 14 interruptor usuário estado` (1 tomado, 2 ocupado por outro, 0 solto). A cada segundo `C1 1A B2 20` (interruptor, 1, lado, guild — a da aliança se houver —, nome) ao usuário e ao viewport; saindo de 3 células o interruptor é solto. Os dois nas mãos do mesmo lado abrem a coroa (efeito 20, `C1 05 B2 16 00`).
- **Coroa** (`FUN_00426a50`, `FUN_00429570`, `FUN_0041edc0`): mestre atacante envolvido a menos de 4 células com a coroa aberta começa o registro (`C1 0C B2 15 estado pad acumulado(ms)`: 0 início, 1 sucesso, 2 interrompido, 3 outro na coroa, 4 interruptores de outro lado) e `C1 0D B2 18 00 guild` ao mapa. Ficando parado 30 s (menos o tempo acumulado) a guild toma o castelo: B2 15 1, B2 16 2, pedras da vida removidas, lados trocados (os vencedores viram 1, os antigos defensores herdam o número), `B2 18 01 guild`, não defensores saem e `B2 1E` com o tempo. Interrupção soma o tempo menos `CastleSiegeLowerAccumulatedTimeValue` (limite 31000 ms); fora da área (150..200 × 175..225) o acumulado cai `CastleSiegeDecayAccumulatedTimeValue` por segundo.
- **Fim** (`FUN_0041c2e0`/`FUN_0041efc0`): o ocupante vira dono (aviso 119), senão o dono mantém (118) ou ninguém (120); dono novo zera dinheiro e taxas (`WZ_CS_ResetCastleTaxInfo`); grava dono e `SIEGE_ENDED`, NPCs, `C1 05 B2 17 00`, limpa a lista do cerco e vai para END.
- **Tempo restante** `C1 06 B2 1E horas minutos` ao mapa a cada 3 min (a cada minuto nos últimos 3).
- **Minimapa**: `B2 1B` (mestre com lado; `C1 05 B2 1B r`: 1 ok, 2 fora do START, 3 sem direito), `B2 1C` sai da lista, `B2 1D` repassa 4 bytes de comando (`C1 08 B2 1D`) ao mapa 30 e ao mesmo lado. A cada 3 s: `C2 B6 count(u32) {x,y}` com os jogadores do lado e `C2 BB count(u32) {tipo 0/1, x, y}` com portões e estátuas.
- **Estilingues** (`CCastleSiegeWeapon`): fala com 221/222 → interface 15/16 e `C1 08 B7 00 01 tipo npc`; `B7 01 npc zona` sorteia o alvo nas tabelas `DAT_00547990/9a0` (4 retângulos por tipo; a zona 4 do de defesa é vazia) e responde `C1 0A B7 01 01 npc tipo x y` (02 aos que veem o NPC) e `C1 07 B7 02 tipo x y` a quem está a 6 células do alvo; `B7 04 npc` atinge tudo a 3 células 100 ms depois com `CastleSiegeOffensive/DefensiveWeaponDamage` (armadilhas, estilingues e monstros de atributo 6 são poupados).
- **Pedra da vida** (scroll 13,11 +1 largado no chão, `FUN_004a35b0`): mestre atacante no START, uma por guild, fora da torre (151..209 × 176..229); mensagens 122..127. Cresce um nível a cada 12 s (`C1 07 B9 01 índice nível`); no nível 5 cura 1%/s vida, mana e BP do lado a 4 células e serve de ressurreição (`FUN_004a3840`). No `C2 13` o byte alto da classe vira `1 | 0x80 (mesmo lado ou guild) | nível<<4`.
- **Mercenários** (scroll 13,7 +0/+1 → 286/287, `FUN_004b2bd0`): defensores mestre/assistente no START, até 101; mensagens 128..133; escolhem como alvo um jogador visível de outro lado (`FUN_004b2ec0`).
- **Estátua** (`FUN_00481410`): cura 1%/s defensores a 4 células.
- **Ressurreição** no mapa 30: pedra da vida crescida; senão gate 101 (dentro do castelo) para defensores e 100 para os demais.

## 7. Dinheiro e taxas

- `CCastleSiegeSync+8/+0xc/+0x10` = taxas do Chaos Machine, da loja e da Land of Trial; membros da guild dona e da aliança dela não pagam (`FUN_00429b00/bc0`). Os impostos cobrados somam em `Sync+0x14` e vão a cada minuto para o dinheiro do castelo (`WZ_CS_ModifyMoney`, 0x80 0x18) em todos os servidores do grupo. Loja: taxa = taxa·preço/100 e `C1 06 B2 1A 02 taxa` ao abrir; Chaos: `B2 1A 01`.
- Senior (223, `FUN_004c1cf0`): só fora do START, guild dona, mestre/assistente (mensagens 241/242/243); janela `C3 0B 30 0C` sobre a caixa do chaos e `B2 1A 01`.
- Guarda (224): mensagens 244/248 ou janela `30 0D`.
- `B2 08` (mestre dono): `C1 0F B2 08 1 chaos loja dinheiro(BE u64)`.
- `B2 09 tipo taxa(BE)`: tipos 1/2 até 3%, tipo 3 até 300000 zen; `C1 0A B2 09 r tipo taxa(BE)`.
- `B2 10 zen(LE)`: saque pelo mestre dono; `C1 0D B2 10 r dinheiro(BE u64)`.
- `B2 1F aberto`: abre/fecha a Land of Trial (não no START); `C1 06 B2 1F r aberto` (1 ok, 0 em batalha, 2 sem direito).
- Porteiro (220, `FUN_00426d70`): `C1 14 B9 03 tipo aberto pad taxa 300000 10000` (tipo 3 mestre dono, 2 dono/aliança (aberto 0), 1 outros). `B9 05`: entra no gate 95 pagando a taxa (grátis para dono/aliança; fechada só para eles); `C1 05 B9 05 r`.
- `B9 02`: `C1 24 B9 02 marca[32]` da guild dona.

## 8. Persistência (DataServer 0x80..0x89 → `store.CastleStore`)

| DS | Procedimento | Store |
|---|---|---|
| 0x81 | `WZ_CS_GetCastleTotalInfo`, `WZ_CS_GetCastleNpcInfo` | `CastleTotalInfo`, `CastleNPCs` |
| 0x80 01 | `WZ_CS_GetOwnerGuildMaster` | calculado em memória + guilds |
| 0x80 03/04/05/16 | `ReqNpcBuy/Repair/Upgrade`, `DELETE MuCastle_NPC` | `BuyCastleNPC`, `RepairCastleNPC`, `UpgradeCastleNPC`, `RemoveCastleNPC` |
| 0x80 06/07/08/18/19 | `GetCastleTaxInfo`, `ModifyTaxRate`, `ModifyMoney` (+ `MuCastle_MONEY_STATISTICS`), `ResetCastleTaxInfo` | `CastleTaxInfo`, `ModifyCastleTaxRate`, `ModifyCastleMoney`, `ResetCastleTaxInfo` |
| 0x80 09/0B/0C/0E | `ModifyCastleSchedule`, `ModifySiegeEnd`, `ModifyCastleOwnerInfo`, `ResetCastleSiege` | `SetCastleSchedule`, `SetCastleSiegeEnded`, `SetCastleOwner`, `ResetCastleSiege` |
| 0x80 0A/0D/10/12 | `GetGuildMarkRegInfo`, `ReqRegAttackGuild`, `ReqRegGuildMark`, `ModifyGuildGiveUp` | `GuildMarkRegInfo`, `RegisterSiegeGuild`, `RegisterGuildMark`, `GiveUpSiegeGuild` |
| 0x80 1A/1B | `ResetSiegeGuildInfo`, `ResetRegSiegeInfo` | `ResetCastleSiegeGuilds`, `ResetCastleRegSiege` |
| 0x83 / 0x85 | `SELECT TOP 100 … ASC`, `WZ_CS_GetCalcRegGuildList` | `RegisteredSiegeGuilds`, `CalcRegGuildList` |
| 0x84 / 0x89 | `DELETE` + `INSERT MuCastle_NPC` | `ReplaceCastleNPCs` |
| 0x87 / 0x88 | `WZ_CS_SetSiegeGuildInfo`, `WZ_CS_GetSiegeGuildInfo` | `SetSiegeGuildList`, `SiegeGuildList` |
| (guild) | `WZ_CS_CheckSiegeGuildList` | `CheckSiegeGuildList` |

Tabelas PostgreSQL `mu_castle_data`, `mu_castle_npc`, `mu_castle_reg_siege`, `mu_castle_siege_guildlist`, `mu_castle_money_statistics` (nomes `citext` como a colação do SQL Server). `cmd/migrate` importa as cinco tabelas `MuCastle_*`.

## 9. Incógnitas

| ID | Questão |
|---|---|
| U-CS-01 | Canhão (288, `FUN_00418e10`): monta um pacote `C1 xx 10 00` (skill 0) com os não defensores a 7 células e o entrega ao handler 0x10 (`FUN_00500b70`), que recusa a skill 0; depois `FUN_00501740(x,y,0)`. Não reproduzido. |
| U-CS-02 | O byte de minutos do aviso 116 (READY) vem de registrador; usamos `restante/60000 + 1`. |
| U-CS-03 | `FUN_0041fa40` (C2 BB) parece listar portões/estátuas inválidos (condição invertida pelo Ghidra); enviamos os vivos. |
| U-CS-04 | Gates de ressurreição do mapa 30 (`FUN_004c53c0`) perdidos no decompilado; usamos 101 (defensores) e 100 (demais) pela posição no Gate.txt. |
| U-CS-05 | O envio final de `FUN_00422e00` (marca de lado) e de `FUN_004218a0`/`FUN_00422bf0` foi conferido só no `CastleSiege.exe`; `FUN_0041e710` (membro que entra na guild durante o START) não é chamado — o refresh de 5 s cobre o caso. |
| U-CS-06 | Pontuação (`GUILD_SCORE`) gravada pela lista do cerco é lixo de pilha no original; gravamos 0. |
| U-CS-07 | ~~Skills de guild 67–72/75 pendentes.~~ **Resolvida**: implementadas em `skill_guild.go` (doc [08c](08c-skills-efeitos-pvp.md) seção 5b). Confirmado no binário que só dependem do mapa 30/31 (nenhum teste do estado do cerco); Greater Mana filtra pela guild (obj+0x308), Invisibility pela party, Brand não filtra. A Summon Party recusa membros de outro lado (obj+0x97e) na batalha e o MoveSummon fecha o núcleo do castelo (x 0xa1..0xbf, y 0xbc..0xd8) fora da batalha; o Teleport Ally (B0) passa por `FUN_0041f400`/`FUN_00426e80` no mapa 30. O empurrão (`FUN_005110e0`/`FUN_005114f0`) poupa o usuário da coroa (CCastleSiege+0x80) montado em Dark Horse e não atravessa células 0x10 no mapa 30. |
| U-CS-08 | IA dos mercenários usa o resto do `monster_ai` (só a escolha do alvo foi substituída); vida da pedra/mercenários vem do MonsterList. |
