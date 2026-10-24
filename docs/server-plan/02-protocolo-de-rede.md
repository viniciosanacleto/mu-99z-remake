# 02 — Protocolo de rede

Este é o contrato de fio que o servidor Go precisa reproduzir byte a byte para que o cliente original (`Main.exe` + `Main.dll`) funcione.

## 1. Enquadramento (observado)

Parser incremental idêntico nos 4 servidores (CS `FUN_00406dc0`, GS `FUN_005049c0`):

| Byte 0 | Cabeçalho | Tamanho | Head |
|---|---|---|---|
| `0xC1` | `C1 size head …` | `size` = byte 1 (u8), inclui o cabeçalho | byte 2 |
| `0xC2` | `C2 sizeHi sizeLo head …` | `size` = bytes 1–2 **big-endian** | byte 3 |
| `0xC3` | `C3 size <cifrado>` | byte 1 | (dentro do cifrado) |
| `0xC4` | `C4 sizeHi sizeLo <cifrado>` | bytes 1–2 BE | (dentro do cifrado) |

Regras:

- Qualquer outro primeiro byte ⇒ `Protocol header error` ⇒ **desconectar**.
- `size < 3` ⇒ erro. `size > limite` ⇒ erro. Limite: **0x800** (CS/JS/DS) e **0x2000** (GS). Após decifrar, o conteúdo de C3/C4 tem que caber em **0x7FE** bytes.
- Se o buffer tem menos bytes que `size`, aguarda mais dados (o resto é movido com `memmove` para o início).
- Pacotes com subcódigo usam o byte seguinte ao head (`buf[3]` em C1, `buf[4]` em C2). O GameServer escolhe o subcódigo assim: `sub = (buf[0]==0xC1) ? buf[3] : buf[4]` (`0x4d6690`).
- ConnectServer só aceita C1/C2 (sem criptografia).

## 2. Criptografia

### 2.1 XOR32 nos pacotes C1/C2 cliente → GameServer (observado)

`CPacketManager` (`FUN_004ceb30` → `FUN_004cebb0`) aplica, a todo pacote C1/C2 recebido do cliente, antes do dispatch:

```
for n = size-1 downto start+1:
    data[n] ^= data[n-1] ^ XorKey[n % 32]
```

- `XorKey` (32 bytes, gravado em `FUN_004ce420`, little-endian a partir de `0x0173e350`):
  `E7 6D 3A 89 BC B2 9F 73 23 A8 FE B6 49 5D 39 5D 8A CB 63 8D EA 7D 2B 5F C3 B1 E9 83 29 51 E8 56`
- O processamento vai do fim para o início (usa o byte anterior **ainda cifrado**).
- `start` = posição do head: **observado** em `FUN_004ceb30`, que carrega `EDI = 2` (C1) ou `3` (C2) e `EAX = size-1` antes de chamar `FUN_004cebb0`. O head nunca é alterado; o primeiro byte transformado é `start+1`.
- O XOR32 também é aplicado aos pacotes C3/C4 **depois** do SimpleModulus, sobre o pacote já remontado como C1/C2 (`FUN_005049c0` chama `FUN_004ceb30` nos três ramos). O cliente, portanto, aplica XOR32 ao pacote em claro e só depois cifra com SimpleModulus.
- Pacotes **servidor → cliente** não passam por XOR32 (`FUN_00504f00` só transforma C3/C4).
- Implementação: `apps/server/internal/protocol/crypto/xor32.go`.

### 2.2 SimpleModulus em C3/C4 (observado)

Usado nos dois sentidos para pacotes C3/C4.

**Arquivos de chave** (`FUN_004ce4e0`):

| Lado | Cifrar com | Decifrar com |
|---|---|---|
| Servidor | `Data\Hack\Enc2.dat` | `Data\Hack\Dec1.dat` |
| Cliente | `Data\Enc1.dat` | `Data\Dec2.dat` |

Formato do arquivo: `u16 magic = 0x1112`, `u32 = 0x36`, depois 3 blocos de 16 bytes (4 × u32). Cada u32 lido é XOR-ado com `{0x3F08A79B, 0xE25CC287, 0x93D27AB9, 0x20DEA7BF}` (índice = posição no bloco). Os 3 blocos são, na ordem, **Modulus[4]**, **Key[4]** (encrypt ou decrypt) e **XorKey[4]**. O par cifrar é armazenado em `+4`, o par decifrar em `+0x34` do contexto.

**Cifrar** (`FUN_004ce630`), por bloco de até 8 bytes de entrada (4 × u16) → 11 bytes de saída:

