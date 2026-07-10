# Shared Source Catalog And Detail Folder Action

## Status

Approved for implementation on 2026-07-10. This is an incremental design on top of `2026-07-10-mandatory-sync-provider-design.md`.

## Goals

- Show every source represented in the shared sync catalog on a new machine, even when that source has not been installed locally.
- Let one source card switch between Pull and Clone behavior based on local installation state.
- Restore every missing skill from that source after a successful Clone.
- Attach an existing local checkout directly from a missing source card.
- Keep repositories shallow while updating them without false fixed-depth divergence failures.
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
- Missing Git source: card with portable source ID, shared skill count, a clear Missing indicator, a Use Existing Checkout action, and a Clone action in the same button position normally occupied by Pull.
- Selecting either card filters the middle Skills panel by the portable source ID.
- Open, Rename, and Remove are not shown for a missing source because no local installation exists.

The Pull/Clone button keeps the cloud-download icon but has state-specific accessibility text and tooltip. Pull remains disabled for dirty local repositories. Clone is disabled only while another blocking operation is running or when the shared locator lacks a usable clone URL.

## Clone Destination Naming

Clicking Clone on a missing Git source opens a native directory picker for the parent directory. Cancel performs no action.

After selection, a compact confirmation dialog shows the repository, chosen parent directory, editable folder name, and final destination. It does not expose the Clone URL or other Git options.

The default folder name is `owner-repository`, derived from the final two components of the source ID. For example, `github.com/mattpocock/skills` becomes `mattpocock-skills`. The user may replace it before cloning. Empty names, `.`, `..`, absolute paths, and names containing path separators are rejected.

After confirmation, the application:

1. Clones the shared locator's clone URL into the named destination.
2. Validates that the cloned remote resolves to the expected source ID.
3. Adds the resulting local installation to v2 `config.json`.
4. Rescans and automatically reconciles all shared skills for that source.
5. Updates the same left-panel card from Missing to installed.

If the destination already exists, clone or validation fails, the card stays Missing and the normal error banner shows the concrete failure. No partial installation is added to local config.

The existing per-skill Install Repository action and configurable Clone modal are removed because installation is now source-scoped from the left panel.

## Use Existing Checkout

A missing Git source card provides a separate Use Existing Checkout folder action. It opens a native directory picker and validates the selected checkout before changing local config.

The selected directory must:

- be inside a Git repository;
- have a usable `origin` remote;
- normalize to the exact source ID of the selected missing card.

On success, the local installation is stored, the repository is rescanned, and all matching missing skills recover. On mismatch, the application reports both expected and selected source IDs and does not add the checkout. The global Add Repository flow remains available for adding unrelated new repositories.

## Shallow Repository Updates

New clones continue to use:

```bash
git clone --depth=1 --single-branch --no-tags <url> <destination>
```

Shallow repositories do not use `git pull --ff-only --depth=1`, because constraining every pull to a fixed depth may remove the visible connection between the old and new heads and cause a false divergence error. Increasing the fixed depth to two or three only reduces the failure rate when fewer commits arrive between pulls.

All installed repositories fetch only the checked-out branch into its remote-tracking ref:

```bash
git fetch --no-tags <remote> +refs/heads/<branch>:refs/remotes/<remote>/<branch>
```

Skill Manager validates the commit graph without refreshing the worktree index. If local `HEAD` is an ancestor of the fetched upstream, it advances through:

```bash
git reset --merge <upstream>
```

`--merge` updates only upstream-changed paths, preserves unrelated local changes, and immediately rejects overlapping local changes. This avoids the whole-index refresh performed by `git pull` or `git merge`, which can block indefinitely while macOS hydrates an unrelated iCloud placeholder file.

A repository cloned at depth one normally remains shallow. Fetch downloads the commits created after the original shallow boundary but does not retrieve older pre-clone history.

Some previously depth-limited checkouts can hold the old and new tips as disconnected shallow roots. When no merge base is visible and the checkout is still shallow, Skill Manager fetches 50 additional commits from the same upstream branch and checks ancestry again. This bounded recovery restores a visible merge base without a hard reset. A genuinely divergent local commit remains rejected.

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
- clone folder-name validation and editable owner-repository default;
- clone cancellation and failure without partial config writes;
- existing-checkout remote match, mismatch, and successful installation registration;
- shallow update across more commits than the original clone depth while remaining shallow;
- disconnected depth-one tips recovered through bounded history deepening;
- real local commit divergence refusal;
- unrelated local changes preserved during a fast-forward;
- overlapping local changes rejected without moving `HEAD`;
- source-folder button visibility for available and missing skills.

Final verification includes Go tests, TypeScript checking, frontend production build, Wails build, and a desktop smoke launch against the current v2 data.
