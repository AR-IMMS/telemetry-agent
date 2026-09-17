package nodeexporter

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ar-imms/telemetry-agent/internal/dependency"
)

func TestFreshInstallerPublishesStartsAndChecksNodeExporter(
	t *testing.T,
) {
	var steps []string

	installer := freshInstaller{
		options: Options{
			InstallDir:    "/opt/ar-imms/node-exporter",
			ServicePath:   "/etc/systemd/system/ar-imms-node-exporter.service",
			ListenAddress: "127.0.0.1:9100",
		},
		startupTimeout: time.Second,
		stageArchive: func(
			ctx context.Context,
		) (string, func(), error) {
			steps = append(steps, "stage")

			return "/temporary/node-exporter.tar.gz", func() {
				steps = append(steps, "cleanup")
			}, nil
		},
		installArchive: func(archivePath string) error {
			if archivePath != "/temporary/node-exporter.tar.gz" {
				t.Fatalf("archive path = %q", archivePath)
			}

			steps = append(steps, "publish")
			return nil
		},
		writeUnit: func(options Options) error {
			steps = append(steps, "write-unit")
			return nil
		},
		startService: func(
			ctx context.Context,
			serviceName string,
		) error {
			if serviceName != "ar-imms-node-exporter.service" {
				t.Fatalf("service name = %q", serviceName)
			}

			steps = append(steps, "start")
			return nil
		},
		waitForHealth: func(
			ctx context.Context,
			endpoint string,
		) error {
			if endpoint != "http://127.0.0.1:9100/metrics" {
				t.Fatalf("health endpoint = %q", endpoint)
			}

			steps = append(steps, "health")
			return nil
		},
	}

	result, err := installer.Install(context.Background())
	if err != nil {
		t.Fatalf("installer.Install() error = %v", err)
	}

	wantResult := dependency.InstallResult{
		Name:   "node-exporter",
		Reused: false,
	}
	if !reflect.DeepEqual(result, wantResult) {
		t.Fatalf("install result = %#v, want %#v", result, wantResult)
	}

	wantSteps := []string{
		"stage",
		"publish",
		"write-unit",
		"start",
		"health",
		"cleanup",
	}
	if !reflect.DeepEqual(steps, wantSteps) {
		t.Fatalf("steps = %v, want %v", steps, wantSteps)
	}
}

func TestFreshInstallerUpdatesUnitRestartsAndChecksNodeExporter(
	t *testing.T,
) {
	var steps []string

	installer := freshInstaller{
		options: Options{
			InstallDir:    "/opt/ar-imms/node-exporter",
			ServicePath:   "/etc/systemd/system/ar-imms-node-exporter.service",
			ListenAddress: "127.0.0.1:9100",
		},
		startupTimeout: time.Second,
		stageArchive: func(
			ctx context.Context,
		) (string, func(), error) {
			t.Fatal("update must not stage an archive")
			return "", nil, nil
		},
		installArchive: func(archivePath string) error {
			t.Fatal("update must not publish an archive")
			return nil
		},
		writeUnit: func(options Options) error {
			steps = append(steps, "write-unit")
			return nil
		},
		restartService: func(
			ctx context.Context,
			serviceName string,
		) error {
			if serviceName != "ar-imms-node-exporter.service" {
				t.Fatalf("service name = %q", serviceName)
			}

			steps = append(steps, "restart")
			return nil
		},
		waitForHealth: func(
			ctx context.Context,
			endpoint string,
		) error {
			if endpoint != "http://127.0.0.1:9100/metrics" {
				t.Fatalf("health endpoint = %q", endpoint)
			}

			steps = append(steps, "health")
			return nil
		},
	}

	result, err := installer.Update(context.Background())
	if err != nil {
		t.Fatalf("installer.Update() error = %v", err)
	}

	wantResult := dependency.InstallResult{
		Name: "node-exporter",
	}
	if !reflect.DeepEqual(result, wantResult) {
		t.Fatalf("update result = %#v, want %#v", result, wantResult)
	}

	wantSteps := []string{
		"write-unit",
		"restart",
		"health",
	}
	if !reflect.DeepEqual(steps, wantSteps) {
		t.Fatalf("steps = %v, want %v", steps, wantSteps)
	}
}

func TestFreshInstallerUpdateSkipsHealthWhenRestartFails(t *testing.T) {
	healthCalls := 0

	installer := freshInstaller{
		options: Options{
			InstallDir:    "/opt/ar-imms/node-exporter",
			ServicePath:   "/etc/systemd/system/ar-imms-node-exporter.service",
			ListenAddress: "127.0.0.1:9100",
		},
		startupTimeout: time.Second,
		writeUnit: func(options Options) error {
			return nil
		},
		restartService: func(
			ctx context.Context,
			serviceName string,
		) error {
			return errors.New("intentional systemd update failure")
		},
		waitForHealth: func(
			ctx context.Context,
			endpoint string,
		) error {
			healthCalls++
			return nil
		},
	}

	_, err := installer.Update(context.Background())

	if err == nil {
		t.Fatal("installer.Update() error = nil, want restart failure")
	}
	if !strings.Contains(err.Error(), "restart Node Exporter") {
		t.Fatalf("installer.Update() error = %v, want restart context", err)
	}
	if healthCalls != 0 {
		t.Fatalf("health calls = %d, want 0 after restart failure", healthCalls)
	}
}
