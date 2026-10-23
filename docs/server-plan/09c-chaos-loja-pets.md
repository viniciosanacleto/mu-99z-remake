# 09c — Chaos Machine, loja pessoal e pets (achados do REA)

Complementa o [09](09-itens-e-inventario.md). Tudo observado no `GameServer.exe` (endereços citados) e no `DataServer.exe` (`.re/ds.c`); implementado em `apps/server/internal/game/{chaos,pshop,pet}.go`, `internal/store/pet*.go` (migração `0006_pets.sql`). As tabelas de opção de item e os item bags são do agente de drops (`internal/gamedata/drops.go`, `sets.go`); a Chaos Machine só os consome.

## 1. Chaos Machine

### 1.1 Abertura, fechamento e transação de inventário

- **NPC 0xEE (Chaos Goblin)** — `FUN_004c1490` só despacha se nenhuma interface estiver em uso (`obj+0x3d4 & 3 == 0`) e a loja pessoal estiver fechada (`obj+0x2468 == 0`). `FUN_004c2390`: se `obj+0x3dc != 1` (sem transação aberta) → interface tipo 7 (`obj+0x3d4 = ...|0x1c1`), envia **`C3 0B 30 03`** + 7 bytes (lixo de pilha no original; enviamos zeros), **`C1 06 B2 1A 01 taxa`** (`FUN_004dae50`; taxa do castelo = 0 se não houver dono), zera `obj+0x5d7` e abre a transação (`FUN_0050d380`).
- **Transação de inventário** (`obj+0x3dc`): `FUN_0050d380` copia o inventário real (`obj+0x3e4`) para o de trabalho (`obj+0x3e8`) e as magias, estado 1; `FUN_0050d620` (commit) copia de volta, estado 2; `FUN_0050d750` (rollback, só usado pelo trade) restaura, estado 3. O save do personagem (`GJSetCharacterInfo`) grava o inventário real ⇒ enquanto o baú está aberto o inventário salvo é o de antes. No Go: `chaosState.backup` + `characterSaveHooks` (enterworld.go) troca o inventário gravado enquanto `Trans == 1`.
- **`C1 03 87`** (`FUN_0042fcb0`): exige interface 7; zera a interface, **limpa o baú** (`FUN_0042a910`, itens que estiverem nele somem), commit, zera `obj+0x430/0x5d7/0x985` e responde `C1 03 87`.
- **31 (fechar NPC)** só fecha o tipo 7 se `obj+0x984 != 0` (nunca ligado aqui) — o cliente usa o 87. Tipo 13 (`0x340`): limpa + commit.
- **Logout** (`FUN_004c4b40`): baú aberto (tipo 7/13) e `obj+0x5d7` (houve mix) ⇒ commit e devolve os itens do baú ao inventário (`FUN_0042ad20`); sem mix o inventário salvo continua o de antes (implementado como rollback).

### 1.2 Movimentos (24, `FUN_00499410`)

Flags 3 e 6..20 exigem interface 7 e `obj+0x430 == 0` (nenhum mix em curso); flag 5 exige interface 13. O roteamento trata 3 e 5..20 do mesmo jeito:

| De → para | Função | Regras | Resultado |
|---|---|---|---|
| inventário → baú | `FUN_00497fc0` | origem < 76, destino < 32; item válido sem `CItem+0xb7`; flag 16 só aceita "lucky item" (`CLuckyItem` vazio ⇒ recusa sempre); `Lock` da conta ⇒ mensagem 0x6F2; encaixe 8×4 (`FUN_004965b0`/`FUN_00496540`) | flag destino |
| baú → inventário | `FUN_004979a0` | `canEquip` + `placeInventory` | 0 |
| baú → baú | `FUN_00498090` | origens distintas, encaixe ignorando a própria origem | flag destino |

Resposta `C3 0C 24 resultado slot item[7]` com o item lido do baú quando o destino é baú.

### 1.3 Mix — `86 [3] tipo` (`FUN_0042ebd0`)

