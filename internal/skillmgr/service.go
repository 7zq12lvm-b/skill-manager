package skillmgr

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Service struct{}

func NewService() *Service {
	return &Service{}
}

func (s *Service) Scan(_ context.Context, config Config) (Inventory, error) {
	config = normalizeConfig(config)
	scannedAt := time.Now().Format(time.RFC3339)
	sources := make([]SkillSource, 0, len(config.Sources))
	skills := make([]Skill, 0)

	for _, sourceConfig := range config.Sources {
		source := SkillSource{
			ID:            sourceConfig.ID,
			Path:          sourceConfig.Path,
			Alias:         sourceConfig.Alias,
			Enabled:       sourceConfig.Enabled,
			LastScannedAt: scannedAt,
		}
		if !sourceConfig.Enabled {
			sources = append(sources, source)
			continue
		}

		entries, err := os.ReadDir(sourceConfig.Path)
		if err != nil {
			source.ErrorCount = 1
			source.Error = err.Error()
			sources = append(sources, source)
			continue
		}

		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			name := entry.Name()
			sourcePath := filepath.Join(sourceConfig.Path, name)
			if !hasSkillFile(sourcePath) {
				continue
			}
			skill := Skill{
				ID:            skillID(sourceConfig.ID, name),
				Name:          name,
				SourceID:      sourceConfig.ID,
				SourceAlias:   displaySourceName(sourceConfig),
				SourcePath:    sourcePath,
				TargetPath:    filepath.Join(config.TargetDir, name),
				SymlinkPath:   filepath.Join(config.TargetDir, name),
				Status:        StatusDisabled,
				LastScannedAt: scannedAt,
			}
			attachSkillMetadata(&skill)
			validateSkill(&skill, config.Validation)
			source.SkillCount++
			if len(skill.ValidationErrors) > 0 {
				source.ErrorCount++
			}
			skills = append(skills, skill)
		}
		sources = append(sources, source)
	}

	deriveStatuses(skills, config.TargetDir)
	sort.Slice(skills, func(i, j int) bool {
		if skills[i].Name == skills[j].Name {
			return skills[i].SourcePath < skills[j].SourcePath
		}
		return skills[i].Name < skills[j].Name
	})

	return Inventory{
		Config:  config,
		Sources: sources,
		Skills:  skills,
		Summary: summarize(skills),
	}, nil
}

func (s *Service) Enable(_ context.Context, config Config, skill Skill) error {
	config = normalizeConfig(config)
	if skill.Name == "" || skill.SourcePath == "" {
		return errors.New("skill name and source path are required")
	}
	if len(skill.ValidationErrors) > 0 || skill.Status == StatusInvalid {
		return fmt.Errorf("cannot enable invalid skill %q", skill.Name)
	}
	targetPath := filepath.Join(config.TargetDir, skill.Name)
	if err := os.MkdirAll(config.TargetDir, 0o755); err != nil {
		return err
	}
	info, err := os.Lstat(targetPath)
	if err == nil {
		if info.Mode()&os.ModeSymlink == 0 {
			return fmt.Errorf("target path is occupied and is not a symlink: %s", targetPath)
		}
		currentTarget, err := resolvedSymlinkTarget(targetPath)
		if err != nil {
			return err
		}
		if samePath(currentTarget, skill.SourcePath) {
			return nil
		}
		return fmt.Errorf("target symlink already points to %s", currentTarget)
	}
	if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return os.Symlink(skill.SourcePath, targetPath)
}

func (s *Service) Disable(_ context.Context, config Config, skill Skill) error {
	config = normalizeConfig(config)
	if skill.Name == "" || skill.SourcePath == "" {
		return errors.New("skill name and source path are required")
	}
	targetPath := filepath.Join(config.TargetDir, skill.Name)
	info, err := os.Lstat(targetPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return fmt.Errorf("target path is not a symlink: %s", targetPath)
	}
	currentTarget, err := resolvedSymlinkTarget(targetPath)
	if err != nil {
		return err
	}
	if !samePath(currentTarget, skill.SourcePath) {
		return fmt.Errorf("refusing to remove symlink for %q because it points to %s", skill.Name, currentTarget)
	}
	return os.Remove(targetPath)
}

