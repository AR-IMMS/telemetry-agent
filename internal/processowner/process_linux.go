//go:build linux

package processowner

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"syscall"
	"time"
)

const processShutdownGrace = 5 * time.Second

func runContained(ctx context.Context, options Options) error {
	command := exec.Command(options.Executable, options.Arguments...)
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	command.Env = processEnvironment(options.Environment)
	if err := command.Start(); err != nil {
		return fmt.Errorf("start contained process: %w", err)
	}
	processGroup := -command.Process.Pid
	waited := make(chan error, 1)
	go func() { waited <- command.Wait() }()
	select {
	case err := <-waited:
		if cleanupErr := terminateProcessGroup(processGroup); cleanupErr != nil {
			return fmt.Errorf("clean up process descendants: %w", cleanupErr)
		}
		if err != nil {
			return fmt.Errorf("contained process exited: %w", err)
		}
		return fmt.Errorf("contained process exited unexpectedly")
	case <-ctx.Done():
		_ = syscall.Kill(processGroup, syscall.SIGTERM)
		timer := time.NewTimer(processShutdownGrace)
		defer timer.Stop()
		select {
		case <-waited:
			return terminateProcessGroup(processGroup)
		case <-timer.C:
			cleanupErr := terminateProcessGroup(processGroup)
			select {
			case <-waited:
				return cleanupErr
			case <-time.After(processShutdownGrace):
				return fmt.Errorf("wait for contained process shutdown: %w", ctx.Err())
			}
		}
	}
}

func terminateProcessGroup(processGroup int) error {
	if err := syscall.Kill(processGroup, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
		return fmt.Errorf("send kill to process group: %w", err)
	}
	deadline := time.Now().Add(processShutdownGrace)
	for time.Now().Before(deadline) {
		err := syscall.Kill(processGroup, 0)
		if errors.Is(err, syscall.ESRCH) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("verify process group termination: %w", err)
		}
		time.Sleep(25 * time.Millisecond)
	}
	return fmt.Errorf("process group %d remained after termination", -processGroup)
}
