//go:build windows

package agentinstallation

import (
	"context"
	"os"
	"os/exec"
	"testing"
	"time"
)

const windowsServiceProcessWaitHelperEnv = "AR_IMMS_SERVICE_PROCESS_WAIT_HELPER"

func TestWaitForWindowsServiceProcessExitWaitsForProcess(t *testing.T) {
	if os.Getenv(windowsServiceProcessWaitHelperEnv) == "1" {
		time.Sleep(300 * time.Millisecond)
		return
	}

	child := exec.Command(os.Args[0], "-test.run=^TestWaitForWindowsServiceProcessExitWaitsForProcess$")
	child.Env = append(os.Environ(), windowsServiceProcessWaitHelperEnv+"=1")
	if err := child.Start(); err != nil {
		t.Fatalf("start process-wait helper: %v", err)
	}
	t.Cleanup(func() {
		if child.Process != nil {
			_ = child.Process.Kill()
			_, _ = child.Process.Wait()
		}
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := waitForWindowsServiceProcessExit(ctx, uint32(child.Process.Pid)); err != nil {
		t.Fatalf("wait for process exit: %v", err)
	}
	if err := child.Wait(); err != nil {
		t.Fatalf("wait for process-wait helper: %v", err)
	}
}
