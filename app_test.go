package main

import (
	"context"
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
