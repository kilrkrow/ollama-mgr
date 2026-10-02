//go:build windows

package winutil

import (
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

// StartNoWindow starts a process with no console window (CREATE_NO_WINDOW).
func StartNoWindow(cmd *exec.Cmd) error {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.HideWindow = true
	cmd.SysProcAttr.CreationFlags |= windows.CREATE_NO_WINDOW
	return cmd.Start()
}

// OpenURL opens a URL with the default handler without a console flash.
func OpenURL(rawURL string) error {
	cmd := exec.Command("rundll32", "url.dll,FileProtocolHandler", rawURL)
	return StartNoWindow(cmd)
}

// StartInteractiveConsole launches a new visible console running ollama run <model>.
// The helper cmd.exe used to spawn it is hidden (no flash); the interactive window is intentional.
func StartInteractiveConsole(model string) error {
	cmd := exec.Command(
		"cmd.exe", "/C",
		"start", "",
		"cmd.exe", "/K",
		"ollama", "run", model,
	)
	return StartNoWindow(cmd)
}