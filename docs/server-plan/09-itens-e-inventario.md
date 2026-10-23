# 09 — Itens, inventário, drops, lojas e mix

## 1. Índice de item (observado)

`ItemIndex = categoria * 512 + índice` (`GET_ITEM(cat, idx)`; nos arquivos de dados é escrito `cc,iii`, ex. `14,013`). 16 categorias (0–15), índices 0–511 ⇒ 8192 itens possíveis (o `Item.bmd` do cliente tem exatamente 8192 registros). Itens válidos: `0 ≤ index < 0x2000`. O código trata `0x1A13`, `0x1C40`, `0x1CDF` como especiais (não serializa — `0x496830`).

| Categoria | Conteúdo (confirmado pelos primeiros nomes de cada seção do `Item.txt`: 0 Kris/Short Sword, 1 Small Axe, 2 Mace/Thunderbolt Scepter, 3 Light Spear, 4 Short Bow/Devil Crossbow, 5 Skull Staff, 6 Small Shield, 7–11 Bronze Helm/Armor/Pants/Gloves/Boots, 12 Wings of Elf, 13 Guardian Angel/Imp/Horn of Uniria, 14 Apple/Healing Potion, 15 Scroll of Poison) |
|---|---|
| 0 espadas · 1 machados · 2 maças/cetros · 3 lanças · 4 arcos/bestas (`04,007` Bolt, `04,015` Arrow) · 5 cajados | armas |
| 6 escudos · 7 elmos · 8 armaduras · 9 calças · 10 luvas · 11 botas | equipamentos |
| 12 asas, joias especiais (`12,015` Jewel of Chaos, `12,030` pacote de Bless) · 13 anéis/pets/acessórios · 14 consumíveis, joias (`14,013` Bless, `14,014` Soul, `14,016` Life, `14,022` Creation), poções · 15 pergaminhos | outros |

## 2. Formato binário de item

### 2.1 Banco de dados — 16 bytes (observado, `0x496830` grava / `0x4969c0` lê)

| Byte | Bits | Campo |
|---|---|---|
| 0 | 7..0 | `index & 0xFF` |
| 1 | 7 | skill |
| 1 | 6..3 | nível (+0..+15) |
| 1 | 2 | luck |
| 1 | 1..0 | opção adicional, bits 0–1 |
| 2 | — | durabilidade |
| 3–6 | — | serial (u32 **big-endian**: byte 3 = mais significativo) |
| 7 | 7 | `index` bit 8 (`0x100`) |
| 7 | 6 | opção adicional bit 2 (opção > 3) |
| 7 | 5..0 | opções excelentes (6 flags) |
| 8 | 3..0 | opção de set (ancient) |
| 8 | 6 | flag (`ItemOptionEx & 0x40`) |
| 9 | 7..4 | `index` bits 9–12 (categoria) |
| 9 | 3 | flag (`ItemOptionEx & 0x80`) |
| 9 | 2 | flag `+0xb6` (item periódico? — a confirmar) |
| 9 | 1 | flag `+0xb7` (a confirmar) |
| 10 | — | opção Jewel of Harmony |
| 11–15 | — | não gravados pelo GS (permanecem como vieram; slot vazio = `0xFF` × 16) |

Decodificação: `index = b0 | ((b7 & 0x80) << 1) | ((b9 & 0xF0) << 5)`; vazio se `b0 == 0xFF && (b7 & 0x80) && (b9 & 0xF0) == 0xF0`. Ao ler, a durabilidade pode ser limitada por `ItemStack.txt`; o item `0x1A46` tem durabilidade fixada em 2.

### 2.2 Pacote para o cliente — 7 bytes (observado, `0x496700`)

| Byte | Conteúdo |
|---|---|
| 0 | `index & 0xFF` |
| 1 | `skill<<7 \| nível<<3 \| luck<<2 \| (opção & 3)` |
| 2 | durabilidade |
| 3 | `(index & 0x100) >> 1` (bit 7) `\|` `0x40` se opção > 3 `\|` excelentes (6 bits) |
| 4 | opção de set; bit 7 ligado se `ItemOptionEx & 0x400000` e o item consta na tabela de `CCustomXShop` (`0x402b20`) — custom |
| 5 | `(index >> 5) & 0xF0` (categoria) `\|` bit 3 de `ItemOptionEx` |
| 6 | opção Jewel of Harmony |

### 2.3 Inventário e baú

- Inventário: 108 slots (DB 1728 bytes). 0–11 equipamento, 12–75 grade 8×8, 76–107 área estendida (U-ITEM-02 resolvida, ver §8). `CInventoryEquipment`. Enviado ao entrar no mundo como `C4 F3 10` (`0x49b3f0`).
- Baú: 120 slots (1920 bytes), dinheiro, senha (`pw`), múltiplos baús via `ExtWarehouse.Number` (`/bau`, `CommandWareNumber_AL*`).
- O GS salva o personagem inteiro (`C2 30` → DS, [05](05-persistencia-data-server.md)) e só o inventário (`C2 31`).
- Tamanho do item na grade: `Width`/`Height` de `Item.txt`.

