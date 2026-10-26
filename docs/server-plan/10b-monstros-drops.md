# 10b — Monstros (skills, área, traps, summons) e sistema de drop

Reconstrução do lado "monstro" e do drop do GameServer (emulador customizado 97D/99z). Código em `apps/server/internal/game/{drop.go,monster_skill.go,summon.go}` e `internal/gamedata/{drops.go,sets.go}`. Todos os endereços são do `GameServer.exe` original.

## 1. Drop na morte do monstro — `FUN_004b7290`

Chamado pela mensagem 1 (morte, 500 ms depois, `FUN_004b70b0`). Ordem exata:

1. dono = maior dano (`FUN_004b6df0`) ou o matador; summons (`obj+0x310` válido) não dropam;
2. classes sem drop: `0x84..0x86, 0xF7, 0xF9, 0x115, 0x116, 0x11B, 0x120, 0x1CC..0x1CE, 0x20C, 0x20D, 0x20F, 0x210, 0x25D..0x25F`;
3. `Custom/CustomGuard.txt` seção 0 (até 100 classes, `FUN_0044f040`) → sem drop;
4. `obj+0x24b4` (CustomMonster com SpecialBag) → sem drop;
5. bag por classe (`FUN_004913c0`, EventItemBagManager coluna MonsterClass) → termina;
6. item de quest (`CQuestObjective FUN_004de9b0`, `questDropHook`) → termina;
7. `DropEvent.dat` (`FUN_00471690`) — vazio nesta instalação;
8. `Item/ItemDrop.txt` (`FUN_00492870`) → termina se dropou;
9. `Item/ItemMoneyDrop.txt` (`FUN_0049c920`) → termina se dropou;
10. mapas 18..23 (Chaos Castle) → nada;
11. taxa de item = `bonus(1, mapItemRate × (ItemDropRate_AL × (obj+0x874 + obj+0x770) / 100) / 100)` (bonus = 100 sem CBonusManager);
12. nível ≥ 25: `rand % 1e6 < SetItemDropRate` → ancient aleatório (`FUN_004f3cb0`) e fim; senão `rand % 1e6 < ExcItemDropRate` → nível −25 e drop excelente;
13. item da tabela de nível (`FUN_004ba8c0`): excelente só considera índices `< 0x1800`; `rand % ItemRate < taxa` → item;
14. opções: para cada opção N (0 nível, 1 skill, 2 luck, 3 JoL, 4 qtde exc, 5 set) a linha do `MapRateInfo.txt` (mapa ou `*`, tipo 0 comum / 1 excelente) indica a linha do `ItemOptionRate.txt`; para itens `>= 0x1800` nível, skill, luck e exc são zerados (JoL e set não); `MakeNewOption(idx, 1, qtde)` e `MakeSetOption(idx, set)`;
15. sem item: zen = `MoneyAmountDropRate_AL × obj+0xf8 (EXP dada) / 100 × obj+0x878 / 100` (mín. 1) se `rand % MoneyRate < mapMoneyRate / 10`.

### MapManager (resolve offsets "inferidos" do agente B)

O loader `FUN_004ae480` grava o registro (`value` em `node+0x10`): `+0x00` Map, `+0x04` NonPK, `+0x08` ViewRange, `+0x0C` ExperienceRate, `+0x10` (não lido), `+0x14` ItemDropRate, `+0x18` MoneyDropRate, `+0x1C` ExcItemDropRate, `+0x20` SetItemDropRate, `+0x30` ItemDrop, `+0x34` PKItemDrop, …, `+0x58` RespawnGate. Logo, relativo ao nó: **`+0x1c` = ExperienceRate, `+0x24` = ItemDropRate, `+0x28` = MoneyDropRate, `+0x2c` Exc, `+0x30` Set, `+0x40` = flag ItemDrop, `+0x68` = RespawnGate** — confirmados.

### Geradores de opção

- `ItemOptionRate.txt` (`FUN_0049ea70..FUN_0049efc0`): seções 0..5 com 16, 2, 2, 8, 7, 7 taxas; escolha `rand % soma` sobre intervalos `[acum, acum+taxa)` só das taxas ≠ 0; linha inexistente ou soma 0 deixam o byte como estava.
- `MapRateInfo.txt`: linhas de 9 ints (`map`, tipo, 6 linhas de opção); primeira que bate (`map == -1` ou igual) — `FUN_004afd90..FUN_004afe80`; sem linha → 0.
- `MakeNewOption` (`FUN_0049f0d0`): limite 3 para 12,3 e 12,37; `WingOption.MaxNewOption` (≤ 6) para asas; 4 para CustomWing; 6 caso contrário. Modo 0 = máscara direta, 1 = N opções, 2 = `rand % N`. Cada sorteio vem do `ExcellentOptionRate.txt` (`FUN_0047bd30`: linhas cujo intervalo contém o item, peso = DropRate); só conta bits novos. **O original entra em laço infinito se não há opções distintas suficientes; aqui o laço para após 1000 sorteios.**
- `MakeSetOption` (`FUN_0049f270`), byte ancient = tier (bits 0-1) | nível (bits 2-3, 1 = +5, 2 = +10): valor 1/2 → candidatos tier1 (`1→5`, `2→9`) e tier2 (`2→6`, `1→10`) sorteados; 3/4 → tier1 (5/9); 5/6 → tier2 (6/10). Assimetria observada: com valor 1 o tier 2 recebe +10.

