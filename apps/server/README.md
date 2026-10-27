# MU Server (Go)

A single service that replaces the processes of the original 97D/99z emulator: ConnectServer, JoinServer, DataServer, GameServer and GameServerCS (Castle Siege). The original client protocol is kept, so the original `Main.exe` connects without changes. Data is stored in PostgreSQL.

The reverse-engineering specs are in [`docs/server-plan/`](../../docs/server-plan/) (in Portuguese). Progress and the next step are tracked in [`13-progresso.md`](../../docs/server-plan/13-progresso.md).

## Requirements

- Go 1.27.2 or newer (the `go.mod` version).
- PostgreSQL 13 or newer, with the `citext` extension (part of the standard package). Optional: without a database the server uses an in-memory store.
- The original client (`Cliente/` from the original package), to play.

Every command below runs from `apps/server`.

## Quick start (no database)

```sh
go run ./cmd/muserver -blacklist none -public-ip 127.0.0.1
```

Without `-db`, everything stays in memory and is lost when the process stops. Since `account create` runs in another process, it can't see that store: in memory mode you can only test the connection and the server list. To create an account and play, use PostgreSQL.

## Step by step with PostgreSQL

### 1. Start the database

Any PostgreSQL works. With Docker:

```sh
docker run -d --name mu-postgres \
  -e POSTGRES_USER=mu -e POSTGRES_PASSWORD=mu -e POSTGRES_DB=mu \
  -p 55432:5432 postgres:17
```

Connection URL for this example:

```sh
export MU_DATABASE_URL='postgres://mu:mu@localhost:55432/mu?sslmode=disable'
```

The schema is created and updated automatically on the first connection. The migrations are embedded in the binary (`internal/store/migrations/*.sql`, applied in order) and the applied ones are tracked in the `schema_migrations` table. There is no manual step.

### 2. Create an account

```sh
go run ./cmd/muserver account create <login> <password>
```

The login has 1 to 10 characters and is case-insensitive. The password is stored with argon2id. Characters are created by the client itself, on the character selection screen.

### 3. Allow 127.0.0.1

The `BlackList` in `[ConnectServer]` of `config/server.toml` (the original `BlackList.txt`) blocks `127.0.0.1`. To test on the same machine, pass `-blacklist none` (empty list) or remove the address from the file.

### 4. Start the server

```sh
go run ./cmd/muserver -blacklist none -public-ip 127.0.0.1
```

`-db` defaults to `MU_DATABASE_URL`. On startup the log shows:

| Module | Port | Source |
|---|---|---|
| ConnectServer | 44405 | `config/server.toml`, `[ConnectServer] ConnectServerPortTCP` |
| GameServer "Principal" (code 0) | 55900 | `config/gameserver/gameserver.toml`, `[Common] ServerPort` |
| GameServer "Castle Siege" (code 19, `ServerType = 1`) | 55919 | `config/gameserver/gameserver-cs.toml`, `[Common] ServerPort` |

`Ctrl+C` stops the process and disconnects every player; the disconnect saves the characters.

### 5. Connect the client

Point the original client at the machine's IP on port 44405. The ConnectServer sends the server list, and the client connects to the chosen GameServer using the IP from the `ServerList` in `config/server.toml`. That IP is `192.168.1.1` in the original files, and `-public-ip` replaces it.

`Main.dll` reads the ConnectServer IP from `License.wzt`, not from `MainInfo.ini`. To change it: edit `IpAddress` in `PATCH CLIENTE/MainInfo.ini`, run `GetMainInfo.exe` in that folder (it also reads `PATCH CLIENTE/Data/Custom*.txt`) and copy `PATCH CLIENTE/Path/License.wzt` to the client folder. Use the machine's network IP (e.g. `192.168.18.5`), not `127.0.0.1`: with `127.0.0.1` in `License.wzt` the client never opens a socket and the server list doesn't show up (observed on 2026-10-24). Pass the same IP in `-public-ip`.

