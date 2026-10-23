# 08c — Skills de jogador, efeitos (buffs/debuffs), PvP e duelo

Reconstrução a partir do GameServer WizardTeam 97D/99z (`4.GameServer/GameServer.exe`). Código: `apps/server/internal/game/{skill,effect,pvp,duel}.go` (+ edições cirúrgicas em `combat.go`, `object.go`, `world.go`). Endereços citados = funções do binário original.

## 1. Pacotes

| Pacote | Direção | Layout | Original |
|---|---|---|---|
| `C3 xx 19` | C→S | `[3]` skill, `[4..5]` alvo BE (skill `0x3a` = início da carga da Nova) | `FUN_00500e70` |
| `C3 xx 1E` | C→S | `[3]` skill, `[4]` x, `[5]` y, `[6]` dir, `[8]` ângulo (0..255), `[9..10]` alvo BE | `FUN_00501080` |
| `C3 xx 10` | C→S | `[3]` skill, `[7]` contagem (máx. 5), alvos de 3 bytes a partir de `[8]` (índice BE + 1 byte) | `FUN_00500b70` |
| `C1 04 1B` | C→S | `[3]` skill: remove o efeito (coluna Effect da SkillList) | `FUN_00501220` |
| `C3 08 19` | S→C | `[3]` skill, `[4..5]` usuário BE, `[6..7]` alvo BE, bit 7 de `[6]` = sucesso (CL) | `FUN_00501570` |
| `C3 09 1E` | S→C | `[3]` skill, `[4..5]` usuário BE, `[6]` x, `[7]` y, `[8]` dir — enviado antes do teste de mana | `FUN_00501740` |
| `C1 0C 07` | S→C | `[3]` 1 = efeito ligado / 0 = desligado, `[4..7]` bit do ViewSkillState (LE), `[8..9]` objeto BE, `[10..11]` lixo | `FUN_0047ab60` |
| `C1 07 BA` | S→C | `[3..4]` usuário BE, `[5]` 0x28, `[6]` estágio da carga da Nova | `FUN_0050e640` |
| `C1 05 B8` | S→C | `[3]` 1, `[4]` contador de kills de jogadores (obj+0x98c) | `FUN_0050e0c0` |
| `C1 A4 F3 E0` | S→C | atualização da ficha (nível, pontos, EXP, stats u16, vida/mana/BP u16 e u32, classe) enviada após perda de EXP | `FUN_004dba70` |
| `C1 10 AA` | S→C | `[3]` resultado (CL: 1 ok, 0 recusado), `[4..5]` índice BE do outro lado, `[6..15]` nome | `FUN_00478280` |
| `C1 0F AB` | S→C | `[3..4]` índice BE **do próprio receptor**, `[5..14]` nome dele | `FUN_004783e0` |
| `C1 0F AC` | S→C | pedido de duelo: `[3..4]` solicitante BE, `[5..14]` nome | `FUN_00478300` |
| `C1 09 AD` | S→C | placar: `[3..4]` A BE, `[5..6]` B BE, `[7]` placar A, `[8]` placar B | `FUN_00478460` |
| `C1 07 F3 40` | S→C | fogos do vencedor: `[5]` x, `[6]` y | `FUN_00477430` |
| `C1 10 BF 0A` | S→C | Chain Lightning: `[4]` skill alto, `[5]` skill baixo, `[6..7]` lançador **LE**, `[8]` contagem (sempre 3), `[9]` não inicializado (0 aqui), `[10..15]` três alvos u16 **LE**; vai ao lançador e ao VP2 | `FUN_004fdbd0` |
| `C1 07 B0` | C→S | Teleport Ally: `[3..4]` alvo **LE**, `[5]` x, `[6]` y | `FUN_005012a0` |
| `C1 07 DF` | S→C | posição após o push-back (`FUN_00511cb0` → `FUN_004d80d0`): `[3..4]` objeto BE, `[5]` x, `[6]` y | `FUN_004d80d0` |
| `C3 xx AA/AB/AC` | C→S | AA `[3..4]` alvo BE + nome; AC `[3]` aceita, `[4..5]` solicitante BE + nome; AB vazio | `FUN_00477890` / `FUN_00477d60` / `FUN_00478170` |

`HackPacketCheck.txt` exige C3 para 0x10, 0x19, 0x1E e AA..AC.

