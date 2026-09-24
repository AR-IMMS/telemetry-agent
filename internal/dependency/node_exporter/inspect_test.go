package nodeexporter

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestInspectNodeExporterInstallationReportsHealthyService(t *testing.T) {
	healthCalls := 0

	state, err := inspectNodeExporterInstallation(
		context.Background(),
		"ar-imms-node-exporter.service",
		func(
			ctx context.Context,
			serviceName string,
		) (bool, error) {
			if serviceName != "ar-imms-node-exporter.service" {
				t.Fatalf("service name = %q", serviceName)
			}

			return true, nil
		},
		func(ctx context.Context) error {
			healthCalls++
			return nil
		},
		func(ctx context.Context) (bool, error) {
			return true, nil
		},
	)
	if err != nil {
		t.Fatalf("inspectNodeExporterInstallation() error = %v", err)
	}
	if !state.ServiceExists {
		t.Fatal("ServiceExists = false, want true")
	}
	if !state.Healthy {
		t.Fatal("Healthy = false, want true")
	}
	if !state.UnitMatches {
		t.Fatal("UnitMatches = false, want true")
	}
	if healthCalls != 1 {
		t.Fatalf("health calls = %d, want 1", healthCalls)
	}
}

func TestInspectNodeExporterInstallationSkipsHealthWhenServiceIsAbsent(
	t *testing.T,
) {
	healthCalls := 0
	unitMatchCalls := 0

	state, err := inspectNodeExporterInstallation(
		context.Background(),
		"ar-imms-node-exporter.service",
		func(
			ctx context.Context,
			serviceName string,
		) (bool, error) {
			return false, nil
		},
		func(ctx context.Context) error {
			healthCalls++
			return nil
		},
		func(ctx context.Context) (bool, error) {
			unitMatchCalls++
			return true, nil
		},
	)
	if err != nil {
		t.Fatalf("inspectNodeExporterInstallation() error = %v", err)
	}
	if state.ServiceExists {
		t.Fatal("ServiceExists = true, want false")
	}
	if state.Healthy {
		t.Fatal("Healthy = true, want false")
	}
	if healthCalls != 0 {
		t.Fatalf("health calls = %d, want 0", healthCalls)
	}
	if unitMatchCalls != 0 {
		t.Fatalf("unit match calls = %d, want 0", unitMatchCalls)
	}
}

func TestInspectNodeExporterInstallationReportsHealthyDriftedUnit(
	t *testing.T,
) {
	state, err := inspectNodeExporterInstallation(
		context.Background(),
		"ar-imms-node-exporter.service",
		func(
			ctx context.Context,
			serviceName string,
		) (bool, error) {
			return true, nil
		},
		func(ctx context.Context) error {
			return nil
		},
		func(ctx context.Context) (bool, error) {
			return false, nil
		},
	)
	if err != nil {
		t.Fatalf("inspectNodeExporterInstallation() error = %v", err)
	}
	if !state.ServiceExists || !state.Healthy {
		t.Fatalf(
			"installation state = %+v, want existing healthy service",
			state,
		)
	}
	if state.UnitMatches {
		t.Fatal("UnitMatches = true, want false for drifted unit")
	}
}

func TestNewInstallationInspectorReadsUnitAndMetricsEndpoint(t *testing.T) {
	unitPath := filepath.Join(
		t.TempDir(),
		"ar-imms-node-exporter.service",
	)

	server := httptest.NewServer(http.HandlerFunc(
		func(writer http.ResponseWriter, request *http.Request) {
			if request.URL.Path != "/metrics" {
				t.Fatalf("request path = %q, want /metrics", request.URL.Path)
			}

			writer.WriteHeader(http.StatusOK)
		},
	))
	defer server.Close()

	options := Options{
		InstallDir:    "/opt/ar-imms/node-exporter",
		ServicePath:   unitPath,
		ListenAddress: strings.TrimPrefix(server.URL, "http://"),
	}

	rendered, err := RenderSystemdUnit(options)
	if err != nil {
		t.Fatalf("RenderSystemdUnit() error = %v", err)
	}

	if err := os.WriteFile(unitPath, rendered, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	inspector := newInstallationInspector(
		options,
		server.Client(),
		10*time.Millisecond,
	)

	state, err := inspector(context.Background())
	if err != nil {
		t.Fatalf("installation inspector error = %v", err)
	}
	if !state.ServiceExists || !state.Healthy || !state.UnitMatches {
		t.Fatalf(
			"installation state = %+v, want existing healthy matching service",
			state,
		)
	}
}

func TestInspectNodeExporterInstallationReportsUnhealthyMatchingUnit(
	t *testing.T,
) {
	unitMatchCalls := 0

	state, err := inspectNodeExporterInstallation(
		context.Background(),
		"ar-imms-node-exporter.service",
		func(
			ctx context.Context,
			serviceName string,
		) (bool, error) {
			return true, nil
		},
		func(ctx context.Context) error {
			return errors.New("metrics endpoint is unavailable")
		},
		func(ctx context.Context) (bool, error) {
			unitMatchCalls++

			return true, nil
		},
	)
	if err != nil {
		t.Fatalf("inspectNodeExporterInstallation() error = %v", err)
	}

	if !state.ServiceExists {
		t.Fatal("ServiceExists = false, want true")
	}
	if state.Healthy {
		t.Fatal("Healthy = true, want false")
	}
	if !state.UnitMatches {
		t.Fatal("UnitMatches = false, want true")
	}
	if unitMatchCalls != 1 {
		t.Fatalf(
			"unit match calls = %d, want 1",
			unitMatchCalls,
		)
	}
}
