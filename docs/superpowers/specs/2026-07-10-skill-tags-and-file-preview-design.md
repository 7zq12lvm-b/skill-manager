# Skill Tag Workflows And File Preview

## Status

Approved for implementation on 2026-07-10. This design extends the current Skills table and Skill Detail file tree without changing the v2 sync data model.

## Goals

- Move tags out of the Skill name cell into a dedicated Tags column.
- Support one-step tag creation or selection for one skill.
- Support selecting multiple visible skills and applying one or more tags to all of them.
- Use the same selection model for bulk Enable and Disable actions.
- Make the Skill Detail file tree control the preview panel while preventing binary content from rendering as garbled text.

## Considered Approaches

For bulk actions, the chosen approach is a persistent selection checkbox column plus a contextual toolbar. Reusing the active detail row would only address one skill, while adding selection controls inside each action popover would hide selection state and make Enable and Disable inconsistent.

For bulk writes, the chosen approach is one backend operation per user action followed by one inventory refresh. Calling the existing single-skill methods repeatedly from React would trigger repeated disk writes and rescans, expose partial completion in the UI, and scale poorly.

For previews, the chosen approach is a backend text-file reader with path containment, size, UTF-8, and binary checks. An extension allowlist would reject valid text files without known extensions and could still display renamed binary files.

## Skills Table

The table columns become:

1. Selection
2. On
3. Skill
4. Tags
5. Source
6. Status

Tags are removed from beneath the Skill description. The Tags column displays the existing deterministic colored chips followed by a small Plus icon button. The button remains visible when the skill has no tags. Its tooltip explains that it can find an existing tag or create a new one for that skill.

Adding the new columns updates the persisted width schema. Older saved widths are normalized by filling missing columns from the new defaults, so existing local preferences do not break the table.

## Single-Skill Tag Addition

Clicking the Plus button opens a compact anchored popover without selecting or opening the row. It contains:

- a focused search input;
- matching tags from the complete inventory, excluding tags already assigned to the skill;
- an empty-state action represented by the typed text when no exact tag exists.

Clicking an existing result immediately appends that tag, saves, and closes the popover. Typing a new tag and pressing Enter does the same. Empty input and duplicate tags do nothing. Tag normalization continues to trim, deduplicate, and sort values through the existing tag rules.

The full Tags editor in Skill Detail remains available for removing tags and editing the selected skill.

## Selection And Bulk Actions

Each skill row has a checkbox. The header checkbox selects or clears all skills in the current filtered result. It is indeterminate when only part of the current filtered result is selected.

Selection is stored by skill ID. Filtering does not silently discard selected skills, but the bulk toolbar reports the total selected count and actions operate on the selected IDs. Skills that disappear from Inventory after a rescan are removed from selection.

When at least one skill is selected, the Skills panel header shows:

- selected count;
- Enable;
- Disable;
- Add Tags;
- Clear Selection.

The previous Enable all button is removed. Enable and Disable act only on selected skills. Enable skips skills that are already enabled or cannot be enabled because they are missing, invalid, conflicting, or in error. Disable skips skills that are already disabled or unavailable. The result message reports changed, unchanged, skipped, and failed counts. Row checkboxes and all bulk buttons stop row-click propagation.

## Bulk Tag Addition

Add Tags opens a compact multi-select popover. The user can search and toggle multiple existing tags, or type a new tag and press Enter to add it to the pending set. A final `Add to N Skills` command appends every pending tag to every selected skill.

Bulk tag addition never removes or replaces existing tags. Duplicate tags are ignored. The backend updates all shared records in one operation and refreshes Inventory once. If validation fails before writing, no record is changed. Disk or sync-store failures are surfaced through the existing error banner.

## File-Driven Preview

Skill Detail initially previews the scanned default file, normally `SKILL.md`, and the section title is `Preview: SKILL.md`. The file tree receives the current preview path and reports file selections to its parent.

Clicking a file row:

1. marks that file as selected in the tree;
2. changes the section title to `Preview: <relative path>`;
3. shows a loading state while content is read;
4. renders the content only when the backend classifies it as previewable text.

Clicking a directory only expands or collapses it and does not change the preview.

When a selected file is binary, invalid, unavailable, or too large, the preview panel replaces prior content with a clear non-previewable or error message for that file. It never leaves stale content visible under the new filename.

## Safe File Reading

A new Wails backend method accepts a skill ID and relative file path and returns a structured preview result containing the normalized path, previewability, content when previewable, and a reason when it is not.

The service:

- rejects empty, absolute, parent-traversal, and directory paths;
- confines local reads to the selected skill source directory;
- reads repository-backed content from the current Git `HEAD` when repository metadata is available, matching the existing tree listing behavior;
- enforces a bounded preview size before returning content;
- treats NUL-containing or invalid UTF-8 data as binary;
- returns an explicit non-previewable result instead of converting arbitrary bytes to text.

The existing scanned preview remains the initial fast path, so opening Skill Detail does not require another backend request for `SKILL.md`.

## Components And Data Flow

- `App` owns selected skill IDs, single-tag popover state, bulk toolbar state, and inventory-wide tag suggestions.
- A focused tag picker component supports immediate single-skill mode and staged bulk mode.
- New store methods expose bulk Enable, bulk Disable, bulk Add Tags, and file preview reads.
- Backend bulk methods validate IDs, update the sync store, perform required link operations, and refresh Inventory once.
- `SkillDetail` owns the active preview result for the selected skill and resets it to the scanned default whenever the skill changes.
- `SkillFileTree` remains responsible for lazy directory loading and emits only file-selection events.

## Verification

Tests cover:

- new column width normalization and tag rendering behavior;
- header select-all, indeterminate state, row propagation, and selection cleanup;
- selected-only Enable and Disable result counts;
- single-tag existing selection, new tag creation, deduplication, save, and close behavior;
- bulk tag union across skills without removing existing tags;
- safe path rejection for preview reads;
- text, UTF-8, binary, oversized, missing, local, and Git-backed preview cases;
- preview reset on skill change and replacement of stale content for non-previewable files.

Final verification includes Go tests, Go vet, TypeScript checking, frontend production build, Wails build, and desktop visual smoke checks for table resizing, tag popovers, bulk selection, lazy file navigation, and preview states.