```
src = 4 x u16 (pad com zero)
r0 = ((src0 ^ Xor[0])        * EncKey[0]) % Mod[0]
r1 = ((src1 ^ Xor[1] ^ r0&0xFFFF) * EncKey[1]) % Mod[1]
r2 = ((src2 ^ Xor[2] ^ r1&0xFFFF) * EncKey[2]) % Mod[2]
r3 = ((src3 ^ Xor[3] ^ r2&0xFFFF) * EncKey[3]) % Mod[3]
t0 = Xor[0] ^ r0 ^ (r1 & 0xFFFF)
t1 = Xor[1] ^ r1 ^ (r2 & 0xFFFF)
t2 = Xor[2] ^ r2 ^ (r3 & 0xFFFF)
t3 = r3            (sem máscara: local_c; só t2 usa r3 & 0xFFFF)
para cada ti: grava 16 bits + 2 bits altos (bit-packing via FUN_004ce990, offsets 0 e 22)
checksum = XOR dos 8 bytes de entrada ^ 0xF8
size_byte = blockLen ^ 0xF8 ^ checksum (blockLen = min(8, restante))
grava (checksum<<8 | size_byte) ^ 0xF83D nos últimos 16 bits
```

**Decifrar** (`FUN_004ce7e0`): processo inverso, por bloco de 11 bytes; usa `DecKey` e valida o checksum `0xF8`; bloco inválido retorna `-1` e o pacote é descartado. O comprimento final é a soma dos `blockLen` decodificados.

Detalhes observados que a implementação reproduz (`apps/server/internal/protocol/crypto/simplemodulus.go`):

- As palavras de entrada são 4 × u16 **little-endian**; a aritmética é u32 com overflow (`(x ^ xor) * key % mod`).
- `FUN_004ce990(dst, dstBit, src, srcBit, n)` copia `n` bits em ordem **MSB-first** dentro de cada byte (o deslocamento de buffer `FUN_004cea90` desloca à direita para `n > 0` e à esquerda para `n ≤ 0`). Os "2 bits altos" lidos do bit 22 de um u32 LE são os bits 16–17 do valor.
- Os dois últimos bytes do bloco são `[checksum ^ 0xF8 ^ len ^ 0x3D, checksum ^ 0xF8]`; na decifragem, `len = b9 ^ b10 ^ 0x3D` e o bloco é válido se `XOR(8 bytes decifrados) ^ 0xF8 == b10`.
- As chaves de decifragem são os inversos modulares das de cifragem (verificado: `Enc1.Key[i] * Dec1.Key[i] ≡ 1 (mod Modulus[i])`). Pares: cliente `Enc1.dat` ↔ servidor `Dec1.dat`; servidor `Enc2.dat` ↔ cliente `Dec2.dat` (mesmos `Modulus`/`Xor` em cada par). Testes de ida e volta com os arquivos reais passam para tamanhos de 1 a 64 bytes.

### 2.3 Contador serial em C3/C4 (observado)

- **Envio** (`FUN_00504f00`): o texto em claro cifrado é `[serial][head][corpo…]`, onde `serial = serialOut[aIndex]++` (u8 por conexão, `DAT_01020345 + index*8`). Saída: `C3 (encLen+2) <enc>` ou `C4 hi lo <enc>` (tamanho = encLen+3).
- **Recepção** (`FUN_005049c0`): após decifrar `size-2` (C3) ou `size-3` (C4) bytes, o primeiro byte em claro é o serial e o segundo é o head. O pacote é remontado como `C1 (decLen+1) head corpo` (ou `C2 (decLen+2)…`), passa pelo XOR32 (§2.1) e o serial vai para a checagem `HackPacketCheck` (string `Packet serial error … Serial: [%d][%d]`). Limites: C1/C2 com `size-1 > 0x7FE`, C3 com `decLen > 0x7FE` e C4 com `decLen+1 > 0x7FE` são descartados com desconexão.

### 2.4 "Bux" (observado)

XOR cíclico de 3 bytes `FC CF AB` (`FUN_00512390`). Usado em:

- Conta e senha no login (`F1 01`).
- Arquivos binários de dados do cliente (`Item.bmd`, `Gate.bmd`, `Skill.bmd`…, ver plano do cliente).

### 2.5 Cifra de fluxo do socket do GameServer (observado)

Além das camadas acima, **todo byte** que passa pelo socket de clientes do GameServer (incluindo os cabeçalhos `C1..C4`) é transformado:

| Sentido | Função | Transformação |
|---|---|---|
| recepção | `FUN_005051d0` (antes do parser `FUN_005049c0`) | `b = (b ^ K2) - K1` |
| envio | `FUN_00487a30` (em `FUN_00504f00`, depois do SimpleModulus) e `FUN_005126b0` (envio bloqueante) | `b = (b + K1) ^ K2` |

`K1 = DAT_00770d7c`, `K2 = DAT_00770d7d`, calculados uma vez na inicialização (`FUN_0047d3c0` → `FUN_00487a60`):

```
name[32]   = CustomerName      (CServerInfo+0x04; passado a wsprintfA como formato)
serial[17] = ServerSerial      (CServerInfo+0xA2; 16 caracteres + NUL)
sum (u16)  = Σ_{i=0..31} (serial[i % 17] ^ name[i])
K2 = (sum & 0xFF) - 0x1E
K1 = (sum >> 8) + 0x76
```