### Criação do item — `FUN_004753e0`

Durabilidade 0 → `FUN_00494500` (máxima); skill zerada se Item.txt `Skill = 0`; serial 0 para `HaveSerial = 0`, senão `0xFFFFFFFF` (pedido ao DataServer na coleta); mapa `0xEB` = direto no inventário (resposta `FUN_004742b0`: `InventoryInsertItem` + `C1 0C F3 14`).

### ItemDrop.txt / ItemMoneyDrop.txt

- Só em mapas com `ItemDrop = 1`. Cada linha que bate (mapa, monstro, faixa de nível; `*` = qualquer) passa por `rand % 1e6 < DropRate[AL]` (`*` = sempre) e entra com peso `1e6 / DropRate`; sorteia uma. Durabilidade = coluna Grade, nível = coluna Level (substituída se a linha de opção existe).
- **Tokenizer original**: uma palavra (`N`) não altera o número corrente, então lê o valor da coluna numérica anterior. Nas linhas "simples +0" do ItemDrop as opções `N` valem 0 = linha "Common" do ItemOptionRate (até +3, 6% skill, 4% luck, 4% JoL), apesar do comentário do arquivo dizer "sem skill/luck".
- ItemMoneyDrop: 7 colunas (Map, Monster, MinLevel, MaxLevel, AccountLevel, MoneyAmount, Rate); valor `*` = EXP do monstro (ou 1); × `obj+0x878` / 100.

### Bags — `EventItemBagManager.txt` + `EventItemBag/**`

- Gerenciador (entradas de `0x1a8` bytes): ItemIndex, ItemLevel, MonsterClass, SpecialValue, SpecialLevel, SpecialState, caminho. Buscas: por classe (`FUN_004913c0`), por valor especial (`FUN_00491430`, usado por eventos: 1 = Devil Square, 2 = Blood Castle, 3 = Chaos Castle… e pelo SpecialBag do CustomMonster com nível −1) e por item (`FUN_00491340`, ao largar o item com `23`).
- `CItemBagEx` (`FUN_0048f2e0`): seção 2 `ItemRate MoneyDrop BagUseEffect DropType`; 3 `BagIndex DropRate MapNumber`; 4 `BagIndex Section SectionRate SetDropRate Min Max DW DK FE MG DL` (contagem inválida descartada); ≥5 itens `cc,iii Level Grade Opt0..5 Duration`.
- Drop (`FUN_0048feb0`): para cada bag do mapa com `rand % 10000 < DropRate`, escolhe uma linha da seção 4 pelo peso entre as que permitem a classe (`valor ≠ 0` e `≤ changeUp + 1`); `rand % (Max-Min+1) + Min` drops; posição exata no 1º drop do 1º bag se a célula serve, senão aleatória ±2 (10 tentativas, `FUN_0050c860`), senão a do jogador; set se `rand % 10000 < SetDropRate` ou seção `*`; zen (`MoneyDrop`) se `rand % 100 >= ItemRate`; senão item aleatório da seção com opções do ItemOptionRate; byte excelente vazio recebe o Grade; itens empilháveis (`ItemStack`) usam Grade como durabilidade; DropType ≠ 0 vai ao inventário se cabe. No fim, `BagUseEffect` 1/2 → `C1 07 F3 40 tipo X Y`, 3 → `F3 40 3A idx`.
- Largar item (`FUN_004988f0`): bag por índice consome o item (resposta `23` sucesso); fogos `0x1c3f`/`0x1c63` → `C1 07 F3 40 00 X Y` e consomem o item.

### Zen na coleta com party — `FUN_004982e0`

Com `PartyMoneyDistribute` (`CServerInfo+0x5c4`) e party: divide igualmente entre membros no mesmo mapa a distância < 10 do coletor (ele incluso), cada um recebe `C3 10 22 FE money(BE)`; sem isso tudo vai ao coletor.

### CustomMonster.txt (11 colunas, registro `0x3c`)

`FUN_00466d40` na criação: MaxLife, DamageMin/Max, Defense, AttackRate, DefenseRate em %; SpecialBag ≠ `*` liga `obj+0x24b4` (sem drop normal). Na morte (`FUN_00466f00`): KillMessage (aviso, não feito) e SpecialBag via `FUN_00491430(bag, -1)`. **Não há colunas de taxa de drop/zen nesta versão** — o "drop rate" do CustomMonster é o SpecialBag. ExperienceRate já estava no agente D.

