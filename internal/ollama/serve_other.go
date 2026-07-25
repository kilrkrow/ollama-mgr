//go:build !windows

package ollama

import (
	"os/exec"
	"syscall"
)

// startServeDetached launches `ollama serve` detached from the controlling terminal.
func startServeDetached() error {
	cmd := exec.Command("ollama", "serve")
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid: true,
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	if cmd.Process != nil {
		_ = cmd.Process.Release()
	}
	return nil
}
