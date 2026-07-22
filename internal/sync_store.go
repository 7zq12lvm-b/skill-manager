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

const SyncFileName = "skillManager.db"

const syncSchemaVersion = 2

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
	Note                string        `json:"note,omitempty"`
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

func (s *SyncStore) CheckIntegrity() error {
	db, err := s.open()
	if err != nil {
		return err
	}
	defer db.Close()
	var result string
	if err := db.QueryRow(`PRAGMA integrity_check`).Scan(&result); err != nil {
		return err
	}
	if result != "ok" {
		return errors.New("sync database integrity check failed")
	}
	rows, err := db.Query(`PRAGMA foreign_key_check`)
	if err != nil {
		return err
	}
	defer rows.Close()
	if rows.Next() {
		return errors.New("sync database foreign key check failed")
	}
	return rows.Err()
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
	if err := ensureSchema(tx); err != nil {
		return document, err
	}

	row := tx.QueryRow(`SELECT base_url, api_key, model, temperature, max_tokens FROM llm_config WHERE id = 1`)
	if err := row.Scan(&document.LLM.BaseURL, &document.LLM.APIKey, &document.LLM.Model, &document.LLM.Temperature, &document.LLM.MaxTokens); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return document, err
	}
	if err := loadProfilesTx(tx, &document); err != nil {
		return document, err
	}
	if err := loadSkillsTx(tx, &document); err != nil {
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

func loadProfilesTx(tx *sql.Tx, document *SyncDocument) error {
	return visitRowsTx(tx, `SELECT sync_id, summary_zh, use_cases_json, generated_at, model, source_hash, error FROM profiles`, func(rows *sql.Rows) error {
		var syncID, useCasesJSON string
		var profile SkillProfile
		if err := rows.Scan(&syncID, &profile.SummaryZh, &useCasesJSON, &profile.GeneratedAt, &profile.Model, &profile.SourceHash, &profile.Error); err != nil {
			return err
		}
		useCases, err := decodeStringList(useCasesJSON)
		if err != nil {
			return fmt.Errorf("decode profile use cases for %s: %w", syncID, err)
		}
		profile.UseCasesZh = useCases
		document.Profiles[syncID] = profile
		return nil
	})
}

func loadSkillsTx(tx *sql.Tx, document *SyncDocument) error {
	return visitRowsTx(tx, `SELECT sync_id, enabled, target_name, previous_target_names_json, tags_json, note, updated_at, provider, source_id, clone_url, subpath, ref FROM skills`, func(rows *sql.Rows) error {
		var syncID, previousTargetNamesJSON, tagsJSON string
		var enabled int
		var record SyncSkillRecord
		if err := rows.Scan(&syncID, &enabled, &record.TargetName, &previousTargetNamesJSON, &tagsJSON, &record.Note, &record.UpdatedAt, &record.Source.Provider, &record.Source.ID, &record.Source.Locator.CloneURL, &record.Source.Locator.Subpath, &record.Source.Locator.Ref); err != nil {
			return err
		}
		previousTargetNames, err := decodeStringList(previousTargetNamesJSON)
		if err != nil {
			return fmt.Errorf("decode previous target names for %s: %w", syncID, err)
		}
		tags, err := decodeStringList(tagsJSON)
		if err != nil {
			return fmt.Errorf("decode tags for %s: %w", syncID, err)
		}
		record.Enabled = enabled != 0
		record.PreviousTargetNames = previousTargetNames
		record.Tags = tags
		document.Skills[syncID] = record
		return nil
	})
}

func loadLegacyProfilesTx(tx *sql.Tx, document *SyncDocument) error {
	if err := visitRowsTx(tx, `SELECT sync_id, summary_zh, generated_at, model, source_hash, error FROM profiles`, func(rows *sql.Rows) error {
		var syncID string
		var profile SkillProfile
		if err := rows.Scan(&syncID, &profile.SummaryZh, &profile.GeneratedAt, &profile.Model, &profile.SourceHash, &profile.Error); err != nil {
			return err
		}
		document.Profiles[syncID] = profile
		return nil
	}); err != nil {
		return err
	}
	return visitRowsTx(tx, `SELECT sync_id, use_case_zh FROM profile_use_cases ORDER BY sync_id, position`, func(rows *sql.Rows) error {
		var syncID, useCase string
		if err := rows.Scan(&syncID, &useCase); err != nil {
			return err
		}
		profile := document.Profiles[syncID]
		profile.UseCasesZh = append(profile.UseCasesZh, useCase)
		document.Profiles[syncID] = profile
		return nil
	})
}

func loadLegacySkillsTx(tx *sql.Tx, document *SyncDocument) error {
	if err := visitRowsTx(tx, `SELECT sync_id, enabled, target_name, note, updated_at, provider, source_id, clone_url, subpath, ref FROM skills`, func(rows *sql.Rows) error {
		var syncID string
		var enabled int
		var record SyncSkillRecord
		if err := rows.Scan(&syncID, &enabled, &record.TargetName, &record.Note, &record.UpdatedAt, &record.Source.Provider, &record.Source.ID, &record.Source.Locator.CloneURL, &record.Source.Locator.Subpath, &record.Source.Locator.Ref); err != nil {
			return err
		}
		record.Enabled = enabled != 0
		document.Skills[syncID] = record
		return nil
	}); err != nil {
		return err
	}
	if err := visitRowsTx(tx, `SELECT sync_id, target_name FROM skill_previous_target_names ORDER BY sync_id, position`, func(rows *sql.Rows) error {
		var syncID, targetName string
		if err := rows.Scan(&syncID, &targetName); err != nil {
			return err
		}
		record := document.Skills[syncID]
		record.PreviousTargetNames = append(record.PreviousTargetNames, targetName)
		document.Skills[syncID] = record
		return nil
	}); err != nil {
		return err
	}
	return visitRowsTx(tx, `SELECT sync_id, tag FROM skill_tags ORDER BY sync_id, tag`, func(rows *sql.Rows) error {
		var syncID, tag string
		if err := rows.Scan(&syncID, &tag); err != nil {
			return err
		}
		record := document.Skills[syncID]
		record.Tags = append(record.Tags, tag)
		document.Skills[syncID] = record
		return nil
	})
}

func visitRowsTx(tx *sql.Tx, query string, visit func(*sql.Rows) error) error {
	rows, err := tx.Query(query)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		if err := visit(rows); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	return rows.Close()
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
		for _, record := range document.Skills {
			if err := upsertSkillTx(tx, record); err != nil {
				return err
			}
		}
		for syncID, profile := range document.Profiles {
			if _, exists := document.Skills[syncID]; !exists {
				continue
			}
			if err := upsertProfileTx(tx, syncID, profile); err != nil {
				return err
			}
		}
		for _, record := range document.Skills {
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
		_, err := tx.Exec(`DELETE FROM skills WHERE sync_id = ?`, strings.TrimSpace(syncID))
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
	if err := ensureSchema(tx); err != nil {
		return err
	}
	if err := operation(tx); err != nil {
		return err
	}
	return tx.Commit()
}

func emptySyncDocument() SyncDocument {
	return SyncDocument{
		Version:  2,
		Profiles: map[string]SkillProfile{},
		Skills:   map[string]SyncSkillRecord{},
	}
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
	db.SetMaxIdleConns(1)
	closeWithError := func(openErr error) (*sql.DB, error) {
		_ = db.Close()
		if isNew {
			_ = os.Remove(s.path)
		}
		return nil, openErr
	}
	if _, err := db.Exec(`PRAGMA busy_timeout=5000`); err != nil {
		return closeWithError(err)
	}
	if _, err := db.Exec(`PRAGMA foreign_keys=ON; PRAGMA synchronous=FULL;`); err != nil {
		return closeWithError(err)
	}
	if isNew {
		if err := initializeSyncDatabase(db); err != nil {
			return closeWithError(err)
		}
		if err := os.Chmod(s.path, 0o644); err != nil {
			return closeWithError(err)
		}
	} else if err := prepareSyncDatabase(db); err != nil {
		return closeWithError(err)
	}
	var journalMode string
	if err := db.QueryRow(`PRAGMA journal_mode=DELETE`).Scan(&journalMode); err != nil {
		return closeWithError(err)
	}
	if !strings.EqualFold(journalMode, "delete") {
		return closeWithError(fmt.Errorf("configure sync database journal mode: expected delete, got %s", journalMode))
	}
	return db, nil
}

type rowQueryer interface {
	QueryRow(query string, args ...any) *sql.Row
}

func ensureSchema(queryer rowQueryer) error {
	var version int
	if err := queryer.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		return fmt.Errorf("read sync database schema version: %w", err)
	}
	if version != syncSchemaVersion {
		return fmt.Errorf("unsupported sync database schema version %d; expected %d", version, syncSchemaVersion)
	}
	return nil
}

const syncSchema = `
CREATE TABLE llm_config (
  id INTEGER PRIMARY KEY CHECK (id = 1),
  base_url TEXT NOT NULL,
  api_key TEXT NOT NULL,
  model TEXT NOT NULL,
  temperature REAL NOT NULL,
  max_tokens INTEGER NOT NULL
);
CREATE TABLE skills (
  sync_id TEXT PRIMARY KEY,
  enabled INTEGER NOT NULL,
  target_name TEXT NOT NULL,
  previous_target_names_json TEXT NOT NULL DEFAULT '[]' CHECK(json_valid(previous_target_names_json) AND json_type(previous_target_names_json) = 'array'),
  tags_json TEXT NOT NULL DEFAULT '[]' CHECK(json_valid(tags_json) AND json_type(tags_json) = 'array'),
  note TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  provider TEXT NOT NULL,
  source_id TEXT NOT NULL,
  clone_url TEXT NOT NULL,
  subpath TEXT NOT NULL,
  ref TEXT NOT NULL,
  UNIQUE(provider, source_id, subpath)
);
CREATE TABLE profiles (
  sync_id TEXT PRIMARY KEY REFERENCES skills(sync_id) ON DELETE CASCADE,
  summary_zh TEXT NOT NULL,
  use_cases_json TEXT NOT NULL DEFAULT '[]' CHECK(json_valid(use_cases_json) AND json_type(use_cases_json) = 'array'),
  generated_at TEXT NOT NULL,
  model TEXT NOT NULL,
  source_hash TEXT NOT NULL,
  error TEXT NOT NULL
);
PRAGMA user_version = 2;`

func prepareSyncDatabase(db *sql.DB) error {
	version, legacy, err := inspectSyncSchemaVersion(db)
	if err != nil {
		return err
	}
	switch {
	case !legacy && version == syncSchemaVersion:
		return nil
	case legacy && version == 1:
		return migrateLegacySyncDatabase(db)
	default:
		return fmt.Errorf("unsupported sync database schema version %d; expected %d", version, syncSchemaVersion)
	}
}

func inspectSyncSchemaVersion(db *sql.DB) (version int, legacy bool, err error) {
	if err := db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		return 0, false, fmt.Errorf("read sync database schema version: %w", err)
	}
	if version != 0 {
		return version, false, nil
	}
	if err := db.QueryRow(`SELECT version FROM schema_meta WHERE id = 1`).Scan(&version); err != nil {
		return 0, false, fmt.Errorf("read legacy sync database schema version: %w", err)
	}
	return version, true, nil
}

