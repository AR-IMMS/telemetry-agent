package librehardwaremonitor

import (
	"context"
	"io"
	"reflect"
	"testing"
	"time"

	"github.com/ar-imms/telemetry-agent/internal/dependency"
)

func TestNewInstallerComposesAdminReconcileAndFreshInstall(t *testing.T) {
	var steps []string

	install := newInstaller(
		DefaultOptions(),
		installerDependencies{
			administrator: func() (bool, error) {
				steps = append(steps, "admin")
				return true, nil
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

				return `C:\Temp\LibreHardwareMonitor.zip`, func() {
					steps = append(steps, "cleanup")
				}, nil
			},
			installArchive: func(
				archivePath string,
				installDir string,
			) error {
				steps = append(steps, "install")
				return nil
			},
			writeConfig: func(options Options) error {
				steps = append(steps, "config")
				return nil
			},
			configureFirewall: func(
				ctx context.Context,
				options Options,
			) error {
				steps = append(steps, "firewall")
				return nil
			},
			registerTask: func(
				ctx context.Context,
				options Options,
			) error {
				steps = append(steps, "task")
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
		Name: "libre-hardware-monitor",
	}
	if !reflect.DeepEqual(result, wantResult) {
		t.Fatalf("result = %#v, want %#v", result, wantResult)
	}

	wantSteps := []string{
		"admin",
		"inspect",
		"stage",
		"install",
		"config",
		"firewall",
		"task",
		"health",
		"cleanup",
	}
	if !reflect.DeepEqual(steps, wantSteps) {
		t.Fatalf("steps = %v, want %v", steps, wantSteps)
	}
}

func TestNewInstallerReturnsDependencyInstaller(t *testing.T) {
	installer := NewInstaller(
		DefaultOptions(),
		testDownloader(func(
			ctx context.Context,
			url string,
			writer io.Writer,
		) error {
			return nil
		}),
	)

	if installer == nil {
		t.Fatal("NewInstaller() = nil, want dependency installer")
	}
}
