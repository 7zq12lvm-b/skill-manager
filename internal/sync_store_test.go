package skillmgr

import (
	"bytes"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
)

func TestSyncPathFromFolderUsesDatabaseFilename(t *testing.T) {
	folder := filepath.Join(t.TempDir(), "shared")
	want := filepath.Join(folder, "skillManager.db")
	if got := SyncPathFromFolder(folder); got != want {
		t.Fatalf("expected sync database path %q, got %q", want, got)
	}
}

func TestSyncStoreSavesLLMConfigAndSkillProfile(t *testing.T) {
	path := filepath.Join(t.TempDir(), SyncFileName)
	store := NewSyncStore(path)
	syncID := "git:example.com/me/repo//skills/code-review"

	err := store.Save(SyncDocument{
		Version: 2,
		Skills: map[string]SyncSkillRecord{
			syncID: {
				Enabled:    true,
				TargetName: "code-review",
				Source:     SyncSource{Provider: GitProvider, ID: "example.com/me/repo", Locator: SourceLocator{Subpath: "skills/code-review"}},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	const sqliteHeader = "SQLite format 3\x00"
	if len(data) < len(sqliteHeader) || string(data[:len(sqliteHeader)]) != sqliteHeader {
		t.Fatalf("expected SQLite database header, got %q", data)
	}
	if err := store.SaveLLMConfig(SyncLLMConfig{
		BaseURL:     " https://api.deepseek.com ",
		APIKey:      " secret ",
		Model:       " deepseek-v4-flash ",
		Temperature: -1,
		MaxTokens:   -100,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertSkillProfile(syncID, SkillProfile{
		SummaryZh:  "  代码审阅助手。 ",
		UseCasesZh: []string{"指出回归风险。", " 解释修改建议。 ", ""},
		Model:      " deepseek-v4-flash ",
	}); err != nil {
		t.Fatal(err)
	}

	document, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if document.LLM.BaseURL != "https://api.deepseek.com" || document.LLM.APIKey != "secret" ||
		document.LLM.Model != "deepseek-v4-flash" || document.LLM.Temperature != 0 || document.LLM.MaxTokens != 0 {
		t.Fatalf("unexpected LLM config: %#v", document.LLM)
	}
	profile, ok := document.Profiles[syncID]
	if !ok {
		t.Fatalf("expected top-level profile for %s", syncID)
	}
	if profile.SummaryZh != "代码审阅助手。" || len(profile.UseCasesZh) != 2 ||
		profile.UseCasesZh[0] != "指出回归风险。" || profile.UseCasesZh[1] != "解释修改建议。" {
		t.Fatalf("unexpected profile: %#v", profile)
	}
	if document.Skills[syncID].Profile == nil || document.Skills[syncID].Profile.SummaryZh != "代码审阅助手。" {
		t.Fatalf("expected synced skill record to mirror profile, got %#v", document.Skills[syncID].Profile)
	}
}

func TestSyncStoreUpsertSkillsWritesMultipleRecords(t *testing.T) {
	store := NewSyncStore(filepath.Join(t.TempDir(), SyncFileName))
	records := []SyncSkillRecord{
		{
			Enabled:             true,
			PreviousTargetNames: []string{" old-z ", "old-a", "old-a"},
			Tags:                []string{" zeta ", "alpha", "zeta"},
			Source:              SyncSource{Provider: GitProvider, ID: "example.com/me/repo", Locator: SourceLocator{Subpath: "skills/review"}},
		},
		{
			Tags:   []string{"writing"},
			Source: SyncSource{Provider: GitProvider, ID: "example.com/me/repo", Locator: SourceLocator{Subpath: "skills/write"}},
		},
	}
	if err := store.UpsertSkills(records); err != nil {
		t.Fatal(err)
	}
	document, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(document.Skills) != 2 {
		t.Fatalf("expected two records, got %#v", document.Skills)
	}
	var sharedUpdatedAt string
	for id, record := range document.Skills {
		if record.UpdatedAt == "" {
			t.Fatalf("expected %s to have a shared update timestamp", id)
		}
		if sharedUpdatedAt == "" {
			sharedUpdatedAt = record.UpdatedAt
		} else if record.UpdatedAt != sharedUpdatedAt {
			t.Fatalf("expected one shared update timestamp, got %q and %q", sharedUpdatedAt, record.UpdatedAt)
		}
	}
	if tags := document.Skills["git:example.com/me/repo//skills/review"].Tags; len(tags) != 2 || tags[0] != "alpha" || tags[1] != "zeta" {
		t.Fatalf("expected normalized tags, got %#v", tags)
	}
	if names := document.Skills["git:example.com/me/repo//skills/review"].PreviousTargetNames; len(names) != 2 || names[0] != "old-a" || names[1] != "old-z" {
		t.Fatalf("expected normalized previous target names, got %#v", names)
	}
}

func TestSyncStoreUpsertSkillsIsAtomic(t *testing.T) {
	store := NewSyncStore(filepath.Join(t.TempDir(), SyncFileName))
	err := store.UpsertSkills([]SyncSkillRecord{
		{Source: SyncSource{Provider: GitProvider, ID: "example.com/me/repo", Locator: SourceLocator{Subpath: "skills/valid"}}},
		{Source: SyncSource{Provider: "local", ID: "local", Locator: SourceLocator{Subpath: "skills/invalid"}}},
	})
	if err == nil {
		t.Fatal("expected incomplete sync source to reject the batch")
	}
	document, loadErr := store.Load()
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	if len(document.Skills) != 0 {
		t.Fatalf("expected failed batch to write no records, got %#v", document.Skills)
	}
}

func TestSyncStoreNormalizesAndClearsSkillNote(t *testing.T) {
	path := filepath.Join(t.TempDir(), SyncFileName)
	store := NewSyncStore(path)
	record := SyncSkillRecord{
		Note:   "  first line\nsecond line  ",
		Source: SyncSource{Provider: GitProvider, ID: "example.com/me/repo", Locator: SourceLocator{Subpath: "skills/review"}},
	}
	if err := store.UpsertSkill(record); err != nil {
		t.Fatal(err)
	}
	document, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	syncID := "git:example.com/me/repo//skills/review"
	if note := document.Skills[syncID].Note; note != "first line\nsecond line" {
		t.Fatalf("expected normalized multiline note, got %q", note)
	}
	record.Note = " \n\t "
	if err := store.UpsertSkill(record); err != nil {
		t.Fatal(err)
	}
	document, err = store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if note := document.Skills[syncID].Note; note != "" {
		t.Fatalf("expected note to be cleared, got %q", note)
	}
}

func TestSyncStoreDeleteSkillPreservesOtherSharedState(t *testing.T) {
	store := NewSyncStore(filepath.Join(t.TempDir(), SyncFileName))
	deletedID := "git:example.com/me/repo//skills/deleted"
	keptID := "git:example.com/me/repo//skills/kept"
	document := SyncDocument{
		Version: 2,
		LLM: SyncLLMConfig{
			BaseURL: "https://api.example.com",
			APIKey:  "secret",
			Model:   "model-v1",
		},
		Profiles: map[string]SkillProfile{
			deletedID: {SummaryZh: "保留的独立简介。", UseCasesZh: []string{"用于验证删除语义。"}},
		},
		Skills: map[string]SyncSkillRecord{
			deletedID: {
				Tags:   []string{"deleted"},
				Source: SyncSource{Provider: GitProvider, ID: "example.com/me/repo", Locator: SourceLocator{Subpath: "skills/deleted"}},
			},
			keptID: {
				Enabled: true,
				Tags:    []string{"kept"},
				Source:  SyncSource{Provider: GitProvider, ID: "example.com/me/repo", Locator: SourceLocator{Subpath: "skills/kept"}},
			},
		},
	}
	if err := store.Save(document); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteSkill(deletedID); err != nil {
		t.Fatal(err)
	}

	loaded, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := loaded.Skills[deletedID]; exists {
		t.Fatalf("expected %s to be deleted", deletedID)
	}
	if record, exists := loaded.Skills[keptID]; !exists || !record.Enabled || len(record.Tags) != 1 || record.Tags[0] != "kept" {
		t.Fatalf("expected other skill to remain unchanged, got %#v", record)
	}
	if profile, exists := loaded.Profiles[deletedID]; !exists || profile.SummaryZh != "保留的独立简介。" {
		t.Fatalf("expected top-level profile to survive skill deletion, got %#v", profile)
	}
	if loaded.LLM.BaseURL != "https://api.example.com" || loaded.LLM.APIKey != "secret" || loaded.LLM.Model != "model-v1" {
		t.Fatalf("expected LLM config to remain unchanged, got %#v", loaded.LLM)
	}
}

func TestSyncStoreCheckIntegrityAcceptsValidDatabase(t *testing.T) {
	store := NewSyncStore(filepath.Join(t.TempDir(), SyncFileName))
	if err := store.Save(SyncDocument{Version: 2, Skills: map[string]SyncSkillRecord{}}); err != nil {
		t.Fatal(err)
	}
	if err := store.CheckIntegrity(); err != nil {
		t.Fatalf("expected valid database to pass integrity checks: %v", err)
	}
}

func TestSyncStoreSkillUpsertDoesNotOverwriteAuthoritativeProfile(t *testing.T) {
	store := NewSyncStore(filepath.Join(t.TempDir(), SyncFileName))
	syncID := "git:example.com/me/repo//skills/review"
	authoritative := SkillProfile{SummaryZh: "权威简介。", UseCasesZh: []string{"权威用例。"}}
	if err := store.Save(SyncDocument{
		Version:  2,
		Profiles: map[string]SkillProfile{syncID: authoritative},
		Skills: map[string]SyncSkillRecord{
			syncID: {
				Profile: &authoritative,
				Source:  SyncSource{Provider: GitProvider, ID: "example.com/me/repo", Locator: SourceLocator{Subpath: "skills/review"}},
			},
		},
	}); err != nil {
		t.Fatal(err)
	}
	stale := SkillProfile{SummaryZh: "过期简介。", UseCasesZh: []string{"过期用例。"}}
	if err := store.UpsertSkill(SyncSkillRecord{
		Note:    "updated note",
		Profile: &stale,
		Source:  SyncSource{Provider: GitProvider, ID: "example.com/me/repo", Locator: SourceLocator{Subpath: "skills/review"}},
	}); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if profile := loaded.Profiles[syncID]; profile.SummaryZh != "权威简介。" || len(profile.UseCasesZh) != 1 || profile.UseCasesZh[0] != "权威用例。" {
		t.Fatalf("unexpected top-level profile: %#v", profile)
	}
	if profile := loaded.Skills[syncID].Profile; profile == nil || profile.SummaryZh != "权威简介。" {
		t.Fatalf("expected skill mirror to use authoritative profile, got %#v", profile)
	}
}

func TestSyncStoreLoadPreservesCorruptDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), SyncFileName)
	original := []byte("not a SQLite database")
	if err := os.WriteFile(path, original, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := NewSyncStore(path).Load(); err == nil {
		t.Fatal("expected corrupt database to fail loading")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, original) {
		t.Fatal("corrupt database was modified or replaced")
	}
}

func TestSyncStoreDoesNotImportOrModifyLegacyJSON(t *testing.T) {
	root := t.TempDir()
	legacyPath := filepath.Join(root, "skill-manager-sync.json")
	legacy := []byte(`{
  "version": 2,
  "skills": {
    "git:example.com/me/repo//skills/legacy": {
      "enabled": true,
      "targetName": "legacy",
      "source": {
        "provider": "git",
        "id": "example.com/me/repo",
        "locator": {"subpath": "skills/legacy"}
      }
    }
  }
}`)
	if err := os.WriteFile(legacyPath, legacy, 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := NewSyncStore(filepath.Join(root, SyncFileName)).Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Skills) != 0 || len(loaded.Profiles) != 0 {
		t.Fatalf("expected app storage to ignore legacy JSON, got %#v", loaded)
	}
	after, err := os.ReadFile(legacyPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, legacy) {
		t.Fatal("legacy JSON was modified")
	}
}

func TestSyncStoreLoadPreservesUnsupportedSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), SyncFileName)
	store := NewSyncStore(path)
	if err := store.Save(SyncDocument{Version: 2, Skills: map[string]SyncSkillRecord{}}); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE schema_meta SET version = 999 WHERE id = 1`); err != nil {
		db.Close()
		t.Fatal(err)
	}
	var journalMode string
	if err := db.QueryRow(`PRAGMA journal_mode=WAL`).Scan(&journalMode); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if journalMode != "wal" {
		db.Close()
		t.Fatalf("expected WAL fixture, got %q", journalMode)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load(); err == nil {
		t.Fatal("expected unsupported schema to fail loading")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, before) {
		t.Fatal("unsupported database was modified")
	}
}
