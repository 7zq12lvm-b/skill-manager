package skillmgr

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestReadSkillFilePreviewClassifiesLocalFiles(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "skill")
	mustWrite(t, filepath.Join(source, "notes.txt"), "你好, skill manager\n")
	if err := os.WriteFile(filepath.Join(source, "image.bin"), []byte{0x89, 0x50, 0x00, 0xff}, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "invalid-utf8.bin"), []byte{0xff, 0xfe}, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "large.txt"), bytes.Repeat([]byte("a"), int(maxSkillFilePreviewBytes)+1), 0o644); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(root, "outside.txt")
	mustWrite(t, outside, "outside\n")
	if err := os.Symlink(outside, filepath.Join(source, "outside-link.txt")); err != nil {
		t.Fatal(err)
	}

	service := NewService()
	skill := Skill{ID: "skill", SourcePath: source}
	preview, err := service.ReadSkillFilePreview(context.Background(), skill, "notes.txt")
	if err != nil {
		t.Fatal(err)
	}
	if !preview.Previewable || preview.Content != "你好, skill manager\n" {
		t.Fatalf("unexpected text preview: %#v", preview)
	}
	binary, err := service.ReadSkillFilePreview(context.Background(), skill, "image.bin")
	if err != nil {
		t.Fatal(err)
	}
	if binary.Previewable || binary.Reason == "" || binary.Content != "" {
		t.Fatalf("expected binary file to be rejected, got %#v", binary)
	}
	invalidUTF8, err := service.ReadSkillFilePreview(context.Background(), skill, "invalid-utf8.bin")
	if err != nil {
		t.Fatal(err)
	}
	if invalidUTF8.Previewable || invalidUTF8.Reason == "" || invalidUTF8.Content != "" {
		t.Fatalf("expected invalid UTF-8 file to be rejected, got %#v", invalidUTF8)
	}
	large, err := service.ReadSkillFilePreview(context.Background(), skill, "large.txt")
	if err != nil {
		t.Fatal(err)
	}
	if large.Previewable || large.Reason == "" {
		t.Fatalf("expected large file to be rejected, got %#v", large)
	}
	if _, err := service.ReadSkillFilePreview(context.Background(), skill, "../outside.txt"); err == nil {
		t.Fatal("expected parent traversal to be rejected")
	}
	if _, err := service.ReadSkillFilePreview(context.Background(), skill, "outside-link.txt"); err == nil {
		t.Fatal("expected symlink escape to be rejected")
	}
}

func TestReadSkillFilePreviewUsesGitHead(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	runGit(t, root, "init", repo)
	runGit(t, repo, "config", "user.email", "test@example.com")
	runGit(t, repo, "config", "user.name", "Test")
	mustWrite(t, filepath.Join(repo, "skills", "demo", "notes.txt"), "committed\n")
	runGit(t, repo, "add", ".")
	runGit(t, repo, "commit", "-m", "add preview")
	mustWrite(t, filepath.Join(repo, "skills", "demo", "notes.txt"), "working tree\n")

	preview, err := NewService().ReadSkillFilePreview(context.Background(), Skill{
		ID:          "demo",
		RepoPath:    repo,
		RepoSubpath: "skills/demo",
		SourcePath:  filepath.Join(repo, "skills", "demo"),
	}, "notes.txt")
	if err != nil {
		t.Fatal(err)
	}
	if !preview.Previewable || preview.Content != "committed\n" {
		t.Fatalf("expected committed Git content, got %#v", preview)
	}
}

func TestScanDiscoversFirstLevelSkillsAndDerivesStatuses(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	target := filepath.Join(root, "target")
	mustMkdir(t, filepath.Join(source, "summarize-pdf"))
	mustMkdir(t, filepath.Join(source, "code-review", "nested"))
	mustWrite(t, filepath.Join(source, "summarize-pdf", "SKILL.md"), "# summarize-pdf\n")
	mustWrite(t, filepath.Join(source, "code-review", "SKILL.md"), "# code-review\n")
	mustMkdir(t, target)
	mustSymlink(t, filepath.Join(source, "summarize-pdf"), filepath.Join(target, "summarize-pdf"))

	config := Config{
		TargetDirs: []string{target},
		Sources: []SkillSourceConfig{{
			ID:      "local",
			Path:    source,
			Alias:   "Local",
			Enabled: true,
		}},
		Validation: ValidationConfig{Mode: ValidationStrict},
	}

	inventory, err := NewService().Scan(context.Background(), config)
	if err != nil {
		t.Fatal(err)
	}

	if inventory.Summary.SkillsFound != 2 {
		t.Fatalf("expected 2 first-level skills, got %d", inventory.Summary.SkillsFound)
	}
	assertSkillStatus(t, inventory, "summarize-pdf", StatusEnabled)
	assertSkillStatus(t, inventory, "code-review", StatusDisabled)
}

