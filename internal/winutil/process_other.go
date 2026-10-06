//go:build !windows

package winutil

import (
	"fmt"
	"os/exec"
	"runtime"
)

func StartNoWindow(cmd *exec.Cmd) error {
	return cmd.Start()
}

func OpenURL(rawURL string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", rawURL)
	default:
		cmd = exec.Command("xdg-open", rawURL)
	}
	return cmd.Start()
}

func StartInteractiveConsole(_ string) error {
	return fmt.Errorf("interactive console launch is Windows-only")
}