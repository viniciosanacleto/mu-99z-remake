# 15 — Proxy de captura e vetores golden

`apps/server/cmd/muproxy` é um man-in-the-middle TCP entre o `Main.exe` original e um ConnectServer ou GameServer (o original ou o `muserver`). É a ferramenta do bloco 14 (teste com o cliente original): permite comparar, byte a byte, o tráfego real com o que os codecs Go produzem. A lógica fica em `apps/server/internal/capture`.

## O que ele faz

- Repassa os bytes sem alteração, pacote a pacote, e decodifica uma cópia com os mesmos codecs do servidor:
  - **ConnectServer**: framing C1/C2 puro nos dois sentidos (`internal/protocol/frame`);
  - **GameServer, cliente → servidor**: cifra de fluxo, framing, C3/C4 (SimpleModulus com `Dec1.dat` + serial) e XOR32 (`gscodec.Server`);
  - **GameServer, servidor → cliente**: cifra de fluxo, framing e C3/C4 (SimpleModulus com `Dec2.dat` do cliente + serial) (`gscodec.Client`).
- A cifra de fluxo vem de `CustomerName`/`ServerSerial` do `GameServerInfo - Common.dat` (`FUN_00487a60`). As duas instâncias usam a mesma chave (`WizardTeam` / `A9kT3wX5rBz7fQ1u`).
- Grava um log com horário, uma linha por pacote:

  ```
  2026-10-23T02:19:08.141451 #1 game c2s C3 F1 01 len=49 serial=00 c131f101...
  2026-10-23T02:19:08.141670 #1 game s2c C1 F1 00 len=12 c10cf1000153fc3130343034
  ```

  Os campos são: sessão, protocolo, sentido, tipo no fio (C1..C4), head, sub, tamanho do pacote decodificado, serial (só C3/C4) e o hex do pacote decodificado. O pacote decodificado está sempre na forma C1/C2, sem o byte de serial e já sem o XOR32 no sentido cliente → servidor.
- Com `-golden <pasta>`, salva cada sessão (cada conexão TCP) como JSON, um arquivo de vetores golden:

  ```json
  {"version":1,"id":1,"kind":"game","client":"...","upstream":"...","start":"...",
   "stream_add":..,"stream_xor":..,
   "records":[{"t":123,"dir":"c2s","wire":"<bytes no fio>","plain":"<C1/C2 decodificado>","enc":true,"serial":0}]}
  ```

  `t` é o tempo desde o início da sessão, em µs. Quando o proxy reescreve um pacote, o `fwd` guarda os bytes realmente enviados; `wire` continua sendo o que o servidor mandou. `err` marca um pacote que não decodificou (a sessão continua).
- **Reescrita de endereço** (`-rewrite`): troca o IP/porta do GameServer em `C1 16 F4 03` do ConnectServer (`FUN_004035f0`: `ip[16]` no offset 4, porta u16 LE no 20) e em `C3 2C B1 00`, a troca de servidor de mapa (`FUN_004a1c10`, mesmos offsets). No lugar entra um listener do proxy em `-rewrite-host`, aberto na hora em porta livre, que encaminha para o endereço original. Assim a conexão seguinte do cliente também é capturada. O `B1 00` é recifrado com `Enc2.dat` e o mesmo serial. O SimpleModulus é determinístico, então um pacote sem mudanças volta idêntico (há teste para isso).
- Se o framing de um sentido se perder (header ou tamanho inválidos), o proxy registra o erro e passa a repassar os bytes crus desse sentido. A sessão não cai.

## Uso

Rodar a partir de `apps/server`. As chaves do cliente (`Enc1.dat`/`Dec2.dat`) foram copiadas para `apps/client/assets/Data/`.

```sh
# Proxy na frente do ConnectServer (porta 44405 do ConnectServer.ini), com reescrita
go run ./cmd/muproxy -listen 0.0.0.0:44405 -upstream 192.168.1.1:44405 \
    -rewrite -rewrite-host 192.168.1.50 -golden captures -log captures/packets.log

# Proxy direto num GameServer
go run ./cmd/muproxy -kind game -listen 127.0.0.1:55900 -upstream 192.168.1.1:55900 -golden captures
```

