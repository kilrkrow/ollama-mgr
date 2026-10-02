//go:build windows

package winutil

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const (
	runKeyPath            = `Software\Microsoft\Windows\CurrentVersion\Run`
	runValueName          = "KilrKrowOllamaMgr"
	TrayArgument          = "--tray"
	StartMenuShortcutName = "ollama-mgr.lnk"
	startMenuDescription  = "ollama-mgr - local Ollama model manager"
)

// StartMenuShortcutPath is %AppData%\Microsoft\Windows\Start Menu\Programs\ollama-mgr.lnk
func StartMenuShortcutPath() string {
	programs := filepath.Join(os.Getenv("APPDATA"), `Microsoft\Windows\Start Menu\Programs`)
	return filepath.Join(programs, StartMenuShortcutName)
}

// ExePath returns the running executable path.
func ExePath() (string, error) {
	return os.Executable()
}

// ApplyStartWithWindows writes or removes the HKCU Run value.
// Config is the source of truth; call on every launch (both directions).
func ApplyStartWithWindows(enable bool) error {
	exe, err := ExePath()
	if err != nil {
		return err
	}
	key, _, err := registry.CreateKey(registry.CURRENT_USER, runKeyPath, registry.SET_VALUE|registry.QUERY_VALUE)
	if err != nil {
		return err
	}
	defer key.Close()

	if enable {
		val := fmt.Sprintf("\"%s\" %s", exe, TrayArgument)
		return key.SetStringValue(runValueName, val)
	}
	_ = key.DeleteValue(runValueName)
	return nil
}

// IsStartWithWindows reports whether the HKCU Run value exists.
func IsStartWithWindows() bool {
	key, err := registry.OpenKey(registry.CURRENT_USER, runKeyPath, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer key.Close()
	_, _, err = key.GetStringValue(runValueName)
	return err == nil
}

// IsStartMenuShortcutPresent reports whether the Start Menu .lnk exists.
func IsStartMenuShortcutPresent() bool {
	_, err := os.Stat(StartMenuShortcutPath())
	return err == nil
}

// ApplyStartMenuShortcut creates or removes the Start Menu Programs shortcut.
func ApplyStartMenuShortcut(enable bool) error {
	path := StartMenuShortcutPath()
	if !enable {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}

	exe, err := ExePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	workDir := filepath.Dir(exe)

	psPath := strings.ReplaceAll(path, "'", "''")
	psExe := strings.ReplaceAll(exe, "'", "''")
	psWork := strings.ReplaceAll(workDir, "'", "''")
	psDesc := strings.ReplaceAll(startMenuDescription, "'", "''")

	script := fmt.Sprintf(
		"$s = (New-Object -ComObject WScript.Shell).CreateShortcut('%s');"+
			"$s.TargetPath = '%s';"+
			"$s.WorkingDirectory = '%s';"+
			"$s.Description = '%s';"+
			"$s.IconLocation = '%s,0';"+
			"$s.Save()",
		psPath, psExe, psWork, psDesc, psExe,
	)

	cmd := exec.Command(
		"powershell.exe",
		"-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass",
		"-WindowStyle", "Hidden",
		"-Command", script,
	)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: windows.CREATE_NO_WINDOW,
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("create start menu shortcut: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}