Pré-condições: jogando, sem contagem de logout, `obj+0x964 == 0`, `obj+0x430 == 0`, loja fechada, `obj+0x20ad == 0`. Liga `obj+0x430` (trava), zera `obj+0x428` (zen) e `obj+0x42c` (taxa), liga `obj+0x5d7`. **Não verifica a interface aberta.** Resultado `C1 0B 86 r item[7]` (`FUN_0042ff70`, item `FF×7` sem item; libera a trava): 0 falha, 1 sucesso, 2 zen insuficiente, 7 itens errados, 8 nível errado, 0xF0 charms demais. Lista do baú `C2 tam 31 tipo n {slot,item[7]}` (`FUN_0042fd60`; tipo 3 para a interface 7, 5 para a 13; só enviada se a interface certa estiver aberta e com estado ≠ 1).

Fluxo comum: taxa → **Charm of Luck** (14,53; `FUN_0042ae00`: soma a durabilidade de cada charm à taxa; total > 10 ⇒ 0xF0) → `CBonusManager` tipo 2 (`FUN_004188f0`; `BonusManager.dat` vazio ⇒ identidade) → teto → zen → imposto (`zen*taxa/100`, 0 sem dono do castelo) → zen > dinheiro (u32) ⇒ 2 → desconta e envia `C3 10 22 FE` → sorteio `rand()%100 < taxa` (`FUN_005138d0`/`FUN_00513720`). Falha comum: limpa o baú, envia a lista e 86 00.

Itens criados vão ao DataServer (`FUN_004753e0` com mapa 0xFF): a resposta (`FUN_004742b0`) só é aplicada se a interface 7 ainda estiver aberta; limpa o baú, põe o item no slot 0 e envia 86 01 com o item. Durabilidade: a passada pelo mix para 13,18 / 13,37 / 13,51 / 14,7 / 14,19 / 14,109, senão `FUN_00494500` (máxima). Dark Horse/Raven recebem nível 1 / exp 0 (`FUN_0048d010`).

