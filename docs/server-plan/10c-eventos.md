# 10c — Eventos (observado no GameServer)

Implementação: `apps/server/internal/game/event.go` (peças comuns) e um arquivo por evento. Dados em `apps/server/config/data/Event/*.json` (originais `Data/Event/*.dat`) (`internal/gamedata/events.go`).

## Peças comuns

- **Agenda** (`CScheduleManager`, `0x4e52d0` / `0x4e53c0`): linhas `Year Month Day DayOfWeek Hour Minute Second`, `*` = qualquer valor. O próximo início é o menor horário ≥ agora que casa com alguma linha. DayOfWeek é `tm_wday` (0 = domingo).
- **EventEntryLevel.dat** (`CEventEntryLevel`, `0x47b370`): por seção (0 Blood Castle, 1 Devil Square, 2 Chaos Castle, 3 Kalima, …), linhas `Level Min Max MinMG MaxMG`. Vale a **última** linha cuja faixa contém o nível; MG, DL e classe 6 usam as colunas MG.
- **EventStageSpawn.dat** (`CEventSpawnStage`): seção = evento (0 BC, 1 CC, 2 DS), linhas `Nível Fase Monstro MaxRegenTime`. Cada linha cria um monstro por registro da **seção 4** do arquivo de spawn do mapa com essa classe (`0x46fa30`); `MaxRegenTime` (ms) substitui o tempo de regen, `*` mantém o do MonsterList.
- **Aviso global** `0x4c0c70`: `C1 0D` do tipo dado, com o texto do `Message.txt` no idioma de cada jogador (só jogadores em jogo, sem logout pendente nem troca de servidor).
- **Enviar a todos** `0x512600`: todo jogador em jogo.
- **Pedido de tempo restante** `91` (`0x4d8430`): `C1 07 91 tipo [4] minutos [5] usuários [6] extra`. Tipo 1 = DS, 2 = BC, 4 = CC, 5 = outro (sempre recusado com a mensagem 45). Tipos sem ramo respondem com zeros.

## Devil Square (`CDevilSquare`, `0x46dcf0` … `0x4705d0`)

- **Arquivo** `Event/DevilSquare.dat` (`0x46deb0`):
  - seção 0: `ReadyTime StandTime EventTime CloseTime` em minutos (5 1 20 4);
  - seção 1: agenda;
  - seções 2 e 3: EXP e zen por praça (0–5) e posição (10 colunas).
- **Configuração** (`GameServerInfo - Event.dat`): `DevilSquareEvent`, `DevilSquareMaxUser`, `DevilSquareMaxEntryCount_AL0..3` (0 = sem limite; conta `obj+0x209c`), `DevilSquareNPCWithoutEntrance`, `DevilSquareDuelEnable`.
- **Praças**: 6; mapa 9 para as praças 0–3 e 32 para as 4–5. Gates de entrada `0x3A 0x3B 0x3C 0x3D 0x6F 0x70`; saída gate 27.

### Estados (timer de 1 s `0x46e260`; "restante" = alvo − agora)

| Estado | Entrada | Por segundo |
|---|---|---|
| EMPTY (1) | `0x46e8e0`: fecha a entrada, tira os usuários (gate 27), apaga os monstros, alvo = próximo horário da agenda (sem agenda → BLANK) | Nos últimos `ReadyTime` minutos, a praça 0 avisa a todos (msg 725, minutos); em 0, msg 726 e READY |
| READY (2) | `0x46e930`: entrada aberta, `ReadyTime` minutos | Limpa usuários fora do mapa; a praça 0 avisa por minuto (msg 30, exceto o primeiro minuto quando `ReadyTime > 1`); a ≤ 30 s **cada praça** envia `C1 04 92 00` a todos; em 0: praça 0 msg 31, usuários msg 32 (praça+1, StandTime), STAND |
| STAND (3) | `0x46e990`: entrada fechada, `StandTime` | Sem usuários → EMPTY; ≤ 30 s `C1 04 92 01` aos usuários; em 0 msg 33, START |
| START (4) | `0x46e9f0`: fase 0 de monstros, `EventTime` | Sem usuários → EMPTY; fases 1–4 quando `restante·100 / (EventTime·60)` cai a 80/60/40/20; ≤ 30 s `C1 04 92 02`; em 0 msg 34, CLEAN |
| CLEAN (5) | `0x46ea60`: apaga monstros, classifica, EXP, zen, item bag, janela de ranking; `CloseTime` | Limpa usuários; em 0 → EMPTY |

### Entrada (`90`, `0x4701f0`)

Pacote `[3]` praça, `[4]` slot + 12. Resposta `C1 04 90 r`:

| r | Situação |
|---|---|
| 0 | Entrou: o item é consumido, `obj+0x3d4` é zerado e o gate da praça é usado |
| 1 | Praça > 5, slot ≥ 76 ou item errado: precisa do Devil's Invitation (`0x1C13`) com nível = praça + 1, ou do item `0x1A2E` |
| 2 | Entrada fechada (ou a checagem `0x4b0540`, não identificada) |
| 3 | Praça pedida abaixo da faixa do personagem |
| 4 | Praça pedida acima da faixa do personagem |
| 5 | `DevilSquareMaxUser` atingido |
| 6 | PK > 4 com `PKLimitFree = 0` e `PKLimitEventEntry ≠ 0` |

Com o limite diário (`MaxEntryCount`) atingido, o jogador recebe o aviso customizado (msg 715) e não recebe resposta `90`.

### Charon (NPC `0xED`, `0x46fbc0`)

- Sem faixa de nível → msg 35.
- Sem convite, sem `0x1A2E` e sem `NPCWithoutEntrance` → `C1 07 F3 40 01 02 00`.
- Entrada fechada → `C1 07 F3 40 01 03 00`.
- Caso contrário abre a janela `C3 0B 30 04`.

### Pontos, posições e prêmios