Ambas as chaves vêm de `GameServerInfo - Common.dat` (`FUN_004ecb40`). Com os valores desta instalação (`CustomerName = WizardTeam`, `ServerSerial = A9kT3wX5rBz7fQ1u`): `sum = 0x0869`, `K1 = 0x7E`, `K2 = 0x4B`. O `MainInfo.ini` do cliente tem os mesmos `CustomerName` e `ClientSerial`, então o `Main.dll` deriva as mesmas chaves (**inferido**: o lado do cliente não foi decompilado). O ConnectServer **não** usa essa cifra (`FUN_00407240` entrega os bytes direto ao parser).

Ordem completa no socket do GameServer:

```
cliente → servidor:  pacote em claro → XOR32 → [C3/C4: serial + SimpleModulus(Enc1)] → fluxo
servidor (recepção): fluxo⁻¹ → frame → [C3/C4: SimpleModulus⁻¹(Dec1), separa serial, remonta C1/C2] → XOR32⁻¹
servidor → cliente:  pacote em claro → [C3/C4: serial + SimpleModulus(Enc2)] → fluxo
```

Implementação: `apps/server/internal/protocol/crypto/stream.go` e o pipeline em `apps/server/internal/protocol/gscodec`.

## 3. Sequência de conexão

```
Cliente                         ConnectServer (TCP 44405)
  │ ◄──── C1 04 00 01 ──────────── (hello ao aceitar a conexão; constante 0x010004C1)
  │ ◄──── C2 .. F3 EA ………… ──── (nomes dos servidores, custom WizardTeam)
  │ ───── C1 .. F4 06 ──────────► pedir lista
  │ ◄──── C2 hi lo F4 06 count[2 BE] {code u16, load u8, 0xCC}×N
  │ ───── C1 .. F4 03 code[2] ──► pedir endereço do servidor `code`
  │ ◄──── C1 16 F4 03 ip[16] port[2]
  ╳ fecha
Cliente                         GameServer (TCP 55900)
  │ ◄──── C1 0E F1 00 01 idx[2 BE] version[5] ServerCode[2 LE]
  │ ───── C1 .. F3 09 hwid[45 Bux] type          (obrigatório em até 5 s)
  │ ───── C3 .. F1 01 account[10] pass[10] tick[4] version[5] serial[16]
  │ ◄──── C1 05 F1 01 result
  │ ───── F3 00 ──► C3 .. F3 00 lista + C1 32 F3 F9 (por personagem) + C1 0E DE 00
  │ ───── F3 01 (criar) / F3 02 (apagar)
  │ ───── F3 03 nome  (entrar no mundo)
  │ ◄──── C3 74 F3 03 + C4 F3 10 (inventário) + C1 F3 11 (magias) + C2 12 (o próprio jogador) + …
```

Notas:
- `F1 00` (servidor → cliente), builder **`0x4db050`**, enviado logo após o accept (`0x505580` → `FUN_0050c260` aloca o índice em round-robin; servidor fechando ⇒ `F1 01 02`, cheio (`ServerMaxUserNumber`, máx. 1000) ⇒ `F1 01 04`, ambos com envio bloqueante e fechamento): `[4] 1`, `[5..6] índice BE`, `[7..11] versão (5 bytes)`, `[12..13] ServerCode u16 LE`. No cliente (`Main.exe 0x4bc2b0`) a versão é comparada com `ClientVersionEncoded[i] - (i+1)`. Seriais iniciais por conexão: recepção `0xFF`, envio `0`.
- `F3 09` (`0x441730`): `[4..48]` hardware id de 44 caracteres codificado com Bux (com `-` nas posições 8/17/26/35), `[0x31]` idioma do cliente (0 Eng, 1 Por, 2 Spn; >2 ⇒ 0). Sem ele o tick de 1 s fecha a conexão após 5 s ("Not verified"); HWID inválido fecha na hora. O HWID é o enviado ao JoinServer no login.
- Inatividade: 60 s sem pacotes ⇒ fecha ("Timeout").
- A versão configurada é `1.00.00` (`ServerVersion` em `Common.dat` = `ClientVersion` em `PATCH CLIENTE/MainInfo.ini`); o serial é `A9kT3wX5rBz7fQ1u` (16 bytes) nos dois lados. Conversão `"1.00.00"` → 5 bytes **observada** em `FUN_004ecb40`: o GS guarda os caracteres 0, 2, 3, 5 e 6 da string (`"10000"`) em `CServerInfo+0x9c`; o GameServer compara `int` em `+0x1c` e `char` em `+0x20` do contexto de versão (`0x4d9360`).

## 4. Pacotes de login e personagem (observado)

### `F1 01` — login (cliente → GS), handler `0x4d9360`

| Offset | Tamanho | Campo |
|---|---|---|
| 0 | 4 | `C1/C3 size F1 01` |
| 4 | 10 | conta (Bux `FC CF AB`) |
| 14 | 10 | senha (Bux) |
| 24 | 4 | TickCount do cliente (guardado em `obj+0x60`) |
| 28 | 5 | versão do cliente |
| 33 | 16 | serial do cliente |