| Tipo | Função | Receita | Taxa (`GameServerInfo - ChaosMix.dat`) | Zen | Resultado |
|---|---|---|---|---|---|
| 01 | `0x42ae60` | ≥1 Chaos + ≥1 item nível > 3 com JoL; Bless/Soul/Chaos somam valor | `ChaosItemMixRate_AL` (-1 ⇒ valor/20000) teto 100 | taxa×10000 | bag "Chaos Item Mix" (SpecialValue 0, nível 0) |
| 02 | `0x42b2a0` | 1 Chaos, 1 Devil's Eye (14,17) e 1 Key (14,18) de mesmo nível ≤ 7 (nível > 7 ⇒ 8) | `DevilSquareMixRate1..6` (nível 7 lê offset sem chave ⇒ 0) | 100k/200k/400k/700k/1,1M/1,6M/2M | 14,19 nível N, dur 1 |
| 03/04/16/17/31/32 | `0x42b790` (p = 0..5) | 1 Chaos, ≥ p+1 Bless e Soul, exatamente 1 item +(9+p) (exceto 14,96 / 14,190) | comum `PlusCommonItemLevelMixRate{p+1}`; excelente/set `PlusExcSetItemLevelMixRate{p+1}`; +`AddLuckSuccessRate2_AL` (Common.dat) com luck; p 4/5 (+14/+15) leem offsets sem chave ⇒ 0 | (p+1)×2M | mesmo item +1, `dur = dur × máx(novo)/máx(antigo)`, slot 0; falha por `FailMixMode_AL`: 0 limpa tudo, 1 mantém o item com nível 0 (dur reescalada), 2 mantém o item intacto |
| 05 | `0x42bde0` | 1 Chaos + 10 Uniria (13,2) com durabilidade 255 | `DinorantMixRate` | 500k | 13,3 com excelentes: quantidade pela linha 3 da seção 4 do ItemOptionRate, bits do ExcellentOptionRate (máx. 3) |
| 06 | `0x42c230` | 1 Chaos + 1 Creation | `FruitMixRate` | 3M | 13,15 nível `rand%5` |
| 07 / 18 | `0x42c620` | 1 Chaos, 1 Feather (13,14 +0) / Crest (+1), 1 asa 12,0..2 ou 12,41; itens exc ≥ +4 somam valor | `Wing2MixRate` (-1 ⇒ `valorExc/40000 + valorAsa/4M`, teto 90; senão teto 100) — capa usa a mesma chave | 5M | bag "Wing 2 Mix" / "Cape Mix" (0/2, 0/3); itens 13,88..92 forçam 12,5/12,4/12,3/12,42/12,6 (só asa) |
| 08 | `0x42cc50` | 1 Chaos, 1 Scroll of Archangel (13,16) e 1 Blood Bone (13,17) do mesmo nível ≤ 8 | `BloodCastleMixRate1..7` (nível 8 sem chave ⇒ 0). **Falha só se `taxa < rand%100`** | 50k/80k/150k/250k/400k/600k/850k/1,05M | 13,18 nível N, dur 1 |
| 0B | `0x42d170` | ≥1 Chaos + ≥1 arma chaos (2,6 / 4,6 / 5,7) +4 com JoL; jóias e itens +4 com JoL somam valor | `Wing1MixRate` (-1 ⇒ valor/20000) teto 100 | taxa×10000 | bag "Wing 1 Mix" (0/1); itens 13,83..86 forçam 12,2/12,1/12,0/12,41 |
| 0D / 0E | `0x42d680` (t 0/1) | 1 Chaos, 1 Spirit 13,31 +t, 5−3t Bless e Soul, 1 Creation | `PetMixRate` | 5M − t×4M | 13,4 / 13,5; a lista da falha vai só à interface 13 |
| 0F / 10 | `0x42d9a0` | 1..25 Bless (0F) ou Soul (10), sem Chaos, sem sorteio | — | (2−t)×50k | 14,7 nível t, dur (bless+soul)×10 |
| 11 | `0x42dcf0` | 1 Chaos, 5 Bless, 5 Soul, 1 Guardian (14,31), sem sorteio | — | 5M | 13,11 nível 1 |
| 12 | `0x42dff0` | só o mestre da guild dona do castelo, 1×/dia, 3 asas 12,30/31 + 30 Guardian, 1 bilhão | — | — | sem dono de castelo: aviso 0x2E7 + 86 00 |
| 28 | `0x42e4c0` | 1 Chaos Card (14,54 / 92 / 93 / 95; vale o último tipo visto) | — | — | bag SpecialValue **7**, nível tipo−1 (não existe no `EventItemBagManager.txt` ⇒ 86 00) |
| 29 | `0x42e900` | Cherry Blossom 14,88/89/90 com soma das pilhas = 10 / 30 / 255 | — | — | bag SpecialValue **9** (inexistente ⇒ 86 00) |

Tipos sem caso deixam a trava ligada (como no original).

**Anúncio do +13** (`FUN_005120b0`, `chaos_announce.go`): chamado pelo `0x42b790` depois do resultado de sucesso e antes do log, com o tipo do mix na pilha, o jogador em ECX e, em EAX, `movswl -0xf4(%ebp)` = CItem+4 da cópia já promovida, ou seja, o **índice** do item (não o nível). Só o tipo 3 (86 17, item +12 → +13) anuncia; +10..+12 e +14/+15 nunca. Com `ChaosMixPlusItemAnnounce` (CServerInfo+0x178) ≠ 0 e `índice ≥ AnnounceMinLevelChaosMix` (CServerInfo+0x17c, padrão 0xd, arquivo 11) — bug do original: o limite de "nível" é comparado com o índice, então só os itens 0,0..0,10 ficam de fora —, o texto é a mensagem 500 (`"%s colocou o item %s +%d"`) com o nome do personagem, `FUN_00494a40(índice, 13)` (nome do ItemLevel para o nível 13, senão o do Item.txt) e a constante 13:
- modo 1: aviso dourado (tipo 0) a todos os jogadores do servidor, cada um no seu idioma (`FUN_004c0c70`);
- modo 2: aviso dourado ao grupo de servidores pelo DataServer (`FUN_00475b50`, `C1 94 21` com o grupo da entrada deste servidor no MapServerInfo), no idioma do servidor (CServerInfo+0x70, nunca gravado ⇒ 0);
- outros modos não enviam nada. A instalação vem com o anúncio desligado.

