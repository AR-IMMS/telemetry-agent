package nodeexporter

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/ar-imms/telemetry-agent/internal/agentstate"
)

func TestNodeExporterEnablerStartsOwnedSystemdUnit(
	t *testing.T,
) {
	options := DefaultOptions()
	var commands []systemdCommand

	enabler := nodeExporterEnabler{
		options: options,
		runSystemd: func(
			_ context.Context,
			command systemdCommand,
		) error {
			commands = append(commands, command)

			return nil
		},
	}

	err := enabler.Enable(
		context.Background(),
		[]agentstate.OwnedResource{
			{
				Kind:       "systemd-unit",
				Identifier: options.ServicePath,
			},
		},
	)
	if err != nil {
		t.Fatalf("Enable() error = %v", err)
	}

	want := []systemdCommand{
		{
			Executable: "systemctl",
			Args: []string{
				"enable",
				"--now",
				filepath.Base(options.ServicePath),
			},
		},
	}

	if !reflect.DeepEqual(commands, want) {
		t.Fatalf("systemd commands = %#v, want %#v", commands, want)
	}
}

func TestNodeExporterEnablerRejectsMissingOwnedSystemdUnit(
	t *testing.T,
) {
	called := false

	enabler := nodeExporterEnabler{
		options: DefaultOptions(),
		runSystemd: func(
			context.Context,
			systemdCommand,
		) error {
			called = true

			return nil
		},
	}

	err := enabler.Enable(
		context.Background(),
		[]agentstate.OwnedResource{
			{
				Kind:       "directory",
				Identifier: enabler.options.InstallDir,
			},
		},
	)

	if err == nil {
		t.Fatal("Enable() error = nil, want ownership error")
	}
	if called {
		t.Fatal("systemd runner was called without owned systemd unit")
	}
}

func TestNodeExporterEnablerRejectsNilContext(
	t *testing.T,
) {
	called := false
	options := DefaultOptions()

	enabler := nodeExporterEnabler{
		options: options,
		runSystemd: func(
			context.Context,
			systemdCommand,
		) error {
			called = true

			return nil
		},
	}

	err := enabler.Enable(
		nil,
		[]agentstate.OwnedResource{
			{
				Kind:       "systemd-unit",
				Identifier: options.ServicePath,
			},
		},
	)

	if err == nil {
		t.Fatal("Enable() error = nil, want nil-context error")
	}
	if called {
		t.Fatal("systemd runner was called with nil context")
	}
}