- **Morte de monstro** no mapa 9/32 (`0x46fe60`, dentro de `0x4cda80`): o maior causador de dano soma `nível do monstro × (praça + 1)`, se o monstro pertence à praça dele e ela está em START.
- **Jogador morto** em START sai da lista (`0x46fdb0`).
- **Posição** (`0x46f140`): número de usuários com mais pontos, ou com os mesmos pontos em slot anterior.
- **Prêmios** para as posições 0–9:
  - EXP = tabela × `AddEventExperienceRate_AL` (nada sob os efeitos 31/42), no máximo `MaxLevelUpEvent` níveis; sem level up envia `9C FFFF`;
  - zen (limite 2.000.000.000);
  - item bag tipo 1, índice = praça, na posição do jogador.
- **Janela de ranking** `C1 size 93 posição+1 n` (`0x4705d0`): entradas de 24 bytes (`nome[10]` + 2, pontos, EXP, zen), primeiro a do próprio jogador e depois as posições 1–9. O relatório `C1 20 3F` ao DataServer não é reproduzido.
- **Detalhe fiel**: as mensagens aos usuários (`0x46ff50`) usam o idioma de `objects[slot]` (o número do slot, não o usuário).

### Incógnitas

| ID | Questão |
|---|---|
| U-EVT-01 | `0x4b0540` na entrada (resultado 2) não identificado. |
| ~~U-EVT-02~~ | Resolvida: `obj+0x2098..` são as colunas de `EventEntryCount` (BC, CC, DS, DG, IT, IG). O DataServer as carrega com o personagem (`WZ_GetEventEntryInfo`, `0x40c4c0`), zerando no primeiro acesso de um novo dia (o `UPDATE` do procedure mantém o ITCount na tabela por usar `@ITCount=@ITCount`), e as grava com o personagem (`WZ_SetEventEntryInfo`, `0x40e9c0`). Implementado em `store/event_entry.go` (migração `0007_event_entry.sql`) e `game/event_entry.go`. |
| U-EVT-03 | Significado do `C1 07 F3 40 01 xx 00` (pacote WizardTeam) no cliente. |

## Blood Castle (`CBloodCastle`, `0x4137b0` … `0x417b20`)

- **Arquivo** `Event/BloodCastle.dat` (`0x413900`):
  - seção 0: tempos em minutos (5 1 15 4);
  - seção 1: agenda;
  - seções 2 e 3, por castelo: EXP e zen `[vencedor, demais]`;
  - seção 4: vida do portão e da estátua;
  - seção 5: posição da estátua.
- **Configuração**: `BloodCastleEvent`, `BloodCastleMaxUser`, `BloodCastleMaxEntryCount_AL0..3` (conta `obj+0x2098`), `BloodCastleNPCWithoutEntrance`, `BloodCastleDuelEnable`.
- **Castelos**: 7, nos mapas 11–17. Gates de entrada `0x42 … 0x47, 0x50`; saída gate 22.
- **Estados**: os mesmos do DS, com estas diferenças:
  - EMPTY avisa por minuto com a msg 727 e não tem aviso de abertura;
  - READY avisa por minuto com a msg 0; a ≤ 30 s envia `92 03` a todos; ao fim, msg 1 a todos e msg 2 (castelo+1, StandTime) aos usuários;
  - STAND envia `92 04`; ao fim, msg 3;
  - START envia `92 05`; termina com a msg 6 (tempo esgotado) ou a msg 12 (nome do vencedor);
  - CLEAN envia `92 06`.
- **Terreno** (`C1 size 46 00 atributo remover n` + retângulos `x1 y1 x2 y2`, `0x4da600`):
  - entrada `(13,15)-(15,23)` com atributo 4 bloqueada de EMPTY até START;
  - ponte `(13,70)-(15,75)` com atributo 8;
  - área da estátua com atributo 4 em três retângulos: `(13,76)-(15,79)`, `(11,80)-(25,89)`, `(8,80)-(10,83)`.

### Etapas do START (`obj r[9]`, `0x414170`)

| Etapa | Condição para avançar | Efeito |
|---|---|---|
| 0 | `usuários × 40` mortes de monstros (exceto esqueletos mágicos) | `9B 03`, ponte liberada no servidor, fase 1 de spawn, msg 4 |
| 1 | 3 s | Pacote `46` da ponte aos usuários, portão (`0x83` em (14,75)) perde os efeitos 0x3F/0x40, fase 2 |
| 2 | Portão destruído (msg 10 com o nome) | Meta = `usuários × 2`, área da estátua liberada, fase 3 |
| 3 | Mortes de esqueletos mágicos (`0x59 0x5F 0x70 0x76 0x7C 0x82 0x8F 0x1B1`) | Estátua `0x84 + rand%3` na posição da seção 5, msg 5, fase 4 |
| 4 | Estátua destruída | Arma do Arcanjo `0x1A13` com nível = classe − 0x84 cai na posição da estátua (dono = maior causador de dano), msg 11, fase 5 |

- A cada segundo os usuários recebem `C1 0E 9B tipo restante meta mortes portador nível+1` (`0x417ab0`), tipo 1 nas etapas 0–2 e 4 nas etapas 3–4.
- **Arma do Arcanjo**:
  - pegar a arma (`0x416750`, msg 9) define o portador;
  - largar a arma (`0x4166b0`, msg 8) define a vida do item no mapa como "agora + segundos restantes" **em milissegundos**; reproduzido como no original;
  - entregar ao Archangel (NPC `0xE8`, `0x4162c0`) responde `C1 07 F3 40 01 17 00` e define o vencedor; `18` = recusado, `2E` = já entregue.
- **CLEAN** (`0x4146c0`):
  - **vencedor e party**: EXP e zen da primeira coluna, mais itens no chão. Castelos 1–3 dão 3× Box of Luck +11, Jewel of Life e Jewel of Chaos; castelos 4–7 dão 4× Box of Luck +12, Bless, Soul e Chaos. Resultado `C1 1D 93 01 FF nome[10]`, bytes 15–16 não escritos, pontos/EXP/zen u32 em 17/21/25 (`0x4178b0`; pontos = EXP / 5);
  - **demais**: segunda coluna e resultado `93 00`.
- **Saídas e mortes**:
  - sair do castelo em START com a arma faz o portador largá-la no chão (`C1 05 23 r slot`);
  - morrer em START dá a recompensa de perdedor e o resultado chega 8 s depois (`0x4170e0`).
