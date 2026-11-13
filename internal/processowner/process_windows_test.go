//go:build windows

package processowner

import (
	"context"
	"os/exec"
	"testing"
	"time"
)

func TestRunContainsWindowsProcessUntilCancellation(t *testing.T) {
	commandShell, err := exec.LookPath("cmd.exe")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, Options{Executable: commandShell, Arguments: []string{"/c", "ping -n 30 127.0.0.1 > nul"}})
	}()
	time.Sleep(300 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run() after cancellation: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Run() did not clean up the Job Object")
	}
}
