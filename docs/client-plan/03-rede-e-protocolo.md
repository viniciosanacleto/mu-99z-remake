# 03 — Rede e protocolo (lado cliente)

O contrato de fio completo está em [../server-plan/02-protocolo-de-rede.md](../server-plan/02-protocolo-de-rede.md). Aqui fica o que é específico do cliente.

## 1. Endereços e identidade (observado)

- O `Main.exe` traz hosts de fábrica (`connect.muonline.co.kr`, `connect.muonline.com`, `connection.muonline.com.tw`, `connect.muchina.com`, `connect.muonline.com.ph`); o `Main.dll` substitui pelo `IpAddress`/`IpAddressPort` de `MainInfo.ini` (via `License.wzt`). Padrão atual: `192.168.1.1:44405`.
- Versão e serial enviados no `F1 01`: `ClientVersion = 1.00.00`, `ClientSerial = A9kT3wX5rBz7fQ1u`. O `Main.exe` tem embutido em `0x6b7248` o valor codificado `32 32 34 38 3B` (decodifica, com `b[i]-(i+1)`, para `"10146"`) e o serial de fábrica `dlgurwocjswozizi`; ambos são sobrescritos pelo `Main.dll` (**inferido**: a substituição em memória não foi rastreada — U-NET-01).
- Logs de depuração do original: `[Connect to Server] ip address = %s, port = %d`, `[Virtual Connection] Connect IP : %s, Port : %d`, `[ReceiveServerConnect]`, `Failed to connect.`, `Connectting error.`, `Version dismatch - Join server.`.

## 2. Criptografia no envio/recepção

| Direção | Tipo | Tratamento no cliente |
|---|---|---|
| Cliente → servidor | C1/C2 | aplicar XOR32 inverso do que o servidor desfaz (ver servidor §2.1). Forma exata de cifrar: para `n = start+1 .. size-1`: `data[n] ^= data[n-1] ^ XorKey[n % 32]` usando o byte anterior **já cifrado** — **inferido** como inverso do decifrador observado; validar com captura (U-NET-02) |
| Cliente → servidor | C3/C4 | `[serial][head][corpo]` cifrado com SimpleModulus usando `Data\Enc1.dat`; contador serial do cliente incrementado por pacote C3/C4 |
| Servidor → cliente | C1/C2 | em claro |
| Servidor → cliente | C3/C4 | decifrar com `Data\Dec2.dat`; descartar o byte serial |
| Login | — | conta e senha com Bux `FC CF AB` |
| Ambos (só GameServer) | todos os bytes | cifra de fluxo: enviar `b = (b + K1) ^ K2`, receber `b = (b ^ K2) - K1`, chaves derivadas de `CustomerName` + `ClientSerial` (servidor: [02 §2.5](../server-plan/02-protocolo-de-rede.md#25-cifra-de-fluxo-do-socket-do-gameserver-observado)). XOR32 vem antes do SimpleModulus também em C3. Ordem confirmada no servidor; do lado do cliente é **inferida** (U-PROTO-08) |

A implementação de `CPacketManager` do `Main.dll` (classe presente no RTTI) é onde isso acontece; os detalhes do lado cliente não foram decompilados (U-NET-03). Quais heads o cliente envia cifrados (C3) é definido pelo servidor em `HackPacketCheck.txt` — o cliente precisa enviar exatamente como o original (ex. `F1 01` sempre C3).

## 3. Fluxo por cena

```
[Logo/Login]
  conecta CS (TCP 44405) ── recebe C1 04 00 01 e C2 F3 EA (nomes)
  envia F4 06 ─────────────── recebe lista {code, load%}
  usuário escolhe servidor ── envia F4 03 code ── recebe ip:porta ── fecha CS
  conecta GS ─────────────── recebe F1 00 {result, index, versão} (valida versão)
  envia C3 F1 01 {conta, senha, tick, versão, serial} ── recebe F1 01 result
[Character]
  envia F3 00 ── recebe lista de personagens
  F3 01 criar / F3 02 apagar
  envia F3 03 nome ── recebe C3 F3 03 (dados) + inventário, skills, viewport… (opcodes a identificar)
[Main/World]
  envia D3 (andar), D7 (atacar), 18 (ação), 10/19/1E (skills), 00/02 (chat), 22–26 (itens) …
  recebe 12/13/14/20/21 (viewport), D3/18 (broadcasts), 0E? (tempo), …
  ping periódico 0E {tick, physiSpeed, magicSpeed}
```

Os subcódigos de inventário/skills após o `F3 03` vêm do catálogo do servidor (`C1 F3 {05..08,11,13,14,…}`, `C3 F3 04`) e ainda precisam ser associados campo a campo (U-NET-04).

## 4. Reconexão

`ReconnectInfo.ReconnectTime = 300000` (5 min) no cliente; no servidor `ReconnectTime` e `Reconnect*Switch` em `Common.dat`, com estado guardado em `ReconnectData`. O cliente custom reconecta automaticamente após queda e o servidor restaura party/attack/loja (**inferido** pelos nomes das chaves; U-NET-05).

## 5. Requisitos para a implementação

- Socket TCP não bloqueante com parser incremental idêntico ao do servidor.
- Fila de envio com limite e coalescência; enviar `0E` em intervalo fixo (a extrair).
- Todas as estruturas de pacote vindas de um schema compartilhado com o servidor Go (gerar código para os dois lados).
- Modo "bot/headless" do cliente para testes automatizados do servidor.
