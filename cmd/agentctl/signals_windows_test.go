//go:build windows

package main

import (
	"os"
	"testing"
)

func TestTerminationSignalsIncludeInterrupt(t *testing.T) {
	for _, signal := range terminationSignals() {
		if signal == os.Interrupt {
			return
		}
	}

	t.Fatal("terminationSignals() does not include os.Interrupt")
}