### 1.4 Prévia — `88 [4..7] tipo u32` (`FUN_0042ee40`)

Exige interface 7 ou 13. Recalcula a receita como o mix (sem sortear): receita ausente ⇒ **sem resposta**; tipos sem ramo (inclusive 31/32) ⇒ zeros. Resposta **`C1 10 88 00 taxa(u32) zen(u32) imposto(u32)`** com taxa limitada a 0..100 e imposto `zen*taxa_castelo/100`. A prévia do +10..+13 não exclui 14,96/14,190 e não tem ramo para +14/+15.

### 1.5 Item bags (`FUN_004912f0` + `FUN_0048f980`)

`CItemBagManager` acha a primeira linha do `EventItemBagManager.txt` com `SpecialValue` = valor, `SpecialLevel` = -1 ou nível e `SpecialState` = -1 (registradores: EAX = valor, EDI = nível, confirmado no 0x42e4c0). Geração (formato novo, `+0x54 != 0`):

1. seção 3 em ordem de BagIndex: a primeira com `rand%10000 < DropRate` é usada (nenhuma ⇒ falha, resultado 0 **depois** do zen descontado);
2. linhas da seção 4 desse BagIndex permitidas à classe (`valor != 0 && valor <= ChangeUp+1`) com SectionRate ≠ 0, sorteio ponderado `rand % soma`;
3. item da seção com `rand % quantidade`;
4. Option0..5 são linhas do `ItemOptionRate.txt` (nível, skill, luck, JoL, nº de excelentes, set); linha inexistente mantém o nível do item e 0 no resto; bits excelentes por `FUN_0049f0d0` modo 1 (máx. 6; 3 para 13,3/13,37), set por `FUN_0049f270` (1/2: sorteio entre tiers com opção → 5 ou 9 / 6 ou 10; 3/4: tier 1; 5/6: tier 2); sem excelente o GradeCount vira o "new option" do `Convert`.

### 1.6 Log

`FUN_00401450(7, ...)` + `FUN_004a3d00` escrevem em `.\LOG\CHAOS_MIX_LOG` (índice 7) quando `WriteChaosMixLog` (Common, 0x114) ≠ 0; mesmos textos (`[ChaosItemMix][Success][%s][%s] - (ChaosSuccessRate: %d,ChaosMoney: %d)` etc.) via slog.

## 2. Loja pessoal (3F)

Estado em `obj+0x2468..0x2498`: aberta (`0x2468`), venda em curso (`0x2469`), mudou (`0x2470`, zerado a cada ciclo de 1 s), reiniciar lista conhecida (`0x2471`, ligado no gate move `FUN_004bec30`), título de 36 bytes (`0x2472`), último pedido de compra (`0x246c`), olhando (`0x2496`/`0x2498`), lista de lojas já anunciadas (`obj+0x454`, contagem `obj+0x580`).

**Preço:** `CItem+0x84`, definido por `3F 01` em **qualquer** slot do inventário (< 108) e copiado junto com o item; `FUN_00496830` não o grava nos 16 bytes do banco ⇒ **não é persistido** (some no relog, como no original).

