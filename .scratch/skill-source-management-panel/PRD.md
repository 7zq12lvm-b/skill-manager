Status: ready-for-agent
Title: Skill Source Management and Skill Enablement Panel

## Problem Statement

Users who manage local AI Agent skills across several directories cannot easily see which directories are being scanned, which skills were discovered, which skills are currently enabled, and whether `~/.agents/skills` accurately reflects the source directories they intended to use.

Today the mental model is file-system first: users must remember source paths, inspect folders manually, and reason about symlinks themselves. That makes common tasks risky: enabling the wrong version of a skill, overwriting a symlink target, missing a broken skill, or assuming a rescan changed the active skill set when it only refreshed discovery.

The user needs a simple local Skill Manager that makes source directories, discovered skills, validation problems, conflicts, and symlink sync state visible at a glance.

## Solution

Build a "Skill Source Management + Skill Enablement Panel" for local AI Agent skills.

The application presents a three-pane interface:

- A Skill Sources pane for adding, removing, rescanning, opening, and renaming scanned source directories.
- A Skills pane for searching, filtering, and toggling discovered skills.
- A Skill Detail pane for inspecting the selected skill's source path, target symlink path, sync status, validation state, files, metadata, preview content, and available actions.

The core model is explicit symlink synchronization into a target skill directory, defaulting to `~/.agents/skills`.

Rescans discover source folders and refresh derived status only. Rescans must not create, delete, or retarget symlinks. Only explicit user actions, such as enabling or disabling a skill, may modify the target skill directory.

## User Stories

