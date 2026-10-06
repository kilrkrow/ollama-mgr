//go:build !windows

package gui

import (
	"log"

	"github.com/kilrkrow/ollama-mgr/internal/winutil"
)

// openWebView is Windows-only (WebView2); elsewhere open the default browser.
func openWebView(url string) {
	log.Printf("WebView2 is Windows-only; opening browser at %s", url)
	_ = winutil.OpenURL(url)
}