## 2. Validação do uso de skill (`FUN_004f8dd0`/`FUN_004f92d0`)

1. Jogador jogando, sem logout pendente, vivo; alvo conectado e vivo (0x19).
2. Zona segura (atributo 1): só skills da seção 0 de `SkillUseArea.txt` e dentro de um retângulo da seção 1 (`FUN_00502880`).
3. Lista de magias (`FUN_004f8cb0`, 0x3a→0x28); delay da SkillList (`FUN_004f83b0`, por skill em obj+0x600); `ReqKillCount ≤ obj+0x98c`; `ReqGuildStatus` (`r&0x20`, `r&0x40` ou `r&0x80` igual ao status da guild); coluna de classe.
4. Skills de arma (18–24, 49, 56, 62, 76, 0xdf..): um item equipado com opção de skill cuja coluna Skill do Item.txt seja a skill (`FUN_004f8800`). 41–43: item na mão direita; 41–43, 55, 60, 61, 65, 78: sem cajado (0xa00..0xc00); 24/51/52: regras de arco/besta + flechas.
5. Combo: classes 3/4/6 sempre; DK precisa da quest 3 completa (estado 2), DW/FE da quest 2 (`FUN_004dd740`, índice em EAX = 2 + (classe==1)). Máquina de estados `FUN_00434360` com `ComboSkill.txt`: skill estágio 0 abre, estágio 1 dentro do delay é o 2º golpe, outra skill estágio 1 dentro do delay fecha o combo (obj+0x6c4 = hora).
6. Mana `custo*obj+0x8b0/100 ≤ mana` e BP idem (taxas 100, `FUN_0050bc70`); só são cobrados se a skill foi executada; depois `C1 27 FF`.
7. `FUN_004f9730` zera obj+0x8d4 (skill Defense) e, com Authority 1, remove o efeito 18.

## 3. Skills implementadas (`FUN_004f9730`)