Login requires the client version and serial to match `ServerVersion` (`1.00.00`) and `ServerSerial` in `[Common]` of the instance file (`config/gameserver/*.toml`).

## Running with Docker

The `docker-compose.yml` at the repository root runs the server and PostgreSQL 17 in containers. The commands in this section run from the repository root.

```sh
cp .env.example .env   # set MU_PUBLIC_IP to the machine's network IP
docker compose up -d --build
docker compose exec server muserver account create <login> <password>
docker compose logs -f server
```

- The image (`apps/server/Dockerfile`) builds a static `muserver` and runs it on `distroless/static` as a non-root user. `config/` and `data/` are copied into the image.
- Ports 44405, 55900 and 55919 are published on the host. The client connects to the machine's IP, as in "Connect the client" above.
- `.env`:

  | Variable | Use |
  |---|---|
  | `MU_PUBLIC_IP` | Required. Becomes `-public-ip`: the machine's network IP, the same as in `License.wzt` |
  | `MU_BLACKLIST` | Becomes `-blacklist`. Empty keeps the `BlackList` of `config/server.toml` |
  | `MU_INSTANCES` | Becomes `-instances` |
  | `MU_DEBUG` | Any value enables debug logging |
  | `POSTGRES_USER`, `POSTGRES_PASSWORD`, `POSTGRES_DB` | Database credentials. The server's `MU_DATABASE_URL` is built from them |
  | `POSTGRES_HOST_PORT` | Database port on the host (default 55432), for `cmd/migrate`, `cmd/mueditor` and `psql` |

- Volumes: `mu-postgres` holds the database. `mu-config` and `mu-data` receive the image's `config/` and `data/` on the first start and keep what the server rewrites (XShop, Castle Siege reset, recharge images). After changing the configuration in the repository, recreate these two volumes to use it: `docker compose down && docker volume rm mu-remake_mu-config mu-remake_mu-data && docker compose up -d --build`. `docker compose down -v` also deletes the database.
- The database on the host works with the other commands: `go run ./cmd/migrate -script dump.sql -to 'postgres://mu:mu@localhost:55432/mu?sslmode=disable'`.
- With Docker Desktop (macOS/Windows), every connection reaches the container from the Docker gateway IP, not the client's IP. The ConnectServer `BlackList` and `MaxConnectionPerIP = 1` then apply to that single IP, so a second simultaneous client may be refused. On Linux the client's real IP is kept.

## `muserver` flags

| Flag | Default | Use |
|---|---|---|
| `-config` | `config` | TOML configuration folder (`server.toml`, `gameserver/`, `data/`) |
| `-data` | `data` | Binary files folder (`Data/Terrain`, `Data/Script`, `Data/Hack` keys, `Data/Custom` images) |
| `-db` | `$MU_DATABASE_URL` | PostgreSQL URL; empty uses the in-memory store |
| `-instances` | `gameserver,gameserver-cs` | GameServers to start: each name is a `<config>/gameserver/<name>.toml` file and becomes an instance in the same process |
| `-connect-addr` | `:44405` | ConnectServer address (overrides the configured port) |
| `-bind` | all interfaces | Host for every listener |
| `-public-ip` | — | IP sent to the client instead of the `ServerList` and `MapServerInfo` addresses |
| `-blacklist` | — | Comma-separated IPs the ConnectServer refuses, instead of the `BlackList` in `server.toml`; `none` = no IPs |

Environment variables:

- `MU_DATABASE_URL`: default for `-db` (and for `-to` in `migrate`).
- `MU_DEBUG`: any value enables debug logging.

Administrative subcommand: `muserver account create <login> <password>` (uses the same `-db`).

## Configuration

All configuration lives in `config/`, in TOML files (UTF-8) generated from the original emulator's `.ini`, `.dat` and `.txt` files by `cmd/configconv`. Keys and columns keep their original names, and the comments of the original files are preserved. The format and the original → TOML mapping are in [`11-arquivos-de-configuracao.md`](../../docs/server-plan/11-arquivos-de-configuracao.md).

