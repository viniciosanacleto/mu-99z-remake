# 10f — Comandos de GM, eventos custom e customs de mundo

Continuação do [10e](10e-customs.md): os índices 100–135 do `CommandManager.txt`, os sete eventos de `Data/Custom/Event/`, o esconde-esconde e os customs de mundo de `Data/Custom/`. Fonte: `GameServer.exe` (`.re/gs.c`, assembly via `objdump` para argumentos em registrador), `DataServer.exe` (`.re/ds.c`) e `MuOnline.sql`.

Implementação: `internal/game/command_gm.go`, `customevent.go`, `customevent_sobre.go`, `customevent_kill.go`, `customevent_duel.go`, `customevent_pair.go`, `customevent_roleta.go`, `custom_world.go`; `internal/gamedata/customevent.go`, `customworld.go`; `internal/store/ban.go`, `customevent.go`; migração `0015_custom_events.sql`. Bloco 12 (customs de mundo restantes, agente P): `custom_world_ext.go`, `customarena.go`, `customeventdrop.go`, `lukehelper.go`, `healthbar.go`, `killcount.go`, `summonscroll.go`, `creationgold.go`, `dailyreward.go`, `patente.go`, `windowtime.go`; `internal/gamedata/customworld_ext.go`; `internal/store/customworld.go`; migração `0017_custom_world.sql`; importação em `internal/migrate/custom_world.go`.

## Comandos de GM (`FUN_00435400`, índices 100–135)

Todos passam pelas checagens comuns do `CommandManager` (o arquivo dá GMLevel 8, ou 32 para `/make` e `/drop`). As mensagens de `FUN_004c08f0` vão para o objeto em `EDI`; conferido no assembly: as de sucesso vão ao GM e as de "você foi alterado" ao alvo. O nome do alvo é o token 0 (`FUN_004352a0(buf, 0xB)` + `FUN_0050c6d0`, nome exato, jogador jogando, sem logout pendente); sem alvo → 0x171. Índices dos tokens numéricos lidos em `EBX` antes de cada `FUN_00435210`.

| Índice | Comando | Função | Regra |
|---|---|---|---|
| 100 | `/gmove nome mapa x y` | `0x4394b0` | `FUN_0050fd50` (teleporte que mantém a direção) do alvo; log `CommandGMMove` |
| 101 | `/gg texto` | `0x439620` | Mensagem 0x172 `"[%s] %s"` (nome do GM, texto inteiro) como aviso tipo 0 ao grupo de servidores (`FUN_00475b50`, DS `C1 94 21`, `groupNotice`) |
| 102 | `/trace nome` | `0x439760` | O GM vai até o alvo (o log chama `CommandTrack`) |
| 103 | `/track nome` | `0x4398a0` | O alvo vem até o GM (log `CommandTrace`) |
| 104 | `/dc nome` | `0x4399e0` | `FUN_005102a0`: `obj+0x0a = 6` no alvo |
| 105 | `/firework` | `0x439b10` | Dez `C1 07 F3 40 00 x y` ao GM e ao viewport: dois `rand` por fogo (r1 e depois r2), `x = X + (r2%5-2)*2`, `y = Y + (r1%5-2)*2` |
| 106 | `/make` | `0x439c60` | `sec idx nível skill luck opção exc set JoH 380 sockets duração` no inventário (`FUN_004753e0` com `CL = 0xEB`); token 0 negativo = quantidade e desloca as colunas. Índice inexistente (`FUN_00513740`) para sem mensagem; senão 0x173 |
| 107 | `/drop` | `0x43a0c0` | Mesmas colunas sem JoH/380, na célula do GM com o GM como dono; com quantidade cada item seguinte sai de `FUN_0050c860(±2, 10 tentativas)` a partir da célula anterior (falha volta à do GM). 0x173 |
| 108 | `/skin nome n` | `0x43a510` | `obj+0x314 = n` e `FUN_0050da50` (reapresenta); o `C2 45` de personagem transformado não existe ainda (U-GM-01) |
| 109 | `/setzen nome n` | `0x43a690` | `n < 1` zera; senão soma até 2.000.000.000 (`FUN_0050cf10`); `C3 10 22 FE` ao alvo |
| 110 | `/hide` | `0x43a850` | Sem o efeito 0x12 (Invisibility): adiciona com duração 0 (permanente) e manda `C1 06 14 01 índice` ao viewport (`FUN_0050d9b0`); com ele: remove e reapresenta (`FUN_0050da50`). `FUN_00515160` pula objetos com 0x12 e Authority 4 ou 8 |
| 111 | `/edit nome nível pontos for agi vit ene lid` | `0x43a950` | Negativo = reset (nível 1, pontos 0, atributos do `DefaultClassInfo`, liderança só DL com o valor da classe 4); positivo soma. Nível limitado a `MaxLevel`, atributos a `MaxStatPoint_AL<nível da conta do alvo>`. EXP sobe ao piso do nível (`FUN_0050be30`), `F3 E0`, `FUN_00475c30` (save), CalcCharacter; 0x174 ao GM e 0x175 (nome do GM) ao alvo se for outro |
| 112 | `/setcoin` | `0x43ade0` | Só registra log de Coin1–3 e manda 0x176/0x177; não há linha no `CommandManager.txt`, não reproduzido |
| 113 | `/setpk nome nível` | `0x43b080` | Nível 0..6; 0 e 6: PKCount 3 e `PKDownRequirePoint3`; 1 e 5: Point2; 2 e 4: Point1; 3: 0. `C1 07 F3 08` ao alvo e viewport; 0x178 ao GM |
| 114 | `/setvip nome nível dias` | `0x43b2f0` | Negativo ou nível > 3 → 0/0. JS `C1 18 11 index conta[10] nível(u16) segundos(u32)` (`FUN_004a2a80`, `WZ_SetAccountLevel`: mesmo nível estende a data, outro começa agora) e JS `C1 12 05 index conta` (`WZ_GetAccountLevel`). A resposta (`FUN_004a2270`): nível < `ServerLock` fecha o jogo; mesmo nível e data só atualizam `obj+0x618/0x61c`; senão mensagem 0x9B + nível com a data. 0x179 ao alvo (se outro) e 0x17A ao GM |
| 115 | `/moveall mapa x y` | `0x43b560` | Todos os jogadores jogando sem logout pendente nem troca de servidor, GM incluído |
| 116 | `/moveguild guild mapa x y` | `0x43b650` | Guild por nome exato (até 8 caracteres, `FUN_00487350`) ou 0x17B; membros online neste servidor |
| 117 | `/mob classe qtd` | `0x43b820` | Zona segura → 0x17C. Qtd 1..50 na célula do GM, direção `rand & 7` (`FUN_004b4b00` + `FUN_004b4fe0`); falha → 0x17D, classe NPC → 0x17E (o objeto é apagado e o laço para) |
| 118 | `/makeset tipo nível luck opção exc set JoH 380 sockets duração` | `0x43bbf0` | Peças das seções 7..11 com índice `tipo` que existem, no inventário, sem skill; 0x173 sempre |
| 119/120 | `/banacc nome`, `/banchar nome` | `0x43bec0`, `0x43c0d0` | DS `C1 18 0F 05/06 index nome[10] 1` (`WZ_BanAccount` na conta do alvo / `WZ_BanCharacter` no nome); 0x2AD ao alvo, `FUN_005102a0`, 0x2AE (nome) ao GM |
| 121 | `/banchat nome minutos` | `0x43c2f0` | Minutos < 1 → 0x2BB (antes de procurar o alvo); `obj+0x16c` (ChatLimitTime) = minutos, `obj+0x170` = 0, 0x2BD (nome, minutos) |
| 122/123 | `/unbanacc conta`, `/unbanchar nome` | `0x43c4a0`, `0x43c5b0` | Menos de 4 caracteres → 0x2B1; DS 0F 05/06 com 0. Resposta (`FUN_0043e710`, `FUN_0043e850`): não encontrado → 0x2B7; senão 0x2AF (nome) e só então cobra o comando |
| 124 | `/unbanchat nome` | `0x43c6c0` | `obj+0x16c = 0`, 0x2BC (nome) |
| 125–127 | `/bcstart`, `/dsstart`, `/ccstart` | `0x43c840` → `0x417c00`; `0x43c910` → `0x470880`; `0x43c9b0` → `0x434080` | `FUN_0048a840` acrescenta à agenda a linha {ano, mês, dia, dia da semana -1, hora, minuto + 1, segundo} de agora e o Init (`FUN_00413880` etc.) põe todos os castelos/salas em EMPTY (expulsando quem estiver dentro); o evento começa em um minuto pelo fluxo normal. No minuto 59 a linha pede minuto 60 e nunca casa |
| 129 | `/setbuff nome buff tipo duração p1 p2 p3 p4` | `0x43ca80` | Buff -1 remove todos os efeitos temporários (`FUN_0047a540`); fora de 0..127 ou ausente do `EffectList.txt` → 0x18B (sem cobrança); efeito ativo é removido, senão adicionado (`FUN_00478ae0`, tipo > 0 guarda `time(NULL) + duração`, U-GM-04) |
| 130 | `/escesc` | `0x45fcd0` | Esconde-esconde (abaixo). Não cobra |
| 131 | `/prainhapvp minutos` | `0x503330` | Abre a Prainha (abaixo). Não cobra |
| 132 | `/matamata minutos classe` | `0x45d9f0` | Abre o Mata-Mata (abaixo). Não cobra |
| 133 | `/dupla minutos` | `0x4cf3a0` | Abre o Mata-Mata de Duplas. Inalcançável com o `PairEvent.txt` original (a sintaxe do jogador também é `/dupla` e o join sempre encerra a linha) |
| 134 | `/patente nome valor` | `0x43f540` | 0x175 (nome do GM) ao alvo se outro; `FUN_00477060(valor em ECX)`. Não cobra |
| 135 | `/rodar minutos` | `0x4e46a0` | Abre a Roleta Russa. Não cobra |

