//go:build windows

package supervisor

import (
	"fmt"
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

func configureChildCommand(command *exec.Cmd) {
	// A new process group is required before Windows can deliver CTRL_BREAK.
	command.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: windows.CREATE_NEW_PROCESS_GROUP,
	}
}

func requestGracefulStop(command *exec.Cmd) error {
	if command.Process == nil {
		return fmt.Errorf("Collector process has not started")
	}

	// CTRL_BREAK is the Windows equivalent of the graceful group signal used on Unix.
	return windows.GenerateConsoleCtrlEvent(
		windows.CTRL_BREAK_EVENT,
		uint32(command.Process.Pid),
	)
}
