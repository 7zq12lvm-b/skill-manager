# Mandatory Sync Provider Design

## Status

Approved for implementation on 2026-07-10. This design replaces the optional-sync behavior described in `2026-07-06-icloud-skill-sync-design.md`.

The application has one user. The implementation may rewrite that user's current JSON files directly and does not need runtime compatibility with the old schemas.

## Goals

- Make cross-device sync mandatory for every managed skill.
- Remove manual adoption and application of sync state.
- Keep machine-specific paths local while sharing portable skill source information.
- Support Git repositories now without embedding Git assumptions in the core skill model.
- Leave a clean provider boundary for a future registry source such as skills.sh.
- Consolidate tags into the shared sync document and remove the legacy tag file.
- Add one-click enablement for all eligible skills in the current filtered Skills view.

## Non-goals

- Implement a skills.sh provider.
- Support non-Git local folders in this release.
- Preserve or parse the old config, sync, or tag schemas at runtime.
- Encrypt the LLM API key. It remains in the shared sync JSON as requested.
- Automatically overwrite occupied target paths or choose between conflicting skills.

## Source Provider Model

The core model separates a portable source description from a machine-local installation.

`SourceDescriptor` is stored in the shared sync document:

```json
{
  "provider": "git",
  "id": "github.com/mattpocock/skills",
  "locator": {
    "cloneUrl": "https://github.com/mattpocock/skills",
    "subpath": "skills/example",
    "ref": "main"
  }
}
```

- `provider` selects the source implementation.
- `id` is the provider-scoped portable source identifier.
- `locator` is provider-owned JSON. Core sync code stores it without interpreting provider-specific fields.

`SourceInstallation` is stored in local config:

```json
{
  "provider": "git",
  "sourceId": "github.com/mattpocock/skills",
  "path": "/Users/example/Code/mattpocock-skills",
  "alias": "Matt Pocock Skills",
  "enabled": true,
  "options": {
    "scanRoots": ["."],
    "ignorePaths": []
  }
}
```

The backend exposes a provider interface responsible for:

- validating an imported local source;
- deriving its portable ID and locator;
- discovering skills;
- resolving a shared skill to a local path;
- installing a missing source when supported;
- listing skill files.

Only `GitSourceProvider` is implemented now. Importing a folder that is not a Git repository with a usable remote is rejected with a clear cross-device-sync requirement message. A future skills.sh provider can add its own locator and installation behavior without changing sync reconciliation or skill UI logic.

## Local Configuration

`~/Library/Application Support/skill-manager/config.json` uses schema version 2 and stores only machine-specific state:

- selected sync folder;
- target directories;
- source installations and local paths;
- source aliases and provider-specific scan options;
- scan and watcher settings;
- conflict handling settings.

It does not store the shared skill catalog, tags, profiles, or desired enabled state.

The current local config is rewritten once during this implementation to the v2 shape. The running application only reads the v2 shape afterward.

## Shared Sync Document

`skill-manager-sync.json` uses schema version 2 and is the catalog and desired-state authority:

```json
{
  "version": 2,
  "llm": {
    "baseUrl": "https://example.invalid/v1",
    "apiKey": "plaintext-as-requested",
    "model": "example-model"
  },
  "profiles": {},
  "skills": {
    "git:github.com/mattpocock/skills//skills/example": {
      "enabled": false,
      "targetName": "example",
      "tags": ["typescript"],
      "updatedAt": "2026-07-10T00:00:00Z",
      "source": {
        "provider": "git",
        "id": "github.com/mattpocock/skills",
        "locator": {
          "cloneUrl": "https://github.com/mattpocock/skills",
          "subpath": "skills/example",
          "ref": "main"
        }
      }
    }
  }
}
```

Skill IDs are provider-prefixed to avoid collisions between Git and future providers.

Tags are stored only in each shared skill record. The existing iCloud `tags.json` is merged into the new sync document while rewriting current data, then deleted. No tag migration or legacy tag store remains in application code.