### 2.4 CharSet (aparência, 18 bytes no viewport)

Montado por `FUN_004c6ba0` (no mundo) e `FUN_00472d00` (lista de personagens), com a mesma lógica (**resolve U-ITEM-03**; implementação em `apps/server/internal/game/charset`). Entradas: os 9 primeiros slots de equipamento. `i0`/`i1` (mãos) usam o índice completo (vazio = `0xFFFF`); `i2..i8` usam `índice & 0x1FF` (vazio = `0x1FF`).

| Byte | Conteúdo |
|---|---|
| 0 | `classe*32 + changeUp*16 - (changeUp>>1)` (byte); `|2` / `|3` para pose `0x80`/`0x81` (`obj+0x25c`); no viewport os 3 bits baixos recebem `obj+0x271` |
| 1, 2 | `i0 & 0xFF`, `i1 & 0xFF` |
| 3 | `i2<<4 \| i3&0xF` |
| 4 | `i4<<4 \| i5&0xF` |
| 5 | `i6<<4`, bits 3–2 asas, bits 1–0 pet |
| 6, 7, 8 | níveis visuais `(nível-1)/2` (3 bits por slot 0..6) como u24: byte 8 = bits 0–7, 7 = 8–15, 6 = 16–23 |
| 9 | bit 4 de i2,i3,i4,i5,i6 em 7,6,5,4,3; bits 2–0: tipo de asa de 2º nível |
| 10 | flags de excelente: `2 << shift[slot]` com `shift = {1,0,6,5,4,3,2}`; bit 0 = Dinorant (pet 3) |
| 11 | flags de set (ancient), mesmo shift; bit 0 = set completo (`obj+0x5d1`) |
| 12 | `(i0>>4) & 0xF0`; bit 0 = Dark Horse (pet 4) |
| 13 | `(i1>>4)&0xF0 \| (i2>>5)&0xF` |
| 14 | `(i3>>1)&0xF0 \| (i4>>5)&0xF` |
| 15 | `(i5>>1)&0xF0 \| (i6>>5)&0xF` |
| 16 | pet custom (`CustomPet.txt`, índice > 69): `(pet+60)*2` |
| 17 | asa custom (`CustomWing.txt`): `índice*2+2`; asas 36–40 sem custom: `(asa+30)*4` |

Asas (`i7`): vazio ⇒ byte5 `|0x0C`; 0–2 ⇒ `asa*4`; 3–6 ⇒ `0x0C` e byte9 `|asa-2`; 30 (Capa) ⇒ `0x0C` e byte9 `|5`; outras não custom ⇒ nenhum bit. Pet (`i8`): vazio ⇒ `|3`; 0–2 ⇒ `|pet`; 3 ⇒ `|3` + byte10 bit 0; 4 ⇒ `|3` + byte12 bit 0.

## 3. `Data/Item/Item.txt` (observado)

Uma seção por categoria (`0` … `15`, cada uma terminada por `end`). Colunas de armas:

`Type(índice) Slot Skill Width Height HaveSerial HaveOption DropItem Name Level DamageMin DamageMax AttackSpeed Durability MagicDurability MagicDamageRate ReqLevel ReqStrength ReqDexterity ReqEnergy ReqVitality ReqLeadership Attr DW DK FE MG DL`

Ex.: `0 0 0 1 2 1 1 1 "Kris" 6 6 11 50 20 0 0 0 40 40 0 0 0 1 1 1 1 1 1`.

Categorias de armadura e consumíveis têm colunas diferentes após `Name` (ex. elmo: `Level Defense MagicDefense? Durability … ReqStr ReqDex … Attr DW DK FE MG DL`; poção: `Level Value`). O parser deve ser por categoria, como no original (validação cruzada com o `Item.bmd` do cliente em [client-plan/02](../client-plan/02-formatos-de-assets.md)).

## 4. Opções e taxas (observado)

