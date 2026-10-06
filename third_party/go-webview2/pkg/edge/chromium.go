//go:build windows
// +build windows

package edge

// Local patch (ollama-mgr): environment/controller completion handlers no
// longer nil-deref or log.Fatal when WebView2 reports a failed HRESULT. The
// failure is recorded, Embed returns false, and the caller can read
// InitError() and show its own error UI. See third_party/go-webview2/PATCHES.md.

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/jchv/go-webview2/internal/w32"
	"golang.org/x/sys/windows"
)

// DefaultInitTimeout bounds how long Embed waits for the WebView2 environment
// and controller before giving up (a late or never-completing environment).
const DefaultInitTimeout = 30 * time.Second

const initTimerID = 0x4F4D

var (
	user32SetTimer  = windows.NewLazySystemDLL("user32.dll").NewProc("SetTimer")
	user32KillTimer = windows.NewLazySystemDLL("user32.dll").NewProc("KillTimer")
)

const (
	initPending uintptr = 0
	initOK      uintptr = 1
	initFailed  uintptr = 2
)

type Chromium struct {
	hwnd                  uintptr
	focusOnInit           bool
	controller            *ICoreWebView2Controller
	webview               *ICoreWebView2
	inited                uintptr
	initErr               error
	envCompleted          *iCoreWebView2CreateCoreWebView2EnvironmentCompletedHandler
	controllerCompleted   *iCoreWebView2CreateCoreWebView2ControllerCompletedHandler
	webMessageReceived    *iCoreWebView2WebMessageReceivedEventHandler
	permissionRequested   *iCoreWebView2PermissionRequestedEventHandler
	webResourceRequested  *iCoreWebView2WebResourceRequestedEventHandler
	acceleratorKeyPressed *ICoreWebView2AcceleratorKeyPressedEventHandler
	navigationCompleted   *ICoreWebView2NavigationCompletedEventHandler

	environment *ICoreWebView2Environment

	// Settings
	DataPath string

	// InitTimeout overrides DefaultInitTimeout when > 0.
	InitTimeout time.Duration

	// permissions
	permissions      map[CoreWebView2PermissionKind]CoreWebView2PermissionState
	globalPermission *CoreWebView2PermissionState

	// Callbacks
	MessageCallback              func(string)
	WebResourceRequestedCallback func(request *ICoreWebView2WebResourceRequest, args *ICoreWebView2WebResourceRequestedEventArgs)
	NavigationCompletedCallback  func(sender *ICoreWebView2, args *ICoreWebView2NavigationCompletedEventArgs)
	AcceleratorKeyCallback       func(uint) bool
}

// hresultFailed reports whether an HRESULT delivered as a uintptr is a
// failure code. HRESULT is a 32-bit LONG; on 64-bit Windows the upper half
// of the register is not sign-extended, so int64(res) < 0 never fires.
func hresultFailed(res uintptr) bool {
	return int32(uint32(res)) < 0
}

func NewChromium() *Chromium {
	e := &Chromium{}
	/*
	 All these handlers are passed to native code through syscalls with 'uintptr(unsafe.Pointer(handler))' and we know
	 that a pointer to those will be kept in the native code. Furthermore these handlers als contain pointer to other Go
	 structs like the vtable.
	 This violates the unsafe.Pointer rule '(4) Conversion of a Pointer to a uintptr when calling syscall.Syscall.' because
	 theres no guarantee that Go doesn't move these objects.
	 AFAIK currently the Go runtime doesn't move HEAP objects, so we should be safe with these handlers. But they don't
	 guarantee it, because in the future Go might use a compacting GC.
	 There's a proposal to add a runtime.Pin function, to prevent moving pinned objects, which would allow to easily fix
	 this issue by just pinning the handlers. The https://go-review.googlesource.com/c/go/+/367296/ should land in Go 1.19.
	*/
	e.envCompleted = newICoreWebView2CreateCoreWebView2EnvironmentCompletedHandler(e)
	e.controllerCompleted = newICoreWebView2CreateCoreWebView2ControllerCompletedHandler(e)
	e.webMessageReceived = newICoreWebView2WebMessageReceivedEventHandler(e)
	e.permissionRequested = newICoreWebView2PermissionRequestedEventHandler(e)
	e.webResourceRequested = newICoreWebView2WebResourceRequestedEventHandler(e)
	e.acceleratorKeyPressed = newICoreWebView2AcceleratorKeyPressedEventHandler(e)
	e.navigationCompleted = newICoreWebView2NavigationCompletedEventHandler(e)
	e.permissions = make(map[CoreWebView2PermissionKind]CoreWebView2PermissionState)

	return e
}

// InitError returns why Embed failed, or nil.
func (e *Chromium) InitError() error {
	if atomic.LoadUintptr(&e.inited) != initFailed {
		return nil
	}
	return e.initErr
}