func (s *Service) ResolveConflict(ctx context.Context, config Config, skill Skill) error {
	config = normalizeConfig(config)
	targetPath := filepath.Join(config.TargetDir, skill.Name)
	info, err := os.Lstat(targetPath)
	if err == nil {
		if info.Mode()&os.ModeSymlink == 0 {
			return fmt.Errorf("target path is occupied and is not a symlink: %s", targetPath)
		}
		if err := os.Remove(targetPath); err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return s.Enable(ctx, config, skill)
}

func (s *Service) ReadEnvFile(skill Skill) (string, error) {
	if skill.SourcePath == "" {
		return "", errors.New("skill source path is required")
	}
	path := filepath.Join(skill.SourcePath, ".env")
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", fmt.Errorf(".env is a directory: %s", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (s *Service) SaveEnvFile(skill Skill, content string) error {
	if skill.SourcePath == "" {
		return errors.New("skill source path is required")
	}
	if !hasSkillFile(skill.SourcePath) {
		return fmt.Errorf("source folder is not a skill: %s", skill.SourcePath)
	}
	return os.WriteFile(filepath.Join(skill.SourcePath, ".env"), []byte(content), 0o600)
}

func deriveStatuses(skills []Skill, targetDir string) {
	byName := map[string][]int{}
	for i := range skills {
		byName[skills[i].Name] = append(byName[skills[i].Name], i)
	}

	for name, indexes := range byName {
		targetPath := filepath.Join(targetDir, name)
		info, lstatErr := os.Lstat(targetPath)
		var hasSymlink bool
		var symlinkTarget string
		var targetError string

		switch {
		case errors.Is(lstatErr, os.ErrNotExist):
		case lstatErr != nil:
			targetError = lstatErr.Error()
		case info.Mode()&os.ModeSymlink == 0:
			targetError = "target path exists but is not a symlink"
		default:
			hasSymlink = true
			symlinkTarget, targetError = readSymlinkTarget(targetPath)
		}

		for _, index := range indexes {
			skill := &skills[index]
			skill.HasSymlink = hasSymlink
			skill.SymlinkTarget = symlinkTarget
			skill.IsActive = hasSymlink && samePath(symlinkTarget, skill.SourcePath)
			if targetError != "" {
				skill.Status = StatusError
				skill.Error = targetError
			} else if len(skill.ValidationErrors) > 0 {
				skill.Status = StatusInvalid
			} else if len(indexes) > 1 {
				skill.Status = StatusConflict
			} else if !hasSymlink {
				skill.Status = StatusDisabled
			} else if skill.IsActive {
				skill.Status = StatusSynced
			} else {
				skill.Status = StatusConflict
			}
		}

		if len(indexes) > 1 {
			conflictSources := make([]ConflictSource, 0, len(indexes))
			for _, index := range indexes {
				skill := skills[index]
				conflictSources = append(conflictSources, ConflictSource{
					SkillID:    skill.ID,
					SourceID:   skill.SourceID,
					SourcePath: skill.SourcePath,
					Status:     skill.Status,
				})
			}
			for _, index := range indexes {
				skills[index].ConflictSources = conflictSources
			}
		}
	}
}

func validateSkill(skill *Skill, config ValidationConfig) {
	var required []string
	switch config.Mode {
	case ValidationStrict:
		required = []string{"SKILL.md"}
	case ValidationCustom:
		required = config.RequiredFiles
	default:
		return
	}
	for _, file := range required {
		if file == "" {
			continue
		}
		if _, err := os.Stat(filepath.Join(skill.SourcePath, file)); errors.Is(err, os.ErrNotExist) {
			skill.ValidationErrors = append(skill.ValidationErrors, "Missing required file: "+file)
		} else if err != nil {
			skill.ValidationErrors = append(skill.ValidationErrors, err.Error())
		}
	}
}

func hasSkillFile(sourcePath string) bool {
	info, err := os.Stat(filepath.Join(sourcePath, "SKILL.md"))
	return err == nil && !info.IsDir()
}

func attachSkillMetadata(skill *Skill) {
	entries, err := os.ReadDir(skill.SourcePath)
	if err == nil {
		for _, entry := range entries {
			skill.Files = append(skill.Files, entry.Name())
		}
		sort.Strings(skill.Files)
	}
	if info, err := os.Stat(skill.SourcePath); err == nil {
		skill.UpdatedAt = info.ModTime().Format(time.RFC3339)
	}
	for _, previewFile := range []string{"SKILL.md", "README.md"} {
		content, err := os.ReadFile(filepath.Join(skill.SourcePath, previewFile))
		if err != nil {
			continue
		}
		text := string(content)
		skill.PreviewFile = previewFile
		skill.Preview = trimPreview(text)
		skill.Description = extractDescription(text)
		return
	}
}

func summarize(skills []Skill) Summary {
	var summary Summary
	summary.SkillsFound = len(skills)
	for _, skill := range skills {
		switch skill.Status {
		case StatusSynced:
			summary.Enabled++
		case StatusConflict:
			summary.Conflicts++
		case StatusInvalid:
			summary.Invalid++
		case StatusError:
			summary.Errors++
		}
	}
	return summary
}

func readSymlinkTarget(path string) (string, string) {
	target, err := resolvedSymlinkTarget(path)
	if err != nil {
		return "", err.Error()
	}
	return target, ""
}

func resolvedSymlinkTarget(path string) (string, error) {
	target, err := os.Readlink(path)
	if err != nil {
		return "", err
	}
	if !filepath.IsAbs(target) {
		target = filepath.Join(filepath.Dir(path), target)
	}
	return filepath.Clean(target), nil
}

func samePath(left, right string) bool {
	leftAbs, leftErr := filepath.Abs(left)
	rightAbs, rightErr := filepath.Abs(right)
	if leftErr == nil {
		left = leftAbs
	}
	if rightErr == nil {
		right = rightAbs
	}
	return filepath.Clean(left) == filepath.Clean(right)
}

func skillID(sourceIDValue, name string) string {
	return sourceIDValue + ":" + name
}

func sourceID(path string) string {
	sum := sha1.Sum([]byte(filepath.Clean(path)))
	return hex.EncodeToString(sum[:])[:12]
}

func displaySourceName(source SkillSourceConfig) string {
	if source.Alias != "" {
		return source.Alias
	}
	return filepath.Base(source.Path)
}

func expandHome(path string) string {
	if path == "~" || strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			if path == "~" {
				return home
			}
			return filepath.Join(home, strings.TrimPrefix(path, "~/"))
		}
	}
	return path
}

func trimPreview(content string) string {
	const max = 4000
	if len(content) <= max {
		return content
	}
	return content[:max] + "\n..."
}

func extractDescription(content string) string {
	lines := strings.Split(content, "\n")
	inFrontmatter := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if i == 0 && trimmed == "---" {
			inFrontmatter = true
			continue
		}
		if inFrontmatter {
			if trimmed == "---" {
				inFrontmatter = false
				continue
			}
			if strings.HasPrefix(trimmed, "description:") {
				return strings.Trim(strings.TrimSpace(strings.TrimPrefix(trimmed, "description:")), "\"")
			}
		}
		if strings.HasPrefix(trimmed, "# ") {
			return strings.TrimSpace(strings.TrimPrefix(trimmed, "# "))
		}
	}
	return ""
}