Os três comandos de evento sem argumento numérico respondem com avisos tipo 1 literais: "Erro de sintaxe!" e "Digite: <tempo>" (ou "Digite: <tempo> <class>"); `/matamata` com classe 0/ausente mostra "Exemplo -> Evento para BKs: 3 1", "ID de classes: ", "0: Wizard    | 1: Knight    | 2: Elf", "3: Gladiator | 4: Dark Lord | 5: Todos" (classe 0 não pode ser escolhida).

### Banimentos e persistência

- `WZ_BanAccount` grava `MEMB_INFO.bloc_code`, `WZ_BanCharacter` grava `Character.CtlCode`; ambos devolvem 0 se a linha não existe. Aqui: `store.BanStore` (`accounts.block_code`, `characters.ctl_code`, sem migração nova).
- O login já recusa `block_code ≠ 0` (resultado 5) e a seleção de personagem recusa `CtlCode & 1`. Como o save do personagem grava `ctl_code`, o `/banchar` também marca o `CtlCode` em memória para o save do logout não desfazer o banimento.

### Patente (`FUN_00477060`, DS `FUN_00410ae0`)

- `obj+0x2500 = valor`; o mesmo valor é tirado de qualquer outro jogador online; DS `C1 14 F3 E7 nome[11] 00 valor(u32)` → `UPDATE Character SET Patente = 0 WHERE Patente = N` e `UPDATE Character SET Patente = N WHERE Name` (só com N > 0). A coluna é lida no carregamento (`ds.c` "Patente") e vai no `F3 03` [112].
- Valores dados pelos eventos: Duel 3..8 pela classe do evento (`0x45f5a0`), Death 9, Theft 10, Sobre 11, dupla campeã 12 (primeiro membro) e 10 (segundo, `lea ecx,[esi-3]` com `esi = 0xd`).
- Aqui: coluna `characters.patente` (migração 0015), `store.CustomEventStore.SetPatente`, campo `Character.Patente` carregado no `LoadCharacter` e enviado no `F3 03`.

### Ranking (`FUN_00476fb0`, DS `0x41` / `FUN_004108f0`)

- `C1 2C 41 index conta nome coluna[11] pontos` com os pontos em `ESI`: `UPDATE Character SET %s = %s + %d`. Colunas: `TopDeath`, `TopTheft`, `TopRoleta`, `TopDupla`, `topsurvivor`, `Duelsm`, `Duelbk`, `Duelelf`, `Duelmg`, `Dueldl`, `Duelall`. Todos somam 1, exceto o segundo membro da dupla campeã (13). Aqui: tabela `custom_rank` (nome, coluna, pontos).

## Infraestrutura comum dos eventos

