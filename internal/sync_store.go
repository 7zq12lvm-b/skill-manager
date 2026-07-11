package skillmgr

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

const (
	SyncFileName           = "skill-manager-sync.db"
	LegacySyncFileName     = "skill-manager-sync.json"
	LegacySyncBackupSuffix = ".migrated.bak"
)

type SyncStore struct {
	path string
}

type SyncDocument struct {
	Version  int                        `json:"version"`
	LLM      SyncLLMConfig              `json:"llm,omitempty"`
	Profiles map[string]SkillProfile    `json:"profiles,omitempty"`
	Skills   map[string]SyncSkillRecord `json:"skills"`
}

type SyncLLMConfig struct {
	BaseURL     string  `json:"baseUrl,omitempty"`
	APIKey      string  `json:"apiKey,omitempty"`
	Model       string  `json:"model,omitempty"`
	Temperature float64 `json:"temperature,omitempty"`
	MaxTokens   int     `json:"maxTokens,omitempty"`
}

type SyncSkillRecord struct {
	Enabled             bool          `json:"enabled"`
	TargetName          string        `json:"targetName"`
	PreviousTargetNames []string      `json:"previousTargetNames,omitempty"`
	Tags                []string      `json:"tags,omitempty"`
	Profile             *SkillProfile `json:"profile,omitempty"`
	UpdatedAt           string        `json:"updatedAt,omitempty"`
	Source              SyncSource    `json:"source"`
}

type SyncSource struct {
	Provider string        `json:"provider"`
	ID       string        `json:"id"`
	Locator  SourceLocator `json:"locator"`
}

type SourceLocator struct {
	CloneURL string `json:"cloneUrl,omitempty"`
	Subpath  string `json:"subpath"`
	Ref      string `json:"ref,omitempty"`
}

func NewSyncStore(path string) *SyncStore {
	return &SyncStore{path: path}
}

func SyncPathFromFolder(folder string) string {
	folder = strings.TrimSpace(expandHome(folder))
	if folder == "" {
		return ""
	}
	return filepath.Join(folder, SyncFileName)
}

func (s *SyncStore) Path() string {
	return s.path
}