- **Entrada** `9A` (`0x4174b0`): `[3]` castelo (1–7) e `[4]` slot sem ajuste. Exige Invisibility Cloak `0x1A12` (nível = castelo) ou `0x1A2F`. Resultados: 1 item/castelo errado, 2 fechado, 3/4 nível, 5 cheio, 7 PK. O limite diário gera o aviso msg 713.
- **Messenger of Archangel** (NPC `0xE9`):
  - nível fora da faixa → msg 7;
  - sem capa → `F3 40 01 15 00`;
  - entrada fechada → `F3 40 01 14 00`;
  - caso contrário abre a janela `C3 0B 30 06`.
- **Pedido de tempo `91`**, tipo 2: usuários em READY, senão os minutos até o próximo início.

### Incógnitas

| ID | Questão |
|---|---|
| U-EVT-04 | O byte "remover" do pacote `46` nos retângulos da entrada e da estátua vem de CL, não lido; usamos 0 para adicionar e 1 para remover, como na ponte (`0x416b00`, CL = 1 ao remover). |
| U-EVT-05 | O conteúdo exato do pacote atrasado de `0x4179e0` (morte em START) foi perdido pelo Ghidra; enviamos o `93` do jogador. |
| U-EVT-06 | `obj+0x294 = 0xFFFF` (portão, estátua e monstros com regen `*`): tratamos como "sem regen" só para portão e estátua (removidos ao morrer). |

## Chaos Castle (`CChaosCastle`, `0x4301a0` … `0x434080`)

Código: `internal/game/chaoscastle.go` (testes em `chaoscastle_test.go`); arquivo em `gamedata.LoadChaosCastle`.

- **Arquivo** `Event/ChaosCastle.dat` (`0x430330`):
  - seção 0: tempos em minutos `WarningTime NotifyTime EventTime CloseTime` (5 1 10 4) = READY, STAND, START, CLEAN (`this+0x2bd0..0x2bdc`);
  - seção 1: agenda (12 horários ímpares);
  - seção 2: por castelo (0–5) `[EXP por usuário morto, EXP por monstro morto]` (`this+0x2bf0 + castelo*8`); castelo fora de 0–5 é ignorado com log.
- **Configuração** (`0x4f1390`): `ChaosCastleEvent` (0x72c), `ChaosCastleMinUser` (0x730), `ChaosCastleMoneyRate` (0x734, jackpot), `ChaosCastleBlowUserRate` (0x738, chance % de empurrão), `ChaosCastleMaxEntryCount_AL0..3` (0x73c+AL*4, contador diário `obj+0x209a` = `entries.CC`). `ChaosCastleDamageRate` (Character, 0xd4c) é aplicado no PvP (`pvp.go`).
- **Castelos**: 6, nos mapas 18–23; registro de 0x748 bytes a partir de `this+4`: `r[0]` índice, `r[1]` estado, `r[2]` mapa, `r[3]` restante, `r[4]` alvo, `r[5]` tick, `r[6]` aberto, `r[7]` último minuto, `r[8]` aviso 30 s, `r[9]` etapa dos buracos, `r[10]` total inicial (usuários + monstros), `r[11]` vencedor, `+0x30` 70 usuários de 0x14 bytes `[índice, pontos, usuários mortos, monstros mortos, EXP]`, `+0x5a8` lista de 100 monstros, `+0x738` fila de pacotes atrasados.
- **Nível**: `EventEntryLevel.dat` seção 2 (`0x47b5d0`), > 5 ⇒ aviso msg 0x16. Taxa de entrada `DAT_00547978`: 25 000, 80 000, 150 000, 250 000, 400 000, 650 000. Gates `0x52 … 0x57` (o `case 6 = 0x110` é inalcançável); saída gate 22.

### Estados (timer de 1 s `0x430670`)

| Estado | Comportamento |
|---|---|
| EMPTY (`0x4307d0`) | Só o castelo 0 avisa a todos por minuto (msg 0x2D8, minutos) quando restante ≤ ReadyTime; ao zerar ⇒ READY |
| READY (`0x430860`) | Limpa usuários; castelo 0 avisa por minuto (msg 0x10); com 0 < restante < 31 envia `92 0A` **a todos** (`0x512600`); ao fim: castelo 0 msg 0x11 a todos, msg 0x12 (castelo+1, StandTime) tipo 1 aos usuários ⇒ STAND |
| STAND (`0x4309a0`) | Usuários < `ChaosCastleMinUser` ⇒ msg 0x15 aos usuários e volta a EMPTY (que os expulsa); `92 0B` aos usuários; ao fim msg 0x13 ⇒ START |
| START (`0x430ab0`) | Sem usuários ⇒ EMPTY. Etapas, quedas, monstros e itens (abaixo), status `9B 06`; termina com **1 usuário e 0 monstros** (sem mensagem) ou no fim do tempo (`92 0C` antes, msg 0x14) ⇒ CLEAN |
| CLEAN (`0x430c30`) | Limpa usuários; `92 0D`; ao fim ⇒ EMPTY |

- **Entrada nos estados**:
  - EMPTY (`0x430d50`): zera r[6..10], r[7]=r[11]=−1; `9B 07` aos usuários; usuários ao gate 22 (`0x431820`); monstros removidos (`0x432000`); sobreposição de zona segura ligada (`0x432570`); buracos dos conjuntos 1, 2, 3 fechados (`0x4327a0`, EAX = 1..3); buracos centrais abertos (`0x432830`); próximo horário da agenda (`0x431250`; sem agenda ⇒ BLANK);
  - READY (`0x430e00`): aberto, ReadyTime;
  - STAND (`0x430e60`): jackpot (`0x431a40`), buracos centrais, StandTime;
  - START (`0x430ee0`): `r[10]` = usuários; zona segura desligada (`0x432640`); spawn; `9B 05`; EventTime;
  - CLEAN (`0x430fa0`): vencedor = sem monstros ? usuário de maior pontuação (`0x431970`, o primeiro slot vence empates) : −1; monstros removidos; cada usuário recebe EXP e `93` (vencedor `93 01` + itens, demais `93 00`); `C1 20 3E` de ranking ao DataServer (`0x476ce0`, não mantido); `9B 07`; CloseTime.

### Terreno

