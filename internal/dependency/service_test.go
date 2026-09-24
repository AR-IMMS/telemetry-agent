package dependency

import (
	"context"
	"strings"
	"testing"
)

func TestServiceRejectsUnknownDependency(t *testing.T) {
	service := Service{
		Catalog: newServiceCatalog(t),
		OS:      "windows",
	}

	_, err := service.Install(context.Background(), "not-real")

	if err == nil || !strings.Contains(err.Error(), "unknown dependency") {
		t.Fatalf("Install() error = %v, want unknown dependency error", err)
	}
}

func TestServiceRejectsUnsupportedPlatformBeforeInstallation(t *testing.T) {
	called := false

	service := Service{
		Catalog: newServiceCatalog(
			t,
			testServiceIntegration(
				"windows-exporter",
				"windows",
				func(context.Context) (InstallResult, error) {
					called = true

					return InstallResult{}, nil
				},
			),
		),
		OS: "linux",
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
		Catalog: newServiceCatalog(
			t,
			testServiceIntegration(
				"windows-exporter",
				"windows",
				func(context.Context) (InstallResult, error) {
					calls++

					return InstallResult{
						Reused: true,
					}, nil
				},
			),
		),
		OS: "windows",
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

func TestServiceInstallsNodeExporter(t *testing.T) {
	installed := false

	service := Service{
		Catalog: newServiceCatalog(
			t,
			testServiceIntegration(
				"node-exporter",
				"linux",
				func(context.Context) (InstallResult, error) {
					installed = true

					return InstallResult{}, nil
				},
			),
		),
		OS: "linux",
	}

	result, err := service.Install(
		context.Background(),
		"node-exporter",
	)
	if err != nil {
		t.Fatalf("Service.Install() error = %v", err)
	}

	if !installed {
		t.Fatal("node-exporter installer was not called")
	}
	if result.Name != "node-exporter" {
		t.Fatalf(
			"install result name = %q, want node-exporter",
			result.Name,
		)
	}
}

func newServiceCatalog(
	t *testing.T,
	integrations ...Integration,
) Catalog {
	t.Helper()

	catalog, err := NewCatalog(integrations)
	if err != nil {
		t.Fatalf("NewCatalog() error = %v", err)
	}

	return catalog
}

func testServiceIntegration(
	name string,
	osName string,
	install Installer,
) Integration {
	return Integration{
		Definition: Definition{
			Name:        name,
			DisplayName: name,
			Description: "Test dependency.",
			SupportedOS: []string{osName},
		},
		Install: install,
	}
}