| Arquivo | Colunas |
|---|---|
| `ItemOption.txt` | `Index OptionIndex OptionValue ItemIndexMin ItemIndexMax SkillSwitch LuckSwitch Aditional Excellent` — mapeia flags do item para efeitos (`OptionIndex` 80–127, referência em "Tools\Reference Item Options.txt", não incluso) |
| `ExcellentOptionRate.txt` | `Index ItemMinIndex ItemMaxIndex Name DropRate` |
| `ItemOptionRate.txt` | seção 0: `Index Rate0..Rate15` (chance por nível); seção 1: `Index Rate0 Rate1` |
| `SetItemType.txt` | `ItemIndex StatType OptionIndex1 OptionIndex2 ItemDropRate` |
| `SetItemOption.txt` | `Index Name Option1 Value1 … Option12 Value12 FullOption1 FullValue1 …` |
| `WingOption.txt` | `ItemIndex DefenseConstA IncDamageConstA IncDamageConstB DecDamageConstA DecDamageConstB MaxNewOption` |
| `ItemLevel.txt` | `ItemIndex ItemLevel Name` |
| `ItemStack.txt` | seção 0: `ItemIndex ItemLevel MaxStack`; seção 1: `ItemIndex ItemCount NextItemIndex` (empacotar) |
| `ItemMove.txt` | `ItemIndex AllowDrop AllowSell AllowTrade AllowVault` |
| `ItemValue.txt` | `ItemIndex ItemLevel ItemGrade MoneyValue` (preço fixo) |
| `ItemLoot.txt` | `ItemIndex ItemLevel AllowLoot` |
| `ItemMoneyDrop.txt` | `MapNumber MonsterIndex MonsterMinLevel MonsterMaxLevel AccountLevel MoneyAmount MoneyDropRate` |
| `ItemDrop.txt` | drop específico; 16 colunas por linha (ex. `12,015 0 0 * * * * * * 0 * * 12 150 500 940`), **sem cabeçalho no arquivo** — nomes das colunas a extrair do parser nativo (U-ITEM-04) |

Classes de suporte (RTTI): `CItemManager`, `CItem`, `CItemDrop`, `CItemOption`, `CItemOptionRate`, `CExcellentOptionRate`, `CSetItemType`, `CSetItemOption`, `CJewelOfHarmonyOption`, `C380ItemOption/Type`, `CSocketItemOption/Type`, `CLuckyItem`, `CItemValue`, `CItemStack`, `CItemMove`, `CItemLevel`, `CItemLoot`, `CItemMoneyDrop`, `CWingOption`, `CPeriodicItem`.

## 5. Bags de evento (observado)

`Data/EventItemBagManager.txt` + `Data/EventItemBag/{Item,Monster,Event,Special}/*.txt`:

- Manager: `ItemIndex ItemLevel MonsterClass SpecialValue SpecialLevel SpecialState BagPath` (quem dispara cada bag).
- Bag: seção 2 `ItemRate MoneyDrop BagUseEffect DropType`; seção 3 `BagIndex DropRate MapNumber`; seção 4 `BagIndex Section SectionRate SetDropRate MinDropCount MaxDropCount DW DK FE MG DL`; seção 5 `ItemIndex ItemLevel GradeCount Option0..Option5 Duration`.
- Usado por caixas (Box of Kundun 1–5, Box of Luck/Heaven…), monstros dourados/chefes, recompensas de eventos (BC 1–7, DS 1–6, CC 1–6, quiz, roleta, mata-mata…) e mixes (Wing 1/2, Senior, Cape, Chaos Item).

## 6. Lojas (observado)

`Data/ShopManager.txt` mapeia NPC → arquivo; `Data/Shop/<NPC> - <Nome>.txt`: `ItemIndex ItemLevel Durability SkillSwitch LuckSwitch Aditional Excellent SetOption`. Compra `32`, venda `33`, reparo `34` (classe `CItemManager`). Taxas do castelo afetam preço (`TAX_RATE_STORE`).

## 7. Chaos Machine (observado)

`DATA/GameServerInfo - ChaosMix.dat`: por AL, `FailMixMode_AL*`, `ChaosItemMixRate_AL*`, `DevilSquareMixRate1..6_AL*`, `PlusCommonItemLevelMixRate*_AL*`, `PlusExcSetItemLevelMixRate*_AL*`, etc. (165 linhas). Pacotes `86`/`87`/`88` (`CChaosBox`), resposta `C1 10 86`. Mix de joias empacotadas: `CJewelMix` (`BC 00/01`). Custom: `Custom/CustomMix.txt`, `CustomJewel.txt`, `CustomWing.txt`, `CustomPet.txt`, `CustomEvolution.txt`.

Os algoritmos de cada receita (itens exigidos, taxa, resultado) estão em código nativo e ainda não foram extraídos (U-ITEM-05).

## 8. Reconstrução (itens, mapa, lojas, baú, trade) — observado no GameServer

Implementado em `apps/server/internal/game/{inventory,itemcalc,itemmove,itemuse,mapitem,shop,warehouse,trade}.go` e `internal/gamedata/itemdata.go`.

