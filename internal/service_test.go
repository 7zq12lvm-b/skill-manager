package skillmgr

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

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
	assertSkillStatus(t, inventory, "summarize-pdf", StatusSynced)
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

	assertSkillStatus(t, inventory, "code-review", StatusSyncing)
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