| File | What it defines |
|---|---|
| `config/server.toml` | `[ConnectServer]`: TCP port, idle timeout, `MaxConnectionPerIP` (1 in the original), `MaxPacketPerSecond`, `BlackList` and `ServerList` (code, name, IP, port, visible); `[JoinServer]`: login `MaxConnectionPerIP` and `MaxConnectionPerHID` (0 = no limit); `[DataServer]`: `BadSyntax` |
| `config/gameserver/gameserver.toml` | One instance: the tables `[Common]` (name, code, type, port, version/serial, rates), `[Character]`, `[ChaosMix]`, `[Command]`, `[Custom]`, `[Event]`, `[Item]`, `[Skill]` (the original's 8 `GameServerInfo - *.dat` files) |
| `config/gameserver/gameserver-cs.toml` | The same for the Castle Siege instance |
| `config/data/common.toml` | Files at the root of `Data/`: `MapManager`, `MapServerInfo` (which server handles each map), `MapRateInfo`, the GameServer `BlackList`, `CommandManager`, `CommandRequirement`, `ConnectMember` |
| `config/data/util.toml` | `GameMaster` (GMs), `Filter`, `DamageTable`, `ExperienceTable`, `PetDamageTable`, `ResetTable`, `ResetLimit` |
| `config/data/custom.toml`, `custom-event.toml`, `xshop.toml` | The emulator's custom systems (Arena, Marriage, LuckyWheel, GHRS, Pick etc.), the `Custom/Event` events and the XShop |
| `config/data/event.toml`, `eventitembag.toml` | Events and their drops |
| `config/data/monster.toml`, `spawn.toml`, `item.toml`, `shop.toml`, `move.toml`, `quest.toml`, `skill.toml`, `character.toml`, `hack.toml` | Monsters and spawns, items, shops, gates/moves, quests, skills, classes, anti-hack |
| `config/data/lang.toml` | Texts sent to the player (`Message`, `Notice`) per language |
| `data/Data/Terrain/*`, `data/Data/Script/**.lua`, `data/Data/Hack/{Enc2,Dec1}.dat`, `data/Data/Skill/SkillElect.hit` | Binary files and Lua scripts (Lua 5.2), with their original paths |

A wrong key or column name in a TOML file is a load error, reported with file and line. Items are referenced as `"cc,iii"` (category, index), and `-1` is the original's `*` (any). Changes take effect on the next process start. The exceptions are the values the original rereads on every use (`CreationGold`, `CustomXShopRecharge`) and the ones the server writes: the Castle Siege reset in `[Event]` of the instance file, XShop prices changed by a GM and recharges uploaded by a GM (`config/data/xshop.toml`).

### Reimporting from the original files

```sh
go run ./cmd/configconv -from /path/to/MuServer -to config [-only server,gameserver,item,...]
```

`-from` points to a folder with the original layout (`1.ConnectServer`, `2.DataServer`, `3.JoinServer`, `4.GameServer/DATA`, `5.GameServerCS/DATA`, `Data`). The conversion applies the original reader's rules (sections, `*`, comma as number separator, missing columns etc.) and rewrites the files in `config/`.

## Database

- The schema keeps the byte layout of the original MuOnline binary columns (1728-byte inventory, 180-byte magic list etc.). The table-by-table mapping is in [`06-schema-postgresql.md`](../../docs/server-plan/06-schema-postgresql.md).
- Main tables: `accounts`, `characters`, `warehouses`, `character_options`, `guilds`/`guild_members`, `friend_*`, `pet_item_info`, `mu_castle_*`, `event_entry_count`, `reset_data`, `ranking_*`, `periodic_item`, `custom_*`, `daily_reward`.
- Common tasks in SQL:

  ```sql
  -- XShop / LuckyWheel credits
  UPDATE accounts SET credits = 1000 WHERE login = 'teste';
  -- account level (VIP) with expiry
  UPDATE accounts SET account_level = 1, level_expires_at = now() + interval '30 days' WHERE login = 'teste';
  -- block an account / character
  UPDATE accounts SET block_code = 1 WHERE login = 'teste';
  UPDATE characters SET ctl_code = ctl_code | 1 WHERE name = 'Fulano';  -- bit 0: character banned
  ```

  Make these changes with the character offline. An online player is kept in memory and written back on logout.

### Importing an original database (MuOnline)

`cmd/migrate` copies a MuOnline database from SQL Server into the PostgreSQL schema. It imports accounts, characters, warehouses, options, friends and letters, guilds, pets, event entries, Golden Archer, resets, rankings, periodic items and custom data. The target PostgreSQL must have no accounts, and the import runs in a single transaction.

```sh
# straight from a SQL Server with MuOnline restored
go run ./cmd/migrate -from 'sqlserver://sa:password@host:1433?database=MuOnline' -to "$MU_DATABASE_URL"

# or from a SQL script with INSERTs (UTF-16 or UTF-8)
go run ./cmd/migrate -script dump.sql -to "$MU_DATABASE_URL"
```

Imported passwords are stored as `plain:` (the original uses `MD5Encryption = 0`). They move to argon2id on each account's first login. The `data/DB/MuOnline.sql` shipped with the package only has the schema and the procedures, with no player data.

## Tests

```sh
go vet ./... && gofmt -l .
go test -p 2 ./...
```

The PostgreSQL store and migration tests only run with a test database:

```sh
MU_TEST_DATABASE_URL='postgres://mu:mu@localhost:55432/mu?sslmode=disable' go test -p 2 -count=1 ./...
```

For the network code, use `-race` on a subset (`go test -race -p 1 -run <Test> ./internal/...`).

## Capture proxy

`cmd/muproxy` sits between the original client and a server (the original one or this one) and records the decoded packets and golden sessions. Usage in [`15-captura.md`](../../docs/server-plan/15-captura.md).

## Character editor

`cmd/mueditor` is a web editor that changes stats, zen and items (inventory and warehouse 0) of **offline** characters directly in PostgreSQL. Online accounts are read-only, because the GameServer rewrites the character on logout.

```sh
go run ./cmd/mueditor -db postgres://... [-config config] [-addr 127.0.0.1:8080] [-password secret]
```

`-db` and `-password` can also come from `MU_DATABASE_URL` and `MU_EDITOR_PASSWORD` (without a password there is no authentication). The maximum level comes from `ServerMaxLevel` of the instance in `-instance` (default `gameserver`, the file `config/gameserver/gameserver.toml`).

## Layout

| Folder | Contents |
|---|---|
| `cmd/muserver` | The service and the `account create` subcommand |
| `cmd/migrate` | MuOnline import (SQL Server or script) |
| `cmd/muproxy` | Capture proxy |
| `cmd/configconv` | Conversion of the original files to the TOML configuration |
| `config/` | The configuration (TOML) |
| `data/` | Binary files from the original (terrain, Lua, keys) and `MuOnline.sql` |
| `internal/connect` | ConnectServer (server list) |
| `internal/auth` | JoinServer: login, sessions, passwords |
| `internal/game` | GameServer: `World` (one goroutine per instance) and every game system |
| `internal/gamedata` | Readers for the `config/data` tables and the terrain |
| `internal/config` | Reading of `server.toml` and the instances, the TOML encoder (`tomlconf`) and the original-format readers used only by `configconv` (`legacy`, `mutext`, `ini`) |
| `internal/store` | Persistence (memory and PostgreSQL) and migrations |
| `internal/protocol` | C1–C4 framing, encryption (SimpleModulus, XOR32, stream cipher) |
| `internal/migrate` | Reading and conversion of the original database |
