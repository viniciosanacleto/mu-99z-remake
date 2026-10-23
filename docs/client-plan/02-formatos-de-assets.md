# 02 — Formatos de assets do cliente

Fonte: `Main.exe` (loaders decompilados com REA) + inspeção direta dos arquivos em `Cliente/Data/`.

## 1. Criptografias de arquivo (observado)

### 1.1 `MapFileDecrypt` (terrenos e BMD v0x0C)

Usada em `FUN_005e6e20` (`.map`), `FUN_005e6a00` (`.att`), `FUN_005f4830` (`.obj`) e `FUN_004e98c0` (BMD versão `0x0C`):

```
key = D1 73 52 F6 D2 9A CB 27 3E AF 59 31 37 B3 E7 A2
w = 0x5E
for i in 0..n-1:
    out[i] = (in[i] ^ key[i % 16]) - w        (byte, wrap-around)
    w      = in[i] + 0x3D                     (usa o byte CIFRADO)
```

### 1.2 "Bux" (tabelas `.bmd` de texto/dados)

XOR cíclico com `FC CF AB` (`FUN_0053e300`, aplicada por registro). Arquivos com zeros aparecem como `fc cf ab fc cf ab…` — assinatura fácil de reconhecer.

### 1.3 Chaves de rede

`Data/Enc1.dat` (cifrar envio C3/C4) e `Data/Dec2.dat` (decifrar recepção) — formato em [../server-plan/02-protocolo-de-rede.md §2.2](../server-plan/02-protocolo-de-rede.md#22-simplemodulus-em-c3c4-observado).

## 2. Texturas

Carregadas por `FUN_0062c1e0` (nome `.jpg` → arquivo `.OZJ`) e `FUN_0062c670` (`.tga` → `.OZT`) com `(caminho, idTextura, filtro, wrap, …)`; os parâmetros são enums OpenGL: `0x2600` `GL_NEAREST`, `0x2601` `GL_LINEAR`, `0x2900` `GL_CLAMP`, `0x2901` `GL_REPEAT` (observado em `FUN_006096b0`). O caminho no código usa a extensão "lógica" (`Interface\Menu01_new.jpg`) e o arquivo em disco tem a extensão `OZ*`.

| Extensão | Conteúdo (pelos bytes do arquivo) | Status |
|---|---|---|
| `.OZJ` / `.ozj` | 24 bytes de prefixo + JPEG (em `Interface/Menu01.OZJ` o segundo `FF D8 FF E0` começa no offset `0x18`) | **inferido** pelos bytes; loader não decompilado |
| `.OZT` / `.ozt` | 4 bytes de prefixo + TGA não comprimido (em `Interface/Cursor.ozt`: tipo 2, 32×32, 32 bpp, descritor `0x08`) | **inferido** |
| `.OZB` | 4 bytes de prefixo (`42 4B 6D 4B`) + BMP (`TerrainHeight.OZB`: BMP 256×256, 8 bpp, paleta) | **inferido** |

Quantidade: ~2900 OZJ, ~775 OZT, 19 OZB.

## 3. Modelos — BMD (observado, `FUN_004e98c0`)

### 3.1 Cabeçalho e versões

| Bytes | Campo |
|---|---|
| 0–2 | `"BMD"` |
| 3 | versão |

Distribuição no `Data/`: versão `0x0A` (1320 arquivos, em claro), `0x0C` (1041, cifrados), `0x0E` (3, ex. `Object1/heroNpcchair.bmd`), `0x00` (4, ex. `Object3/Object27.bmd`). O loader só trata `0x0C` como cifrado; `0x0E`/`0x00` provavelmente não são usados (U-ASSET-01).

Se versão `0x0C`: `u32 tamanho` em `[4..7]`, depois `tamanho` bytes cifrados com `MapFileDecrypt`; o resto do parse ocorre sobre o buffer decifrado começando no offset 0. Caso contrário o parse começa no offset 4.

### 3.2 Corpo (offsets relativos ao início do corpo)

```
char   Name[32]              // ex. "Data2\Player\Armor\class_01.smd" (origem .smd)
int16  NumMeshes             // +0x20
int16  NumBones              // +0x22
int16  NumActions            // +0x24
// +0x26: meshes
for each mesh:
    int16 NumVertices, NumNormals, NumTexCoords, NumTriangles, TextureIndex   // 10 bytes
    Vertex   [NumVertices]   // 16 bytes: int16 Node, int16 pad?, float3 Position
    Normal   [NumNormals]    // 20 bytes: int16 Node, int16 pad?, float3 Normal, int16 BindVertex, int16 pad?
    TexCoord [NumTexCoords]  // 8 bytes: float U, float V
    Triangle [NumTriangles]  // 64 bytes no arquivo, 36 usados (copiados 9 dwords)
    char TextureName[32]
for each action:
    int16 NumAnimationKeys
    uint8 LockPositions
    if LockPositions: float3 Positions[NumAnimationKeys]
for each bone:
    uint8 Dummy
    if !Dummy:
        char  Name[32]
        int16 Parent
        for each action: float3 Position[keys]; float3 Rotation[keys]   // rotação em ângulos; o loader gera quaternions (FUN_005eb280)
```

Os subcampos de `Vertex`/`Normal`/`Triangle` (índices de vértice/normal/UV, `Polygon` 3/4) estão "copiados em bloco" pelo loader — o significado de cada byte precisa ser confirmado contra o renderer (U-ASSET-02). Nomes de textura em `TextureName` apontam para arquivos no mesmo diretório do modelo (com troca de extensão para OZJ/OZT).

### 3.3 Organização e nomes

- Diretórios: `Player\` (corpo e equipamento por classe: `ArmorClass01.bmd`, `HelmMale01.bmd`…), `Item\`, `Monster\`, `NPC\`, `Object<N>\` (objetos do mapa N), `Skill\`, `Effect\`, `Logo\`.
- Formatos de nome (`FUN_005faca0`): `%s.bmd`, `%s0%d.bmd` (índice < 10) e `%s%d.bmd`. Ausência de arquivo ⇒ MessageBox "file does not exist" (exceto para `Monster`, `PlayerTest` e um terceiro prefixo).
- Modelos custom vêm de `PATCH CLIENTE/Data/CustomItem.txt`, `CustomWing.txt`, `CustomPet.txt`, `CustomJewel.txt`, `CustomMonster.txt` (campo `ModelName`, pasta `Data\Item` ou `FolderPath`).

## 4. Terreno (observado)

Para o mapa `m` (0-based), o diretório é `World{m+1}` e os arquivos `EncTerrain{m+1}.*` (com exceções: Blood Castle 11–17 usam `World12`, Chaos Castle 18–23 usam `World19`, Kalima 24–29 e 36 usam `World25` — `FUN_00606fa0`). O número de mapa gravado no arquivo é validado contra o esperado; divergência ⇒ "file corrupted".

### 4.1 `EncTerrainN.map` — tiles (`FUN_005e6e20`)

Após `MapFileDecrypt`:

| Offset | Tamanho | Conteúdo |
|---|---|---|
| 0 | 1 | versão |
| 1 | 1 | número do mapa |
| 2 | 65536 | camada 1: índice de tile por célula (256×256) |
| 0x10002 | 65536 | camada 2: índice de tile |
| 0x20002 | 65536 | alpha da camada 2 (byte / 255.0) |

Texturas de terreno carregadas por `FUN_00606fa0` (id interno → arquivo, **observado**): `0x23` `TileGrass01`, `0x24` `TileGrass02`, `0x25` `TileGround01`, `0x26` `TileGround02`, `0x27` `TileGround03`, `0x28` `TileWater01`, `0x29` `TileWood01`, `0x2A..0x30` `TileRock01..07` (OZJ, `GL_REPEAT`); `0x32..0x34` `TileGrass01..03` (OZT, com alpha); `100/0x65` `leaf01/leaf02`; `0x66..0x68` `rain01..03`. A correspondência "índice de tile no `.map` = id − `0x23`" (logo 0 = TileGrass01 … 13 = TileRock07, 15–17 = grama OZT) é **inferida** e precisa ser confirmada no renderer de terreno (U-ASSET-07).

### 4.2 `EncTerrainN.att` — atributos (`FUN_005e6a00`)

Após `MapFileDecrypt` **e** Bux (`FUN_005e6cc0`, **inferido** pelo papel da chamada): tamanho `0x10004` (1 byte por célula) ou `0x20004` (2 bytes por célula). Cabeçalho de 4 bytes: `[0] versão`, `[1] mapa`, `[2] largura-1 (0xFF)`, `[3] altura-1 (0xFF)`. Qualquer célula com valor `> 0x7F` ⇒ arquivo rejeitado. Mesmos bits do servidor ([../server-plan/07-mundo-e-objetos.md §3](../server-plan/07-mundo-e-objetos.md#3-atributos-de-terreno-observado)).

### 4.3 `EncTerrainN.obj` — objetos estáticos (`FUN_005f4830`)

Após `MapFileDecrypt`: `[0] versão`, `[1] mapa`, `[2..3] int16 count`, depois `count` registros de 30 bytes: `int16 type`, `float3 position`, `float3 angle`, `float scale`. `type` indexa os modelos `Object{m+1}\ObjectNN.bmd`.

### 4.4 Altura e luz

- `TerrainHeight.OZB` (BMP 8 bits 256×256) — `FUN_005e7670`; escala da altura a extrair (U-ASSET-03).
- `TerrainLight.OZJ` (JPEG 256×256 lightmap) — `FUN_005e75b0`; `TerrainLight2.jpg` alternativo no mapa 30 durante Castle Siege.

## 5. Tabelas de dados (`Data\Local\…`) (observado)

O `Main.dll` redireciona para `Data\Local\<lang>\<Nome>_<lang>.bmd` (ex. `Item_Eng.bmd`).

### 5.1 `Item.bmd` — 8192 × 80 bytes + checksum (`FUN_005404f0`)

- Tamanho `0xA0000` + `u32` checksum (arquivo de 655364 bytes).
- Checksum (validado contra `Item_Eng.bmd` real: `0x09E230C4`):

```
key = 0x01C5E200
for off in 0, 4, …, 0x9FFFC:
    v = u32le(data[off])
    if ((off/4 - 1) & 1) == 0: key ^= v   else: key += v
    if off % 16 == 0: key ^= (key + 0xE2F1) >> ((off/4 & 7) + 1)
```

- Cada registro de 80 bytes é decodificado com Bux. Layout validado por comparação com `MuServer/Data/Item/Item.txt` (Kris, Rapier, Katana, Bronze Helm, Small Healing Potion, Jewel of Chaos):

| Offset | Tipo | Campo | Evidência |
|---|---|---|---|
| 0 | char[30] | Nome | "Kris" |
| 30 | u8 | (two-hand? sempre 0 nas amostras) | — |
| 31 | u8 | Level | Kris 6, Helm 16, poção 10 |
| 32 | s8 | Slot (`0xFF` = nenhum) | Helm 2, poção −1 |
| 33 | u8 | Skill | Katana 20 |
| 34 | u8 | Width | 1 |
| 35 | u8 | Height | 2/3 |
| 36 | u8 | DamageMin | 6 / 9 / 16 |
| 37 | u8 | DamageMax | 11 / 15 / 26 |
| 38 | u8 | DefenseRate | — |
| 39 | u8 | Defense | Helm 9 |
| 40 | u8 | MagicDefense | — |
| 41 | u8 | AttackSpeed | 50 / 40 / 35 |
| 42 | u8 | WalkSpeed | — |
| 43 | u8 | Durability | 20 / 23 / 27 / 34 |
| 44 | u8 | MagicDurability | — |
| 46 | u16 | ReqStrength | 40 / 50 / 80 |
| 48 | u16 | ReqDexterity | 40 / 20 |
| 50–63 | — | ReqEnergy, ReqVitality, ReqLeadership, ReqLevel, Value (poção 10 em 58)… | **inferido** |
| 64 | u8 | atributo/flag (1 armas, 4 elmo) | coincide com a coluna `Attr` do servidor |
| 65–69 | u8×5 | classes DW, DK, FE, MG, DL | Rapier `0 1 1 1 1` = servidor |
| 70–79 | — | resistências / reservado | **desconhecido** |

### 5.2 Outras tabelas

| Arquivo | Tamanho | Formato | Status |
|---|---|---|---|
| `Data\Gate.bmd` | 2304 = 256 × 9 | registros de 9 bytes Bux (`FUN_0053efa0`): gate → mapa/retângulo/destino | campos **a mapear** contra `Move/Gate.txt` |
| `Skill_<lang>.bmd` | 15364 = 256 × 60 + 4 | registros Bux com nome char[32] em `+0` do registro ("Poison" no registro 1) + atributos (MP 42 visível) | layout a mapear (U-ASSET-04) |
| `Text_<lang>.bmd` | 614400 | Bux; primeiro texto "Tahoma" — não é registro fixo de 256 | loader `0x60afe8` a decompilar (U-ASSET-05) |
| `Dialog_<lang>.bmd` | 204800 | Bux; texto de NPC ("Aye! Is there anything you need?") | idem |
| `Quest_<lang>.bmd` | 136000 | — | idem |
| `Slide_<lang>.bmd` | 41008 | mensagens rolantes | idem |
| `Movereq_<lang>.bmd` | 2884 | cifra diferente de Bux (padrão `dW3` repetido) + nomes ("Lorencia") | idem |
| `ItemSetOption_<lang>.bmd`, `ItemSetType.bmd` | 4868 / — | sets ancient | idem |
| `Filter.bmd`, `FilterName.bmd` | — | palavras proibidas em chat/nome | idem |
| `NpcName_<lang>.txt` | texto | nomes de NPC | texto puro |
| `Data\Macro.txt` | texto | macros de chat | — |

## 6. Áudio

`Data\Sound\*.wav` (≈550 arquivos: ambiente `a*`, efeitos `e*`, interface `i*`, monstros, `BattleCastle\`), carregados por id (`FUN_0041cf50(id, caminho, canais, 3D?)`, ex. id 4 `iTitle.wav`, 0x19 `iButtonClick.wav`). Música: `Data\Music\%s` (Ogg; pasta ausente nesta cópia — U-ASSET-06). Mapas custom definem `MusicPath` em `CustomMap.txt`.

## 7. Pipeline recomendado

1. Biblioteca `assets` (linguagem do cliente) com decoders acima + testes que abrem **todos** os arquivos de `Data/`.
2. Ferramenta CLI `mu-assets export` → glTF 2.0 (malha + skin + animações), PNG/KTX2, JSON (tabelas), para depuração e para engines que preferem formatos padrão.
3. O cliente pode carregar direto os formatos originais (compatibilidade com patches de servidores) ou o cache convertido.
