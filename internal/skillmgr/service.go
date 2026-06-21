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

	"gopkg.in/yaml.v3"
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
				TargetPath:    filepath.Join(config.TargetDirs[0], name),
				SymlinkPath:   filepath.Join(config.TargetDirs[0], name),
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

	deriveStatuses(skills, config.TargetDirs)
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
	for _, targetDir := range config.TargetDirs {
		if err := enableInTarget(targetDir, skill); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) Disable(_ context.Context, config Config, skill Skill) error {
	config = normalizeConfig(config)
	if skill.Name == "" || skill.SourcePath == "" {
		return errors.New("skill name and source path are required")
	}
	removedAny := false
	var blockers []string
	for _, targetDir := range config.TargetDirs {
		removed, blocker, err := disableInTarget(targetDir, skill)
		if err != nil {
			return err
		}
		removedAny = removedAny || removed
		if blocker != "" {
			blockers = append(blockers, blocker)
		}
	}
	if !removedAny && len(blockers) > 0 {
		return errors.New(strings.Join(blockers, "; "))
	}
	return nil
}

func (s *Service) ResolveConflict(ctx context.Context, config Config, skill Skill) error {
	config = normalizeConfig(config)
	for _, targetDir := range config.TargetDirs {
		targetPath := filepath.Join(targetDir, skill.Name)
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
	}
	return s.Enable(ctx, config, skill)
}