Comportamento: estado ≠ 1 ⇒ desconecta. Versão (5 bytes) ou serial (16 bytes) diferentes ⇒ `C1 05 F1 01 06`. Com um pedido pendente (`obj+8`) o pacote é ignorado; `obj+9` conta tentativas. Em seguida o GS envia o pedido ao JoinServer (agora `auth.Login`).

Resultado `F1 01` (resposta do JS tratada em `0x4a18d0`; o código vai em `CL` para `FUN_004db220`):

| Código | Condição |
|---|---|
| 1 | sucesso ⇒ estado 2 |
| 0 / 2 / 3 / 4 / 7 | resultado do JoinServer (senha errada, conta inválida, já conectada, cheio, limite IP/HWID); vira **8** a partir da 3ª tentativa |
| 2 | `ConnectMemberCheck` ligado e conta fora de `ConnectMember.txt` |
| 5 | `bloc_code ≠ 0` |
| 6 | versão/serial do cliente |
| 10 (`0x0A`) | `AccountLevel < ServerLock` |
| — | `MaxConnectionPerIP/HID` do GS estourado (`FUN_004413f0`) ⇒ desconecta sem resposta |

### `F1 02` — logout (cliente → GS), handler `0x4d9560`

`[4] type` (0 sair, 1 trocar personagem, 2 trocar servidor; > 2 ⇒ 0) — válido em estado 2 ou 3 (`FUN_004c4f80`; bloqueado em estados especiais com mensagens 0x105–0x109). Arma a contagem `obj+0x0a = 1`; o tick de 1 s (`FUN_00510810`) envia `C3 05 F1 02 type` quando a contagem chega a 1 (tipo 1 volta para a seleção de personagens, salvando o personagem), e para contagens maiores envia a mensagem 0x121 com os segundos restantes.

### `F3 00` — lista de personagens, `F3 01` — criar, `F3 02` — apagar, `F3 03` — entrar

- `F3 00` (`0x475120` → DS `0x01` → `0x472d00`): só em estado 2. Resposta **C3** `F3 00`: `[4]` flag de classe (2; 3 se o maior nível da conta ≥ `MGCreateLevel_AL<n>`; 4 se ≥ `DLCreateLevel_AL<n>`), `[5]` `MoveCnt`, `[6]` quantidade, depois 34 bytes por personagem: `slot`, nome[10], (1 byte livre), nível u16, `CtlCode`, CharSet[18], `G_Status` (0xFF sem guild). O `ExtClass` da conta é recalculado (bit 2 = MG, bit 1 = DL) e o valor do banco é ignorado. Depois: `C1 32 F3 F9 slot` + 45 bytes (9 itens × 5 bytes do DS: bytes 0,1,7,8,9 do item) por personagem, e `C1 0E DE 00 flag 00 ExpConstA ExpConstB MaxLevel CharacterDeleteMaxLevel` (u16 LE; `0x4daef0`).
- `F3 01` (`0x4d95c0`): nome em `[4..13]`; `[14]` classe. `CharacterCreateSwitch` desligado, nome com `' ' " % ' / \ _` (`CheckTextSymbol`), classe fora da lista ou sem o bit de `ExtClass` (`0x30`↔bit 2, `0x40`↔bit 1, `0x50`↔bit 0, `0x60`↔bit 3) ⇒ `C1 2C F3 01 00 nome`. O DS (`0x40bcf0`) aplica `BadSyntax.txt` (resultado 0), procura o primeiro `GameID1..5` vazio (sem slot ⇒ 2) e executa `WZ_CreateCharacter` (0 nome existe, 1 ok, 2 erro, 3 sem slot). Resposta `C1 2C F3 01 result nome[10] slot nível(u16=1) byteDeClasse 0xFF×24` (`0x473770`; byteDeClasse = byte 0 do CharSet da classe).
- `F3 02` (`0x4d9780`): exige estado 2 e `obj+0x0c ≠ 0` (na tela de seleção). `CharacterDeleteSwitch` desligado, código pessoal (`PersonalCodeCheck`: 7 caracteres a partir do 7º de `sno__numb`) errado ou conta com `Lock > 0` (mensagem 0x6F2) ⇒ `C1 05 F3 02 02`; senão DS `WZ_DeleteCharacter` ⇒ 1 (ok) / 0. O limite `CharacterDeleteMaxLevel` é enviado ao cliente no `DE 00` (o GS não o verifica).
- `F3 03` (`0x4d98a0`): exige estado 2; zera `obj+0x0c` e pede a carga do personagem (DS `0x04`). Personagem inexistente ou com `CtlCode & 1` ⇒ desconecta.

### `C3 B0 F3 03` — dados do personagem ao entrar (GS → cliente), builder `0x473a10`