| Pacote | Função | Layout / regras |
|---|---|---|
| `C3 3F 01 slot preço(u32)` → `C1 06 3F 01 r slot` | `0x4d4ab0` / `0x4d5990` | 2 slot ≥ 108, 3 sem item, 4 preço ≤ 0 (com sinal), 5 nível ≤ 5, 1 gravado |
| `C3 3F 02 título[36]` → `C1 05 3F 02 r` | `0x4d4be0` / `0x4d5a30` | interface em uso (≠ 8) ignora; mapa sem loja (`MapManager.PShopEnable`, `*` ⇒ `PersonalShopSwitch`) ⇒ aviso 0x326; sem itens/preço ⇒ 0; nível < 6 ⇒ 2; ok ⇒ 1. Já aberta: só troca o título e envia `C1 34 3F 10 idx título[36] nome[10]` ao VP2 (`0x4d6040`). Títulos `/...` do CustomStore WizardTeam não reproduzidos |
| `C3 3F 03` → `C1 07 3F 03 01 idx` | `0x4d4f00` / `0x4d5ad0` | para si e VP2 |
| `C3 3F 05 idx nome[10]` → `C2 tam 3F 05 r idx nome[10] título[36] n {slot,item[7],preço u32}` | `0x4d4fc0` / `0x4d5bb0` | loja fechada ⇒ r 3 e idx FFFF; nome errado ⇒ 2; ok ⇒ 1 e passa a "olhar" a loja |
| `C3 3F 06 idx nome[10] slot` → `C1 0F 3F 06 r idx item[7] slot` | `0x4d5140` / `0x4d5e40` | 2 s entre compras; 3 fechada, 4 venda em curso, 5 slot fora de 76..107, 6 transação aberta (trade/baú) / nome / item / preço < 1, 7 zen do comprador (u32), 8 zen do vendedor estoura 2e9 ou sem espaço; o item enviado é o do inventário do comprador no slot do resultado (**slot 0 nas falhas**) |
| (venda) | `0x4d5140` | comprador: −preço, `C3 10 22 FE`, 3F 06 r 1, save; vendedor: +preço, `C3 10 22 FE`, `C1 0F 3F 08 slot nomeComprador[10]` (`0x4d5f70`), `C1 05 28 slot 01`, save; loja vazia ⇒ fecha com 3F 03 |
| `C3 3F 07` | `0x4d55b0` | deixa de olhar |
| `C2 tam 3F 00 n {idx, título[36]}` | `0x4d5600` | a cada 1 s (`FUN_0050a020`): lojas abertas de jogadores no VP (estado 2) ainda não anunciadas (todas se a lista estiver vazia) |
| `C2 3F 13 ...` / `C1 06 3F 12 FF FF` | `0x4d5600` | quem olha recebe a lista nova se a loja mudou, ou 3F 12 se a loja fechou/o dono saiu |

Movimentos (24 flag 4; recusados com a loja aberta ou venda em curso): inventário→loja `FUN_00498160` (origem qualquer < 108, `FUN_00494ee0`: item válido, `ItemMove.AllowTrade`, TradeItemBlock/Exc/Set (`FUN_00495140`, GM 8 isento), `Lock` ⇒ 0x6F2; coloca a cópia antes de apagar a origem; resultado **0**), loja→inventário `FUN_00497a70` (Lock ⇒ 0x6F2, `canEquip`), loja→loja `FUN_00498200`.

Log de venda: `[SellPesonalShopItem]...` no TRADE_LOG (índice 3) com `WriteTradeLog`.

## 3. Pets — Dark Horse (13,4) e Dark Raven (13,5)