1. As a skill user, I want to see all configured skill source directories, so that I know which places are being scanned.
2. As a skill user, I want to add a source directory, so that skills in a new local folder can be discovered.
3. As a skill user, I want to browse for a source directory, so that I do not have to type long local paths manually.
4. As a skill user, I want the add-source dialog to explain the scan rule, so that I understand that first-level subfolders are treated as skill candidates.
5. As a skill user, I want each source to show its skill count, so that I can quickly understand what was found.
6. As a skill user, I want each source to show the last scanned time, so that I know whether the list is fresh.
7. As a skill user, I want each source to show scan errors, so that I can fix inaccessible or malformed directories.
8. As a skill user, I want to rescan one source, so that I can refresh a changed directory without rescanning everything.
9. As a skill user, I want to rescan all sources, so that I can refresh the entire skill inventory.
10. As a skill user, I want to open a source folder from the UI, so that I can inspect or edit files quickly.
11. As a skill user, I want to rename a source alias, so that the UI can show a friendly name instead of a long path.
12. As a skill user, I want to remove a source directory from scanning, so that old or irrelevant skill folders stop appearing.
13. As a skill user, I want removing a source to avoid deleting source files, so that my original skill folders remain safe.
14. As a skill user, I want a global summary of skills found, enabled skills, conflicts, and invalid skills, so that I can assess the current state at a glance.
15. As a skill user, I want to see the target skill directory, so that I know where enabled skills are being synced.
16. As a skill user, I want to open the target skill directory, so that I can verify the real filesystem state if needed.
17. As a skill user, I want to search skills by name, so that I can find a specific skill quickly.
18. As a skill user, I want to filter skills by source, so that I can focus on one skill collection at a time.
19. As a skill user, I want to filter skills by status, so that I can find disabled, synced, conflicting, invalid, or error states.
20. As a skill user, I want the skill list to use a dense table by default, so that many skills remain scannable.
21. As a skill user, I want each skill row to show its enablement control, name, source, status, and updated time, so that I can make decisions without opening every detail page.
22. As a skill user, I want status text beside any visual indicator, so that synced, disabled, conflict, invalid, and error states are unambiguous.
23. As a skill user, I want to enable a disabled skill, so that a symlink is created in `~/.agents/skills`.
24. As a skill user, I want enabling a skill to transition through a temporary syncing state, so that I know an action is in progress.
25. As a skill user, I want a successfully enabled skill to show as synced, so that I know the symlink points to the selected source.
26. As a skill user, I want to disable a synced skill, so that its symlink is removed from `~/.agents/skills`.
27. As a skill user, I want disabling a skill to warn me that only the symlink will be removed, so that I do not fear losing the original source folder.
28. As a skill user, I want disabling to delete only a symlink that points to the selected skill, so that unrelated target entries are not damaged.
29. As a skill user, I want a skill to show disabled when no target symlink exists, so that inactive skills are clear.
30. As a skill user, I want a skill to show synced when the target symlink points to that source path, so that active skills are clear.
31. As a skill user, I want a skill to show conflict when another source provides the same skill name, so that I do not accidentally enable the wrong version.
32. As a skill user, I want a skill to show conflict when the target symlink points somewhere else, so that I can reconcile the active target.
33. As a skill user, I want conflict detail to list every source that provides the same skill name, so that I can choose deliberately.
34. As a skill user, I want conflict detail to show the current symlink target, so that I understand the currently active source.
35. As a skill user, I want to choose the active source for a conflicting skill, so that I can resolve the conflict explicitly.
36. As a skill user, I want applying a conflict resolution to update the symlink only after confirmation, so that source priority changes are intentional.
37. As a skill user, I want a skill to show invalid when it fails validation, so that broken or incomplete skill folders do not look usable.
38. As a skill user, I want invalid skill detail to explain the missing required files, so that I know how to fix the skill.
39. As a skill user, I want to open an invalid skill folder, so that I can add or repair the missing files.
40. As a skill user, I want to ignore invalid folders, so that noisy directories do not distract me.
41. As a skill user, I want a setting for strict validation, so that I can require `SKILL.md` or another configured metadata file.
42. As a skill user, I want a setting for loose validation, so that every first-level folder can be treated as a skill candidate.
43. As a skill user, I want a custom required-files setting, so that teams with `skill.json` or mixed conventions can validate correctly.
44. As a skill user, I want the detail pane to show the skill name and status, so that selected skill context is obvious.
45. As a skill user, I want the detail pane to show source path and symlink path, so that I can reason about the exact file relationship.
46. As a skill user, I want the detail pane to show whether the symlink exists and where it points, so that I can debug mismatches.
47. As a skill user, I want the detail pane to list important files, so that I can see whether the skill includes expected documentation and examples.
48. As a skill user, I want the detail pane to preview `SKILL.md`, so that I can inspect a skill without opening an editor.
49. As a skill user, I want the detail pane to preview `README.md` when available, so that I can understand a skill's purpose.
50. As a skill user, I want the detail pane to show metadata such as description and last scanned time, so that I can make informed enablement decisions.
51. As a skill user, I want skill actions in the detail pane, so that I can open folders, rescan a skill, enable it, disable it, or resolve conflicts from one place.
52. As a skill user, I want settings for the target skill directory, so that I can use a target other than `~/.agents/skills`.
53. As a skill user, I want an auto-rescan-on-startup setting, so that the UI can start with fresh discovery when desired.
54. As a skill user, I want a watch-source-folders setting, so that changes can appear without manual rescanning when desired.
55. As a skill user, I want conflict handling settings, so that I can choose between asking every time, source priority, latest modified, or manual priority rules.
56. As a skill user, I want source priority ordering, so that predictable conflict resolution can be configured when I choose a priority-based mode.
57. As a skill user, I want target paths that are occupied by non-symlink entries to show an error state, so that the app does not overwrite real files or directories.
58. As a skill user, I want rescan behavior to be read-only, so that refreshing discovery never changes which skills are enabled.
59. As a skill user, I want enable and disable operations to be explicit, so that all changes to `~/.agents/skills` are intentional.
60. As a skill user, I want filesystem errors to be visible in the affected source or skill, so that permission and path problems can be diagnosed.

## Implementation Decisions