- **Zona segura** (`0x432570` liga / `0x432640` desliga): atributo 1 em x 23..44 e y 75..108 (o laço anda y = 75, 77 … 107 e grava y e y+1), depois `46` a cada usuário com o retângulo `DAT_00547928 = (23,75,44,108)`: `C1 0B 46 00 01 remover 01 17 4B 2C 6C` (remover: CL = 0 ao ligar, 1 ao desligar; DL = 1 retângulo).
- **Buracos** (atributo 8, **sem pacote**: o cliente abre os buracos ao receber `9B 08/09/0A`), 4 retângulos por conjunto em `0x547930 + conjunto*16`:

| Conjunto | Retângulos |
|---|---|
| 0 (cópia de 1, não usado) | — |
| 1 | (23,75,44,76) (43,77,44,108) (23,107,42,108) (23,77,24,106) |
| 2 | (25,77,42,78) (41,79,42,106) (25,105,40,106) (25,79,26,104) |
| 3 | (27,79,40,80) (39,81,40,104) (27,103,38,104) (27,81,28,102) |
| centro (`0x547970`) | (32,84,35,87) (32,96,35,99) |

### START

- **Etapas** (`r[9]`, `0x4310d0` / `0x431150` / `0x4311d0`, uma por segundo): usuários + monstros < 41 ⇒ etapa 1, `9B 08`, conjunto 1; < 31 ⇒ etapa 2, `9B 09`, conjunto 2; < 21 ⇒ etapa 3, `9B 0A`, conjunto 3.
- **Quedas** (`0x4328b0`): usuário em célula com atributo 8 morre: vida 0, `obj+0x6e` = 0, live 4, `+0x285` = 1, `+0x286` = 1, caminho zerado, teleporte 1 ⇒ 0; `C1 08 17 idx 00 00 00` (`0x4da270`, skill 0, matador 0) a ele e ao VP2; recebe a EXP; resultado atrasado; sai da lista.
- **Monstros em buracos** (`0x432a10`): procura a primeira célula livre (sem 2, 4 ou 8; fora do mapa = 4) em x−8..x+7 (externo) × y−8..y+7 (interno) (`0x4330e0`) e reposiciona com `0x511cb0`; grava `obj+0x438`.
- **Itens em buracos** (`0x432b10`): item do mapa sobre atributo 8 ⇒ `live = 0`, `+0xdf = 1`, estado 8 (removido).
- **Status** a cada segundo: `C1 0E 9B 06 restante(u16) total_inicial(u16) usuários+monstros(u16) 00 00 FF ??` (o byte 13 não é escrito; 0 aqui).
- **Spawn** (`0x432340` / `0x4323f0`): linhas da seção 1 do `EventStageSpawn.dat` com nível = castelo e fase 0; cada registro da seção 4 do mapa com a classe vira monstro enquanto `r[10]` ≤ 99 (usuários + monstros ≤ 100). O `MaxRegenTime` da linha é ignorado.
- **Morte de monstro** (`0x433330`, só em START e se o monstro está na lista): `rand%100 < ChaosCastleBlowUserRate` ⇒ empurra usuários a até 3 células (`0x432eb0`, ignorando mortos `+0x286` e em teleporte `+0x284`); sai da lista; o maior causador de dano (`0x4b6df0`, senão o matador), se usuário do castelo, ganha **+2 pontos** e +1 monstro morto.
- **Empurrão** (`0x432f40`, assembly): `dx = ux − mx`, `dy = uy − my`; direção por eixo = sinal de d, ou com d = 0 `rand ímpar ⇒ −1, par ⇒ +1`; `dist = (int)sqrtf(dx²+dy²)`; novo x = `(rand%2 − dist + 4) × sx + ux`, novo y = `(rand%2 − dist + 4) × sy + uy` (um `rand` por eixo, x antes de y); `0x511cb0` e `obj+0x438` (que bloqueia o `DF` por 1 s nos mapas 18–23).
- **Jogador morto** (`0x4331d0`, só em START): a vítima recebe a EXP, o resultado atrasado e sai; matador jogador usuário do castelo ganha **+1 ponto** e +1 usuário morto.
- **Jackpot** (`0x431a40`): com `ChaosCastleMoneyRate` ≠ 0 e usuários, um usuário sorteado recebe `rate × taxa × usuários / 100` zen (teto 2 000 000 000), `C3 10 22 FE` e todos os usuários leem a msg 0x2D4 (nome, valor) tipo 1.

### Recompensas e resultado

- **EXP** (`0x431c30`): `exp = tabela1 × usuários mortos + tabela2 × monstros mortos` (64 bits, teto 0x7FFFFFFF); `pontos = exp / 5`; se exp > 0, `exp × AddEventExperienceRate_AL` (teto 0x7FFFFFFF); sem EXP sob os efeitos 31/42; `MaxLevelUpEvent` níveis no máximo, senão `9C` com monstro 0xFFFF.
- **Itens do vencedor** (`0x431de0`): 5× Box of Luck +12 (`0x1C0B`), Jewel of Bless (`0x1C0E`), Jewel of Chaos (`0x180F`) na célula do vencedor (`0x4753e0`).
- **Resultado** `93` (`0x433e80`; cópia atrasada `0x433fb0` com sucesso 0): `C1 1D 93 sucesso FE nome[10]`, bytes 15–16 não escritos, `[17]` monstros mortos, `[21]` EXP, `[25]` usuários mortos (todos u32). A fila atrasada (`0x433500`) guarda `[tick, índice, pacote(32)]` e envia após 8 s se o objeto ainda está em jogo.

### Pedidos do cliente