func (s *SyncStore) Load() (SyncDocument, error) {
	document := emptySyncDocument()
	if s == nil || s.path == "" {
		return document, nil
	}
	db, err := s.open()
	if err != nil {
		return document, err
	}
	defer db.Close()
	tx, err := db.Begin()
	if err != nil {
		return document, err
	}
	defer tx.Rollback()
	if err := ensureSchemaTx(tx); err != nil {
		return document, err
	}

	row := tx.QueryRow(`SELECT base_url, api_key, model, temperature, max_tokens FROM llm_config WHERE id = 1`)
	if err := row.Scan(&document.LLM.BaseURL, &document.LLM.APIKey, &document.LLM.Model, &document.LLM.Temperature, &document.LLM.MaxTokens); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return document, err
	}
	profileRows, err := tx.Query(`SELECT sync_id, summary_zh, generated_at, model, source_hash, error FROM profiles`)
	if err != nil {
		return document, err
	}
	for profileRows.Next() {
		var syncID string
		var profile SkillProfile
		if err := profileRows.Scan(&syncID, &profile.SummaryZh, &profile.GeneratedAt, &profile.Model, &profile.SourceHash, &profile.Error); err != nil {
			profileRows.Close()
			return document, err
		}
		document.Profiles[syncID] = profile
	}
	if err := profileRows.Err(); err != nil {
		profileRows.Close()
		return document, err
	}
	if err := profileRows.Close(); err != nil {
		return document, err
	}
	useCaseRows, err := tx.Query(`SELECT sync_id, use_case_zh FROM profile_use_cases ORDER BY sync_id, position`)
	if err != nil {
		return document, err
	}
	for useCaseRows.Next() {
		var syncID, useCase string
		if err := useCaseRows.Scan(&syncID, &useCase); err != nil {
			useCaseRows.Close()
			return document, err
		}
		profile := document.Profiles[syncID]
		profile.UseCasesZh = append(profile.UseCasesZh, useCase)
		document.Profiles[syncID] = profile
	}
	if err := useCaseRows.Err(); err != nil {
		useCaseRows.Close()
		return document, err
	}
	if err := useCaseRows.Close(); err != nil {
		return document, err
	}

	skillRows, err := tx.Query(`SELECT sync_id, enabled, target_name, updated_at, provider, source_id, clone_url, subpath, ref FROM skills`)
	if err != nil {
		return document, err
	}
	defer skillRows.Close()
	for skillRows.Next() {
		var syncID string
		var enabled int
		var record SyncSkillRecord
		if err := skillRows.Scan(&syncID, &enabled, &record.TargetName, &record.UpdatedAt, &record.Source.Provider, &record.Source.ID, &record.Source.Locator.CloneURL, &record.Source.Locator.Subpath, &record.Source.Locator.Ref); err != nil {
			return document, err
		}
		record.Enabled = enabled != 0
		document.Skills[syncID] = record
	}
	if err := skillRows.Err(); err != nil {
		return document, err
	}
	if err := skillRows.Close(); err != nil {
		return document, err
	}
	nameRows, err := tx.Query(`SELECT sync_id, target_name FROM skill_previous_target_names ORDER BY sync_id, position`)
	if err != nil {
		return document, err
	}
	for nameRows.Next() {
		var syncID, targetName string
		if err := nameRows.Scan(&syncID, &targetName); err != nil {
			nameRows.Close()
			return document, err
		}
		record := document.Skills[syncID]
		record.PreviousTargetNames = append(record.PreviousTargetNames, targetName)
		document.Skills[syncID] = record
	}
	if err := nameRows.Err(); err != nil {
		nameRows.Close()
		return document, err
	}
	if err := nameRows.Close(); err != nil {
		return document, err
	}
	tagRows, err := tx.Query(`SELECT sync_id, tag FROM skill_tags ORDER BY sync_id, tag`)
	if err != nil {
		return document, err
	}
	for tagRows.Next() {
		var syncID, tag string
		if err := tagRows.Scan(&syncID, &tag); err != nil {
			tagRows.Close()
			return document, err
		}
		record := document.Skills[syncID]
		record.Tags = append(record.Tags, tag)
		document.Skills[syncID] = record
	}
	if err := tagRows.Err(); err != nil {
		tagRows.Close()
		return document, err
	}
	if err := tagRows.Close(); err != nil {
		return document, err
	}
	for syncID, profile := range document.Profiles {
		if record, ok := document.Skills[syncID]; ok {
			profileCopy := profile
			record.Profile = &profileCopy
			document.Skills[syncID] = record
		}
	}
	if err := tx.Commit(); err != nil {
		return document, err
	}
	return normalizeSyncDocument(document), nil
}

