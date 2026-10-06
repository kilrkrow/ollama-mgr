//go:build windows

package ollama

import (
	"os"
	"os/exec"
	"path/filepath"
	"syscall"

	"golang.org/x/sys/windows"
)

// startServeDetached launches `ollama serve` with a hidden console and no
// attachment to the parent terminal. Logs go to %LOCALAPPDATA%\ollama-mgr\ollama-serve.log.
func startServeDetached() error {
	cmd := exec.Command("ollama", "serve")
	cmd.Stdin = nil

	if logPath := serveLogPath(); logPath != "" {
		_ = os.MkdirAll(filepath.Dir(logPath), 0o755)
		if f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644); err == nil {
			cmd.Stdout = f
			cmd.Stderr = f
			// Process keeps the file handle; we intentionally do not Close here.
		} else {
			cmd.Stdout = nil
			cmd.Stderr = nil
		}
	} else {
		cmd.Stdout = nil
		cmd.Stderr = nil
	}

	// CREATE_NO_WINDOW gives ollama a hidden console that its own console
	// children (gpu-discover, llama-server runners) inherit, so none of them
	// opens a window. Do not add DETACHED_PROCESS: Windows ignores
	// CREATE_NO_WINDOW when it is combined with DETACHED_PROCESS, and a
	// console-less ollama makes every child allocate a new, visible console
	// (a Windows Terminal window each when Terminal is the default host).
	const flags = windows.CREATE_NEW_PROCESS_GROUP | windows.CREATE_NO_WINDOW
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: uint32(flags),
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
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		return ""
	}
	return filepath.Join(base, "ollama-mgr", "ollama-serve.log")
}