- **Comandos de entrada**: antes dos comandos fixos e da tabela, `FUN_00435400` compara (strcmp) a primeira palavra com a sintaxe do Sobre (`FUN_00503490`, para se devolve 1), Duel (`FUN_0045dcc0`, sempre para), Pair (`FUN_004cf510`, sempre para), Roleta (`FUN_004e4800`, para se 1), Theft (`FUN_00506670`) e Death (`FUN_0045bf60`) — estes dois só deixam a linha seguir com resultado ≠ 0. Uma inscrição bem-sucedida na Prainha devolve 0 e a linha chega ao comando 131: um jogador comum recebe também a mensagem 0x12D, como no original.
- **Bloqueios** (`FUN_004d6690`): `36` trade, `40` party e `AA` duelo de quem tem `obj+0x24f0` ou entrada nas tabelas do Theft (`DAT_0104e8b4`), Death (`DAT_007742b8`) ou Roleta (`DAT_00ff4420`) recebem "Proibido dar trade no evento.", "Proibido formar party no evento." e "Proibido iniciar o duelo no evento." (aviso tipo 1). O mesmo teste é o `commandEventBlockHook` das respostas de reset (**resolve U-CUST-07**).
- **Trade** (`FUN_00507cf0`): antes das checagens, o tiro da Roleta (`FUN_004e4e70`); depois delas, a vitória do esconde-esconde (`FUN_0045fed0`) e, com `obj+0x24f0` em qualquer um dos dois, o pedido é descartado sem resposta (log "Tentei dar Trade durante o Evento").
- **Ataques** (`FUN_0040b110`, atacante em `EBX`, alvo em `EDI`): Death/Theft com inscrições abertas recusam o atacante inscrito ainda em espera (byte +1 da tabela); Sobre (`ESI = alvo`) protege os inscritos nos estados 1 e 2; Duel (`ECX` atacante, `EAX` alvo) e Pair (`ECX` alvo, `EAX` atacante) abaixo.
- **Mortes**: `FUN_0050f330` chama os contadores do Theft e do Death; `FUN_0050e8a0` chama Sobre, Duel e Pair.
- **Saída** (`DelCharacterInfo FUN_004c4b40`, `DelClient FUN_0048a010`, `gObjMoveGate FUN_0050f6d0`, CustomMove): `FUN_00503ee0`, `FUN_005076f0`, `FUN_0045cfb0`, `FUN_0045f680`, `FUN_004d1580`, `FUN_004e5260`.
- **Volta para casa** (`FUN_0050e280`, a cada segundo por jogador vivo): `obj+0x24f9` → elfa para Noria (171 + rand%6, 108 + rand&7), demais para Lorencia (133 + rand%15, 118 + rand%15); `obj+0x24f8` → PK 3 e `F3 08`.
- Os avisos usam `FUN_004c07f0(índice, tipo, …)` (tipo 0 dourado, 1 azul) e `FUN_004c0ab0` (dourado para todos). Os PK dos lutadores: 6 / contagem 3 / `PKDownRequirePoint3`.

## Eventos (`Data/Custom/Event`)

Os arquivos são lidos pelo leitor de script custom (`FUN_00409600` / `FUN_00451390`): seções numeradas fechadas por `end`, comentários `;` e `//`. Célula ausente = -1 (inteiros) ou 0xFF (mapa/x/y).

### Prainha da Morte (`SobreEvent.txt`, `DAT_0104c260`)

- Seção 0: `Active Syntax Patente`; `Level Map X Y Players`. Seção 1: `Dia Hora Minuto Tempo`.
- Início (`FUN_005037a0`): agenda no segundo 0 ou `/prainhapvp`; minutos 0 = 1; contador = minutos × 60 + 1; estado 1; classe 5.
- Inscrição: já inscrito 0x459; nível 0x457; em party 0x458; em duelo 0x45A (tipo 0); senão `obj+0x24f0 = 1`, vai ao local, efeitos limpos, PK 6, 0x45B.
- Estado 1: a cada minuto, a todos: 0x4B0 (minutos), 0x4B1 (sintaxe), 0x4B2 (mínimo). No fim, com `Players` ou mais: 0x4AF global, estado 2 (6 s). Senão a todos 0x4AD e 0x4AE; inscritos perdem `0x24f0`, ganham volta para casa e PK 3.
- Estado 2: 5..1 → 0x4B3 aos inscritos; no fim 0x4B4 global e estado 3 (300 s).
- Estado 3: morto por jogador → 0x4B7 global (nome), `0x24f8/0x24f9`, Lorencia 125,125, sai da lista. Com 0–1 vivos ou tempo esgotado → estado 4.
- Estado 4: com mais de um vivo, 0x4B5 global e a lista é esvaziada (e o 0x4B5 sai de novo); com um, 0x4B6 (nome), bag 0x1F, ranking `topsurvivor`, patente 11 e Lorencia 125,125.

### Death-Match (`DeathEvent.txt`, `DAT_00772d98`) e Theft of Points (`TheftEvent.txt`, `DAT_0104d290`)

- Seção 0: `Active Syntax Patente`; `Level Map X Y`. Seção 1: `Dia Hora Minuto Tempo Duração` (duração × 60000 ms).
- Rodam na thread `FUN_005107e0` (com o Raffle) que dorme `Sleep(60000)` por minuto de inscrição e `Sleep(duração)` durante o evento; aqui é uma linha do tempo não bloqueante.
- Disparo: inscrição aberta, contador e melhor zerados; a cada minuto, global: cabeçalho (0x464 / 0x4B8), "começa em N" (0x465 / 0x4B9), sintaxe (0x466 / 0x4BA). Com menos de 2 inscritos: cabeçalho + 0x469 / 0x4BD a todos; Death limpa `obj+0x24f0/0x24f1/0x24f4` (não a tabela, que continua marcada até o jogador sair) e dá PK 3; Theft limpa a tabela e dá PK 6 / 3 / `PKDownRequirePoint3`. Com 2+: pontuação 0 (Death) ou 1 (Theft), fim da espera, a todos cabeçalho + 0x467/0x468 (0x4BB/0x4BC).
- Inscrição: fechada 0x456; nível 0x457; party 0x458; `obj+0x24f0` 0x459; Death também recusa quem já está na tabela (o Theft não testa e permite inscrever duas vezes); duelo 0x45A; senão contador +1, efeitos limpos, tabela marcada, PK 6, 0x45B e teleporte (Theft teleporta antes da mensagem).
- Abate entre inscritos com o evento rodando: Death +1 (0x46A ao matador); Theft soma os pontos da vítima (0x4BE ao matador, 0x4BF à vítima, que volta a 1); placar 0x46B/0x46C ou 0x4C0/0x4C1 aos inscritos.
- Fim: vence o primeiro (por índice) com a maior pontuação > 0; sem vencedor 0x46E / 0x4C3. Death: bag 0x19, `TopDeath`, 0x46D (nome, kills), patente 9. Theft: bag 0x20, `TopTheft`, 0x4C2, patente 10. Todos os inscritos: volta para casa, PK 3 (Theft zera PKTime), Lorencia 125,125.

### Mata-Mata (`DuelEvent.txt`, `DAT_007a01d8`)

