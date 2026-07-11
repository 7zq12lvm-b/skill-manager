package skillmgr

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
)

func TestSyncStoreUsesRelationalSchemaInsteadOfJSONColumns(t *testing.T) {
	path := SyncPathFromFolder(t.TempDir())
	store := NewSyncStore(path)
	if err := store.UpsertSkill(SyncSkillRecord{
		Enabled:             true,
		PreviousTargetNames: []string{"old-review"},
		Tags:                []string{"quality"},
		Profile:             &SkillProfile{SummaryZh: "简介", UseCasesZh: []string{"审查代码"}},
		Source:              SyncSource{Provider: GitProvider, ID: "example.com/me/repo", Locator: SourceLocator{Subpath: "skills/review"}},
	}); err != nil {
		t.Fatal(err)
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	rows, err := db.Query(`SELECT name FROM pragma_table_info('skills') UNION ALL SELECT name FROM pragma_table_info('profiles')`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var column string
		if err := rows.Scan(&column); err != nil {
			t.Fatal(err)
		}
		if column == "profile_json" || column == "tags_json" || column == "previous_target_names_json" {
			t.Fatalf("structured data must not be stored in JSON column %q", column)
		}
	}
	for _, table := range []string{"skill_tags", "skill_previous_target_names", "profile_use_cases"} {
		var count int
		if err := db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("expected relational child table %q", table)
		}
	}
}

func TestSyncStoreMigratesV1DatabaseToRelationalSchema(t *testing.T) {
	path := SyncPathFromFolder(t.TempDir())
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`
CREATE TABLE schema_meta (version INTEGER NOT NULL);
INSERT INTO schema_meta(version) VALUES(1);
CREATE TABLE llm_config (id INTEGER PRIMARY KEY, base_url TEXT NOT NULL, api_key TEXT NOT NULL, model TEXT NOT NULL, temperature REAL NOT NULL, max_tokens INTEGER NOT NULL);
CREATE TABLE profiles (sync_id TEXT PRIMARY KEY, profile_json TEXT NOT NULL);
CREATE TABLE skills (sync_id TEXT PRIMARY KEY, enabled INTEGER NOT NULL, target_name TEXT NOT NULL, previous_target_names_json TEXT NOT NULL, tags_json TEXT NOT NULL, profile_json TEXT, updated_at TEXT NOT NULL, provider TEXT NOT NULL, source_id TEXT NOT NULL, clone_url TEXT NOT NULL, subpath TEXT NOT NULL, ref TEXT NOT NULL);
INSERT INTO profiles VALUES('git:example.com/me/repo//skills/review', '{"summaryZh":"简介","useCasesZh":["审查代码"]}');
INSERT INTO skills VALUES('git:example.com/me/repo//skills/review', 1, 'review', '["old-review"]', '["quality"]', '{"summaryZh":"旧的内嵌简介","useCasesZh":["旧场景"]}', '2026-07-11T00:00:00Z', 'git', 'example.com/me/repo', '', 'skills/review', 'main');`)
	if err != nil {
		db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	document, err := NewSyncStore(path).Load()
	if err != nil {
		t.Fatal(err)
	}
	record := document.Skills["git:example.com/me/repo//skills/review"]
	if len(record.Tags) != 1 || record.Tags[0] != "quality" || len(record.PreviousTargetNames) != 1 || record.Profile == nil {
		t.Fatalf("v1 data was not preserved: %#v", record)
	}
	if record.Profile.SummaryZh != "简介" || len(record.Profile.UseCasesZh) != 1 || record.Profile.UseCasesZh[0] != "审查代码" {
		t.Fatalf("top-level v1 profile must win over its stale skill mirror: %#v", record.Profile)
	}
	db, err = sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var version int
	if err := db.QueryRow(`SELECT version FROM schema_meta`).Scan(&version); err != nil || version != 2 {
		t.Fatalf("expected schema version 2, got %d, %v", version, err)
	}
}

func TestSyncStoreSavePrefersTopLevelProfileOverSkillMirror(t *testing.T) {
	path := SyncPathFromFolder(t.TempDir())
	syncID := "git:example.com/me/repo//skills/review"
	store := NewSyncStore(path)
	if err := store.Save(SyncDocument{
		Version:  2,
		Profiles: map[string]SkillProfile{syncID: {SummaryZh: "权威简介", UseCasesZh: []string{"权威场景"}}},
		Skills: map[string]SyncSkillRecord{syncID: {
			Enabled: true,
			Profile: &SkillProfile{SummaryZh: "旧镜像", UseCasesZh: []string{"旧场景"}},
			Source:  SyncSource{Provider: GitProvider, ID: "example.com/me/repo", Locator: SourceLocator{Subpath: "skills/review"}},
		}},
	}); err != nil {
		t.Fatal(err)
	}
	document, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if profile := document.Skills[syncID].Profile; profile == nil || profile.SummaryZh != "权威简介" || profile.UseCasesZh[0] != "权威场景" {
		t.Fatalf("top-level profile must remain canonical: %#v", profile)
	}
}

func TestSyncStoreSkillUpsertDoesNotOverwriteCanonicalProfile(t *testing.T) {
	store := NewSyncStore(SyncPathFromFolder(t.TempDir()))
	record := SyncSkillRecord{
		Enabled: true,
		Profile: &SkillProfile{SummaryZh: "初始镜像"},
		Source:  SyncSource{Provider: GitProvider, ID: "example.com/me/repo", Locator: SourceLocator{Subpath: "skills/review"}},
	}
	if err := store.UpsertSkill(record); err != nil {
		t.Fatal(err)
	}
	syncID := syncRecordID(record)
	if err := store.UpsertSkillProfile(syncID, SkillProfile{SummaryZh: "权威简介"}); err != nil {
		t.Fatal(err)
	}
	record.Profile = &SkillProfile{SummaryZh: "过期镜像"}
	if err := store.UpsertSkill(record); err != nil {
		t.Fatal(err)
	}
	document, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if profile := document.Profiles[syncID]; profile.SummaryZh != "权威简介" {
		t.Fatalf("skill upsert overwrote canonical profile: %#v", profile)
	}
}

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
