package dependency

import (
	"context"
	"strings"
	"testing"
)

func TestServiceRejectsUnknownDependency(t *testing.T) {
	service := Service{
		Registry: DefaultRegistry(),
		OS:       "windows",
	}

	_, err := service.Install(context.Background(), "not-real")

	if err == nil || !strings.Contains(err.Error(), "unknown dependency") {
		t.Fatalf("Install() error = %v, want unknown dependency error", err)
	}
}

func TestServiceRejectsUnsupportedPlatformBeforeInstallation(t *testing.T) {
	called := false

	service := Service{
		Registry: DefaultRegistry(),
		OS:       "linux",
		InstallWindowsExporter: func(
			context.Context,
		) (InstallResult, error) {
			called = true
			return InstallResult{}, nil
		},
	}

	_, err := service.Install(context.Background(), "windows-exporter")

	if err == nil || !strings.Contains(err.Error(), "does not support") {
		t.Fatalf("Install() error = %v, want unsupported-platform error", err)
	}
	if called {
		t.Fatal("Windows Exporter installer was called on Linux")
	}
}

func TestServiceInstallsWindowsExporter(t *testing.T) {
	calls := 0

	service := Service{
		Registry: DefaultRegistry(),
		OS:       "windows",
		InstallWindowsExporter: func(
			context.Context,
		) (InstallResult, error) {
			calls++

			return InstallResult{
				Reused: true,
			}, nil
		},
	}

	result, err := service.Install(
		context.Background(),
		"windows-exporter",
	)
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if calls != 1 {
		t.Fatalf("installer calls = %d, want 1", calls)
	}
	if result.Name != "windows-exporter" {
		t.Fatalf("result name = %q, want windows-exporter", result.Name)
	}
	if !result.Reused {
		t.Fatal("result Reused = false, want true")
	}
}
