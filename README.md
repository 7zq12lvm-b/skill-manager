# AI Agent Skill Manager

A Wails v2 desktop app for managing local AI Agent skills across multiple source directories.

The app scans configured skill sources, shows discovered first-level skill folders, derives their sync status against one or more target directories, and only mutates configured target directories through explicit enable, disable, or conflict-resolution actions.

## Stack

- Desktop shell: Wails v2
- Backend: Go
- Frontend: React + TypeScript
- UI: Tailwind CSS with shadcn/ui-style primitives
- State: Zustand
- Config: JSON under the user config directory
- Shared sync state: SQLite (`skillManager.db`) in the selected sync folder
- File watching: fsnotify

## Development

```bash
wails dev
```

Wails also exposes a browser dev server at `http://localhost:34115` while `wails dev` is running.

## Build

```bash
wails build
```

The macOS build output is written under `build/bin/`.

## Test

```bash
go test ./...
cd frontend && npm run build
```

## One-time sync data migration

The app only reads and writes `skillManager.db`; it does not automatically import the retired `skill-manager-sync.json` file. Migrate an existing shared catalog once with:

```bash
go run ./scripts/migrate-sync-json \
  --source "/path/to/skill-manager-sync.json" \
  --destination "/path/to/skillManager.db"
```

The command refuses to overwrite an existing destination, validates the completed database before publishing it, and leaves the source JSON unchanged.
