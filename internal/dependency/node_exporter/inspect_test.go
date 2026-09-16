package nodeexporter

import (
	"context"
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
	if healthCalls != 1 {
		t.Fatalf("health calls = %d, want 1", healthCalls)
	}
}

func TestInspectNodeExporterInstallationSkipsHealthWhenServiceIsAbsent(
	t *testing.T,
) {
	healthCalls := 0

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
}

func TestNewInstallationInspectorReadsUnitAndMetricsEndpoint(t *testing.T) {
	unitPath := filepath.Join(
		t.TempDir(),
		"ar-imms-node-exporter.service",
	)

	if err := os.WriteFile(unitPath, []byte("[Unit]\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(
		func(writer http.ResponseWriter, request *http.Request) {
			if request.URL.Path != "/metrics" {
				t.Fatalf("request path = %q, want /metrics", request.URL.Path)
			}

			writer.WriteHeader(http.StatusOK)
		},
	))
	defer server.Close()

	inspector := newInstallationInspector(
		Options{
			InstallDir:    "/opt/ar-imms/node-exporter",
			ServicePath:   unitPath,
			ListenAddress: strings.TrimPrefix(server.URL, "http://"),
		},
		server.Client(),
		10*time.Millisecond,
	)

	state, err := inspector(context.Background())
	if err != nil {
		t.Fatalf("installation inspector error = %v", err)
	}
	if !state.ServiceExists || !state.Healthy {
		t.Fatalf("installation state = %+v, want existing healthy service", state)
	}
}
