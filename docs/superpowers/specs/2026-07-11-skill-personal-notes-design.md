# Skill Personal Notes Design

## Goal

Add a cross-device personal note to every synced skill. Notes are multiline plain text, visible in the Skills table and shown before the other content in Skill Detail.

## Data Model

- Add an optional `note` string to `SyncSkillRecord` in `skill-manager-sync.json`.
- Add the same optional field to the runtime `Skill` model.
- Keep sync document version 2. Existing documents remain valid because the field is optional.
- Trim leading and trailing whitespace when saving. An empty value clears the note and is omitted from JSON.
- Copy the note through installed and missing-source skill projections so it survives rescans and restores on another device.

The note belongs inside each skill record, beside `tags` and `enabled`. A separate top-level notes index would duplicate skill identity and cleanup logic without adding useful behavior.

## Backend Interface

Expose two operations:

```text
SaveSkillNote(skillID, note) -> Inventory
RemoveMissingSkill(skillID) -> Inventory
```

The `SaveSkillNote` operation:

1. Resolves the selected skill and requires it to exist in the shared catalog.
2. Builds the current sync record without changing enabled state, tags, profile, source locator, or target name.
3. Replaces only the normalized note.
4. Writes the sync document atomically through `SyncStore`.
5. Refreshes and returns the inventory.

`RemoveMissingSkill` is intentionally restricted to a `missing-source` skill whose repository is installed on this machine but whose recorded skill path is absent. It removes managed target symlinks that still point to the missing skill path, deletes the skill record from the shared Sync JSON, refreshes inventory, and returns the result. It must not remove repository files or accept valid skills. It also rejects a missing skill when the whole repository checkout is unavailable, because that condition is recoverable by Clone or Use Existing Checkout.

## Skills Table

- Add a `Note` column after `Tags` and before `Source`.
- A saved note displays as a maximum two-line preview.
- An empty note displays a small plus icon.
- Clicking the cell opens a compact multiline editor without selecting the row.
- Blurring the editor saves changed content.
- `Cmd+Enter` on macOS or `Ctrl+Enter` elsewhere saves and closes immediately.
- `Escape` closes and restores the last saved value.
- While saving, the editor is disabled and remains open if saving fails so the draft is not lost.

The note is not included in search or bulk actions in this change.

## Skill Detail

- Insert `Personal Note` immediately below the skill title, description, and status, before `Paths`.
- Show the complete note in a multiline editor rather than a truncated preview.
- When empty, show an explicit add-note affordance.
- Use the same blur, shortcut, cancellation, loading, and error behavior as the table editor.

Both surfaces update through the same store action and receive the refreshed inventory returned by the backend.

## Unified Missing Source Status

Expose a single `missing-source` runtime status and display it as `Missing Source`. Remove the `missing-path` status from the model, filters, labels, status pills, and tests. The unified status means that the synced Skill cannot currently be found, whether the whole repository checkout is unavailable or the repository exists but its recorded `repoSubpath` no longer contains a valid Skill.

- When the repository checkout is unavailable, recovery remains a repository-level action: Clone or Use Existing Checkout. Do not show the Skill removal button in this case.
- When the repository is installed but the Skill path is absent, show a trash icon in the Skill Detail toolbar. Deleting or moving the original Skill folder, or removing its required `SKILL.md`, produces this form of Missing Source after a rescan.
- Its tooltip explains that the action removes the missing skill from the shared catalog on every synced device; it does not delete repository files.
- Require a confirmation dialog naming the skill and explaining the cross-device effect.
- On confirmation, remove only managed target symlinks that still refer to this skill, then delete its record from `skill-manager-sync.json`.
- Close the confirmation dialog and select the next available skill after the refreshed inventory arrives.

The UI therefore has one simple status while the backend retains enough repository-installation context to choose the safe recovery or removal action.

## Responsive Behavior

- The new table column participates in the existing resizable percentage-width system.
- Allocate space primarily from the current Skill and Tags columns while preserving the fixed selection and toggle controls.
- Note content wraps only inside its two-line preview and must not widen the table.
- In compact mode, the Skills table remains horizontally contained; narrow content truncates instead of creating page-level horizontal overflow.

## Testing

Backend tests cover:

- Saving and trimming a note without changing other sync fields.
- Clearing a note and omitting it from serialized JSON.
- Loading notes from an existing sync document.
- Applying notes to installed and missing skills after refresh.
- Projecting both a missing repository and a missing path as `missing-source`.
- Rejecting removal unless the current status is `missing-source`, the repository is installed, and the recorded Skill path is absent.
- Removing a missing skill record and its matching managed target symlink without touching unrelated paths.

Frontend verification covers:

- Empty and populated Note cells.
- Blur save, keyboard save, and Escape cancellation.
- Detail placement before Paths and immediate refresh after save.
- Missing Source recovery-versus-removal visibility, confirmation, cancellation, and post-removal selection.
- Desktop, split, and compact layouts without horizontal page overflow.

Production verification runs TypeScript/Vite build, Go tests, Go vet, and `wails build`.