| Skills | Função | Comportamento |
|---|---|---|
| padrão (1–4, 7, 11, 17, 19–23, ...) | `FUN_004f9fd0` | alvo dentro de Range (Range 0 nunca passa; DL com Dark Horse +2); ataque com 0x19; combo manda 0x19 skill 59 |
| 5, 8, 9, 10, 12, 13, 14, 24, 47, 52, 55, 78 | `FUN_004fa0b0` | grava obj+0x38c; 5/47/13/78 também atingem o alvo; os demais alvos chegam por 0x10 (máx. 5 pacotes, alvos dentro de Radio; 0x19 só para 55) |
| 16 Mana Shield | `FUN_004fa4c0` | mesma party (dois sem party também); `ConstA + Dex/ConstB + Ene/ConstC` ≤ MaxRate × Rate[classe]; duração `TimeConstA + Ene/TimeConstB`; efeito 4 = redução de dano % |
| 18 Defense | `FUN_004fa720` | escudo com skill na mão esquerda; obj+0x8d4 = agora; ação `C1 09 18` 0x12; dano recebido −DefenseConstA% por DefenseTimeConstA s |
| 26 Heal | `FUN_004fa7b0` | vida + Ene/HealConstB + HealConstA (≤ máx.) |
| 27 Greater Defense | `FUN_004fa9d0` | `(Ene/ConstB + ConstA) × Rate/100 × 2` (confirmado no assembly: `LEA EDX,[ESI+ESI]`) |
| 28 Greater Damage | `FUN_004fac10` | `(Ene/ConstB + ConstA) × Rate/100` |
| 38/39 Decay/Ice Storm | `FUN_004fb080` | ponto dentro de Range; até 10 alvos dentro de Radio do ponto |
| 40 Nova | `FUN_004fb1c0`, `FUN_0050e640` | 0x3a inicia (obj+0x5c9, estágio 0); a cada 500 ms estágio+1 e `BA`; 12 solta sozinho; soltar = mensagem 0x32 após 600 ms a até 20 alvos; dano mágico + Str/NovaC + estágio×NovaB×NovaA |
| 41 Twisting | `FUN_004fb310` | até 10 alvos dentro de Radio; nada com cajado |
| 42 Rageful / 62 Earthquake / 76 Plasma | `FUN_004fb440` / `FUN_004fc0c0` / `FUN_004fd690` | mensagem 0x32 (0x35 p/ 76) após 500 ms; 10/20/10 alvos |
| 43 Death Stab | `FUN_004fb520` | 0x19, golpe no alvo, depois frustum 6×6 (ângulo = 0 no caminho 0x19) |
| 56 Power Slash | `FUN_004fbbb0` | frustum `FUN_004f7c00`: pontos (−6,6) (6,6) (1,0) (−1,0) girados por `ângulo×360/255` graus, truncados; teste `FUN_004f8340` |
| 44/57/73 e 45/46/74 | `FUN_004fc800` | só com `EnableCSSkillsAllMaps` ou mapas 30/31 |
| 48 Greater Life | `FUN_004fb720` | `Vit/B + A + Ene/C` ≤ MaxRate; duração `Ene/TimeB + TimeA`; party dentro de Radio (a taxa de classe multiplica o valor acumulado membro a membro, como no original) |
| 60 Force (Wave) | `FUN_004fbd40` | sem obj+0x5d8: golpe único; com: linha até o alvo |
| 61 Fire Burst | `FUN_004fbf00` | alvo + 10 dentro de Radio do alvo |
| 64 Critical Damage | `FUN_004fc3d0` | `Lead/ConstB + Ene/ConstA`, duração `Ene/TimeB + TimeA`, party em Radio |
| 65 Electric Spark | `FUN_004fc640` | `SkillElect.hit` (36 setores × 21 × 21), mensagem 0x32 após 1000 ms |
| 77 Infinity Arrow | `FUN_004fd760` | só em si; duração CServerInfo+0x1830 sem chave ⇒ 0 ⇒ nunca expira |
| 215 Chain Lightning | `FUN_004fdbd0` | testada antes da tabela de salto (`skill == 0xD7`). Alvo dentro de Range (`FUN_004f8130`); jogador passa pelo HackSkillSpeedCheck (`FUN_004e8290` + `FUN_00488550`), falha ⇒ não lança nem cobra mana. Três posições começam no alvo; o VP2 é percorrido em ordem e os dois primeiros objetos ≠ alvo aceitos por `FUN_004f8440` e dentro de Radio **do alvo** (`FUN_004f8280`) ocupam as posições 1 e 2. Posição i: mensagem 0x34 (`FUN_00510690`) após 200·(i+1) ms com golpe i+1; com combo, `0x19` skill 0x3B a cada posição. Por fim `C1 10 BF 0A` ao lançador (`FUN_00504f00`) e ao VP2. A `SkillList.txt` deste servidor não tem a linha 215, então a lista de magias (`FUN_004f8cb0`) a recusa antes, na prática (`skill_chain.go`) |

Alvos de área (`FUN_004f8440`): percorre VP2 (obj+0x3bc); aceita monstros (exceto 100..110 e 0x20b), invocações seguem o dono; jogadores só se forem o alvo explícito, em duelo com o lançador ou em guerra de guild.

## 4. Dano (`FUN_0040b110`)

Ordem: imunidades (efeito 34 sempre erra; 59 sem skill / 60 com skill; monstros 63/64 — usados pelo Blood Castle via `eventEffectHook`) → maça 2,4/2,5: 1/30 de lançar Ice → acerto (`AttackRate + obj+0x70c` vs `DefRate + obj+0x71c`; abaixo ⇒ 5% com 30% do dano) → defesa (`Defense + Greater Defense`, metade contra jogador, IgnoreDefense) → dano físico (`FUN_0040ec10`: skill soma DamageMin/Max = Damage, Damage×1,5; DL soma `Str/DLSkillConstA + Ene/DLSkillConstB`, Earthquake e Electric Spark têm fórmulas próprias; duas armas com `TwoWeaponsDamage*ConstA`; Greater Damage obj+0x6e8; crítico = máx + bônus + obj+0x750; excelente = máx×1,2 + bônus) ou mágico (`FUN_0040f380`, DW/MG com skill não física; cajado soma a "magic rise" %) → `SkillDamage.txt` seção 0 → redução 30% → DamageReduction[6] → Mana Shield → Defense → dano mínimo `nível/10` → multiplicador DK (`DKDamageMultiplier` ou 200%) / DL / Plasma / Chain Lightning (215: golpe 2 = 70%, golpe 3 = 50%, os demais 100%, pelo `param_7`) → duplo/triplo → combo (`ComboB × (Str+Dex+Ene)/ComboA + 2×dano`, × ComboC/100, skill 59, efeito |0x80) → taxas PvM ou PvP (`GeneralDamageRatePvP × <C>DamageRatePvP × <C1>DamageRateTo<C2>`; duelo `DuelDamageRate`; arena; Chaos Castle) → DamageTable → 4 rolagens de recarga do atacante → efeitos da skill (`FUN_0040e300`) e 0x19 → SD → vida → monstro: msg 0 → self defense → `DamageStuckRate` (flag do D7) → reflexão (`Opt109` total, ou `DamageReflect + efeito 71` %, mensagem 10 após 10 ms; o refletor ataca com dano fixo, efeito 4, `ReflectDamageRate*` e matriz `<C1>ReflectDamageRateTo<C2>`) → `lifeCheck`.

