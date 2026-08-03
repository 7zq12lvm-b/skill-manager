package skillmgr

import (
	"bytes"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
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

func TestSyncStorePersistsStarredSkills(t *testing.T) {
	store := NewSyncStore(filepath.Join(t.TempDir(), SyncFileName))
	syncID := "git:example.com/me/repo//skills/review"
	if err := store.UpsertSkill(SyncSkillRecord{
		Starred: true,
		Source:  SyncSource{Provider: GitProvider, ID: "example.com/me/repo", Locator: SourceLocator{Subpath: "skills/review"}},
	}); err != nil {
		t.Fatal(err)
	}
	document, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if !document.Skills[syncID].Starred {
		t.Fatal("expected starred skill to remain starred after reload")
	}
}

func TestSyncStoreMigratesCompactVersionTwoSchemaForStarredSkills(t *testing.T) {
	path := filepath.Join(t.TempDir(), SyncFileName)
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	schemaV2 := strings.Replace(syncSchema, "  starred INTEGER NOT NULL DEFAULT 0,\n", "", 1)
	schemaV2 = strings.Replace(schemaV2, "PRAGMA user_version = 3", "PRAGMA user_version = 2", 1)
	if _, err := db.Exec(schemaV2); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO skills(sync_id, enabled, target_name, previous_target_names_json, tags_json, note, updated_at, provider, source_id, clone_url, subpath, ref)
VALUES('git:example.com/me/repo//skills/review', 0, 'review', '[]', '[]', '', '', 'git', 'example.com/me/repo', '', 'skills/review', '')`); err != nil {
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
	if document.Skills["git:example.com/me/repo//skills/review"].Starred {
		t.Fatal("expected migrated skill to default to unstarred")
	}
	verifyDB, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer verifyDB.Close()
	var version int
	if err := verifyDB.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != syncSchemaVersion {
		t.Fatalf("expected user_version %d, got %d", syncSchemaVersion, version)
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

func TestSyncStoreDeleteSkillRemovesAssociatedProfileAndPreservesOtherSharedState(t *testing.T) {
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
			deletedID: {SummaryZh: "应删除的简介。", UseCasesZh: []string{"应随简介一起删除。"}},
			keptID:    {SummaryZh: "应保留的简介。", UseCasesZh: []string{"用于验证其他共享状态不受影响。"}},
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
	if profile, exists := loaded.Profiles[deletedID]; exists {
		t.Fatalf("expected associated profile to be deleted, got %#v", profile)
	}
	if profile, exists := loaded.Profiles[keptID]; !exists || profile.SummaryZh != "应保留的简介。" || len(profile.UseCasesZh) != 1 {
		t.Fatalf("expected other profile to remain unchanged, got %#v", profile)
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

func TestSyncStoreMigratesLegacySchemaToJSONColumns(t *testing.T) {
	path := filepath.Join(t.TempDir(), SyncFileName)
	syncID := "git:example.com/me/repo//skills/review"
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(legacySyncSchemaV1); err != nil {
		db.Close()
		t.Fatal(err)
	}
	statements := []string{
		`INSERT INTO llm_config(id, base_url, api_key, model, temperature, max_tokens) VALUES(1, 'https://api.example.com', 'secret', 'model-v1', 0.2, 900)`,
		`INSERT INTO skills(sync_id, enabled, target_name, note, updated_at, provider, source_id, clone_url, subpath, ref) VALUES('` + syncID + `', 1, 'review', 'shared note', '2026-07-21T00:00:00Z', 'git', 'example.com/me/repo', 'https://example.com/me/repo.git', 'skills/review', 'main')`,
		`INSERT INTO skill_previous_target_names(sync_id, position, target_name) VALUES('` + syncID + `', 0, 'old-review')`,
		`INSERT INTO skill_tags(sync_id, tag) VALUES('` + syncID + `', 'alpha')`,
		`INSERT INTO skill_tags(sync_id, tag) VALUES('` + syncID + `', 'quality')`,
		`INSERT INTO profiles(sync_id, summary_zh, generated_at, model, source_hash, error) VALUES('` + syncID + `', '代码审阅助手。', '2026-07-20T00:00:00Z', 'model-v1', 'hash-v1', '')`,
		`INSERT INTO profile_use_cases(sync_id, position, use_case_zh) VALUES('` + syncID + `', 0, '检查回归风险。')`,
		`INSERT INTO profile_use_cases(sync_id, position, use_case_zh) VALUES('` + syncID + `', 1, '解释修改建议。')`,
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			db.Close()
			t.Fatal(err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	document, err := NewSyncStore(path).Load()
	if err != nil {
		t.Fatal(err)
	}
	record := document.Skills[syncID]
	if !record.Enabled || record.TargetName != "review" || record.Note != "shared note" ||
		len(record.Tags) != 2 || record.Tags[0] != "alpha" || record.Tags[1] != "quality" ||
		len(record.PreviousTargetNames) != 1 || record.PreviousTargetNames[0] != "old-review" {
		t.Fatalf("unexpected migrated skill: %#v", record)
	}
	profile := document.Profiles[syncID]
	if profile.SummaryZh != "代码审阅助手。" || len(profile.UseCasesZh) != 2 ||
		profile.UseCasesZh[0] != "检查回归风险。" || profile.UseCasesZh[1] != "解释修改建议。" {
		t.Fatalf("unexpected migrated profile: %#v", profile)
	}
	if document.LLM.BaseURL != "https://api.example.com" || document.LLM.APIKey != "secret" || document.LLM.Model != "model-v1" {
		t.Fatalf("unexpected migrated LLM config: %#v", document.LLM)
	}

	db, err = sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var version int
	if err := db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != syncSchemaVersion {
		t.Fatalf("expected user_version %d, got %d", syncSchemaVersion, version)
	}
	rows, err := db.Query(`SELECT name FROM sqlite_master WHERE type = 'table' ORDER BY name`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var tables []string
	for rows.Next() {
		var table string
		if err := rows.Scan(&table); err != nil {
			t.Fatal(err)
		}
		tables = append(tables, table)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	wantTables := []string{"llm_config", "profiles", "skills"}
	if len(tables) != len(wantTables) {
		t.Fatalf("expected compact schema tables %v, got %v", wantTables, tables)
	}
	for index := range wantTables {
		if tables[index] != wantTables[index] {
			t.Fatalf("expected compact schema tables %v, got %v", wantTables, tables)
		}
	}
	var profileParent, deleteAction string
	if err := db.QueryRow(`SELECT "table", on_delete FROM pragma_foreign_key_list('profiles') WHERE "from" = 'sync_id'`).Scan(&profileParent, &deleteAction); err != nil {
		t.Fatal(err)
	}
	if profileParent != "skills" || deleteAction != "CASCADE" {
		t.Fatalf("expected profiles to cascade from skills, got parent=%q delete=%q", profileParent, deleteAction)
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
	if _, err := db.Exec(`PRAGMA user_version = 999`); err != nil {
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

const legacySyncSchemaV1 = `
CREATE TABLE schema_meta (
  id INTEGER PRIMARY KEY CHECK (id = 1),
  version INTEGER NOT NULL
);
INSERT INTO schema_meta(id, version) VALUES(1, 1);
CREATE TABLE llm_config (
  id INTEGER PRIMARY KEY CHECK (id = 1),
  base_url TEXT NOT NULL,
  api_key TEXT NOT NULL,
  model TEXT NOT NULL,
  temperature REAL NOT NULL,
  max_tokens INTEGER NOT NULL
);
CREATE TABLE profiles (
  sync_id TEXT PRIMARY KEY,
  summary_zh TEXT NOT NULL,
  generated_at TEXT NOT NULL,
  model TEXT NOT NULL,
  source_hash TEXT NOT NULL,
  error TEXT NOT NULL
);
CREATE TABLE profile_use_cases (
  sync_id TEXT NOT NULL REFERENCES profiles(sync_id) ON DELETE CASCADE,
  position INTEGER NOT NULL,
  use_case_zh TEXT NOT NULL,
  PRIMARY KEY (sync_id, position)
);
CREATE TABLE skills (
  sync_id TEXT PRIMARY KEY,
  enabled INTEGER NOT NULL,
  target_name TEXT NOT NULL,
  note TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  provider TEXT NOT NULL,
  source_id TEXT NOT NULL,
  clone_url TEXT NOT NULL,
  subpath TEXT NOT NULL,
  ref TEXT NOT NULL
);
CREATE TABLE skill_previous_target_names (
  sync_id TEXT NOT NULL REFERENCES skills(sync_id) ON DELETE CASCADE,
  position INTEGER NOT NULL,
  target_name TEXT NOT NULL,
  PRIMARY KEY (sync_id, position),
  UNIQUE (sync_id, target_name)
);
CREATE TABLE skill_tags (
  sync_id TEXT NOT NULL REFERENCES skills(sync_id) ON DELETE CASCADE,
  tag TEXT NOT NULL,
  PRIMARY KEY (sync_id, tag)
);`