- **Entrada** `AF 01` (`0x4338c0`, `[5]` slot): em ordem — evento ligado e jogador em jogo; nível (> 5 ⇒ msg 0x16); limite diário (msg 0x2CA); slot > 0x4B ou item ≠ Armor of Guardsman `0x1A1D` ⇒ 1; pet `0x1A02/0x1A03/0x1A25` no slot 8 ⇒ aviso msg 0x9F; party ⇒ 1; guild em guerra (`+0x7fd`) ⇒ 1; duelo ⇒ 1; loja pessoal aberta (`+0x2468`) ⇒ 1; transformado (`obj+0x314 ≠ −1`, anel de transformação nos slots 10/11, `0x4fa1a0`) ⇒ msg 0x9F; castelo fechado ⇒ 2; mapa não servido por este GS (`0x4b0540`) ⇒ 2; zen < taxa ⇒ 7; PK (PKLimitFree 0, PK > 4, PKLimitEventEntry) ⇒ 8; sem slot livre (70) ⇒ 5. **Não há limite de usuários além dos 70 slots.** Sucesso: desconta a taxa (`C3 10 22 FE`), apaga o item (`C1 05 28`), `C1 05 AF 01 00`, remove o summon (`C1 05 F3 20 00`), limpa a interface e move ao gate.
- **Mover dentro do castelo** `AF 02` (`0x433d50`): usuário no próprio castelo é reposicionado em `([4], [5])` por `0x511cb0` em **qualquer estado**, gravando `obj+0x438`.
- **Tempo restante** `91` tipo 4: castelo pelo nível (> 5 ⇒ msg 0x16); READY ⇒ `[5]` usuários (`0x431690`), senão `[4]` minutos até o próximo horário (`0x431470`).
- **PvP**: entre dois jogadores nos mapas 18–23 o ataque só vale em START (`0x431420` = estado); o `stuckInWall` não se aplica em START.

### Incógnitas

| ID | Descrição |
|---|---|
| U-CC-01 | Sorteio do jackpot (`0x4e0b70` / `0x4933d0` / `0x4e0c50`): assumido uniforme entre os usuários |
| U-CC-02 | `0x4b0540` (mapa com valor −2 na tabela de servidores): reproduzido como "o mapa do castelo é servido por este GameServer" |
| U-CC-03 | Anel `0x1A2A` só transforma com `obj+0x250 > 3` (campo não identificado); aqui sempre conta como transformado |
| U-CC-04 | Monstros do castelo usam o regen do MonsterList; não se verificou se o original impede o renascimento (o monstro morto sai da lista e não conta mais) |
| U-CC-05 | ✅ Resolvido: o resultado `93` do Blood Castle (`0x4178b0`) tem o mesmo layout (17/21/25); `bloodcastle.go` corrigido |

## Invasões (`CInvasionManager`, `0x488be0` … `0x48a730`)

- **Arquivo** `Event/InvasionManager.dat` (`0x488f40`), até 20 invasões:
  - seção 0: agenda por invasão;
  - seção 1: `RespawnMessage DespawnMessage MonsterBoss BossKilledMessage InvasionTime(s) EffectType Nome` (`*` = sem mensagem);
  - seção 2: grupos `Group Map Stage`;
  - seção 3: linhas de spawn `Group Monstro Raio Mapa X1 Y1 X2 Y2 Quantidade Stage RegenType RegenTime(ms) FlagMensagem`.
- **Configuração**: `InvasionManagerSwitch`. Sem agenda, a invasão fica BLANK.
- **Estados** (timer de 1 s `0x489610`): EMPTY → no horário, aviso RespawnMessage a todos (texto no idioma padrão) → START; START dura `InvasionTime` → aviso DespawnMessage → EMPTY. A entrada em EMPTY (`0x489910`) apaga os monstros, zera os chefes por mapa e desliga o efeito de mapa (`C1 05 0B 00 efeito` a todos).
- **START** (`0x489960`):
  - para cada grupo com linhas, sorteia uma linha `(mapa, stage)`;
  - cria `Quantidade` monstros de cada linha de spawn desse grupo, mapa e stage, numa célula livre aleatória do retângulo (senão no canto `X1,Y1`);
  - os monstros recebem direção aleatória, raio de movimento = raio, atributo 5 (`obj+0x37e`), `obj+0x24a4` = invasão e regen = `InvasionTime·1000` ou `RegenTime` conforme RegenType;
  - contadores por classe (vivos e máximo) em 20 slots;
  - cada chefe criado soma no contador do mapa e, com EffectType ≠ 0, envia `C1 05 0B 01 efeito` aos jogadores do mapa.
- **Morte** (`0x48a2d0`):
  - o contador da classe diminui;
  - se a linha de spawn **na mesma posição do contador** tem a flag de mensagem: msg 1100 quando zerar, senão msg 1101 (`matador, monstro, mortos/máximo`);
  - chefe: contador do mapa diminui (efeito desligado ao zerar) e aviso BossKilledMessage com o nome do matador e do mapa.
- **Ao entrar num mapa** (`0x48a690`, no reshow do teleporte): `C1 05 0B 01 efeito` se alguma invasão ativa tem chefe vivo no mapa, senão `C1 05 0B 00 00`.

| ID | Questão |
|---|---|
| U-EVT-07 | `0x50c9c0` (célula aleatória do retângulo) foi aproximado por 100 tentativas evitando os atributos 1/4/8. |
| U-EVT-08 | O byte 4 de `0x4d9ea0` (efeito) vem de registrador; usamos o EffectType da invasão. |

## BonusManager (`CBonusManager`, `0x417c70` … `0x4188f0`)

- **Arquivo** `Event/BonusManager.dat`, vazio nesta instalação:
  - seção 0: agenda;
  - seção 1: `StartMessage FinalMessage BonusTime(s) Nome`;
  - seção 2: `EventIndex BonusIndex Valor_AL0..3 Mapa`.
- **Configuração**: `BonusManagerSwitch`.
- **Ciclo**: EMPTY → START com StartMessage (aviso tipo 0 a todos) → depois de `BonusTime` → FinalMessage → EMPTY (`0x418400`, `0x4184c0`, `0x4185c0`).
- **Aplicação** (`0x4188f0`, valor `v`, só eventos em START e mapa igual ou `*`):

  | Índice | Efeito |
  |---|---|
  | 0 (EXP), 1 (drop) | `v += taxa·v/100` |
  | 2 (Chaos Mix) | `v += taxa` |
  | 3 (drop excelente), 4 (drop de set) | `v += taxa·(1000000 − v)/1000000` |

  A taxa é a do AccountLevel do jogador. A EXP de monstro, o drop e o Chaos Mix devem chamar `w.bonusValue`; essa ligação ainda está pendente.

## DropEvent (`CDropEvent`, `0x4708f0` … `0x471690`)