**Corrigido em 2026-10-24 pelo cliente (U-PROTO-09).** A leitura do builder `0x473a10` dava 116 bytes (`0x74`), com o bloco u32 em 60 e o nível em 108. O `Main.dll` da WizardTeam (`PATCH CLIENTE/Path/Main.dll`, handler `0x1001d640`, caso 03 da tabela de subcódigos `F3` em `0x1001d08c`/`0x1001d0e0`) lê **176 bytes (`0xB0`)**. Com o pacote de 116 bytes o cliente mostrou resets = LevelUpPoint, nível 0 e atributos com lixo (observado). Os bytes 0–55 são a parte 97D lida pelo `main.exe` e não mudaram. Valores > 65000 são truncados para 65000 nos campos u16 e repetidos em u32 no final.

Parte lida pelo `Main.dll` (offset → variável global do cliente):

| Off | Tipo | Campo |
|---|---|---|
| 56 (`0x38`) | u16 | classe (`0x1002d038`, a mesma do `F3 E0` [46]) |
| 60 (`0x3c`) | u32 | ResetCount (`0x1027178c`, painel "Resetes") |
| 64 (`0x40`) | u32 | MasterResetCount (`0x10271790`) |
| 68 (`0x44`) | u32 | AccountLevel (`0x10271794`) |
| 72 (`0x48`) | 30 bytes | custom não identificado (`0x10164b20`; mesmos campos do `F3 E0` [60..89]); enviado 0 |
| 102 (`0x66`) | u32 ×5 | custom não identificado (`0x101644cc..dc`; `F3 E0` [90..109]); enviado 0 |
| 124 (`0x7c`) | u32 | autoridade de GM (`0x10271798`; `F3 E0` [112]) |
| 128..172 (`0x80..0xac`) | u32 ×12 | LevelUpPoint, Vida, Vida máx., Mana, Mana máx., BP, BP máx., Força, Agilidade, Vitalidade, Energia, Liderança (`0x1027179c..0x102717d4`) |

O nível e a EXP da janela de personagem do `Main.dll` (`0x10271788`, `0x102717bc`/`0x102717c0`) **não** vêm do `F3 03`: só o `F3 05` (level up, `0x1001d780`) e o `F3 E0` (`0x1001d820`) os gravam. Por isso o servidor manda um `F3 E0` logo depois da entrada no mundo (**inferido** do cliente: não se sabe em que ponto o GS original o envia). A Patente (`obj+0x2500`) saiu do `F3 03`.

Leitura antiga do builder, mantida como referência (offsets ≥ 56 substituídos pela tabela acima):

| Off | Tipo | Campo |
|---|---|---|
| 0 | 4 | `C3 74 F3 03` |
| 4 | u8 | X |
| 5 | u8 | Y |
| 6 | u8 | Mapa |
| 7 | u8 | Direção |
| 8 | u32 | Experiência |
| 12 | u32 | Próxima experiência |
| 16 | u16 | LevelUpPoint |
| 18 | u16 | Força |
| 20 | u16 | Agilidade |
| 22 | u16 | Vitalidade |
| 24 | u16 | Energia |
| 26 | u16 | Vida |
| 28 | u16 | Vida máx. (inclui bônus) |
| 30 | u16 | Mana |
| 32 | u16 | Mana máx. |
| 34 | u16 | BP |
| 36 | u16 | BP máx. |
| 40 | u32 | Zen |
| 44 | u8 | PK level |
| 45 | u8 | CtlCode |
| 46 | u16 | FruitAddPoint |
| 48 | u16 | Max FruitAddPoint |
| 50 | u16 | Liderança |
| 52 | u16 | FruitSubPoint |
| 54 | u16 | Max FruitSubPoint |
| 56 | u32 | ResetCount (`obj+0x6dc`) |
| 60..104 | u32 ×12 | LevelUpPoint, Vida, Vida máx., Mana, Mana máx., BP, BP máx., Força, Agilidade, Vitalidade, Energia, Liderança (sem o corte em 65000) |
| 108 | u16 | Nível |
| 112 | u32 | `obj+0x2500` (ranking custom "Patente", `F3 E7`) |

`MaxFruitAddPoint`/`MaxFruitSubPoint` vêm da tabela `CFruit` (`0x47c350`): por nível L (1..1000), começando em 2, a cada L múltiplo de 10 soma `2 + v/400` (classes comuns), `2 + v/700` (MG) ou `2 + v/500` (DL), com `v = 33 + 3(L-1)`.

Após o `F3 03` o GS envia `C4 F3 10 count` + 8 bytes por item (slot + item de 7 bytes, `0x49b3f0`), `C1 F3 11 count(u16)` + 3 bytes por magia (slot, `idx & 0xFF`, `(idx/255)&7 | nível<<3`, `0x5019d0`), o próprio `C2 12` (count 1, para si e para o viewport, `0x5167b0`) e os pedidos aos subsistemas (opções, amigos, quest, guild…).

## 5. Movimento, ação e ataque (observado)

### `D3` — andar (cliente → GS), handler `0x4d8b80`

