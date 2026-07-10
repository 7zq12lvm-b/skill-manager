package skillmgr

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

type Service struct {
	logger func(string, ...any)
}

const maxSkillFilePreviewBytes int64 = 512 * 1024

func NewService() *Service {
	return &Service{}
}

func (s *Service) SetLogger(logger func(string, ...any)) {
	s.logger = logger
}

func (s *Service) logf(format string, args ...any) {
	if s.logger != nil {
		s.logger(format, args...)
	}
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
	applySyncProfiles(skills, syncDocument)
	deriveStatuses(skills, config.TargetDirs)
	repositories = projectSharedRepositories(repositories, syncDocument)
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
	startedAt := time.Now()
	s.logf("scan repository start repo_id=%q path=%q scan_roots=%v ignore_paths=%v", config.RepoID, config.Path, config.ScanRoots, config.IgnorePaths)
	repository := Repository{
		ID:            config.ID,
		Provider:      GitProvider,
		SourceKey:     PortableSourceKey(GitProvider, config.RepoID),
		RepoID:        config.RepoID,
		Path:          config.Path,
		Alias:         config.Alias,
		Enabled:       config.Enabled,
		CloneURL:      config.CloneURL,
		ScanRoots:     append([]string(nil), config.ScanRoots...),
		IgnorePaths:   append([]string(nil), config.IgnorePaths...),
		Installed:     true,
		LastScannedAt: scannedAt,
	}
	if !config.Enabled {
		s.logf("scan repository skipped disabled repo_id=%q duration=%s", config.RepoID, time.Since(startedAt))
		return repository, nil
	}
	gitRoot, ok := gitRepositoryRoot(ctx, config.Path)
	if !ok {
		repository.ErrorCount = 1
		repository.Error = "repository path is not inside a git repository"
		s.logf("scan repository error repo_id=%q path=%q error=%q duration=%s", config.RepoID, config.Path, repository.Error, time.Since(startedAt))
		return repository, nil
	}
	repository.IsGitRepo = true
	if !samePath(gitRoot, config.Path) {
		repository.Path = gitRoot
	}
	if repository.CloneURL == "" {
		if remote, ok := gitRemoteURL(ctx, repository.Path); ok {
			repository.CloneURL = remote
			config.CloneURL = remote
		}
	}
	repository.CurrentRef = gitCurrentRef(ctx, repository.Path)

	skillPaths := s.discoverSkillFolders(ctx, repository.Path, config.ScanRoots, config.IgnorePaths)
	skillContents := repositorySkillContents(ctx, repository.Path, skillPaths)
	skills := make([]Skill, 0, len(skillPaths))
	for _, skillPath := range skillPaths {
		repoSubpath, err := filepath.Rel(repository.Path, skillPath)
		if err != nil {
			continue
		}
		repoSubpath = cleanRepoSubpath(repoSubpath)
		targetName := targetNameForRepositorySkill(config, repoSubpath)
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
			SourceKey:     PortableSourceKey(GitProvider, config.RepoID),
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
			Ref:           repository.CurrentRef,
			LastScannedAt: scannedAt,
		}
		if content, ok := skillContents[repoSubpath]; ok {
			skill.Files = []string{"SKILL.md"}
			attachSkillTextMetadata(&skill, "SKILL.md", content)
		}
		validateRepositorySkill(ctx, &skill, appConfig.Validation)
		if skill.Manifest != nil && skill.Manifest.Name != "" {
			skill.DisplayName = skill.Manifest.Name
		}
		repository.SkillCount++
		if len(skill.ValidationErrors) > 0 {
			repository.ErrorCount++
		}
		skills = append(skills, skill)
	}
	s.logf("scan repository done repo_id=%q path=%q skills=%d errors=%d dirty=%v duration=%s", config.RepoID, repository.Path, repository.SkillCount, repository.ErrorCount, repository.Dirty, time.Since(startedAt))
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
						skill.Status = StatusEnabled
					} else if conflictTarget != "" {
						skill.Status = StatusConflict
						skill.SymlinkTarget = conflictTarget
					} else {
						skill.Status = StatusDisabled
					}
				} else if activeCount > 0 {
					skill.Status = StatusEnabled
				} else {
					skill.Status = StatusDisabled
				}
			} else if !hasSymlink {
				skill.Status = StatusDisabled
			} else if activeCount > 0 {
				skill.Status = StatusEnabled
			} else if conflictTarget != "" {
				skill.Status = StatusConflict
				skill.SymlinkTarget = conflictTarget
			} else {
				skill.Status = StatusError
				skill.Error = "target link could not be resolved"
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
		repository, hasRepository := repositories[record.Source.ID]
		sourcePath := ""
		status := StatusMissingSource
		errorMessage := "repository is not configured on this machine"
		if hasRepository {
			sourcePath = filepath.Join(repository.Path, filepath.FromSlash(record.Source.Locator.Subpath))
			status = StatusMissingPath
			errorMessage = "SKILL.md was not found at the synced repository path"
		}
		skill := Skill{
			ID:                  id,
			SyncID:              id,
			Name:                record.TargetName,
			TargetName:          record.TargetName,
			PreviousTargetNames: append([]string(nil), record.PreviousTargetNames...),
			SourceID:            record.Source.ID,
			SourceKey:           PortableSourceKey(record.Source.Provider, record.Source.ID),
			SourceAlias:         record.Source.ID,
			SourcePath:          sourcePath,
			RepoID:              record.Source.ID,
			RepoPath:            repository.Path,
			RepoSubpath:         record.Source.Locator.Subpath,
			CloneURL:            record.Source.Locator.CloneURL,
			Status:              status,
			IsSynced:            true,
			DesiredEnabled:      &desired,
			CanSync:             true,
			Ref:                 record.Source.Locator.Ref,
			Tags:                append([]string(nil), record.Tags...),
			Profile:             cloneSkillProfile(record.Profile),
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
	skill.SourceKey = PortableSourceKey(record.Source.Provider, record.Source.ID)
	skill.IsSynced = true
	skill.DesiredEnabled = desired
	skill.TargetName = record.TargetName
	skill.Name = record.TargetName
	skill.PreviousTargetNames = append([]string(nil), record.PreviousTargetNames...)
	skill.Tags = append([]string(nil), record.Tags...)
	skill.Profile = cloneSkillProfile(record.Profile)
	skill.RefMismatch = record.Source.Locator.Ref != "" && currentRef != "" && currentRef != record.Source.Locator.Ref
	skill.Ref = record.Source.Locator.Ref
	skill.CloneURL = record.Source.Locator.CloneURL
	if skill.RepoID == "" {
		skill.RepoID = record.Source.ID
	}
	if skill.RepoSubpath == "" {
		skill.RepoSubpath = record.Source.Locator.Subpath
	}
}

func projectSharedRepositories(installed []Repository, document SyncDocument) []Repository {
	repositories := append([]Repository(nil), installed...)
	bySource := make(map[string]int, len(repositories))
	for index := range repositories {
		if repositories[index].Provider == "" {
			repositories[index].Provider = GitProvider
		}
		if repositories[index].SourceKey == "" {
			repositories[index].SourceKey = PortableSourceKey(repositories[index].Provider, repositories[index].RepoID)
		}
		repositories[index].Installed = true
		bySource[repositories[index].SourceKey] = index
	}

	sharedCounts := map[string]int{}
	for _, record := range document.Skills {
		key := PortableSourceKey(record.Source.Provider, record.Source.ID)
		if key == "" {
			continue
		}
		sharedCounts[key]++
		if index, exists := bySource[key]; exists {
			if repositories[index].CloneURL == "" {
				repositories[index].CloneURL = record.Source.Locator.CloneURL
			}
			continue
		}
		bySource[key] = len(repositories)
		repositories = append(repositories, Repository{
			ID:        key,
			Provider:  record.Source.Provider,
			SourceKey: key,
			RepoID:    record.Source.ID,
			CloneURL:  record.Source.Locator.CloneURL,
			IsGitRepo: record.Source.Provider == GitProvider,
			Installed: false,
		})
	}
	for index := range repositories {
		if count := sharedCounts[repositories[index].SourceKey]; count > 0 {
			repositories[index].SkillCount = count
		}
	}
	sort.Slice(repositories, func(i, j int) bool {
		return repositories[i].SourceKey < repositories[j].SourceKey
	})
	return repositories
}

func applySyncProfiles(skills []Skill, document SyncDocument) {
	if len(document.Profiles) == 0 {
		return
	}
	for index := range skills {
		syncID := skills[index].SyncID
		if syncID == "" {
			continue
		}
		if profile, ok := document.Profiles[syncID]; ok {
			skills[index].Profile = cloneSkillProfile(&profile)
		}
	}
}

func cloneSkillProfile(profile *SkillProfile) *SkillProfile {
	if profile == nil {
		return nil
	}
	cloned := *profile
	cloned.UseCasesZh = append([]string(nil), profile.UseCasesZh...)
	return &cloned
}

func targetNameForSkill(skill Skill) string {
	if skill.TargetName != "" {
		return skill.TargetName
	}
	return skill.Name
}

func targetNameForRepositorySkill(repository RepositoryConfig, repoSubpath string) string {
	repoSubpath = cleanRepoSubpath(repoSubpath)
	if repoSubpath != "." {
		return filepath.Base(filepath.FromSlash(repoSubpath))
	}
	if alias := strings.TrimSpace(repository.Alias); alias != "" {
		return alias
	}
	if repository.Path != "" {
		base := filepath.Base(filepath.Clean(repository.Path))
		if base != "" && base != "." && base != string(filepath.Separator) {
			return base
		}
	}
	if repository.RepoID != "" {
		return filepath.Base(filepath.FromSlash(repository.RepoID))
	}
	return "skill"
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

func (s *Service) discoverSkillFolders(ctx context.Context, repoPath string, scanRoots []string, ignorePaths []string) []string {
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
	if found, ok := discoverSkillFoldersFromGitIndex(ctx, repoPath, scanRoots, ignoreSet); ok {
		s.logf("discover skills via git index path=%q skills=%d", repoPath, len(found))
		if len(found) > 0 {
			return found
		}
	}
	s.logf("discover skills fallback walk path=%q", repoPath)
	return discoverSkillFoldersByWalking(repoPath, scanRoots, ignoreSet)
}

func discoverSkillFoldersFromGitIndex(ctx context.Context, repoPath string, scanRoots []string, ignoreSet map[string]bool) ([]string, bool) {
	if _, err := exec.LookPath("git"); err != nil {
		return nil, false
	}
	cmd := exec.CommandContext(ctx, "git", "-C", repoPath, "ls-files", "-z")
	var output bytes.Buffer
	cmd.Stdout = &output
	if err := cmd.Run(); err != nil {
		return nil, false
	}
	found := make([]string, 0)
	seen := map[string]bool{}
	for _, rawPath := range bytes.Split(output.Bytes(), []byte{0}) {
		if len(rawPath) == 0 {
			continue
		}
		rel := filepath.ToSlash(string(rawPath))
		if filepath.Base(rel) != "SKILL.md" {
			continue
		}
		folder := cleanRepoSubpath(filepath.Dir(rel))
		if !repoPathWithinScanRoots(folder, scanRoots) || repoPathIgnored(folder, ignoreSet) {
			continue
		}
		skillPath := filepath.Join(repoPath, filepath.FromSlash(folder))
		if !seen[skillPath] {
			seen[skillPath] = true
			found = append(found, skillPath)
		}
	}
	sort.Strings(found)
	return found, true
}

func discoverSkillFoldersByWalking(repoPath string, scanRoots []string, ignoreSet map[string]bool) []string {
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

func repoPathWithinScanRoots(path string, scanRoots []string) bool {
	path = cleanRepoSubpath(path)
	for _, root := range scanRoots {
		root = cleanRepoSubpath(root)
		if root == "." || path == root || strings.HasPrefix(path, root+"/") {
			return true
		}
	}
	return false
}

func repoPathIgnored(path string, ignoreSet map[string]bool) bool {
	path = cleanRepoSubpath(path)
	if ignoreSet[path] {
		return true
	}
	for ignorePath := range ignoreSet {
		if ignorePath != "." && strings.HasPrefix(path, ignorePath+"/") {
			return true
		}
	}
	for _, part := range strings.Split(path, "/") {
		if defaultIgnoredDirName(part) {
			return true
		}
	}
	return false
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
		attachSkillTextMetadata(skill, previewFile, string(content))
		return
	}
}

func attachSkillTextMetadata(skill *Skill, previewFile string, content string) {
	skill.PreviewFile = previewFile
	skill.Preview = trimPreview(content)
	if previewFile == "SKILL.md" {
		manifest := parseSkillManifest(content)
		if manifest != nil {
			skill.Manifest = manifest
			skill.Description = manifest.Description
		}
	}
	if skill.Description == "" {
		skill.Description = extractDescription(content)
	}
}

func repositorySkillContents(ctx context.Context, repoPath string, skillPaths []string) map[string]string {
	results := map[string]string{}
	if len(skillPaths) == 0 {
		return results
	}
	if _, err := exec.LookPath("git"); err != nil {
		return results
	}
	type request struct {
		repoSubpath string
		objectPath  string
	}
	requests := make([]request, 0, len(skillPaths))
	var input strings.Builder
	for _, skillPath := range skillPaths {
		repoSubpath, err := filepath.Rel(repoPath, skillPath)
		if err != nil {
			continue
		}
		repoSubpath = cleanRepoSubpath(repoSubpath)
		objectPath := repoObjectPath(repoSubpath, "SKILL.md")
		requests = append(requests, request{repoSubpath: repoSubpath, objectPath: objectPath})
		input.WriteString("HEAD:")
		input.WriteString(objectPath)
		input.WriteByte('\n')
	}
	if len(requests) == 0 {
		return results
	}
	cmd := exec.CommandContext(ctx, "git", "-C", repoPath, "cat-file", "--batch")
	cmd.Stdin = strings.NewReader(input.String())
	output, err := cmd.Output()
	if err != nil {
		return results
	}
	reader := bufio.NewReader(bytes.NewReader(output))
	for _, request := range requests {
		header, err := reader.ReadString('\n')
		if err != nil {
			return results
		}
		header = strings.TrimSpace(header)
		if strings.HasSuffix(header, " missing") {
			continue
		}
		fields := strings.Fields(header)
		if len(fields) < 3 {
			return results
		}
		size, err := strconv.Atoi(fields[2])
		if err != nil || size < 0 {
			return results
		}
		content := make([]byte, size)
		if _, err := io.ReadFull(reader, content); err != nil {
			return results
		}
		if _, err := reader.ReadByte(); err != nil {
			return results
		}
		results[request.repoSubpath] = string(content)
	}
	return results
}

func (s *Service) attachRepositorySkillMetadata(ctx context.Context, skill *Skill) {
	if skill.RepoPath == "" || skill.RepoSubpath == "" {
		attachSkillMetadata(skill)
		return
	}
	if files, ok := gitListTreeNames(ctx, skill.RepoPath, skill.RepoSubpath); ok {
		skill.Files = files
	}
	if updatedAt := gitLastCommitTime(ctx, skill.RepoPath, repoObjectPath(skill.RepoSubpath, "SKILL.md")); updatedAt != "" {
		skill.UpdatedAt = updatedAt
	}
	for _, previewFile := range []string{"SKILL.md", "README.md"} {
		content, ok := gitShowFile(ctx, skill.RepoPath, skill.RepoSubpath, previewFile)
		if !ok {
			continue
		}
		skill.PreviewFile = previewFile
		skill.Preview = trimPreview(content)
		if previewFile == "SKILL.md" {
			manifest := parseSkillManifest(content)
			if manifest != nil {
				skill.Manifest = manifest
				skill.Description = manifest.Description
			}
		}
		if skill.Description == "" {
			skill.Description = extractDescription(content)
		}
		return
	}
}

func (s *Service) ListSkillFiles(ctx context.Context, skill Skill, relativeDir string) ([]SkillFileEntry, error) {
	cleanDir, err := cleanSkillRelativeDir(relativeDir)
	if err != nil {
		return nil, err
	}
	if skill.RepoPath != "" && skill.RepoSubpath != "" {
		if entries, ok := gitListTreeEntries(ctx, skill.RepoPath, skill.RepoSubpath, cleanDir); ok {
			return entries, nil
		}
	}
	if skill.SourcePath == "" {
		return nil, errors.New("skill source path is unavailable")
	}
	return listLocalSkillEntries(skill.SourcePath, cleanDir)
}

func (s *Service) ReadSkillFilePreview(ctx context.Context, skill Skill, relativeFile string) (SkillFilePreview, error) {
	cleanFile, err := cleanSkillRelativeFile(relativeFile)
	if err != nil {
		return SkillFilePreview{}, err
	}
	var content []byte
	if skill.RepoPath != "" && skill.RepoSubpath != "" {
		content, err = readGitSkillFile(ctx, skill.RepoPath, skill.RepoSubpath, cleanFile)
	} else {
		if skill.SourcePath == "" {
			return SkillFilePreview{}, errors.New("skill source path is unavailable")
		}
		content, err = readLocalSkillFile(skill.SourcePath, cleanFile)
	}
	if err != nil {
		if errors.Is(err, errSkillPreviewTooLarge) {
			return SkillFilePreview{
				Path:   cleanFile,
				Reason: "Files larger than 512 KB cannot be previewed.",
			}, nil
		}
		return SkillFilePreview{}, err
	}
	if bytes.IndexByte(content, 0) >= 0 || !utf8.Valid(content) {
		return SkillFilePreview{
			Path:   cleanFile,
			Reason: "Binary files cannot be previewed.",
		}, nil
	}
	return SkillFilePreview{
		Path:        cleanFile,
		Previewable: true,
		Content:     string(content),
	}, nil
}

var errSkillPreviewTooLarge = errors.New("skill file is too large to preview")

func cleanSkillRelativeFile(relativeFile string) (string, error) {
	relativeFile = strings.TrimSpace(filepath.ToSlash(relativeFile))
	if relativeFile == "" || relativeFile == "." {
		return "", errors.New("skill file path is required")
	}
	cleaned := path.Clean(relativeFile)
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") || strings.HasPrefix(cleaned, "/") {
		return "", fmt.Errorf("invalid relative file: %s", relativeFile)
	}
	return cleaned, nil
}

func readLocalSkillFile(sourcePath, relativeFile string) ([]byte, error) {
	sourceRoot, err := filepath.EvalSymlinks(sourcePath)
	if err != nil {
		return nil, err
	}
	sourceRoot, err = filepath.Abs(sourceRoot)
	if err != nil {
		return nil, err
	}
	targetPath, err := filepath.EvalSymlinks(filepath.Join(sourceRoot, filepath.FromSlash(relativeFile)))
	if err != nil {
		return nil, err
	}
	targetPath, err = filepath.Abs(targetPath)
	if err != nil {
		return nil, err
	}
	rel, err := filepath.Rel(sourceRoot, targetPath)
	if err != nil {
		return nil, err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return nil, fmt.Errorf("skill file escapes source folder: %s", relativeFile)
	}
	info, err := os.Stat(targetPath)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("skill file is not a regular file: %s", relativeFile)
	}
	if info.Size() > maxSkillFilePreviewBytes {
		return nil, errSkillPreviewTooLarge
	}
	file, err := os.Open(targetPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	content, err := io.ReadAll(io.LimitReader(file, maxSkillFilePreviewBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(content)) > maxSkillFilePreviewBytes {
		return nil, errSkillPreviewTooLarge
	}
	return content, nil
}

func readGitSkillFile(ctx context.Context, repoPath, repoSubpath, relativeFile string) ([]byte, error) {
	if _, err := exec.LookPath("git"); err != nil {
		return nil, err
	}
	objectPath := repoObjectPath(repoSubpath, relativeFile)
	object := "HEAD:" + objectPath
	objectType, err := exec.CommandContext(ctx, "git", "-C", repoPath, "cat-file", "-t", object).Output()
	if err != nil {
		return nil, fmt.Errorf("skill file is unavailable: %s", relativeFile)
	}
	if strings.TrimSpace(string(objectType)) != "blob" {
		return nil, fmt.Errorf("skill file is not a regular file: %s", relativeFile)
	}
	rawSize, err := exec.CommandContext(ctx, "git", "-C", repoPath, "cat-file", "-s", object).Output()
	if err != nil {
		return nil, fmt.Errorf("could not inspect skill file: %s", relativeFile)
	}
	size, err := strconv.ParseInt(strings.TrimSpace(string(rawSize)), 10, 64)
	if err != nil {
		return nil, fmt.Errorf("could not inspect skill file size: %s", relativeFile)
	}
	if size > maxSkillFilePreviewBytes {
		return nil, errSkillPreviewTooLarge
	}
	content, err := exec.CommandContext(ctx, "git", "-C", repoPath, "show", object).Output()
	if err != nil {
		return nil, fmt.Errorf("could not read skill file: %s", relativeFile)
	}
	return content, nil
}

func cleanSkillRelativeDir(relativeDir string) (string, error) {
	relativeDir = strings.TrimSpace(filepath.ToSlash(relativeDir))
	if relativeDir == "" || relativeDir == "." {
		return "", nil
	}
	cleaned := path.Clean(relativeDir)
	if cleaned == "." {
		return "", nil
	}
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") || strings.HasPrefix(cleaned, "/") {
		return "", fmt.Errorf("invalid relative directory: %s", relativeDir)
	}
	return cleaned, nil
}

func listLocalSkillEntries(sourcePath string, relativeDir string) ([]SkillFileEntry, error) {
	sourceAbs, err := filepath.Abs(sourcePath)
	if err != nil {
		return nil, err
	}
	targetAbs := filepath.Join(sourceAbs, filepath.FromSlash(relativeDir))
	targetAbs, err = filepath.Abs(targetAbs)
	if err != nil {
		return nil, err
	}
	rel, err := filepath.Rel(sourceAbs, targetAbs)
	if err != nil {
		return nil, err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return nil, fmt.Errorf("invalid relative directory: %s", relativeDir)
	}
	dirEntries, err := os.ReadDir(targetAbs)
	if err != nil {
		return nil, err
	}
	entries := make([]SkillFileEntry, 0, len(dirEntries))
	for _, entry := range dirEntries {
		entryPath := entry.Name()
		if relativeDir != "" {
			entryPath = path.Join(relativeDir, entry.Name())
		}
		entries = append(entries, SkillFileEntry{
			Name:  entry.Name(),
			Path:  entryPath,
			IsDir: entry.IsDir(),
		})
	}
	sortSkillFileEntries(entries)
	return entries, nil
}

func validateRepositorySkill(ctx context.Context, skill *Skill, config ValidationConfig) {
	var required []string
	switch config.Mode {
	case ValidationStrict:
		return
	case ValidationCustom:
		required = config.RequiredFiles
	default:
		return
	}
	for _, file := range required {
		if file == "" {
			continue
		}
		if !gitPathExists(ctx, skill.RepoPath, repoObjectPath(skill.RepoSubpath, file)) {
			skill.ValidationErrors = append(skill.ValidationErrors, "Missing required file: "+file)
		}
	}
}

func gitListTreeNames(ctx context.Context, repoPath, repoSubpath string) ([]string, bool) {
	if _, err := exec.LookPath("git"); err != nil {
		return nil, false
	}
	cmd := exec.CommandContext(ctx, "git", "-C", repoPath, "ls-tree", "-z", "--name-only", "HEAD:"+cleanRepoSubpath(repoSubpath))
	var output bytes.Buffer
	cmd.Stdout = &output
	if err := cmd.Run(); err != nil {
		return nil, false
	}
	files := make([]string, 0)
	for _, rawName := range bytes.Split(output.Bytes(), []byte{0}) {
		if len(rawName) == 0 {
			continue
		}
		files = append(files, string(rawName))
	}
	sort.Strings(files)
	return files, true
}

func gitListTreeEntries(ctx context.Context, repoPath, repoSubpath, relativeDir string) ([]SkillFileEntry, bool) {
	if _, err := exec.LookPath("git"); err != nil {
		return nil, false
	}
	objectPath := cleanRepoSubpath(repoSubpath)
	if relativeDir != "" {
		if objectPath == "." {
			objectPath = relativeDir
		} else {
			objectPath = objectPath + "/" + relativeDir
		}
	}
	cmd := exec.CommandContext(ctx, "git", "-C", repoPath, "ls-tree", "-z", "HEAD:"+objectPath)
	var output bytes.Buffer
	cmd.Stdout = &output
	if err := cmd.Run(); err != nil {
		return nil, false
	}
	entries := make([]SkillFileEntry, 0)
	for _, rawEntry := range bytes.Split(output.Bytes(), []byte{0}) {
		if len(rawEntry) == 0 {
			continue
		}
		metadata, rawName, ok := bytes.Cut(rawEntry, []byte{'\t'})
		if !ok {
			continue
		}
		fields := strings.Fields(string(metadata))
		if len(fields) < 2 {
			continue
		}
		name := string(rawName)
		entryPath := name
		if relativeDir != "" {
			entryPath = path.Join(relativeDir, name)
		}
		entries = append(entries, SkillFileEntry{
			Name:  name,
			Path:  entryPath,
			IsDir: fields[1] == "tree",
		})
	}
	sortSkillFileEntries(entries)
	return entries, true
}

func sortSkillFileEntries(entries []SkillFileEntry) {
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].IsDir != entries[j].IsDir {
			return entries[i].IsDir
		}
		return strings.ToLower(entries[i].Name) < strings.ToLower(entries[j].Name)
	})
}