Efeitos de skill ao acertar (`FUN_0040e300`, com `SkillDamage.txt` seção 1 como chance): Poison/Decay (valores: atacante, intervalo `ConstA` s, `ConstB`% da vida atual, teto), Ice/Ice Storm (lento: obj+0x296), Ice Arrow (imobiliza), Fire Slash (`ConstA + Ene/ConstB` ≤ MaxRate), Lightning (3) e DK 19..23 (push-back, abaixo).

**Push-back** (`pushback.go`): o `FUN_0040e300` põe a mensagem 2 no alvo, 150 ms depois, com o atacante como remetente (`FUN_00510550`, sem condição extra). Monstros a tratam em `FUN_004b48f0` caso 2 e jogadores em `FUN_004c4120` caso 2; as duas chamam `FUN_005110e0` só se o remetente estiver vivo:
- recusa: mapa fora do MapManager; jogador teleportando ou com Authority (obj+0x250) 8; classes 0x83..0x86, 0xd7..0xdb, 0x113, 0x115, 0x116, 0x11b, 0x120, 0x15c, 0x1cb..0x1ce, 0x20b..0x210, 0x256..0x25a, 0x273..0x277, 0x291, 0x2a1; classes de `Custom/CustomMobCombo.txt` (lista de até 100 lida por `FUN_0044ef50` e consultada por `FUN_004e8260`/`FUN_0044f040`; o arquivo traz só 500); o usuário da coroa (CCastleSiege+0x80) montado em Dark Horse;
- direção = a do atacante (obj+0x1ac), invertida (±4) quando `rand()%3 == 0`; uma célula (`DAT_005683d4`), aceita se o atributo `& 0x0f == 0` e, no mapa 30, sem `0x10`;
- move com `FUN_00511cb0` (`C1 05 DF x y` sintetizado → `FUN_004d80d0`, que envia `C1 07 DF` ao objeto e ao VP2) e zera o caminho (obj+0x1d8) dos jogadores.

`FUN_005114f0` (mensagens 6 e 7 do monstro, 2 e 3 passos) é o empurrão de vários passos: direção `FUN_005122d0` do atacante para o alvo; cada passo exige `atributo & 0x1d == 0`, senão inverte a direção para o passo seguinte; do terceiro passo em diante só move as classes 1, 3 e 4; poupa também os mercenários 0x11e/0x11f; reenvia a posição mesmo sem andar. O `CAttack` manda a mensagem 6 (tratada na hora) à bola do battle soccer (classe 200) e o golpe erra. A mensagem 0x39 (também `FUN_005114f0`) vem das skills de monstro e não foi ligada.

**SD:** `FUN_00410370` existe, mas a taxa vem de CServerInfo+0x494, que nenhuma chave carrega neste build ⇒ 0 ⇒ o escudo nunca absorve dano.

## 5. Sistema de efeitos

