# 18 — Teste de carga

Mede CPU e memória do `muserver` com 1, 10, 50 e 100 jogadores simultâneos, para dimensionar a hospedagem. Os jogadores são bots headless que falam o protocolo do cliente original.

## Peças

| Peça | Onde | O que faz |
|---|---|---|
| `muserver -seed-load N` | `cmd/muserver/seed.go` | Cria (ou reposiciona) as contas `load0001..` com senha `load` e um personagem cada (`ld0001..`, DW/DK/Elf alternados, nível 50, +150 em cada atributo) no campo de Spider/Budge Dragon a leste de Lorencia (x 176–200, y 108–132), em células sem bloqueio e fora da zona segura. Um único hash argon2 serve todas as contas; cada login ainda roda o argon2 |
| `muserver -metrics-addr :6060` | `cmd/muserver/metrics.go` | `net/http/pprof` em `/debug/pprof/` e `GET /debug/load` (JSON): CPU do processo (`getrusage`, zero no Windows), pico de RSS, heap, `Sys`, goroutines, GC e os usuários de cada GameServer (o mesmo número que vai ao ConnectServer). Não expor em servidor público |
| `muserver -max-conn-per-ip N` | `cmd/muserver/main.go` | Sobrepõe `MaxConnectionPerIP` do ConnectServer, do JoinServer e de cada GameServer (0 = sem limite). Necessário porque todos os bots saem do mesmo IP |
| `muserver -max-users N` | idem | Sobrepõe `ServerMaxUserNumber` (máximo 1000) |
| `internal/loadbot` | pacote novo | Cliente headless: `GameServerAddress` (F4 06 → F4 03 no ConnectServer), `Dial`/`Conn` (gscodec com Enc1/Dec2), builders F3 09, F1 01 (C3), F3 00, F3 03, D3, D7, 00, 0E e o `Bot` |
| `cmd/muload` | ferramenta nova | Roda os degraus, amostra o servidor e grava `samples.csv` e `summary.md` |

### Comportamento do bot (perfil "leve + combate")

Cada bot faz o caminho do cliente: ConnectServer → F1 00 → F3 09 (HWID) → F1 01 → F3 00 → F3 03. Depois:

- ataca (D7, ação 0x78) o monstro visível mais próximo a cada ~1 s quando está no alcance que o servidor aceita (`combat.go`: 3 células, Elf 6; o bot usa 2 e 5 porque o monstro anda entre a visão do bot e a do servidor);
- se o monstro está a até 8 células, anda até ele (D3, até 3 passos);
- sem monstro por perto, anda 1–3 passos aleatórios a cada 1–2 s, sem se afastar mais de 4 células do ponto inicial;
- manda chat a cada 20–40 s e o keepalive C3 0E a cada 20 s;
- acompanha monstros e a própria posição pelos pacotes C2 13, C1 14, D3, DF, 17 e F3 04, e consulta o `Terrain1.att` para não andar em célula bloqueada ou zona segura.

Os intervalos mudam com `-walk-every`, `-attack-every` e `-chat-every`.

## Como rodar

Local (de `apps/server`):

```sh
go run ./cmd/muserver -blacklist none -max-conn-per-ip 0 -seed-load 100 -metrics-addr 127.0.0.1:6060
# em outro terminal
go run ./cmd/muload -metrics http://127.0.0.1:6060/debug/load -pid $(pgrep muserver) \
    -steps 1,10,50,100 -steady 2m -cooldown 30s
```

Docker (da raiz; o projeto `-p mu-loadtest` isola volumes e banco dos dados normais):

```sh
export MU_PUBLIC_IP=127.0.0.1 MU_BLACKLIST=none MU_METRICS_ADDR=:6060 MU_SEED_LOAD=100 \
       MU_MAX_CONN_PER_IP=0 POSTGRES_HOST_PORT=55433
docker compose -p mu-loadtest up -d --build
cd apps/server && go run ./cmd/muload -metrics http://127.0.0.1:6060/debug/load -docker mu-loadtest-server-1
docker compose -p mu-loadtest down -v
```

Para medir num perfil de VPS, limite o container (`docker update --cpus 2 --memory 2g mu-loadtest-server-1`) antes de rodar o `muload`.

Flags do `muload`: `-connect` (padrão `127.0.0.1:44405`), `-game-host` (padrão: o host de `-connect`; `server` usa o IP que o ConnectServer devolve), `-steps`, `-ramp` (logins por segundo, padrão 5), `-idle` (linha de base antes do primeiro degrau), `-steady`, `-cooldown`, `-interval`, `-metrics`, `-pid` ou `-docker`, `-out`, `-first` (primeira conta), `-gs-config`, `-keys` (pasta com `Enc1.dat`/`Dec2.dat`, padrão `internal/protocol/crypto/testdata`), `-data`.

### Saída

- `samples.csv`: uma linha por intervalo com degrau, fase (`idle`, `login`, `steady`, `cooldown`), bots no mundo, usuários que o servidor reporta, CPU %, RSS, heap em uso, `Sys`, goroutines, número de GCs e pausa de GC no intervalo.
- `summary.md`: por degrau, o regime (CPU média/p95, RSS, heap, goroutines, GC/min, pacotes/s e KiB/s nos dois sentidos), a fase de login (latência p50/p95/máx, pico de CPU e memória), a atividade dos bots (passos, ataques, mortes, desconexões e erros agrupados) e o custo por jogador (regressão linear sobre os jogadores de fato online).

