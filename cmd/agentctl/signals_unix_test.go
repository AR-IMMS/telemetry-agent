//go:build linux

package main

import (
	"os"
	"syscall"
	"testing"
)

func TestTerminationSignalsIncludeInterruptAndSIGTERM(t *testing.T) {
	var hasInterrupt bool
	var hasSIGTERM bool

	for _, signal := range terminationSignals() {
		switch signal {
		case os.Interrupt:
			hasInterrupt = true
		case syscall.SIGTERM:
			hasSIGTERM = true
		}
	}

	if !hasInterrupt {
		t.Fatal("terminationSignals() does not include os.Interrupt")
	}

	if !hasSIGTERM {
		t.Fatal("terminationSignals() does not include syscall.SIGTERM")
	}
}