func gitShowFile(ctx context.Context, repoPath, repoSubpath, file string) (string, bool) {
	if _, err := exec.LookPath("git"); err != nil {
		return "", false
	}
	objectPath := repoObjectPath(repoSubpath, file)
	cmd := exec.CommandContext(ctx, "git", "-C", repoPath, "show", "HEAD:"+objectPath)
	var output bytes.Buffer
	cmd.Stdout = &output
	if err := cmd.Run(); err != nil {
		return "", false
	}
	return output.String(), true
}

func gitPathExists(ctx context.Context, repoPath, objectPath string) bool {
	if _, err := exec.LookPath("git"); err != nil {
		return false
	}
	cmd := exec.CommandContext(ctx, "git", "-C", repoPath, "cat-file", "-e", "HEAD:"+objectPath)
	return cmd.Run() == nil
}

func gitLastCommitTime(ctx context.Context, repoPath, objectPath string) string {
	if _, err := exec.LookPath("git"); err != nil {
		return ""
	}
	output, err := exec.CommandContext(ctx, "git", "-C", repoPath, "log", "-1", "--format=%cI", "--", objectPath).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

func repoObjectPath(repoSubpath, file string) string {
	repoSubpath = cleanRepoSubpath(repoSubpath)
	file = cleanRepoSubpath(file)
	if repoSubpath == "." {
		return file
	}
	return repoSubpath + "/" + file
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
