package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"skill-manager/internal"

	"github.com/fsnotify/fsnotify"
	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

type App struct {
	ctx       context.Context
	store     *skillmgr.ConfigStore
	service   *skillmgr.Service
	logPath   string
	mu        sync.Mutex
	config    skillmgr.Config
	inventory skillmgr.Inventory
	watcher   *fsnotify.Watcher
}

func NewApp() *App {
	configPath, err := skillmgr.DefaultConfigPath()
	if err != nil {
		configPath = filepath.Join(".", "config.json")
	}
	app := &App{
		store:   skillmgr.NewConfigStore(configPath),
		service: skillmgr.NewService(),
		logPath: defaultDebugLogPath(),
	}
	app.service.SetLogger(app.debugLogf)
	return app
}

func defaultDebugLogPath() string {
	if runtime.GOOS == "darwin" {
		homeDir, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(homeDir, "Library", "Logs", "skill-manager", "debug.log")
		}
	}
	cacheDir, err := os.UserCacheDir()
	if err == nil {
		return filepath.Join(cacheDir, "skill-manager", "debug.log")
	}
	return filepath.Join(".", "skill-manager-debug.log")
}

func (a *App) debugLogf(format string, args ...any) {
	path := a.logPath
	if path == "" {
		path = defaultDebugLogPath()
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		fmt.Println("debug log:", err)
		return
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		fmt.Println("debug log:", err)
		return
	}
	defer file.Close()
	message := fmt.Sprintf(format, args...)
	_, _ = fmt.Fprintf(file, "%s %s\n", time.Now().Format(time.RFC3339Nano), message)
}

func (a *App) GetDebugLogPath() string {
	return a.logPath
}

func (a *App) startup(ctx context.Context) {
	a.debugLogf("startup begin")
	a.mu.Lock()
	defer a.mu.Unlock()
	a.ctx = ctx
	config, err := a.store.Load()
	if err != nil {
		a.debugLogf("load config error: %v", err)
		fmt.Println("load config:", err)
		config = skillmgr.DefaultConfig()
	}
	a.config = config
	a.debugLogf("config loaded repositories=%d sources=%d sync_folder=%q watch=%v", len(config.Repositories), len(config.Sources), config.Sync.Folder, config.Scan.WatchSourceFolders)
	if err := a.refreshLocked(ctx); err != nil {
		a.debugLogf("initial scan error: %v", err)
		fmt.Println("initial scan:", err)
	}
	if config.Scan.WatchSourceFolders {
		if err := a.restartWatcherLocked(); err != nil {
			a.debugLogf("start watcher error: %v", err)
			fmt.Println("start watcher:", err)
		}
	}
	a.debugLogf("startup done")
}

func (a *App) shutdown(ctx context.Context) {
	a.debugLogf("shutdown begin")
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.watcher != nil {
		_ = a.watcher.Close()
		a.watcher = nil
	}
	a.debugLogf("shutdown done")
}

func (a *App) GetInventory() (skillmgr.Inventory, error) {
	a.debugLogf("GetInventory begin")
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.inventory.Skills == nil && a.inventory.Sources == nil {
		if err := a.refreshLocked(a.ctx); err != nil {
			a.debugLogf("GetInventory refresh error: %v", err)
			return skillmgr.Inventory{}, err
		}
	}
	a.debugLogf("GetInventory done skills=%d repositories=%d", len(a.inventory.Skills), len(a.inventory.Repositories))
	return a.inventory, nil
}

