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
		TargetDir: target,
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
		TargetDir: target,
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
	config := Config{TargetDir: target, Validation: ValidationConfig{Mode: ValidationLoose}}
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

	err := NewService().Enable(context.Background(), Config{TargetDir: target}, Skill{Name: "code-review", SourcePath: skillPath})
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

	err := NewService().Disable(context.Background(), Config{TargetDir: target}, Skill{Name: "code-review", SourcePath: skillPath})
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

	err = NewService().Disable(context.Background(), Config{TargetDir: target}, Skill{Name: "code-review", SourcePath: otherSkillPath})
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
		TargetDir: target,
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
		TargetDir: target,
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
		TargetDir: target,
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

func TestScanSkipsDotGitAndFoldersWithoutSkillFile(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	mustMkdir(t, filepath.Join(source, ".git"))
	mustMkdir(t, filepath.Join(source, "notes"))
	mustMkdir(t, filepath.Join(source, "real-skill"))
	mustWrite(t, filepath.Join(source, "real-skill", "SKILL.md"), "# real skill\n")

	inventory, err := NewService().Scan(context.Background(), Config{
		TargetDir: filepath.Join(root, "target"),
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
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}
