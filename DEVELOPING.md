# Developing eqemu-log-parser

This document is intended for contributors and developers. For basic usage, see the main `README.md`.

## Repository overview

This repo has two primary entrypoints:

- **CLI**: `cmd/eqlog` (parses a log file, prints tables)
- **Desktop UI**: `cmd/eqlogui` (Wails v2 backend + React/Tailwind frontend)

Both share the same core libraries under `internal/`.

### High-level data flow

Most workflows follow this pipeline:

- **Log lines** (EverQuest combat log)
- `internal/parse` parses each line into a `model.Event`
- `internal/engine` ingests events and maintains aggregates:
  - overall actor totals (`engine.Engine`)
  - per-target encounters (`engine.EncounterSegmenter`)
- the CLI prints tables; the UI exposes snapshot APIs over Wails

## Directory layout

### `cmd/`

- `cmd/eqlog`
  - CLI program.
  - Commands:
    - `parse`: actor/target totals
    - `encounters`: encounter list and per-encounter actor tables
- `cmd/eqlogui`
  - Wails desktop app.
  - Go backend in `cmd/eqlogui/*.go`
  - Frontend in `cmd/eqlogui/frontend`

### `internal/`

- `internal/model`
  - Shared event and enum types used across parsing and aggregation.
- `internal/parse`
  - Parsing and classification from raw log lines into `model.Event`.
  - Timestamp parsing is based on the log prefix: `[Mon Jan 02 15:04:05 2006]`.
- `internal/engine`
  - Aggregation and derived views.
  - Key components:
    - `engine.Engine`: simple per-actor totals and top targets.
    - `engine.EncounterSegmenter`: target-based encounter segmentation and per-encounter aggregation.
    - `snapshot.go`: converts internal encounters into UI/CLI-friendly `EncounterView`/`Snapshot`.
    - `identity.go` + `filter.go`: name identity heuristics (LikelyPC/LikelyNPC) and filtering.
- `internal/tail`
  - Windows-native file tailing used by the CLI (`--follow`) and UI.

### `testdata/`

- Contains real log fixtures used by regression tests.

### `bin/`

- Small helper scripts used during development.

## Development prerequisites

### Go

- The repo is currently `go 1.22`.
- Install Go and ensure `go` is on your `PATH`.

### UI prerequisites (Wails + Node)

For the Wails UI (`cmd/eqlogui`):

- **Node.js + npm**
- **Wails v2 CLI**
  - Example install:

```sh
go install github.com/wailsapp/wails/v2/cmd/wails@latest
```

## Common workflows

### Run CLI from source

```sh
go run ./cmd/eqlog parse --file /path/to/eqlog_Character_Server.txt
go run ./cmd/eqlog encounters --file /path/to/eqlog_Character_Server.txt
```

### Build CLI

```sh
go build -o eqlog ./cmd/eqlog
```

### Run tests

From repo root:

```sh
go test ./...
```

Notes:

- Most unit tests live under `internal/engine` and `internal/parse`.
- Regression tests use fixtures under `testdata/`.

## Desktop UI (Wails) developer workflow

### Dev mode (Windows)

From `cmd/eqlogui`:

```sh
npm install
./dev.cmd
```

What `dev.cmd` does:

- Starts the Vite dev server (`cmd/eqlogui/frontend`)
- Runs `wails dev` pointed at `http://127.0.0.1:5173`

### Manual dev mode

Terminal 1 (from `cmd/eqlogui/frontend`):

```sh
npm install
npm run dev
```

Terminal 2 (from `cmd/eqlogui`):

```sh
wails dev -s -frontenddevserverurl http://127.0.0.1:5173
```

### Production build

From `cmd/eqlogui`:

```sh
npm install
wails build
```

Wails configuration lives in `cmd/eqlogui/wails.json`.

## Architecture notes

### Encounter segmentation

Encounters are keyed by **target name** and segmented using an **idle timeout**. Duration uses **inclusive seconds**.

Derived metrics are computed from aggregates:

- Encounter DPS: `TotalDamage / EncounterSeconds`
- Actor SDPS: `ActorTotalDamage / ActorActiveSeconds`

### UI API shape

The Wails backend (`cmd/eqlogui/app.go`) owns tailing, caching, and exposes methods like:

- snapshot/encounter list
- encounter detail fetch
- per-actor damage breakdown fetch

The frontend is a React app under `cmd/eqlogui/frontend`.

## Contributing

### Ground rules

- Prefer **additive changes** (new types/functions/tests) when possible.
- Keep parsing rules and encounter math stable unless a change is explicitly intended and tested.
- Add or update tests for behavior changes.

### Suggested contribution process

- Create a feature branch
- Keep commits small and focused
- Run:

```sh
go test ./...
```

- If you touch the UI:
  - ensure the frontend still builds (`npm run build`)
  - ensure Wails dev/build works

## Where to start when exploring the code

- Parsing:
  - `internal/parse/parse.go` (`ParseLine`, regexes, event classification)
- Encounter math and views:
  - `internal/engine/encounters.go`
  - `internal/engine/snapshot.go`
- Wails backend glue:
  - `cmd/eqlogui/app.go`
  - `cmd/eqlogui/viewmodel.go`
- React UI:
  - `cmd/eqlogui/frontend/src/pages/Dashboard.jsx`
  - `cmd/eqlogui/frontend/src/pages/EncounterDetail.jsx`