func TestScanIsReadOnly(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	target := filepath.Join(root, "target")
	mustMkdir(t, filepath.Join(source, "code-review"))
	mustWrite(t, filepath.Join(source, "code-review", "SKILL.md"), "# code-review\n")
	mustMkdir(t, target)

	config := Config{
		TargetDirs: []string{target},
		Sources: []SkillSourceConfig{{
			ID:      "local",
			Path:    source,
			Enabled: true,
		}},
		Validation: ValidationConfig{Mode: ValidationStrict},
	}

	if _, err := NewService().Scan(context.Background(), config); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Lstat(filepath.Join(target, "code-review")); !os.IsNotExist(err) {
		t.Fatalf("scan should not create a symlink, lstat err = %v", err)
	}
}

func TestEnableCreatesSymlinkWhenTargetIsEmpty(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	target := filepath.Join(root, "target")
	skillPath := filepath.Join(source, "code-review")
	mustMkdir(t, skillPath)

	service := NewService()
	config := Config{TargetDirs: []string{target}, Validation: ValidationConfig{Mode: ValidationLoose}}
	err := service.Enable(context.Background(), config, Skill{Name: "code-review", SourcePath: skillPath})
	if err != nil {
		t.Fatal(err)
	}

	actual, err := os.Readlink(filepath.Join(target, "code-review"))
	if err != nil {
		t.Fatal(err)
	}
	if actual != skillPath {
		t.Fatalf("expected symlink to %s, got %s", skillPath, actual)
	}
}

func TestEnableAndDisableApplyToAllTargetDirs(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	targetA := filepath.Join(root, "target-a")
	targetB := filepath.Join(root, "target-b")
	skillPath := filepath.Join(source, "code-review")
	mustMkdir(t, skillPath)

	service := NewService()
	config := Config{
		TargetDirs: []string{targetA, targetB},
		Validation: ValidationConfig{Mode: ValidationLoose},
	}
	err := service.Enable(context.Background(), config, Skill{Name: "code-review", SourcePath: skillPath})
	if err != nil {
		t.Fatal(err)
	}

	for _, target := range []string{targetA, targetB} {
		actual, err := os.Readlink(filepath.Join(target, "code-review"))
		if err != nil {
			t.Fatal(err)
		}
		if actual != skillPath {
			t.Fatalf("expected symlink in %s to %s, got %s", target, skillPath, actual)
		}
	}

	err = service.Disable(context.Background(), config, Skill{Name: "code-review", SourcePath: skillPath})
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{targetA, targetB} {
		if _, err := os.Lstat(filepath.Join(target, "code-review")); !os.IsNotExist(err) {
			t.Fatalf("expected symlink in %s to be removed, lstat err = %v", target, err)
		}
	}
}

