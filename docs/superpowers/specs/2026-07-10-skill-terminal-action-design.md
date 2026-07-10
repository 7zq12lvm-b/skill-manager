# Skill Terminal Action

## Status

Approved for implementation on 2026-07-10.

## Goal

Add a Terminal button to the Skill Detail header that opens macOS Terminal.app with the selected skill's original source directory as the working directory.

## Interaction

- Place a Lucide terminal icon beside the existing Finder and VS Code actions.
- Show the action only when the selected skill has a usable `sourcePath` and is not in a missing-source or missing-path state.
- Use a functional tooltip that explains both the application and the resulting working directory.
- Open a new Terminal.app window through Launch Services by passing the source directory directly to the Terminal bundle. Do not construct or quote a shell `cd` command.

## Backend

Expose `OpenInTerminal(path)` through Wails. The method:

- rejects an empty path;
- verifies that the path exists and is a directory;
- supports macOS Terminal.app explicitly;
- launches bundle `com.apple.Terminal` with the directory as its document argument;
- returns actionable errors through the existing application error banner.

## Verification

- Go tests cover empty, missing, and non-directory path validation without launching Terminal.
- TypeScript and frontend production builds verify the generated binding and button wiring.
- Wails build verifies packaging.
- Desktop smoke testing confirms visibility, tooltip accessibility, icon placement, and that Terminal.app opens at the selected skill source directory.