| Off | Campo |
|---|---|
| 3 | X inicial |
| 4 | Y inicial |
| 5 | `(dir0 << 4) \| pathCount` |
| 6.. | nibbles de direção (2 por byte, nibble alto primeiro) |

- Tabela de direções (`0x5683d4`, pares `int16 dx,dy`): `0(-1,-1) 1(0,-1) 2(1,-1) 3(1,0) 4(1,1) 5(0,1) 6(-1,1) 7(-1,0)`.
- Rejeita se: intervalo desde o último movimento `< 100 ms`; destino a mais de 15 células da posição atual (X ou Y); célula bloqueada (atributos 4/8 do terreno). Se bloqueado, reenvia posição (`FUN_00511cb0`).
- Broadcast para o viewport: `C1 08 D3 [index BE 2] [X] [Y] [dir<<4]`.

### `DF` — corrigir posição, handler `0x4d80d0`

`[3] X [4] Y`; aceito se dentro de ±15 da posição atual. Resposta/broadcast `C1 07 DF …`.

### `18` — ação/animação, handler `0x4d8300`

`[3] dir [4] action [5..6] alvo`. Ações `0x80/0x81/0x82` definem estado especial (`obj+0x271 = 2/3/4`). Broadcast `C1 09 18 [index BE] [dir] [action] [alvo 2]`.

### `D7` — ataque físico, handler `0x410a30` (`CAttack`)

`[3..4] alvo BE`, `[5] action`, `[6] dir`. Exige alvo vivo no mesmo mapa (`obj+0x1ad`). Distância máxima para jogadores: **3** células (classe ≠ 2) ou **6** (classe 2 = Elf).

### `10` — ataque de skill em vários alvos, handler `0x500b70` (`CSkillManager`)

`[3] skill (índice na lista de magias)`, `[7] count (≤5)`, `[8..]` alvos de 3 bytes (`index BE 2 + 1 byte`). Só skills de tipo 5, 8, 9, 10, 12, 13, 14, 24, 47, 50, 52, 55, 78 (campo `+6` do registro de skill). Para jogadores, contador de hits por uso (`obj+0x38e`, máx. 5).

### `0E` — "live client" / anti-speedhack, handler `0x4d7e30`

`[4] TickCount u32`, `[8] PhysiSpeed u16`, `[10] MagicSpeed u16` (**inferido** pelas strings `Client Latency Hack Detected` / `Client Speed Hack Detected`).

### `00` — chat, `02` — whisper

- `00`: `[3..12] nome`, `[13..72] mensagem (60)`; broadcast `C1 49 00`. Log `[General]`.
- `02`: `[3..12] destinatário`, `[13..] mensagem`; log `[Whisper]`.

## 6. Viewport (observado)

### `C2 hi lo 12 count` — jogadores no viewport, builder `0x515160`

Entrada de **44 bytes**:

| Off | Campo |
|---|---|
| 0–1 | índice BE (bit 7 do byte alto ligado se `obj+0x26c == 1` e não teleportando) |
| 2 | X |
| 3 | Y |
| 4–21 | CharSet (18 bytes; ver [09](09-itens-e-inventario.md)); os 3 bits baixos do byte 0 recebem `obj+0x271` (estado de ação) |
| 22–23 | padding |
| 24–27 | ViewSkillState (u32) |
| 28–37 | Nome (10) |
| 38 | TX (destino) |
| 39 | TY |
| 40 | `(dir << 4) \| (pkLevel & 0xF)` (pk = 6 em situações custom, `FUN_00443f00`) |
| 41–43 | padding (alinhamento da struct; **resolve U-PROTO-04**) |

Ciclo do viewport (timer de 1 s, `FUN_0050a020`): para cada objeto, remoção (`0x513e60`: alvo saiu do jogo, morto com estado 1, teleportando, estado 8/0x20 ou fora do quadrado ±`ViewRange` do `MapManager`, padrão 12) e adição (`0x514700` jogadores varrem a tabela inteira; `0x514860` monstros varrem a partir de `0x5014`), mantendo as listas "o que vejo" (`+0x3b8`) e "quem me vê" (`+0x3bc`); depois o envio (`0x50dbf0`): `C1 size 14 count` + índices BE dos removidos (tipos 1–3), `C2 12` dos jogadores novos, `C2 13` dos monstros novos, itens, e os novos passam a visíveis. Broadcasts (`0x512730`) vão para as entradas de tipo 1 da lista "quem me vê".

### `C2 hi lo 13 count` — monstros/NPCs, builder `0x515510`

Entrada de **16 bytes** (`0x515510`): `[0..1]` índice BE (bit 7 = novo; bit 6 = teleportando, só quando novo), `[2..3]` classe u16 BE, `[4..7]` estado de skills de visualização `obj+0x274` u32 **LE**, `[8..11]` `X Y TX TY`, `[12]` `dir<<4 | (obj+0x19c & 0xF)`, `[13..15]` padding. **U-PROTO-05 resolvida.**