| Flag | Padrão | Uso |
|---|---|---|
| `-listen` | `127.0.0.1:44405` | endereço onde o cliente conecta |
| `-upstream` | (obrigatório) | servidor real |
| `-kind` | `connect` | `connect` ou `game` |
| `-data` | `data` | raiz dos arquivos do servidor no formato original |
| `-gs-config` | `config/gameservers/4.GameServer.json` | `CustomerName`/`ServerSerial` da instância (chave de fluxo) |
| `-server-keys` | `<data>/Data/Hack` | `Enc2.dat`, `Dec1.dat` |
| `-client-keys` | `../client/assets/Data` | `Enc1.dat`, `Dec2.dat` |
| `-log` | `-` (stdout) | arquivo de log (append) |
| `-golden` | vazio (desligado) | pasta dos JSON de sessão |
| `-rewrite` | falso | reescreve F4 03 / B1 00 |
| `-rewrite-host` | `127.0.0.1` | host anunciado ao cliente e usado nos listeners (até 15 caracteres) |

Para o cliente original passar pelo proxy:

1. Aponte o `Main.exe` para o `-listen` do proxy. O `Main.dll` lê o endereço do ConnectServer em `IpAddress`/`IpAddressPort` do `MainInfo.ini`, via `License.wzt`; o padrão é `192.168.1.1:44405` (veja `docs/client-plan/03-rede-e-protocolo.md`).
2. Com `-rewrite`, o `-rewrite-host` precisa ser alcançável pelo cliente, por exemplo o IP do Mac na LAN quando o cliente roda numa VM Windows.
3. Sem `-rewrite`, o cliente conecta direto no GameServer da lista. Nesse caso rode um segundo proxy `-kind game` e faça o endereço da lista apontar para ele.

O `BlackList.txt` original do ConnectServer (membro `BlackList` de `config/server.json`) bloqueia `127.0.0.1`. Com o `muserver` local, use `-blacklist` com uma tabela vazia, `{"rows": []}` (veja `13-progresso.md`).

## Vetores golden

- `capture.Replay(sessão, chaves)` refaz cada registro nos dois papéis, em ordem:
  - **cliente → servidor**: `gscodec.Server` decodifica `wire` e deve dar `plain`; `gscodec.Client` (com `SetSerial` do registro) recodifica `plain` e deve dar `wire`;
  - **servidor → cliente**: `gscodec.Client` decodifica e `gscodec.Server` recodifica.
  - Nas sessões de ConnectServer, só o framing é conferido.
- `gscodec.Server`/`Client` ganharam `SetSerial`, porque uma sessão capturada pode não começar no serial 0. Exemplo: o contador de C3 do cliente pode continuar após uma troca de servidor; isso ainda não foi confirmado.
- Para guardar uma captura como teste de regressão, copie o JSON para `apps/server/internal/capture/testdata/golden/`. `TestGoldenVectors` refaz todos os arquivos dessa pasta e é pulado enquanto ela estiver vazia.

## Testes

Todos usam só listeners em `127.0.0.1` dentro do processo, com o par `gscodec.Client`/`Server` fazendo o papel de cliente e servidor (`internal/capture/capture_test.go`, `cmd/muproxy/main_test.go`):

- sessão de GameServer com C1/C3/C2 nos dois sentidos, escrita byte a byte, decodificação, log, arquivo golden e `Replay`;
- `Replay` detecta `plain` ou serial adulterados;
- recifragem do `B1 00` volta idêntica;
- `F4 03` com e sem reescrita, com a conexão seguinte passando pelo listener criado;
- `B1 00` reescrito, com o cliente chegando ao segundo GameServer;
- passagem crua depois de um erro de framing;
- framing incremental por pedaços;
- flags e execução do comando.

## Incógnitas

- O serial C3 do cliente original após a troca de servidor (`B1 00` → nova conexão) começa em 0 ou continua? Uma captura vai responder.
- Como mudar o `IpAddress` empacotado no `License.wzt` sem a ferramenta original.
