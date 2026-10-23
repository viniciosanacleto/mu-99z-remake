# 01 — Arquitetura alvo (serviço Go único)

## 1. O que o original faz hoje (observado)

Os quatro executáveis analisados compartilham o mesmo núcleo de rede:

- **Aceite/IO**: `WSASocket` + `CreateIoCompletionPort` + N threads de worker (strings `[SocketTCP] CreateIoCompletionPort()…`, `SetThreadPriority`).
- **Fila de mensagens**: cada pacote completo é copiado para um buffer de `0x808` bytes e inserido numa fila circular com capacidade **0x800 (2048) mensagens**, protegida por `CRITICAL_SECTION`; um semáforo acorda **uma única thread de processamento** (ConnectServer `FUN_00406dc0` → `FUN_004039d0`; GameServer `FUN_005049c0`). Toda a lógica de jogo roda nessa thread ⇒ não há concorrência na lógica.
- **Buffer de recepção**: 0x800 bytes por conexão no CS/JS/DS; **0x2000 (8192) bytes** no GameServer (`FUN_005049c0`). Pacote maior que o buffer ⇒ desconexão.
- **Envio**: buffer de envio de 0x2000 bytes + fila secundária de 0x4000 bytes por conexão (`FUN_00504f00`); estouro ⇒ desconexão.
- **Logs** diários em `.\LOG\<categoria>\YYYY-MM-DD.txt` (`.\%s\%04d-%02d-%02d.txt`), crash dumps via `MiniDumpWriteDump`.

### Tabela de objetos do GameServer (observado)

- Array global de ponteiros de objeto `DAT_0107a7dc[index]`.
- Índices de **jogador**: `0x53FC..0x57E3` (21500–22499 ⇒ **1000 slots**). Verificação em `FUN_00504f00`: `0x53fb < index < 0x57e4`.
- Índices `< 21500`: monstros, NPCs e objetos de evento (o limite inferior exato por tipo é **inferido** e precisa ser confirmado — ver incógnitas).
- Estado da conexão em `obj+4`: `1` = conectado (pré-login), `2` = logado (seleção de personagem), `3` = no jogo. Vários handlers checam `== 3` (jogo) ou `== 2` (seleção) — observado em `0x4d9360`, `0x4d95c0`, `0x500b70`.
- `obj+0x6c` (short) = tipo do objeto (`1` = jogador).
- Viewport: 75 entradas de 4 bytes por objeto (laço `while (i < 300)` passo 4 em `0x515160`, `0x4d8300`).

## 2. Princípios do serviço novo

1. **Um processo, quatro módulos** (`connect`, `auth`, `persistence`, `game`) + pacote compartilhado `protocol`.
2. **Compatibilidade de fio com o cliente original** para todas as portas expostas ao cliente (44405 e 55900+). As portas internas (55557, 55960, 55970) **somem**.
3. **Lógica de jogo determinística**: preservar o modelo "uma thread de lógica" do original. Em Go: uma goroutine `world` por *instância de servidor de jogo* (code 0, code 19…) consumindo um canal de eventos; IO em goroutines por conexão.
4. **Dados de jogo continuam em arquivos texto** (mesmos formatos de `Data/`), carregados na inicialização e recarregáveis (o original tem comandos "Reload …"). Isso permite reaproveitar o conteúdo existente sem conversão.
5. **PostgreSQL** substitui SQL Server; stored procedures viram métodos de repositório com transações explícitas e **queries parametrizadas** (o original monta SQL com `sprintf` e só filtra espaço/aspas — ver [04](04-auth-join-server.md)).

## 3. Layout proposto do código

```
apps/server/
  cmd/muserver/main.go          # bootstrap, flags, carga de config
  internal/config/              # leitura de .ini/.dat (formato GetPrivateProfile) e Data/*.txt
  internal/protocol/
    frame/                      # C1/C2/C3/C4: parser incremental, limites de tamanho
    crypto/                     # XOR32, SimpleModulus (Enc/Dec .dat), Bux (FC CF AB)
    packets/                    # structs + encode/decode por opcode (cliente↔GS, cliente↔CS)
  internal/net/                 # listeners TCP, sessão, fila de envio, anti-flood
  internal/connect/             # módulo ConnectServer
  internal/auth/                # módulo JoinServer (contas, sessões, VIP, move entre servidores)
  internal/persistence/         # repositórios PostgreSQL + migrations
  internal/game/
    world/                      # loop, timers, instâncias de mapa, viewport
    object/                     # Object, Player, Monster, NPC
    combat/ skill/ item/ inventory/ shop/ chaosmix/
    party/ guild/ trade/ pshop/ duel/ friend/ quest/ warehouse/
    event/ (bloodcastle, devilsquare, chaoscastle, castlesiege, crywolf, kalima, illusiontemple, goldenarcher, invasion, custom/*)
    custom/ (xshop, jewelbank, attack(offline), luckywheel, marriage, reset…)
    command/                    # comandos de chat (/move, /reset…)
    script/                     # Lua (gopher-lua) — ver seção 6
  migrations/                   # SQL PostgreSQL
```