- Seção 0: `Active Syntax Patente`; `Level WaitMap WaitX WaitY FightMap FightX FightY` (bytes intercalados em +0x28..+0x2d). Seção 1: `Dia Hora Minuto Tempo Classe`.
- Inscrição (sempre encerra a linha): só no estado 1; classe errada 0x45C..0x460; já inscrito 0x459; nível 0x457; duelo 0x45A; senão sai da party (dissolve se < 3), `0x24f0`, entra na lista, efeitos limpos, vai ao WAIT, 0x45B.
- Estado 1: a cada minuto cabeçalho da classe (0x46F + 0x470..0x474) + 0x478 + 0x479; no fim, 2+ → 0x476/0x477 e estado 2 em 3 s; senão cabeçalho + 0x475 e os inscritos voltam.
- Estado 2: descarta inválidos do início e do fim da lista; primeiro × último lutam (0x47B a todos, 0x47C aos dois, PK 6, FIGHT com y−1 / y+1, estado 3 com 6 s); um só passa por W.O. (0x47A); lista vazia → estado 5.
- Estado 3: 3..1 → 0x47D; 0 → 0x47E uma vez. Morte de um lutador → estado 4 em 4 s.
- Estado 4: empate (os dois mortos) → 0x47F; senão o outro pontua (0x480, 0x481, 0x482); três pontos vencem: 0x483, vencedor para a lista de vencedores e WAIT, perdedor sai em Lorencia; ambos PK 3 / 3 / 0. Senão nova contagem de 6 s.
- Estado 5: a lista vira a dos vencedores; 0 → fim, 1 → campeão, par → 0x484 ("RODADA FINAL!"), ímpar → 0x485.
- Estado 6 (lutador saiu): o outro vence como no estado 4.
- Estado 7: 0x486 sem ninguém; senão bag 0x1A, ranking por classe (`Duelsm`..`Duelall`), patente 3 + classe, 0x487 a todos, Lorencia.
- Ataque (`FUN_0045f5c0`): no estado 3 os lutadores só atacam um ao outro e só com o contador em 0; ninguém mais os atinge.

### Mata-Mata de Duplas (`PairEvent.txt`, `DAT_00fee628`)

- Mesmo arquivo do Duel sem classe. Inscrição como a do Duel (precisa estar vivo; sem teste de classe).
- Estado 1: linhas 0x488/0x48D/0x48E por minuto (`FUN_004d1d10` monta três buffers que vão a todos); no fim, com 4+ inscritos: `std::sort` dos ponteiros (ordem de índice), duplas consecutivas, o ímpar recebe 0x48A e sai; 0x48B/0x48C e estado 2 em 3 s. Senão 0x488 + 0x489.
- Estado 2: descarta duplas incompletas do início/fim; primeira × última (0x488 + 0x491 com os quatro nomes, estado 3 com 8 s); uma só passa por W.O. (0x48F / 0x490).
- Estado 3: em 5 os quatro recebem 0x492, PK 6 e vão ao FIGHT (dupla 1 em y−1, dupla 2 em y+1, parceiros em x−1/x+1); 3..1 → 0x493; 0 → 0x494.
- Morte: revive em 7 s (`FUN_004d1410` leva ao WAIT quando chega a 1); uma dupla toda morta → estado 4 em 4 s. Decisão (`pairWinnerIndex`), placar 0x496/0x497/0x498, três pontos vencem (0x499 / 0x49A), senão nova contagem de 8 s; tudo morto → 0x495.
- Estado 6 (alguém saiu na luta): a dupla completa vence por W.O.; o primeiro membro volta ao WAIT e o segundo ao FIGHT (como no original).
- Campeões: bag 0x1C (as duas na célula do primeiro), `TopDupla` (+1 e +13), patente 12 e 10, 0x49D / 0x49E, Lorencia.

### Roleta Russa (`RoletaEvent.txt`, `DAT_00ff43e0`)

- Seção 0: `Active Syntax`; `Level Map X Y`. Sem agenda: só `/rodar` começa.
- Inscrição (estado 1): já inscrito 0x459, nível 0x457, duelo 0x45A (todos tipo 0); senão tabela marcada, teleporte, 0x45B.
- Estado 1: minutos (0x49F, 0x4A2, 0x4A3); no fim, 2+ → 0x4A1 e estado 2 (6 s), senão 0x49F + 0x4A0 e os inscritos voltam. Estado 2: 5..1 → 0x4A4; fim 0x4A5 e estado 3.
- Estado 3: um inscrito pedindo trade a um GM (Authority > 1) puxa o gatilho (`FUN_004e4ed0`, thread de 1 s por passo): 0x4A7..0x4AA, depois `rand() % 6 == 3` elimina (0x4AB; morte, volta para casa, Lorencia, sai) ou poupa (0x4AC). Com 0–1 restantes → estado 4.
- Estado 4: bag 0x1E, `TopRoleta`, 0x4A6, fogo de artifício. Sem patente.

### Loteria (`RaffleEvent.txt`, `FUN_004e0770`)

- Seção 0 `Active`, seção 1 `Dia Hora Minuto`. No segundo 0: sorteia `rand() % n` entre os jogadores de Authority 1 (não GM); 0x461 e 0x462 (nome) a todos, bag 0x1D e 0x463 ao sorteado.

### Esconde-esconde (`CEventHideAndSeek`, `EventHideAndSeekSwitch` / `EventHideAndSeekMaxTime`)

- `/escesc`: parado → começa com o GM como alvo (0x68F com o nome, 0x690) por `MaxTime` minutos; rodando → cancela (0x68E).
- A cada segundo (`FUN_0045fbf0`): minutos restantes arredondados para cima (0x692) quando mudam, 0x68C nos últimos 10 s, 0x68D no fim sem vencedor.
- Quem pedir trade ao GM vence (`FUN_0045fed0`): 0x691 (nome), fogo de artifício, bag 0x1B; o trade não abre.

## Customs de mundo (`Data/Custom`)

| Arquivo | Carregador | Uso | Estado |
|---|---|---|---|
| `CustomMove.txt` | `FUN_0044f210` (map por Id) | Primeira palavra do chat (stricmp, `FUN_0044f430`) em `FUN_004d7720`; `FUN_0044f550`: 0xE0 nível mín., 0xE6 máx., 0xE7/0xE8 resets, 0xE9/0xEA master resets, 0xE2 nível de conta ou ocupado, 0xE3 PK (se `PKMove = 0`), Atlans com Uniria/Dinorant 0x112, Icarus sem asas/Dinorant/Fenrir 0xE4; sai dos eventos e teleporta | ✅ |
| `CustomGate.txt` | `FUN_0044b650` | `FUN_0044b850` no proc de 1 s: jogador na célula exata de origem vai ao destino (mesmas mensagens; imobilizado → 0xE2; Icarus aceita também Dark Horse) | ✅ |
| `CustomRespawn.txt` | `FUN_004585f0` | Fim do respawn `FUN_004c53c0` (`FUN_00458770`): por mapa e nível de conta, -1 = célula da morte, 0 = célula da morte com y + rand%3, > 0 = gate, -2 = normal | ✅ |
| `CustomNPCCommand.txt` | `FUN_004503a0` | NPC talk `FUN_004c1490` depois das falas: NPC na célula exata roda o comando (`FUN_00435400`) | ✅ |
| `CustomDeathMessage.txt` | `FUN_0044aac0` | Morte por monstro (`FUN_004cda80` → `FUN_0044ac50`): o monstro "diz" a frase (`FUN_004d9c60`) à vítima | ✅ |
| `CustomArena.txt` | `FUN_00443040` (+ `.dat` em `FUN_00442fd0`) | 30 arenas agendadas no mapa `CustomArenaMapNumber` (abaixo) | ✅ (`customarena.go`) |
| `CustomEventDrop.txt` | `FUN_004623e0` | 10 eventos de chuva de itens (abaixo) | ✅ (`customeventdrop.go`) |
| `CustomLukeHelper.txt` | `FUN_00467550` | NPC que aparece em lugar sorteado (abaixo) | ✅ (`lukehelper.go`) |
| `CustomHealthBar.txt` | `FUN_0044c3b0` (getter `FUN_004d6490`) | Filtro do pacote `C2 F3 E2` de barra de vida (abaixo) | ✅ (`healthbar.go`) |
| `CustomMonsterKillCount.txt` | `FUN_004b8920` | Contador de mortes por classe, DS 0x50 (abaixo); o arquivo original não tem linhas | ✅ (`killcount.go`, tabela `monster_kill_count`) |
| `SummonScroll.txt` | `FUN_00505d20` | Soltar o pergaminho (14,213) invoca monstros (abaixo) | ✅ (`summonscroll.go`) |
| `CreationGold.txt` | `GetPrivateProfileInt` em `FUN_0049a690` | `FA 12`: dois cliques na Jewel of Creation (abaixo) | ✅ (`creationgold.go`, `accounts.gold`) |
| `DailyReward.txt` | `FUN_0045a200` | Janela de recompensa diária `FB 17`/`FB 18`/`FB 02` (abaixo) | ✅ (`dailyreward.go`, tabela `daily_reward`) |
| `CustomPatente.txt` | `FUN_00451f90` | Lista de patentes do pacote `C1 0C F3 E5` (`FUN_00452250`, abaixo); o arquivo não existe nesta instalação → lista vazia, posição -1 | ✅ (`patente.go`) |
| `CustomMobCombo.txt` | `FUN_0044ef50` (o mesmo do CustomGuard) | Lido pelo push-back (`FUN_005110e0`/`FUN_005114f0`): classes que não são empurradas | ✅ (`pushback.go`, agente Q) |
| `CustomDailyReward.txt` | — | Nenhuma string/carregador no binário; arquivo vazio | não usado |