- **Tabela de exp** (`FUN_0050ad00`, `DAT_00770de8`): `PetExp[L] = (L+10)·L³·PetExperienceMultiplierConstA` para L ≥ 3, 0 abaixo (int32).
- **Iniciar** (`FUN_0048d010`): `+0xc0 = 1`, nível limitado a `MaxPetLevel` (50), exp limitada a `[PetExp[L], PetExp[L+1])` (acima volta ao piso); depois `Convert` e `FUN_0048cf60`.
- **Preço** (`FUN_0048cf60`): compra = nível × 2.000.000 (cavalo) / 1.000.000 (corvo), venda = valor/3, ambos com o arredondamento >99 / >999.
- **Ganho** (`FUN_0048db00`): durabilidade 0 ou não-pet ⇒ nada; nível < 1 ⇒ vira 1/0 (conta como level up); `ganho = exp / PetExperienceRateDivisor`; nível ≥ MaxPetLevel ⇒ nada; `exp+ganho ≥ PetExp[L+1]` ⇒ L+1 e exp = `PetExp[L+1]` (o excedente se perde).
- **Perda** (`FUN_0048dc80`): abaixo de `PetExp[L]` ⇒ L−1 e exp = `PetExp[L]−1`.
- **Exp de kill** (`FUN_004c69d0`, chamado por `FUN_004c5a30` e `FUN_004c5ea0` antes do level up do jogador, com a exp do jogador): cavalo no slot 8 e corvo no slot 1. Sem level up ⇒ `A9` com InvenType 0. Com level up: se o dono ainda pode equipar (`FUN_00496d00`) ⇒ `A9` com **InvenType 0xFE**; senão desfaz com `FUN_0048dc80` e envia `A9` InvenType 0 se o nível caiu.
- **`C1 0C A9 petType invenType pos nível 00 exp(u32)`** (`FUN_004dacf0`; petType 0 corvo, 1 cavalo; pos 8 cavalo, 1 corvo).
- **Pedido A9** (`FUN_004d8730`): `[3]` petType 0/1, `[4]` local (0 inventário < 108, 1 baú < 120 carregado, 2 trade próprio < 32, 3 trade do parceiro, 4 baú do chaos < 32, 5 loja olhada 76..107), `[5]` pos; responde nível/exp do item (qualquer item; não-pet dá 0).
- **A7** (`FUN_004d86c0` → `FUN_0046b450`): `[3]` deve ser 0 (corvo), `[4]` modo 0 normal / 1 ataque aleatório / 2 ataca junto / 3 alvo `[5..6]`; só DL (classe 4) com o corvo ativo (CDarkSpirit+0x14 válido, ver abaixo) e só se o modo mudar (o 3, `FUN_0046b2d0`, grava o alvo e sempre responde). Resposta **`C1 07 A7 00 modo alvoHi alvoLo`**; nos modos 0..2 os bytes de alvo ficam sem inicializar no original (enviamos 0). Os resets mandam `C1 07 A7 00 00 FF FF`.
- **Atributos do corvo** (`FUN_0046b0c0`, chaves `DarkSpirit*` do Character.dat): dano mín `Cmd/C + B·nível + A`, máx idem, velocidade `B·nível/C + A + Cmd/D`, acerto `A + B·nível/C` (`Cmd = obj+0x10c + obj+0x184`).

### IA do Dark Raven (CDarkSpirit, `pet_raven.go`)

Estrutura em `DAT_007a1628 + índice·0x28`: `+4` dano mín, `+8` máx, `+0xc` velocidade, `+0x10` acerto, `+0x14` dono, `+0x18` alvo, `+0x1c` próximo ataque (tick), `+0x20` modo, `+0x24` CItem do corvo.