## 2. Ancient (set) — `FUN_004f2f70`, `FUN_004f30f0`, `FUN_004f3240` (resolve U-STATS-01)

- `SetItemType.txt`: ItemIndex, StatType, OptionIndex1, OptionIndex2, DropRate (mapa ordenado por índice).
- `SetItemOption.txt`: 100 linhas de `0xac` bytes: índice, nome, 6 tiers × 2 (opção, valor) a partir de `+0x24`, 5 opções completas em `+0x84`.
- `FUN_004f2f70` (laço de requisitos do CalcCharacter): cada peça ancient equipada com StatType ≠ `*` soma `((set>>2)&3) × 5` ao atributo (sem checar validade).
- `FUN_004f2d50`: peças válidas (`CItem+0xb5`) do set (índice da opção pelo tier `set&3`); armas (slots 0/1) e anéis (10/11) iguais do mesmo set contam uma vez.
- `FUN_004f30f0(pass)`: com n peças aplica os tiers 0..n−2; se `n−1` = nº de tiers com alguma opção, aplica as 5 completas e liga `obj+0x5d1`. Passe 1 (no laço de requisitos) só aplica códigos 0..4; passe 0 (depois das opções de item) só os demais.
- Códigos (`FUN_004f3240`): 0 Str, 1 Agi, 2 Ene, 3 Vit, 4 Comando (`+0x174..+0x184`); 5 dano mín. (+0x33c/+0x344); 6 dano máx.; 7 mágico %; 8 mín.+máx.; 9 taxa de ataque %; 10 defesa; 11 vida (`+0x1b4`); 12 mana (`+0x1b8`); 13 BP (`+0x190`); 14 `+0x884`; 15 taxa crítica; 16 dano crítico (`+0x848`); 17 taxa excelente; 18 dano excelente (`+0x850`); 19 `+0x830`; 20 `+0x834`; 21 ignorar defesa; 22 defesa % com escudo; 23 dano % com arma de duas mãos; 24/25 dano + For/Agi ÷ v; 26/27 defesa + Agi/Vit ÷ v; 28 mágico + Ene ÷ v; 29..35 resistências (bytes).
- Ancient aleatório (`FUN_004f3cb0` + `FUN_004f3ad0`): peso DropRate entre itens do SetItemType com Item.txt `DropItem`; opções da linha tipo 2 do MapRateInfo, sem a máscara `0x1800`.

## 3. Ataques de monstro

- **MagicList do monstro** (`FUN_004b4fe0`): slot 0 = AttackType (150 → 50) quando ≠ 0; classes de evento ganham skills extras (tabela em `monsterSkillList`).
- **Escolha do ataque** (`FUN_004b4090`): se a classe tem MonsterSkill → `rand % 4 == 0` D7, senão unidade de skill; senão AttackType 50 → área; `> 99` → área com `rand % 5 == 0`; senão `FUN_004b7d00`.
- **`FUN_004b7d00`**: D7 = `C1 07 D7 alvo 0x78 dir` para o handler D7; magia = `C1 07 19 slot alvo 00` para `FUN_00500e70` (o byte [3] é o *slot* da MagicList). Por classe: 0x42/0x49/0x4d e 0x59.. → slot 1; grupos BC/DS → 50% slot 0 / 50% D7; 0x95.. → slot `rand&1`; 0xa1.. (Kalima/BC bosses) → slot `rand%6+1` + ataque em área; 0xa3..0xad, 0x1ab → slot 0; demais: AttackType 0 ou >99 → D7, senão slot 0.
- **Área** (`FUN_004b7a20`): `C3 09 1E skill idx(BE) X Y 00` ao VP2; alvos = jogadores do VP2 a distância < 6 (até 20). Bosses 0xa1.. batem em todos; os outros passam por `FUN_00500b70`: skill tem de estar em {5,8,9,10,12,13,14,24,47,50,52,55,78} e no máximo 5 alvos.
- **Bug do agente B corrigido**: em `FUN_004b85a0` os 10% de hits ignorados dependem do *atacante* não ser summon, não do monstro.
- O handler da IA ignora monstros com efeito 72 (`0x48`) (`FUN_004b44a0`).

### MonsterSkill (`Monster/Skill/*.txt`)