Demais pacotes de viewport (destino/remoção, itens no chão) aparecem no catálogo da seção 8.

## 7. Tabela de opcodes cliente → GameServer (observado)

Dispatcher: `ProtocolCore` em `GameServer.exe 0x4d6690`. Antes do dispatch, para jogadores (`obj+0x6c == 1`) e head ≠ `0xFA`, `HackPacketCheck` (`0x488070`) valida contagem, serial e criptografia por head (`Data/Hack/HackPacketCheck.txt`).

O "módulo" é a classe cujo singleton é obtido antes da chamada (identificada pela `vftable` no construtor). "Significado" só é preenchido quando há evidência (strings, layout lido); o resto fica "a confirmar".

| Head | Sub | Handler | Módulo | Significado / evidência |
|---|---|---|---|---|
| 00 | — | 4d7720 | — | Chat geral (log `[General]`) |
| 02 | — | 4d7c10 | — | Whisper (log `[Whisper]`) |
| 04 | — | 445a20 | CCustomAttack | a confirmar |
| 06 | — | 4d7de0 | — | a confirmar |
| 0E | — | 4d7e30 | — | Live client / speed hack |
| 10 | — | 500b70 | CSkillManager | Skill multi-alvo (layout acima) |
| 18 | — | 4d8300 | — | Ação/animação |
| 19 | — | 500e70 | CSkillManager | Ataque com skill (layout a extrair) |
| 1B | — | 501220 | CSkillManager | a confirmar (cancelar skill?) |
| 1C | — | 4bf010 | CMove | Teleporte / move (responde `C3 08 1C`) |
| 1E | — | 501080 | CSkillManager | Skill de área com posição (`[3..10]`) |
| 22 | — | 4982e0 | CItemManager | Pegar item (respostas `C3 .. 22 FE/FF`) |
| 23 | — | 4988f0 | CItemManager | Dropar item (`C1 05 23`; string "SuperDrop") |
| 24 | — | 499410 | CItemManager | Mover item (`C3 0C 24 FF`) |
| 26 | — | 499a20 | CItemManager | Usar item |
| 30 | — | 4c2eb0 | CNpcTalk | Falar com NPC |
| 31 | — | 4c3170 | CNpcTalk | Fechar janela de NPC |
| 32 | — | 49a910 | CItemManager | Comprar em loja (`C1 0B 32 FF`) |
| 33 | — | 49adb0 | CItemManager | Vender (string "Dont Sell Item Reforced") |
| 34 | — | 49b080 | CItemManager | Reparar (`C1 08 34`) |
| 36 | — | 507cf0 | CTrade | Pedir trade (bloqueado em evento) |
| 37 | — | 507f80 | CTrade | Responder trade |
| 3A | — | 5082d0 | CTrade | Zen no trade |
| 3C | — | 508460 | CTrade | Confirmar trade |
| 3D | — | 5087f0 | CTrade | Cancelar trade |
| 3F | 01,02,03,05,06,07 | 4d4ab0… | CPersonalShop | Loja pessoal (06 = comprar; log `SellPesonalShopItem`) |
| 40 | — | 4d2970 | CParty | Pedir party (bloqueado em evento) |
| 41 | — | 4d2c80 | CParty | Responder party |
| 42 | — | 4d2e80 | CParty | Lista da party |
| 43 | — | 4d3100 | CParty | Sair/expulsar |
| 50,51,52,53,54,55,57,61,66 | — | 483580… | CGuild | Guild (55 = criar, log `Guild Create error`) |
| 81,82,83 | — | 517390… | CWarehouse | Baú |
| 86,87,88 | — | 42ebd0… | CChaosBox | Chaos mix (88 = executar, `C1 10 86…`) |
| 90 | — | 4701f0 | CDevilSquare | Entrar no Devil Square |
| 91 | — | 4d8430 | — | a confirmar |
| 95,96,97 | — | 461920… | CGoldenArcher | Evento Golden Archer |
| 9A | — | 4174b0 | CBloodCastle | Entrar no Blood Castle |
| A0, A2 | — | 4ddb20, 4ddb70 | CQuest | Quest (string "Quest de Marlon concluída! Combo do BK liberado.") |
| A7, A9 | — | 4d86c0, 4d8730 | — | a confirmar (pets?) |
| AA,AB,AC | — | 477890… | CDuel | Duelo |
| AF | 01,02 | 4338c0… | CChaosCastle | Chaos Castle |
| B0 | — | 5012a0 | CSkillManager | a confirmar |
| B1 | 01 | 4d89e0 | — | a confirmar |
| B2 | 00–09,10,12,1B,1C,1D,1F | 4203b0… | CCastleSiege | Castle Siege (07 = upgrade de NPC) |
| B3,B4,B5,B9(02,05) | — | 4219b0… | CCastleSiege | Castle Siege |
| B7 | 01,04 | 42a460… | CCastleSiegeWeapon | Armas de cerco |
| BC | 00,01 | 4a1170… | CJewelMix | Mix de pacotes de joias |
| C0–C5,C7–CB | — | 460a90… | CFriend | Amigos e mail |
| D3 | — | 4d8b80 | — | Andar |
| D7 | — | 410a30 | CAttack | Ataque físico |
| DF | — | 4d80d0 | — | Corrigir posição |
| E1,E2,E5,E6,E9,EB | — | 484a40… | CGuild | Guild (alianças/hostilidade) |
| F1 | 01, 02 | 4d9360, 4d9560 | — | Login, logout |
| F2 | 05 | 451040 | cPackage (custom) | a confirmar |
| F3 | 00,01,02,03 | ver §4 | — | Personagens |
| F3 | 06 | 4d98f0 | — | a confirmar (responde `C1 30 F3 06`) |
| F3 | 09 | 441730 | — | Verificação de cliente/HWID (log `Verified client`, `Invalid Hardware ID`) |
| F3 | 0A, 0B | 447380, 447800 | CCustomAttack | `/attack` (auto-attack; mensagens em PT) |
| F3 | 12, 30 | 4d9bb0, 4d9c10 | — | a confirmar |
| F3 | F5, F6 | 44c610, 44c830 | CCustomJewelBank | Banco de joias |
| F3 | F7 | 518170 | CWarehouse | a confirmar |
| FA | 00/02,05,07,08,0E,10,12 | 4dc890… | CCustomXShop | Cash shop custom (05 = preço GM; 0E = recarga PIX; 12 = `CreationGold.txt`). Tamanhos fixos verificados: 05=12, 07=16, 08=6, 0E=4, 10=198, 12=5 |
| FB | 02 | 45a7c0 | (classe não identificada) | a confirmar |
| FB | 16 | 44e570 | CLuckyWheel | Roleta da sorte |