- **Arquivo** `Event/DropEvent.dat`, vazio nesta instalação:
  - seções 0 e 1: como no BonusManager (agenda; `StartMessage FinalMessage EventTime Nome`);
  - seção 2: `EventIndex Item(gg,iii) Nível Durabilidade Option0..5 Duração Mapa Monstro MinLevel MaxLevel AccountLevel Taxa` (`*` = qualquer).
- **Configuração**: `DropEventSwitch`. Ciclo EMPTY/START igual ao do BonusManager.
- **Drop** (`0x471690`), no drop do monstro logo depois do item de quest:
  - **filtro**: linhas de eventos em START com mapa, classe e nível do monstro e AccountLevel do dono;
  - **sorteio**: cada linha passa em `rand%1e6 < Taxa` (`*` = sempre) e entra com peso `1e6/Taxa`;
  - **item**: recebe as opções do `ItemOptionRate` (nível, skill, sorte, opção, excelente via `0x49f0d0` e set via `0x49f270`) e cai na posição do monstro com dono = maior causador de dano;
  - ter caído um item cancela o drop normal.

## Golden Archer (`CGoldenArcher`, `0x461610` … `0x4621f0`)

- **Arquivo** `Event/GoldenArcher.dat`: `Item(gg,iii) Grupo Option0..5 Duração Peso`, grupos 0–4.
- **Contagem de Renas por conta** na tabela do DataServer `EventGoldenArcher` (`store/golden_archer.go`, migração 0009):
  - `0x413750`: lê, criando a linha com 0;
  - `0x413c80`: soma;
  - `0x4139a0`: desconta se houver o bastante.
- **NPC `0xEC`** (`0x4c22f0`): interface tipo 9 (`obj+0x3d4 = … | 0x241`) e pedido da contagem → `C1 10 94 00 contagem(u32)` + 8 zeros (`0x461b60`). Um pedido pendente (`obj+0x6a8`) não é repetido.
- **`95`** (`0x461920`): uma Rena (14,21) sai do inventário, a conta ganha +1 e a contagem é pedida de novo.
- **`96 [3] grupo`** (`0x461a40`):
  - pede ao DataServer para descontar 10/20/30/50/100 Renas (`DAT_00547920`), com `obj+0x6ac` como trava;
  - **com sucesso** (`0x461c90`): sorteio ponderado entre as linhas do grupo, opções do `ItemOptionRate` e item no inventário se couber (`0x4952a0`), senão no chão do jogador; fogos `C1 07 F3 40 00 X Y` ao jogador e ao viewport; a contagem é pedida de novo.
- **`97`** (`0x461b00`): fecha a interface tipo 9.
- **Detalhe**: o original manda "somar" e "ler" pela mesma conexão do DataServer, que responde em ordem; aqui as duas operações rodam na mesma tarefa assíncrona.

## Kalima (`CKalima`, `0x4a2c20` … `0x4a3510`)

- **Arquivo** `Event/Kalima.dat` (`0x4a2c20`): linhas `KALevel CreateNextPortal` até `end`; níveis fora de 0–5 são ignorados. A linha *n* habilita o portão para o Kalima *n*+2 quando morre o Illusion of Kundun do Kalima *n*+1. Original: `1 1 1 1 1 0`, então o Kundun do Kalima 6 não abre o Kalima 7.
- **Portão** ("Magic stone", classes 152–158, `obj+0x37e = 6`): criado com `MoveRange` 0, `RegenTime` 0, dono em `obj+0x310`, contador em `obj+0x2b4`, nível 0–6 em `obj+0x2b8` e hora de criação em `obj+0x28c`. O dono fica com `obj+0x2ac = 1` e `obj+0x2b0 =` índice do portão.
- **Lost Map** (14,28) solto no chão (`23` → `0x4a2f20`):
  - nível do item 1–7 → classe `0x97 + nível`, nível do portão `nível − 1`;
  - recusado em Icarus (10), Kalima (24–29, 36), Devil Square (9, 32), Blood Castle (11–17) e Chaos Castle (18–23);
  - recusado com o jogador em zona segura (`0x4ad4d0(1)` na célula dele) ou com portão já aberto (`obj+0x2ac`);
  - célula do drop com qualquer atributo → célula livre em ±3 (`0x50c860`, 50 tentativas);
  - recusado → `C1 05 23 00 slot` e o item fica; aceito → o item é consumido como uma bag.
- **Illusion of Kundun** (`0x4a3150`, chamado de `0x4b70b0` antes do drop) num mapa de Kalima: classes `0xA1/0xB5/0xBD/0xC5/0x10B/0x152` = Kalima 1–6. Se `Kalima.dat` habilita, o matador (jogando, sem logout pendente e sem portão) ganha portão classe `0x98 + n`, nível *n*, na célula do Kundun (atributo 0) ou numa livre em ±3. Não há checagem de zona segura aqui.
- **IA do portão** (`0x4a2d10`, a cada segundo no laço `0x510810` para `obj+0x37e == 6`):
  - só com o portão vivo e `obj+0x26c == 2`;
  - dono inválido (`0x50cc00`: fora do jogo ou com logout pendente) → `C1 08 17` sem matador e o portão é apagado, **sem limpar `obj+0x2ac` do dono** (fiel: o dono não abre outro até relogar);
  - nada antes de 2 s;
  - dono morto (`obj+0x286`) ou mais de 4 entradas → `0x4a3480`; idade ≥ 180 s → `0x4a3480`;
  - percorre o VP2 do portão: jogadores vivos a ±2 do portão (`0x513bf0` com alcance 2), que sejam o dono ou da mesma party (`obj+0x2f0`), vão para o gate `0x58..0x5D` / `0x74` pelo nível (Kalima 1–7); cada um soma 1 ao contador e o laço para quando o valor anterior passa de 4.