- `MonsterSkill.txt` (registro `0x58`): monstro, contagem, 10 tipos, 10 ponteiros de unidade. Só 304 Witch Queen, 305 Blue Golem e 309 Hell Maine (Aida) têm linhas.
- Unidade escolhida ao acaso entre os slots com Tipo 0 (`FUN_004bdd10`); `C1 08 69 unit monstro(LE) alvo(LE)` ao VP2; TargetType 2 = o próprio monstro; ScopeType −1 = só o alvo, 0 = jogadores vivos do VP2 a distância < ScopeValue, 1 = tabela de área (aproximada por distância).
- Elementos (`FUN_004be5b0`, sucesso `rand % 100 <= SuccessRate`): 0 stun (efeito 61) salvo efeito 134; 2 vida / 3 mana (IncAndDecType 1/101 +v% do atual, 2/102 −v%, 11/111 +v, 12/112 −v; limitado a `[0, Max+Add]`, envia `C1 26/27 FF`; não mata); 7 durabilidade de um slot 2..7 ao acaso; 14 remove efeitos 27/28; 16 imunidade (efeito 63). FUN_004be030 só acrescenta CAttack para classes de evento.

### Traps (classes 100..110 e 0x20B) — `FUN_004b5be0`

Escolhe ao acaso um jogador do VP2 (Authority ≠ 2 e ≠ 8) dentro do quadrado ±AttackRange (com AttackRange 0 o primeiro). Alvo → ActAttack com delay AttackSpeed; sem alvo → delay MoveSpeed. Depois segue o fluxo normal de ataque (traps não retornam cedo).

### Regen por Attribute — `FUN_004b4e40`

6: renasce no lugar; 5/7/8/9: célula livre a ±5 da morte (30 tentativas, `FUN_0050c860`); demais: registro de spawn.

## 4. Summons

- Criação (`FUN_004fae50`, chamada pelas skills 30..36 e 432 via `summonSkillHook`): não em Icarus (10) nem Chaos Castle (18..23); classe = `SummonMonster1..7` (Skill.dat, `CServerInfo+0x17b0..0x17c8`; 432 lê `+0x17cc`, chave desconhecida); já existe summon → é removido e `C1 05 F3 20 00`. Índice round-robin `0x5014..0x53FB` (`FUN_004b4ce0`), posição (X−1, Y+1), dir 2, Attribute 4, MoveRange 15, dono em `obj+0x310`; `C1 05 F3 20 vida%` ao dono.
- IA: o timer processa todos os objetos (summons inclusos); Attribute 4 sem alvo segue o dono (`FUN_004b6370`); em outro mapa ou a ≥ 15 células → teleporta para o dono (`FUN_0050e180`); ataque do dono dá alvo ao summon ocioso (`FUN_004b58f0`, emoção 1, contador 30); dano no summon envia `F3 20` ao dono.
- Remoção (`FUN_004b5960`): morte do summon (após o drop), morte do dono, saída do dono, novo cast. **Não há temporizador de expiração neste build.**
- Viewport: `C2 1F` (`FUN_00515890`), entradas de 24 bytes: índice BE (bit 7 novo), classe BE, estado de skill (u32), X Y TX TY, `dir<<4 | PK`, nome do dono (10), 1 byte de padding. O `C2 13` pula summons.

## Incógnitas

| ID | Descrição |
|---|---|
| U-DROP-01 | `obj+0x874`/`+0x770` (taxa de drop do dono) e `CBonusManager` assumidos 100/0/sem bônus. |
| U-DROP-02 | Chave `SpecialState` (registrador EDX) de `FUN_00491430` não decodificada: aceita qualquer estado. |
| U-DROP-03 | `Duration` dos itens de ItemDrop/bag não é guardada (`item.Item` sem expiração). |
| U-DROP-04 | Campo `+0x2c` do CItemBagEx (avisos tipo 1..3 com `FUN_004c08f0`) nunca é carregado pelos arquivos; não reproduzido. |
| U-DROP-05 | Retorno de `FUN_00492870`/`FUN_0049c920` assumido "dropou". |
| U-DROP-06 | Demais ramos do `23` especial (12,7 invocação CS, 14,28 Kalima, lista `FUN_00493780`), CustomMonsterKillCount (`FUN_004b8f90`, pacote DS) e KillMessage do CustomMonster não feitos. |
| U-MON-01 | Pacote `C1 0C 1E` das classes 0x211..0x21B e 0x231 não reproduzido (lança slot 0). |
| U-MON-02 | Elementos 4/5/6/8/9 (empurrão `FUN_005114f0`)/15..21 e efeitos contínuos (`+0x9b0..0x9c4`) não feitos; ScopeType 1 aproximado por distância. |
| U-MON-03 | Remoção por tempo (600 s) de Attribute 7/8/9 em `FUN_004c3b10` não feita. |
| U-MON-04 | Efeitos (stun, imunidade) dependem de `addEffectHook`/`removeEffectHook`, a ligar pelo módulo de buffs; ataques mágicos de monstro usam `monsterSkillAttackHook` se o módulo de skills o definir. |
| U-SUM-01 | Chave de configuração do summon da skill 432 (`CServerInfo+0x17cc`). |