### CustomArena (`CCustomArena`, `DAT_010be4f8`)

- Chaves de `GameServerInfo - Custom.dat` lidas uma vez (`FUN_00442fd0`): `CustomArenaSwitch` (+4), `CustomArenaMapNumber` (+0x15c50), `CustomArenaVictimScoreDecrease`, `CustomArenaKillerScoreIncrease`. Na instalação original o switch é 0 e o mapa -1.
- Arquivo: seção 0 `EventIndex Ano Mês Dia DiaSemana Hora Min Seg` (agenda, `CScheduleManager`); seção 1 `EventIndex Name AlarmTime StandTime EventTime CloseTime StartGate FinalGate MinUser MaxUser MaxDeath MinLevel MaxLevel MinReset MaxReset MinMasterReset MaxMasterReset RewardBagIndex DW DK FE MG DL`. O carregador zera os campos +0x6c/+0x70 (nível master mín./máx.), que o join compara com `obj+0x5e4` (sempre 0).
- Entrada de 0xb9c bytes: +4 estado, +8 restante (s), +0xc alvo, +0x14 inscrição aberta, +0x18/+0x1c minuto, +0x20 nome, regra a partir de +0x40, +0x9c 100 linhas `{índice, pontos, posição, kills, sequência de kills, mortes, sequência de mortes}`.
- Início (`FUN_00442f70`, só com o switch): estado 1 e próxima data; sem agenda → estado 0.
- Estado 1 (`FUN_00443550`): dentro de `AlarmTime` minutos a inscrição abre e cada minuto novo vai a todos (0x1F5, nome, minutos); no zero 0x1F6 (todos), 0x1F7 aos inscritos (tipo 1, `StandTime`) e estado 2.
- Estado 2 (`FUN_00443670`): a cada segundo, menos que `MinUser` (`*` = 1) → todos vão ao `FinalGate` (`FUN_004440f0`) e volta ao estado 1 com a próxima data; no fim 0x1F8 aos inscritos e estado 3 (`EventTime`).
- Estado 3 (`FUN_00443710`): ninguém → recomeça; um só → resultado imediato (sem 0x1F9); fim do tempo → 0x1F9 e resultado.
- Resultado (`FUN_00443920`): posição = 1 + jogadores com mais pontos ou com os mesmos pontos numa linha anterior (`FUN_00444290`); o primeiro é anunciado a todos (0x1FA, nome); **cada** inscrito recebe o bag `RewardBagIndex` (`*` = 0x14) com o índice da arena como SpecialLevel na sua célula e a mensagem 0x1FB (posição, pontos); estado 4 por `CloseTime`, depois recomeça.
- Purga (`FUN_00444150`, no começo de cada estado): sai da lista quem não está jogando ou saiu do mapa da arena.
- Entrada (`FUN_00443c50`, chamada por `CMove::Move` `FUN_004bec30` quando o gate do Move.txt leva ao mapa da arena): em party, guild em guerra, duelo, loja pessoal aberta, já no mapa da arena ou mapa não servido aqui → 0x1FC; o gate tem de ser o `StartGate` de uma arena (senão o move é recusado sem mensagem); inscrição fechada 0x1FD; `MaxUser` (até 100) 0x1FE; nível 0x1FF/0x200, nível master 0x201/0x202, resets 0x203/0x204, master resets 0x205/0x206; coluna da classe 0 ou maior que change-up+1 → 0x207. Sucesso limpa os efeitos temporários e inscreve (já inscrito = recusa silenciosa). Mensagens via `FUN_004dc920` (C1 86 F3 E4, tipo 1 cor 2).
- PvP: no mapa da arena atacante e alvo só se atacam com o atacante inscrito numa arena no estado 3 (`FUN_00443f50` em `FUN_0040dca0`, e nas skills de área `FUN_004f8440`). No viewport (`C2 12`, próprio personagem) o inscrito aparece com nível PK 6 (`FUN_00443f00`).
- Morte (`FUN_00444570` em `FUN_0050e8a0`, vítima no mapa da arena): só a primeira arena em luta é olhada; vítima ou matador fora da lista → nada. Vítima perde min(pontos, VictimScoreDecrease), +1 morte e sequência de mortes, zera a sequência de kills; matador ganha KillerScoreIncrease, +1 kill e sequência, zera a de mortes.
- Respawn (`FUN_004443f0` no `FUN_004c53c0`, só quando o RespawnGate do MapManager do mapa é `*`): inscrito de arena em luta volta no `StartGate`; com `MaxDeath` atingido vai ao `FinalGate` e sai da lista. O CustomRespawn, no fim, ainda pode sobrescrever.

### CustomEventDrop (`CCustomEventDrop`, `DAT_015ae540`)