`EffectList.txt` (`FUN_004786c0`) + 16 slots por objeto (obj+0x5fc, 0x1c bytes). `AddEffect FUN_00478ae0(obj, 0, efeito, duração, v1..v4, envia)`: mesmo grupo substitui (com `EffectOverwriteMode 0` uma cópia mais fraca do mesmo efeito é recusada); colunas Power/Duration do arquivo sobrescrevem. EffectOn/Off (`FUN_00478ef0`/`FUN_004798d0`): 1 → +dano físico/mágico/maldição; 2 → +defesa; 3 → ambos; 4 → redução de dano; 5 → +dano crítico; 8/9 → LifeRate/ManaRate (CalcCharacter); 11 → velocidades; 56 → passo lento; 71 → reflexão; depois CalcCharacter. Bit do ViewSkillState (`FUN_0047a9e0`, tabela em 0x47ab10): 1→0x4, 2→0x8, 3→0x2000000, 4→0x100, 5→0x800, 6→0x4000000, 8→0x10, 9→0x2000, 18→0x4000, 28→0x1000000, 55→0x1, 56→0x2, 57→0x20, 58→0x40, 61→0x1000 (efeitos > 70 não têm bit). Timer de 1 s (`FUN_00478910`): decrementa, dano periódico (`FUN_0047a6b0`, efeito de dano 5 no D7) e expira. `ClearAllEffect FUN_0047a540` remove os de Type 0 (na morte). `moveEffectHook` = "objeto tem o efeito".

**Persistência (Save = 1)** (`effect_save.go`): o save do personagem (`FUN_00475c30`) grava os 16 slots em `Character.EffectList` (208 bytes) com `FUN_0047a3d0`, 13 bytes por slot: `[0]` índice, `[1..4]` contagem em segundos (big endian; para Type 2 o horário Unix de fim, CEffect+0xc), `[5..12]` os 4 valores u16 big endian; slot livre ou sem Save = 13 × 0xFF. Na carga (`FUN_004cc730`, depois do inventário e antes do CalcCharacter) cada registro com índice ≠ 0xFF volta por `AddEffect(obj, 1, …, envia = 0)`: com `param_2 = 1` a duração do EffectList.txt **não** sobrescreve a salva (as colunas Power continuam valendo); Type 2 é recusado se o fim já passou (`FUN_004785e0`). No EffectList.txt deste build têm Save = 1: 1..6, 8..11 e 71. No Go os efeitos somem no `leaveWorld` antes do save final, por isso um gancho anterior guarda a lista em `Char.EffectList`.

## 5b. Summon Party, Teleport Ally e skills de guild do Castle Siege (`skill_guild.go`)

- **63 Summon Party** (`FUN_004fc1a0`): para cada outro membro da party (`CParty`, 5 slots), sem limite de distância: se `CMoveSummon` (`FUN_004bff20`, `Move/MoveSummon.txt`) recusa o destino (mapa/posição do lançador) ⇒ mensagem 272; no mapa 30 durante a batalha (estado 7) com lado diferente (obj+0x97e) ⇒ 272; senão uma célula a ±4 do lançador (`FUN_0050c860`, 10 tentativas, senão a própria posição) e contagem obj+0x5d2 = 5 com obj+0x5d4 mapa, +0x5d5 X, +0x5d6 Y e mensagem 273 ("em %d segundo(s)"). Depois `0x19` ao lançador; retorna 1 mesmo sem party.
- **Contagem** (`FUN_00510170`, a cada segundo): 273 com o valor restante; ao chegar a 0 refaz o `FUN_004bff20` no destino (falha ⇒ 272) e chama `FUN_00510000`: no mesmo mapa `0x19` skill 6 a si mesmo + Teleport (`FUN_0050e180`); em outro mapa troca direta com `C3 08 1C FF mapa x y dir` (regen pendente como no gate). Objetos em modo offline não se movem. Andar (`FUN_004d8b80`) ou mover por gate (`FUN_0050f6d0`) zera a contagem com 272.
- **`FUN_004bff20`** recusa: teleportando, morrendo (obj+0x286), `PKLevel > 4` com `PKLimitMoveSummon` e sem `PKLimitFree`, mapa 30 fora da batalha em x 0xa1..0xbf, y 0xbc..0xd8 (núcleo do castelo), Atlans com Uniria/Dinorant, Icarus sem asas nem Dinorant/Fenrir, modos offline, self defense ativa (os 5 slots obj+0x2c4/0x2d8), e exige uma linha do MoveSummon.txt (`Map X1 Y1 X2 Y2 MinLevel MaxLevel MinReset MaxReset AL0..3`) com o ponto dentro: nível mínimo ×2/3 para MG, DL e classe 6 (`FUN_00480f80`), máximo, resets (obj+0x6dc) e a chave da AccountLevel.
- **B0 Teleport Ally** (`FUN_005012a0`, skill 15 "Teleport Party" do DW): `C1 07 B0 alvo(u16 **little endian**) x y`. Alvo inválido ⇒ o próprio lançador. Exige party igual dos dois, lançador sem efeito 61/72/147/178 (`FUN_0047a470`), mesmo mapa e `FUN_0050d0a0` para o **alvo** (vivo, sem teleporte, destino a ±8 células do alvo, nem a célula do alvo nem o destino em safe zone). No mapa 30: lançador com Y < 0x90 passa por `FUN_0041f400` (linhas de portão 0x71/0xa0/0xcb mantêm o Y do lado do lançador) e `FUN_00426e80` prende um alvo da sala da coroa em x 0xa4..0xbb, y 0xc0..0xd5. Skill 15 na lista; delay da SkillList (falha ⇒ `C3 08 1C 00` ao lançador, `FUN_004bf340`); mana/BP; `0x19` (skill 15, alvo), Teleport no alvo (`FUN_0050e180`) e o lançador paga mana/BP (`C1 27 FF`).
- **Skills de guild (mapas 30/31 apenas; fora ⇒ falha sem custo)**: todas zeram o contador de kills (obj+0x98c) com `C1 05 B8 01 00` e agem sobre os jogadores do VP2 dentro do Radio da skill:

