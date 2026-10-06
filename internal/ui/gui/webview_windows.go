//go:build windows

package gui

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"unsafe"

	"github.com/jchv/go-webview2"
	"github.com/jchv/go-webview2/webviewloader"
	"github.com/kilrkrow/ollama-mgr/internal/winutil"
	"golang.org/x/sys/windows"
)

// testFailEnv is a test-only hook, not exposed as a flag or in the UI, that
// forces the WebView2 failure path so QA can verify the error dialog:
//
//	coinit  - skip the COM STA init on the UI thread (the original crash cause)
//	runtime - point WebView2 at a runtime folder that does not exist
const testFailEnv = "OLLAMA_MGR_TEST_WEBVIEW2_FAIL"

const (
	mbOKCancel      = 0x00000001
	mbIconWarning   = 0x00000030
	mbSetForeground = 0x00010000
	mbTopmost       = 0x00040000
	idOK            = 1

	wmSetIcon   = 0x0080
	iconSmall   = 0
	iconBig     = 1
	imageIcon   = 1
	lrShared    = 0x00008000
	smCxIcon    = 11
	smCyIcon    = 12
	smCxSmIcon  = 49
	smCySmIcon  = 50
	dialogTitle = "ollama-mgr - window could not open"
)

var (
	user32               = windows.NewLazySystemDLL("user32.dll")
	procLoadImageW       = user32.NewProc("LoadImageW")
	procSendMessageW     = user32.NewProc("SendMessageW")
	procGetSystemMetrics = user32.NewProc("GetSystemMetrics")
)

func testFailMode() string {
	return strings.ToLower(strings.TrimSpace(os.Getenv(testFailEnv)))
}

// uiThreadInit pins the calling goroutine to its OS thread and enters a COM
// single-threaded apartment there. WebView2 must be created, pumped and called
// on one STA thread with a message loop; a plain goroutine can run on any OS
// thread, and those threads never had CoInitializeEx called on them.
//
// Call it once at the top of the goroutine that calls openWebView. That
// goroutine must never call runtime.UnlockOSThread.
func uiThreadInit() {
	runtime.LockOSThread()
	if testFailMode() == "coinit" {
		log.Printf("test hook %s=coinit: skipping CoInitializeEx", testFailEnv)
		return
	}
	// S_FALSE (already STA on this thread) is returned as Errno(1); that is fine.
	if err := windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED); err != nil && err != windows.Errno(1) {
		log.Printf("CoInitializeEx(STA): %v", err)
	}
}

// webViewDataPath is the WebView2 user data folder:
// %LOCALAPPDATA%\ollama-mgr\WebView2 (always user-writable, unlike the exe dir).
func webViewDataPath() string {
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		if d, err := os.UserCacheDir(); err == nil {
			base = d
		} else {
			base = os.TempDir()
		}
	}
	p := filepath.Join(base, "ollama-mgr", "WebView2")
	if err := os.MkdirAll(p, 0o755); err != nil {
		log.Printf("webview2 data dir %s: %v", p, err)
	}
	return p
}

// openWebView opens the WebView2 window and blocks until it is closed.
// It must run on the thread prepared by uiThreadInit. Any failure, including
// a panic, ends in a clear dialog with a browser fallback instead of a crash.
func openWebView(url string) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("webview panic: %v\n%s", r, debug.Stack())
			webViewFailed(url, fmt.Errorf("internal error: %v", r))
		}
	}()

	mode := testFailMode()
	if mode == "" {
		if v, err := webviewloader.GetInstalledVersion(); err == nil && v == "" {
			webViewFailed(url, errors.New("the Microsoft Edge WebView2 Runtime is not installed"))
			return
		}
	}
	if mode == "runtime" {
		log.Printf("test hook %s=runtime: using a bogus WebView2 runtime folder", testFailEnv)
		_ = os.Setenv("WEBVIEW2_BROWSER_EXECUTABLE_FOLDER", `C:\ollama-mgr-test\no-such-webview2-runtime`)
	}

	w, err := webview2.NewWithOptionsErr(webview2.WebViewOptions{
		Debug:     false,
		AutoFocus: true,
		DataPath:  webViewDataPath(),
		WindowOptions: webview2.WindowOptions{
			Title:  "ollama-mgr",
			Width:  1100,
			Height: 640,
			Center: true,
		},
	})
	if err != nil || w == nil {
		if err == nil {
			err = errors.New("WebView2 could not be created")
		}
		log.Printf("webview2: %v", err)
		webViewFailed(url, err)
		return
	}
	defer w.Destroy()
	setWindowIcon(uintptr(w.Window()))
	w.Navigate(url)
	w.Run()
}

// webViewFailed explains the problem in plain ASCII and offers the browser
// fallback (same local server the window would have shown).
func webViewFailed(url string, cause error) {
	detail := "unknown error"
	if cause != nil {
		detail = asciiOnly(cause.Error())
	}
	msg := "ollama-mgr could not open its app window.\r\n\r\n" +
		"The Microsoft Edge WebView2 component failed to start:\r\n" +
		detail + "\r\n\r\n" +
		"Click OK to open ollama-mgr in your web browser instead.\r\n" +
		"ollama-mgr keeps running in the system tray; use the tray icon to reopen it or to exit."
	text, _ := windows.UTF16PtrFromString(msg)
	title, _ := windows.UTF16PtrFromString(dialogTitle)
	ret, _ := windows.MessageBox(0, text, title, mbOKCancel|mbIconWarning|mbSetForeground|mbTopmost)
	if ret == idOK {
		if err := winutil.OpenURL(url); err != nil {
			log.Printf("open browser: %v", err)
		}
	}
}

// setWindowIcon applies the exe's embedded app icon (go-winres "APP") to the
// WebView2 window so it never shows the default Windows icon.
func setWindowIcon(hwnd uintptr) {
	if hwnd == 0 {
		return
	}
	var hinst windows.Handle
	if err := windows.GetModuleHandleEx(0, nil, &hinst); err != nil {
		return
	}
	name, _ := windows.UTF16PtrFromString("APP")
	load := func(cxMetric, cyMetric uintptr) uintptr {
		cx, _, _ := procGetSystemMetrics.Call(cxMetric)
		cy, _, _ := procGetSystemMetrics.Call(cyMetric)
		h, _, _ := procLoadImageW.Call(uintptr(hinst), uintptr(unsafe.Pointer(name)), imageIcon, cx, cy, lrShared)
		if h == 0 {
			// fall back to the first icon group by ordinal
			h, _, _ = procLoadImageW.Call(uintptr(hinst), 1, imageIcon, cx, cy, lrShared)
		}
		return h
	}
	if big := load(smCxIcon, smCyIcon); big != 0 {
		_, _, _ = procSendMessageW.Call(hwnd, wmSetIcon, iconBig, big)
	}
	if small := load(smCxSmIcon, smCySmIcon); small != 0 {
		_, _, _ = procSendMessageW.Call(hwnd, wmSetIcon, iconSmall, small)
	}
}

func asciiOnly(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == '\n' || r == '\r' || r == '\t':
			b.WriteByte(' ')
		case r >= 0x20 && r < 0x7f:
			b.WriteRune(r)
		default:
			b.WriteByte('?')
		}
	}
	return strings.TrimSpace(b.String())
}
