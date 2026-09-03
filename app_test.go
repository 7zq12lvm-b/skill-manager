package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	skillmgr "skill-manager/internal"
)

func TestUseExistingRepositoryRequiresMatchingRemoteAndRestoresSkills(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	root := t.TempDir()
	repositoryPath := filepath.Join(root, "checkout")
	runAppGit(t, root, "init", repositoryPath)
	runAppGit(t, repositoryPath, "config", "user.email", "test@example.com")
	runAppGit(t, repositoryPath, "config", "user.name", "Test")
	runAppGit(t, repositoryPath, "remote", "add", "origin", "https://github.com/example/shared-skills.git")
	if err := os.MkdirAll(filepath.Join(repositoryPath, "skills", "review"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repositoryPath, "skills", "review", "SKILL.md"), []byte("# Review\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runAppGit(t, repositoryPath, "add", ".")
	runAppGit(t, repositoryPath, "commit", "-m", "add skill")

	config := skillmgr.DefaultConfig()
	config.Scan.WatchSourceFolders = false
	config.TargetDirs = []string{filepath.Join(root, "targets")}
	config.Sync.Folder = filepath.Join(root, "sync")
	syncStore := skillmgr.NewSyncStore(skillmgr.SyncPathFromFolder(config.Sync.Folder))
	if err := syncStore.Save(skillmgr.SyncDocument{Version: 2, Skills: map[string]skillmgr.SyncSkillRecord{
		"git:github.com/example/shared-skills//skills/review": {
			TargetName: "review",
			Source: skillmgr.SyncSource{
				Provider: skillmgr.GitProvider,
				ID:       "github.com/example/shared-skills",
				Locator: skillmgr.SourceLocator{
					CloneURL: "https://github.com/example/shared-skills.git",
					Subpath:  "skills/review",
				},
			},
		},
	}}); err != nil {
		t.Fatal(err)
	}
	app := &App{
		ctx:     context.Background(),
		store:   skillmgr.NewConfigStore(filepath.Join(root, "config.json")),
		service: skillmgr.NewService(),
		config:  config,
	}
	if _, err := app.UseExistingRepository("github.com/another/repo", repositoryPath); err == nil {
		t.Fatal("expected mismatched remote to be rejected")
	}
	inventory, err := app.UseExistingRepository("github.com/example/shared-skills", repositoryPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(app.config.Repositories) != 1 {
		t.Fatalf("expected one matching installation, got %#v", app.config.Repositories)
	}
	storedInfo, storedErr := os.Stat(app.config.Repositories[0].Path)
	selectedInfo, selectedErr := os.Stat(repositoryPath)
	if storedErr != nil || selectedErr != nil || !os.SameFile(storedInfo, selectedInfo) {
		t.Fatalf("expected matching installation to be stored, got %#v", app.config.Repositories)
	}
	if len(inventory.Repositories) != 1 || !inventory.Repositories[0].Installed {
		t.Fatalf("expected shared repository to become installed, got %#v", inventory.Repositories)
	}
	if len(inventory.Skills) != 1 || inventory.Skills[0].Status == skillmgr.StatusMissingSource {
		t.Fatalf("expected missing skill to recover, got %#v", inventory.Skills)
	}
}

func TestBulkTagAdditionAndDisableSelectedSkills(t *testing.T) {
	root := t.TempDir()
	syncFolder := filepath.Join(root, "sync")
	target := filepath.Join(root, "target")
	source := filepath.Join(root, "source", "review")
	if err := os.MkdirAll(source, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(source, filepath.Join(target, "review")); err != nil {
		t.Fatal(err)
	}
	enabled := true
	skill := skillmgr.Skill{
		ID:                  "review-id",
		Name:                "review",
		TargetName:          "review",
		SourcePath:          source,
		RepoID:              "example.com/me/repo",
		RepoSubpath:         "skills/review",
		IsSynced:            true,
		IsActive:            true,
		DesiredEnabled:      &enabled,
		LegacySharedEnabled: true,
		Tags:                []string{"existing"},
	}
	app := &App{
		ctx:     context.Background(),
		store:   skillmgr.NewConfigStore(filepath.Join(root, "config.json")),
		service: skillmgr.NewService(),
		config: skillmgr.Config{
			TargetDirs: []string{target},
			Sync:       skillmgr.SyncConfig{Folder: syncFolder},
		},
		inventory: skillmgr.Inventory{Skills: []skillmgr.Skill{skill}},
	}
	if err := skillmgr.NewSyncStore(skillmgr.SyncPathFromFolder(syncFolder)).UpsertSkill(skillmgr.SyncSkillRecord{
		Enabled: true,
		Tags:    []string{"existing"},
		Source:  skillmgr.SyncSource{Provider: skillmgr.GitProvider, ID: skill.RepoID, Locator: skillmgr.SourceLocator{Subpath: skill.RepoSubpath}},
	}); err != nil {
		t.Fatal(err)
	}

	tagResult, err := app.AddSkillTags([]string{skill.ID, skill.ID}, []string{"new", "existing", " new "})
	if err != nil {
		t.Fatal(err)
	}
	if tagResult.Updated != 1 || tagResult.Unchanged != 0 {
		t.Fatalf("unexpected bulk tag result: %#v", tagResult)
	}
	document, err := skillmgr.NewSyncStore(skillmgr.SyncPathFromFolder(syncFolder)).Load()
	if err != nil {
		t.Fatal(err)
	}
	record := document.Skills["git:example.com/me/repo//skills/review"]
	if len(record.Tags) != 2 || record.Tags[0] != "existing" || record.Tags[1] != "new" {
		t.Fatalf("expected merged tags, got %#v", record.Tags)
	}

	app.inventory.Skills = []skillmgr.Skill{skill}
	disableResult, err := app.DisableSkills([]string{skill.ID, skill.ID})
	if err != nil {
		t.Fatal(err)
	}
	if disableResult.Disabled != 1 || disableResult.AlreadyDisabled != 0 {
		t.Fatalf("unexpected bulk disable result: %#v", disableResult)
	}
	if _, err := os.Lstat(filepath.Join(target, "review")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected selected skill link to be removed, got %v", err)
	}
}

func TestSaveSkillNotePreservesSharedRecord(t *testing.T) {
	root := t.TempDir()
	syncFolder := filepath.Join(root, "sync")
	enabled := true
	skill := skillmgr.Skill{
		ID:                  "git:example.com/me/repo//skills/review",
		SyncID:              "git:example.com/me/repo//skills/review",
		Name:                "review",
		TargetName:          "review",
		RepoID:              "example.com/me/repo",
		RepoSubpath:         "skills/review",
		IsSynced:            true,
		DesiredEnabled:      &enabled,
		LegacySharedEnabled: true,
		Tags:                []string{"existing"},
	}
	app := &App{
		ctx:     context.Background(),
		store:   skillmgr.NewConfigStore(filepath.Join(root, "config.json")),
		service: skillmgr.NewService(),
		config: skillmgr.Config{
			TargetDirs: []string{filepath.Join(root, "target")},
			Sync:       skillmgr.SyncConfig{Folder: syncFolder},
		},
		inventory: skillmgr.Inventory{Skills: []skillmgr.Skill{skill}},
	}
	store := skillmgr.NewSyncStore(skillmgr.SyncPathFromFolder(syncFolder))
	if err := store.UpsertSkill(skillmgr.SyncSkillRecord{
		Enabled:    true,
		TargetName: "review",
		Tags:       []string{"existing"},
		Source:     skillmgr.SyncSource{Provider: skillmgr.GitProvider, ID: skill.RepoID, Locator: skillmgr.SourceLocator{Subpath: skill.RepoSubpath}},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := app.SaveSkillNote(skill.ID, "  first line\nsecond line  "); err != nil {
		t.Fatal(err)
	}
	document, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	record := document.Skills[skill.SyncID]
	if record.Note != "first line\nsecond line" || !record.Enabled || len(record.Tags) != 1 || record.Tags[0] != "existing" {
		t.Fatalf("unexpected saved note record: %#v", record)
	}
}

func TestSaveSkillStarredIsIndependentFromEnabledState(t *testing.T) {
	root := t.TempDir()
	syncFolder := filepath.Join(root, "sync")
	disabled := false
	skill := skillmgr.Skill{
		ID:             "git:example.com/me/repo//skills/review",
		SyncID:         "git:example.com/me/repo//skills/review",
		Name:           "review",
		TargetName:     "review",
		RepoID:         "example.com/me/repo",
		RepoSubpath:    "skills/review",
		IsSynced:       true,
		DesiredEnabled: &disabled,
	}
	app := &App{
		ctx:       context.Background(),
		store:     skillmgr.NewConfigStore(filepath.Join(root, "config.json")),
		service:   skillmgr.NewService(),
		config:    skillmgr.Config{Sync: skillmgr.SyncConfig{Folder: syncFolder}},
		inventory: skillmgr.Inventory{Skills: []skillmgr.Skill{skill}},
	}
	store := skillmgr.NewSyncStore(skillmgr.SyncPathFromFolder(syncFolder))
	if err := store.UpsertSkill(skillmgr.SyncSkillRecord{
		Source: skillmgr.SyncSource{Provider: skillmgr.GitProvider, ID: skill.RepoID, Locator: skillmgr.SourceLocator{Subpath: skill.RepoSubpath}},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := app.SaveSkillStarred(skill.ID, true); err != nil {
		t.Fatal(err)
	}
	document, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	record := document.Skills[skill.SyncID]
	if !record.Starred || record.Enabled {
		t.Fatalf("expected a starred but disabled skill, got %#v", record)
	}
}

func TestRemoveMissingSkillRequiresInstalledRepositoryAndCleansManagedLink(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	root := t.TempDir()
	repositoryPath := filepath.Join(root, "repo")
	runAppGit(t, root, "init", repositoryPath)
	runAppGit(t, repositoryPath, "config", "user.email", "test@example.com")
	runAppGit(t, repositoryPath, "config", "user.name", "Test")
	runAppGit(t, repositoryPath, "remote", "add", "origin", "https://github.com/example/shared-skills.git")
	if err := os.WriteFile(filepath.Join(repositoryPath, "README.md"), []byte("skills\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runAppGit(t, repositoryPath, "add", ".")
	runAppGit(t, repositoryPath, "commit", "-m", "initial")

	syncFolder := filepath.Join(root, "sync")
	targetDir := filepath.Join(root, "target")
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		t.Fatal(err)
	}
	missingPath := filepath.Join(repositoryPath, "skills", "removed")
	managedLink := filepath.Join(targetDir, "removed")
	if err := os.Symlink(missingPath, managedLink); err != nil {
		t.Fatal(err)
	}
	unrelatedPath := filepath.Join(root, "unrelated")
	if err := os.MkdirAll(unrelatedPath, 0o755); err != nil {
		t.Fatal(err)
	}
	unrelatedLink := filepath.Join(targetDir, "old-removed")
	if err := os.Symlink(unrelatedPath, unrelatedLink); err != nil {
		t.Fatal(err)
	}
	syncID := "git:github.com/example/shared-skills//skills/removed"
	config := skillmgr.DefaultConfig()
	config.Scan.WatchSourceFolders = false
	config.TargetDirs = []string{targetDir}
	config.Sync.Folder = syncFolder
	config.Repositories = []skillmgr.RepositoryConfig{{
		ID:      "github.com/example/shared-skills",
		RepoID:  "github.com/example/shared-skills",
		Path:    repositoryPath,
		Enabled: true,
	}}
	store := skillmgr.NewSyncStore(skillmgr.SyncPathFromFolder(syncFolder))
	if err := store.UpsertSkill(skillmgr.SyncSkillRecord{
		Enabled:             true,
		TargetName:          "removed",
		PreviousTargetNames: []string{"old-removed"},
		Source: skillmgr.SyncSource{Provider: skillmgr.GitProvider, ID: "github.com/example/shared-skills", Locator: skillmgr.SourceLocator{
			CloneURL: "https://github.com/example/shared-skills.git",
			Subpath:  "skills/removed",
		}},
	}); err != nil {
		t.Fatal(err)
	}
	app := &App{
		ctx:     context.Background(),
		store:   skillmgr.NewConfigStore(filepath.Join(root, "config.json")),
		service: skillmgr.NewService(),
		config:  config,
	}
	if err := app.refreshLocked(app.ctx); err != nil {
		t.Fatal(err)
	}
	if len(app.inventory.Skills) != 1 || app.inventory.Skills[0].Status != skillmgr.StatusMissingSource {
		t.Fatalf("expected missing source before removal, got %#v", app.inventory.Skills)
	}
	if _, err := app.RemoveMissingSkill(syncID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(managedLink); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected managed dangling link to be removed, got %v", err)
	}
	if target, err := os.Readlink(unrelatedLink); err != nil || target != unrelatedPath {
		t.Fatalf("expected unrelated link to remain untouched, target=%q err=%v", target, err)
	}
	document, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := document.Skills[syncID]; exists {
		t.Fatalf("expected missing skill record to be removed, got %#v", document.Skills[syncID])
	}

	app.inventory = skillmgr.Inventory{Skills: []skillmgr.Skill{{
		ID:       syncID,
		SyncID:   syncID,
		RepoID:   "github.com/example/missing-repo",
		IsSynced: true,
		Status:   skillmgr.StatusMissingSource,
	}}}
	if _, err := app.RemoveMissingSkill(syncID); err == nil || !strings.Contains(err.Error(), "repository checkout is unavailable") {
		t.Fatalf("expected unavailable repository removal to be rejected, got %v", err)
	}
}

func TestRemoveSkillExcludesRepositoryPathWithoutDeletingSource(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	root := t.TempDir()
	repositoryPath := filepath.Join(root, "repo")
	runAppGit(t, root, "init", repositoryPath)
	runAppGit(t, repositoryPath, "config", "user.email", "test@example.com")
	runAppGit(t, repositoryPath, "config", "user.name", "Test")
	runAppGit(t, repositoryPath, "remote", "add", "origin", "https://github.com/example/shared-skills.git")
	for _, subpath := range []string{"skills/main", "examples/demo/skills/duplicate"} {
		path := filepath.Join(repositoryPath, filepath.FromSlash(subpath))
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(path, "SKILL.md"), []byte("---\nname: test\ndescription: test skill\n---\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	runAppGit(t, repositoryPath, "add", ".")
	runAppGit(t, repositoryPath, "commit", "-m", "add skills")

	targetDir := filepath.Join(root, "target")
	syncFolder := filepath.Join(root, "sync")
	config := skillmgr.DefaultConfig()
	config.Scan.WatchSourceFolders = false
	config.TargetDirs = []string{targetDir}
	config.Sync.Folder = syncFolder
	config.Repositories = []skillmgr.RepositoryConfig{{
		ID:        "github.com/example/shared-skills",
		RepoID:    "github.com/example/shared-skills",
		Path:      repositoryPath,
		Enabled:   true,
		CloneURL:  "https://github.com/example/shared-skills.git",
		ScanRoots: []string{"."},
	}}
	app := &App{
		ctx:     context.Background(),
		store:   skillmgr.NewConfigStore(filepath.Join(root, "config.json")),
		service: skillmgr.NewService(),
		config:  config,
	}
	if err := app.refreshLocked(app.ctx); err != nil {
		t.Fatal(err)
	}
	var duplicate skillmgr.Skill
	for _, skill := range app.inventory.Skills {
		if skill.RepoSubpath == "examples/demo/skills/duplicate" {
			duplicate = skill
			break
		}
	}
	if duplicate.ID == "" || !duplicate.CanRemove {
		t.Fatalf("expected removable example skill, got %#v", duplicate)
	}
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		t.Fatal(err)
	}
	managedLink := filepath.Join(targetDir, duplicate.TargetName)
	if err := os.Symlink(duplicate.SourcePath, managedLink); err != nil {
		t.Fatal(err)
	}

	inventory, err := app.RemoveSkill(duplicate.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(inventory.Skills) != 1 || inventory.Skills[0].RepoSubpath != "skills/main" {
		t.Fatalf("expected only the main skill after removal, got %#v", inventory.Skills)
	}
	if _, err := os.Stat(filepath.Join(duplicate.SourcePath, "SKILL.md")); err != nil {
		t.Fatalf("expected source files to remain: %v", err)
	}
	if _, err := os.Lstat(managedLink); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected managed link to be removed, got %v", err)
	}
	loaded, err := app.store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Repositories) != 1 || len(loaded.Repositories[0].IgnorePaths) != 1 || loaded.Repositories[0].IgnorePaths[0] != duplicate.RepoSubpath {
		t.Fatalf("expected removed path to persist in ignorePaths, got %#v", loaded.Repositories)
	}
	document, err := skillmgr.NewSyncStore(skillmgr.SyncPathFromFolder(syncFolder)).Load()
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := document.Skills[duplicate.SyncID]; exists {
		t.Fatalf("expected shared skill record to be deleted, got %#v", document.Skills[duplicate.SyncID])
	}
	rescanned, err := app.RescanAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(rescanned.Skills) != 1 || rescanned.Skills[0].RepoSubpath != "skills/main" {
		t.Fatalf("expected ignored skill to stay removed after rescan, got %#v", rescanned.Skills)
	}
}

func TestValidateTerminalDirectory(t *testing.T) {
	root := t.TempDir()
	filePath := filepath.Join(root, "file.txt")
	if err := os.WriteFile(filePath, []byte("not a directory\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := validateTerminalDirectory(root); err != nil {
		t.Fatalf("expected temporary directory to be accepted: %v", err)
	}
	for _, path := range []string{"", filepath.Join(root, "missing"), filePath} {
		if err := validateTerminalDirectory(path); err == nil {
			t.Fatalf("expected %q to be rejected", path)
		}
	}
}

func runAppGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	command := exec.Command("git", "-C", dir)
	command.Args = append(command.Args, args...)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, output)
	}
	return strings.TrimSpace(string(output))
}

func TestSkillSwitchesAreDeviceLocal(t *testing.T) {
	root, pathErr := filepath.EvalSymlinks(t.TempDir())
	if pathErr != nil {
		t.Fatal(pathErr)
	}
	repo := filepath.Join(root, "repo")
	runAppGit(t, root, "init", repo)
	runAppGit(t, repo, "remote", "add", "origin", "https://github.com/example/local-switches.git")
	source := filepath.Join(repo, "review")
	if err := os.MkdirAll(source, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "SKILL.md"), []byte("---\nname: review\ndescription: Review code\n---\n"), 0644); err != nil {
		t.Fatal(err)
	}
	newDevice := func(name string, active bool) *App {
		config := skillmgr.DefaultConfig()
		config.Scan.WatchSourceFolders = false
		config.Sync.Folder = filepath.Join(root, "sync")
		config.TargetDirs = []string{filepath.Join(root, name, "target")}
		config.Repositories = []skillmgr.RepositoryConfig{{RepoID: "github.com/example/local-switches", Path: repo, Enabled: true, ScanRoots: []string{"."}}}
		if err := os.MkdirAll(config.TargetDirs[0], 0755); err != nil {
			t.Fatal(err)
		}
		if active {
			if err := os.Symlink(source, filepath.Join(config.TargetDirs[0], "review")); err != nil {
				t.Fatal(err)
			}
		}
		app := &App{ctx: context.Background(), config: config, store: skillmgr.NewConfigStore(filepath.Join(root, name, "config.json")), service: skillmgr.NewService()}
		if err := app.refreshLocked(app.ctx); err != nil {
			t.Fatal(err)
		}
		return app
	}
	a, b := newDevice("a", true), newDevice("b", false)
	id := a.inventory.Skills[0].ID
	check := func(app *App, want bool) {
		t.Helper()
		if err := app.refreshLocked(app.ctx); err != nil {
			t.Fatal(err)
		}
		if len(app.inventory.Skills) != 1 || app.inventory.Skills[0].IsActive != want {
			t.Fatalf("expected active=%v, got %#v", want, app.inventory.Skills)
		}
	}
	check(a, true)
	check(b, false)
	store := skillmgr.NewSyncStore(skillmgr.SyncPathFromFolder(a.config.Sync.Folder))
	before, err := os.ReadFile(store.Path())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.EnableSkills([]string{id}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.DisableSkill(id); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(store.Path())
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("switches changed shared database")
	}
	check(a, false)
	check(b, true)
	// A remote legacy switch and metadata update must not change either device.
	doc, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	record := doc.Skills[id]
	record.Enabled = true
	record.Note = "shared note"
	if err := store.UpsertSkill(record); err != nil {
		t.Fatal(err)
	}
	check(a, false)
	check(b, true)
	if a.inventory.Skills[0].Note != "shared note" {
		t.Fatal("metadata did not sync")
	}
	// Restart reloads the local preference, without importing the shared switch.
	a.config, err = a.store.Load()
	if err != nil {
		t.Fatal(err)
	}
	check(a, false)
	if _, err := b.DisableSkills([]string{id}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.EnableSkill(id); err != nil {
		t.Fatal(err)
	}
	check(a, true)
	check(b, false)
}
