//go:build darwin

package main

import (
	_ "embed"
	"sync"

	"fyne.io/systray"
	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

//go:embed build/appicon.png
var trayIcon []byte

var trayState struct {
	sync.RWMutex
	app     *App
	end     func()
	started bool
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

func stopSystemTray() {
	trayState.Lock()
	end := trayState.end
	trayState.end = nil
	trayState.started = false
	trayState.Unlock()

	if end != nil {
		end()
	}
}
