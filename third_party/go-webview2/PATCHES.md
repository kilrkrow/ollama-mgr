# Local patches to github.com/jchv/go-webview2

Base: github.com/jchv/go-webview2 v0.0.0-20260205173254-56598839c808 (upstream
master 5659883, the latest commit as of 2026-10). Wired in with a `replace`
directive in the root go.mod. Upstream has no fix for the issues below.

Only `pkg/edge/chromium.go` and `webview.go` are changed. `cmd/` (demo) and
`.github/` were not copied.

1. HRESULT checks. `EnvironmentCompleted` and
   `CreateCoreWebView2ControllerCompleted` tested `int64(res) < 0`. HRESULT is
   a 32-bit LONG that arrives zero-extended in a uintptr on amd64, so failure
   codes such as CO_E_NOTINITIALIZED (0x800401F0) looked like success and the
   nil environment was dereferenced (the ollama-mgr crash). They now test
   `int32(uint32(res)) < 0` and also guard nil env/controller/webview.
2. No process exit on failure. `log.Fatal`/`log.Fatalf` in the completion
   handlers, `NewWithOptions`, `Eval`, `WebResourceRequested` and
   `AddWebResourceRequestedFilter` are replaced with recorded errors or
   log.Printf. Completion handlers also recover panics.
3. `Chromium.InitError()` and `webview2.NewWithOptionsErr()` expose why
   creation failed so the app can show its own dialog. `Embed` returns false
   on failure instead of calling `Init` on a nil webview.
4. A window whose WebView2 never started is destroyed without posting
   WM_QUIT, so the caller can still show a MessageBox on that thread.
5. Startup timeout. `Embed` gives up after `DefaultInitTimeout` (30s,
   overridable via `Chromium.InitTimeout`) if the environment or controller
   never completes (seen when the user data folder path is a file), instead
   of leaving a blank window forever. Late completions are ignored.

Threading note: upstream's `init()` locks the main goroutine and calls
CoInitializeEx(STA) only on the main thread. ollama-mgr runs systray on the
main thread, so its WebView2 goroutine calls `runtime.LockOSThread` and
`CoInitializeEx(COINIT_APARTMENTTHREADED)` itself (internal/ui/gui/webview_windows.go).