- `CustomEventDropSwitch` (+4). Seção 0 agenda, seção 1 `EventIndex Name MapNumber X Y DropRadius AlarmTime EventTime`, seção 2 `EventIndex Item Level DropCount DropDelay` (item fora de 0..0x1FFF = -1).
- Estado 1 (`FUN_004628f0`): minutos do alarme a todos (0x20D); no zero 0x20E e estado 2 por `EventTime`.
- Estado 2 (`FUN_00462a00`): cada linha ainda não solta com `DropDelay <= EventTime*60 - restante` solta `DropCount` itens; cada um numa célula a até `DropRadius` do ponto (`FUN_0050c860`, 50 tentativas, senão o ponto), com fogo de artifício `C1 07 F3 40 00 x y` aos jogadores do mapa no alcance de visão (`FUN_00462dd0`) e item sem dono (`FUN_004753e0` com loot -1, o índice do primeiro jogador só serve de rota ao DS). No fim 0x20F, marcas zeradas e próxima data.

### Luke Helper (`CCustomLukeHelper`, `DAT_012524d0`)

- `CustomLukeHelperSwitch` (+0x24). Seção 0 `EventTime`; seção 1 `MapNumber MonsterClass RangeStartX RangeStartY RangeEndX RangeEndY BagSpecial` (chave = número da linha); seção 2 agenda.
- Estado 1 (`FUN_004678e0`): abaixo de 300 s, minutos a todos (0x212); no zero sorteia uma linha (`rand() % linhas`), anuncia 0x213 (nome do mapa no MapManager ou "Unknown") e 0x214, cria o NPC (`FUN_00467e70`: célula aleatória em volta de (StartX, StartY) com raio (EndX-StartX, EndY-StartY), 10 tentativas, direção `rand & 7`) e fica `EventTime` minutos. Fim sem achado: 0x215, NPC removido, próxima data.
- Conversa (`FUN_00468060`, antes das falas do NpcTalk em `FUN_004c2eb0`): dez fogos (`x = X-4 + (r2%5)*2`, `y = Y-4 + (r1%5)*2`, primeiro `rand` para o y) ao jogador e ao viewport, o NPC "diz" 0x216, 0x217 (nome) a todos, NPC removido, e o bag da linha (`*` = 0x15) cai na célula do jogador.

### Barra de vida (`C2 F3 E2`, `FUN_004dc610`)

- A cada segundo, por jogador (`FUN_00510810`), se `MonsterHPBar` do MapManager do mapa for 1 (`*` = `MonsterHealthBarSwitch` do Common.dat): `C2 tamanho F3 E2 [5] quantidade` e entradas `{índice u16 LE, tipo do objeto, vida %}` para os monstros do viewport (estado 1/2, vivos, estado de vida 2, não invocados) cuja primeira linha do `CustomHealthBar.txt` (classe e mapa, `*` = qualquer) tem BarSwitch 1. Porcentagem = `Life*100/(MaxLife+AddLife)` arredondada pelo `fistp`. O pacote vai mesmo vazio.

### MonsterKillCount (`CCustomMonsterKillCount`, `DAT_0125c2fc`)

- Seção 0 `DBIndex MonsterIndex AL0 AL1 AL2 AL3 KillCount Money Message` (std::map por DBIndex; as moedas +0x30..+0x38 não são lidas e ficam 0, então a mensagem 0xB7 nunca sai); seção 1 itens `DBIndex Item Level Durability Skill Luck Option Excellent SetOption Duration`.
- Morte de monstro não invocado (`FUN_004b7290` → `FUN_004b8f90`, com o maior causador de dano): a primeira linha da classe com a coluna do nível de conta ligada manda DS `C1 1E 50 {índice u16, conta[11], nome[11], classe u16}`. O DataServer (`FUN_0040e6d0`) soma 1 em `MonsterKillCount (Name, MonsterClass)` e responde `C1 20 50` com a contagem (u16) em +0x1e.
- Resposta (`FUN_004b9070`): para cada linha da classe, contagem menor → aviso azul `"%s %d/%d"` (nome do monstro, contagem, KillCount); igual → zen (`FUN_0050cf10`, teto 2.000.000.000), itens a ±2 da célula anterior (50 tentativas, falha volta à do jogador; dono = jogador; Duration em segundos), fogo de artifício e a mensagem da linha; para. Maior → nada.

### SummonScroll (`CSummonScroll`)

- Seção 0 `Index Item`; seção 1 `Index Group MonsterClass CreateRate`. No `23` (`FUN_004988f0`), depois dos fogos de artifício: item de pergaminho com o jogador ou a célula de queda em zona segura, ou mapa DS (9, 32), BC (11..17) ou CC (18..23) → a queda falha. Senão as linhas do pergaminho com taxa ≠ 0 formam até 20 grupos; em cada grupo uma linha é sorteada com a taxa como **peso** (`rand() % soma`, então um grupo de uma linha sempre invoca); o monstro aparece a até 3 células da queda (50 tentativas; falha = nada), direção 1. O pergaminho é consumido.

### CreationGold (`FA 12`, `FUN_0049a690`)

- `C1 05 FA 12 slot`. Jogador jogando, sem interface/transação, item (14,22) no slot. `GoldValue` é relido a cada clique (`GetPrivateProfileInt`, padrão 50): < 1 → "O bonus da Jewel of Creation esta desativado.". Primeiro clique (ou outro slot, ou mais de 1,5 s) arma: "Clique novamente com o botao direito para usar a Creation e receber +%d Gold."; segundo clique com menos de 100 ms é ignorado; senão a joia é apagada (`C1 05 28 slot 1`), DS `C1 20 F7 06 {índice, conta[11], nome[11], ouro u32}` (`UPDATE MEMB_INFO SET Gold = ISNULL(Gold,0) + n`, `FUN_00410c50`) e "Voce recebeu +%d Gold de bonus usando a Creation!". Ouro limitado a 2.000.000.000. Aqui: `accounts.gold` (`store.CustomWorldStore.AddGold`).

### DailyReward (`FUN_0045a200`, janela do Main.dll)

- `DailyReward.txt` seção 0: `DAY ITEM LEVEL SKILL LUCK ADDOPTION DUR EXC ANC DURATION INFO INFO` (std::map por dia; item inválido = 0xFFFF).
- Com o personagem o DataServer lê `DailyReward WHERE Acc AND month = MONTH(GETDATE())` (`FUN_0040c4c0`); a resposta 0x68 (`FUN_0045a620`) guarda o dia de hoje e as 31 marcas e manda `C1 2C FB 17` (`FUN_0045a810`: [4] dias do mês — fevereiro 28/29 por ano%4, senão `(m>>3 ^ m) | 30`; [5] dia; [6] dia da semana do dia 1; [7..37] marcas; [40] `time()`) e `C1 E0 FB 18` (`FUN_0045a9d0`: [4] item u16, [6] durabilidade, [7..11] item em 5 bytes de `FUN_0045ab20`, [24] Info1[100], [124] Info2[100]) do dia de hoje.
- A cada segundo (`FUN_0045a150`) um dia novo reenvia os dois.
- `C1 FB 02` (`FUN_0045a7c0`): [4] 0 reenvia, 1 manda o `FB 18` do dia [8], 2 resgata o dia [8] (`FUN_0045abd0`): sem linha "No Reward!"; dia já marcado "You Complete!"; sem área 4x4 livre "Inventory full!"; dia ≠ hoje "You Can't Receive Gifts!"; senão o item vai ao inventário (`FUN_004753e0` com 0xEB), "You Received Success Gifts!", marca, reenvia e salva — o DS 0x68 (`FUN_00410980`) insere `(acc, name, date)` dos dias marcados que ainda não existem no mês.
- **A tabela é `DailyReward (acc, name, date, month)`**, ausente do `MuOnline.sql`. `DailyRewardData` (do script) só aparece nas procedures de rename/delete; nenhum binário a usa. Aqui: `daily_reward` (só o número do mês, como o original).

