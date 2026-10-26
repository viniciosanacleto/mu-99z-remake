# MU Server (Go)

Serviço único que substitui os processos do emulador original 97D/99z: ConnectServer, JoinServer, DataServer, GameServer e GameServerCS (Castle Siege). O protocolo do cliente original foi mantido, então o `Main.exe` original conecta sem mudanças. Os dados ficam em PostgreSQL.

As especificações de engenharia reversa estão em [`docs/server-plan/`](../../docs/server-plan/). O progresso e o próximo passo estão em [`13-progresso.md`](../../docs/server-plan/13-progresso.md).

## Requisitos

- Go 1.27.2 ou mais novo (versão do `go.mod`).
- PostgreSQL 13 ou mais novo, com a extensão `citext` (vem no pacote padrão). Opcional: sem banco o servidor usa um store em memória.
- O cliente original (`Cliente/` do pacote original), para jogar.

Todos os comandos abaixo rodam a partir de `apps/server`.

## Subir rápido (sem banco)

```sh
go run ./cmd/muserver -blacklist none -public-ip 127.0.0.1
```

Sem `-db`, tudo fica em memória e some ao parar o processo. Como o `account create` roda em outro processo, ele não enxerga esse store: no modo em memória só dá para testar a conexão e a lista de servidores. Para criar conta e jogar, use o PostgreSQL.

## Passo a passo com PostgreSQL

### 1. Subir o banco

Qualquer PostgreSQL serve. Com Docker:

```sh
docker run -d --name mu-postgres \
  -e POSTGRES_USER=mu -e POSTGRES_PASSWORD=mu -e POSTGRES_DB=mu \
  -p 55432:5432 postgres:17
```

URL de conexão desse exemplo:

```sh
export MU_DATABASE_URL='postgres://mu:mu@localhost:55432/mu?sslmode=disable'
```

O esquema é criado e atualizado sozinho na primeira conexão. As migrações ficam embutidas no binário (`internal/store/migrations/*.sql`, aplicadas em ordem). O controle das já aplicadas fica na tabela `schema_migrations`. Não há passo manual.

### 2. Criar uma conta

```sh
go run ./cmd/muserver account create <login> <senha>
```

O login tem de 1 a 10 caracteres e não diferencia maiúsculas. A senha é gravada com argon2id. Os personagens são criados pelo próprio cliente, na tela de seleção.

### 3. Liberar o 127.0.0.1

A `BlackList` de `[ConnectServer]` em `config/server.toml` (o `BlackList.txt` original) bloqueia `127.0.0.1`. Para testar na mesma máquina, passe `-blacklist none` (lista vazia) ou tire o endereço do arquivo.

### 4. Subir o servidor

```sh
go run ./cmd/muserver -blacklist none -public-ip 127.0.0.1
```

`-db` usa `MU_DATABASE_URL` por padrão. Ao subir, o log mostra:

| Módulo | Porta | Origem |
|---|---|---|
| ConnectServer | 44405 | `config/server.toml`, `[ConnectServer] ConnectServerPortTCP` |
| GameServer "Principal" (código 0) | 55900 | `config/gameserver/gameserver.toml`, `[Common] ServerPort` |
| GameServer "Castle Siege" (código 19, `ServerType = 1`) | 55919 | `config/gameserver/gameserver-cs.toml`, `[Common] ServerPort` |

`Ctrl+C` encerra o processo, desconectando todos os jogadores; a desconexão dispara a gravação dos personagens.

### 5. Conectar o cliente

Aponte o cliente original para o IP da máquina na porta 44405. O ConnectServer entrega a lista de servidores, e o cliente conecta no GameServer escolhido usando o IP da `ServerList` de `config/server.toml`. Esse IP é `192.168.1.1` nos arquivos originais, e `-public-ip` o substitui.

O `Main.dll` lê o IP do ConnectServer do `License.wzt`, não do `MainInfo.ini`. Para trocar: edite `IpAddress` em `PATCH CLIENTE/MainInfo.ini`, rode o `GetMainInfo.exe` nessa pasta (ele lê também `PATCH CLIENTE/Data/Custom*.txt`) e copie `PATCH CLIENTE/Path/License.wzt` para a pasta do cliente. Use o IP da máquina na rede (ex.: `192.168.18.5`), não `127.0.0.1`: com `127.0.0.1` no `License.wzt` o cliente não chega a abrir socket e a lista de servidores não aparece (observado em 2026-10-24). Passe o mesmo IP em `-public-ip`.

O login exige que versão e serial do cliente batam com `ServerVersion` (`1.00.00`) e `ServerSerial` de `[Common]` no arquivo da instância (`config/gameserver/*.toml`).

## Flags do `muserver`