func (a *App) ListSkillFiles(skillID string, relativeDir string) ([]skillmgr.SkillFileEntry, error) {
	a.debugLogf("ListSkillFiles begin skill=%q dir=%q", skillID, relativeDir)
	a.mu.Lock()
	var selected skillmgr.Skill
	found := false
	for _, skill := range a.inventory.Skills {
		if skill.ID == skillID {
			selected = skill
			found = true
			break
		}
	}
	ctx := a.ctx
	a.mu.Unlock()
	if !found {
		return nil, fmt.Errorf("skill not found: %s", skillID)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	entries, err := a.service.ListSkillFiles(ctx, selected, relativeDir)
	if err != nil {
		a.debugLogf("ListSkillFiles error: %v", err)
		return nil, err
	}
	a.debugLogf("ListSkillFiles done skill=%q dir=%q entries=%d", skillID, relativeDir, len(entries))
	return entries, nil
}

func (a *App) ReadSkillFilePreview(skillID string, relativeFile string) (skillmgr.SkillFilePreview, error) {
	a.debugLogf("ReadSkillFilePreview begin skill=%q file=%q", skillID, relativeFile)
	a.mu.Lock()
	selected, err := a.findSkillLocked(skillID)
	ctx := a.ctx
	a.mu.Unlock()
	if err != nil {
		return skillmgr.SkillFilePreview{}, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	preview, err := a.service.ReadSkillFilePreview(ctx, selected, relativeFile)
	if err != nil {
		a.debugLogf("ReadSkillFilePreview error: %v", err)
		return skillmgr.SkillFilePreview{}, err
	}
	a.debugLogf("ReadSkillFilePreview done skill=%q file=%q previewable=%t", skillID, preview.Path, preview.Previewable)
	return preview, nil
}

func (a *App) RescanAll() (skillmgr.Inventory, error) {
	a.debugLogf("RescanAll begin")
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := a.refreshLocked(a.ctx); err != nil {
		a.debugLogf("RescanAll error: %v", err)
		return skillmgr.Inventory{}, err
	}
	a.debugLogf("RescanAll done skills=%d repositories=%d", len(a.inventory.Skills), len(a.inventory.Repositories))
	return a.inventory, nil
}

func (a *App) AddSource(path string) (skillmgr.Inventory, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if path == "" {
		return skillmgr.Inventory{}, errors.New("source path is required")
	}
	if a.config.Sync.Folder == "" {
		return skillmgr.Inventory{}, errors.New("choose a sync folder before adding repositories")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return skillmgr.Inventory{}, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return skillmgr.Inventory{}, err
	}
	if !info.IsDir() {
		return skillmgr.Inventory{}, fmt.Errorf("source path is not a directory: %s", abs)
	}
	if repository, ok := a.repositoryConfigFromPathLocked(a.ctx, abs); ok {
		for _, existing := range a.config.Repositories {
			if existing.RepoID == repository.RepoID {
				return a.inventory, nil
			}
		}
		a.config.Repositories = append(a.config.Repositories, repository)
		if err := a.persistAndRefreshLocked(); err != nil {
			return skillmgr.Inventory{}, err
		}
		return a.inventory, nil
	}
	return skillmgr.Inventory{}, errors.New("cross-device sync currently supports only Git repositories with a usable remote")
}

func (a *App) AddRepository(path string) (skillmgr.Inventory, error) {
	return a.AddSource(path)
}

func (a *App) RemoveSource(sourceID string) (skillmgr.Inventory, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	nextRepos := a.config.Repositories[:0]
	removedRepo := false
	for _, repository := range a.config.Repositories {
		if repository.ID == sourceID || repository.RepoID == sourceID {
			removedRepo = true
			continue
		}
		nextRepos = append(nextRepos, repository)
	}
	a.config.Repositories = nextRepos
	if removedRepo {
		if err := a.persistAndRefreshLocked(); err != nil {
			return skillmgr.Inventory{}, err
		}
		return a.inventory, nil
	}
	next := a.config.Sources[:0]
	for _, source := range a.config.Sources {
		if source.ID != sourceID {
			next = append(next, source)
		}
	}
	a.config.Sources = next
	if err := a.persistAndRefreshLocked(); err != nil {
		return skillmgr.Inventory{}, err
	}
	return a.inventory, nil
}

func (a *App) RemoveRepository(repoID string) (skillmgr.Inventory, error) {
	return a.RemoveSource(repoID)
}

func (a *App) RenameSource(sourceID string, alias string) (skillmgr.Inventory, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for i := range a.config.Repositories {
		if a.config.Repositories[i].ID == sourceID || a.config.Repositories[i].RepoID == sourceID {
			a.config.Repositories[i].Alias = alias
			if err := a.persistAndRefreshLocked(); err != nil {
				return skillmgr.Inventory{}, err
			}
			return a.inventory, nil
		}
	}
	for i := range a.config.Sources {
		if a.config.Sources[i].ID == sourceID {
			a.config.Sources[i].Alias = alias
			break
		}
	}
	if err := a.persistAndRefreshLocked(); err != nil {
		return skillmgr.Inventory{}, err
	}
	return a.inventory, nil
}

func (a *App) RenameRepository(repoID string, alias string) (skillmgr.Inventory, error) {
	return a.RenameSource(repoID, alias)
}

func (a *App) PullSource(sourceID string) (skillmgr.PullSourceResult, error) {
	a.mu.Lock()
	repository, repoErr := a.findRepositoryConfigLocked(sourceID)
	if repoErr == nil {
		a.mu.Unlock()
		return a.pullRepositoryConfig(repository)
	}
	source, err := a.findSourceConfigLocked(sourceID)
	a.mu.Unlock()
	if err != nil {
		return skillmgr.PullSourceResult{}, err
	}

	baseCtx := a.ctx
	if baseCtx == nil {
		baseCtx = context.Background()
	}
	ctx, cancel := context.WithTimeout(baseCtx, 2*time.Minute)
	defer cancel()
	message, err := a.service.PullSource(ctx, source)
	if err != nil {
		return skillmgr.PullSourceResult{}, err
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	if err := a.refreshLocked(a.ctx); err != nil {
		return skillmgr.PullSourceResult{}, err
	}
	return skillmgr.PullSourceResult{Inventory: a.inventory, Message: message}, nil
}

func (a *App) PullRepository(repoID string) (skillmgr.PullSourceResult, error) {
	a.mu.Lock()
	repository, err := a.findRepositoryConfigLocked(repoID)
	a.mu.Unlock()
	if err != nil {
		return skillmgr.PullSourceResult{}, err
	}
	return a.pullRepositoryConfig(repository)
}

func (a *App) pullRepositoryConfig(repository skillmgr.RepositoryConfig) (skillmgr.PullSourceResult, error) {
	baseCtx := a.ctx
	if baseCtx == nil {
		baseCtx = context.Background()
	}
	ctx, cancel := context.WithTimeout(baseCtx, 2*time.Minute)
	defer cancel()
	message, err := a.service.PullRepository(ctx, repository)
	if err != nil {
		return skillmgr.PullSourceResult{}, err
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	if err := a.refreshLocked(a.ctx); err != nil {
		return skillmgr.PullSourceResult{}, err
	}
	return skillmgr.PullSourceResult{Inventory: a.inventory, Message: message}, nil
}

func (a *App) SaveConfig(config skillmgr.Config) (skillmgr.Inventory, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if strings.TrimSpace(config.Sync.Folder) == "" {
		return skillmgr.Inventory{}, errors.New("sync folder is required")
	}
	a.config = config
	if err := a.persistAndRefreshLocked(); err != nil {
		return skillmgr.Inventory{}, err
	}
	return a.inventory, nil
}

func (a *App) BrowseForSource() (string, error) {
	return wailsRuntime.OpenDirectoryDialog(a.ctx, wailsRuntime.OpenDialogOptions{
		Title: "Add Repository or Local Folder",
	})
}

func (a *App) BrowseForTarget() (string, error) {
	return wailsRuntime.OpenDirectoryDialog(a.ctx, wailsRuntime.OpenDialogOptions{
		Title: "Add Target Skill Directory",
	})
}

func (a *App) BrowseForSyncFolder() (string, error) {
	return wailsRuntime.OpenDirectoryDialog(a.ctx, wailsRuntime.OpenDialogOptions{
		Title: "Choose iCloud Sync Folder",
	})
}

func (a *App) BrowseForCloneParent() (string, error) {
	return wailsRuntime.OpenDirectoryDialog(a.ctx, wailsRuntime.OpenDialogOptions{
		Title: "Choose Clone Parent Folder",
	})
}

func (a *App) BrowseForExistingRepository() (string, error) {
	return wailsRuntime.OpenDirectoryDialog(a.ctx, wailsRuntime.OpenDialogOptions{
		Title: "Choose Existing Repository",
	})
}

func (a *App) UseExistingRepository(expectedRepoID string, path string) (skillmgr.Inventory, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if strings.TrimSpace(expectedRepoID) == "" {
		return skillmgr.Inventory{}, errors.New("expected repository ID is required")
	}
	if strings.TrimSpace(path) == "" {
		return skillmgr.Inventory{}, errors.New("repository path is required")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return skillmgr.Inventory{}, err
	}
	provider, _ := skillmgr.ProviderFor(skillmgr.GitProvider)
	installation, remote, err := provider.Inspect(a.ctx, abs)
	if err != nil {
		return skillmgr.Inventory{}, err
	}
	if installation.SourceID != expectedRepoID {
		return skillmgr.Inventory{}, fmt.Errorf("selected repository is %s, expected %s", installation.SourceID, expectedRepoID)
	}
	repository := skillmgr.RepositoryConfig{
		ID:        installation.SourceID,
		RepoID:    installation.SourceID,
		Path:      installation.Path,
		Enabled:   true,
		CloneURL:  remote,
		ScanRoots: append([]string(nil), installation.Options.ScanRoots...),
	}
	replaced := false
	for index := range a.config.Repositories {
		if a.config.Repositories[index].RepoID == expectedRepoID {
			repository.Alias = a.config.Repositories[index].Alias
			a.config.Repositories[index] = repository
			replaced = true
			break
		}
	}
	if !replaced {
		a.config.Repositories = append(a.config.Repositories, repository)
	}
	if err := a.persistAndRefreshLocked(); err != nil {
		return skillmgr.Inventory{}, err
	}
	return a.inventory, nil
}

func (a *App) CloneRepository(repoID string, cloneURL string, parentDir string, folderName string) (skillmgr.CloneRepositoryResult, error) {
	baseCtx := a.ctx
	if baseCtx == nil {
		baseCtx = context.Background()
	}
	ctx, cancel := context.WithTimeout(baseCtx, 5*time.Minute)
	defer cancel()
	path, message, err := a.service.CloneRepository(ctx, cloneURL, parentDir, folderName)
	if err != nil {
		return skillmgr.CloneRepositoryResult{}, err
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	repository, ok := a.repositoryConfigFromPathLocked(ctx, path)
	if !ok {
		return skillmgr.CloneRepositoryResult{}, fmt.Errorf("cloned folder is not a usable git repository: %s", path)
	}
	if repoID != "" && repository.RepoID != repoID {
		return skillmgr.CloneRepositoryResult{}, fmt.Errorf("cloned repository is %s, expected %s", repository.RepoID, repoID)
	}
	replaced := false
	for i := range a.config.Repositories {
		if a.config.Repositories[i].RepoID == repository.RepoID {
			a.config.Repositories[i] = repository
			replaced = true
			break
		}
	}
	if !replaced {
		a.config.Repositories = append(a.config.Repositories, repository)
	}
	if err := a.persistAndRefreshLocked(); err != nil {
		return skillmgr.CloneRepositoryResult{}, err
	}
	return skillmgr.CloneRepositoryResult{Inventory: a.inventory, Message: message}, nil
}

func (a *App) EnableSkill(skillID string) (skillmgr.Inventory, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	skill, err := a.findSkillLocked(skillID)
	if err != nil {
		return skillmgr.Inventory{}, err
	}
	store := a.currentSyncStoreLocked()
	if store == nil || !skill.IsSynced {
		return skillmgr.Inventory{}, errors.New("skill is not available in the shared catalog")
	}
	record := syncRecordForSkill(skill, true)
	if err := store.UpsertSkill(record); err != nil {
		return skillmgr.Inventory{}, err
	}
	if err := a.service.Enable(a.ctx, a.config, skill); err != nil {
		return skillmgr.Inventory{}, err
	}
	if err := a.refreshLocked(a.ctx); err != nil {
		return skillmgr.Inventory{}, err
	}
	return a.inventory, nil
}

func (a *App) EnableSkills(skillIDs []string) (skillmgr.BulkEnableResult, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	store := a.currentSyncStoreLocked()
	if store == nil {
		return skillmgr.BulkEnableResult{}, errors.New("sync folder is not configured")
	}
	result := skillmgr.BulkEnableResult{}
	records := make([]skillmgr.SyncSkillRecord, 0, len(skillIDs))
	for _, skillID := range uniqueSkillIDs(skillIDs) {
		skill, err := a.findSkillLocked(skillID)
		if err != nil {
			result.Skipped++
			result.Failed = append(result.Failed, skillID+": "+err.Error())
			continue
		}
		if skill.IsActive {
			result.AlreadyEnabled++
			continue
		}
		if !bulkEnableEligible(skill) {
			result.Skipped++
			continue
		}
		if err := a.service.Enable(a.ctx, a.config, skill); err != nil {
			result.Skipped++
			result.Failed = append(result.Failed, skill.Name+": "+err.Error())
			continue
		}
		records = append(records, syncRecordForSkill(skill, true))
		result.Enabled++
	}
	if len(records) > 0 {
		if err := store.UpsertSkills(records); err != nil {
			return skillmgr.BulkEnableResult{}, err
		}
	}
	if err := a.refreshLocked(a.ctx); err != nil {
		return skillmgr.BulkEnableResult{}, err
	}
	result.Inventory = a.inventory
	return result, nil
}

func (a *App) SaveLLMConfig(config skillmgr.SyncLLMConfig) (skillmgr.Inventory, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	store := a.currentSyncStoreLocked()
	if store == nil {
		return skillmgr.Inventory{}, errors.New("sync folder is not configured")
	}
	if err := store.SaveLLMConfig(config); err != nil {
		return skillmgr.Inventory{}, err
	}
	if err := a.refreshLocked(a.ctx); err != nil {
		return skillmgr.Inventory{}, err
	}
	return a.inventory, nil
}

func (a *App) GenerateSkillProfile(skillID string, force bool) (skillmgr.SkillProfileResult, error) {
	a.mu.Lock()
	skill, err := a.findSkillLocked(skillID)
	if err != nil {
		a.mu.Unlock()
		return skillmgr.SkillProfileResult{}, err
	}
	if skill.SyncID == "" {
		a.mu.Unlock()
		return skillmgr.SkillProfileResult{}, errors.New("skill does not have a sync identity")
	}
	store := a.currentSyncStoreLocked()
	if store == nil {
		a.mu.Unlock()
		return skillmgr.SkillProfileResult{}, errors.New("sync folder is not configured")
	}
	document, err := store.Load()
	if err != nil {
		a.mu.Unlock()
		return skillmgr.SkillProfileResult{}, err
	}
	llmConfig := document.LLM
	if !force {
		if skill.Profile != nil && skill.Profile.SummaryZh != "" && len(skill.Profile.UseCasesZh) > 0 {
			result := skillmgr.SkillProfileResult{
				Inventory: a.inventory,
				Profile:   cloneSkillProfileForApp(skill.Profile),
				Generated: false,
				Message:   "Profile loaded from sync cache.",
			}
			a.mu.Unlock()
			return result, nil
		}
		if profile, ok := document.Profiles[skill.SyncID]; ok && profile.SummaryZh != "" && len(profile.UseCasesZh) > 0 {
			result := skillmgr.SkillProfileResult{
				Inventory: a.inventory,
				Profile:   cloneSkillProfileForApp(&profile),
				Generated: false,
				Message:   "Profile loaded from sync cache.",
			}
			a.mu.Unlock()
			return result, nil
		}
	}
	a.mu.Unlock()

	sourceText, err := skillmgr.SkillSourceText(a.ctx, skill)
	if err != nil {
		return skillmgr.SkillProfileResult{}, err
	}
	ctx, cancel := context.WithTimeout(a.ctx, 90*time.Second)
	defer cancel()
	profile, err := skillmgr.GenerateSkillProfile(ctx, skill, llmConfig, sourceText)
	if err != nil {
		return skillmgr.SkillProfileResult{}, err
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	store = a.currentSyncStoreLocked()
	if store == nil {
		return skillmgr.SkillProfileResult{}, errors.New("sync folder is not configured")
	}
	if err := store.UpsertSkillProfile(skill.SyncID, *profile); err != nil {
		return skillmgr.SkillProfileResult{}, err
	}
	if err := a.refreshLocked(a.ctx); err != nil {
		return skillmgr.SkillProfileResult{}, err
	}
	return skillmgr.SkillProfileResult{
		Inventory: a.inventory,
		Profile:   cloneSkillProfileForApp(profile),
		Generated: true,
		Message:   "Profile generated.",
	}, nil
}

func (a *App) DisableSkill(skillID string) (skillmgr.Inventory, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	skill, err := a.findSkillLocked(skillID)
	if err != nil {
		return skillmgr.Inventory{}, err
	}
	store := a.currentSyncStoreLocked()
	if store == nil || !skill.IsSynced {
		return skillmgr.Inventory{}, errors.New("skill is not available in the shared catalog")
	}
	if err := store.UpsertSkill(syncRecordForSkill(skill, false)); err != nil {
		return skillmgr.Inventory{}, err
	}
	if err := a.service.Disable(a.ctx, a.config, skill); err != nil {
		return skillmgr.Inventory{}, err
	}
	if err := a.refreshLocked(a.ctx); err != nil {
		return skillmgr.Inventory{}, err
	}
	return a.inventory, nil
}

func (a *App) DisableSkills(skillIDs []string) (skillmgr.BulkDisableResult, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	store := a.currentSyncStoreLocked()
	if store == nil {
		return skillmgr.BulkDisableResult{}, errors.New("sync folder is not configured")
	}
	result := skillmgr.BulkDisableResult{}
	records := make([]skillmgr.SyncSkillRecord, 0, len(skillIDs))
	for _, skillID := range uniqueSkillIDs(skillIDs) {
		skill, err := a.findSkillLocked(skillID)
		if err != nil {
			result.Skipped++
			result.Failed = append(result.Failed, skillID+": "+err.Error())
			continue
		}
		if !skill.IsActive {
			result.AlreadyDisabled++
			continue
		}
		if !skill.IsSynced || skill.SourcePath == "" {
			result.Skipped++
			continue
		}
		if err := a.service.Disable(a.ctx, a.config, skill); err != nil {
			result.Skipped++
			result.Failed = append(result.Failed, skill.Name+": "+err.Error())
			continue
		}
		records = append(records, syncRecordForSkill(skill, false))
		result.Disabled++
	}
	if len(records) > 0 {
		if err := store.UpsertSkills(records); err != nil {
			return skillmgr.BulkDisableResult{}, err
		}
	}
	if err := a.refreshLocked(a.ctx); err != nil {
		return skillmgr.BulkDisableResult{}, err
	}
	result.Inventory = a.inventory
	return result, nil
}

func (a *App) ResolveConflict(skillID string) (skillmgr.Inventory, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	skill, err := a.findSkillLocked(skillID)
	if err != nil {
		return skillmgr.Inventory{}, err
	}
	store := a.currentSyncStoreLocked()
	if store == nil || !skill.IsSynced {
		return skillmgr.Inventory{}, errors.New("skill is not available in the shared catalog")
	}
	if err := store.UpsertSkill(syncRecordForSkill(skill, true)); err != nil {
		return skillmgr.Inventory{}, err
	}
	if err := a.service.ResolveConflict(a.ctx, a.config, skill); err != nil {
		return skillmgr.Inventory{}, err
	}
	if err := a.refreshLocked(a.ctx); err != nil {
		return skillmgr.Inventory{}, err
	}
	return a.inventory, nil
}

func (a *App) ReadSkillEnvFile(skillID string) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	skill, err := a.findSkillLocked(skillID)
	if err != nil {
		return "", err
	}
	return a.service.ReadEnvFile(skill)
}

func (a *App) SaveSkillEnvFile(skillID string, content string) (skillmgr.Inventory, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	skill, err := a.findSkillLocked(skillID)
	if err != nil {
		return skillmgr.Inventory{}, err
	}
	if err := a.service.SaveEnvFile(skill, content); err != nil {
		return skillmgr.Inventory{}, err
	}
	if err := a.refreshLocked(a.ctx); err != nil {
		return skillmgr.Inventory{}, err
	}
	return a.inventory, nil
}

func (a *App) SaveSkillTags(skillID string, tags []string) (skillmgr.Inventory, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	skill, err := a.findSkillLocked(skillID)
	if err != nil {
		return skillmgr.Inventory{}, err
	}
	if !skill.IsSynced {
		return skillmgr.Inventory{}, errors.New("skill is not available in the shared catalog")
	}
	store := a.currentSyncStoreLocked()
	if store == nil {
		return skillmgr.Inventory{}, errors.New("sync folder is not configured")
	}
	record := syncRecordForSkill(skill, skill.DesiredEnabled != nil && *skill.DesiredEnabled)
	record.Tags = tags
	if err := store.UpsertSkill(record); err != nil {
		return skillmgr.Inventory{}, err
	}
	if err := a.refreshLocked(a.ctx); err != nil {
		return skillmgr.Inventory{}, err
	}
	return a.inventory, nil
}

func (a *App) AddSkillTags(skillIDs []string, tags []string) (skillmgr.BulkTagResult, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	cleanTags := mergeSkillTags(nil, tags)
	if len(cleanTags) == 0 {
		return skillmgr.BulkTagResult{}, errors.New("at least one tag is required")
	}
	store := a.currentSyncStoreLocked()
	if store == nil {
		return skillmgr.BulkTagResult{}, errors.New("sync folder is not configured")
	}
	result := skillmgr.BulkTagResult{}
	records := make([]skillmgr.SyncSkillRecord, 0, len(skillIDs))
	for _, skillID := range uniqueSkillIDs(skillIDs) {
		skill, err := a.findSkillLocked(skillID)
		if err != nil {
			result.Skipped++
			result.Failed = append(result.Failed, skillID+": "+err.Error())
			continue
		}
		if !skill.IsSynced {
			result.Skipped++
			continue
		}
		merged := mergeSkillTags(skill.Tags, cleanTags)
		if stringSlicesEqual(merged, mergeSkillTags(nil, skill.Tags)) {
			result.Unchanged++
			continue
		}
		record := syncRecordForSkill(skill, skill.DesiredEnabled != nil && *skill.DesiredEnabled)
		record.Tags = merged
		records = append(records, record)
		result.Updated++
	}
	if len(records) > 0 {
		if err := store.UpsertSkills(records); err != nil {
			return skillmgr.BulkTagResult{}, err
		}
	}
	if err := a.refreshLocked(a.ctx); err != nil {
		return skillmgr.BulkTagResult{}, err
	}
	result.Inventory = a.inventory
	return result, nil
}

func bulkEnableEligible(skill skillmgr.Skill) bool {
	if !skill.IsSynced || skill.SourcePath == "" {
		return false
	}
	switch skill.Status {
	case skillmgr.StatusConflict, skillmgr.StatusInvalid, skillmgr.StatusMissingSource, skillmgr.StatusMissingPath, skillmgr.StatusError:
		return false
	default:
		return true
	}
}

func uniqueSkillIDs(skillIDs []string) []string {
	seen := make(map[string]bool, len(skillIDs))
	unique := make([]string, 0, len(skillIDs))
	for _, skillID := range skillIDs {
		skillID = strings.TrimSpace(skillID)
		if skillID == "" || seen[skillID] {
			continue
		}
		seen[skillID] = true
		unique = append(unique, skillID)
	}
	return unique
}

func mergeSkillTags(existing, additions []string) []string {
	seen := make(map[string]bool, len(existing)+len(additions))
	merged := make([]string, 0, len(existing)+len(additions))
	for _, values := range [][]string{existing, additions} {
		for _, tag := range values {
			tag = strings.TrimSpace(tag)
			if tag == "" || seen[tag] {
				continue
			}
			seen[tag] = true
			merged = append(merged, tag)
		}
	}
	sort.Strings(merged)
	return merged
}

func stringSlicesEqual(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func (a *App) OpenPath(path string) error {
	if path == "" {
		return errors.New("path is required")
	}
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", path).Start()
	case "windows":
		return exec.Command("explorer", path).Start()
	default:
		return exec.Command("xdg-open", path).Start()
	}
}

func (a *App) OpenInVSCode(path string) error {
	if path == "" {
		return errors.New("path is required")
	}
	if runtime.GOOS == "darwin" {
		if err := exec.Command("open", "-b", "com.microsoft.VSCode", path).Run(); err == nil {
			return nil
		}
	}
	if _, err := exec.LookPath("code"); err != nil {
		return errors.New("VS Code command not found")
	}
	return exec.Command("code", path).Start()
}

func (a *App) OpenInTerminal(path string) error {
	if err := validateTerminalDirectory(path); err != nil {
		return err
	}
	if runtime.GOOS != "darwin" {
		return errors.New("Terminal.app is only available on macOS")
	}
	if err := exec.Command("open", "-b", "com.apple.Terminal", path).Run(); err != nil {
		return fmt.Errorf("open Terminal.app: %w", err)
	}
	return nil
}

func validateTerminalDirectory(path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return errors.New("terminal directory is required")
	}
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("terminal directory is unavailable: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("terminal path is not a directory: %s", path)
	}
	return nil
}

func (a *App) refreshLocked(ctx context.Context) error {
	startedAt := time.Now()
	a.debugLogf("refresh begin repositories=%d sources=%d", len(a.config.Repositories), len(a.config.Sources))
	syncStore := a.currentSyncStoreLocked()
	if syncStore == nil {
		inventory, err := a.service.Scan(ctx, a.config)
		if err != nil {
			return err
		}
		a.config = inventory.Config
		a.inventory = inventory
		a.inventory.SyncConfigured = false
		return nil
	}
	a.debugLogf("sync load begin path=%q", syncStore.Path())
	syncDocument, syncErr := syncStore.Load()
	if syncErr != nil {
		a.debugLogf("sync load error path=%q error=%v", syncStore.Path(), syncErr)
		inventory, scanErr := a.service.Scan(ctx, a.config)
		if scanErr != nil {
			return scanErr
		}
		a.config = inventory.Config
		a.inventory = inventory
		a.inventory.SyncConfigured = true
		a.inventory.SyncPath = syncStore.Path()
		a.inventory.SyncError = syncErr.Error()
		return nil
	}
	if _, err := os.Stat(syncStore.Path()); errors.Is(err, os.ErrNotExist) {
		if err := syncStore.Save(syncDocument); err != nil {
			return err
		}
	}
	a.debugLogf("sync load done path=%q records=%d", syncStore.Path(), len(syncDocument.Skills))
	inventory, err := a.service.ScanWithSync(ctx, a.config, syncDocument)
	if err != nil {
		a.debugLogf("refresh scan error: %v duration=%s", err, time.Since(startedAt))
		return err
	}
	seeded := false
	for _, skill := range inventory.Skills {
		if !skill.CanSync || skill.SyncID == "" {
			continue
		}
		if _, exists := syncDocument.Skills[skill.SyncID]; exists {
			continue
		}
		record := syncRecordForSkill(skill, skill.IsActive)
		record.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
		syncDocument.Skills[skill.SyncID] = record
		seeded = true
	}
	if seeded {
		if err := syncStore.Save(syncDocument); err != nil {
			return err
		}
		inventory, err = a.service.ScanWithSync(ctx, a.config, syncDocument)
		if err != nil {
			return err
		}
	}
	reconcileErrors := map[string]string{}
	reconciled := false
	for _, skill := range inventory.Skills {
		if !skill.IsSynced || skill.DesiredEnabled == nil {
			continue
		}
		if skill.Status == skillmgr.StatusConflict || skill.Status == skillmgr.StatusInvalid ||
			skill.Status == skillmgr.StatusMissingSource || skill.Status == skillmgr.StatusMissingPath || skill.Status == skillmgr.StatusError {
			continue
		}
		if *skill.DesiredEnabled && !skill.IsActive {
			if err := a.service.Enable(ctx, a.config, skill); err != nil {
				reconcileErrors[skill.ID] = err.Error()
			} else {
				reconciled = true
			}
		} else if !*skill.DesiredEnabled && skill.IsActive {
			if err := a.disableSyncedSkillLocked(skill); err != nil {
				reconcileErrors[skill.ID] = err.Error()
			} else {
				reconciled = true
			}
		}
	}
	if reconciled || len(reconcileErrors) > 0 {
		inventory, err = a.service.ScanWithSync(ctx, a.config, syncDocument)
		if err != nil {
			return err
		}
		for index := range inventory.Skills {
			if message := reconcileErrors[inventory.Skills[index].ID]; message != "" {
				inventory.Skills[index].Status = skillmgr.StatusError
				inventory.Skills[index].Error = message
			}
		}
	}
	a.config = inventory.Config
	a.inventory = inventory
	a.inventory.SyncPath = syncStore.Path()
	a.inventory.LLMConfig = syncDocument.LLM
	a.debugLogf("refresh done skills=%d repositories=%d sources=%d duration=%s", len(a.inventory.Skills), len(a.inventory.Repositories), len(a.inventory.Sources), time.Since(startedAt))
	return nil
}

func (a *App) persistAndRefreshLocked() error {
	if err := a.store.Save(a.config); err != nil {
		return err
	}
	if err := a.refreshLocked(a.ctx); err != nil {
		return err
	}
	if a.config.Scan.WatchSourceFolders {
		if err := a.restartWatcherLocked(); err != nil {
			return err
		}
	} else if a.watcher != nil {
		_ = a.watcher.Close()
		a.watcher = nil
	}
	return nil
}

func (a *App) findSkillLocked(skillID string) (skillmgr.Skill, error) {
	for _, skill := range a.inventory.Skills {
		if skill.ID == skillID {
			return skill, nil
		}
	}
	return skillmgr.Skill{}, fmt.Errorf("skill not found: %s", skillID)
}

func (a *App) findSourceConfigLocked(sourceID string) (skillmgr.SkillSourceConfig, error) {
	for _, source := range a.config.Sources {
		if source.ID == sourceID {
			return source, nil
		}
	}
	return skillmgr.SkillSourceConfig{}, fmt.Errorf("source not found: %s", sourceID)
}

func (a *App) findRepositoryConfigLocked(repoID string) (skillmgr.RepositoryConfig, error) {
	for _, repository := range a.config.Repositories {
		if repository.ID == repoID || repository.RepoID == repoID {
			return repository, nil
		}
	}
	return skillmgr.RepositoryConfig{}, fmt.Errorf("repository not found: %s", repoID)
}

func (a *App) currentSyncStoreLocked() *skillmgr.SyncStore {
	path := skillmgr.SyncPathFromFolder(a.config.Sync.Folder)
	if path == "" {
		return nil
	}
	return skillmgr.NewSyncStore(path)
}

func (a *App) repositoryConfigFromPathLocked(ctx context.Context, path string) (skillmgr.RepositoryConfig, bool) {
	if ctx == nil {
		ctx = context.Background()
	}
	provider, _ := skillmgr.ProviderFor(skillmgr.GitProvider)
	installation, remote, err := provider.Inspect(ctx, path)
	if err != nil {
		return skillmgr.RepositoryConfig{}, false
	}
	return skillmgr.RepositoryConfig{
		ID:        installation.SourceID,
		RepoID:    installation.SourceID,
		Path:      installation.Path,
		Enabled:   installation.Enabled,
		CloneURL:  remote,
		ScanRoots: append([]string(nil), installation.Options.ScanRoots...),
	}, true
}

func syncRecordForSkill(skill skillmgr.Skill, enabled bool) skillmgr.SyncSkillRecord {
	targetName := strings.TrimSpace(skill.TargetName)
	if targetName == "" {
		targetName = skill.Name
	}
	return skillmgr.SyncSkillRecord{
		Enabled:             enabled,
		TargetName:          targetName,
		PreviousTargetNames: append([]string(nil), skill.PreviousTargetNames...),
		Tags:                append([]string(nil), skill.Tags...),
		Profile:             cloneSkillProfileForApp(skill.Profile),
		Source: skillmgr.SyncSource{
			Provider: skillmgr.GitProvider,
			ID:       skill.RepoID,
			Locator: skillmgr.SourceLocator{
				CloneURL: skill.CloneURL,
				Subpath:  skill.RepoSubpath,
				Ref:      skill.Ref,
			},
		},
	}
}

func cloneSkillProfileForApp(profile *skillmgr.SkillProfile) *skillmgr.SkillProfile {
	if profile == nil {
		return nil
	}
	cloned := *profile
	cloned.UseCasesZh = append([]string(nil), profile.UseCasesZh...)
	return &cloned
}

func (a *App) disableSyncedSkillLocked(skill skillmgr.Skill) error {
	for _, previousTargetName := range skill.PreviousTargetNames {
		previous := skill
		previous.Name = previousTargetName
		previous.TargetName = previousTargetName
		_, _, _ = skillmgr.DisableInTargetForApp(a.config.TargetDirs, previous)
	}
	return a.service.Disable(a.ctx, a.config, skill)
}

func (a *App) restartWatcherLocked() error {
	a.debugLogf("watcher restart begin")
	if a.watcher != nil {
		_ = a.watcher.Close()
		a.watcher = nil
	}
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	for _, source := range a.config.Sources {
		if source.Enabled {
			if err := watcher.Add(source.Path); err != nil {
				a.debugLogf("watcher add source error path=%q error=%v", source.Path, err)
			} else {
				a.debugLogf("watcher add source path=%q", source.Path)
			}
		}
	}
	for _, repository := range a.config.Repositories {
		if repository.Enabled {
			if err := watcher.Add(repository.Path); err != nil {
				a.debugLogf("watcher add repository error repo_id=%q path=%q error=%v", repository.RepoID, repository.Path, err)
			} else {
				a.debugLogf("watcher add repository repo_id=%q path=%q", repository.RepoID, repository.Path)
			}
		}
	}
	if a.config.Sync.Folder != "" {
		if err := os.MkdirAll(a.config.Sync.Folder, 0o755); err != nil {
			_ = watcher.Close()
			return err
		}
		if err := watcher.Add(a.config.Sync.Folder); err != nil {
			a.debugLogf("watcher add sync folder error path=%q error=%v", a.config.Sync.Folder, err)
		} else {
			a.debugLogf("watcher add sync folder path=%q", a.config.Sync.Folder)
		}
	}
	a.watcher = watcher
	go a.watchLoop(watcher)
	a.debugLogf("watcher restart done")
	return nil
}

func (a *App) watchLoop(watcher *fsnotify.Watcher) {
	const debounceDelay = 900 * time.Millisecond
	var timer *time.Timer
	var timerC <-chan time.Time
	scheduleRefresh := func(event fsnotify.Event) {
		if event.Op == fsnotify.Chmod {
			return
		}
		a.debugLogf("watcher event name=%q op=%s", event.Name, event.Op.String())
		if timer == nil {
			timer = time.NewTimer(debounceDelay)
			timerC = timer.C
			return
		}
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		timer.Reset(debounceDelay)
	}
	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			scheduleRefresh(event)
		case <-timerC:
			timer = nil
			timerC = nil
			a.mu.Lock()
			if watcher == a.watcher {
				a.debugLogf("watcher refresh begin")
				if err := a.refreshLocked(a.ctx); err != nil {
					a.debugLogf("watcher refresh error: %v", err)
				} else {
					a.debugLogf("watcher refresh emit skills=%d repositories=%d", len(a.inventory.Skills), len(a.inventory.Repositories))
					wailsRuntime.EventsEmit(a.ctx, "inventory:changed", a.inventory)
				}
			}
			a.mu.Unlock()
		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			a.debugLogf("watcher error: %v", err)
			fmt.Println("watcher:", err)
		}
	}
}