// fail records the first initialization error and wakes the Embed message loop.
func (e *Chromium) fail(err error) {
	if atomic.LoadUintptr(&e.inited) != initPending {
		return
	}
	e.initErr = err
	atomic.StoreUintptr(&e.inited, initFailed)
	if e.hwnd != 0 {
		// WM_NULL just unblocks GetMessageW so the loop re-checks e.inited.
		_, _, _ = w32.User32PostMessageW.Call(e.hwnd, 0, 0, 0)
	}
}

func (e *Chromium) Embed(hwnd uintptr) bool {
	e.hwnd = hwnd

	dataPath := e.DataPath
	if dataPath == "" {
		currentExePath := make([]uint16, windows.MAX_PATH)
		_, err := windows.GetModuleFileName(windows.Handle(0), &currentExePath[0], windows.MAX_PATH)
		if err != nil {
			e.fail(fmt.Errorf("GetModuleFileName: %v", err))
			return false
		}
		currentExeName := filepath.Base(windows.UTF16ToString(currentExePath))
		dataPath = filepath.Join(os.Getenv("AppData"), currentExeName)
	}

	res, err := createCoreWebView2EnvironmentWithOptions(nil, windows.StringToUTF16Ptr(dataPath), 0, e.envCompleted)
	if err != nil {
		log.Printf("Error calling Webview2Loader: %v", err)
		e.fail(fmt.Errorf("could not load WebView2Loader: %v", err))
		return false
	} else if res != 0 {
		log.Printf("Result: %08x", res)
		e.fail(fmt.Errorf("CreateCoreWebView2EnvironmentWithOptions failed (HRESULT 0x%08X)", uint32(res)))
		return false
	}
	timeout := e.InitTimeout
	if timeout <= 0 {
		timeout = DefaultInitTimeout
	}
	deadline := time.Now().Add(timeout)
	// A periodic WM_TIMER keeps GetMessageW returning so the deadline is checked.
	_, _, _ = user32SetTimer.Call(e.hwnd, initTimerID, 500, 0)
	defer func() { _, _, _ = user32KillTimer.Call(e.hwnd, initTimerID) }()

	var msg w32.Msg
	for {
		if atomic.LoadUintptr(&e.inited) != initPending {
			break
		}
		if time.Now().After(deadline) {
			e.fail(fmt.Errorf("WebView2 did not finish starting within %d seconds", int(timeout/time.Second)))
			break
		}
		r, _, _ := w32.User32GetMessageW.Call(
			uintptr(unsafe.Pointer(&msg)),
			0,
			0,
			0,
		)
		if int32(r) == 0 {
			e.fail(fmt.Errorf("message loop ended before WebView2 finished starting"))
			break
		}
		if int32(r) == -1 {
			e.fail(fmt.Errorf("GetMessageW failed while waiting for WebView2"))
			break
		}
		_, _, _ = w32.User32TranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		_, _, _ = w32.User32DispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
	}
	if atomic.LoadUintptr(&e.inited) != initOK || e.webview == nil {
		if e.initErr == nil {
			e.fail(fmt.Errorf("WebView2 did not finish starting"))
		}
		return false
	}
	e.Init("window.external={invoke:s=>window.chrome.webview.postMessage(s)}")
	return true
}

func (e *Chromium) Navigate(url string) {
	if e.webview == nil {
		return
	}
	_, _, _ = e.webview.vtbl.Navigate.Call(
		uintptr(unsafe.Pointer(e.webview)),
		uintptr(unsafe.Pointer(windows.StringToUTF16Ptr(url))),
	)
}

func (e *Chromium) NavigateToString(htmlContent string) {
	if e.webview == nil {
		return
	}
	_, _, _ = e.webview.vtbl.NavigateToString.Call(
		uintptr(unsafe.Pointer(e.webview)),
		uintptr(unsafe.Pointer(windows.StringToUTF16Ptr(htmlContent))),
	)
}

func (e *Chromium) Init(script string) {
	if e.webview == nil {
		return
	}
	_, _, _ = e.webview.vtbl.AddScriptToExecuteOnDocumentCreated.Call(
		uintptr(unsafe.Pointer(e.webview)),
		uintptr(unsafe.Pointer(windows.StringToUTF16Ptr(script))),
		0,
	)
}

func (e *Chromium) Eval(script string) {
	if e.webview == nil {
		return
	}
	_script, err := windows.UTF16PtrFromString(script)
	if err != nil {
		log.Printf("Eval: %v", err)
		return
	}

	_, _, _ = e.webview.vtbl.ExecuteScript.Call(
		uintptr(unsafe.Pointer(e.webview)),
		uintptr(unsafe.Pointer(_script)),
		0,
	)
}

