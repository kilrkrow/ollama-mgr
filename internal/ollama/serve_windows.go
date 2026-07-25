//go:build windows

package ollama

import (
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

// startServeDetached launches `ollama serve` with no visible console and no
// attachment to the parent terminal. The process continues after we return.
func startServeDetached() error {
	cmd := exec.Command("ollama", "serve")
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil
	// CREATE_NO_WINDOW: no console for a console-subsystem child
	// CREATE_NEW_PROCESS_GROUP: independent signal/job group from parent
	// DETACHED_PROCESS: do not inherit the parent's console
	const flags = windows.CREATE_NEW_PROCESS_GROUP | windows.CREATE_NO_WINDOW | windows.DETACHED_PROCESS
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: uint32(flags),
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	// Detach from Wait semantics so ollama-mgr can exit without reaping
	if cmd.Process != nil {
		_ = cmd.Process.Release()
	}
	return nil
}
