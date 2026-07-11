package skillmgr

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSyncStoreUsesSQLiteDatabaseInSyncFolder(t *testing.T) {
	folder := t.TempDir()
	path := SyncPathFromFolder(folder)
	if filepath.Ext(path) != ".db" {
		t.Fatalf("expected SQLite database path, got %q", path)
	}

	store := NewSyncStore(path)
	if err := store.UpsertSkill(SyncSkillRecord{
		Enabled: true,
		Source:  SyncSource{Provider: GitProvider, ID: "example.com/me/repo", Locator: SourceLocator{Subpath: "skills/review"}},
	}); err != nil {
		t.Fatal(err)
	}

	header := make([]byte, 16)
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if _, err := file.Read(header); err != nil {
		t.Fatal(err)
	}
	if string(header) != "SQLite format 3\x00" {
		t.Fatalf("expected SQLite file header, got %q", header)
	}
}

func TestSyncStoreMigratesLegacyJSONOnce(t *testing.T) {
	folder := t.TempDir()
	legacyPath := filepath.Join(folder, LegacySyncFileName)
	syncID := "git:example.com/me/repo//skills/review"
	legacy := `{
  "version": 2,
  "llm": {"baseUrl": "https://example.test", "model": "test-model"},
  "profiles": {"` + syncID + `": {"summaryZh": "迁移后的简介。"}},
  "skills": {
    "` + syncID + `": {
      "enabled": true,
      "targetName": "review",
      "tags": ["quality"],
      "source": {"provider": "git", "id": "example.com/me/repo", "locator": {"subpath": "skills/review"}}
    }
  }
}`
	if err := os.WriteFile(legacyPath, []byte(legacy), 0o644); err != nil {
		t.Fatal(err)
	}

	store := NewSyncStore(SyncPathFromFolder(folder))
	document, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if !document.Skills[syncID].Enabled || document.Skills[syncID].Tags[0] != "quality" {
		t.Fatalf("legacy skill was not migrated: %#v", document.Skills[syncID])
	}
	if document.Profiles[syncID].SummaryZh != "迁移后的简介。" || document.LLM.Model != "test-model" {
		t.Fatalf("legacy metadata was not migrated: %#v %#v", document.Profiles, document.LLM)
	}
	if _, err := os.Stat(legacyPath + LegacySyncBackupSuffix); err != nil {
		t.Fatalf("expected migrated JSON backup: %v", err)
	}

	if err := os.WriteFile(legacyPath, []byte(`{"version":2,"skills":{}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	document, err = store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := document.Skills[syncID]; !ok {
		t.Fatal("existing database should win over a later legacy JSON file")
	}
}

func TestSyncStoreLeavesDatabaseQuiescentAfterEachOperation(t *testing.T) {
	folder := t.TempDir()
	store := NewSyncStore(SyncPathFromFolder(folder))
	record := SyncSkillRecord{
		Enabled: true,
		Source:  SyncSource{Provider: GitProvider, ID: "example.com/me/repo", Locator: SourceLocator{Subpath: "skills/review"}},
	}
	if err := store.UpsertSkill(record); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load(); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveLLMConfig(SyncLLMConfig{Model: "test-model"}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertSkillProfile(syncRecordID(record), SkillProfile{SummaryZh: "简介"}); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteSkill(syncRecordID(record)); err != nil {
		t.Fatal(err)
	}

	entries, err := os.ReadDir(folder)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != SyncFileName {
		names := make([]string, 0, len(entries))
		for _, entry := range entries {
			names = append(names, entry.Name())
		}
		t.Fatalf("expected a closed, single-file SQLite database, got %v", names)
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
		UseCasesZh: []string{"指出回归风险。", ""},
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
	if profile.SummaryZh != "代码审阅助手。" || len(profile.UseCasesZh) != 1 {
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
			Enabled: true,
			Tags:    []string{" review ", "review"},
			Source:  SyncSource{Provider: GitProvider, ID: "example.com/me/repo", Locator: SourceLocator{Subpath: "skills/review"}},
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
	for id, record := range document.Skills {
		if record.UpdatedAt == "" {
			t.Fatalf("expected %s to have a shared update timestamp", id)
		}
	}
	if tags := document.Skills["git:example.com/me/repo//skills/review"].Tags; len(tags) != 1 || tags[0] != "review" {
		t.Fatalf("expected normalized tags, got %#v", tags)
	}
}
