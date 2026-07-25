//go:build !windows

package ollama

import (
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

// startServeDetached launches `ollama serve` detached from the controlling terminal.
func startServeDetached() error {
	cmd := exec.Command("ollama", "serve")
	cmd.Stdin = nil
	if logPath := serveLogPath(); logPath != "" {
		_ = os.MkdirAll(filepath.Dir(logPath), 0o755)
		if f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644); err == nil {
			cmd.Stdout = f
			cmd.Stderr = f
		} else {
			cmd.Stdout = nil
			cmd.Stderr = nil
		}
	} else {
		cmd.Stdout = nil
		cmd.Stderr = nil
	}
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

func serveLogPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".cache", "ollama-mgr", "ollama-serve.log")
}
