//go:build windows

package gui

import (
	_ "embed"
	"log"
	"sync"

	"github.com/getlantern/systray"
	"github.com/kilrkrow/ollama-mgr/internal/config"
	"github.com/kilrkrow/ollama-mgr/internal/winutil"
)

//go:embed assets/appicon.ico
var trayIconICO []byte

// runWithTray hosts the Brand Book tray icon on the main thread and opens the
// WebView2 window from a goroutine pinned to its own COM STA thread. Closing
// the window returns to tray-only;
// Exit on the tray quits the process.
func runWithTray(cfg config.Config, url string, startHidden bool) {
	_ = cfg
	ui := config.LoadUISettings()
	_ = winutil.ApplyStartWithWindows(ui.StartWithWindows)

	var (
		mu          sync.Mutex
		openCh      = make(chan struct{}, 1)
		webviewOpen bool
	)

	requestOpen := func() {
		select {
		case openCh <- struct{}{}:
		default:
		}
	}

	if !startHidden {
		requestOpen()
	}

	onReady := func() {
		if len(trayIconICO) > 0 {
			systray.SetIcon(trayIconICO)
		}
		systray.SetTitle("ollama-mgr")
		systray.SetTooltip("ollama-mgr - local Ollama model manager")

		mOpen := systray.AddMenuItem("Open", "Show the ollama-mgr window")
		systray.AddSeparator()
		mStartWin := systray.AddMenuItemCheckbox("Start with Windows", "Launch at logon (tray)", ui.StartWithWindows)
		mStartMenu := systray.AddMenuItemCheckbox("Add to Start Menu", "Show/hide Start Menu shortcut", winutil.IsStartMenuShortcutPresent())
		systray.AddSeparator()
		mQuit := systray.AddMenuItem("Exit", "Quit ollama-mgr")

		go func() {
			for {
				select {
				case <-mOpen.ClickedCh:
					requestOpen()
				case <-mStartWin.ClickedCh:
					ui = config.LoadUISettings()
					ui.StartWithWindows = !ui.StartWithWindows
					if err := winutil.ApplyStartWithWindows(ui.StartWithWindows); err != nil {
						log.Printf("start with windows: %v", err)
						ui.StartWithWindows = !ui.StartWithWindows
					} else if err := config.SaveUISettings(ui); err != nil {
						log.Printf("save settings: %v", err)
					}
					if ui.StartWithWindows {
						mStartWin.Check()
					} else {
						mStartWin.Uncheck()
					}
				case <-mStartMenu.ClickedCh:
					want := !winutil.IsStartMenuShortcutPresent()
					if err := winutil.ApplyStartMenuShortcut(want); err != nil {
						log.Printf("start menu shortcut: %v", err)
					}
					if winutil.IsStartMenuShortcutPresent() {
						mStartMenu.Check()
					} else {
						mStartMenu.Uncheck()
					}
				case <-mQuit.ClickedCh:
					systray.Quit()
					return
				}
			}
		}()

		go func() {
			// WebView2 needs a dedicated STA thread with its own message loop.
			uiThreadInit()
			for range openCh {
				mu.Lock()
				if webviewOpen {
					mu.Unlock()
					continue
				}
				webviewOpen = true
				mu.Unlock()

				openWebView(url)

				mu.Lock()
				webviewOpen = false
				mu.Unlock()
			}
		}()
	}

	onExit := func() {}
	systray.Run(onReady, onExit)
}
