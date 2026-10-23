# 04 — Módulo `auth` (substitui o JoinServer)

Fonte: `JoinServer.exe` (SHA-256 `7698c4a6e8fc…`, 164 funções de usuário), `JoinServer.ini`, `AllowableIpList.txt`.

## 1. Configuração (observado)

| Chave (`[JoinServerInfo]`) | Valor | Uso |
|---|---|---|
| `JoinServerODBC` | MuOnline | DSN |
| `JoinServerUSER` / `JoinServerPASS` | (vazio) | credenciais ODBC |
| `JoinServerPort` | 55970 | porta para GameServers |
| `ConnectServerAddress` / `ConnectServerPort` | 127.0.0.1 / 55557 | destino do heartbeat UDP `0xA2` |
| `MaxConnectionPerIP` | 0 (sem limite) | limite de contas por IP |
| `MaxConnectionPerHID` | 0 | limite por hardware ID |
| `MD5Encryption` | 0 | senha em texto (0) ou MD5 (1) |

`AllowableIpList.txt`: IPs de GameServers autorizados a conectar.

O JoinServer não fecha enquanto houver GameServers conectados ("Can not close JoinServer with opened GameServer… Danger of data loss!").

## 2. Opcodes GS ↔ JS (observado, dispatcher `FUN_00404850`)

| Head | Direção | Handler | Função | Resposta |
|---|---|---|---|---|
| `0x00` | GS→JS | `FUN_00404970` | registro do GameServer: `[4] serverCode u16`, `[6] nome char[50]`, `[0x38] porta u16` | `C1 08 00 …` |
| `0x01` | GS→JS | `FUN_00404a50` | login de conta | `C1 3C 01 …` |
| `0x02` | GS→JS | `FUN_00405230` | logout (executa `WZ_DISCONNECT_MEMB`) | `C1 12 02` |
| `0x03` | GS→JS | `FUN_004054b0` | pedido de troca de servidor (map server move) | `C1 38 03` |
| `0x04` | GS→JS | `FUN_00405760` | autenticação da troca (4 códigos u32 em `+0x20..+0x2c`) | `C1 58 04` |
| `0x05` | GS→JS | `FUN_00405ba0` | consultar nível de conta (`WZ_GetAccountLevel`) | `C1 28 05` |
| `0x10` | GS→JS | `FUN_00405e10` | a confirmar (mexe em sessão) | — |
| `0x11` | GS→JS | `FUN_00405f00` | gravar nível de conta (`WZ_SetAccountLevel '%s','%d','%d'`) | — |
| `0x12` | GS→JS | `FUN_004061e0` | `UPDATE memb_info SET Lock = %d WHERE memb___id = '%s'` | — |
| `0x20` | GS→JS | inline | contagem de usuários do GS (`[4] users u16`, `[6] max u16`) | — |
| `0x30` | GS→JS | `FUN_00405fc0` | a confirmar | — |
| `0x30` | JS→GS | `FUN_00406120` | **derrubar conta** (login duplicado): `C1 12 30 …` | — |
| `0x02` | JS→GS | `FUN_00406060` | aviso de desconexão `C1 12 02` | — |

## 3. Fluxo de login (`FUN_00404a50`, observado)

Entrada (GS→JS `0x01`): `[4] index u16`, `[6] conta char[11]`, `[0x11] senha char[11]`, `[0x1c] IP char[16]`, `[0x2c] HWID char[45]`.

Passos e código de resultado (`local_5d`, enviado em `C1 3C 01`):

1. Conta contém espaço, `"` ou `'` ⇒ **2** (inválida). *(Filtro anti-SQL-injection do original.)*
2. Conta já está online (tabela de contas em memória) ⇒ **3** (já conectado). Se a senha enviada for igual à senha da sessão existente, o JS envia `0x30` ao GS dono da sessão para derrubá-la (`FUN_00406120`).
3. Limite por IP/HWID estourado (`FUN_004019c0`) ⇒ **7**.
4. Mais de 10000 contas online ⇒ **4** (servidor cheio).
5. `SELECT memb___id,memb__pwd FROM MEMB_INFO WHERE memb___id='%s'`
   - sem linha ⇒ **2**;
   - `MD5Encryption=0`: compara texto da senha; diferente ⇒ **0** (senha errada);
   - `MD5Encryption=1`: lê `memb__pwd` binário e compara com MD5 (contexto iniciado com `67452301 EFCDAB89 98BADCFE 10325476`, função `FUN_00406710(senha, conta…)`). A forma exata de "salgar" com a conta é **a confirmar** (U-AUTH-01).
6. `SELECT sno__numb,bloc_code FROM MEMB_INFO …` (personal ID de 13/18 chars e bloqueio).
7. `EXEC WZ_GetAccountLevel '%s'` ⇒ `AccountLevel`, `AccountExpireDate` (string 20).
8. `SELECT top 1 Lock FROM MEMB_INFO …` (coluna `Lock` adicionada por `DB/Query/Rodar.sql`; padrão 0).
9. `EXEC WZ_CONNECT_MEMB '%s','%s','%s'` (conta, nome do servidor, IP) — grava `MEMB_STAT`.
10. Resultado **1** (ok). A sessão em memória guarda: conta, senha, IP, HWID, índice no GS, código do servidor, nível/expiração, bloc_code, lock.

Log: `[AccountInfo] Account connected (Account: %s, IpAddress: %s, GameServerCode: %d)`.

Resposta `C1 3C 01` contém: `index u16`, conta (10), resultado, `bloc_code`, `sno__numb` (13), `AccountLevel u16`, `AccountExpireDate` (20), `Lock`. Offsets exatos dos campos a validar (U-AUTH-02).

> O mapeamento destes códigos para o resultado que o cliente recebe em `F1 01` não foi rastreado (U-AUTH-03).

## 4. Logout (`FUN_00405230`)

`EXEC WZ_DISCONNECT_MEMB '%s'` (grava `ConnectStat=0`, `DisConnectTM`, soma `OnlineHours`). Log `Account disconnected`.

## 5. Troca de servidor (`0x03`/`0x04`)

Usada quando o jogador entra em um mapa hospedado por outro código de servidor (ver `MapServerInfo.txt`, [07](07-mundo-e-objetos.md)). `0x03` gera 4 códigos de autenticação; `0x04` (vindo do servidor de destino) valida conta + 4 códigos e refaz as consultas de `bloc_code`, nível e `Lock`. Resultado `4` = falha de autenticação. Na reescrita isso vira uma transferência de sessão **dentro do mesmo processo**, mas o cliente ainda precisa receber o pacote de reconexão em outra porta — manter as duas portas e os códigos de autenticação.

## 6. Desenho na reescrita

```go
type AuthService interface {
    Login(ctx, req LoginRequest) (LoginResult, *Session)
    Logout(account string)
    BeginServerMove(account string, target ServerCode, mapNo, x, y int) (MoveTicket, error)
    CompleteServerMove(account string, codes [4]uint32) (*Session, error)
    AccountLevel(account string) (level int, expires time.Time, err error)
    SetAccountLevel(account string, level, days int) error
    SetLock(account string, lock int) error
}
```

Requisitos novos (não existem no original):

- Queries parametrizadas; remover o filtro de caracteres como única defesa.
- Senhas: aceitar `plain`/`md5` legados para migração, mas armazenar `argon2id` ao primeiro login bem-sucedido (o cliente sempre envia a senha em claro + Bux, então o servidor pode re-hash).
- Sessão única por conta garantida por mapa em memória + coluna `memb_stat.connect_stat` (para recuperação após crash, zerar tudo no boot como o original faz implicitamente).
