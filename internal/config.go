package skillmgr

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

type ConfigStore struct {
	path string
}

func NewConfigStore(path string) *ConfigStore {
	return &ConfigStore{path: path}
}

func DefaultConfigPath() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, "skill-manager", "config.json"), nil
}

func (s *ConfigStore) Load() (Config, error) {
	config := DefaultConfig()
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return config, nil
	}
	if err != nil {
		return config, err
	}
	if err := json.Unmarshal(data, &config); err != nil {
		return config, err
	}
	return normalizeConfig(config), nil
}

func (s *ConfigStore) Save(config Config) error {
	config = normalizeConfig(config)
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0o644)
}

func normalizeConfig(config Config) Config {
	if len(config.TargetDirs) == 0 {
		config.TargetDirs = append([]string(nil), DefaultConfig().TargetDirs...)
	}
	config.TargetDirs = cleanTargetDirs(config.TargetDirs)
	config.Validation.Mode = ValidationStrict
	config.Validation.RequiredFiles = []string{"SKILL.md"}
	config.Validation.ShowInvalid = false
	if config.ConflictHandling == "" {
		config.ConflictHandling = "ask"
	}
	for i := range config.Sources {
		config.Sources[i].Path = expandHome(config.Sources[i].Path)
		if config.Sources[i].ID == "" {
			config.Sources[i].ID = sourceID(config.Sources[i].Path)
		}
	}
	config.Sync.Folder = expandHome(strings.TrimSpace(config.Sync.Folder))
	for i := range config.Repositories {
		config.Repositories[i].Path = filepath.Clean(expandHome(config.Repositories[i].Path))
		config.Repositories[i].RepoID = strings.Trim(strings.TrimSpace(config.Repositories[i].RepoID), "/")
		if config.Repositories[i].ID == "" {
			config.Repositories[i].ID = config.Repositories[i].RepoID
		}
		if config.Repositories[i].RepoID == "" {
			config.Repositories[i].RepoID = config.Repositories[i].ID
		}
		if len(config.Repositories[i].ScanRoots) == 0 {
			config.Repositories[i].ScanRoots = []string{"."}
		}
		config.Repositories[i].ScanRoots = cleanRelativePaths(config.Repositories[i].ScanRoots, []string{"."})
		config.Repositories[i].IgnorePaths = cleanRelativePaths(config.Repositories[i].IgnorePaths, nil)
	}
	return config
}

func cleanTargetDirs(targetDirs []string) []string {
	defaultTargetDirs := DefaultConfig().TargetDirs
	seen := map[string]bool{}
	cleaned := make([]string, 0, len(targetDirs))
	for _, targetDir := range targetDirs {
		targetDir = filepath.Clean(expandHome(targetDir))
		if targetDir == "." || targetDir == "" || seen[targetDir] {
			continue
		}
		seen[targetDir] = true
		cleaned = append(cleaned, targetDir)
	}
	if len(cleaned) == 0 {
		return append([]string(nil), defaultTargetDirs...)
	}
	return cleaned
}

func cleanRelativePaths(paths []string, fallback []string) []string {
	seen := map[string]bool{}
	cleaned := make([]string, 0, len(paths))
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		path = filepath.Clean(path)
		if filepath.IsAbs(path) {
			continue
		}
		if path == string(filepath.Separator) {
			path = "."
		}
		path = filepath.ToSlash(path)
		if path == "" || seen[path] {
			continue
		}
		seen[path] = true
		cleaned = append(cleaned, path)
	}
	if len(cleaned) == 0 && fallback != nil {
		return append([]string(nil), fallback...)
	}
	return cleaned
}