| Skill | Função | Efeito |
|---|---|---|
| 67 Stern | `FUN_004fc970` | byte de ângulo do 1E = `(dy+8)·16 + (dx+8)` do ponto; graus = `atan2(8−alto, 8−baixo)·180/π + 90` (truncado, +360 se negativo), setor = graus/10 em `SkillElect.hit`; jogadores no setor, sem teleporte e sem os efeitos 0x86/0xa5: Stern (61) por `SternTimeConstA` s, `FUN_00511cb0` na própria posição e `0x19` por alvo (nenhum `0x19` ao lançador) |
| 68 Remove Stern | `FUN_004fccc0` | `0x19` ao lançador; tira o efeito 61 de cada jogador no Radio + `0x19` |
| 69 Greater Mana | `FUN_004fce40` | efeito da SkillList (9) com valor `GreaterManaConstA` por `GreaterManaTimeConstA` s no lançador e nos membros da mesma guild (obj+0x308) no Radio |
| 70 Invisibility | `FUN_004fd0c0` | alvo jogador da mesma party (senão o próprio lançador) dentro do Range: efeito 18 por `InvisibilityTimeConstA` s; zera o contador por `FUN_0050e0c0` |
| 71 Remove Invisibility | `FUN_004fd1b0` | como 68 com o efeito 18 |
| 72 Remove All Effect | `FUN_004fd330` | como 68 com `FUN_0047a540` (todos os efeitos Type 0) |
| 75 Brand | `FUN_004fd4a0` | efeito 19 no lançador e em **todo** jogador no Radio (sem filtro de guild); duração CServerInfo+0x182c, sem chave ⇒ 0 ⇒ não expira |

## 6. PvP (`FUN_0040dca0`) e morte

Alvo não jogador sempre permitido; alvo com Authority 8 nunca; `PartyDisableKillBetweenMembers`; guerra de guild (`guildWarHook`) permite fora de NonPK; arena; DS/BC só com `*DuelEnable`; Chaos Castle só no estado 4; por fim `EnableKillLevel1` ou ambos com nível > 5 ⇒ permitido se o mapa não é NonPK (`FUN_004ae780`: coluna NonPK do MapManager ou chave `NonPK`).

Morte de jogador (`FUN_004cda80` → `FUN_0050e8a0`): matador jogador ganha +1 em obj+0x98c (`B8`) e `guildWarKillHook`; vítima zera o contador; anúncio de PK (`PKDeathAnnounce`, msg 0x17f); em duelo só placar (`FUN_00477830`); fora disso penalidade `FUN_0050ec70`: EXP −`ExperienceDeduceRateN`% do intervalo do nível (N por PKLevel: <3, 3, 4, 5, ≥6; aritmética u32, piso no início do nível) + `F3 E0`, zen −`MoneyDeduceRate`%; depois PK (`playerKillHook`, fora de duelo) e fim de duelo (`FUN_00477430`); efeitos de Type 0 limpos.

## 7. Duelo

