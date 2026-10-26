# MU Remake

A from-scratch rewrite of a MU Online server (season 97D/99z) in Go, built by reverse engineering the original server binaries. The original game client keeps working unchanged: it connects to the new server with the same network protocol it always used.

## Why

A classic MU Online server setup runs five separate Windows processes on top of SQL Server:

| Original process | Role |
|---|---|
| ConnectServer | Gives the client the list of available servers |
| JoinServer | Account authentication and sessions |
| DataServer | Bridge between the game servers and the database |
| GameServer | The game world: characters, maps, monsters, items, events, commands |
| GameServerCS | A GameServer instance dedicated to Castle Siege |

This project merges all of them into **one Go service** backed by **PostgreSQL**, and keeps the client-facing protocol byte-compatible.

## What has been done

- **Reverse engineering.** The original emulator and its client were decompiled and analyzed with Ghidra through the REA MCP server. Every packet layout, formula and data structure in the rewrite is grounded in evidence from the binaries, their data files or the original database schema. The findings are written up in [`docs/server-plan/`](docs/server-plan/), with each claim marked as observed, inferred or unknown.
- **Network protocol.** C1–C4 packet framing, SimpleModulus, XOR32 and the GameServer stream cipher, plus the original anti-hack packet checks.
- **ConnectServer and login.** Server list, authentication, single-session enforcement, IP/HWID limits, VIP expiry and map-server transfers. Passwords are stored with argon2id.
- **Game world.** Movement, gates and teleports, chat and commands, monsters with AI and pathfinding, combat and damage formulas, experience and leveling, items, inventory, shops, warehouse, trade, parties, PK, guilds, quests, chaos mixes, pets, friends and mail.
- **Events and custom systems.** Blood Castle, Devil Square, Chaos Castle, Illusion Temple, Kalima, Castle Siege, GM events, and the WizardTeam customs (XShop, LuckyWheel, JewelBank, marriage, arena, resets, rankings, Lua scripts).
- **Configuration.** The original `.ini`, `.dat` and `.txt` configuration was converted into TOML files in [`apps/server/config/`](apps/server/config/) (`cmd/configconv` keeps the original column names, comments and reader quirks); binary files such as terrain and Lua scripts stay in [`apps/server/data/`](apps/server/data/). A test loads every table the original binary reads.
- **PostgreSQL.** Schema and embedded migrations that keep the binary column layouts of the original database (inventory, skills, etc.).
- **Tooling.**
  - `cmd/migrate` imports an existing MuOnline database from SQL Server.
  - `cmd/muproxy` is a capture proxy that decodes traffic between the client and a server and records golden sessions.
  - `cmd/mueditor` is a web editor for offline characters.
  - `cmd/configconv` converts the original configuration files into the TOML files of `apps/server/config`.
- **Tests.** Each module ships with unit tests checked against the values documented in `docs/`.

Some bugs of the original server are reproduced on purpose and listed in the progress log until a decision is made about each one.

## Status

The server is implemented and its test suite passes. End-to-end testing with the original client is the current step. Progress, open items and the next step are tracked in [`docs/server-plan/13-progresso.md`](docs/server-plan/13-progresso.md).

## Running the server

See **[`apps/server/README.md`](apps/server/README.md)** for requirements, setting up PostgreSQL, creating an account, starting the service, pointing the client at it, importing an original database and running the tests.

## Repository layout

| Path | Contents |
|---|---|
| [`apps/server/`](apps/server/) | The Go service (`github.com/viniciosanacleto/mu-remake/apps/server`), its TOML configuration (`config/`) and the original binary data (`data/`) |
| [`docs/server-plan/`](docs/server-plan/) | Reverse-engineering specs: architecture, protocol, persistence, formulas, game systems, events, roadmap and open unknowns |

The documentation in `docs/` and the server README are written in Portuguese.
