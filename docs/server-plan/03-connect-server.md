# 03 — Módulo `connect` (substitui o ConnectServer)

Fonte: `ConnectServer.exe` (SHA-256 `1b415695aa82…`, 385 funções), `ConnectServer.ini`, `ServerList.dat`, `BlackList.txt`.

## 1. Configuração (observado)

`ConnectServer.ini`, seção `[ConnectServerInfo]`:

| Chave | Valor atual | Uso |
|---|---|---|
| `ConnectServerPortTCP` | 44405 | porta dos clientes |
| `ConnectServerPortUDP` | 55557 | heartbeats de GS e JS |
| `MaxConnectionIdle` | 60 | segundos sem pacote ⇒ desconecta (`[%s] Client disconnected, timeout.`) |
| `MaxConnectionPerIP` | 1 | conexões simultâneas por IP |
| `MaxPacketPerSecond` | 10 | anti-flood (`[DataRecv] Packets exceeded`) |

`ServerList.dat` (texto):

```
// ServerCode   ServerName       IpAddress           Port        Visible
   0            "Servidor 01"     "192.168.1.1"        55900       1
   19           "EventServer"    "192.168.1.1"        55919       0
end
```

Comentário do próprio arquivo: `ServerCode: 0~3: Gold-NoPvP | 4: Gold~PvP | 5~6: No-PvP | 7~18: PvP`. Nome até 20 caracteres.

`BlackList.txt`: lista de IPs bloqueados (log em `.\LOG\HACK_LOG`).

Classes (RTTI): `CClientManager`, `CServerList`, `CSocketManager`, `CSocketManagerUdp`, `CQueue`, `CLog`, `CServerDisplayer`.

## 2. Protocolo com o cliente (observado)

Ver diagrama em [02 §3](02-protocolo-de-rede.md#3-sequência-de-conexão).

| Direção | Pacote | Layout |
|---|---|---|
| S→C | hello | `C1 04 00 01` (enviado ao aceitar, `FUN_004025a0`; em seguida `F3 EA`) |
| S→C | `F3 EA` (custom) | `C2 size F3 EA count(u8)`, depois `count` entradas de 34 bytes: `code u16 LE` + `name char[32]` (todo servidor com heartbeat, **visível ou não**; `FUN_00403870`/`FUN_00405d00`) |
| C→S | `F4 06` | pedido de lista |
| S→C | `F4 06` | `C2 size F4 06 count u16 BE`, entradas de 4 bytes: `code u16 LE`, `load u8 = users*100/maxUsers`, `0xCC` (padding) — `size = 7 + 4·count` (`FUN_00403740`) |
| C→S | `F4 03` | `[4] code` (**u8**, `FUN_004035f0`) |
| S→C | `F4 03` | `C1 16 F4 03 ip char[16] port u16 LE` |

Regras observadas no dispatcher (`FUN_00407930`; o endereço `0x4039d0` citado antes é um destrutor de `CCriticalSection`):

- Só processa pacotes com head `0xF4`; subs `0x03` e `0x06`. Qualquer outro head é ignorado. O sub é lido em `packet[3]` independentemente do tipo, então um `C2 F4 …` nunca casa.
- A lista e o endereço só são servidos se o **JoinServer estiver vivo** (heartbeat recebido) **e** a fila do JoinServer for `< 101` (`DAT_0040f5ac < 0x65`). Caso contrário o cliente não recebe resposta.
- Um servidor só entra na lista se `Visible = 1` **e** houver heartbeat recente (`+0x44 != 0 && +0x48 != 0`).
- Índice de cliente limitado a 100 conexões simultâneas (`index < 100`).

- Com o JoinServer fora do ar ou com fila ≥ 101, `F3 EA` e `F4 06` são enviados com contagem 0 e `F4 03` não tem resposta.
- O anti-flood (`FUN_00407240`) conta recepções por cliente e desconecta quando a contagem chega a `MaxPacketPerSecond` (`limite <= contagem`). O zeramento do contador a cada segundo é **inferido** pelo nome da chave.
- `BlackList.txt` desta instalação contém `"127.0.0.1"`: conexões locais são recusadas com a configuração original.

Implementação: `apps/server/internal/connect` (testes com os layouts acima).

## 3. Heartbeats UDP (observado)

Porta 55557, mesmo enquadramento C1/C2, buffer de 0x1000 bytes (`FUN_00407c60`).

| Head | Origem | Conteúdo | Efeito |
|---|---|---|---|
| `0xA1` | GameServer | `[4] serverCode`, `[8] userCount`, `[12] maxUserCount` (u32 cada, **inferido** pelos offsets lidos) | marca o servidor vivo, guarda `GetTickCount()` e contadores |
| `0xA2` | JoinServer | `[4] queueSize` (u32) | marca o JS vivo |

Logs: `[SocketUDP] GameServer connected [%s] [%s:%d][%d]`, `... disconnected`, `JoinServer connected/disconnected`. A queda é detectada por timeout (valor não extraído — incógnita U-CS-01).

## 4. Desenho na reescrita

- `connect.Registry` em memória com as instâncias de jogo do próprio processo; `ReportServer` é chamado pelo módulo `game` a cada tick (não há mais UDP).
- Manter o listener TCP 44405 com o mesmo comportamento de anti-flood, idle-timeout e limite por IP.
- `ServerList.dat` continua sendo a fonte de nome/IP/porta/visibilidade; IP público configurável.
- O endereço devolvido em `F4 03` é o listener de jogo da instância correspondente (ex. código 0 → 55900, código 19 → 55919).
- O "load" é `users*100/max` (u8); com `max = 0` evitar divisão por zero (o original não protege).