Estado obj+0x43c (pedi), +0x440 (me pediram), +0x444 (oponente), +0x448 (placar), +0x44c (tick). AA: mapa com `DuelEnable` (ou `DuelSwitch`), alvo válido, livres, flag 2 de obj+0x434 do alvo, nome confere; mensagens 0xa0..0xab. AC recusa ⇒ `AA` resultado 0 ao solicitante; aceita ⇒ `AA` 1 aos dois + `AD`. Pedido expira em 10 s, duelo em 60 s sem golpes (cada golpe entre os duelistas renova obj+0x44c). `DuelMaxScore` termina com `AB` aos dois e `F3 40` no vencedor; anúncios com `DuelAnnounceSwitch` (msgs 0x2d0..0x2d2).

## 8. Incógnitas

| Id | Descrição |
|---|---|
| U-SKILL-01 | ~~Invocações, Summon Party, Teleport Ally e skills 67–72/75 não implementadas.~~ **Resolvida**: invocações pelo agente H; o resto na seção 5b. |
| U-SKILL-07 | `FUN_004bff20` (MoveSummon): obj+0x20b0, obj+0x3d4&3 e obj+0x2468 sem leitor identificado (ignorados); os 5 slots obj+0x2c4/+0x2d8 aproximados pela self defense do Go (`SelfDefenseTime`). |
| U-SKILL-08 | `FUN_00510000` chama `FUN_00472230` e `FUN_00487d60` no fim da troca de mapa da Summon Party (não identificados); usamos os `mapChangeHooks` e o `placeAt` do gate. |
| U-SKILL-09 | Efeitos 0x86 e 0xa5 (imunes ao Stern) não existem no EffectList.txt deste build. O sucesso (CL) do `0x19` skill 6 da Summon Party foi assumido 1. |
| U-SKILL-02 | Byte de elemento do atacante (obj+0x3af+Type) somado ao dano de skill: tratado como 0. |
| U-SKILL-03 | Atraso da mensagem 0x32 de 45/46/74 (`FUN_004fc800`) passado em registrador perdido; usado 250 ms. |
| U-SKILL-04 | Linha do Force Wave (`FUN_004bd920`/`FUN_004f75d0`) e ângulo do Electric Spark (atan2 com argumentos perdidos): aproximados (frustum até o alvo; ângulo do lançador ao ponto). |
| U-SKILL-05 | ~~Push-back não reproduzido.~~ **Resolvida** (seção 4, `pushback.go`). Continuam abertos os efeitos de skill 0xd6.. e a mensagem 0x39 das skills de monstro. |
| U-SKILL-10 | ~~Skill 215 (Chain Lightning, `FUN_004fdbd0`) ausente e sem a trava do HackSkillSpeedCheck.~~ **Resolvida** (seção 3, `skill_chain.go`). O byte `[9]` do `BF 0A` fica sem inicializar no original (lixo de pilha); enviamos 0. O cliente 97D não usa a skill (sem linha na SkillList). |
| U-SKILL-06 | Combo customizado (obj+0xcc/0xce/0xd2, `FUN_004f8d10`, comando `/combo`) e `CheckAutoComboHack` não reproduzidos. |
| U-EFF-01 | ~~Persistência dos efeitos com Save=1 não feita.~~ **Resolvida** (seção 5, `effect_save.go`). Nenhuma linha do EffectList.txt tem Type 2: o caminho do horário absoluto só é testado com uma linha injetada. |
| U-EFF-02 | Campo obj+0x780 (valor 2 da Soul Potion) e obj+0x268 do Ice sem leitor identificado. |
| U-PVP-01 | SD: taxa CServerInfo+0x494 nunca carregada ⇒ 0 (shield não absorve). Confirmar com captura. |
| U-PVP-02 | Asas/pets no dano (`FUN_0040cbd0`, `FUN_0040cee0`), durabilidade (`FUN_0040d220`, `FUN_0040d9a0`, `FUN_0040db70`) e a busca de `FUN_00455310` do DamageStuck não reproduzidas (stuck = jogador sem pet + taxa da classe). |
| U-PVP-03 | Campos custom do `F3 E0` (obj+0x618..0x62c, textos 0x2ea..0x2ed, obj+0x6dc/0x6e0) enviados como 0. Drop de item na morte (`DropItemRate*`, todos 0 aqui) não feito. |
| U-DUEL-01 | Flag "aceita duelo" = bit 1 de obj+0x434 (party `Flags`, padrão 3); origem do valor não confirmada. Teste de trade ocupado via `tradeOpenHook` (não ligado pelo módulo de trade). |
