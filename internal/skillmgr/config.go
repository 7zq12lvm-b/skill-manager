package skillmgr

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
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
