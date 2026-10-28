# Changelog

All changes to this project are recorded here, newest first.

## 2026-10-28

- Reworked `apps/server` against the Go skills (design patterns, concurrency, context, error handling, testing):
  - `internal/game`: the ~100 package-level hook variables and dispatch tables (`startHooks`, `tick1000Hooks`, `worldHandlers`, `npcTalkHooks`, ...) filled by ~50 `init()` functions are now a per-`World` `hooks` table (`hooks.go`). `newHooks` builds it by calling the former `init` bodies, now `register<File>(h *hooks)`, in their old order. Tests stub hooks on their own `World` instead of swapping globals. Removed the dead `pendingCommands` map, the unused `errNoSession`, the empty placeholder `type ()` in `ext.go` and the shop `sync.Once`, which only registered shop NPC talks on the first `World`.
  - `internal/config/legacy`: the 13 `init()`/`register` calls are now one ordered `dataGroups` table in `convert.go` (same output order; `TestOutputs`).
  - Fixed: a clean shutdown did not save the characters in game. Disconnect events were dropped once the loop stopped, and async store work got a cancelled context. `World.shutdown` now removes every player itself, and `async` runs with `context.WithoutCancel` bounded by 30 s (`world.go`; `TestServeShutdownSavesPlayingCharacters`).
  - Fixed: `World.Serve` hung forever when `Accept` failed while its context was alive (`TestServeReturnsListenerError`). It now cancels its own context, uses `context.AfterFunc` instead of a watcher goroutine and `WaitGroup.Go` for async work, and replaces the stored `ctx` used by `post` with a `stopped` channel. That also removes a data race between `Serve` and early `post` calls.
  - Fixed: `connect.Server.Serve` waited for open clients after its listener failed (`TestServeListenerFailureClosesClients`). It also gets the same `AfterFunc`/`WaitGroup.Go` cleanup and a doc comment on `NewServer`.
  - Fixed: a `muserver` start-up failure, such as a busy ConnectServer port, left the game servers already started running with the store closed. `run` now cancels and waits for them. Its wiring moved into `modules.start`/`startGameServer`/`startConnectServer`, with wrapped listen errors (`cmd/muserver/main.go`; `TestRunStopsStartedModulesOnStartupFailure`).
  - `internal/auth`: password-rehash and logout store failures are now logged through the new `auth.Config.Log` instead of being discarded with `_ =` (`TestBestEffortStoreFailuresAreLogged`).
  - `internal/capture`: the proxy uses `WaitGroup.Go` and logs per-route `Serve` errors.
  - `internal/capture` and `cmd/muproxy` tests read the client keys from `internal/protocol/crypto/testdata`, not the deleted `apps/client` folder, so the whole suite passes again.
  - Recorded the open `tradeOpen` unknown and the follow-ups (RNG globals, splitting `internal/game`) in `docs/server-plan/13-progresso.md`.
- Copied 13 Go skills (`golang-code-style`, `golang-testing`, `golang-error-handling`, `golang-security`, `golang-design-patterns`, `golang-performance`, `golang-concurrency`, `golang-context`, `golang-naming`, `golang-database`, `golang-safety`, `golang-project-layout`, `golang-documentation`) into `.claude/skills/`, and added a "Go skills" section to `CLAUDE.md` saying when to load each one for Go work.

## 2026-10-27

- Fixed players being disconnected when about 50 are in the same area (U-LOAD-01). The patente packet (`C1 F3 E5`) went to each player for every online player, every second and again for each new viewport entry, which overflowed the 1024-packet send queue. It now covers only the player and its viewport (`internal/game/patente.go` `patenteViewport`), and a new `C2 12` entry sends only that player's patente (`patenteEntry`, `custom_world_ext.go`). Test: `TestPatenteViewportOnly`. Reran the load test up to 200 players with no disconnects; results in `docs/server-plan/18-teste-de-carga.md`, notes in `10f-gm-eventos-custom.md` and `13-progresso.md`.
- Added a load test. `cmd/muload` runs steps of 1, 10, 50 and 100 fake players against a running server, samples its CPU and memory (`/debug/load`, `ps` or `docker stats`) and writes `samples.csv` and `summary.md`. `internal/loadbot` is the headless client: ConnectServer lookup, login, character select, walk, attack, chat and keepalive.
- Added load test flags to `muserver` (`cmd/muserver/metrics.go`, `seed.go`, `main.go`):
  - `-metrics-addr` serves pprof and `/debug/load`.
  - `-seed-load N` creates the `load0001..` accounts with one character each in a Lorencia monster field.
  - `-max-conn-per-ip` overrides `MaxConnectionPerIP` of the ConnectServer, JoinServer and GameServers.
  - `-max-users` overrides `ServerMaxUserNumber`.
- Added `docker-compose.loadtest.yml` (an overlay that turns on the load test flags and publishes port 6060) and `.env.loadtest.example` (its own variables: `MU_METRICS_ADDR`, `MU_METRICS_HOST_PORT`, `MU_SEED_LOAD`, `MU_MAX_CONN_PER_IP`, `MU_MAX_USERS` and a separate database port). The production `docker-compose.yml` and `.env.example` are unchanged; `.gitignore` keeps `.env.loadtest.example` versioned.
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
