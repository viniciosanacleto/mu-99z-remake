# Changelog

All changes to this project are recorded here, newest first.

## 2026-10-27

- Added a load test. `cmd/muload` runs steps of 1, 10, 50 and 100 fake players against a running server, samples its CPU and memory (`/debug/load`, `ps` or `docker stats`) and writes `samples.csv` and `summary.md`. `internal/loadbot` is the headless client: ConnectServer lookup, login, character select, walk, attack, chat and keepalive.
- Added load test flags to `muserver` (`cmd/muserver/metrics.go`, `seed.go`, `main.go`):
  - `-metrics-addr` serves pprof and `/debug/load`.
  - `-seed-load N` creates the `load0001..` accounts with one character each in a Lorencia monster field.
  - `-max-conn-per-ip` overrides `MaxConnectionPerIP` of the ConnectServer, JoinServer and GameServers.
  - `-max-users` overrides `ServerMaxUserNumber`.
- Added the matching optional variables to `docker-compose.yml` and `.env.example` (`MU_METRICS_ADDR`, `MU_METRICS_HOST_PORT`, `MU_SEED_LOAD`, `MU_MAX_CONN_PER_IP`, `MU_MAX_USERS`) and published port 6060.
- Added `docs/server-plan/18-teste-de-carga.md` with the method, the first results (local and Docker) and sizing. The test found that the reproduced patente broadcast (`C1 F3 E5` to every player for every player) overflows the 1024-packet send queue and drops players once about 50 are in the same area. Updated `docs/server-plan/README.md`, `13-progresso.md` and `apps/server/README.md`.

- Moved the GM level from `config/data/util.toml` (`[GameMaster]`, from `Util/GameMaster.txt`) to the database: new column `characters.gm_level` (`internal/store/migrations/0019_gm_level.sql`), `Character.GMLevel` loaded by `LoadCharacter` and left untouched by `SaveCharacter`, and `store.GMStore.SetGMLevel` (`internal/store/gm.go`, Memory and Postgres). `game.gmLevel` (`internal/game/chat.go`) reads it from the character, so a new GM applies on the next login without a restart.
- Removed `GameMaster` from `gamedata.UtilConfig` and from the `util` group of `cmd/configconv`; `config/data/util.toml` no longer has the section. `legacy.ReadGameMasters` (`internal/config/legacy/gamemaster.go`) still reads the original file.
- Added `cmd/migrate -gamemasters <MuServer folder>`: imports `Data/Util/GameMaster.txt` into `characters.gm_level` (account + character match), alone or after `-from`/`-script` (`internal/migrate/gamemaster.go`).
- Added the `muserver character gm <name> <level>` administrative subcommand (`cmd/muserver/main.go`).
- GM level 32 now gets every privilege of level 8: the original's `Authority == 8` tests became `gmFull` (level 8 or higher, `internal/game/monster_skill.go`) in PvP, Castle Siege, pushback, `/hide`, `/superdrop`, trade block, trap targeting and the xshop GM packets (U-GM-05 in `docs/server-plan/10f-gm-eventos-custom.md`).
- Updated `docs/server-plan/06`, `11`, `13` and `16` for the new column; tests in `internal/store/gm_test.go`, `internal/game/gm_level_test.go`, `internal/config/legacy/gamemaster_test.go`, `cmd/muserver/command_test.go` and `cmd/migrate/main_test.go`.

- Fixed the comments of four rows of `[Shop."255 - Lumen the Barmaid"]` in `apps/server/config/data/shop.toml` that said "Scroll of Blood +5": items 12,000 / 12,001 / 12,002 / 13,030 are Wings of Elf, Wings of Heaven, Wings of Satan and Cape of Lord (names from `config/data/item.toml`). Comments only; the items sold are unchanged.
- Moved the GM level from `config/data/util.toml` (`[GameMaster]`, from `Util/GameMaster.txt`) to the database: new column `characters.gm_level` (`internal/store/migrations/0019_gm_level.sql`), `Character.GMLevel` loaded by `LoadCharacter` and left untouched by `SaveCharacter`, and `store.GMStore.SetGMLevel` (`internal/store/gm.go`, Memory and Postgres). `game.gmLevel` (`internal/game/chat.go`) reads it from the character, so a new GM applies on the next login without a restart.
- Removed `GameMaster` from `gamedata.UtilConfig` and from the `util` group of `cmd/configconv`; `config/data/util.toml` no longer has the section. `legacy.ReadGameMasters` (`internal/config/legacy/gamemaster.go`) still reads the original file.
- Added `cmd/migrate -gamemasters <MuServer folder>`: imports `Data/Util/GameMaster.txt` into `characters.gm_level` (account + character match), alone or after `-from`/`-script` (`internal/migrate/gamemaster.go`).
- Added the `muserver character gm <name> <level>` administrative subcommand (`cmd/muserver/main.go`).
- GM level 32 now gets every privilege of level 8: the original's `Authority == 8` tests became `gmFull` (level 8 or higher, `internal/game/monster_skill.go`) in PvP, Castle Siege, pushback, `/hide`, `/superdrop`, trade block, trap targeting and the xshop GM packets (U-GM-05 in `docs/server-plan/10f-gm-eventos-custom.md`).
- Updated `docs/server-plan/06`, `11`, `13` and `16` for the new column; tests in `internal/store/gm_test.go`, `internal/game/gm_level_test.go`, `internal/config/legacy/gamemaster_test.go`, `cmd/muserver/command_test.go` and `cmd/migrate/main_test.go`.

