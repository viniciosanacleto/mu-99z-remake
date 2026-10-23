# 07 — Mundo, objetos, mapas e monstros

## 1. Modelo de objetos (observado)

| Faixa de índice | Uso |
|---|---|
| `0 .. 0x5013` | monstros e NPCs: alocação round-robin, reaproveitando primeiro um índice liberado há mais de 10 s (U-WORLD-01 resolvida) |
| `0x5014 .. 0x53FB` | summons |
| `21500 .. 22499` (`0x53FC..0x57E3`) | jogadores (1000 slots) |

Campos do objeto citados em handlers (offsets do struct original, úteis para conferir comportamento; não precisam ser replicados):

| Offset | Significado (evidência) |
|---|---|
| `+0x00` | índice |
| `+0x04` | estado da conexão (1 conectado, 2 seleção de personagem, 3 jogando) |
| `+0x6c` | tipo (1 = jogador) |
| `+0x6e` | vivo (≠0) |
| `+0x70` | classe do monstro (`C2 13`) |
| `+0x1a8`, `+0x1aa` | X, Y atuais |
| `+0x1ac` | direção |
| `+0x1ad` | número do mapa |
| `+0x1ca/+0x1cc`, `+0x1ce/+0x1d0` | posição anterior / destino (TX, TY) |
| `+0x388` | lista de magias (registros de 20 bytes) |
| `+0x3b8`/`+0x3bc` | viewport (75 × 4 bytes: `state, index u16, type`) |
| `+0x396..+0x3a7` | CharSet (18 bytes de aparência) |
| `+0x93f` | possui guild (>0) |

Estado de "duelo/evento" para bloquear trade/party/duelo: flags em `+0x93c*4` e em três tabelas globais por índice (`0x104e8b4`, `0x7742b8`, `0xff4420`) — mensagens "Proibido dar trade/formar party/iniciar o duelo no evento." (custom WizardTeam).

Proposta Go:

```go
type ObjectKind uint8 // Player, Monster, NPC, EventObject
type Object struct {
    Index    uint16
    Kind     ObjectKind
    Map      uint8
    X, Y     uint8
    TX, TY   uint8
    Dir      uint8
    Alive    bool
    Viewport *Viewport // 75 entradas, ver §4
}
```

## 2. Mapas e servidores (observado)

`Data/MapManager.txt` — uma linha por mapa:

`MapNumber NonPK ViewRange ExperienceRate ItemDropRate MoneyDropRate ExcItemDropRate SetItemDropRate ItemDrop PKItemDrop MonsterHPBar TradeEnable PShopEnable ChatEnable PartyEnable DuelEnable GuildWarEnable SpawnGate RespawnGate Name`

Mapas configurados: 0 Lorencia, 1 Dungeon, 2 Devias, 3 Noria, 4 Lost Tower, 6 Arena, 7 Atlans, 8 Tarkan, 9 Devil Square 1, 10 Icarus, 11–17 Blood Castle 1–7, 18–23 Chaos Castle 1–6, 24–29 Kalima 1–6, 30 Valley of Loren (Castle Siege), 31 Land of Trial, 32 Devil Square 2, 33 Aida, 34 Crywolf, 36 Lost Kalima, 40 Silent. `ViewRange` padrão 12 (10 em alguns mapas).

`Data/MapServerInfo.txt` — seção 0: `ServerCode ServerGroup InitSetVal IpAddress Port` (códigos 0 e 19); seção 1: `ServerCode MoveOption NextMap NextServerCode` — mapas 30, 31 e 34 do servidor 0 redirecionam para o servidor 19 (EventServer/Castle Siege). Isso dispara a troca de servidor via `auth` (ver [04 §5](04-auth-join-server.md)).

`Data/MapRateInfo.txt` — taxas por mapa (formato em [11](11-arquivos-de-configuracao.md)).

## 3. Atributos de terreno (observado)

`Data/Terrain/TerrainN.att` (N = mapa+1): **65539 bytes** = cabeçalho de 3 bytes (`00 FF FF` = versão, largura-1, altura-1) + 256×256 bytes de flags, **sem criptografia** no servidor (o cliente usa versões cifradas, ver plano do cliente).

Flags usadas no código (função `FUN_004ad4d0(flag)` testa a célula do objeto):

| Bit | Evidência | Significado provável |
|---|---|---|
| `0x01` | ataques/skills e `/attack` checam antes de agir | zona segura (sem combate) — **inferido** |
| `0x04`, `0x08` | `D3` cancela o caminho se a célula tem 4 ou 8 | bloqueio de movimento (parede/objeto) |
| `0x10` | checado no mapa 30 durante o caminho | portões do castelo — **inferido** |

## 4. Viewport