- **`0x4a3480`** (apaga o portão do jogador): só se o índice guardado é um objeto vivo com atributo 6; manda `C1 08 17` sem matador, apaga e limpa `obj+0x2ac/0x2b0`. `gObjMoveGate` (`0x50f6d0`) chama para o dono que troca de mapa, então **o portão some quando o dono entra** (os membros que o laço ainda não alcançou não entram).
- **Kundun** (classe `0x113`, `0x4a3510`): a cada 300 s recupera `MaxLife × 0,25` (limitado ao máximo); o timer `obj+0x9f8` começa em 0, então a primeira recuperação ocorre no primeiro segundo.
- **Item (13,48)** (`0x499a20`): abriria um portão pelo nível do personagem na seção 3 do `EventEntryLevel.dat` (`0x47b840`), mas o índice base 0 é passado a `0x4a2f20`, que subtrai 1 de novo (um nível abaixo; Kalima 1 nunca). O item não existe no `Item.txt` deste servidor, então o ramo não foi implementado.
- Implementação: `internal/game/kalima.go`, `internal/gamedata/kalima.go`; o drop chama `lostMapDrop` em `dropSpecialItem`, `monsterDie` chama `kundunDie` e a troca de mapa usa `mapChangeHooks`.

### Incógnitas

| ID | Questão |
|---|---|
| U-KAL-01 | `0x429cc0`, chamado antes do teste de alcance na IA do portão, não foi identificado |
| U-KAL-02 | `obj+0x6e` dos jogadores no laço do portão foi tratado como "vivo" (`Alive`) |

## Castle Deep (`CCastleDeep`, `0x419060` … `0x41a2c0`)

Invasão do Valley of Loren (mapa 30). Singleton em `0x01241a68`: `+4` estado (0 DISABLED, 1 EMPTY, 2 START), `+8` restante (s), `+0xc` hora-alvo, `+0x10` tick, `+0x14` aviso dado, `+0x18` etapa, `+0x1c` grupo sorteado, `+0x20/+0x30/+0x40/+0x50` os quatro vetores do arquivo. Configuração em `GameServerInfo - Event.dat`: `CastleDeepEvent` (CServerInfo+0x6e4, chave liga/desliga) e `CastleDeepEventTime` (+0x6e8, minutos de duração). O código existe igual no `GameServer.exe` e no `CastleSiege.exe` (mesmas strings `[Castle Deep] …`).

- **Arquivo** `Event/CastleDeepEvent.dat` (`0x419320`):
  - seção 0: horários `Ano Mês Dia DiaDaSemana Hora Min Seg` (`*` = qualquer), mesma regra do `CScheduleManager`;
  - seção 1: `Grupo Chance` (`0x41a1c0`);
  - seção 2: `Grupo Etapa Minuto` (`0x4933d0`), minuto contado desde o início;
  - seção 3: `Grupo Etapa Monstro Quantidade X1 Y1 X2 Y2` (`0x41a220`);
  - seções fora de 0–3 são puladas.
- **Início** (`0x419260`, chamado pelo spawn de partida `0x4baa20`): com a chave desligada nada acontece (o construtor deixa o estado em EMPTY, mas o tick não roda); ligada → EMPTY.
- **Tick** (`0x419720`, laço de 1 s `0x509cc0`): só com `CastleDeepEvent ≠ 0`; restante = `difftime(alvo, agora)` truncado.
- **EMPTY** (`0x4197c0`):
  - `0 < restante < 301` e sem aviso → aviso dado e mensagem 52 ("começará em 5 minutos") para o grupo de servidores (abaixo);
  - `restante < 1` → mensagem 53 para o grupo e START.
- **Entrada em EMPTY** (`0x419b50`): aviso zerado, etapa 1, grupo 0, remove os monstros (`0x419e10`) e sincroniza (`0x419c60`): próximo horário da seção 0; sem horário → DISABLED (`0x419ad0(0)`).
- **Entrada em START** (`0x419b90`): restante = `CastleDeepEventTime × 60`; sorteio do grupo percorrendo a seção 1 em ordem, a primeira linha com `rand % 100 < Chance` vence; o laço repete até alguém vencer (**trava o servidor** se nenhuma linha puder vencer; aqui o sorteio é abandonado e o grupo fica 0).
- **START** (`0x419960`):
  - minuto decorrido = `(EventTime × 60 − restante) / 60`; a linha da seção 2 com o grupo e a etapa atuais cujo minuto é igual ao decorrido faz o spawn da etapa e a etapa avança;
  - `restante < 1` → mensagem 54 a todos os jogadores **deste** servidor (`0x4c0c70`, idioma de cada um) e volta a EMPTY.
- **Spawn** (`0x419e60`), para cada linha da seção 3 com o grupo e a etapa, `Quantidade` vezes:
  - índice de monstro (`0x4b4b00`); célula aleatória por `0x50c860` (mapa 30, 10 tentativas) **em volta de `(X1, Y1)` com raio `X2 − X1` / `Y2 − Y1`** (metade das células sorteadas cai fora do retângulo do arquivo); sem célula livre, `(X1, Y1)`;
  - `obj+0x294 = 0xFFFF` (sem registro de spawn), direção `rand & 7`, mapa 30, `0x4b4fe0` com a classe (falhou → índice liberado);
  - depois: `obj+0x286 = 0`, `obj+0x28c = 1`, atributo `obj+0x37e = 7`, `RegenTime obj+0x290 = 1000` ms, `obj+0x9fc = GetTickCount()`. Com atributo 7, o monstro morto renasce perto da célula da morte (`0x4b4e40`) 1 s + 1 s depois, até o fim do evento.
- **Limpeza** (`0x419e10`): apaga **todo** monstro jogando (índices 0..0x5013) com atributo 7, em qualquer mapa.
- **Morte** (`0x41a050`, chamado por `0x4cda80` para monstros mortos no mapa 30): se o atributo é 7 e a classe é 295 (Erohim) ou 300–303, mensagem 55 ("%s foi morto por %s") a todos os jogadores do servidor com o nome do monstro e o do maior causador de dano (`0x4b6df0`), ou do matador quando não há.
- **Aviso ao grupo** (`0x475b50`): `C1 94 21 grupo(u16) tipo contagem opacidade 00 atraso(u16) cor(u32) velocidade texto[128]` ao DataServer, com tipo e demais campos 0 e o texto da mensagem no idioma padrão. O DataServer (`0x40e4a0`) repassa a todos os GameServers e cada um (`0x474c90`) mostra o notice `0D` a todos os seus jogadores (`0x4c0ab0`) se o grupo do pacote é o seu (campo `+4` da própria entrada do `CMapServerManager`, `DAT_01239534`). Na reescrita as instâncias do Hub fazem o papel das conexões do DataServer.
- **Menu do servidor** "Start Castle Deep" (`0x41a150`, item 0x7d5): acrescenta o horário "agora + 1 minuto" e reinicia o evento. Sem equivalente no protocolo do cliente; não implementado.
- **Dados originais**: as classes 49 e 293 da seção 3 (etapa 3 dos dois grupos) não existem no `MonsterList.txt`, então esses 20 monstros nunca nascem. Os dois servidores do grupo 0 rodam o evento se a chave estiver ligada em ambos; o GameServer 0 cria monstros no mapa 30, que ele não hospeda (o `MapServerInfo.txt` manda o mapa 30 para o servidor 19), e os avisos ao grupo chegam em dobro.
- Implementação: `internal/game/castledeep.go`, `internal/gamedata/castledeep.go`; testes em `castledeep_test.go` e `gamedata/events_test.go`.

