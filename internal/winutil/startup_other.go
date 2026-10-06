//go:build !windows

package winutil

const TrayArgument = "--tray"

func StartMenuShortcutPath() string { return "" }

func ApplyStartWithWindows(enable bool) error { return nil }

func IsStartWithWindows() bool { return false }

func IsStartMenuShortcutPresent() bool { return false }

func ApplyStartMenuShortcut(enable bool) error { return nil }

func ExePath() (string, error) { return "", nil }