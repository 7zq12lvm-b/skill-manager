package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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
	tagStore  *skillmgr.SkillTagStore
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
	tagPath, err := skillmgr.DefaultSkillTagPath()
	if err != nil {
		tagPath = filepath.Join(".", "tags.json")
	}
	app := &App{
		store:    skillmgr.NewConfigStore(configPath),
		tagStore: skillmgr.NewSkillTagStore(tagPath),
		service:  skillmgr.NewService(),
		logPath:  defaultDebugLogPath(),
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
	if a.migrateSourcesToRepositoriesLocked(ctx) {
		a.debugLogf("migrated legacy sources repositories=%d remaining_sources=%d", len(a.config.Repositories), len(a.config.Sources))
		if err := a.store.Save(a.config); err != nil {
			a.debugLogf("save migrated config error: %v", err)
			fmt.Println("save migrated config:", err)
		}
	}
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
	for _, source := range a.config.Sources {
		if filepath.Clean(source.Path) == filepath.Clean(abs) {
			return a.inventory, nil
		}
	}
	a.config.Sources = append(a.config.Sources, skillmgr.NewSkillSourceConfig(abs))
	if err := a.persistAndRefreshLocked(); err != nil {
		return skillmgr.Inventory{}, err
	}
	return a.inventory, nil
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

func (a *App) ApplySync() (skillmgr.ApplySyncResult, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.config.Sync.Folder == "" {
		return skillmgr.ApplySyncResult{}, errors.New("sync folder is not configured")
	}
	applied, skipped := 0, 0
	for _, skill := range a.inventory.Skills {
		if !skill.IsSynced || skill.DesiredEnabled == nil {
			continue
		}
		if skill.Status == skillmgr.StatusMissingSource || skill.Status == skillmgr.StatusMissingPath ||
			skill.Status == skillmgr.StatusInvalid || skill.Status == skillmgr.StatusError ||
			skill.Status == skillmgr.StatusConflict {
			skipped++
			continue
		}
		if *skill.DesiredEnabled {
			if skill.SourcePath == "" {
				skipped++
				continue
			}
			if err := a.service.Enable(a.ctx, a.config, skill); err != nil {
				skipped++
				continue
			}
			applied++
			continue
		}
		if err := a.disableSyncedSkillLocked(skill); err != nil {
			skipped++
			continue
		}
		applied++
	}
	a.config.Sync.LastAppliedAt = time.Now().Format(time.RFC3339)
	if err := a.persistAndRefreshLocked(); err != nil {
		return skillmgr.ApplySyncResult{}, err
	}
	message := fmt.Sprintf("Applied %d synced changes.", applied)
	if skipped > 0 {
		message = fmt.Sprintf("%s Skipped %d items that need attention.", message, skipped)
	}
	return skillmgr.ApplySyncResult{Inventory: a.inventory, Message: message}, nil
}

func (a *App) AdoptCurrentEnabledSkills() (skillmgr.AdoptSyncResult, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	store := a.currentSyncStoreLocked()
	if store == nil {
		return skillmgr.AdoptSyncResult{}, errors.New("sync folder is not configured")
	}
	document, err := store.Load()
	if err != nil {
		return skillmgr.AdoptSyncResult{}, err
	}
	adopted := 0
	var skipped []string
	now := time.Now().UTC().Format(time.RFC3339)
	for _, skill := range a.inventory.Skills {
		if !skill.IsActive {
			continue
		}
		if !skill.CanSync || skill.RepoID == "" || skill.RepoSubpath == "" {
			skipped = append(skipped, skill.Name)
			continue
		}
		record := syncRecordForSkill(skill, true)
		record.UpdatedAt = now
		document.Skills[skill.SyncID] = record
		adopted++
	}
	if err := store.Save(document); err != nil {
		return skillmgr.AdoptSyncResult{}, err
	}
	a.config.Sync.LastAppliedAt = time.Now().Format(time.RFC3339)
	if err := a.persistAndRefreshLocked(); err != nil {
		return skillmgr.AdoptSyncResult{}, err
	}
	return skillmgr.AdoptSyncResult{Inventory: a.inventory, Adopted: adopted, Skipped: skipped}, nil
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
	if a.config.Sync.Folder != "" && skill.CanSync {
		store := a.currentSyncStoreLocked()
		if store == nil {
			return skillmgr.Inventory{}, errors.New("sync folder is not configured")
		}
		record := syncRecordForSkill(skill, true)
		if err := store.UpsertSkill(record); err != nil {
			return skillmgr.Inventory{}, err
		}
	}
	if err := a.service.Enable(a.ctx, a.config, skill); err != nil {
		return skillmgr.Inventory{}, err
	}
	if err := a.refreshLocked(a.ctx); err != nil {
		return skillmgr.Inventory{}, err
	}
	return a.inventory, nil
}

func (a *App) EnableSkillLocalOnly(skillID string) (skillmgr.Inventory, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	skill, err := a.findSkillLocked(skillID)
	if err != nil {
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

func (a *App) RemoveSkillFromSync(skillID string) (skillmgr.Inventory, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	skill, err := a.findSkillLocked(skillID)
	if err != nil {
		return skillmgr.Inventory{}, err
	}
	if skill.SyncID == "" {
		return a.inventory, nil
	}
	store := a.currentSyncStoreLocked()
	if store == nil {
		return skillmgr.Inventory{}, errors.New("sync folder is not configured")
	}
	if err := store.DeleteSkill(skill.SyncID); err != nil {
		return skillmgr.Inventory{}, err
	}
	if err := a.refreshLocked(a.ctx); err != nil {
		return skillmgr.Inventory{}, err
	}
	return a.inventory, nil
}

func (a *App) DisableSkill(skillID string) (skillmgr.Inventory, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	skill, err := a.findSkillLocked(skillID)
	if err != nil {
		return skillmgr.Inventory{}, err
	}
	if a.config.Sync.Folder != "" && skill.IsSynced {
		store := a.currentSyncStoreLocked()
		if store == nil {
			return skillmgr.Inventory{}, errors.New("sync folder is not configured")
		}
		record := syncRecordForSkill(skill, false)
		if err := store.UpsertSkill(record); err != nil {
			return skillmgr.Inventory{}, err
		}
	}
	if err := a.service.Disable(a.ctx, a.config, skill); err != nil {
		return skillmgr.Inventory{}, err
	}
	if err := a.refreshLocked(a.ctx); err != nil {
		return skillmgr.Inventory{}, err
	}
	return a.inventory, nil
}

func (a *App) ResolveConflict(skillID string) (skillmgr.Inventory, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	skill, err := a.findSkillLocked(skillID)
	if err != nil {
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
	if a.config.Sync.Folder != "" {
		if !skill.IsSynced {
			return skillmgr.Inventory{}, errors.New("tags can only be synced after the skill has been added to sync")
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
	if err := a.tagStore.SetSkillTags(skill.Name, tags); err != nil {
		return skillmgr.Inventory{}, err
	}
	a.applyLegacySkillTagsLocked()
	return a.inventory, nil
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

func (a *App) refreshLocked(ctx context.Context) error {
	startedAt := time.Now()
	a.debugLogf("refresh begin repositories=%d sources=%d", len(a.config.Repositories), len(a.config.Sources))
	var syncDocument skillmgr.SyncDocument
	syncStore := a.currentSyncStoreLocked()
	var syncErr error
	if syncStore != nil {
		a.debugLogf("sync load begin path=%q", syncStore.Path())
		syncDocument, syncErr = syncStore.Load()
		if syncErr != nil {
			a.debugLogf("sync load error path=%q error=%v", syncStore.Path(), syncErr)
			syncDocument = skillmgr.SyncDocument{}
		} else {
			a.debugLogf("sync load done path=%q records=%d", syncStore.Path(), len(syncDocument.Skills))
		}
	}
	inventory, err := a.service.ScanWithSync(ctx, a.config, syncDocument)
	if err != nil {
		a.debugLogf("refresh scan error: %v duration=%s", err, time.Since(startedAt))
		return err
	}
	a.config = inventory.Config
	a.inventory = inventory
	if syncStore != nil {
		a.inventory.SyncPath = syncStore.Path()
	}
	if syncErr != nil {
		a.inventory.SyncError = syncErr.Error()
	}
	a.applyLegacySkillTagsLocked()
	if syncStore != nil {
		a.migrateLegacyTagsToSyncLocked(syncStore)
	}
	a.debugLogf("refresh done skills=%d repositories=%d sources=%d duration=%s", len(a.inventory.Skills), len(a.inventory.Repositories), len(a.inventory.Sources), time.Since(startedAt))
	return nil
}

func (a *App) applyLegacySkillTagsLocked() {
	document, err := a.tagStore.Load()
	if err != nil {
		fmt.Println("load skill tags:", err)
		return
	}
	for index := range a.inventory.Skills {
		if len(a.inventory.Skills[index].Tags) > 0 {
			continue
		}
		a.inventory.Skills[index].Tags = append([]string(nil), document.Skills[a.inventory.Skills[index].Name]...)
	}
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
	gitRoot, ok := skillmgr.GitRepositoryRootForApp(ctx, path)
	if !ok {
		return skillmgr.RepositoryConfig{}, false
	}
	remote, ok := skillmgr.GitRemoteURLForApp(ctx, gitRoot)
	if !ok {
		return skillmgr.RepositoryConfig{}, false
	}
	repoID, ok := skillmgr.CanonicalGitRemoteForApp(remote)
	if !ok {
		return skillmgr.RepositoryConfig{}, false
	}
	return skillmgr.RepositoryConfig{
		ID:        repoID,
		RepoID:    repoID,
		Path:      gitRoot,
		Enabled:   true,
		CloneURL:  remote,
		ScanRoots: []string{"."},
	}, true
}

func (a *App) migrateSourcesToRepositoriesLocked(ctx context.Context) bool {
	if len(a.config.Sources) == 0 {
		return false
	}
	existing := map[string]bool{}
	for _, repository := range a.config.Repositories {
		existing[repository.RepoID] = true
	}
	changed := false
	remaining := a.config.Sources[:0]
	for _, source := range a.config.Sources {
		repository, ok := a.repositoryConfigFromPathLocked(ctx, source.Path)
		if !ok {
			remaining = append(remaining, source)
			continue
		}
		if !existing[repository.RepoID] {
			repository.Alias = source.Alias
			repository.Enabled = source.Enabled
			a.config.Repositories = append(a.config.Repositories, repository)
			existing[repository.RepoID] = true
		}
		changed = true
	}
	a.config.Sources = remaining
	return changed
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
		Source: skillmgr.SyncSource{
			RepoID:      skill.RepoID,
			CloneURL:    skill.CloneURL,
			RepoSubpath: skill.RepoSubpath,
			Ref:         skill.Ref,
		},
	}
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

func (a *App) migrateLegacyTagsToSyncLocked(store *skillmgr.SyncStore) {
	legacy, err := a.tagStore.Load()
	if err != nil || len(legacy.Skills) == 0 {
		return
	}
	document, err := store.Load()
	if err != nil || len(document.Skills) == 0 {
		return
	}
	changed := false
	for id, record := range document.Skills {
		if len(record.Tags) > 0 {
			continue
		}
		tags := legacy.Skills[record.TargetName]
		if len(tags) == 0 {
			continue
		}
		record.Tags = tags
		document.Skills[id] = record
		changed = true
	}
	if !changed {
		return
	}
	if err := store.Save(document); err != nil {
		fmt.Println("migrate legacy tags:", err)
		return
	}
	if err := a.tagStore.Remove(); err != nil {
		fmt.Println("remove legacy tags:", err)
		return
	}
	a.inventory.LegacyTagMessage = "Legacy tags were migrated into the sync file."
	for index := range a.inventory.Skills {
		if record, ok := document.Skills[a.inventory.Skills[index].SyncID]; ok {
			a.inventory.Skills[index].Tags = append([]string(nil), record.Tags...)
		}
	}
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
