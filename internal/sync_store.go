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
	Version int                        `json:"version"`
	Skills  map[string]SyncSkillRecord `json:"skills"`
}

type SyncSkillRecord struct {
	Enabled             bool       `json:"enabled"`
	TargetName          string     `json:"targetName"`
	PreviousTargetNames []string   `json:"previousTargetNames,omitempty"`
	Tags                []string   `json:"tags,omitempty"`
	UpdatedAt           string     `json:"updatedAt,omitempty"`
	Source              SyncSource `json:"source"`
}

type SyncSource struct {
	RepoID      string `json:"repoId"`
	CloneURL    string `json:"cloneUrl,omitempty"`
	RepoSubpath string `json:"repoSubpath"`
	Ref         string `json:"ref,omitempty"`
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
		Version: 1,
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
	return os.WriteFile(s.path, data, 0o644)
}

func (s *SyncStore) UpsertSkill(record SyncSkillRecord) error {
	document, err := s.Load()
	if err != nil {
		return err
	}
	record = normalizeSyncSkillRecord(record)
	id := syncSkillID(record.Source.RepoID, record.Source.RepoSubpath)
	if id == "" {
		return errors.New("sync skill source is incomplete")
	}
	record.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	document.Skills[id] = record
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

func normalizeSyncDocument(document SyncDocument) SyncDocument {
	document.Version = 1
	if document.Skills == nil {
		document.Skills = map[string]SyncSkillRecord{}
	}
	for id, record := range document.Skills {
		record = normalizeSyncSkillRecord(record)
		cleanID := syncSkillID(record.Source.RepoID, record.Source.RepoSubpath)
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
	record.Source.RepoID = strings.Trim(strings.TrimSpace(record.Source.RepoID), "/")
	record.Source.RepoSubpath = cleanRepoSubpath(record.Source.RepoSubpath)
	record.Source.CloneURL = strings.TrimSpace(record.Source.CloneURL)
	record.Source.Ref = strings.TrimSpace(record.Source.Ref)
	record.TargetName = strings.TrimSpace(record.TargetName)
	if record.TargetName == "" {
		record.TargetName = filepath.Base(record.Source.RepoSubpath)
	}
	record.PreviousTargetNames = cleanNameList(record.PreviousTargetNames)
	record.Tags = cleanSkillTags(record.Tags)
	return record
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
