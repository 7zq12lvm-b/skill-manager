# AI Agent Skill Manager

A Wails v2 desktop app for managing local AI Agent skills across multiple source directories.

The app scans configured skill sources, shows discovered first-level skill folders, derives their sync status against a target directory, and only mutates `~/.agents/skills` through explicit enable, disable, or conflict-resolution actions.

## Stack

- Desktop shell: Wails v2
- Backend: Go
- Frontend: React + TypeScript
- UI: Tailwind CSS with shadcn/ui-style primitives
- State: Zustand
- Config: JSON under the user config directory
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
