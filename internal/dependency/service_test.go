package dependency

import (
	"context"
	"errors"
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

func TestServiceReportsStatusFromIntegrationInspector(
	t *testing.T,
) {
	inspected := false

	catalog, err := NewCatalog([]Integration{
		{
			Definition: Definition{
				Name:        "windows-exporter",
				DisplayName: "Windows Exporter",
				SupportedOS: []string{"windows"},
			},
			Install: func(
				context.Context,
			) (InstallResult, error) {
				return InstallResult{}, nil
			},
			Inspect: func(
				context.Context,
			) (Inspection, error) {
				inspected = true

				return Inspection{
					Enabled: true,
					Healthy: true,
				}, nil
			},
		},
	})
	if err != nil {
		t.Fatalf("NewCatalog() error = %v", err)
	}

	service := Service{
		Catalog: catalog,
		OS:      "windows",
	}

	status, err := service.Status(
		context.Background(),
		"windows-exporter",
	)
	if err != nil {
		t.Fatalf("Service.Status() error = %v", err)
	}

	if !inspected {
		t.Fatal("integration inspector was not called")
	}

	if status.Availability != AvailabilityEnabled ||
		status.Health != HealthHealthy {
		t.Fatalf(
			"status = %+v, want enabled and healthy",
			status,
		)
	}
}

func TestServiceListsStatusesForCurrentOperatingSystem(
	t *testing.T,
) {
	catalog, err := NewCatalog([]Integration{
		{
			Definition: Definition{
				Name:        "node-exporter",
				DisplayName: "Node Exporter",
				SupportedOS: []string{"linux"},
			},
			Install: func(
				context.Context,
			) (InstallResult, error) {
				return InstallResult{}, nil
			},
			Inspect: func(
				context.Context,
			) (Inspection, error) {
				return Inspection{
					Enabled: true,
					Healthy: true,
				}, nil
			},
		},
		{
			Definition: Definition{
				Name:        "windows-exporter",
				DisplayName: "Windows Exporter",
				SupportedOS: []string{"windows"},
			},
			Install: func(
				context.Context,
			) (InstallResult, error) {
				return InstallResult{}, nil
			},
			Inspect: func(
				context.Context,
			) (Inspection, error) {
				t.Fatal("Windows inspector must not run on Linux")

				return Inspection{}, nil
			},
		},
	})
	if err != nil {
		t.Fatalf("NewCatalog() error = %v", err)
	}

	service := Service{
		Catalog: catalog,
		OS:      "linux",
	}

	results, err := service.ListStatus(context.Background())
	if err != nil {
		t.Fatalf("Service.ListStatus() error = %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("status results = %d, want 1", len(results))
	}
	if results[0].Definition.Name != "node-exporter" {
		t.Fatalf(
			"result name = %q, want node-exporter",
			results[0].Definition.Name,
		)
	}
	if results[0].Status.Availability != AvailabilityEnabled ||
		results[0].Status.Health != HealthHealthy {
		t.Fatalf(
			"status = %+v, want enabled and healthy",
			results[0].Status,
		)
	}
}

func TestServiceListsOtherStatusesWhenOneInspectorFails(
	t *testing.T,
) {
	catalog, err := NewCatalog([]Integration{
		{
			Definition: Definition{
				Name:        "a-exporter",
				DisplayName: "A Exporter",
				SupportedOS: []string{"linux"},
			},
			Install: func(
				context.Context,
			) (InstallResult, error) {
				return InstallResult{}, nil
			},
			Inspect: func(
				context.Context,
			) (Inspection, error) {
				return Inspection{}, errors.New("permission denied")
			},
		},
		{
			Definition: Definition{
				Name:        "b-exporter",
				DisplayName: "B Exporter",
				SupportedOS: []string{"linux"},
			},
			Install: func(
				context.Context,
			) (InstallResult, error) {
				return InstallResult{}, nil
			},
			Inspect: func(
				context.Context,
			) (Inspection, error) {
				return Inspection{
					Enabled: true,
					Healthy: true,
				}, nil
			},
		},
	})
	if err != nil {
		t.Fatalf("NewCatalog() error = %v", err)
	}

	results, err := (Service{
		Catalog: catalog,
		OS:      "linux",
	}).ListStatus(context.Background())
	if err != nil {
		t.Fatalf("Service.ListStatus() error = %v", err)
	}

	if len(results) != 2 {
		t.Fatalf("status results = %d, want 2", len(results))
	}

	if results[0].Status.Availability != AvailabilityUnknown ||
		results[0].Status.Health != HealthUnknown {
		t.Fatalf(
			"failed status = %+v, want unknown",
			results[0].Status,
		)
	}
	if results[0].InspectionError == nil ||
		!strings.Contains(
			results[0].InspectionError.Error(),
			"permission denied",
		) {
		t.Fatalf(
			"inspection error = %v, want permission diagnostics",
			results[0].InspectionError,
		)
	}

	if results[1].Status.Availability != AvailabilityEnabled ||
		results[1].Status.Health != HealthHealthy {
		t.Fatalf(
			"healthy status = %+v, want enabled and healthy",
			results[1].Status,
		)
	}
}
