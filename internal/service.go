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

func (s *Service) Scan(ctx context.Context, config Config) (Inventory, error) {
	return s.ScanWithSync(ctx, config, SyncDocument{})
}

func (s *Service) ScanWithSync(ctx context.Context, config Config, syncDocument SyncDocument) (Inventory, error) {
	config = normalizeConfig(config)
	syncDocument = normalizeSyncDocument(syncDocument)
	scannedAt := time.Now().Format(time.RFC3339)
	repositories := make([]Repository, 0, len(config.Repositories))
	sources := make([]SkillSource, 0, len(config.Sources))
	skills := make([]Skill, 0)

	for _, repositoryConfig := range config.Repositories {
		repository, repositorySkills := s.scanRepository(ctx, repositoryConfig, config, scannedAt)
		repositories = append(repositories, repository)
		skills = append(skills, repositorySkills...)
	}

	for _, sourceConfig := range config.Sources {
		source := SkillSource{
			ID:            sourceConfig.ID,
			Path:          sourceConfig.Path,
			Alias:         sourceConfig.Alias,
			Enabled:       sourceConfig.Enabled,
			LastScannedAt: scannedAt,
		}
		if gitRoot, ok := gitRepositoryRoot(ctx, sourceConfig.Path); ok {
			source.IsGitRepo = true
			source.GitRoot = gitRoot
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
			targetName := name
			skill := Skill{
				ID:            skillID(sourceConfig.ID, name),
				Name:          name,
				TargetName:    targetName,
				SourceID:      sourceConfig.ID,
				SourceAlias:   displaySourceName(sourceConfig),
				SourcePath:    sourcePath,
				TargetPath:    filepath.Join(config.TargetDirs[0], targetName),
				SymlinkPath:   filepath.Join(config.TargetDirs[0], targetName),
				Status:        StatusDisabled,
				LocalOnly:     true,
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

	applySyncDocument(&skills, config, syncDocument, scannedAt)
	deriveStatuses(skills, config.TargetDirs)
	sort.Slice(skills, func(i, j int) bool {
		if skills[i].Name == skills[j].Name {
			return skills[i].SourcePath < skills[j].SourcePath
		}
		return skills[i].Name < skills[j].Name
	})

	return Inventory{
		Config:         config,
		Sources:        sources,
		Repositories:   repositories,
		Skills:         skills,
		Summary:        summarize(skills),
		SyncConfigured: config.Sync.Folder != "",
		SyncPath:       SyncPathFromFolder(config.Sync.Folder),
	}, nil
}

func (s *Service) scanRepository(ctx context.Context, config RepositoryConfig, appConfig Config, scannedAt string) (Repository, []Skill) {
	repository := Repository{
		ID:            config.ID,
		RepoID:        config.RepoID,
		Path:          config.Path,
		Alias:         config.Alias,
		Enabled:       config.Enabled,
		CloneURL:      config.CloneURL,
		ScanRoots:     append([]string(nil), config.ScanRoots...),
		IgnorePaths:   append([]string(nil), config.IgnorePaths...),
		LastScannedAt: scannedAt,
	}
	if !config.Enabled {
		return repository, nil
	}
	gitRoot, ok := gitRepositoryRoot(ctx, config.Path)
	if !ok {
		repository.ErrorCount = 1
		repository.Error = "repository path is not inside a git repository"
		return repository, nil
	}
	repository.IsGitRepo = true
	if !samePath(gitRoot, config.Path) {
		repository.Path = gitRoot
	}
	repository.CurrentRef = gitCurrentRef(ctx, repository.Path)
	if dirty, err := gitWorktreeDirty(ctx, repository.Path); err == nil {
		repository.Dirty = dirty
	}

	skills := make([]Skill, 0)
	for _, skillPath := range discoverSkillFolders(repository.Path, config.ScanRoots, config.IgnorePaths) {
		repoSubpath, err := filepath.Rel(repository.Path, skillPath)
		if err != nil {
			continue
		}
		repoSubpath = cleanRepoSubpath(repoSubpath)
		targetName := filepath.Base(repoSubpath)
		id := syncSkillID(config.RepoID, repoSubpath)
		if id == "" {
			id = skillID(config.ID, repoSubpath)
		}
		skill := Skill{
			ID:            id,
			SyncID:        syncSkillID(config.RepoID, repoSubpath),
			Name:          targetName,
			TargetName:    targetName,
			SourceID:      config.ID,
			SourceAlias:   displayRepositoryName(config),
			SourcePath:    skillPath,
			RepoID:        config.RepoID,
			RepoPath:      repository.Path,
			RepoSubpath:   repoSubpath,
			CloneURL:      config.CloneURL,
			TargetPath:    filepath.Join(appConfig.TargetDirs[0], targetName),
			SymlinkPath:   filepath.Join(appConfig.TargetDirs[0], targetName),
			Status:        StatusDisabled,
			CanSync:       config.RepoID != "",
			LocalOnly:     config.RepoID == "",
			Ref:           repository.CurrentRef,
			LastScannedAt: scannedAt,
		}
		attachSkillMetadata(&skill)
		validateSkill(&skill, appConfig.Validation)
		if skill.Manifest != nil && skill.Manifest.Name != "" {
			skill.DisplayName = skill.Manifest.Name
		}
		repository.SkillCount++
		if len(skill.ValidationErrors) > 0 {
			repository.ErrorCount++
		}
		skills = append(skills, skill)
	}
	return repository, skills
}

func (s *Service) PullSource(ctx context.Context, source SkillSourceConfig) (string, error) {
	return pullGitRepository(ctx, source.Path)
}

func (s *Service) PullRepository(ctx context.Context, repository RepositoryConfig) (string, error) {
	return pullGitRepository(ctx, repository.Path)
}

func (s *Service) CloneRepository(ctx context.Context, cloneURL, parentDir, folderName string) (string, string, error) {
	return cloneGitRepository(ctx, cloneURL, parentDir, folderName)
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
		targetPath := filepath.Join(targetDir, targetNameForSkill(skill))
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
	targetPath := filepath.Join(targetDir, targetNameForSkill(skill))
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
	targetPath := filepath.Join(targetDir, targetNameForSkill(skill))
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
		return false, fmt.Sprintf("refusing to remove symlink for %q because it points to %s", targetNameForSkill(skill), currentTarget), nil
	}
	return true, "", os.Remove(targetPath)
}

func DisableInTargetForApp(targetDirs []string, skill Skill) (bool, []string, error) {
	removedAny := false
	var blockers []string
	for _, targetDir := range targetDirs {
		removed, blocker, err := disableInTarget(targetDir, skill)
		if err != nil {
			return removedAny, blockers, err
		}
		removedAny = removedAny || removed
		if blocker != "" {
			blockers = append(blockers, blocker)
		}
	}
	return removedAny, blockers, nil
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
		byName[targetNameForSkill(skills[i])] = append(byName[targetNameForSkill(skills[i])], i)
	}

	for _, indexes := range byName {
		desiredEnabledCount := 0
		legacyDuplicate := len(indexes) > 1
		for _, index := range indexes {
			if skills[index].CanSync || skills[index].IsSynced {
				legacyDuplicate = false
			}
			if skills[index].DesiredEnabled != nil && *skills[index].DesiredEnabled {
				desiredEnabledCount++
			}
		}
		for _, index := range indexes {
			skill := &skills[index]
			if skill.TargetName == "" {
				skill.TargetName = skill.Name
			}
			targetStates := inspectTargets(skill.TargetName, skill.SourcePath, targetDirs)
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
			} else if skill.Status == StatusMissingSource || skill.Status == StatusMissingPath {
				continue
			} else if legacyDuplicate {
				skill.Status = StatusConflict
			} else if skill.IsSynced {
				desired := skill.DesiredEnabled != nil && *skill.DesiredEnabled
				if desired {
					if desiredEnabledCount > 1 {
						skill.Status = StatusConflict
					} else if activeCount == len(targetStates) {
						skill.Status = StatusSynced
					} else if conflictTarget != "" {
						skill.Status = StatusConflict
						skill.SymlinkTarget = conflictTarget
					} else {
						skill.Status = StatusNeedsApply
					}
				} else if activeCount > 0 {
					skill.Status = StatusNeedsApply
				} else {
					skill.Status = StatusDisabled
				}
			} else if !hasSymlink {
				skill.Status = StatusDisabled
			} else if activeCount > 0 {
				skill.Status = StatusLocalOnly
			} else if conflictTarget != "" {
				skill.Status = StatusConflict
				skill.SymlinkTarget = conflictTarget
			} else {
				skill.Status = StatusSyncing
			}
		}

		if legacyDuplicate || desiredEnabledCount > 1 {
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

func applySyncDocument(skills *[]Skill, config Config, document SyncDocument, scannedAt string) {
	if len(document.Skills) == 0 {
		return
	}
	bySyncID := map[string]int{}
	for i := range *skills {
		if (*skills)[i].SyncID != "" {
			bySyncID[(*skills)[i].SyncID] = i
		}
	}
	repositories := map[string]RepositoryConfig{}
	for _, repository := range config.Repositories {
		repositories[repository.RepoID] = repository
	}
	for id, record := range document.Skills {
		desired := record.Enabled
		if index, ok := bySyncID[id]; ok {
			skill := &(*skills)[index]
			applySyncRecordToSkill(skill, id, record, &desired)
			continue
		}
		repository, hasRepository := repositories[record.Source.RepoID]
		sourcePath := ""
		status := StatusMissingSource
		errorMessage := "repository is not configured on this machine"
		if hasRepository {
			sourcePath = filepath.Join(repository.Path, filepath.FromSlash(record.Source.RepoSubpath))
			status = StatusMissingPath
			errorMessage = "SKILL.md was not found at the synced repository path"
		}
		skill := Skill{
			ID:                  id,
			SyncID:              id,
			Name:                record.TargetName,
			TargetName:          record.TargetName,
			PreviousTargetNames: append([]string(nil), record.PreviousTargetNames...),
			SourceID:            record.Source.RepoID,
			SourceAlias:         record.Source.RepoID,
			SourcePath:          sourcePath,
			RepoID:              record.Source.RepoID,
			RepoPath:            repository.Path,
			RepoSubpath:         record.Source.RepoSubpath,
			CloneURL:            record.Source.CloneURL,
			Status:              status,
			IsSynced:            true,
			DesiredEnabled:      &desired,
			CanSync:             true,
			Ref:                 record.Source.Ref,
			Tags:                append([]string(nil), record.Tags...),
			Error:               errorMessage,
			LastScannedAt:       scannedAt,
		}
		*skills = append(*skills, skill)
	}
}

func applySyncRecordToSkill(skill *Skill, syncID string, record SyncSkillRecord, desired *bool) {
	currentRef := skill.Ref
	skill.ID = syncID
	skill.SyncID = syncID
	skill.IsSynced = true
	skill.DesiredEnabled = desired
	skill.TargetName = record.TargetName
	skill.Name = record.TargetName
	skill.PreviousTargetNames = append([]string(nil), record.PreviousTargetNames...)
	skill.Tags = append([]string(nil), record.Tags...)
	skill.RefMismatch = record.Source.Ref != "" && currentRef != "" && currentRef != record.Source.Ref
	skill.Ref = record.Source.Ref
	skill.CloneURL = record.Source.CloneURL
	if skill.RepoID == "" {
		skill.RepoID = record.Source.RepoID
	}
	if skill.RepoSubpath == "" {
		skill.RepoSubpath = record.Source.RepoSubpath
	}
}

func targetNameForSkill(skill Skill) string {
	if skill.TargetName != "" {
		return skill.TargetName
	}
	return skill.Name
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

func discoverSkillFolders(repoPath string, scanRoots []string, ignorePaths []string) []string {
	if len(scanRoots) == 0 {
		scanRoots = []string{"."}
	}
	ignoreSet := defaultIgnorePathSet()
	for _, ignorePath := range ignorePaths {
		ignorePath = cleanRepoSubpath(ignorePath)
		if ignorePath != "" {
			ignoreSet[ignorePath] = true
		}
	}
	found := make([]string, 0)
	seen := map[string]bool{}
	for _, scanRoot := range scanRoots {
		scanRoot = cleanRepoSubpath(scanRoot)
		rootPath := repoPath
		if scanRoot != "." {
			rootPath = filepath.Join(repoPath, filepath.FromSlash(scanRoot))
		}
		_ = filepath.WalkDir(rootPath, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if entry.IsDir() {
				if shouldIgnoreScanDir(repoPath, path, entry.Name(), ignoreSet) {
					return filepath.SkipDir
				}
				return nil
			}
			if entry.Name() != "SKILL.md" {
				return nil
			}
			skillPath := filepath.Dir(path)
			if !seen[skillPath] {
				seen[skillPath] = true
				found = append(found, skillPath)
			}
			return nil
		})
	}
	sort.Strings(found)
	return found
}

func shouldIgnoreScanDir(repoPath, path, name string, ignoreSet map[string]bool) bool {
	if path == repoPath {
		return false
	}
	if defaultIgnoredDirName(name) {
		return true
	}
	rel, err := filepath.Rel(repoPath, path)
	if err != nil {
		return false
	}
	rel = cleanRepoSubpath(rel)
	if ignoreSet[rel] {
		return true
	}
	for ignorePath := range ignoreSet {
		if ignorePath != "." && strings.HasPrefix(rel, ignorePath+"/") {
			return true
		}
	}
	return false
}

func defaultIgnorePathSet() map[string]bool {
	return map[string]bool{
		".git":         true,
		"node_modules": true,
		"vendor":       true,
		"dist":         true,
		"build":        true,
		"target":       true,
		".venv":        true,
		"venv":         true,
		"__pycache__":  true,
	}
}

func defaultIgnoredDirName(name string) bool {
	switch name {
	case ".git", "node_modules", "vendor", "dist", "build", "target", ".venv", "venv", "__pycache__":
		return true
	default:
		return false
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
		if skill.IsActive {
			summary.Enabled++
		}
		switch skill.Status {
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

func displayRepositoryName(repository RepositoryConfig) string {
	if repository.Alias != "" {
		return repository.Alias
	}
	if repository.RepoID != "" {
		return repository.RepoID
	}
	return filepath.Base(repository.Path)
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
