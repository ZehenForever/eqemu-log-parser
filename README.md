# eqemu-log-parser
EverQuest Emulator combat log parser

> [!NOTE]
> Currently designed for use with the Imperium EQEmu server.  Not guaranteed to be accurate elsewhere.

## For developers

If you're looking to contribute or want a deeper architectural overview, see `DEVELOPING.md`.

## Getting started

### Prerequisites

- **Go** (1.21+ recommended)

### Run against a log file

Use `go run` to run directly from source:

```sh
go run ./cmd/eqlog parse --file /path/to/eqlog_Character_Server.txt
go run ./cmd/eqlog encounters --file /path/to/eqlog_Character_Server.txt
```

Or build a binary:

```sh
go build -o eqlog ./cmd/eqlog
./eqlog parse --file /path/to/eqlog_Character_Server.txt
./eqlog encounters --file /path/to/eqlog_Character_Server.txt
```

## What it does

This repository parses EverQuest combat log lines into structured events, then aggregates:

- Per-actor damage totals (melee vs non-melee)
- Per-target totals
- Encounter summaries (grouped by target)

Important details:

- Outgoing DPS/SDPS and encounters are driven only by **amount-bearing outgoing damage** events.
- Some lines are parsed into distinct event kinds but are intentionally excluded from damage totals and encounters:
  - **Heals** (e.g. "been healed")
  - **Incoming damage to you** (e.g. "You have taken ... by non-melee")

The CLI lives at `cmd/eqlog`.

## Commands

### `eqlog parse`

Reads a log and prints:

- A per-actor damage table
- A “top targets” table

Usage:

```sh
eqlog parse --file /path/to/eqlog_Character_Server.txt
```

### `eqlog encounters`

Reads a log and prints:

- A top-level encounter list (one row per encounter)
- A per-encounter actor table (damage and DPS metrics)

Usage:

```sh
eqlog encounters --file /path/to/eqlog_Character_Server.txt
```

#### Encounter timing

Encounter duration uses **inclusive seconds**:

`EncounterSeconds = (End - Start).Seconds() + 1` (clamped to at least 1)

#### Actor table columns

The per-encounter actor table includes:

- **DPS(enc)**: `TotalDamage / EncounterSeconds`
- **SDPS**: `TotalDamage / ActorActiveSeconds`
- **Sec**: `ActorActiveSeconds`

Actor-active time is also computed using **inclusive seconds**, but only from the actor’s own
amount-bearing damage events within the encounter:

`ActorActiveSeconds = (ActorLast - ActorFirst).Seconds() + 1` (clamped to at least 1)

This matches the common EQLogParser behavior where SDPS differs from encounter DPS when an
actor joins late or stops early.

## Encounter grouping and PC target filtering

By default, encounters are grouped by **target name**, but the `encounters` command filters out
encounters keyed on **likely player-character (PC)** targets.

This prevents “incoming damage to a PC” from appearing as a top-level encounter, while still
allowing you to view them when needed.

Additionally, the parser recognizes common heal and incoming-damage lines and ensures they do not
create encounters (for example, avoiding bogus targets like "been healed" or "by non-melee").

### Identity classifier

The tool infers an identity score for names seen in amount-bearing damage events and assigns:

- `LikelyPC`
- `LikelyNPC`
- `Unknown`

### Relevant flags

- `--include-pc-targets`
  - Include encounters keyed on likely-PC targets (restores legacy behavior).
- `--pc-threshold <n>`
  - Adjust the score threshold for `LikelyPC` classification.
- `--force-pc <Name>` (repeatable)
  - Force a name to be treated as PC regardless of score.
- `--force-npc <Name>` (repeatable)
  - Force a name to be treated as NPC regardless of score.
  - If a name is in both `--force-pc` and `--force-npc`, **force-npc wins**.
- `--debug-identities`
  - Print a concise identity table (`Name | Score | Class | Reasons`) before the encounter list.

Example:

```sh
eqlog encounters --file /path/to/eqlog.txt --debug-identities
eqlog encounters --file /path/to/eqlog.txt --include-pc-targets
eqlog encounters --file /path/to/eqlog.txt --force-npc Innoruuk
```

## Desktop UI (Wails)

A Wails-based desktop UI app lives under `cmd/eqlogui`.
The frontend uses React + React Router (`HashRouter`) + Tailwind.
The backend reuses the same Go parsing/engine packages as the CLI (`internal/parse`, `internal/engine`).
Log tailing runs in Windows-native Go (no WSL required).

Performance note:

- The dashboard polls a lightweight summary endpoint (`GetEncounterList`) that omits per-actor arrays.
- Per-encounter actor breakdown is fetched on demand via `GetEncounter`.
- Polling is adaptive: slower when not tailing, faster only while tailing.

### UI prerequisites

- **Go** (same as CLI)
- **Node.js + npm** (for the React frontend)
- **Wails v2 CLI** (`wails` must be on your `PATH`)
  - Install example:
    - `go install github.com/wailsapp/wails/v2/cmd/wails@latest`
- **Windows shell**
  - The provided `dev.cmd` works from `cmd.exe` or PowerShell.

### UI dev workflow (Windows)

From `cmd/eqlogui`:

```sh
npm install
dev.cmd
```

What this does:

- Starts the Vite dev server (`frontend/npm run dev`)
- Runs `wails dev` pointing at the external dev server (`http://127.0.0.1:5173`)

### Manual UI dev workflow

If you prefer to run the processes yourself:

Terminal 1 (from `cmd/eqlogui/frontend`):

```sh
npm install
npm run dev
```

Terminal 2 (from `cmd/eqlogui`):

```sh
wails dev -s -frontenddevserverurl http://127.0.0.1:5173
```