### Incógnitas

| ID | Questão |
|---|---|
| U-CD-01 | O campo `+4` da entrada do `CMapServerManager` comparado com o `[4..5]` do `21` foi tratado como o grupo do `MapServerInfo.txt` (construtor `0x4aff00` inicia `+4` e `+8` com -1); falta ver o carregador |
| U-CD-02 | `obj+0x9fc` (hora do spawn) e `obj+0x28c = 1` não têm uso identificado; não reproduzidos |

## Illusion Temple (`CIllusionTemple`)

Neste build o Illusion Temple **não existe** no GameServer além de uma casca:

- o singleton em `0x01296100` (getter `0x47dbe0`, também chamado pelo item de menu 0x7d3, que só o constrói) tem um vetor em `+0xd70` que nunca é preenchido;
- a vftable (`0x54cac0`, localizada pelo RTTI `.?AVCIllusionTemple@@`) tem só o destrutor `0x488b80`; nenhum código lê campos do singleton (varredura por endereços `0x012960f8..0x01296e7c` no `.text`);
- `Event/IllusionTemple.dat` existe na pasta `Data`, mas o caminho não está nas strings do `GameServer.exe`, do `GameServer-bck.exe` nem do `CastleSiege.exe`: o arquivo nunca é lido;
- não há protocolo próprio (nenhum head `BF` é tratado), nem NPC, timer ou spawn.

O único comportamento observável é o ramo tipo 5 do **`91`** (`0x4d8430`, código em `0x4d868d`): `[4]` do pedido recebe `0xFF` (o "sem nível" da consulta de nível embutida), o jogador recebe `C1 86 F3 E4 00 00` + texto da mensagem 45 ("Você não é forte o suficiente para entrar no Illusion Temple") no idioma dele (`obj+0x56`), e **nenhuma resposta `91`** é enviada. O tipo 3 não tem ramo e recebe o `C1 07 91 03 00 00 00` zerado. O contador `IT` da tabela `EventEntryCount` é só carregado e gravado pelo DataServer; o GameServer nunca o altera.

As mensagens 40–50 do `Message.txt` (fechamento, início, artefato, "não é hora de entrar"…) não são usadas por nenhum código do GameServer além da 45.

Implementação: `internal/game/illusiontemple.go` (`eventTimeHandlers[5]`); teste em `illusiontemple_test.go`. O `IllusionTemple.dat` não é carregado.

### Incógnitas

| ID | Questão |
|---|---|
| U-IT-01 | Se o cliente 97D/99z deste pacote tem interface do Illusion Temple (mapas 45–50, NPC 385) que dependa de pacotes que o servidor não implementa; precisa da análise do `Main.exe` |

## Crywolf e Kanturu

**Não existem** neste build. `Event/Crywolf.dat`, `Custom/Crywolf.dat` e `Event/Kanturu.dat` estão na pasta `Data`, mas os caminhos não aparecem nas strings do `GameServer.exe` nem do `CastleSiege.exe` (a lista de arquivos de evento lidos termina em `Event\Kalima.dat`), e não há código que os leia. O mapa 34 "Crywolf" existe só no `MapManager.txt` e no `Move.txt`, como um mapa comum.

## Rankings (DS `0x3C`..`0x40`)

O GameServer manda ao DataServer, no fim de cada evento, um relatório com conta (`obj+0x6f`) e personagem (`obj+0x7a`), e o DataServer soma o valor na linha do personagem, inserindo-a se não existir (`SELECT` → `UPDATE ... SET Score=Score+%d` ou `INSERT`, `FUN_0040f470`..`FUN_0040fb80` no `DataServer.exe`):

| Head | Origem no GS | Valor | Tabela original |
|---|---|---|---|
| `C1 24 3C` | `0x476bc0`, chamado por `0x477430` no fim do duelo por `DuelMaxScore` | vencedor `(1, 0)`, perdedor `(0, 1)` | `RankingDuel` (`WinScore`, `LoseScore`) |
| `C1 20 3D` | `0x476c50`, no laço de `0x4146c0` (fim do Blood Castle) | campo `+4` do usuário (pontuação do resultado `93`) | `RankingBloodCastle` |
| `C1 20 3E` | `0x476ce0`, no laço de `0x430fxx` (fim do Chaos Castle) | campo `+4` do usuário (a pontuação que `0x431970` usa para achar o vencedor) | `RankingChaosCastle` |
| `C1 20 3F` | fim de `0x4705d0` (janela de ranking do Devil Square) | pontuação do usuário | `RankingDevilSquare` |
| `0x40` | nenhum emissor no GS (Illusion Temple ausente) | — | `RankingIllusionTemple` |

Layout `C1 20`: `[3]` byte não inicializado, `[4..5]` índice, `[6..15]` conta, `[16..25]` nome, `[28..31]` valor. O `3C` tem `[28]` vitórias e `[32]` derrotas. Quem morre no Blood Castle antes do fim já saiu da lista e não é reportado.

Implementação: `internal/game/ranking.go` (`reportDuelRanking`, `reportEventRanking`), chamado de `duel.go`, `bloodcastle.go`, `chaoscastle.go` e `devilsquare.go`; `internal/store/ranking.go` (`RankingStore`, migração `0011_ranking.sql`: `ranking_duel` e `ranking_event` com `event` = head do DS); importação das tabelas originais em `internal/migrate`.
