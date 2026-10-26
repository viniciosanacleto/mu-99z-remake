# Changelog

All changes to this project are recorded here, newest first.

## 2026-10-26

- Moved the server configuration to JSON files in `apps/server/config/`: `server.json` (ConnectServer, ServerList, BlackList, JoinServer, BadSyntax), `gameservers/<instance>.json` (the eight `GameServerInfo - *.dat` files as sections) and `data/**.json` (the `MuServer/Data` tables), with the original section, key and column names; the converted `.ini`/`.dat`/`.txt` files and four admin notes were removed from `apps/server/data/`, which keeps only terrain, keys, Lua scripts, `SkillElect.hit`, images and `MuOnline.sql`.
- Added `internal/config/jsonconf` (JSON tables and settings that give the loaders the original tokens and `GetPrivateProfile*` semantics, Windows-1252 text support) and `mutext.TokenizeScript`; `gsconfig`, `serverlist`, `connect.LoadConfig` and every `internal/gamedata` / `internal/game` loader now read `config/`.
- Added `cmd/configconv` to convert an original MuServer folder into `config/`, checking that every table gives back the original tokens.
- `muserver` and `mueditor` take `-config` (JSON) and `-data` (original-format files); `-blacklist` takes a JSON table; `muproxy` takes `-gs-config` instead of `-gs-data`.
- The XShop GM price change, the XShop recharge upload and the Castle Siege event key reset now write JSON (`gamedata.WriteXShopPrice`, `game.rechargeSave`, `gsconfig.Config.WriteKeys`).
- Documented the JSON layout in `docs/server-plan/11-arquivos-de-configuracao.md` §0 and updated `13-progresso.md`, `15-captura.md`, `16-cobertura-dados.md`, `apps/server/README.md`, `README.md` and `CLAUDE.md`.
- Added the project `README.md` for GitHub, pointing to `apps/server/README.md` for running the server.
- Added this `CHANGELOG.md` and the rule in `CLAUDE.md` to record every change in it.