func enableInTarget(targetDir string, skill Skill) error {
	targetPath := filepath.Join(targetDir, skill.Name)
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
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

func disableInTarget(targetDir string, skill Skill) (bool, string, error) {
	targetPath := filepath.Join(targetDir, skill.Name)
	info, err := os.Lstat(targetPath)
	if errors.Is(err, os.ErrNotExist) {
		return false, "", nil
	}
	if err != nil {
		return false, "", err
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return false, fmt.Sprintf("target path is not a symlink: %s", targetPath), nil
	}
	currentTarget, err := resolvedSymlinkTarget(targetPath)
	if err != nil {
		return false, "", err
	}
	if !samePath(currentTarget, skill.SourcePath) {
		return false, fmt.Sprintf("refusing to remove symlink for %q because it points to %s", skill.Name, currentTarget), nil
	}
	return true, "", os.Remove(targetPath)
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

func deriveStatuses(skills []Skill, targetDirs []string) {
	byName := map[string][]int{}
	for i := range skills {
		byName[skills[i].Name] = append(byName[skills[i].Name], i)
	}

	for name, indexes := range byName {
		for _, index := range indexes {
			skill := &skills[index]
			targetStates := inspectTargets(name, skill.SourcePath, targetDirs)
			skill.TargetStates = targetStates
			if len(targetStates) > 0 {
				primary := targetStates[0]
				skill.TargetPath = primary.TargetPath
				skill.SymlinkPath = primary.SymlinkPath
				skill.SymlinkTarget = primary.SymlinkTarget
			}

			activeCount := 0
			hasSymlink := false
			var targetError string
			var conflictTarget string
			for _, targetState := range targetStates {
				if targetState.HasSymlink {
					hasSymlink = true
				}
				if targetState.IsActive {
					activeCount++
				}
				if targetError == "" && targetState.Error != "" {
					targetError = fmt.Sprintf("%s: %s", targetState.TargetPath, targetState.Error)
				}
				if conflictTarget == "" && targetState.HasSymlink && !targetState.IsActive {
					conflictTarget = targetState.SymlinkTarget
				}
			}
			skill.HasSymlink = hasSymlink
			skill.IsActive = activeCount > 0
			if targetError != "" {
				skill.Status = StatusError
				skill.Error = targetError
			} else if len(skill.ValidationErrors) > 0 {
				skill.Status = StatusInvalid
			} else if len(indexes) > 1 {
				skill.Status = StatusConflict
			} else if !hasSymlink {
				skill.Status = StatusDisabled
			} else if activeCount == len(targetStates) {
				skill.Status = StatusSynced
			} else if conflictTarget != "" {
				skill.Status = StatusConflict
				skill.SymlinkTarget = conflictTarget
			} else {
				skill.Status = StatusSyncing
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

func inspectTargets(name string, sourcePath string, targetDirs []string) []SkillTarget {
	targetStates := make([]SkillTarget, 0, len(targetDirs))
	for _, targetDir := range targetDirs {
		targetPath := filepath.Join(targetDir, name)
		targetState := SkillTarget{
			TargetDir:   targetDir,
			TargetPath:  targetPath,
			SymlinkPath: targetPath,
		}
		info, lstatErr := os.Lstat(targetPath)
		switch {
		case errors.Is(lstatErr, os.ErrNotExist):
		case lstatErr != nil:
			targetState.Error = lstatErr.Error()
		case info.Mode()&os.ModeSymlink == 0:
			targetState.Error = "target path exists but is not a symlink"
		default:
			targetState.HasSymlink = true
			symlinkTarget, targetError := readSymlinkTarget(targetPath)
			targetState.SymlinkTarget = symlinkTarget
			targetState.Error = targetError
			targetState.IsActive = targetError == "" && samePath(symlinkTarget, sourcePath)
		}
		targetStates = append(targetStates, targetState)
	}
	return targetStates
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
		if previewFile == "SKILL.md" {
			manifest := parseSkillManifest(text)
			if manifest != nil {
				skill.Manifest = manifest
				skill.Description = manifest.Description
			}
		}
		if skill.Description == "" {
			skill.Description = extractDescription(text)
		}
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

func parseSkillManifest(content string) *SkillManifest {
	frontmatter, ok := extractFrontmatter(content)
	if !ok {
		return nil
	}
	var raw map[string]any
	if err := yaml.Unmarshal([]byte(frontmatter), &raw); err != nil {
		return nil
	}
	manifest := &SkillManifest{
		Name:                   getManifestString(raw, "name"),
		Description:            getManifestString(raw, "description"),
		License:                getManifestString(raw, "license"),
		Compatibility:          getManifestString(raw, "compatibility"),
		Metadata:               getManifestStringMap(raw, "metadata"),
		AllowedTools:           getManifestString(raw, "allowedTools", "allowed-tools", "allowed_tools"),
		WhenToUse:              getManifestString(raw, "whenToUse", "when-to-use", "when_to_use"),
		DisableModelInvocation: getManifestBool(raw, "disableModelInvocation", "disable-model-invocation", "disable_model_invocation"),
		UserInvocable:          getManifestBool(raw, "userInvocable", "user-invocable", "user_invocable"),
		ArgumentHint:           getManifestString(raw, "argumentHint", "argument-hint", "argument_hint"),
		Arguments:              getManifestArguments(raw, "arguments"),
	}
	if manifest.Name == "" &&
		manifest.Description == "" &&
		manifest.License == "" &&
		manifest.Compatibility == "" &&
		len(manifest.Metadata) == 0 &&
		manifest.AllowedTools == "" &&
		manifest.WhenToUse == "" &&
		manifest.DisableModelInvocation == nil &&
		manifest.UserInvocable == nil &&
		manifest.ArgumentHint == "" &&
		manifest.Arguments == nil {
		return nil
	}
	return manifest
}

func extractFrontmatter(content string) (string, bool) {
	content = strings.TrimPrefix(content, "\ufeff")
	if !strings.HasPrefix(content, "---") {
		return "", false
	}
	lines := strings.Split(content, "\n")
	if strings.TrimSpace(lines[0]) != "---" {
		return "", false
	}
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			return strings.Join(lines[1:i], "\n"), true
		}
	}
	return "", false
}

func getManifestString(raw map[string]any, keys ...string) string {
	value, ok := getManifestValue(raw, keys...)
	if !ok || value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return typed
	default:
		return fmt.Sprint(typed)
	}
}

func getManifestBool(raw map[string]any, keys ...string) *bool {
	value, ok := getManifestValue(raw, keys...)
	if !ok {
		return nil
	}
	var result bool
	switch typed := value.(type) {
	case bool:
		result = typed
	case string:
		result = strings.EqualFold(typed, "true") || strings.EqualFold(typed, "yes")
	default:
		return nil
	}
	return &result
}

func getManifestStringMap(raw map[string]any, keys ...string) map[string]string {
	value, ok := getManifestValue(raw, keys...)
	if !ok || value == nil {
		return nil
	}
	result := map[string]string{}
	switch typed := value.(type) {
	case map[string]any:
		for key, value := range typed {
			result[key] = fmt.Sprint(value)
		}
	case map[any]any:
		for key, value := range typed {
			result[fmt.Sprint(key)] = fmt.Sprint(value)
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func getManifestArguments(raw map[string]any, keys ...string) any {
	value, ok := getManifestValue(raw, keys...)
	if !ok || value == nil {
		return nil
	}
	switch typed := value.(type) {
	case string:
		return typed
	case []any:
		result := make([]string, 0, len(typed))
		for _, value := range typed {
			result = append(result, fmt.Sprint(value))
		}
		return result
	case []string:
		return typed
	default:
		return fmt.Sprint(typed)
	}
}

func getManifestValue(raw map[string]any, keys ...string) (any, bool) {
	for _, key := range keys {
		if value, ok := raw[key]; ok {
			return value, true
		}
	}
	return nil, false
}
