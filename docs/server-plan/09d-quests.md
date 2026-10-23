# 09d — Quests (observado no GameServer)

Implementação: `apps/server/internal/game/quest.go` (testes em `quest_test.go`), dados em `apps/server/data/Data/Quest/{Quest,QuestObjective,QuestReward}.txt` (`internal/gamedata/quests.go`).

## Estado

- Cada quest com índice < 200 tem 2 bits na coluna `Quest` (50 bytes, `obj+0x58e`): quest `i` fica no byte `i/4`, bits `(i%4)*2`.
- A coluna começa como `0xFF`, então toda quest começa no estado **3** (nunca iniciada). 0 e 1 = em andamento, 2 = concluída.
- `Quest.txt` tem uma linha por (quest, estado). A linha que vale é a do estado atual (`FUN_004dd6a0`): quest pré-requisito em `RequireState` (`*` = sem pré-requisito), `RequireMinLevel ≤ nível ≤ RequireMaxLevel`, coluna da classe ≠ 0 e ≤ `ChangeUp + 1`.
- A mesma checagem de classe vale para objetivos (`FUN_004de780`) e recompensas (`FUN_004df390`), que também exigem a quest `RequireIndex` no `RequireState`.

## Pacotes

| Pacote | Função | Layout |
|---|---|---|
| `C1 36 A0` | `0x4dde70` | `[3]` contagem = maior índice < 200 do Quest.txt + 1, `[4..53]` os 50 bytes de estado. Enviado **uma vez** por entrada (flag `obj+0x5c0`, zerada em `0x50ae50`): depois de `F3 10`/`F3 11` na entrada no mundo, no pedido `A0` e antes de `A1`/`A3`. |
| `C1 05 A1` | `0x4ddfa0` | `[3]` quest, `[4]` byte de estado inteiro. Abre a janela de quest. |
| `C1 06 A2` | `0x4de060` | `[3]` quest, `[4]` resultado (0 ok, `0xFF` recusado), `[5]` byte de estado. |
| `C1 0C A3` | `0x4de110` | `[3..4]` índice do jogador BE, `[5]` índice da recompensa, `[6]` valor, `[7]` 0, `[8..11]` LevelUpPoint u32 LE. Para o jogador e o viewport. |

Cliente → servidor: `A0` (pedir lista, `0x4ddb20`) e `A2 [3]quest` (avançar, `0x4ddb70`), ambos C3 pelo HackPacketCheck. A2 **não** checa NPC nem distância.

## Conversa com o NPC (`0x4dd850`, chamado por `0x4c1490` antes do NpcTalk.txt)

Percorre o Quest.txt em ordem, uma vez por índice cujo `MonsterClass` é o do NPC:

- estado 2 → lembra como "última concluída";
- senão, se alguma linha desse índice passa em `0x4dd6a0` → escolhe essa quest e para.

Sem quest disponível, usa a última concluída; sem nenhuma, a conversa segue normal. Ao abrir: `obj+0x3d4 = (x & 0xffff0281) | 0x281` (interface tipo 10) e envia `A1`.

## Avanço (`A2`)

1. Linha da quest para o estado atual (`0x4dd5c0`); sem linha, nada.
2. Objetivos completos (`0x4de830`): para cada objetivo ativo da quest, contagem ≥ `Quantity`. Tipo 1 = itens `Index`/`Level` no inventário (`0x4948f0`, pilhas contam a durabilidade); tipo 2 = zen. Falha → `A2 quest FF byte`.
3. Consome os objetivos (`0x4de880`): itens apagados slot a slot (`C1 05 28 slot 1`; pilha maior que o resto perde durabilidade, `C1 06 2A`); zen descontado (`C3 10 22 FE`).
4. Recompensas (`0x4df440`, por tipo):
   - 1: `LevelUpPoint += Quantity` + `A3`;
   - 2 / `0x10`: evolução para ChangeUp 1 / 2 (se menor), recalcula o CharSet, `A3` com `(CU<<4) + (Class<<5) − ((CU<<4)>>5)` em byte;
   - 3: EXP = `CServerInfo[0x340 + AL·4] × Quantity` com MaxLevelUp `CServerInfo[0x3cc]` (ver U-QUEST-01); sem level up, `C3 18 9C FF FF …`;
   - 4: Elfa aprende a skill `0x4D`; `LevelUpPoint += PlusStatPoint × max(nível − PlusStatMinLevel, 0)` + `A3`;
   - 5: zen (limite 2.000.000.000);
   - 6: `Quantity` itens criados no inventário (serial pelo DataServer, `C1 0C F3 14`; sem espaço o item se perde);
   - 7: efeito (`0x478ae0`);
   - 8: só `A3` (ex.: "Gain Combo" do DK).
5. Transição: estado 3 ou 0 → 1; 1 → 2 (salva o personagem; na quest 3 envia o aviso "Quest de Marlon concluida! Combo do BK liberado."); 2 → 2 com resultado `0xFF`. Resposta `A2 quest result byte`.

## Drop de item de quest (`0x4de9b0`, dentro do drop do monstro `0x4b7290`)

Para o maior causador de dano: o primeiro objetivo tipo 1 ativo com `MapNumber` igual (ou `*`), nível do monstro entre `DropMinLevel` e `DropMaxLevel`, `rand % 10000 < ItemDropRate` e contagem < `Quantity` derruba o item (dono = esse jogador) e **substitui** o drop normal. Com `QuestMonsterItemDropParty = 1` e party, testa cada membro em ordem (`0x4deba0`), sem checar distância.

A contagem de mortes por quest (`0x4ded70` / `0x4dee40`) chama a checagem e descarta o resultado: no binário é um stub.

## Incógnitas

| ID | Questão |
|---|---|
| U-QUEST-01 | `CServerInfo` `0x340..0x34c` (taxa de EXP de quest por AccountLevel) e `0x3cc` (MaxLevelUp da quest) não têm chave de configuração; ficam 0. Nenhuma quest dos dados usa o tipo 3. |
| U-QUEST-02 | Pré-condições de A0/A2: `obj+10 < 1` e `obj+0x964 == 0` não identificados; usamos "sem logout pendente e sem troca de servidor". |
| U-QUEST-03 | Nas faixas de nível do drop o Ghidra mistura `+0x2c` com `obj+0xe0`; a regra `DropMinLevel ≤ nível ≤ DropMaxLevel` é a leitura mais provável. |
