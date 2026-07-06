package skillmgr

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

type SkillTagStore struct {
	path string
}

type SkillTagDocument struct {
	Version int                 `json:"version"`
	Skills  map[string][]string `json:"skills"`
}

func NewSkillTagStore(path string) *SkillTagStore {
	return &SkillTagStore{path: path}
}

func DefaultSkillTagPath() (string, error) {
	if runtime.GOOS == "darwin" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(homeDir, "Library", "Mobile Documents", "com~apple~CloudDocs", "SkillManager", "tags.json"), nil
	}
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, "skill-manager", "tags.json"), nil
}

func (s *SkillTagStore) Load() (SkillTagDocument, error) {
	document := SkillTagDocument{
		Version: 1,
		Skills:  map[string][]string{},
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
	return normalizeSkillTagDocument(document), nil
}

func (s *SkillTagStore) Save(document SkillTagDocument) error {
	document = normalizeSkillTagDocument(document)
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0o644)
}

func (s *SkillTagStore) Remove() error {
	if s == nil || s.path == "" {
		return nil
	}
	if err := os.Remove(s.path); errors.Is(err, os.ErrNotExist) {
		return nil
	} else {
		return err
	}
}

func (s *SkillTagStore) SetSkillTags(skillName string, tags []string) error {
	document, err := s.Load()
	if err != nil {
		return err
	}
	skillName = strings.TrimSpace(skillName)
	if skillName == "" {
		return errors.New("skill name is required")
	}
	tags = cleanSkillTags(tags)
	if len(tags) == 0 {
		delete(document.Skills, skillName)
	} else {
		document.Skills[skillName] = tags
	}
	return s.Save(document)
}

func normalizeSkillTagDocument(document SkillTagDocument) SkillTagDocument {
	document.Version = 1
	if document.Skills == nil {
		document.Skills = map[string][]string{}
	}
	for skillName, tags := range document.Skills {
		cleanName := strings.TrimSpace(skillName)
		cleanTags := cleanSkillTags(tags)
		if cleanName == "" || len(cleanTags) == 0 {
			delete(document.Skills, skillName)
			continue
		}
		if cleanName != skillName {
			delete(document.Skills, skillName)
		}
		document.Skills[cleanName] = cleanTags
	}
	return document
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
