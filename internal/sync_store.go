package skillmgr

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const SyncFileName = "skill-manager-sync.json"

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

func (s *SyncStore) Load() (SyncDocument, error) {
	document := SyncDocument{
		Version: 2,
		Skills:  map[string]SyncSkillRecord{},
	}
	if s == nil || s.path == "" {
		return document, nil
	}
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return document, nil
	}
	if err != nil {
		return document, err
	}
	if err := json.Unmarshal(data, &document); err != nil {
		return document, err
	}
	if document.Version != 2 {
		return document, errors.New("unsupported sync document version; expected version 2")
	}
	return normalizeSyncDocument(document), nil
}

func (s *SyncStore) Save(document SyncDocument) error {
	if s == nil || s.path == "" {
		return errors.New("sync file is not configured")
	}
	document = normalizeSyncDocument(document)
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(s.path), ".skill-manager-sync-*.tmp")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if err := temp.Chmod(0o644); err != nil {
		temp.Close()
		return err
	}
	if _, err := temp.Write(data); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(tempPath, s.path)
}

func (s *SyncStore) UpsertSkill(record SyncSkillRecord) error {
	return s.UpsertSkills([]SyncSkillRecord{record})
}

func (s *SyncStore) UpsertSkills(records []SyncSkillRecord) error {
	document, err := s.Load()
	if err != nil {
		return err
	}
	updatedAt := time.Now().UTC().Format(time.RFC3339)
	for _, record := range records {
		record = normalizeSyncSkillRecord(record)
		id := syncRecordID(record)
		if id == "" {
			return errors.New("sync skill source is incomplete")
		}
		record.UpdatedAt = updatedAt
		document.Skills[id] = record
	}
	return s.Save(document)
}

func (s *SyncStore) DeleteSkill(syncID string) error {
	document, err := s.Load()
	if err != nil {
		return err
	}
	delete(document.Skills, syncID)
	return s.Save(document)
}

func (s *SyncStore) SaveLLMConfig(config SyncLLMConfig) error {
	document, err := s.Load()
	if err != nil {
		return err
	}
	document.LLM = normalizeSyncLLMConfig(config)
	return s.Save(document)
}

func (s *SyncStore) UpsertSkillProfile(syncID string, profile SkillProfile) error {
	syncID = strings.TrimSpace(syncID)
	if syncID == "" {
		return errors.New("skill profile sync id is required")
	}
	document, err := s.Load()
	if err != nil {
		return err
	}
	profilePointer := normalizeSkillProfile(&profile)
	if profilePointer == nil {
		return errors.New("skill profile is empty")
	}
	if document.Profiles == nil {
		document.Profiles = map[string]SkillProfile{}
	}
	document.Profiles[syncID] = *profilePointer
	if record, ok := document.Skills[syncID]; ok {
		record.Profile = profilePointer
		document.Skills[syncID] = record
	}
	return s.Save(document)
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