| Flag | Padrão | Uso |
|---|---|---|
| `-config` | `config` | Pasta da configuração TOML (`server.toml`, `gameserver/`, `data/`) |
| `-data` | `data` | Pasta dos arquivos binários (`Data/Terrain`, `Data/Script`, chaves de `Data/Hack`, imagens de `Data/Custom`) |
| `-db` | `$MU_DATABASE_URL` | URL do PostgreSQL; vazio usa o store em memória |
| `-instances` | `gameserver,gameserver-cs` | GameServers a subir: cada nome é um arquivo `<config>/gameserver/<nome>.toml` e vira uma instância no mesmo processo |
| `-connect-addr` | `:44405` | Endereço do ConnectServer (sobrepõe a porta do `.ini`) |
| `-bind` | todas as interfaces | Host de todos os listeners |
| `-public-ip` | — | IP enviado ao cliente no lugar dos endereços da `ServerList` e da `MapServerInfo` |
| `-blacklist` | — | IPs bloqueados no ConnectServer separados por vírgula, no lugar da `BlackList` de `server.toml`; `none` = nenhum |

Variáveis de ambiente:

- `MU_DATABASE_URL`: padrão de `-db` (e de `-to` no `migrate`).
- `MU_DEBUG`: qualquer valor liga o log em nível debug.

Subcomando administrativo: `muserver account create <login> <senha>` (usa o mesmo `-db`).

## Configuração

Toda a configuração fica em `config/`, em arquivos TOML (UTF-8) gerados a partir dos `.ini`, `.dat` e `.txt` do emulador original pelo `cmd/configconv`. As chaves e colunas mantêm os nomes originais e os comentários dos arquivos originais foram preservados. O formato e o mapeamento original → TOML estão em [`11-arquivos-de-configuracao.md`](../../docs/server-plan/11-arquivos-de-configuracao.md).

| Arquivo | O que define |
|---|---|
| `config/server.toml` | `[ConnectServer]`: porta TCP, tempo ocioso, `MaxConnectionPerIP` (1 no original), `MaxPacketPerSecond`, `BlackList` e `ServerList` (código, nome, IP, porta, visível); `[JoinServer]`: `MaxConnectionPerIP` e `MaxConnectionPerHID` do login (0 = sem limite); `[DataServer]`: `BadSyntax` |
| `config/gameserver/gameserver.toml` | Uma instância: as tabelas `[Common]` (nome, código, tipo, porta, versão/serial, taxas), `[Character]`, `[ChaosMix]`, `[Command]`, `[Custom]`, `[Event]`, `[Item]`, `[Skill]` (os 8 `GameServerInfo - *.dat` do original) |
| `config/gameserver/gameserver-cs.toml` | O mesmo para a instância do Castle Siege |
| `config/data/common.toml` | Arquivos da raiz de `Data/`: `MapManager`, `MapServerInfo` (que servidor atende cada mapa), `MapRateInfo`, `BlackList` do GameServer, `CommandManager`, `CommandRequirement`, `ConnectMember` |
| `config/data/util.toml` | `GameMaster` (GMs), `Filter`, `DamageTable`, `ExperienceTable`, `PetDamageTable`, `ResetTable`, `ResetLimit` |
| `config/data/custom.toml`, `custom-event.toml`, `xshop.toml` | Sistemas custom do emulador (Arena, Marriage, LuckyWheel, GHRS, Pick etc.), os eventos de `Custom/Event` e o XShop |
| `config/data/event.toml`, `eventitembag.toml` | Eventos e seus drops |
| `config/data/monster.toml`, `spawn.toml`, `item.toml`, `shop.toml`, `move.toml`, `quest.toml`, `skill.toml`, `character.toml`, `hack.toml` | Monstros e spawns, itens, lojas, gates/moves, quests, skills, classes, anti-hack |
| `config/data/lang.toml` | Textos enviados ao jogador (`Message`, `Notice`) por idioma |
| `data/Data/Terrain/*`, `data/Data/Script/**.lua`, `data/Data/Hack/{Enc2,Dec1}.dat`, `data/Data/Skill/SkillElect.hit` | Arquivos binários e scripts Lua (Lua 5.2), com os caminhos originais |

Um nome de chave ou de coluna errado num TOML é erro na carga, com arquivo e linha. Itens são referenciados como `"cc,iii"` (categoria, índice) e `-1` é o `*` (qualquer) do original. As alterações valem no próximo início do processo. A exceção são os valores que o original relê a cada uso (`CreationGold`, `CustomXShopRecharge`) e os que o servidor grava: o reinício do Castle Siege em `[Event]` do arquivo da instância, os preços do XShop alterados por GM e a recarga enviada por GM (`config/data/xshop.toml`).

### Reimportar dos arquivos originais

```sh
go run ./cmd/configconv -from /caminho/para/MuServer -to config [-only server,gameserver,item,...]
```

`-from` aponta para uma pasta com o layout original (`1.ConnectServer`, `2.DataServer`, `3.JoinServer`, `4.GameServer/DATA`, `5.GameServerCS/DATA`, `Data`). A conversão aplica as regras do leitor original (seções, `*`, vírgula como separador de números, colunas que faltam etc.) e reescreve os arquivos de `config/`.

## Banco de dados