func (e *Chromium) Show() error {
	if e.controller == nil {
		return fmt.Errorf("webview2 controller not ready")
	}
	return e.controller.PutIsVisible(true)
}

func (e *Chromium) Hide() error {
	if e.controller == nil {
		return fmt.Errorf("webview2 controller not ready")
	}
	return e.controller.PutIsVisible(false)
}

func (e *Chromium) QueryInterface(_, _ uintptr) uintptr {
	return 0
}

func (e *Chromium) AddRef() uintptr {
	return 1
}

func (e *Chromium) Release() uintptr {
	return 1
}

func (e *Chromium) EnvironmentCompleted(res uintptr, env *ICoreWebView2Environment) (ret uintptr) {
	defer func() {
		if r := recover(); r != nil {
			e.fail(fmt.Errorf("panic in EnvironmentCompleted: %v", r))
			ret = 0
		}
	}()
	if atomic.LoadUintptr(&e.inited) == initFailed {
		// Embed already gave up (timeout); ignore a late completion.
		return 0
	}
	if hresultFailed(res) {
		e.fail(fmt.Errorf("creating the WebView2 environment failed (HRESULT 0x%08X)", uint32(res)))
		return 0
	}
	if env == nil {
		e.fail(fmt.Errorf("WebView2 returned no environment (HRESULT 0x%08X)", uint32(res)))
		return 0
	}
	_, _, _ = env.vtbl.AddRef.Call(uintptr(unsafe.Pointer(env)))
	e.environment = env

	hr, _, _ := env.vtbl.CreateCoreWebView2Controller.Call(
		uintptr(unsafe.Pointer(env)),
		e.hwnd,
		uintptr(unsafe.Pointer(e.controllerCompleted)),
	)
	if hresultFailed(hr) {
		e.fail(fmt.Errorf("CreateCoreWebView2Controller failed (HRESULT 0x%08X)", uint32(hr)))
	}
	return 0
}

func (e *Chromium) CreateCoreWebView2ControllerCompleted(res uintptr, controller *ICoreWebView2Controller) (ret uintptr) {
	defer func() {
		if r := recover(); r != nil {
			e.fail(fmt.Errorf("panic in CreateCoreWebView2ControllerCompleted: %v", r))
			ret = 0
		}
	}()
	if atomic.LoadUintptr(&e.inited) == initFailed {
		return 0
	}
	if hresultFailed(res) {
		e.fail(fmt.Errorf("creating the WebView2 controller failed (HRESULT 0x%08X)", uint32(res)))
		return 0
	}
	if controller == nil {
		e.fail(fmt.Errorf("WebView2 returned no controller (HRESULT 0x%08X)", uint32(res)))
		return 0
	}
	_, _, _ = controller.vtbl.AddRef.Call(uintptr(unsafe.Pointer(controller)))
	e.controller = controller

	var token _EventRegistrationToken
	_, _, _ = controller.vtbl.GetCoreWebView2.Call(
		uintptr(unsafe.Pointer(controller)),
		uintptr(unsafe.Pointer(&e.webview)),
	)
	if e.webview == nil {
		e.fail(fmt.Errorf("WebView2 controller returned no CoreWebView2"))
		return 0
	}
	_, _, _ = e.webview.vtbl.AddRef.Call(
		uintptr(unsafe.Pointer(e.webview)),
	)
	_, _, _ = e.webview.vtbl.AddWebMessageReceived.Call(
		uintptr(unsafe.Pointer(e.webview)),
		uintptr(unsafe.Pointer(e.webMessageReceived)),
		uintptr(unsafe.Pointer(&token)),
	)
	_, _, _ = e.webview.vtbl.AddPermissionRequested.Call(
		uintptr(unsafe.Pointer(e.webview)),
		uintptr(unsafe.Pointer(e.permissionRequested)),
		uintptr(unsafe.Pointer(&token)),
	)
	_, _, _ = e.webview.vtbl.AddWebResourceRequested.Call(
		uintptr(unsafe.Pointer(e.webview)),
		uintptr(unsafe.Pointer(e.webResourceRequested)),
		uintptr(unsafe.Pointer(&token)),
	)
	_, _, _ = e.webview.vtbl.AddNavigationCompleted.Call(
		uintptr(unsafe.Pointer(e.webview)),
		uintptr(unsafe.Pointer(e.navigationCompleted)),
		uintptr(unsafe.Pointer(&token)),
	)

	_ = e.controller.AddAcceleratorKeyPressed(e.acceleratorKeyPressed, &token)

	atomic.StoreUintptr(&e.inited, initOK)

	if e.focusOnInit {
		e.Focus()
	}

	return 0
}

