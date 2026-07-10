# Shared Source Catalog And Detail Folder Action

## Status

Approved for implementation on 2026-07-10. This is an incremental design on top of `2026-07-10-mandatory-sync-provider-design.md`.

## Goals

- Show every source represented in the shared sync catalog on a new machine, even when that source has not been installed locally.
- Let one source card switch between Pull and Clone behavior based on local installation state.
- Restore every missing skill from that source after a successful Clone.
- Move the global target-folder action into Skill Detail and make it open the selected skill's original source folder.

## Shared Source Projection

The backend derives a source summary from `skill-manager-sync.json` by grouping skill records by `source.provider + source.id`.

Each summary contains:

- provider and portable source ID;
- provider locator data needed for installation, including Git clone URL;
- number of shared skills belonging to the source;
- local installation state;
- local path, alias, current ref, dirty state, scan result, and error state when installed.

The projection is returned in the existing repository area of Inventory so the left panel has one ordered list. Local installation metadata is merged into the matching shared summary. A locally installed source that currently has no shared skills remains visible until the user removes it.

Git is the only provider implemented now, but grouping is provider-scoped so a future skills.sh source cannot collide with a Git source using the same ID.

## Repository Panel

The left panel displays all projected shared sources.

- Installed Git source: normal card with local path, ref, dirty/error indicator, skill count, Open, Rename, Pull, and Remove actions.
- Missing Git source: card with portable source ID, shared skill count, a clear Missing indicator, and a Clone action in the same button position normally occupied by Pull.
- Selecting either card filters the middle Skills panel by the portable source ID.
- Open, Rename, and Remove are not shown for a missing source because no local installation exists.

The Pull/Clone button keeps the cloud-download icon but has state-specific accessibility text and tooltip. Pull remains disabled for dirty local repositories. Clone is disabled only while another blocking operation is running or when the shared locator lacks a usable clone URL.

## One-step Clone

Clicking Clone on a missing Git source opens a native directory picker for the parent directory. Cancel performs no action.

After selection, the application:

1. Derives the destination folder name from the source ID, removing a trailing `.git` when present.
2. Clones the shared locator's clone URL into that folder.
3. Validates that the cloned remote resolves to the expected source ID.
4. Adds the resulting local installation to v2 `config.json`.
5. Rescans and automatically reconciles all shared skills for that source.
6. Updates the same left-panel card from Missing to installed without opening the existing Clone modal.

If the destination already exists, clone or validation fails, the card stays Missing and the normal error banner shows the concrete failure. No partial installation is added to local config.

The existing per-skill Install Repository action and configurable Clone modal are removed because installation is now source-scoped from the left panel.

## Skill Detail Folder Action

The global top-bar folder button is removed.

Skill Detail Header places a Finder folder icon next to the existing VS Code icon. It opens `selectedSkill.sourcePath`, which is the original skill directory rather than the managed target directory.

The Finder button is not rendered when:

- no skill is selected;
- status is `missing-source` or `missing-path`;
- `sourcePath` is empty.

Its tooltip is `Open this skill's original source folder in Finder.` The VS Code action and behavior remain unchanged.

## Verification

Tests cover:

- grouping multiple shared skills into one source summary;
- merging a local installation into a shared source summary;
- preserving a local installation with no shared records;
- missing source card state and source-based filtering;
- Pull versus Clone action selection;
- clone destination name derivation and successful installation registration;
- clone cancellation and failure without partial config writes;
- source-folder button visibility for available and missing skills.

Final verification includes Go tests, TypeScript checking, frontend production build, Wails build, and a desktop smoke launch against the current v2 data.
