package nodeexporter

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type fakeSystemdProcess struct {
	output []byte
	err    error
	called bool
}

func (p *fakeSystemdProcess) CombinedOutput() ([]byte, error) {
	p.called = true

	return p.output, p.err
}

func TestOSSystemdRunnerExecutesProvidedCommand(t *testing.T) {
	var gotExecutable string
	var gotArgs []string

	process := &fakeSystemdProcess{}

	runner := osSystemdRunner{
		newCommand: func(
			ctx context.Context,
			executable string,
			args ...string,
		) systemdProcess {
			gotExecutable = executable
			gotArgs = append([]string(nil), args...)

			return process
		},
	}

	err := runner.Run(
		context.Background(),
		systemdCommand{
			Executable: "systemctl",
			Args:       []string{"daemon-reload"},
		},
	)
	if err != nil {
		t.Fatalf("Run() error = %v, want nil", err)
	}
	if !process.called {
		t.Fatal("CombinedOutput() was not called")
	}
	if gotExecutable != "systemctl" {
		t.Fatalf("executable = %q, want systemctl", gotExecutable)
	}
	if strings.Join(gotArgs, " ") != "daemon-reload" {
		t.Fatalf("arguments = %q, want daemon-reload", gotArgs)
	}
}

func TestOSSystemdRunnerPreservesCommandDiagnostics(t *testing.T) {
	runner := osSystemdRunner{
		newCommand: func(
			ctx context.Context,
			executable string,
			args ...string,
		) systemdProcess {
			return &fakeSystemdProcess{
				output: []byte(
					"Unit ar-imms-node-exporter.service not found.\n",
				),
				err: errors.New("exit status 5"),
			}
		},
	}

	err := runner.Run(
		context.Background(),
		systemdCommand{
			Executable: "systemctl",
			Args: []string{
				"enable",
				"--now",
				"ar-imms-node-exporter.service",
			},
		},
	)

	if err == nil {
		t.Fatal("Run() error = nil, want command failure")
	}
	if !strings.Contains(
		err.Error(),
		"Unit ar-imms-node-exporter.service not found.",
	) {
		t.Fatalf("Run() error = %v, want systemd diagnostics", err)
	}
}

func TestOSSystemdRunnerReturnsCommandOutput(
	t *testing.T,
) {
	process := &fakeSystemdProcess{
		output: []byte("enabled\n"),
	}

	runner := osSystemdRunner{
		newCommand: func(
			ctx context.Context,
			executable string,
			args ...string,
		) systemdProcess {
			if executable != "systemctl" {
				t.Fatalf(
					"executable = %q, want systemctl",
					executable,
				)
			}

			return process
		},
	}

	output, err := runner.Output(
		context.Background(),
		systemdCommand{
			Executable: "systemctl",
			Args: []string{
				"show",
				"--property=UnitFileState",
				"--value",
				"ar-imms-node-exporter.service",
			},
		},
	)
	if err != nil {
		t.Fatalf("Output() error = %v", err)
	}
	if !process.called {
		t.Fatal("CombinedOutput() was not called")
	}
	if string(output) != "enabled\n" {
		t.Fatalf("output = %q, want enabled state", output)
	}
}
