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
		ID:             "review-id",
		Name:           "review",
		TargetName:     "review",
		SourcePath:     source,
		RepoID:         "example.com/me/repo",
		RepoSubpath:    "skills/review",
		IsSynced:       true,
		IsActive:       true,
		DesiredEnabled: &enabled,
		Tags:           []string{"existing"},
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