func (s *SyncStore) Save(document SyncDocument) error {
	if s == nil || s.path == "" {
		return errors.New("sync database is not configured")
	}
	document = normalizeSyncDocument(document)
	db, err := s.open()
	if err != nil {
		return err
	}
	defer db.Close()
	return withTransaction(db, func(tx *sql.Tx) error {
		if _, err := tx.Exec(`DELETE FROM skills`); err != nil {
			return err
		}
		if _, err := tx.Exec(`DELETE FROM profiles`); err != nil {
			return err
		}
		if err := saveLLMConfigTx(tx, document.LLM); err != nil {
			return err
		}
		for syncID, profile := range document.Profiles {
			if err := upsertProfileTx(tx, syncID, profile); err != nil {
				return err
			}
		}
		for _, record := range document.Skills {
			if err := upsertSkillTx(tx, record); err != nil {
				return err
			}
			syncID := syncRecordID(record)
			if _, exists := document.Profiles[syncID]; !exists && record.Profile != nil {
				if err := upsertProfileTx(tx, syncID, *record.Profile); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

func (s *SyncStore) UpsertSkill(record SyncSkillRecord) error {
	return s.UpsertSkills([]SyncSkillRecord{record})
}

func (s *SyncStore) UpsertSkills(records []SyncSkillRecord) error {
	if s == nil || s.path == "" {
		return errors.New("sync database is not configured")
	}
	db, err := s.open()
	if err != nil {
		return err
	}
	defer db.Close()
	updatedAt := time.Now().UTC().Format(time.RFC3339)
	return withTransaction(db, func(tx *sql.Tx) error {
		for _, record := range records {
			record = normalizeSyncSkillRecord(record)
			if syncRecordID(record) == "" {
				return errors.New("sync skill source is incomplete")
			}
			record.UpdatedAt = updatedAt
			if err := upsertSkillTx(tx, record); err != nil {
				return err
			}
			if record.Profile != nil {
				if err := insertProfileIfMissingTx(tx, syncRecordID(record), *record.Profile); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

func (s *SyncStore) DeleteSkill(syncID string) error {
	db, err := s.open()
	if err != nil {
		return err
	}
	defer db.Close()
	return withTransaction(db, func(tx *sql.Tx) error {
		_, err := tx.Exec(`DELETE FROM skills WHERE sync_id = ?`, syncID)
		return err
	})
}

func (s *SyncStore) SaveLLMConfig(config SyncLLMConfig) error {
	db, err := s.open()
	if err != nil {
		return err
	}
	defer db.Close()
	return withTransaction(db, func(tx *sql.Tx) error {
		return saveLLMConfigTx(tx, config)
	})
}

func (s *SyncStore) UpsertSkillProfile(syncID string, profile SkillProfile) error {
	syncID = strings.TrimSpace(syncID)
	if syncID == "" {
		return errors.New("skill profile sync id is required")
	}
	db, err := s.open()
	if err != nil {
		return err
	}
	defer db.Close()
	profilePointer := normalizeSkillProfile(&profile)
	if profilePointer == nil {
		return errors.New("skill profile is empty")
	}
	return withTransaction(db, func(tx *sql.Tx) error {
		return upsertProfileTx(tx, syncID, *profilePointer)
	})
}

func withTransaction(db *sql.DB, operation func(*sql.Tx) error) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := ensureSchemaTx(tx); err != nil {
		return err
	}
	if err := operation(tx); err != nil {
		return err
	}
	return tx.Commit()
}

func emptySyncDocument() SyncDocument {
	return SyncDocument{Version: 2, Profiles: map[string]SkillProfile{}, Skills: map[string]SyncSkillRecord{}}
}

func (s *SyncStore) open() (*sql.DB, error) {
	if s == nil || strings.TrimSpace(s.path) == "" {
		return nil, errors.New("sync database is not configured")
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return nil, err
	}
	_, statErr := os.Stat(s.path)
	isNew := errors.Is(statErr, os.ErrNotExist)
	if statErr != nil && !isNew {
		return nil, statErr
	}
	db, err := sql.Open("sqlite", s.path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`PRAGMA foreign_keys=ON; PRAGMA synchronous=FULL; PRAGMA busy_timeout=5000;`); err != nil {
		db.Close()
		return nil, err
	}
	if isNew {
		if err := s.initialize(db); err != nil {
			db.Close()
			_ = os.Remove(s.path)
			return nil, err
		}
	}
	return db, nil
}

func ensureSchemaTx(tx *sql.Tx) error {
	var version int
	if err := tx.QueryRow(`SELECT version FROM schema_meta LIMIT 1`).Scan(&version); err != nil {
		return fmt.Errorf("read sync database schema version: %w", err)
	}
	switch version {
	case 2:
		return nil
	case 1:
		return migrateV1DatabaseTx(tx)
	default:
		return fmt.Errorf("unsupported sync database schema version %d", version)
	}
}

func migrateV1DatabaseTx(tx *sql.Tx) error {
	document, err := loadV1DocumentTx(tx)
	if err != nil {
		return fmt.Errorf("migrate sync database schema v1: %w", err)
	}
	for _, table := range []string{"skills", "profiles", "schema_meta"} {
		if _, err := tx.Exec(`DROP TABLE ` + table); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(syncSchema); err != nil {
		return err
	}
	for syncID, profile := range document.Profiles {
		if err := upsertProfileTx(tx, syncID, profile); err != nil {
			return err
		}
	}
	for _, record := range document.Skills {
		if err := upsertSkillTx(tx, record); err != nil {
			return err
		}
		syncID := syncRecordID(record)
		if _, exists := document.Profiles[syncID]; !exists && record.Profile != nil {
			if err := upsertProfileTx(tx, syncID, *record.Profile); err != nil {
				return err
			}
		}
	}
	return nil
}

func loadV1DocumentTx(tx *sql.Tx) (SyncDocument, error) {
	document := emptySyncDocument()
	rows, err := tx.Query(`SELECT sync_id, profile_json FROM profiles`)
	if err != nil {
		return document, err
	}
	for rows.Next() {
		var syncID, raw string
		if err := rows.Scan(&syncID, &raw); err != nil {
			rows.Close()
			return document, err
		}
		var profile SkillProfile
		if err := json.Unmarshal([]byte(raw), &profile); err != nil {
			rows.Close()
			return document, err
		}
		document.Profiles[syncID] = profile
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return document, err
	}
	if err := rows.Close(); err != nil {
		return document, err
	}
	rows, err = tx.Query(`SELECT sync_id, enabled, target_name, previous_target_names_json, tags_json, profile_json, updated_at, provider, source_id, clone_url, subpath, ref FROM skills`)
	if err != nil {
		return document, err
	}
	defer rows.Close()
	for rows.Next() {
		var syncID, previousNames, tags string
		var profileJSON sql.NullString
		var enabled int
		var record SyncSkillRecord
		if err := rows.Scan(&syncID, &enabled, &record.TargetName, &previousNames, &tags, &profileJSON, &record.UpdatedAt, &record.Source.Provider, &record.Source.ID, &record.Source.Locator.CloneURL, &record.Source.Locator.Subpath, &record.Source.Locator.Ref); err != nil {
			return document, err
		}
		record.Enabled = enabled != 0
		if err := json.Unmarshal([]byte(previousNames), &record.PreviousTargetNames); err != nil {
			return document, err
		}
		if err := json.Unmarshal([]byte(tags), &record.Tags); err != nil {
			return document, err
		}
		if profileJSON.Valid {
			var profile SkillProfile
			if err := json.Unmarshal([]byte(profileJSON.String), &profile); err != nil {
				return document, err
			}
			record.Profile = &profile
		}
		document.Skills[syncID] = record
	}
	return document, rows.Err()
}

const syncSchema = `
CREATE TABLE IF NOT EXISTS schema_meta (version INTEGER NOT NULL);
INSERT INTO schema_meta(version) SELECT 2 WHERE NOT EXISTS (SELECT 1 FROM schema_meta);
CREATE TABLE IF NOT EXISTS llm_config (
  id INTEGER PRIMARY KEY CHECK (id = 1), base_url TEXT NOT NULL, api_key TEXT NOT NULL,
  model TEXT NOT NULL, temperature REAL NOT NULL, max_tokens INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS profiles (
  sync_id TEXT PRIMARY KEY, summary_zh TEXT NOT NULL, generated_at TEXT NOT NULL,
  model TEXT NOT NULL, source_hash TEXT NOT NULL, error TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS profile_use_cases (
  sync_id TEXT NOT NULL REFERENCES profiles(sync_id) ON DELETE CASCADE,
  position INTEGER NOT NULL, use_case_zh TEXT NOT NULL,
  PRIMARY KEY (sync_id, position)
);
CREATE TABLE IF NOT EXISTS skills (
  sync_id TEXT PRIMARY KEY, enabled INTEGER NOT NULL, target_name TEXT NOT NULL,
  updated_at TEXT NOT NULL, provider TEXT NOT NULL, source_id TEXT NOT NULL,
  clone_url TEXT NOT NULL, subpath TEXT NOT NULL, ref TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS skill_previous_target_names (
  sync_id TEXT NOT NULL REFERENCES skills(sync_id) ON DELETE CASCADE,
  position INTEGER NOT NULL, target_name TEXT NOT NULL,
  PRIMARY KEY (sync_id, position), UNIQUE (sync_id, target_name)
);
CREATE TABLE IF NOT EXISTS skill_tags (
  sync_id TEXT NOT NULL REFERENCES skills(sync_id) ON DELETE CASCADE,
  tag TEXT NOT NULL, PRIMARY KEY (sync_id, tag)
);`

func (s *SyncStore) initialize(db *sql.DB) error {
	if _, err := db.Exec(`PRAGMA journal_mode=DELETE`); err != nil {
		return err
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(syncSchema); err != nil {
		return err
	}

	legacyPath := filepath.Join(filepath.Dir(s.path), LegacySyncFileName)
	data, err := os.ReadFile(legacyPath)
	if errors.Is(err, os.ErrNotExist) {
		return tx.Commit()
	}
	if err != nil {
		return err
	}
	document := emptySyncDocument()
	if err := json.Unmarshal(data, &document); err != nil {
		return fmt.Errorf("migrate legacy sync JSON: %w", err)
	}
	if document.Version != 2 {
		return errors.New("cannot migrate legacy sync JSON: expected version 2")
	}
	document = normalizeSyncDocument(document)
	if err := saveLLMConfigTx(tx, document.LLM); err != nil {
		return err
	}
	for syncID, profile := range document.Profiles {
		if err := upsertProfileTx(tx, syncID, profile); err != nil {
			return err
		}
	}
	for _, record := range document.Skills {
		if err := upsertSkillTx(tx, record); err != nil {
			return err
		}
		syncID := syncRecordID(record)
		if _, exists := document.Profiles[syncID]; !exists && record.Profile != nil {
			if err := upsertProfileTx(tx, syncID, *record.Profile); err != nil {
				return err
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	backupPath := legacyPath + LegacySyncBackupSuffix
	if _, err := os.Stat(backupPath); errors.Is(err, os.ErrNotExist) {
		if err := os.Rename(legacyPath, backupPath); err != nil {
			return fmt.Errorf("archive migrated sync JSON: %w", err)
		}
	}
	return nil
}

func saveLLMConfigTx(tx *sql.Tx, config SyncLLMConfig) error {
	config = normalizeSyncLLMConfig(config)
	_, err := tx.Exec(`INSERT INTO llm_config(id, base_url, api_key, model, temperature, max_tokens) VALUES(1, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET base_url=excluded.base_url, api_key=excluded.api_key, model=excluded.model, temperature=excluded.temperature, max_tokens=excluded.max_tokens`,
		config.BaseURL, config.APIKey, config.Model, config.Temperature, config.MaxTokens)
	return err
}

func upsertProfileTx(tx *sql.Tx, syncID string, profile SkillProfile) error {
	profilePointer := normalizeSkillProfile(&profile)
	if profilePointer == nil {
		return errors.New("skill profile is empty")
	}
	profile = *profilePointer
	if _, err := tx.Exec(`INSERT INTO profiles(sync_id, summary_zh, generated_at, model, source_hash, error) VALUES(?, ?, ?, ?, ?, ?)
ON CONFLICT(sync_id) DO UPDATE SET summary_zh=excluded.summary_zh, generated_at=excluded.generated_at, model=excluded.model, source_hash=excluded.source_hash, error=excluded.error`,
		syncID, profile.SummaryZh, profile.GeneratedAt, profile.Model, profile.SourceHash, profile.Error); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM profile_use_cases WHERE sync_id = ?`, syncID); err != nil {
		return err
	}
	for position, useCase := range profile.UseCasesZh {
		if _, err := tx.Exec(`INSERT INTO profile_use_cases(sync_id, position, use_case_zh) VALUES(?, ?, ?)`, syncID, position, useCase); err != nil {
			return err
		}
	}
	return nil
}

func insertProfileIfMissingTx(tx *sql.Tx, syncID string, profile SkillProfile) error {
	var exists int
	err := tx.QueryRow(`SELECT 1 FROM profiles WHERE sync_id = ?`, syncID).Scan(&exists)
	if err == nil {
		return nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	return upsertProfileTx(tx, syncID, profile)
}

func upsertSkillTx(tx *sql.Tx, record SyncSkillRecord) error {
	record = normalizeSyncSkillRecord(record)
	syncID := syncRecordID(record)
	if syncID == "" {
		return errors.New("sync skill source is incomplete")
	}
	_, err := tx.Exec(`INSERT INTO skills(sync_id, enabled, target_name, updated_at, provider, source_id, clone_url, subpath, ref)
VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(sync_id) DO UPDATE SET enabled=excluded.enabled, target_name=excluded.target_name, updated_at=excluded.updated_at,
provider=excluded.provider, source_id=excluded.source_id, clone_url=excluded.clone_url, subpath=excluded.subpath, ref=excluded.ref`, syncID, record.Enabled, record.TargetName,
		record.UpdatedAt, record.Source.Provider, record.Source.ID, record.Source.Locator.CloneURL, record.Source.Locator.Subpath, record.Source.Locator.Ref)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM skill_previous_target_names WHERE sync_id = ?`, syncID); err != nil {
		return err
	}
	for position, targetName := range record.PreviousTargetNames {
		if _, err := tx.Exec(`INSERT INTO skill_previous_target_names(sync_id, position, target_name) VALUES(?, ?, ?)`, syncID, position, targetName); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`DELETE FROM skill_tags WHERE sync_id = ?`, syncID); err != nil {
		return err
	}
	for _, tag := range record.Tags {
		if _, err := tx.Exec(`INSERT INTO skill_tags(sync_id, tag) VALUES(?, ?)`, syncID, tag); err != nil {
			return err
		}
	}
	return nil
}

func normalizeSyncDocument(document SyncDocument) SyncDocument {
	document.Version = 2
	if document.Skills == nil {
		document.Skills = map[string]SyncSkillRecord{}
	}
	document.LLM = normalizeSyncLLMConfig(document.LLM)
	if document.Profiles == nil {
		document.Profiles = map[string]SkillProfile{}
	}
	for id, profile := range document.Profiles {
		normalized := normalizeSkillProfile(&profile)
		if normalized == nil {
			delete(document.Profiles, id)
			continue
		}
		document.Profiles[id] = *normalized
	}
	for id, record := range document.Skills {
		record = normalizeSyncSkillRecord(record)
		cleanID := syncRecordID(record)
		if cleanID == "" {
			delete(document.Skills, id)
			continue
		}
		if cleanID != id {
			delete(document.Skills, id)
		}
		document.Skills[cleanID] = record
	}
	return document
}

func normalizeSyncSkillRecord(record SyncSkillRecord) SyncSkillRecord {
	record.Source.Provider = strings.TrimSpace(record.Source.Provider)
	record.Source.ID = strings.Trim(strings.TrimSpace(record.Source.ID), "/")
	record.Source.Locator.Subpath = cleanRepoSubpath(record.Source.Locator.Subpath)
	record.Source.Locator.CloneURL = strings.TrimSpace(record.Source.Locator.CloneURL)
	record.Source.Locator.Ref = strings.TrimSpace(record.Source.Locator.Ref)
	record.TargetName = strings.TrimSpace(record.TargetName)
	if record.TargetName == "" {
		record.TargetName = filepath.Base(record.Source.Locator.Subpath)
	}
	record.PreviousTargetNames = cleanNameList(record.PreviousTargetNames)
	record.Tags = cleanSkillTags(record.Tags)
	record.Profile = normalizeSkillProfile(record.Profile)
	return record
}

func syncRecordID(record SyncSkillRecord) string {
	if record.Source.Provider != "git" {
		return ""
	}
	return syncSkillID(record.Source.ID, record.Source.Locator.Subpath)
}

func normalizeSyncLLMConfig(config SyncLLMConfig) SyncLLMConfig {
	config.BaseURL = strings.TrimSpace(config.BaseURL)
	config.APIKey = strings.TrimSpace(config.APIKey)
	config.Model = strings.TrimSpace(config.Model)
	if config.Temperature < 0 {
		config.Temperature = 0
	}
	if config.MaxTokens < 0 {
		config.MaxTokens = 0
	}
	return config
}

func normalizeSkillProfile(profile *SkillProfile) *SkillProfile {
	if profile == nil {
		return nil
	}
	profile.SummaryZh = strings.TrimSpace(profile.SummaryZh)
	profile.UseCasesZh = cleanProfileUseCases(profile.UseCasesZh)
	profile.GeneratedAt = strings.TrimSpace(profile.GeneratedAt)
	profile.Model = strings.TrimSpace(profile.Model)
	profile.SourceHash = strings.TrimSpace(profile.SourceHash)
	profile.Error = strings.TrimSpace(profile.Error)
	if profile.SummaryZh == "" && len(profile.UseCasesZh) == 0 && profile.Error == "" {
		return nil
	}
	return profile
}

func cleanProfileUseCases(values []string) []string {
	cleaned := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		cleaned = append(cleaned, value)
	}
	return cleaned
}

func cleanNameList(values []string) []string {
	seen := map[string]bool{}
	cleaned := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		cleaned = append(cleaned, value)
	}
	sort.Strings(cleaned)
	return cleaned
}

func cleanSkillTags(tags []string) []string {
	seen := map[string]bool{}
	cleaned := make([]string, 0, len(tags))
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if tag == "" || seen[tag] {
			continue
		}
		seen[tag] = true
		cleaned = append(cleaned, tag)
	}
	sort.Strings(cleaned)
	return cleaned
}
