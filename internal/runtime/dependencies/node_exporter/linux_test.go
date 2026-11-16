package nodeexporter

import (
	"strings"
	"testing"
)

func TestRequireLinuxRootRejectsUnelevatedProcess(t *testing.T) {
	err := requireLinuxRoot(func() int {
		return 1000
	})

	if err == nil {
		t.Fatal("requireLinuxRoot() error = nil, want an error")
	}
	if !strings.Contains(err.Error(), "root") {
		t.Fatalf(
			"requireLinuxRoot() error = %v, want root privilege error",
			err,
		)
	}
}

func TestRequireLinuxRootAllowsRootProcess(t *testing.T) {
	err := requireLinuxRoot(func() int {
		return 0
	})
	if err != nil {
		t.Fatalf("requireLinuxRoot() error = %v, want nil", err)
	}
}
