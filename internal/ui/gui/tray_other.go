//go:build !windows

package gui

import (
	"log"

	"github.com/kilrkrow/ollama-mgr/internal/config"
	"github.com/kilrkrow/ollama-mgr/internal/winutil"
)

func runWithTray(cfg config.Config, url string, startHidden bool) {
	_ = cfg
	_ = startHidden
	log.Printf("system tray is Windows-only; opening UI at %s", url)
	_ = winutil.OpenURL(url)
	select {}
}