func (e *Chromium) MessageReceived(sender *ICoreWebView2, args *iCoreWebView2WebMessageReceivedEventArgs) uintptr {
	var message *uint16
	_, _, _ = args.vtbl.TryGetWebMessageAsString.Call(
		uintptr(unsafe.Pointer(args)),
		uintptr(unsafe.Pointer(&message)),
	)
	if e.MessageCallback != nil {
		e.MessageCallback(w32.Utf16PtrToString(message))
	}
	_, _, _ = sender.vtbl.PostWebMessageAsString.Call(
		uintptr(unsafe.Pointer(sender)),
		uintptr(unsafe.Pointer(message)),
	)
	windows.CoTaskMemFree(unsafe.Pointer(message))
	return 0
}

func (e *Chromium) SetPermission(kind CoreWebView2PermissionKind, state CoreWebView2PermissionState) {
	e.permissions[kind] = state
}

func (e *Chromium) SetGlobalPermission(state CoreWebView2PermissionState) {
	e.globalPermission = &state
}

func (e *Chromium) PermissionRequested(_ *ICoreWebView2, args *iCoreWebView2PermissionRequestedEventArgs) uintptr {
	var kind CoreWebView2PermissionKind
	_, _, _ = args.vtbl.GetPermissionKind.Call(
		uintptr(unsafe.Pointer(args)),
		uintptr(kind),
	)
	var result CoreWebView2PermissionState
	if e.globalPermission != nil {
		result = *e.globalPermission
	} else {
		var ok bool
		result, ok = e.permissions[kind]
		if !ok {
			result = CoreWebView2PermissionStateDefault
		}
	}
	_, _, _ = args.vtbl.PutState.Call(
		uintptr(unsafe.Pointer(args)),
		uintptr(result),
	)
	return 0
}

func (e *Chromium) WebResourceRequested(sender *ICoreWebView2, args *ICoreWebView2WebResourceRequestedEventArgs) uintptr {
	req, err := args.GetRequest()
	if err != nil {
		log.Printf("WebResourceRequested: %v", err)
		return 0
	}
	if e.WebResourceRequestedCallback != nil {
		e.WebResourceRequestedCallback(req, args)
	}
	return 0
}

func (e *Chromium) AddWebResourceRequestedFilter(filter string, ctx COREWEBVIEW2_WEB_RESOURCE_CONTEXT) {
	if e.webview == nil {
		return
	}
	err := e.webview.AddWebResourceRequestedFilter(filter, ctx)
	if err != nil {
		log.Printf("AddWebResourceRequestedFilter: %v", err)
	}
}

func (e *Chromium) Environment() *ICoreWebView2Environment {
	return e.environment
}

// AcceleratorKeyPressed is called when an accelerator key is pressed.
// If the AcceleratorKeyCallback method has been set, it will defer handling of the keypress
// to the callback. That callback returns a bool indicating if the event was handled.
func (e *Chromium) AcceleratorKeyPressed(sender *ICoreWebView2Controller, args *ICoreWebView2AcceleratorKeyPressedEventArgs) uintptr {
	if e.AcceleratorKeyCallback == nil {
		return 0
	}
	eventKind, _ := args.GetKeyEventKind()
	if eventKind == COREWEBVIEW2_KEY_EVENT_KIND_KEY_DOWN ||
		eventKind == COREWEBVIEW2_KEY_EVENT_KIND_SYSTEM_KEY_DOWN {
		virtualKey, _ := args.GetVirtualKey()
		status, _ := args.GetPhysicalKeyStatus()
		if !status.WasKeyDown {
			_ = args.PutHandled(e.AcceleratorKeyCallback(virtualKey))
			return 0
		}
	}
	_ = args.PutHandled(false)
	return 0
}

func (e *Chromium) GetSettings() (*ICoreWebViewSettings, error) {
	if e.webview == nil {
		return nil, fmt.Errorf("webview2 not initialized")
	}
	return e.webview.GetSettings()
}

func (e *Chromium) GetController() *ICoreWebView2Controller {
	return e.controller
}

func boolToInt(input bool) int {
	if input {
		return 1
	}
	return 0
}

func (e *Chromium) NavigationCompleted(sender *ICoreWebView2, args *ICoreWebView2NavigationCompletedEventArgs) uintptr {
	if e.NavigationCompletedCallback != nil {
		e.NavigationCompletedCallback(sender, args)
	}
	return 0
}

func (e *Chromium) NotifyParentWindowPositionChanged() error {
	//It looks like the wndproc function is called before the controller initialization is complete.
	//Because of this the controller is nil
	if e.controller == nil {
		return nil
	}
	return e.controller.NotifyParentWindowPositionChanged()
}

func (e *Chromium) Focus() {
	if e.controller == nil {
		e.focusOnInit = true
		return
	}
	_ = e.controller.MoveFocus(COREWEBVIEW2_MOVE_FOCUS_REASON_PROGRAMMATIC)
}