func TestScanMarksPartiallySyncedSkillAsSyncing(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	targetA := filepath.Join(root, "target-a")
	targetB := filepath.Join(root, "target-b")
	skillPath := filepath.Join(source, "code-review")
	mustWrite(t, filepath.Join(skillPath, "SKILL.md"), "# code-review\n")
	mustMkdir(t, targetA)
	mustSymlink(t, skillPath, filepath.Join(targetA, "code-review"))

	inventory, err := NewService().Scan(context.Background(), Config{
		TargetDirs: []string{targetA, targetB},
		Sources: []SkillSourceConfig{{
			ID:      "local",
			Path:    source,
			Enabled: true,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}

	assertSkillStatus(t, inventory, "code-review", StatusEnabled)
	if len(inventory.Skills) != 1 || len(inventory.Skills[0].TargetStates) != 2 {
		t.Fatalf("expected two target states, got %#v", inventory.Skills)
	}
}

func TestDisableRemovesMatchingTargetsEvenWhenAnotherTargetConflicts(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	other := filepath.Join(root, "other")
	targetA := filepath.Join(root, "target-a")
	targetB := filepath.Join(root, "target-b")
	skillPath := filepath.Join(source, "code-review")
	otherSkillPath := filepath.Join(other, "code-review")
	mustMkdir(t, skillPath)
	mustMkdir(t, otherSkillPath)
	mustMkdir(t, targetA)
	mustMkdir(t, targetB)
	mustSymlink(t, skillPath, filepath.Join(targetA, "code-review"))
	mustSymlink(t, otherSkillPath, filepath.Join(targetB, "code-review"))

	err := NewService().Disable(context.Background(), Config{TargetDirs: []string{targetA, targetB}}, Skill{Name: "code-review", SourcePath: skillPath})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(targetA, "code-review")); !os.IsNotExist(err) {
		t.Fatalf("expected matching symlink to be removed, lstat err = %v", err)
	}
	actual, err := os.Readlink(filepath.Join(targetB, "code-review"))
	if err != nil {
		t.Fatal(err)
	}
	if actual != otherSkillPath {
		t.Fatalf("expected conflicting symlink to remain pointed at %s, got %s", otherSkillPath, actual)
	}
}

func TestEnableRefusesOccupiedTarget(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	target := filepath.Join(root, "target")
	skillPath := filepath.Join(source, "code-review")
	mustMkdir(t, skillPath)
	mustMkdir(t, target)
	if err := os.WriteFile(filepath.Join(target, "code-review"), []byte("not a symlink"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := NewService().Enable(context.Background(), Config{TargetDirs: []string{target}}, Skill{Name: "code-review", SourcePath: skillPath})
	if err == nil {
		t.Fatal("expected enable to refuse an occupied non-symlink target")
	}
}

func TestDisableRemovesOnlyMatchingSymlink(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	other := filepath.Join(root, "other")
	target := filepath.Join(root, "target")
	skillPath := filepath.Join(source, "code-review")
	otherSkillPath := filepath.Join(other, "code-review")
	mustMkdir(t, skillPath)
	mustMkdir(t, otherSkillPath)
	mustMkdir(t, target)
	mustSymlink(t, otherSkillPath, filepath.Join(target, "code-review"))

	err := NewService().Disable(context.Background(), Config{TargetDirs: []string{target}}, Skill{Name: "code-review", SourcePath: skillPath})
	if err == nil {
		t.Fatal("expected disable to refuse removing a symlink pointing elsewhere")
	}
	actual, readErr := os.Readlink(filepath.Join(target, "code-review"))
	if readErr != nil {
		t.Fatal(readErr)
	}
	if actual != otherSkillPath {
		t.Fatalf("expected symlink to remain pointed at %s, got %s", otherSkillPath, actual)
	}

	err = NewService().Disable(context.Background(), Config{TargetDirs: []string{target}}, Skill{Name: "code-review", SourcePath: otherSkillPath})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(target, "code-review")); !os.IsNotExist(err) {
		t.Fatalf("expected matching symlink to be removed, lstat err = %v", err)
	}
}

func TestResolveConflictReplacesBrokenSymlink(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	target := filepath.Join(root, "target")
	skillPath := filepath.Join(source, "frontend-design")
	mustWrite(t, filepath.Join(skillPath, "SKILL.md"), "# frontend-design\n")
	mustMkdir(t, target)
	mustSymlink(t, filepath.Join(root, "missing", "frontend-design"), filepath.Join(target, "frontend-design"))

	err := NewService().ResolveConflict(context.Background(), Config{TargetDirs: []string{target}}, Skill{
		Name:       "frontend-design",
		SourcePath: skillPath,
	})
	if err != nil {
		t.Fatal(err)
	}
	actual, err := os.Readlink(filepath.Join(target, "frontend-design"))
	if err != nil {
		t.Fatal(err)
	}
	if actual != skillPath {
		t.Fatalf("expected symlink to point to %s, got %s", skillPath, actual)
	}
}

func TestStrictValidationMarksMissingSkillFileInvalid(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	target := filepath.Join(root, "target")
	mustMkdir(t, filepath.Join(source, "broken-skill"))

	inventory, err := NewService().Scan(context.Background(), Config{
		TargetDirs: []string{target},
		Sources: []SkillSourceConfig{{
			ID:      "local",
			Path:    source,
			Enabled: true,
		}},
		Validation: ValidationConfig{Mode: ValidationStrict},
	})
	if err != nil {
		t.Fatal(err)
	}

	if inventory.Summary.SkillsFound != 0 {
		t.Fatalf("expected folder without SKILL.md to be hidden, got %d skills", inventory.Summary.SkillsFound)
	}
}

func TestDuplicateSkillNamesAreConflicts(t *testing.T) {
	root := t.TempDir()
	sourceA := filepath.Join(root, "source-a")
	sourceB := filepath.Join(root, "source-b")
	target := filepath.Join(root, "target")
	mustMkdir(t, filepath.Join(sourceA, "code-review"))
	mustMkdir(t, filepath.Join(sourceB, "code-review"))
	mustWrite(t, filepath.Join(sourceA, "code-review", "SKILL.md"), "# code-review\n")
	mustWrite(t, filepath.Join(sourceB, "code-review", "SKILL.md"), "# code-review\n")

	inventory, err := NewService().Scan(context.Background(), Config{
		TargetDirs: []string{target},
		Sources: []SkillSourceConfig{
			{ID: "a", Path: sourceA, Enabled: true},
			{ID: "b", Path: sourceB, Enabled: true},
		},
		Validation: ValidationConfig{Mode: ValidationStrict},
	})
	if err != nil {
		t.Fatal(err)
	}

	var conflicts int
	for _, skill := range inventory.Skills {
		if skill.Name == "code-review" && skill.Status == StatusConflict && len(skill.ConflictSources) == 2 {
			conflicts++
		}
	}
	if conflicts != 2 {
		t.Fatalf("expected both duplicate skills to be conflicts, got %d", conflicts)
	}
}

func TestDuplicateSkillActiveSourceIsStillMarkedActive(t *testing.T) {
	root := t.TempDir()
	sourceA := filepath.Join(root, "source-a")
	sourceB := filepath.Join(root, "source-b")
	target := filepath.Join(root, "target")
	activeSkill := filepath.Join(sourceA, "code-review")
	inactiveSkill := filepath.Join(sourceB, "code-review")
	mustWrite(t, filepath.Join(activeSkill, "SKILL.md"), "# active\n")
	mustWrite(t, filepath.Join(inactiveSkill, "SKILL.md"), "# inactive\n")
	mustMkdir(t, target)
	mustSymlink(t, activeSkill, filepath.Join(target, "code-review"))

	inventory, err := NewService().Scan(context.Background(), Config{
		TargetDirs: []string{target},
		Sources: []SkillSourceConfig{
			{ID: "a", Path: sourceA, Enabled: true},
			{ID: "b", Path: sourceB, Enabled: true},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	for _, skill := range inventory.Skills {
		if skill.SourcePath == activeSkill && !skill.IsActive {
			t.Fatalf("expected active duplicate skill to be marked active")
		}
		if skill.SourcePath == inactiveSkill && skill.IsActive {
			t.Fatalf("expected inactive duplicate skill not to be marked active")
		}
	}
}

func TestScanParsesSkillManifestFrontmatter(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	skillPath := filepath.Join(source, "manifest-skill")
	mustWrite(t, filepath.Join(skillPath, "SKILL.md"), `---
name: Manifest Skill
description: Helps users manage local skills.
license: MIT
compatibility: Claude Code
metadata:
  owner: tools
  tier: core
allowed-tools: Read, Write
when-to-use: Use when editing local skill manifests.
disable-model-invocation: true
user-invocable: true
argument-hint: "[path]"
arguments:
  - path
  - mode
---

# Manifest Skill
`)

	inventory, err := NewService().Scan(context.Background(), Config{
		TargetDirs: []string{filepath.Join(root, "target")},
		Sources: []SkillSourceConfig{{
			ID:      "local",
			Path:    source,
			Enabled: true,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(inventory.Skills) != 1 {
		t.Fatalf("expected one skill, got %d", len(inventory.Skills))
	}
	manifest := inventory.Skills[0].Manifest
	if manifest == nil {
		t.Fatal("expected manifest to be parsed")
	}
	if manifest.Name != "Manifest Skill" {
		t.Fatalf("unexpected manifest name: %q", manifest.Name)
	}
	if inventory.Skills[0].Description != "Helps users manage local skills." {
		t.Fatalf("expected description from manifest, got %q", inventory.Skills[0].Description)
	}
	if manifest.Metadata["owner"] != "tools" || manifest.Metadata["tier"] != "core" {
		t.Fatalf("unexpected metadata: %#v", manifest.Metadata)
	}
	if manifest.AllowedTools != "Read, Write" || manifest.WhenToUse == "" {
		t.Fatalf("expected Claude-compatible fields, got %#v", manifest)
	}
	if manifest.DisableModelInvocation == nil || !*manifest.DisableModelInvocation ||
		manifest.UserInvocable == nil || !*manifest.UserInvocable {
		t.Fatalf("expected boolean manifest fields to parse, got %#v", manifest)
	}
	args, ok := manifest.Arguments.([]string)
	if !ok || len(args) != 2 || args[0] != "path" || args[1] != "mode" {
		t.Fatalf("unexpected arguments: %#v", manifest.Arguments)
	}
}

func TestReadEnvFileReturnsEmptyWhenMissing(t *testing.T) {
	root := t.TempDir()
	skillPath := filepath.Join(root, "source", "env-skill")
	mustWrite(t, filepath.Join(skillPath, "SKILL.md"), "# env skill\n")

	content, err := NewService().ReadEnvFile(Skill{SourcePath: skillPath})
	if err != nil {
		t.Fatal(err)
	}
	if content != "" {
		t.Fatalf("expected missing .env to read as empty content, got %q", content)
	}
}

func TestSaveEnvFileWritesEnvInSkillFolder(t *testing.T) {
	root := t.TempDir()
	skillPath := filepath.Join(root, "source", "env-skill")
	mustWrite(t, filepath.Join(skillPath, "SKILL.md"), "# env skill\n")

	err := NewService().SaveEnvFile(Skill{SourcePath: skillPath}, "API_KEY=secret\n")
	if err != nil {
		t.Fatal(err)
	}

	content, err := os.ReadFile(filepath.Join(skillPath, ".env"))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "API_KEY=secret\n" {
		t.Fatalf("unexpected .env content: %q", string(content))
	}
}

func TestSaveEnvFileRefusesNonSkillFolder(t *testing.T) {
	root := t.TempDir()
	notSkillPath := filepath.Join(root, "source", "not-skill")
	mustMkdir(t, notSkillPath)

	err := NewService().SaveEnvFile(Skill{SourcePath: notSkillPath}, "API_KEY=secret\n")
	if err == nil {
		t.Fatal("expected saving .env to require a SKILL.md folder")
	}
}

func TestScanSkipsDotGitAndFoldersWithoutSkillFile(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	mustMkdir(t, filepath.Join(source, ".git"))
	mustMkdir(t, filepath.Join(source, "notes"))
	mustMkdir(t, filepath.Join(source, "real-skill"))
	mustWrite(t, filepath.Join(source, "real-skill", "SKILL.md"), "# real skill\n")

	inventory, err := NewService().Scan(context.Background(), Config{
		TargetDirs: []string{filepath.Join(root, "target")},
		Sources: []SkillSourceConfig{{
			ID:      "local",
			Path:    source,
			Enabled: true,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}

	if inventory.Summary.SkillsFound != 1 {
		t.Fatalf("expected only folders with SKILL.md to be shown, got %d", inventory.Summary.SkillsFound)
	}
	assertSkillStatus(t, inventory, "real-skill", StatusDisabled)
}

func TestScanMarksSourceInsideGitRepository(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	source := filepath.Join(repo, "skills")
	bin := filepath.Join(root, "bin")
	mustMkdir(t, bin)
	mustWriteMode(t, filepath.Join(bin, "git"), "#!/bin/sh\nif [ \"$1\" = \"-C\" ] && [ \"$3\" = \"rev-parse\" ] && [ \"$4\" = \"--show-toplevel\" ]; then\n  printf '%s\\n' \"$TEST_GIT_ROOT\"\n  exit 0\nfi\nexit 1\n", 0o755)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("TEST_GIT_ROOT", repo)

	mustMkdir(t, filepath.Join(source, "real-skill"))
	mustWrite(t, filepath.Join(source, "real-skill", "SKILL.md"), "# real skill\n")

	inventory, err := NewService().Scan(context.Background(), Config{
		TargetDirs: []string{filepath.Join(root, "target")},
		Sources: []SkillSourceConfig{{
			ID:      "local",
			Path:    source,
			Enabled: true,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(inventory.Sources) != 1 {
		t.Fatalf("expected one source, got %d", len(inventory.Sources))
	}
	if !inventory.Sources[0].IsGitRepo {
		t.Fatal("expected nested source folder to be marked as inside a git repository")
	}
	if inventory.Sources[0].GitRoot != repo {
		t.Fatalf("expected git root %s, got %s", repo, inventory.Sources[0].GitRoot)
	}
}

func TestScanRepositoryDiscoversNestedSkillFiles(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	mustWrite(t, filepath.Join(repo, "skills", "code-review", "SKILL.md"), "# code review\n")
	mustWrite(t, filepath.Join(repo, "agents", "writing", "summarize-pdf", "SKILL.md"), "# summarize\n")
	mustWrite(t, filepath.Join(repo, "node_modules", "ignored", "SKILL.md"), "# ignored\n")
	bin := filepath.Join(root, "bin")
	mustMkdir(t, bin)
	mustWriteMode(t, filepath.Join(bin, "git"), "#!/bin/sh\ncase \"$3 $4\" in\n\"rev-parse --show-toplevel\") printf '%s\\n' \"$TEST_GIT_ROOT\";;\n\"branch --show-current\") printf 'main\\n';;\n\"status --porcelain\") exit 0;;\n*) exit 1;;\nesac\n", 0o755)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("TEST_GIT_ROOT", repo)

	inventory, err := NewService().Scan(context.Background(), Config{
		TargetDirs: []string{filepath.Join(root, "target")},
		Repositories: []RepositoryConfig{{
			ID:        "example.com/me/repo",
			RepoID:    "example.com/me/repo",
			Path:      repo,
			Enabled:   true,
			ScanRoots: []string{"."},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}

	if inventory.Summary.SkillsFound != 2 {
		t.Fatalf("expected two nested skills, got %d", inventory.Summary.SkillsFound)
	}
	assertSkillStatus(t, inventory, "code-review", StatusDisabled)
	assertSkillStatus(t, inventory, "summarize-pdf", StatusDisabled)
}

func TestScanRepositoryRootSkillUsesRepositoryFolderAsTargetName(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "serenity-skill")
	mustWrite(t, filepath.Join(repo, "SKILL.md"), "# serenity\n")
	bin := filepath.Join(root, "bin")
	mustMkdir(t, bin)
	mustWriteMode(t, filepath.Join(bin, "git"), "#!/bin/sh\ncase \"$3 $4\" in\n\"rev-parse --show-toplevel\") printf '%s\\n' \"$TEST_GIT_ROOT\";;\n\"branch --show-current\") printf 'main\\n';;\n*) exit 1;;\nesac\n", 0o755)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("TEST_GIT_ROOT", repo)

	inventory, err := NewService().Scan(context.Background(), Config{
		TargetDirs: []string{filepath.Join(root, "target")},
		Repositories: []RepositoryConfig{{
			ID:      "example.com/me/serenity-skill",
			RepoID:  "example.com/me/serenity-skill",
			Path:    repo,
			Enabled: true,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(inventory.Skills) != 1 {
		t.Fatalf("expected one root skill, got %d", len(inventory.Skills))
	}
	skill := inventory.Skills[0]
	if skill.Name != "serenity-skill" || skill.TargetName != "serenity-skill" {
		t.Fatalf("expected root skill target name serenity-skill, got name=%q targetName=%q", skill.Name, skill.TargetName)
	}
	if skill.TargetPath != filepath.Join(root, "target", "serenity-skill") {
		t.Fatalf("expected target path under skill name, got %s", skill.TargetPath)
	}
	assertSkillStatus(t, inventory, "serenity-skill", StatusDisabled)
}

func TestScanWithSyncShowsMissingSource(t *testing.T) {
	root := t.TempDir()
	enabled := false
	inventory, err := NewService().ScanWithSync(context.Background(), Config{
		TargetDirs: []string{filepath.Join(root, "target")},
	}, SyncDocument{Version: 2, Skills: map[string]SyncSkillRecord{
		"git:example.com/me/repo//skills/code-review": {
			Enabled:    true,
			TargetName: "code-review",
			Note:       "Review risky changes first.",
			Source:     SyncSource{Provider: GitProvider, ID: "example.com/me/repo", Locator: SourceLocator{Subpath: "skills/code-review"}},
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(inventory.Skills) != 1 {
		t.Fatalf("expected one synced skill, got %d", len(inventory.Skills))
	}
	if inventory.Skills[0].Status != StatusMissingSource {
		t.Fatalf("expected missing source, got %s", inventory.Skills[0].Status)
	}
	if inventory.Skills[0].CanRemove {
		t.Fatal("expected a skill in an unavailable repository to require repository recovery")
	}
	if inventory.Skills[0].DesiredEnabled == nil || *inventory.Skills[0].DesiredEnabled != enabled {
		t.Fatalf("expected device default disabled, got %#v", inventory.Skills[0].DesiredEnabled)
	}
	if inventory.Skills[0].Note != "Review risky changes first." {
		t.Fatalf("expected note on missing skill, got %q", inventory.Skills[0].Note)
	}
	if len(inventory.Repositories) != 1 || inventory.Repositories[0].Installed || inventory.Repositories[0].RepoID != "example.com/me/repo" {
		t.Fatalf("expected shared repository to remain visible as missing, got %#v", inventory.Repositories)
	}
}

func TestScanWithSyncUsesMissingSourceWhenRepositoryExistsButSkillDoesNot(t *testing.T) {
	root := t.TempDir()
	repositoryPath := filepath.Join(root, "repo")
	mustMkdir(t, repositoryPath)
	inventory, err := NewService().ScanWithSync(context.Background(), Config{
		TargetDirs: []string{filepath.Join(root, "target")},
		Repositories: []RepositoryConfig{{
			ID:      "example.com/me/repo",
			RepoID:  "example.com/me/repo",
			Path:    repositoryPath,
			Enabled: true,
		}},
	}, SyncDocument{Version: 2, Skills: map[string]SyncSkillRecord{
		"git:example.com/me/repo//skills/missing": {
			TargetName: "missing",
			Source:     SyncSource{Provider: GitProvider, ID: "example.com/me/repo", Locator: SourceLocator{Subpath: "skills/missing"}},
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(inventory.Skills) != 1 || inventory.Skills[0].Status != StatusMissingSource {
		t.Fatalf("expected missing source for absent skill path, got %#v", inventory.Skills)
	}
	if inventory.Skills[0].SourcePath != filepath.Join(repositoryPath, "skills", "missing") {
		t.Fatalf("expected projected missing path, got %q", inventory.Skills[0].SourcePath)
	}
	if !inventory.Skills[0].CanRemove {
		t.Fatal("expected missing skill in an installed repository to be removable")
	}
}

func TestProjectSharedRepositoriesIncludesMissingAndInstalledSources(t *testing.T) {
	document := SyncDocument{Version: 2, Skills: map[string]SyncSkillRecord{
		"git:example.com/missing/repo//skills/one": {
			TargetName: "one",
			Source: SyncSource{
				Provider: GitProvider,
				ID:       "example.com/missing/repo",
				Locator:  SourceLocator{CloneURL: "https://example.com/missing/repo.git", Subpath: "skills/one"},
			},
		},
		"git:example.com/missing/repo//skills/two": {
			TargetName: "two",
			Source: SyncSource{
				Provider: GitProvider,
				ID:       "example.com/missing/repo",
				Locator:  SourceLocator{CloneURL: "https://example.com/missing/repo.git", Subpath: "skills/two"},
			},
		},
		"git:example.com/installed/repo//skill": {
			TargetName: "skill",
			Source: SyncSource{
				Provider: GitProvider,
				ID:       "example.com/installed/repo",
				Locator:  SourceLocator{CloneURL: "https://example.com/installed/repo.git", Subpath: "skill"},
			},
		},
	}}
	projected := projectSharedRepositories([]Repository{{
		RepoID:     "example.com/installed/repo",
		Path:       "/tmp/installed",
		Installed:  true,
		SkillCount: 3,
	}}, document)
	if len(projected) != 2 {
		t.Fatalf("expected two projected repositories, got %#v", projected)
	}
	byID := map[string]Repository{}
	for _, repository := range projected {
		byID[repository.RepoID] = repository
	}
	missing := byID["example.com/missing/repo"]
	if missing.Installed || missing.SkillCount != 2 || missing.CloneURL == "" {
		t.Fatalf("unexpected missing repository projection: %#v", missing)
	}
	installed := byID["example.com/installed/repo"]
	if !installed.Installed || installed.Path != "/tmp/installed" || installed.SkillCount != 1 {
		t.Fatalf("unexpected installed repository projection: %#v", installed)
	}
}

func TestProjectSharedRepositoriesPreservesLocalSourceWithoutSharedSkills(t *testing.T) {
	projected := projectSharedRepositories([]Repository{{
		RepoID:     "example.com/local/repo",
		Path:       "/tmp/local",
		SkillCount: 4,
	}}, SyncDocument{Version: 2, Skills: map[string]SyncSkillRecord{}})
	if len(projected) != 1 || !projected[0].Installed || projected[0].SkillCount != 4 {
		t.Fatalf("expected local-only installation to remain visible, got %#v", projected)
	}
}

func TestScanWithSyncLeavesAvailableDesiredSkillDisabledUntilReconciled(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	mustWrite(t, filepath.Join(repo, "skills", "code-review", "SKILL.md"), "# code review\n")
	bin := filepath.Join(root, "bin")
	mustMkdir(t, bin)
	mustWriteMode(t, filepath.Join(bin, "git"), "#!/bin/sh\ncase \"$3 $4\" in\n\"rev-parse --show-toplevel\") printf '%s\\n' \"$TEST_GIT_ROOT\";;\n\"branch --show-current\") printf 'main\\n';;\n\"status --porcelain\") exit 0;;\n*) exit 1;;\nesac\n", 0o755)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("TEST_GIT_ROOT", repo)

	inventory, err := NewService().ScanWithSync(context.Background(), Config{
		TargetDirs: []string{filepath.Join(root, "target")},
		Repositories: []RepositoryConfig{{
			ID:        "example.com/me/repo",
			RepoID:    "example.com/me/repo",
			Path:      repo,
			Enabled:   true,
			ScanRoots: []string{"."},
		}},
	}, SyncDocument{Version: 2, Skills: map[string]SyncSkillRecord{
		"git:example.com/me/repo//skills/code-review": {
			Enabled:    true,
			TargetName: "code-review",
			Source:     SyncSource{Provider: GitProvider, ID: "example.com/me/repo", Locator: SourceLocator{Subpath: "skills/code-review"}},
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	assertSkillStatus(t, inventory, "code-review", StatusDisabled)
}

func TestScanWithSyncAppliesTopLevelProfileWithoutSyncingSkill(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	mustWrite(t, filepath.Join(repo, "skills", "code-review", "SKILL.md"), "# code review\n")
	bin := filepath.Join(root, "bin")
	mustMkdir(t, bin)
	mustWriteMode(t, filepath.Join(bin, "git"), "#!/bin/sh\ncase \"$3 $4\" in\n\"rev-parse --show-toplevel\") printf '%s\\n' \"$TEST_GIT_ROOT\";;\n\"branch --show-current\") printf 'main\\n';;\n\"status --porcelain\") exit 0;;\n*) exit 1;;\nesac\n", 0o755)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("TEST_GIT_ROOT", repo)

	syncID := "git:example.com/me/repo//skills/code-review"
	inventory, err := NewService().ScanWithSync(context.Background(), Config{
		TargetDirs: []string{filepath.Join(root, "target")},
		Repositories: []RepositoryConfig{{
			ID:        "example.com/me/repo",
			RepoID:    "example.com/me/repo",
			Path:      repo,
			Enabled:   true,
			ScanRoots: []string{"."},
		}},
	}, SyncDocument{Version: 2, Profiles: map[string]SkillProfile{
		syncID: {
			SummaryZh:  "代码审阅助手。",
			UseCasesZh: []string{"检查 PR 风险。"},
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(inventory.Skills) != 1 {
		t.Fatalf("expected one skill, got %d", len(inventory.Skills))
	}
	skill := inventory.Skills[0]
	if skill.Profile == nil || skill.Profile.SummaryZh != "代码审阅助手。" {
		t.Fatalf("expected top-level profile to be applied, got %#v", skill.Profile)
	}
	if skill.IsSynced || skill.DesiredEnabled != nil || skill.Status != StatusDisabled {
		t.Fatalf("profile-only skill should remain unsynced disabled, got synced=%v desired=%#v status=%s", skill.IsSynced, skill.DesiredEnabled, skill.Status)
	}
}

func assertSkillStatus(t *testing.T, inventory Inventory, name string, status SkillStatus) {
	t.Helper()
	for _, skill := range inventory.Skills {
		if skill.Name == name {
			if skill.Status != status {
				t.Fatalf("expected %s to be %s, got %s", name, status, skill.Status)
			}
			return
		}
	}
	t.Fatalf("skill %s not found", name)
}

func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
}

func mustSymlink(t *testing.T, oldname, newname string) {
	t.Helper()
	if err := os.Symlink(oldname, newname); err != nil {
		t.Fatal(err)
	}
}

func mustWrite(t *testing.T, path string, data string) {
	t.Helper()
	mustWriteMode(t, path, data, 0o644)
}

func mustWriteMode(t *testing.T, path string, data string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), mode); err != nil {
		t.Fatal(err)
	}
}
