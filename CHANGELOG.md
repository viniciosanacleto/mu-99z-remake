# Changelog

All changes to this project are recorded here, newest first.

## 2026-10-26

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