func migrateLegacySyncDatabase(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	document := emptySyncDocument()
	row := tx.QueryRow(`SELECT base_url, api_key, model, temperature, max_tokens FROM llm_config WHERE id = 1`)
	if err := row.Scan(&document.LLM.BaseURL, &document.LLM.APIKey, &document.LLM.Model, &document.LLM.Temperature, &document.LLM.MaxTokens); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if err := loadLegacyProfilesTx(tx, &document); err != nil {
		return err
	}
	if err := loadLegacySkillsTx(tx, &document); err != nil {
		return err
	}

	if _, err := tx.Exec(`
DROP TABLE profile_use_cases;
DROP TABLE skill_previous_target_names;
DROP TABLE skill_tags;
DROP TABLE profiles;
DROP TABLE skills;
DROP TABLE llm_config;
DROP TABLE schema_meta;`); err != nil {
		return err
	}
	if _, err := tx.Exec(syncSchema); err != nil {
		return err
	}
	if err := saveLLMConfigTx(tx, document.LLM); err != nil {
		return err
	}
	for _, record := range document.Skills {
		if err := upsertSkillTx(tx, record); err != nil {
			return err
		}
	}
	for syncID, profile := range document.Profiles {
		if _, exists := document.Skills[syncID]; !exists {
			continue
		}
		if err := upsertProfileTx(tx, syncID, profile); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func initializeSyncDatabase(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(syncSchema); err != nil {
		return err
	}
	return tx.Commit()
}

func saveLLMConfigTx(tx *sql.Tx, config SyncLLMConfig) error {
	config = normalizeSyncLLMConfig(config)
	_, err := tx.Exec(`INSERT INTO llm_config(id, base_url, api_key, model, temperature, max_tokens) VALUES(1, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET base_url=excluded.base_url, api_key=excluded.api_key, model=excluded.model,
temperature=excluded.temperature, max_tokens=excluded.max_tokens`, config.BaseURL, config.APIKey, config.Model, config.Temperature, config.MaxTokens)
	return err
}

func upsertProfileTx(tx *sql.Tx, syncID string, profile SkillProfile) error {
	syncID = strings.TrimSpace(syncID)
	if syncID == "" {
		return errors.New("skill profile sync id is required")
	}
	profilePointer := normalizeSkillProfile(&profile)
	if profilePointer == nil {
		return errors.New("skill profile is empty")
	}
	profile = *profilePointer
	useCasesJSON, err := encodeStringList(profile.UseCasesZh)
	if err != nil {
		return err
	}
	_, err = tx.Exec(`INSERT INTO profiles(sync_id, summary_zh, use_cases_json, generated_at, model, source_hash, error) VALUES(?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(sync_id) DO UPDATE SET summary_zh=excluded.summary_zh, use_cases_json=excluded.use_cases_json,
generated_at=excluded.generated_at, model=excluded.model, source_hash=excluded.source_hash, error=excluded.error`,
		syncID, profile.SummaryZh, useCasesJSON, profile.GeneratedAt, profile.Model, profile.SourceHash, profile.Error)
	return err
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
	previousTargetNamesJSON, err := encodeStringList(record.PreviousTargetNames)
	if err != nil {
		return err
	}
	tagsJSON, err := encodeStringList(record.Tags)
	if err != nil {
		return err
	}
	_, err = tx.Exec(`INSERT INTO skills(sync_id, enabled, target_name, previous_target_names_json, tags_json, note, updated_at, provider, source_id, clone_url, subpath, ref)
VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(sync_id) DO UPDATE SET enabled=excluded.enabled, target_name=excluded.target_name,
previous_target_names_json=excluded.previous_target_names_json, tags_json=excluded.tags_json, note=excluded.note,
updated_at=excluded.updated_at, provider=excluded.provider, source_id=excluded.source_id, clone_url=excluded.clone_url,
subpath=excluded.subpath, ref=excluded.ref`, syncID, record.Enabled, record.TargetName, previousTargetNamesJSON, tagsJSON,
		record.Note, record.UpdatedAt, record.Source.Provider, record.Source.ID, record.Source.Locator.CloneURL,
		record.Source.Locator.Subpath, record.Source.Locator.Ref)
	return err
}

func encodeStringList(values []string) (string, error) {
	if values == nil {
		values = []string{}
	}
	data, err := json.Marshal(values)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func decodeStringList(value string) ([]string, error) {
	var values []string
	if err := json.Unmarshal([]byte(value), &values); err != nil {
		return nil, err
	}
	if values == nil {
		return []string{}, nil
	}
	return values, nil
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
	record.Note = strings.TrimSpace(record.Note)
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
