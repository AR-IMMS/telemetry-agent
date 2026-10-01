//go:build windows

package main

import (
	"context"
	"testing"
)

func TestNewWindowsAgentServiceIgnoresCommandContextCancellation(
	t *testing.T,
) {
	commandContext, cancel := context.WithCancel(context.Background())
	cancel()

	service := newWindowsAgentService(
		commandContext,
		func(context.Context) error {
			return nil
		},
	)

	select {
	case <-service.parent.Done():
		t.Fatal("Windows service parent context was cancelled by command context")
	default:
	}
}