- Added Docker support: `apps/server/Dockerfile` (static `muserver` on distroless, non-root, with `config/` and `data/`), `apps/server/.dockerignore`, `docker-compose.yml` at the repository root (server plus PostgreSQL 17 with healthcheck and volumes for the database and the files the server rewrites) and `.env.example` (`MU_PUBLIC_IP`, `MU_BLACKLIST`, `MU_INSTANCES`, `MU_DEBUG`, database credentials and host port).
- Added the "Running with Docker" section to `apps/server/README.md`.
- Rewrote `apps/server/README.md` in English; dropped the duplicate `data/` row of the layout table.

## 2026-10-26

- Neutralized server and emulator branding: the player-facing texts in `config/data/lang.toml` (message 990, Notice), `config/data/custom.toml` (4 quiz questions, an "OriX" move comment) and the window title and welcome Lua scripts in `data/Data/Script` now say "MU Online"; `[CustomXShopRecharge]` in `config/data/xshop.toml` is disabled with empty fields; removed `data/Data/Custom/pixFE31.tmp.bmp` (a real PIX QR code) and its entry in `internal/game/datacoverage_test.go`; the `data/DB/MuOnline.sql` seed accounts use `@example.com`; removed the WizardTeam/SSeMU banners from the remaining `data/` files. The changed TOML files were regenerated with `cmd/configconv`.
- Added a comment above `CustomerName` in `config/gameserver/gameserver.toml` and `gameserver-cs.toml` saying it is the stream cipher key and must match the client's `MainInfo.ini`.
- Neutralized emulator names in Go and SQL migration comments, `docs/server-plan/`, both READMEs and `CLAUDE.md`; renamed `docs/server-plan/10e-customs-wizardteam.md` to `10e-customs.md`.
- Updated the tests that read the changed data (`internal/game/lua_test.go`, `xshop_test.go`).
- Moved the whole server configuration to TOML files in `apps/server/config/`: `server.toml` (ConnectServer.ini, ServerList.dat, BlackList.txt, JoinServer.ini, BadSyntax.txt), `gameserver/gameserver.toml` and `gameserver/gameserver-cs.toml` (the eight `GameServerInfo - *.dat` of each instance) and 17 table groups in `config/data/` (every `Data/**` file the original reads). The `.ini`, `.dat` and `.txt` copies were removed from `apps/server/data/`, which now keeps only binary files (terrain, Lua, keys, images) and `DB/MuOnline.sql`.
- Added `cmd/configconv` and `internal/config/legacy`: they convert an original MuServer folder into these TOML files, applying the original reader rules (sections, `*`, comma-split numbers, token-stream records, rows the original drops) and keeping column names and comments.
- Added `internal/config/tomlconf` (strict TOML loading with Windows-1252 strings, one-row-per-line encoder, `Save` for files the server rewrites) and `internal/config/servercfg`; `gsconfig` now reads the instance TOML (same `Int`/`ALInt`/`String` API, `SetInt` for the Castle Siege reset).
- Rewrote every table loader in `internal/gamedata` and `internal/game` to read `config/data/<group>.toml` (decoded files cached while unchanged); a dump of all loader results matches the previous text loaders except `Item.txt` 12,203, now sent in Windows-1252.
- Changed the `muserver` flags: new `-config` (default `config`), `-data` holds only binary files, `-instances` takes `gameserver,gameserver-cs`, `-blacklist` takes addresses or `none`; `mueditor` takes `-config` and `-instance gameserver` (no `-data`); `muproxy` replaced `-gs-data` with `-gs-config`.
- XShop GM price changes and recharge uploads now rewrite `config/data/xshop.toml`; recharge images stay in `data/Data/Custom`. Lua profile paths are relative to `data/`.
- Rewrote the data coverage test (`internal/game/datacoverage_test.go`) for the config groups and the remaining binary files; moved the original-format parser tests to `internal/config/legacy`.
- Updated `apps/server/README.md`, `CLAUDE.md`, this README and `docs/server-plan/11`, `13`, `15`, `16` and `17` for the TOML configuration.
- Added the project `README.md` for GitHub, pointing to `apps/server/README.md` for running the server.
- Added this `CHANGELOG.md` and the rule in `CLAUDE.md` to record every change in it.
