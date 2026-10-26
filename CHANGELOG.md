# Changelog

All changes to this project are recorded here, newest first.

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
