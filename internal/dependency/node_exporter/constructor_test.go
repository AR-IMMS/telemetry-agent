package nodeexporter

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/ar-imms/telemetry-agent/internal/dependency"
)

func TestNewInstallerComposesRootReconciliationAndFreshInstall(
	t *testing.T,
) {
	var steps []string

	install := newInstaller(
		Options{
			InstallDir:    "/opt/ar-imms/node-exporter",
			ServicePath:   "/etc/systemd/system/ar-imms-node-exporter.service",
			ListenAddress: "127.0.0.1:9100",
		},
		installerDependencies{
			effectiveUserID: func() int {
				steps = append(steps, "root")
				return 0
			},
			inspect: func(
				ctx context.Context,
			) (installationState, error) {
				steps = append(steps, "inspect")
				return installationState{}, nil
			},
			stageArchive: func(
				ctx context.Context,
			) (string, func(), error) {
				steps = append(steps, "stage")

				return "/temporary/node-exporter.tar.gz", func() {
					steps = append(steps, "cleanup")
				}, nil
			},
			installArchive: func(archivePath string) error {
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
				steps = append(steps, "start")
				return nil
			},
			waitForHealth: func(
				ctx context.Context,
				endpoint string,
			) error {
				steps = append(steps, "health")
				return nil
			},
			startupTimeout: time.Second,
		},
	)

	result, err := install(context.Background())
	if err != nil {
		t.Fatalf("installer error = %v", err)
	}

	wantResult := dependency.InstallResult{
		Name: "node-exporter",
	}
	if !reflect.DeepEqual(result, wantResult) {
		t.Fatalf("result = %#v, want %#v", result, wantResult)
	}

	wantSteps := []string{
		"root",
		"inspect",
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