- The product will use the domain terms Skill Source, Skill, Target Skill Directory, Symlink, Sync Status, Conflict, Invalid Skill, and Rescan consistently.
- The primary UI will be a three-pane application: Skill Sources, Skills, and Skill Detail.
- The default target skill directory will be `~/.agents/skills`, with a setting to change it.
- A Skill Source is a configured directory that is scanned for first-level subfolders.
- A Skill candidate's name is its first-level folder name.
- A Skill candidate's source path is the first-level folder path under its Skill Source.
- A Skill candidate's target path is the target skill directory plus the skill name.
- The skill list will default to a dense table rather than card-first layout because the main workflow is scanning, comparison, filtering, and status review.
- The Skills pane will support search, source filtering, status filtering, and manual rescan actions.
- Status will be derived from validation results, duplicate skill names, target path existence, symlink state, and symlink target.
- The main statuses will include synced, disabled, conflict, invalid, missing, syncing, and error.
- Disabled means no target symlink exists for that skill name.
- Synced means the target path is a symlink that points to the selected source path.
- Conflict means multiple source paths provide the same skill name, or the target symlink points to a different source path.
- Invalid means the source folder does not satisfy the configured validation rule.
- Error means the target path or source path cannot be safely interpreted, such as when a target path exists but is not a symlink.
- Scanning and rescanning must be read-only and must not create, remove, or retarget symlinks.
- Enabling a skill is an explicit action that creates a symlink in the target skill directory when it is safe to do so.
- Disabling a skill is an explicit action that removes only the symlink for the selected skill when that symlink points to the selected source.
- The application must never delete original source folders as part of disable, remove source, rescan, or conflict resolution workflows.
- The application must not overwrite non-symlink files or directories in the target skill directory.
- Enablement actions should show a temporary syncing state while filesystem mutation is in progress.
- Disable actions should show a confirmation explaining that only the target symlink will be removed.
- Conflict resolution will happen from the Skill Detail pane and will show all source paths providing the same skill name plus the current target symlink, when present.
- Conflict resolution will require the user to choose the active source before applying a symlink change, unless the user has configured a non-interactive conflict handling policy.
- Initial validation modes will include loose validation, strict validation requiring `SKILL.md`, and custom required files.
- The source list will show source alias or path, discovered skill count, last scanned time, and error count.
- Source actions will include open folder, rescan, rename alias, and remove source.
- Removing a source only removes it from scanning; it does not delete source files and does not automatically delete existing symlinks.
- The detail pane will show source path, symlink path, symlink target, status, validation errors, file list, metadata, preview, last scanned time, and actions.
- The detail pane will preview `SKILL.md` and `README.md` when present.
- Settings will include target skill directory, scan behavior, validation mode, conflict handling, source folder watching, auto rescan on startup, and source priority.
- The highest implementation seam should be a Skill Inventory service that can scan configured sources, derive statuses against a target skill directory, and perform explicit enable, disable, and conflict resolution operations.
- The UI should depend on this Skill Inventory service rather than duplicating filesystem status logic in view components.
- Configuration persistence should store sources, aliases, enabled source scanning flags, validation settings, target directory, conflict handling settings, and source priority.

## Testing Decisions

- Tests should assert external behavior: discovered sources, derived statuses, displayed state, and filesystem side effects after explicit user actions.
- Tests should avoid implementation details such as internal helper function ordering, component state variable names, or exact private data structures.
- The highest-value seam is the Skill Inventory service because it owns the central product invariant: scans are read-only, while enable and disable actions are explicit symlink mutations.
- The Skill Inventory service should be tested with temporary directories that include source folders, target directories, symlinks, non-symlink occupied paths, duplicate skill names, invalid skill folders, and inaccessible or missing paths where practical.
- Scan tests should verify that first-level subfolders become skill candidates and deeper nested folders do not become independent skills.
- Status tests should verify disabled, synced, conflict, invalid, and error outcomes from real filesystem arrangements.
- Read-only rescan tests should verify that scanning never creates, removes, or retargets symlinks.
- Enable tests should verify that enabling a safe disabled skill creates the expected symlink.
- Enable tests should verify that occupied target paths and conflicting symlink targets are not overwritten.
- Disable tests should verify that disabling removes only a symlink pointing to the selected source.
- Disable tests should verify that disabling does not remove non-symlink files, directories, or symlinks pointing elsewhere.
- Conflict tests should verify duplicate skill names across sources and existing target symlinks to one candidate.
- Validation tests should cover loose mode, strict `SKILL.md` mode, and custom required files.
- UI tests should exercise the user-observable workflow: add source, scan, filter skills, inspect detail, enable skill, disable skill, and resolve a conflict.
- UI tests should verify that status labels and action availability match the derived Skill Inventory state.
- Settings tests should verify that changing target directory, validation mode, and conflict policy changes subsequent status derivation without unexpectedly mutating symlinks.
- If the application uses a browser UI, end-to-end tests should verify the three-pane layout at desktop size and ensure key controls remain usable at narrower widths.
- Since the current repository has no existing tests or implementation, the first implementation should establish this seam and add tests around it before adding broad UI coverage.

## Out of Scope

- Installing skills from remote marketplaces.
- Downloading skills from GitHub or other registries.
- Editing skill source files inside the app beyond opening folders.
- Creating new skills from templates.
- Deleting original source folders.
- Automatically changing symlink state during scans.
- Full package management, dependency installation, or version resolution for skills.
- Multi-user permission management.
- Cloud synchronization of source configuration.
- Ranking, recommending, or reviewing skill quality.
- Supporting nested scan rules beyond first-level subfolders in the MVP.
- Supporting non-local target directories in the MVP.

## Further Notes

The core product positioning is: manage local AI Agent skills like a lightweight plugin manager.

The MVP should include multiple source directories, first-level subfolder scanning, a skill list, one enablement control per skill, symlink creation on enable, symlink removal on disable, and clear synced, disabled, conflict, invalid, and error statuses.

The most important invariant is safety: discovery is read-only, and filesystem mutation happens only after explicit user intent.
