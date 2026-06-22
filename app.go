package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"

	"skill-manager/internal/skillmgr"

	"github.com/fsnotify/fsnotify"
	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

type App struct {
	ctx       context.Context
	store     *skillmgr.ConfigStore
	service   *skillmgr.Service
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
	return &App{
		store:   skillmgr.NewConfigStore(configPath),
		service: skillmgr.NewService(),
	}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	config, err := a.store.Load()
	if err != nil {
		fmt.Println("load config:", err)
		config = skillmgr.DefaultConfig()
	}
	a.config = config
	if err := a.refreshLocked(ctx); err != nil {
		fmt.Println("initial scan:", err)
	}
	if config.Scan.WatchSourceFolders {
		if err := a.restartWatcherLocked(); err != nil {
			fmt.Println("start watcher:", err)
		}
	}
}

func (a *App) shutdown(ctx context.Context) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.watcher != nil {
		_ = a.watcher.Close()
		a.watcher = nil
	}
}

func (a *App) GetInventory() (skillmgr.Inventory, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.inventory.Skills == nil && a.inventory.Sources == nil {
		if err := a.refreshLocked(a.ctx); err != nil {
			return skillmgr.Inventory{}, err
		}
	}
	return a.inventory, nil
}

func (a *App) RescanAll() (skillmgr.Inventory, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := a.refreshLocked(a.ctx); err != nil {
		return skillmgr.Inventory{}, err
	}
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

func (a *App) RemoveSource(sourceID string) (skillmgr.Inventory, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
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

func (a *App) RenameSource(sourceID string, alias string) (skillmgr.Inventory, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
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
		Title: "Add Skill Source",
	})
}

func (a *App) BrowseForTarget() (string, error) {
	return wailsRuntime.OpenDirectoryDialog(a.ctx, wailsRuntime.OpenDialogOptions{
		Title: "Add Target Skill Directory",
	})
}

func (a *App) EnableSkill(skillID string) (skillmgr.Inventory, error) {
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

func (a *App) DisableSkill(skillID string) (skillmgr.Inventory, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	skill, err := a.findSkillLocked(skillID)
	if err != nil {
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
	inventory, err := a.service.Scan(ctx, a.config)
	if err != nil {
		return err
	}
	a.config = inventory.Config
	a.inventory = inventory
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

func (a *App) restartWatcherLocked() error {
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
			_ = watcher.Add(source.Path)
		}
	}
	a.watcher = watcher
	go a.watchLoop(watcher)
	return nil
}

func (a *App) watchLoop(watcher *fsnotify.Watcher) {
	for {
		select {
		case _, ok := <-watcher.Events:
			if !ok {
				return
			}
			a.mu.Lock()
			if watcher == a.watcher {
				_ = a.refreshLocked(a.ctx)
				wailsRuntime.EventsEmit(a.ctx, "inventory:changed", a.inventory)
			}
			a.mu.Unlock()
		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			fmt.Println("watcher:", err)
		}
	}
}
