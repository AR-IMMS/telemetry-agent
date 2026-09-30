package agentinstallation

import (
	"context"
	"os"
	"reflect"
	"testing"

	"github.com/ar-imms/telemetry-agent/internal/identity"
)

func TestSystemdServiceInstallerWritesReloadsEnablesAndStarts(
	t *testing.T,
) {
	layout, err := DefaultLayout(identity.PlatformInfo{
		OS: "linux",
	})
	if err != nil {
		t.Fatalf("DefaultLayout() error = %v", err)
	}

	var gotPath string
	var gotContent []byte
	var gotMode os.FileMode
	var commands []systemdCommand

	installer := systemdServiceInstaller{
		unitDirectory: "/test/systemd",
		writeFile: func(
			path string,
			content []byte,
			mode os.FileMode,
		) error {
			gotPath = path
			gotContent = append([]byte(nil), content...)
			gotMode = mode

			return nil
		},
		run: func(
			_ context.Context,
			command systemdCommand,
		) error {
			commands = append(commands, command)

			return nil
		},
	}

	if err := installer.Install(context.Background(), layout); err != nil {
		t.Fatalf("Install() error = %v", err)
	}

	if got, want := gotPath,
		"/test/systemd/ar-imms-telemetry-agent.service"; got != want {
		t.Fatalf("unit path = %q, want %q", got, want)
	}
	if got, want := gotMode, os.FileMode(0o644); got != want {
		t.Fatalf("unit mode = %o, want %o", got, want)
	}

	wantUnit, err := RenderSystemdUnit(layout)
	if err != nil {
		t.Fatalf("RenderSystemdUnit() error = %v", err)
	}
	if got, want := string(gotContent), wantUnit; got != want {
		t.Fatalf("unit content = %q, want %q", got, want)
	}

	wantCommands := []systemdCommand{
		{
			Executable: "systemctl",
			Args:       []string{"daemon-reload"},
		},
		{
			Executable: "systemctl",
			Args: []string{
				"enable",
				"ar-imms-telemetry-agent.service",
			},
		},
		{
			Executable: "systemctl",
			Args: []string{
				"restart",
				"ar-imms-telemetry-agent.service",
			},
		},
	}

	if !reflect.DeepEqual(commands, wantCommands) {
		t.Fatalf("commands = %#v, want %#v", commands, wantCommands)
	}
}
