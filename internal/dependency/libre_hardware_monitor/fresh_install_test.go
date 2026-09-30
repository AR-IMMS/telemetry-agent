package librehardwaremonitor

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/ar-imms/telemetry-agent/internal/agentstate"
	"github.com/ar-imms/telemetry-agent/internal/dependency"
)

func TestFreshInstallerStagesConfiguresStartsAndChecksLHM(t *testing.T) {
	var steps []string

	installer := freshInstaller{
		options:        DefaultOptions(),
		startupTimeout: time.Second,
		stageArchive: func(
			ctx context.Context,
		) (string, func(), error) {
			steps = append(steps, "stage")

			return `C:\Temp\LibreHardwareMonitor.zip`, func() {
				steps = append(steps, "cleanup")
			}, nil
		},
		installArchive: func(archivePath string, installDir string) error {
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
			if endpoint != "http://127.0.0.1:9190/metrics" {
				t.Fatalf(
					"health endpoint = %q, want local LHM metrics",
					endpoint,
				)
			}

			steps = append(steps, "health")
			return nil
		},
	}

	result, err := installer.Install(context.Background())
	if err != nil {
		t.Fatalf("freshInstaller.Install() error = %v", err)
	}

	wantResult := dependency.InstallResult{
		Name:   "libre-hardware-monitor",
		Reused: false,
		OwnedResources: []agentstate.OwnedResource{
			{
				Kind:       "scheduled-task",
				Identifier: installer.options.TaskName,
			},
			{
				Kind:       "firewall-rule",
				Identifier: installer.options.FirewallName,
			},
			{
				Kind:       "directory",
				Identifier: installer.options.InstallDir,
			},
		},
	}

	if !reflect.DeepEqual(result, wantResult) {
		t.Fatalf(
			"install result = %#v, want %#v",
			result,
			wantResult,
		)
	}

	wantSteps := []string{
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

func TestFreshInstallerUpdatesManagedResourcesWithoutArchiveDownload(
	t *testing.T,
) {
	var steps []string

	installer := freshInstaller{
		options:        DefaultOptions(),
		startupTimeout: time.Second,
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
	}

	if err := installer.Update(context.Background()); err != nil {
		t.Fatalf("freshInstaller.Update() error = %v", err)
	}

	wantSteps := []string{
		"config",
		"firewall",
		"task",
		"health",
	}

	if !reflect.DeepEqual(steps, wantSteps) {
		t.Fatalf("steps = %v, want %v", steps, wantSteps)
	}
}
