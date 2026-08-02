//go:build darwin

package main

import (
	_ "embed"
	"sort"
	"sync"

	"fyne.io/systray"
	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
	skillmgr "skill-manager/internal"
)

//go:embed build/appicon.png
var trayIcon []byte

type traySkillItem struct {
	item *systray.MenuItem
	stop chan struct{}
}

type trayRepoMenu struct {
	menu     *systray.MenuItem
	children []traySkillItem
}

var trayState struct {
	sync.RWMutex
	app     *App
	end     func()
	started bool

	menuMu        sync.Mutex
	menuReady     bool
	repoMenus     []trayRepoMenu
	pendingSkills []skillmgr.Skill
	pendingRepos  []skillmgr.Repository
}

func setTrayApp(app *App) {
	trayState.Lock()
	trayState.app = app
	trayState.Unlock()
}

func trayApp() *App {
	trayState.RLock()
	defer trayState.RUnlock()
	return trayState.app
}

func startSystemTray() {
	trayState.Lock()
	if trayState.started {
		trayState.Unlock()
		return
	}

	start, end := systray.RunWithExternalLoop(func() {
		systray.SetIcon(trayIcon)
		systray.SetTitle("SM")
		systray.SetTooltip("Skill Manager")

		showItem := systray.AddMenuItem("显示 Skill Manager", "显示 Skill Manager 窗口")
		go func() {
			for range showItem.ClickedCh {
				if app := trayApp(); app != nil {
					wailsRuntime.Show(app.ctx)
				}
			}
		}()

		searchItem := systray.AddMenuItem("搜索全部 Skill…", "打开全部 Skill 搜索并快速启用")
		go func() {
			for range searchItem.ClickedCh {
				if app := trayApp(); app != nil {
					wailsRuntime.Show(app.ctx)
					wailsRuntime.EventsEmit(app.ctx, "tray:search")
				}
			}
		}()

		trayState.menuMu.Lock()
		trayState.menuReady = true
		pendingSkills := append([]skillmgr.Skill(nil), trayState.pendingSkills...)
		pendingRepos := append([]skillmgr.Repository(nil), trayState.pendingRepos...)
		trayState.pendingSkills = nil
		trayState.pendingRepos = nil
		trayState.menuMu.Unlock()
		updateTrayMenus(pendingSkills, pendingRepos)

		systray.AddSeparator()
		quitItem := systray.AddMenuItem("退出 Skill Manager", "退出 Skill Manager")
		go func() {
			for range quitItem.ClickedCh {
				if app := trayApp(); app != nil {
					wailsRuntime.Quit(app.ctx)
				}
			}
		}()
	}, func() {})
	trayState.end = end
	trayState.started = true
	trayState.Unlock()

	start()
}

func updateSystemTrayInventory(inventory skillmgr.Inventory) {
	skills := append([]skillmgr.Skill(nil), inventory.Skills...)
	sort.SliceStable(skills, func(i, j int) bool {
		return traySkillLabel(skills[i]) < traySkillLabel(skills[j])
	})
	updateTrayMenus(skills, inventory.Repositories)
}

func updateTrayMenus(skills []skillmgr.Skill, repositories []skillmgr.Repository) {
	trayState.menuMu.Lock()
	defer trayState.menuMu.Unlock()
	if !trayState.menuReady {
		trayState.pendingSkills = append([]skillmgr.Skill(nil), skills...)
		trayState.pendingRepos = append([]skillmgr.Repository(nil), repositories...)
		return
	}

	for _, repoMenu := range trayState.repoMenus {
		for _, entry := range repoMenu.children {
			close(entry.stop)
			entry.item.Remove()
		}
		repoMenu.menu.Remove()
	}
	trayState.repoMenus = nil

	byRepo := make(map[string][]skillmgr.Skill)
	for _, skill := range skills {
		key := skill.RepoID
		if key == "" {
			key = skill.SourceKey
		}
		if key == "" {
			key = skill.SourceID
		}
		byRepo[key] = append(byRepo[key], skill)
	}

	repoByKey := make(map[string]skillmgr.Repository, len(repositories))
	for _, repository := range repositories {
		key := repository.RepoID
		if key == "" {
			key = repository.SourceKey
		}
		repoByKey[key] = repository
	}
	for key := range byRepo {
		if _, exists := repoByKey[key]; !exists {
			repoByKey[key] = skillmgr.Repository{RepoID: key, ID: key}
		}
	}

	repoKeys := make([]string, 0, len(repoByKey))
	for key := range repoByKey {
		repoKeys = append(repoKeys, key)
	}
	sort.SliceStable(repoKeys, func(i, j int) bool {
		return trayRepoLabel(repoByKey[repoKeys[i]]) < trayRepoLabel(repoByKey[repoKeys[j]])
	})

	for _, key := range repoKeys {
		repo := repoByKey[key]
		menu := systray.AddMenuItem(trayRepoLabel(repo), "该 repo 中的全部 Skill")
		repoMenu := trayRepoMenu{menu: menu}
		repoSkills := byRepo[key]
		sort.SliceStable(repoSkills, func(i, j int) bool {
			return traySkillLabel(repoSkills[i]) < traySkillLabel(repoSkills[j])
		})
		for _, skill := range repoSkills {
			action := "启用"
			if skill.IsActive {
				action = "关闭"
			}
			item := menu.AddSubMenuItemCheckbox(traySkillLabel(skill), "点击"+action+"此 Skill", skill.IsActive)
			stop := make(chan struct{})
			repoMenu.children = append(repoMenu.children, traySkillItem{item: item, stop: stop})
			go watchTraySkillItem(item, stop, skill.ID, skill.IsActive)
		}
		trayState.repoMenus = append(trayState.repoMenus, repoMenu)
	}
}

func watchTraySkillItem(item *systray.MenuItem, stop <-chan struct{}, skillID string, enabled bool) {
	select {
	case <-item.ClickedCh:
		if app := trayApp(); app != nil {
			var err error
			if enabled {
				_, err = app.DisableSkill(skillID)
			} else {
				_, err = app.EnableSkill(skillID)
			}
			if err != nil {
				app.debugLogf("tray toggle skill=%s enabled=%t error=%v", skillID, enabled, err)
			}
		}
	case <-stop:
	}
}

func traySkillLabel(skill skillmgr.Skill) string {
	if skill.DisplayName != "" {
		return skill.DisplayName
	}
	if skill.Name != "" {
		return skill.Name
	}
	return skill.ID
}

func trayRepoLabel(repository skillmgr.Repository) string {
	if repository.Alias != "" {
		return repository.Alias
	}
	if repository.RepoID != "" {
		return repository.RepoID
	}
	if repository.Path != "" {
		return repository.Path
	}
	return repository.ID
}

func stopSystemTray() {
	trayState.Lock()
	end := trayState.end
	trayState.end = nil
	trayState.started = false
	trayState.Unlock()

	trayState.menuMu.Lock()
	for _, repoMenu := range trayState.repoMenus {
		for _, entry := range repoMenu.children {
			close(entry.stop)
		}
	}
	trayState.repoMenus = nil
	trayState.menuReady = false
	trayState.pendingSkills = nil
	trayState.pendingRepos = nil
	trayState.menuMu.Unlock()

	if end != nil {
		end()
	}
}