## 8. Catálogo de pacotes GameServer → cliente/DS/JS (observado)

Extraído das constantes de cabeçalho gravadas em memória (`0xHHSSC1` etc.) em todo o GameServer. Lista os pares `(tipo, head, sub)` encontrados; o mesmo head pode ser cliente ou servidor interno (os heads `0x80`, `0x81`, `0x92`, `0x9B`, `0xA0`, `0xB0`… com sub também são usados na conversa GS↔DS). O mapeamento campo a campo de cada um é trabalho da fase 2 do roadmap.

```
C1 00 | C3 00 | C1 05 | C2 05 | C1 07 | C1 0C | C1 0F {02-06,20-23}
C2 12 | C1/C2 13 | C1 14 | C3 1C {--,FF} | C1 1E | C2 1F | C2 20 | C2 21
C3 22 {FE,FF} | C1 23 | C3 24 FF | C1 26 | C1 27 FF | C1 28 FF | C2 2A | C1 2A 08 | C1 2C 12
C2 30 | C3 30 {02-07,09,0C,0D,11-13,15,16} | C2 31 | C1 32 FF | C1 3D {--,03}
C2 3F | C1 3F {02,03,06,08,10,12} | C1 42 {--,01} | C1 44 | C2 45 | C3 46 | C3 4D
C2 52 | C2 65 | C2 67 | C2 68
C1 80 {01,03-0E,10,12,16-1B} | C1 81 01 | C1 92 {01-06,0A-0D} | C1 94 {01,30}
C1 9B {02,03,05-0A} | C3 9C FF | C1 A0 {01-0C} | C1 AF 01 | C1 B0 {01-03,05-08}
C1 B1 01 | C1 B2 {01-09,10-14,16-18,1A,1B,1D,1E,20} | C2 B3 | C2 B5 | C1 B7 {01,02}
C1 B8 01 | C1 B9 03 | C1 BC 01 | C2 C0 | C1 C0 01 | C1 D5 {01,02} | C3 E9 83 | C1 EB 01
C1 F1 01 | C3 F1 02 | C1 F3 {01,02,05-08,11,13,14,20,30,40,E0,E1,E3,E4,E8,EB,F5,F9}
C3 F3 {03,04} | C1 F7 {04,05,06} | C1 FA {03,04,06,09,0F,11} | C1 FB {16,17,18,FC}
```

Builders já localizados: `C2 12` `0x515160`, `C2 13` `0x515510`/`0x516a90`, `C2 20` (itens no viewport) `0x515b00`, `C3 F3 03` `0x473a10`, `C3 F3 04` `0x4db350`, `C2 30` (salvar personagem → DS) `0x475c30`, `C2 31` (salvar inventário → DS) `0x476230`.

## 9. Recomendações de implementação

- Tipos `Frame{Kind, Head, Sub, Payload}` e codecs gerados a partir de uma tabela declarativa (YAML/Go structs com tags de offset), para que cada layout documentado aqui vire teste.
- Testes "golden": gravar sessões reais com o servidor original (pcap ou proxy TCP) e reproduzir contra a implementação Go.
- Encapsular XOR32/SimpleModulus/serial dentro da sessão; handlers recebem sempre o pacote em claro no formato C1/C2.