- 75 entradas por objeto; entrada = `{state u8, index u16, type u8}` (state 1 = visível; type 0/8/16 ignorados no broadcast de ação — **observado** em `0x4d8300`/`0x4d8b80`).
- Alcance = `ViewRange` do mapa (MapManager).
- Pacotes: `C2 12` (jogadores), `C2 13` (monstros/NPCs), `C2 20`/`C2 21` (itens no chão / remoção — **inferido** pelos builders), `C1 14` (remoção de objetos — **inferido**).
- A reescrita deve recalcular viewport por tick e emitir somente diferenças, como o original (bit 7 do índice marca "objeto novo").

## 5. Movimento

Regras observadas (ver [02 §5](02-protocolo-de-rede.md#5-movimento-ação-e-ataque-observado)):

- caminho de até 15 passos, codificado em nibbles; validação de distância ±15;
- intervalo mínimo de 100 ms entre pacotes de movimento;
- mapas 18–23 (Chaos Castle) têm cooldown de 1000 ms para `DF`;
- célula bloqueada (`0x04|0x08`) cancela o caminho e reenvia a posição real;
- broadcast `C1 08 D3`.

`Data/Hack/HackSkillSpeedCheck.txt`, `HackPacketCheck.txt` e o módulo `CHackMoveSpeedCheck` ("Move count error (Map: %d, Count: [%d][%d])") limitam a taxa por mapa.

## 6. Gates, movimentação e teleporte (observado)

- `Data/Move/Gate.txt`: `GateIndex GateFlag MapNumber RangeStartX RangeStartY RangeEndX RangeEndY TargetGate TargetDir MinLevel MaxLevel MinReset MaxReset` (`GateFlag` 1 = entrada, 2 = destino; `*` = sem limite).
- `Data/Move/Move.txt` (comando `/move` e janela M): `Index Name RequireMoney MinLevel MaxLevel MinReset MaxReset AL0 AL1 AL2 AL3 GateNumber`.
- `Data/Move/MoveSummon.txt`: destinos de pergaminhos de invocação.
- Pacote de teleporte `1C` (classe `CMove`) responde `C3 08 1C`.

## 7. Monstros (observado)

`Data/Monster/MonsterList.txt` (304 linhas):

`Index Type Name Level MaxLife MaxMana DamageMin DamageMax Defense MagicDefense AttackRate DefenseRate MoveRange AttackType AttackRange ViewRange MoveSpeed AttackSpeed RegenTime Attribute ItemRate MoneyRate MaxItemLevel MonsterSkill Resistance1..7`

Exemplo: `0 0 "Bull Fighter" 6 100 0 16 20 6 0 28 6 3 0 1 6 450 1600 5 2 130 20 6 0 0 0 0 0 0 0 0`.

`Data/Monster/Spawn/NNN - <Mapa>.txt` — uma seção por tipo de spawn, terminada por `end`:

| Seção | Colunas | Uso |
|---|---|---|
| `0` | `MonsterIndex SpawnRadius LocationX LocationY Direction` | NPCs fixos |
| `1` | `Monster Range BeginPosX BeginPosY EndPosX EndPosY Direction Quantity` | spawns em área (múltiplos) |
| `2` | igual à `0` | spawns pontuais |

Outros arquivos: `MonsterMove.txt` (NPCs errantes: por classe, a cada `KeepTime` s o NPC some e reaparece numa linha sorteada, `FUN_004b1450`, `monstermove.go`), `MonsterSchedule.txt` (apesar do nome não cria monstros: grupos abrem NPCs por `EnableNpcTime` s nos horários da seção 0, com avisos da seção 1; fora do horário os NPCs da seção 2 respondem `ItsNotTimeMessage`, `FUN_004bb560`/`FUN_004bb8d0`, `monsterschedule.go`), `NpcTalk.txt`, `Skill/MonsterSkill*.txt` (skills de monstro: unit/element), `Custom/CustomMonster.txt`, `CustomRespawn.txt`, `CustomMobCombo.txt`, `CustomMonsterKillCount.txt`.

IA, aggro e regeneração não foram decompilados em detalhe (classe `CMonster`, `CMonsterManager`, `CMonsterMove`, `CMonsterSkillManager`) — ver U-WORLD-02.

## 8. Instâncias e eventos de mapa

Blood Castle (11–17), Chaos Castle (18–23), Devil Square (9, 32), Kalima (24–29, 36), Castle Siege (30), Land of Trial (31), Crywolf (34) e Illusion Temple são instâncias por horário. Configuração em `Data/Event/*.dat` e `GameServerInfo - Event.dat` (ver [10](10-sistemas-de-jogo.md)). Na reescrita, cada mapa de evento deve ser uma instância isolada dentro da goroutine `world`, com seu próprio relógio de fases.