**Inventário (U-ITEM-02 resolvido):** 0–11 equipamento, 12–75 grade 8×8, 76–107 área estendida (linhas 8–11, alcançável só com `obj+0x60c >= 4`; os caminhos de 24/23/26 exigem slot < 76). Mapa de ocupação `obj+0x3ec` (8 bytes/linha, `FUN_004953b0`/`FUN_00495550`); inserção = primeira célula livre linha a linha (`FUN_00495610`).

**Pacotes:**

| Pacote | Layout | Função |
|---|---|---|
| `C3 22` (cliente) | `[3..4]` número do item no mapa (BE) | `FUN_004982e0` |
| `C3 10 22 r` | r = slot / `FF` falha / `FD` empilhado; `[4..10]` item; 5 bytes zero | idem |
| `C3 10 22 FE` | zen u32 **BE** em `[4]` (pegar zen e atualização de zen) | `FUN_004da370` |
| `C1 05 23 r slot` | r 1 ok / 0 falha; cliente `23 [3]X [4]Y [5]slot` | `FUN_004988f0` |
| `C3 0C 24 r slot item[7]` | r = flag destino (0 inv, 1 trade, 2 baú) ou `FF` (+ `FF`×7) | `FUN_00499410` |
| `C1 0C 25 idx(BE) item[7]` | byte 6 = `slot<<4 \| ((nível-1)/2)&0xF`; para VP2 | `FUN_0049b230` |
| `C1 0C 26 tipo vida(BE) 00 00 vida(u32)` | tipo FF atual / FE máx / **FD enviado no início de todo `26`, com só o byte baixo da vida** | `FUN_004da3e0` |
| `C1 10 27 FF mana(BE) bp(BE) mana(u32) bp(u32)` | | `FUN_004da4c0` |
| `C1 05 28 slot flag` / `C1 06 2A slot dur flag` | apagar item / durabilidade | `FUN_0049b2e0` / `FUN_0049b3b0` |
| `C1 18 F3 13 idx(BE) CharSet[18]` | após item chegar ao inventário (VP2) | `FUN_0049b5a0` |
| `C1 0C F3 14 slot item[7]` | item modificado (joias) | `FUN_0049b620` |
| `C2 20 n` + 11 bytes | `idx(BE, bit7 = estado 1)`, X, Y, item[7]; zen = `0F m2 m1 00 m0 E0 m3` | `FUN_00515b00` |
| `C2 21 n` + idx(BE) | remoção de itens do viewport | `FUN_00514e80` |
| `C3 0B 30 t` + 7 zeros | janela NPC: 0 loja, 2 baú | `FUN_004c2eb0`, `FUN_00517ba0` |
| `C2 31 00 n` + {slot, item[7]} | lista da loja / baú | `FUN_004f50b0`, `FUN_005177c0` |
| `C1 0B 32 r item[7]` / `C1 08 33 r zen(u32@4)` / `C1 08 34 00 zen(u32@4)` | compra / venda / reparo | `FUN_0049a910`, `FUN_0049adb0`, `FUN_0049b080` |
| `C1 0C 81 r baú(u32) zen(u32)` / `C1 03 82` / `C1 08 83 estado nCofres(u32)` | baú | `FUN_00517a00`, `FUN_00517540`, `FUN_00517ac0` |
| `C3 0D 36 nome` / `C1 14 37 r nome nível(u16) guild(u32)` / `C1 04 38 slot` / `C1 0B 39 slot item` / `C1 08 3B zen` / `C1 04 3C f` / `C1 04 3D r` | trade | `FUN_005088e0`…`FUN_00508d30` |

**Itens no mapa:** 300 por mapa (`0x87d1c8 + mapa*0x130d8`, slot 0x104); expira em `ItemDropTime` s (zen: `MoneyDropTime`), dono exclusivo por `ItemDropTime*500` ms (zen sem dono); pegar exige |dx|,|dy| ≤ 2; `ItemLoot.txt` AllowLoot 0 = só quem dropou. Não se dropa item +5 ou mais (categorias 0–11), excelente ou de set; nenhuma checagem de distância no drop.

**Fórmulas:** durabilidade máx. `FUN_00494500`; requisitos `((nívelItem(+25 exc/set) + nível*3) * req * 3)/100 + 20` (energia ×4); preço `FUN_0048c7a0` (ItemValue.txt → BuyMoney → poções `V²*10/12 * 2^nível * qtd` → genérico `(L+40)L²/8+100`, asas `(L+40)L²*11+40.000.000`, ×0,8 arma de uma mão, +25% luck, +opção*60%, ×2 por opção excelente); venda = compra/3 menos `0,6*(1-dur/máx)` para equipamentos; reparo `3*√p*√√p*(1-dur/máx)+1` (×1,4 se quebrado), p = compra/3; taxa do baú `nível²*0,04 (+2*nível com senha)`; poções `taxa_classe * (MaxLife+AddLife)/100`.