- O esquema mantém o layout em bytes das colunas binárias do MuOnline original (inventário de 1728 bytes, magic list de 180 etc.). O mapeamento tabela a tabela está em [`06-schema-postgresql.md`](../../docs/server-plan/06-schema-postgresql.md).
- Principais tabelas: `accounts`, `characters`, `warehouses`, `character_options`, `guilds`/`guild_members`, `friend_*`, `pet_item_info`, `mu_castle_*`, `event_entry_count`, `reset_data`, `ranking_*`, `periodic_item`, `custom_*`, `daily_reward`.
- Tarefas comuns por SQL:

  ```sql
  -- créditos do XShop / LuckyWheel
  UPDATE accounts SET credits = 1000 WHERE login = 'teste';
  -- nível de conta (VIP) com validade
  UPDATE accounts SET account_level = 1, level_expires_at = now() + interval '30 days' WHERE login = 'teste';
  -- bloquear conta / personagem
  UPDATE accounts SET block_code = 1 WHERE login = 'teste';
  UPDATE characters SET ctl_code = ctl_code | 1 WHERE name = 'Fulano';  -- bit 0: personagem banido
  ```

  Faça essas alterações com o personagem offline. O jogador online é mantido em memória e gravado de volta ao sair.

### Importar um banco original (MuOnline)

O `cmd/migrate` copia um banco MuOnline do SQL Server para o esquema PostgreSQL. Ele importa contas, personagens, baús, opções, amigos e cartas, guilds, pets, entradas de eventos, Golden Archer, resets, rankings, itens periódicos e dados custom. O PostgreSQL de destino precisa estar sem contas, e a importação roda numa única transação.

```sh
# direto de um SQL Server com o MuOnline restaurado
go run ./cmd/migrate -from 'sqlserver://sa:senha@host:1433?database=MuOnline' -to "$MU_DATABASE_URL"

# ou de um script SQL com INSERTs (UTF-16 ou UTF-8)
go run ./cmd/migrate -script dump.sql -to "$MU_DATABASE_URL"
```

As senhas importadas ficam como `plain:` (o original usa `MD5Encryption = 0`). Elas passam para argon2id no primeiro login de cada conta. O `data/DB/MuOnline.sql` que vem com o pacote só traz o esquema e as procedures, sem dados de jogadores.

## Testes

```sh
go vet ./... && gofmt -l .
go test -p 2 ./...
```

Os testes de store e de migração em PostgreSQL só rodam com um banco de teste:

```sh
MU_TEST_DATABASE_URL='postgres://mu:mu@localhost:55432/mu?sslmode=disable' go test -p 2 -count=1 ./...
```

Para o código de rede, use `-race` num subconjunto (`go test -race -p 1 -run <Teste> ./internal/...`).

## Proxy de captura

`cmd/muproxy` fica entre o cliente original e um servidor (o original ou este) e grava os pacotes decodificados e as sessões golden. Uso em [`15-captura.md`](../../docs/server-plan/15-captura.md).

## Editor de personagens

`cmd/mueditor` é um editor web que muda atributos, zen e itens (inventário e baú 0) de personagens **offline** direto no PostgreSQL. Contas online ficam só leitura, porque o GameServer regrava o personagem no logout.

```sh
go run ./cmd/mueditor -db postgres://... [-config config] [-addr 127.0.0.1:8080] [-password segredo]
```

`-db` e `-password` também podem vir de `MU_DATABASE_URL` e `MU_EDITOR_PASSWORD` (sem senha não há autenticação). O nível máximo vem do `ServerMaxLevel` da instância em `-instance` (padrão `gameserver`, o arquivo `config/gameserver/gameserver.toml`).

## Estrutura

| Pasta | Conteúdo |
|---|---|
| `cmd/muserver` | O serviço e o subcomando `account create` |
| `cmd/migrate` | Importação do MuOnline (SQL Server ou script) |
| `cmd/muproxy` | Proxy de captura |
| `cmd/configconv` | Conversão dos arquivos originais para a configuração TOML |
| `config/` | A configuração (TOML) |
| `data/` | Arquivos binários do original (terreno, Lua, chaves) e o `MuOnline.sql` |
| `internal/connect` | ConnectServer (lista de servidores) |
| `internal/auth` | JoinServer: login, sessões, senhas |
| `internal/game` | GameServer: `World` (uma goroutine por instância) e todos os sistemas de jogo |
| `internal/gamedata` | Leitores das tabelas de `config/data` e do terreno |
| `internal/config` | Leitura de `server.toml` e das instâncias, codificador TOML (`tomlconf`) e os leitores dos formatos originais usados só pelo `configconv` (`legacy`, `mutext`, `ini`) |
| `internal/store` | Persistência (memória e PostgreSQL) e migrações |
| `internal/protocol` | Framing C1–C4, criptografia (SimpleModulus, XOR32, cifra de fluxo) |
| `internal/migrate` | Leitura e conversão do banco original |
| `data/` | Cópia dos dados do `MuServer/` original |
