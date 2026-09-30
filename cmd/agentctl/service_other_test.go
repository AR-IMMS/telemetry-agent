//go:build !windows

package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestRunRejectsServiceCommandOutsideWindows(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := run(
		context.Background(),
		[]string{"service"},
		&stdout,
		&stderr,
		dependencies{},
	)

	if exitCode != 2 {
		t.Fatalf("exit code = %d, want 2", exitCode)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	if !strings.Contains(
		stderr.String(),
		"service command is only supported on Windows",
	) {
		t.Fatalf("stderr = %q", stderr.String())
	}
}