CPU em % de um núcleo (200 = dois núcleos). Com `-metrics` a CPU vem do `getrusage` (delta de tempo de CPU sobre o tempo de parede), mais preciso que a média do `ps`. A RSS vem do `ps` ou do `docker stats`.

## Resultados — 2026-10-27

Ambiente: Apple M4 (10 núcleos, 16 GB), macOS, bots na mesma máquina. Duas instâncias (`gameserver` e `gameserver-cs`), rampa de 5 logins/s, 90 s de regime por degrau, 20 s de pausa. Local com store em memória; Docker com PostgreSQL 17 (Docker Desktop, VM com 10 CPUs e 8 GB).

### Regime

| Degrau | Online (média) | CPU média | CPU p95 | RSS média (MiB) | RSS máx | Heap média | Pacotes/s enviados | KiB/s enviados |
|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| ocioso | 0 | 12,3 | 14,1 | 336 | 360 | 339 | – | – |
| 1 | 1 | 13,4 | 14,7 | 371 | 409 | 384 | 20 | 0,5 |
| 10 | 10 | 14,1 | 15,1 | 480 | 730 | 478 | 302 | 5,9 |
| 50 | 40 | 17,2 | 19,1 | 388 | 872 | 519 | 4 158 | 59,7 |
| 100 | 51 | 19,6 | 22,0 | 581 | 961 | 588 | 7 623 | 104,6 |

Docker (PostgreSQL), mesmo roteiro:

| Degrau | Online (média) | CPU média | CPU p95 | RSS média (MiB) | RSS máx | Heap média | Pacotes/s enviados |
|---:|---:|---:|---:|---:|---:|---:|---:|
| ocioso | 0 | 16,8 | 17,8 | 360 | 368 | 347 | – |
| 1 | 1 | 17,7 | 22,9 | 486 | 502 | 398 | 21 |
| 10 | 10 | 23,5 | 34,3 | 1 000 | 1 056 | 511 | 317 |
| 50 | 40 | 22,8 | 33,5 | 836 | 902 | 604 | 4 203 |
| 100 | 54 | 29,6 | 72,3 | 891 | 948 | 465 | 8 232 |

Login: p50 de 52–66 ms e p95 até 102 ms em todos os degraus (no Docker, o degrau de 10 teve p50 de 264 ms, com o PostgreSQL recém-aberto); nenhuma falha de login.

### Achados

1. **Patente derruba jogadores em área cheia.** O `C1 0C F3 E5` (`patente.go`) é enviado a cada jogador para todos os jogadores online a cada segundo e, de novo, para cada entrada nova do `C2 12`. É o comportamento do original (doc [10f](10f-gm-eventos-custom.md), seção "Patente", na lista de bugs reproduzidos do [13](13-progresso.md)). Quando 50 jogadores chegam juntos, um deles recebe 50 × 50 = 2 500 pacotes de uma vez, a fila de saída de 1 024 pacotes (`conn.go` `sendQueue`) enche e a conexão é fechada sem log. Medido: com 50 bots ficaram 40 online e com 100 ficaram 51 (local) ou 54 (Docker); todas as quedas ocorreram logo depois da entrada no mundo. O tráfego de patente cresce com N² (100 jogadores = 10 000 pacotes/s só de patente), e com o `CustomPatente.txt` ausente todos levam -1. **Até isso ser corrigido, ~50 jogadores na mesma área é o limite prático, independente de CPU e memória.**
2. **CPU não é o gargalo.** O processo ocioso usa 12–17 % de um núcleo (laços de 1 s das duas instâncias). Com ~50 jogadores em combate, chega a 20–30 % de um núcleo, cerca de 0,1 % de núcleo por jogador. Um vCPU aguenta 100 jogadores com folga; 2 vCPUs cobrem os picos de login.
3. **A memória base é alta e o login tem pico.** Cada instância carrega as tabelas de dados: o heap ocioso já tem ~340 MiB. Cada login roda argon2id com 64 MiB (`internal/auth/password.go`). Com logins concorrentes, a RSS sobe para 700 MB–1 GB e o Go demora a devolver essa memória ao sistema. O custo de regime por jogador é pequeno (1–4 MiB pela regressão).
4. **Rede:** ~1 KiB/s por jogador enviado com 100 jogadores juntos, quase tudo patente e viewport. Desprezível para qualquer link.

### Dimensionamento sugerido (com o comportamento atual)

| Jogadores | vCPU | RAM | Observação |
|---|---|---|---|
| até 50 | 1 | 1,5 GB | cobre o pico de logins simultâneos |
| até 100 | 2 | 2 GB | exige corrigir a patente, ou espalhar os jogadores |
| 100+ | 2+ | 2 GB + 64 MB por login simultâneo | limitar logins simultâneos ou baixar o custo do argon2 |

Para rodar mais de uma instância de GameServer por processo, some ~350 MiB de base por instância.

## Incógnitas e próximos passos

| Id | Pendência |
|---|---|
| U-LOAD-01 | Corrigir a patente (mandar só quando o `CustomPatente.txt` tiver linhas, ou só aos jogadores no viewport) depende do aval do usuário: é um bug do original reproduzido de propósito. Depois, repetir o teste com 100 e 200 jogadores |
| U-LOAD-02 | O original tinha um buffer secundário de 0x4000 bytes por conexão (`FUN_00504f00`). Não foi medido se o original também derrubava jogadores nessa situação |
| U-LOAD-03 | Medir com os bots espalhados por vários mapas (hoje todos nascem no mesmo campo, o pior caso de viewport) e com o servidor e os bots em máquinas separadas |