## 4. Mapeamento de responsabilidades

| Original | Contrato que precisa existir na reescrita |
|---|---|
| GS→JS `0x01` login | `auth.Login(account, password, ip, hwid, serverCode) (AccountSession, ResultCode)` |
| GS→JS `0x02` logout | `auth.Logout(account)` |
| GS→JS `0x03`/`0x04` troca de servidor | `auth.BeginServerMove(...)` / `auth.CompleteServerMove(authCodes)` — vira transferência de sessão em memória entre instâncias |
| GS→JS `0x05`, `0x10`, `0x11` | `auth.GetAccountLevel`, `auth.SetAccountLevel` |
| GS→JS `0x12` | `auth.SetLock(account, lock)` |
| GS→JS `0x20` | contadores de usuários (alimenta `connect`) |
| JS→GS `0x30` | `game.KickAccount(account)` (login duplicado) |
| GS↔DS `0x00..0xF7` | Interfaces de repositório (ver [05](05-persistencia-data-server.md)) |
| GS→CS UDP `0xA1` | `connect.ReportServer(code, users, maxUsers)` direto em memória |
| JS→CS UDP `0xA2` | `connect.ReportAuthQueue(size)` |

## 5. Concorrência e tempo

- O original usa `GetTickCount()` (ms) para todos os cooldowns observados: ex. movimento ignora pacotes com intervalo `< 100 ms` (`0x4d8b80`: `99 < now - last`), cooldown de 1000 ms em mapas de evento 18–23 (`0x4d80d0`).
- Recomenda-se um relógio monotônico injetável (`clock.Now()`), para testes determinísticos dos timers.
- Persistência: as chamadas ao banco no original são síncronas dentro do DataServer (processo separado), então o GameServer nunca bloqueia. Na reescrita, operações de banco devem sair da goroutine `world` (worker pool) e responder via evento — nunca bloquear o loop.

## 6. Scripting Lua (observado)

O GameServer embute **Lua 5.2.0** (string `$LuaVersion: Lua 5.2.0`). `Data/Script/` contém:

- `ScriptMain.lua`, `System/ScriptCore.lua` — núcleo.
- `Character/CalcCharacter.lua` — fórmulas de atributos (ativado por `CalcLuaScriptSwitch` em `GameServerInfo - Character.dat`; **está `0`** nesta instalação ⇒ código nativo é usado).
- `Script/*.lua` — mensagens de boas-vindas, drop de boss, nome da janela.

Na reescrita: usar `github.com/yuin/gopher-lua` (compatível com Lua 5.1; validar diferenças 5.2) ou portar os scripts para Go. As funções expostas ao Lua (`GetObjectTotalStrength`, `GetObjectClass`, `GET_ITEM`, …) definem a API mínima a reproduzir.

## 7. Configuração

O original lê `.ini` com `GetPrivateProfileIntA`/`GetPrivateProfileStringA` (seção `[GameServerInfo]`, `[ConnectServerInfo]`…). Manter um leitor compatível permite usar os arquivos existentes sem alteração. Chaves observadas no binário do GameServer (lista parcial): `ServerName`, `ServerCode`, `ServerPort`, `ServerVersion`, `ServerSerial`, `ServerMaxUserNumber`, `ServerMin/MaxLevel`, `ServerMin/MaxReset`, `DataServerAddress/Port`, `JoinServerAddress/Port`, `ConnectServerAddress/Port`, `MaxConnectionPerIP`, `MaxConnectionPerHID`, `Write*Log`, `AntiFlood*`, `CheckSpeedHack*`, `CheckLatencyHack*`, `CheckAutoPotionHack`. Inventário completo em [11-arquivos-de-configuracao.md](11-arquivos-de-configuracao.md).

## 8. Observabilidade

Substituir os logs em arquivo por log estruturado (slog) com as mesmas categorias do original (`CONNECTION_LOG`, `HACK_LOG`, `LOG_ACCOUNT`, chat, trade, chaos mix, comandos, cash shop) e métricas (usuários por servidor, fila de pacotes, latência de banco).