### Patente (`C1 0C F3 E5`, `FUN_00452250`)

- `[4]` índice do jogador (u32), `[8]` posição (i32): posição 1-based da patente do jogador na lista do `CustomPatente.txt` (leitor de script, seção 0, primeira célula de cada linha, até 50), -1 sem posição, para GM (Authority > 1) ou com o Castle Siege no estado 7.
- Enviado a um jogador para **todos** os jogadores online: no fim de cada viewport add (`FUN_00514700`, todo segundo), para cada entrada nova do `C2 12` (`FUN_00515160`) e no `C2 12` próprio (`FUN_005167b0`). Reproduzido como no original.
- O arquivo não existe nesta instalação (o leitor devolve lista vazia e não acusa erro): todos os pacotes levam -1. A posição exibida depende das imagens do Main.dll.

### Janela de horários de eventos (`C2 F3 EF`, `FUN_00458a70`, **resolve U-CEVT-06**)

- `WindowEventSwitch` (Event.dat). O cliente abre com o head `06` (`FUN_004d7de0`: `obj+0x24a8 = 1`); o `DF` (`FUN_004d80d0`) fecha. Aberta, o proc de 1 s manda `C2 tamanho F3 EF [5] linhas` com linhas de 40 bytes `{nome[32], aberto, 3 de preenchimento, segundos i32}` quando há pelo menos uma.
- Linhas, na ordem: Devil Square (0x2EE), Blood Castle (0x2EF), Chaos Castle (0x2F0) — sala 0, estado 0 fechado, 1 aberto com o restante, outro aberto; cada Invasion, Bonus e DropEvent fora do estado 0 (nome do arquivo, restante no estado 1); cada CustomArena fora do estado 0; Luke Helper (0x2F3) e Castle Deep (0x2F4) como o DS; Death (0x2F9), Duel (0x2FA), Pair (0x2FB), Sobre (0x2FC) e Theft (0x2FD) quando ativos: parado = segundos até o próximo início dos próximos 7 dias (`FUN_0045d0f0`; aberto se ≠ 0; -1 sem agenda), inscrição = aberto, em andamento = aberto com o tempo (Death/Theft: duração da linha que começou neste minuto, `FUN_0045d060`; Duel: o contador).

## Pacotes

| Pacote | Sentido | Layout | Origem |
|---|---|---|---|
| `C1 07 F3 40` | S→C | `[4] tipo 0 [5] x [6] y` | `/firework`, esconde-esconde, Roleta (`FUN_004dcb60`) |
| `C1 06 14 01 idx(BE)` | S→C | destrói o próprio objeto no viewport | `/hide` (`FUN_0050d9b0`) |
| `C1 07 F3 08 idx(BE) nível` | S→C | nível PK | `/setpk`, eventos |
| `C1 18 0F 05/06` | GS→DS | `index(u16) nome[10] … valor(u32)` | ban/unban (`WZ_BanAccount`, `WZ_BanCharacter`); resposta `C1 1C 0F 05/06` com resultado em +0x18 e o valor em +0x14 |
| `C1 18 11` | GS→JS | `index conta[10] nível(u16) segundos(u32)` | `/setvip` (`WZ_SetAccountLevel`) |
| `C1 14 F3 E7` | GS→DS | `nome[11] 00 valor(u32)` | patente |
| `C1 2C 41` | GS→DS | `index conta[10] nome[10] coluna[11] pontos(u32)` | ranking custom |
| `C1 94 21` | GS→DS | aviso ao grupo | `/gg` |
| `C1 0C F3 E5` | S→C | `[4] índice u32 [8] posição i32` | patente (`FUN_00452250`) |
| `C2 F3 EF` | S→C | `[5] n` + n × `{nome[32], aberto, pad[3], segundos i32}` | janela de eventos (`FUN_00458a70`) |
| `06` | C→S | sem corpo | abre a janela de eventos (`FUN_004d7de0`) |
| `C2 F3 E2` | S→C | `[5] n` + n × `{índice u16 LE, tipo, vida %}` | barra de vida (`FUN_004dc610`) |
| `C1 05 FA 12` | C→S | `[4] slot` | CreationGold (`FUN_0049a690`) |
| `C1 20 F7 06` | GS→DS | `índice conta[11] nome[11] ouro u32` | `MEMB_INFO.Gold +=` (`FUN_00477310` → DS `FUN_00410c50`) |
| `C1 1E 50` / `C1 20 50` | GS↔DS | `índice conta[11] nome[11] classe u16` (+ contagem u16 em 0x1e na resposta) | MonsterKillCount |
| `C1 2C FB 17` | S→C | mês da DailyReward | `FUN_0045a810` |
| `C1 E0 FB 18` | S→C | recompensa de um dia | `FUN_0045a9d0` |
| `C1 FB 02` | C→S | `[4] tipo [8] dia u32` | DailyReward (`FUN_0045a7c0`) |
| `C2 00 44 68` | GS→DS | `índice conta nome 1 marcas[31]` | save da DailyReward (`FUN_0045a6f0`) |

## Achados

- **U-CUST-07 resolvida**: as três tabelas por índice são as do Theft (`DAT_0104e8b4`), Death (`DAT_007742b8`) e Roleta (`DAT_00ff4420`); `obj+0x24f0` é o "em evento" do Sobre, Duel e Pair. O mesmo teste bloqueia trade, party e duelo.
- `FUN_00476fb0` recebe os pontos em `ESI` (o `%d` do log "[ RANKING ] … Score %d").
- `FUN_00491430` (bag por SpecialValue) recebe o terceiro campo de comparação em `EDX`; os bags dos eventos usam `*` nos dois campos.
- `obj+0x24f8`/`0x24f9` são consumidos por `FUN_0050e280` (proc de 1 s do jogador).
- `FUN_00443c50`, antes tido como checagem de Castle Siege no `CMove::Move`, é a entrada da CustomArena.
- `FUN_0050c860` recebe os raios em `EAX` (x) e `EDX` (y), o mapa em `ECX`, `&x` em `EDI` e `&y` na pilha.
- `FUN_00491430(bag, especial, mapa, x, y)`: a CustomArena passa o índice da arena como SpecialLevel; o Luke Helper passa -1.
- O proc de 1 s do jogador (`FUN_00510810`) chama, nesta ordem, a janela de eventos (`FUN_00458a70`), a barra de vida (`FUN_004dc610`) e a DailyReward (`FUN_0045a150`).

