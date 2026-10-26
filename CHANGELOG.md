# Changelog

All changes to this project are recorded here, newest first.

## 2026-10-26

- Neutralized server and emulator branding in `apps/server/data/`: removed the WizardTeam/SSeMU banners and "OriX" operator notes, made the player-facing texts (Message 990, Notice, window title and welcome Lua scripts, 4 quiz questions) say "MU Online", emptied and disabled `Custom/CustomXShopRecharge.txt`, removed `Custom/pixFE31.tmp.bmp`, and moved the `DB/MuOnline.sql` seed accounts to `@example.com`.
- Added a comment to `GameServerInfo - Common.dat` saying `CustomerName` is the stream cipher key and must match the client's `MainInfo.ini`.
- Neutralized emulator names in Go and SQL migration comments, `docs/server-plan/`, both READMEs and `CLAUDE.md`; renamed `docs/server-plan/10e-customs-wizardteam.md` to `10e-customs.md`.
- Updated the tests that read the changed data (`internal/game/lua_test.go`, `xshop_test.go`, `datacoverage_test.go`).
- Added the project `README.md` for GitHub, pointing to `apps/server/README.md` for running the server.
- Added this `CHANGELOG.md` and the rule in `CLAUDE.md` to record every change in it.