- **Ativação** (`FUN_0046b0c0`, chamada no fim de todo CalcCharacter, `FUN_004c9790`, com EBX = item do slot 1): só para DL; zera dono/alvo/atributos/tick; com 13,5 válido (`CItem+0xb5`) grava dono e item e calcula os atributos; se o corvo estava inativo e o modo ≠ 0 ⇒ modo 0 e `A7 00 00 FF FF`. O gate move (`FUN_0050f6d0`) e a morte do dono (`FUN_004c3b10`) também voltam ao modo 0 com esse pacote.
- **Passo** (`FUN_0046aae0`, a cada 100 ms no `FUN_004c48b0` para jogadores): dono jogando, sem logout nem troca de servidor, DL, corvo com durabilidade, fora de safe zone e tick vencido. Com o `CHelperOffline` (obj+0x2110) o modo vem de obj+0x224c/0x20d8/0x2288/0x228c (U-PET-04). Próximo ataque = agora + `(150 − velocidade)·10` ms. Modo 1 ⇒ `FUN_0046ac40`; 2 e 3 ⇒ `FUN_0046ad80`; 0 nada.
- **Aleatório** (`FUN_0046ac40`): sorteia (`rand() % n`) entre os objetos do VP2 do dono aceitos por `FUN_004f8440` (as regras de alvo de área) a menos de 5 células.
- **Alvo** (`FUN_0046ad80`): o alvo vem de A7 modo 3 ou de todo ataque do dono (`FUN_0046b370`, chamado no CAttack `FUN_0040b110` e no próprio golpe do corvo). Alvo morto, fora do estado 2 ou em outro mapa ⇒ `FUN_0046b3b0` (esquece; no modo 3 volta ao 0 com `A7 00 00 FF FF`). A menos de 7 células: `rand()%100 < DarkSpiritRangeAttackRate` (30) ⇒ ataque em área `FUN_0046ae30`, senão golpe simples.
- **Golpe** (`FUN_0046afa0(alvo, envia, área)`): **`C1 09 A8 00 área donoHi donoLo alvoHi alvoLo`** ao dono e ao VP2 dele, depois mensagem 0x33 na fila do dono (`FUN_00510690`, 600 ms, param1 = envia, param2 = área).
- **Área** (`FUN_0046ae30`): golpe com área = 1 no alvo e mensagem 0x33 (envia 0, área 1) para até 10 outros objetos do VP2 aceitos por `FUN_004f8440` a menos de 4 células **do alvo**.
- **Dano** (`FUN_0046b5a0`, a mensagem 0x33 em `FUN_004c4280`): as mesmas recusas do início do CAttack (mapa, safe zone, objetos do castelo, guerra de guild/battle soccer, NPC, vivo, estado 2, teleporte, classes 100..110/0x20b/0xdd/0xde, Attribute 6, a própria invocação, `FUN_0040dca0`), sem os testes de eventos WizardTeam nem de velocidade; grava o alvo do corvo e assiste a invocação (`FUN_004b58f0`); duelo; imunidades 34/59 e 63 de monstro ⇒ erro (D7 vazio); a bola (classe 200) é empurrada 2 células (`FUN_005114f0`) e erra; obj+0x9d8 > 0 com obj+0x9d4 = 0 ⇒ erro.
  - acerto `FUN_0046c660`: taxa do corvo vs obj+0x370 do alvo; abaixo ⇒ `rand()%100 < 5` e o dano cai a 30%; senão `rand()%taxa ≥ defesa`;
  - defesa `FUN_0046c710`: `(Defense + Greater Defense)·50/100` para qualquer alvo (sem IgnoreDefense);
  - dano `FUN_0046c7b0`: mín/máx do corvo + `(MagicPower/2 + nível·2)·fator de durabilidade` % de um cetro válido (2,x) na mão direita; `mín + rand()%(máx−mín)`; `rand()%100 < DarkSpiritCriticalDamageRate` ⇒ máx (efeito 3); `rand()%100 < DarkSpiritExcellentDamageRate` ⇒ máx·120% (efeito 2); menos a defesa;
  - área ⇒ ×60%; DamageReduction[6], Mana Shield, Defense; mínimo `(nível + obj+0x5e4)/10` (sem parte aleatória); durabilidade do corvo (`FUN_0046c440`); taxas PvM/PvP, duelo, arena, Chaos Castle e Castle Siege 1/2; contra portão/estátua (0x115/0x11b) Bless Potion (efeito 10) +20% e `CastleSiegeDamageRate3`;
  - SD, vida, mensagem 0 ao monstro, self defense (sem duelo, guildas rivais ou em guerra, lado do cerco, arena, Chaos Castle, NonPK), reflexão, recargas e stuck do alvo, `FUN_004cda80`.
- **Durabilidade** (`FUN_0046c440`): `Util/PetDamageTable.txt` (CPetDamageTable, `FUN_004d6360`) linha do índice 5 que casa nível/resets ⇒ dano·taxa/1000 (só na cópia local); `CItem+0x28 += dano·2/100`; ao passar de `PetDurabilityRate·800/100 · obj+0x91c/100` (obj+0x91c é sempre 100) perde 1 de durabilidade (mínimo 0), zera o contador, recalcula se o fator de durabilidade mudou (`FUN_0048d990`) e envia `C1 06 2A 01 dur 00` e `A9` (corvo, inventário, slot 1).
- **DataServer**: pedido `C2 09` (`FUN_004758f0`, ao selecionar o personagem para o inventário e ao abrir o baú para o vault) com `{pos, serial}` dos 13,4/13,5; o DS (`FUN_0040dfc0`) lê `T_PetItem_Info`, insere a linha ausente e responde nível 1 / exp 0; a resposta (`FUN_004747a0`) aplica por pos+serial e recalcula se pos < 12. Save `0x34` (`FUN_00476430`, junto com cada save do personagem e do baú) com `{serial, nível, exp}` dos pets com `+0xc0`; o DS faz UPDATE ou INSERT. Criação de item (DS 0x07, `FUN_0040d8c0`) insere a linha dos pets novos.
- **Persistência no Go**: `store.PetStore` (`LoadPetInfo`, `SavePetInfo`, `CreatePetInfo`), tabela `pet_item_info (item_serial, pet_level, pet_exp)`; conformidade memória/Postgres em `pet_test.go`. O nível alimenta `stats.go` (`convertItem` com `it.PetLevel`, **resolve U-STATS-03**) e os requisitos (`itemRequirements`: corvo exige `15·nível + 185` de comando, cavalo nível `(nível+109)·2`).