## Setup And Startup

When no sync folder is configured, the application shows a focused setup screen instead of the empty three-panel workbench. The user selects a shared folder before managing skills.

After selection:

1. Create an empty v2 sync document if none exists.
2. Load the shared catalog if it exists.
3. Scan configured local source installations.
4. Match shared source descriptors to local installations.
5. Reconcile desired enabled states to local target links.
6. Show missing sources and paths in the normal Skills view.

Changing the sync folder remains available in Settings. Clearing it is not allowed. Switching folders reloads and reconciles against the newly selected document.

## Automatic Sync

There is no manual sync lifecycle.

- Adding a Git source writes every discovered skill to the shared catalog immediately.
- A newly discovered skill defaults to disabled unless it is already correctly active on this machine, in which case its initial shared state is enabled.
- Enabling or disabling first writes the desired state to the shared document, then applies the local target link.
- Startup, Rescan, source changes, and shared sync-file changes trigger reconciliation.
- The sync folder is watched with debounce. Self-authored writes are recognized so they do not create reconciliation loops.
- A failed local apply leaves the desired shared state intact, exposes a concrete error, and is retried by a later reconciliation.

The following commands and UI concepts are removed:

- Adopt Current Enabled Skills;
- Apply Sync;
- Enable Local Only;
- Remove From Sync;
- membership in sync as a separate skill property or user decision.

## Skill Statuses

The user-facing status set becomes:

- `enabled`: shared desired state is enabled and every configured target has the correct link;
- `disabled`: shared desired state is disabled and no matching target link is active;
- `conflict`: a target is occupied, points elsewhere, or multiple skills claim one target name;
- `invalid`: the source folder fails skill validation;
- `missing-source`: no local installation can resolve the shared source descriptor;
- `missing-path`: the source is installed but the shared skill path is absent;
- `error`: file I/O, sync document, provider, or reconciliation failure.

The old `synced`, `local-only`, `needs-apply`, `syncing`, and generic `missing` statuses are removed. Sync is the default operating model rather than a status.

The application never deletes a non-symlink target or silently replaces a link pointing to another source.

## User Interface

- Remove Adopt and Apply Sync from the global toolbar.
- Remove local-only and remove-from-sync actions from Skill Detail.
- Keep Sync Folder settings, but present the folder as required storage rather than an optional feature.
- Keep missing-source installation actions for Git repositories.
- Update status labels, filters, summaries, tooltips, and empty states to the reduced model.
- Continue showing tags from the shared skill records.

Add `Enable all` to the middle Skills panel toolbar. It operates on the exact current filtered list, including source, search, tag, and status filters. It enables eligible disabled skills, ignores skills already enabled, and skips conflict, invalid, missing, and error items. The completion message reports enabled, already enabled, and skipped counts. The button is disabled when the filtered list contains no eligible skill.

## Failure Handling

- Invalid or unreadable shared JSON blocks reconciliation and displays the file path and parse error.
- Sync writes use a temporary file and atomic rename in the same directory.
- Provider validation errors identify the selected source and explain that this release requires a Git repository with a usable remote.
- Missing sources stay visible and installable rather than disappearing from the catalog.
- Bulk enable continues through independent skills and reports partial failure without resolving conflicts automatically.

## Verification

Automated tests cover:

- v2 config and sync serialization;
- Git provider validation, discovery, resolution, and missing-source behavior;
- first-run setup with new and existing sync folders;
- automatic seeding of all discovered Git skills;
- automatic enable and disable reconciliation;
- sync-file watcher debounce and self-write suppression;
- simplified status derivation and conflict protection;
- bulk enable filtering, eligibility, partial failure, and result counts;
- tag reads and writes through the sync document only.

The final verification includes Go tests, frontend type checking and production build, Wails build, and a clean diff check. The current local JSON files are inspected after rewrite, and the legacy iCloud `tags.json` must no longer exist.