## Desvios deliberados e quirks reproduzidos

- Reproduzidos: Death cancelado mantém a tabela marcada; Theft permite inscrição dupla; Prainha com vários sobreviventes deixa `0x24f0` marcado em quem estava na lista; sucesso da inscrição na Prainha chega ao comando 131; `/dupla` de GM inalcançável; ForceStart no minuto 59 nunca dispara; segundo membro da dupla campeã ganha 13 pontos e patente 10; no W.O. por saída o segundo membro vai ao FIGHT.
- Reproduzidos (bloco 12): a patente de todos os jogadores vai a cada jogador a cada segundo e a cada entrada nova do `C2 12`; todo inscrito da arena recebe o bag, não só o vencedor; arena com um só lutador termina sem 0x1F9; a morte na arena só olha a primeira arena em luta; peso do SummonScroll (grupo de uma linha sempre invoca); DailyReward guarda só o mês (linhas do mesmo mês do ano anterior contam); a janela mostra -1 para evento custom sem agenda; o `/move` para o mapa da arena por um gate que não é StartGate é recusado sem mensagem.
- Desvios: Death/Theft não bloqueiam o Raffle (no original a mesma thread dorme durante eles); Roleta vazia no estado 4 não trava (o original lê o vetor vazio); `rand()` é o gerador dos monstros.

## Pendências

- Customs de mundo: nenhum pendente; ficam as incógnitas U-CW-01..10. `CustomDailyReward.txt` continua sem uso no binário (`CustomMobCombo.txt` é a lista de classes sem push-back, ver 08c).
- Os campos da janela de eventos do Pair/Sobre (U-CW-07) e o leitor de `obj+0x2a8` (U-CW-02) pedem análise com REA.
- Validar com o cliente original (Main.dll) o `C2 F3 EF`, o `C2 F3 E2`, o `C1 0C F3 E5` e as janelas `FB 17`/`FB 18`.

## Incógnitas

| ID | Questão |
|---|---|
| U-GM-01 | `/skin` grava `obj+0x314`, mas o viewport `C2 45` (`FUN_00515db0`, personagem transformado) não existe; os outros veem o personagem normal |
| U-GM-02 | `/make`, `/drop`, `/makeset`: JoH, opção 380, sockets e duração (`CPeriodicItem`) não têm efeito em itens 97D aqui |
| U-GM-03 | O texto da data de expiração da resposta JS 05 (`AccountExpireDate` lido como string ODBC) é assumido `AAAA-MM-DD HH:MM:SS` |
| U-GM-04 | `/setbuff` com tipo > 0 guarda `time(NULL) + duração` (validade absoluta); aqui a duração é contada em segundos nos dois casos |
| U-GM-05 | `/hide` só esconde Authority 4 ou 8 (`obj+0x250`); a reconstrução guarda o nível do `GameMaster.txt`, então GMs de nível 32 não ficam ocultos |
| U-GM-06 | `/mob` grava `obj+0x294 = 0xFFFF`; o significado exato do valor para o regen não foi confirmado |
| U-CEVT-01 | O leitor de script (`FUN_00451390`) foi reproduzido pelo formato dos arquivos (`;` e `//` como comentário); não foi decompilado por completo |
| U-CEVT-02 | O byte `obj+0x450` testado pelas inscrições (mensagem 0x45A, "em duelo") é aproximado por "tem oponente de duelo" |
| U-CEVT-03 | Death/Theft/Raffle rodam numa thread com `Sleep`; a linha do tempo aqui é não bloqueante (os anúncios e o fim caem nos mesmos segundos) |
| U-CEVT-04 | O texto dos avisos (`FUN_004b3470`) pode vir do idioma padrão e não do idioma de cada jogador; aqui é o de cada jogador |
| U-CEVT-05 | `FUN_004d1410` testa `obj+0x287` antes de levar o lutador morto da dupla ao WAIT; aproximado por "não está teleportando" |
| ~~U-CEVT-06~~ | Resolvida: `C2 F3 EF` reproduzido (`windowtime.go`), restam U-CW-07/08 |
| U-CEVT-07 | O `std::sort` das inscrições da dupla ordena ponteiros de objeto; assume-se a mesma ordem dos índices |
| U-CEVT-08 | `FUN_0045f5c0` (Duel) tem um ramo que permite o ataque quando o outro lutador é do tipo 2 (monstro); nunca acontece com jogadores |
| ~~U-CEVT-09~~ | Resolvida: todos os customs de mundo da tabela foram reproduzidos (bloco 12) |
| ~~U-CEVT-10~~ | Resolvida em parte: `C1 0C F3 E5` reproduzido; o arquivo falta na distribuição e o leitor aceita a ausência (lista vazia → -1). O formato é deduzido do carregador (seção 0, primeira célula) — U-CW-09 |
| U-CW-01 | `FUN_004b0540` (entrada da arena: mapa servido) lê uma tabela por mapa com o valor -2; aproximado por "MapManager tem o mapa e o MapServerInfo não manda a outro servidor" |
| U-CW-02 | O monstro do SummonScroll recebe `obj+0x2a8 = GetTickCount() + 30 min`; nenhum leitor desse campo foi encontrado, então os invocados ficam até morrer |
| U-CW-03 | CreationGold testa `obj+0x286`, `obj+0x3d4 & 3`, `obj+0x3dc` e `obj+0x20a8`; os três primeiros viraram "morrendo/teleportando", interface aberta e transação; `obj+0x20a8` não foi identificado |
| U-CW-04 | Luke Helper e SummonScroll chamam `FUN_0040ae90` antes do `FUN_004b4fe0` e gravam `obj+0x294 = 0xFFFF` (igual ao U-GM-06); não reproduzidos |
| U-CW-05 | O alvo de `FUN_004dc920` nas recusas da arena (registrador) é assumido como o jogador |
| U-CW-06 | O bag da arena usa o índice da arena como SpecialLevel; com o `EventItemBagManager.txt` original não há linha 0x14 específica, então o resultado depende dos dados |
| U-CW-07 | Janela: Pair (estados 3 e 7) e Sobre (2..4) mostram `DAT_00fee954 * 60` / `DAT_0104c28c * 60`, campos que nenhuma função decompilada grava; aqui 0 |
| U-CW-08 | Janela: os nomes de Invasion/Bonus/DropEvent passam por `wsprintfA(buf, nome)` (o nome é o formato); aqui o nome é copiado literal (diferente só com `%` no nome) |
| U-CW-09 | Formato real do `CustomPatente.txt` (arquivo ausente) |
| U-CW-10 | DailyReward: o tamanho do `FB 02` não é testado pelo original; o item resgatado ignora a coluna DURATION (o original passa 0) e a durabilidade 0 vira a máxima do Item.txt como nos outros `FUN_004753e0` |