## 4. Incógnitas

| ID | Descrição |
|---|---|
| U-CHAOS-01 | Bytes `[4..10]` do `C3 0B 30 03` são lixo de pilha no original (enviamos zeros). |
| U-CHAOS-02 | `obj+0x984` / `obj+0x985` (fechamento do tipo 7 pelo 31, condição de logout) — nunca ligados aqui. |
| U-CHAOS-03 | Faixas exatas dos itens de escolha de asa (13,83..86 / 13,88..92): o pseudocódigo da asa 1 mostra `0x1a52 < s && 0x1a55 < s`; usamos 0x1a53..0x1a56 (os itens não existem no Item.txt desta instalação). |
| U-CHAOS-04 | ~~Anúncio `ChaosMixPlusItemAnnounce` (`FUN_005120b0`) não reproduzido.~~ **Resolvida** (seção 1.3, `chaos_announce.go`): só o +13, mensagem 500, limite comparado com o índice do item (bug mantido). |
| U-CHAOS-05 | Máximo de opções excelentes de `FUN_0049f0d0` por item (`FUN_00518680` / CustomWing ⇒ 4) — usamos 6 (3 para 13,3/13,37). O laço original não termina se faltarem bits candidatos; limitamos a 64 tentativas. |
| U-CHAOS-06 | Bags SpecialValue 7 (Chaos Card) e 9 (Cherry Blossom) e Senior Mix (4) dependem do dono do castelo / bags inexistentes. |
| U-PSHOP-01 | CustomStore WizardTeam (títulos `/...`, compra com jóias `FUN_004695d0`, `obj+0x249c/0x249e`) não reproduzido. |
| U-PSHOP-02 | `CItem+0xb2` testado em inventário→loja (desconhecido, tratado como 0). |
| U-PSHOP-03 | Refresh `C2 3F 13` para quem olha: índice/resultado vêm em registradores não confirmados (usamos dono e 1). |
| U-PET-01 | ~~IA de ataque do Dark Raven não reproduzida.~~ **Resolvida** (seção "IA do Dark Raven", `pet_raven.go`). |
| U-PET-04 | `CHelperOffline`: o corvo só age com obj+0x224c ≠ 0 e escolhe o modo por obj+0x20d8/0x2288/0x228c; nenhum código decompilado grava 0x224c/0x2288/0x228c (talvez um memcpy de configuração do helper). Com o helper offline o corvo fica parado. |
| U-PET-05 | No golpe do corvo contra portão/estátua o acúmulo em obj+0x990 (`≤ 100 ⇒ +dano`, senão 0) não tem leitor conhecido; não guardado. `FUN_0040cbd0`/`FUN_0040cee0`/`FUN_0040d220`/`FUN_0040db70` (asas, pets e durabilidade das armaduras) seguem não reproduzidos, como no CAttack. O contador `CItem+0x28` fica no estado do jogador (não acompanha o item se ele for movido). |
| U-PET-02 | Valor de EBX passado a `FUN_004c69d0` na exp de party (assumido = parte dividida pelo nº de membros). |
| U-PET-03 | Valores do INSERT do DS 0x07 para pets novos (usamos nível 1 / exp 0, como a resposta do 0x09). |
