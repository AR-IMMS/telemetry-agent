package nodeexporter

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/ar-imms/telemetry-agent/internal/agentstate"
)

func TestNodeExporterTeardownDisablesOwnedSystemdUnit(
	t *testing.T,
) {
	options := DefaultOptions()
	var commands []systemdCommand

	teardown := nodeExporterTeardown{
		options: options,
		runSystemd: func(
			ctx context.Context,
			command systemdCommand,
		) error {
			commands = append(commands, command)

			return nil
		},
	}

	err := teardown.Teardown(
		context.Background(),
		agentstate.TeardownActionDisable,
		[]agentstate.OwnedResource{
			{
				Kind:       "systemd-unit",
				Identifier: options.ServicePath,
			},
			{
				Kind:       "directory",
				Identifier: options.InstallDir,
			},
		},
	)
	if err != nil {
		t.Fatalf("Teardown() error = %v", err)
	}

	want := []systemdCommand{
		{
			Executable: "systemctl",
			Args: []string{
				"disable",
				"--now",
				"ar-imms-node-exporter.service",
			},
		},
	}

	if !reflect.DeepEqual(commands, want) {
		t.Fatalf("systemd commands = %#v, want %#v", commands, want)
	}
}

func TestNodeExporterTeardownUninstallsOwnedResources(
	t *testing.T,
) {
	options := DefaultOptions()
	var steps []string

	teardown := nodeExporterTeardown{
		options: options,
		runSystemd: func(
			ctx context.Context,
			command systemdCommand,
		) error {
			steps = append(
				steps,
				"systemctl "+strings.Join(command.Args, " "),
			)

			return nil
		},
		removeUnit: func(path string) error {
			if path != options.ServicePath {
				t.Fatalf(
					"unit path = %q, want %q",
					path,
					options.ServicePath,
				)
			}

			steps = append(steps, "remove-unit")

			return nil
		},
		removeDirectory: func(path string) error {
			if path != options.InstallDir {
				t.Fatalf(
					"directory path = %q, want %q",
					path,
					options.InstallDir,
				)
			}

			steps = append(steps, "remove-directory")

			return nil
		},
	}

	err := teardown.Teardown(
		context.Background(),
		agentstate.TeardownActionUninstall,
		[]agentstate.OwnedResource{
			{
				Kind:       "systemd-unit",
				Identifier: options.ServicePath,
			},
			{
				Kind:       "directory",
				Identifier: options.InstallDir,
			},
		},
	)
	if err != nil {
		t.Fatalf("Teardown() error = %v", err)
	}

	want := []string{
		"systemctl disable --now ar-imms-node-exporter.service",
		"remove-unit",
		"remove-directory",
		"systemctl daemon-reload",
	}

	if !reflect.DeepEqual(steps, want) {
		t.Fatalf("teardown steps = %#v, want %#v", steps, want)
	}
}

func TestNodeExporterTeardownRejectsNilContext(
	t *testing.T,
) {
	options := DefaultOptions()
	runCalls := 0

	teardown := nodeExporterTeardown{
		options: options,
		runSystemd: func(
			_ context.Context,
			_ systemdCommand,
		) error {
			runCalls++

			return nil
		},
	}

	err := teardown.Teardown(
		nil,
		agentstate.TeardownActionDisable,
		[]agentstate.OwnedResource{
			{
				Kind:       "systemd-unit",
				Identifier: options.ServicePath,
			},
		},
	)

	if err == nil {
		t.Fatal("Teardown() error = nil, want nil-context error")
	}
	if !strings.Contains(
		err.Error(),
		"Node Exporter teardown context is required",
	) {
		t.Fatalf("Teardown() error = %v, want context error", err)
	}
	if runCalls != 0 {
		t.Fatalf("systemd runner calls = %d, want 0", runCalls)
	}
}
