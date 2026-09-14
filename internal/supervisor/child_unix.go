//go:build linux

package supervisor

import (
	"fmt"
	"os/exec"
	"syscall"
)

func configureChildCommand(command *exec.Cmd) {
	// Isolate the Collector in its own process group so shutdown includes children.
	command.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}
}

func requestGracefulStop(command *exec.Cmd) error {
	if command.Process == nil {
		return fmt.Errorf("Collector process has not started")
	}

	// Signal the process group, not only the Collector parent process.
	return syscall.Kill(-command.Process.Pid, syscall.SIGTERM)
}

func forceKillCollector(command *exec.Cmd) error {
	// The Collector starts in its own process group. SIGKILL to negative PID
	// reaches every remaining process in that group.
	return syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
}
