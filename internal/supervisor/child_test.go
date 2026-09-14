package supervisor

import (
	"os"
	"os/exec"
	"reflect"
	"testing"
)

func TestLaunchOptionsValidateRejectsMissingRequiredPaths(t *testing.T) {
	tests := []struct {
		name    string
		options LaunchOptions
	}{
		{
			name: "missing Collector binary path",
			options: LaunchOptions{
				ConfigPath: "C:/agent/config/otel.yaml",
			},
		},
		{
			name: "missing Collector config path",
			options: LaunchOptions{
				BinaryPath: "C:/agent/bin/otelcol-contrib.exe",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.options.validate(); err == nil {
				t.Fatal("LaunchOptions.validate() error = nil, want an error")
			}
		})
	}
}

func TestExecStarterStartsCollectorWithFinalConfig(t *testing.T) {
	if os.Getenv("GO_WANT_SUPERVISOR_HELPER") == "1" {
		os.Exit(0)
	}

	t.Setenv("GO_WANT_SUPERVISOR_HELPER", "1")

	var gotBinaryPath string
	var gotArguments []string

	starter := &execStarter{
		newCommand: func(
			binaryPath string,
			arguments ...string,
		) *exec.Cmd {
			gotBinaryPath = binaryPath
			gotArguments = append([]string(nil), arguments...)

			return exec.Command(
				os.Args[0],
				"-test.run=TestExecStarterStartsCollectorWithFinalConfig",
				"--",
			)
		},
	}

	child, err := starter.Start(LaunchOptions{
		BinaryPath: "C:/agent/bin/otelcol-contrib.exe",
		ConfigPath: "C:/agent/config/otel.yaml",
	})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	if got, want := gotBinaryPath, "C:/agent/bin/otelcol-contrib.exe"; got != want {
		t.Fatalf("Collector binary path = %q, want %q", got, want)
	}

	if want := []string{"--config", "C:/agent/config/otel.yaml"}; !reflect.DeepEqual(gotArguments, want) {
		t.Fatalf("Collector arguments = %q, want %q", gotArguments, want)
	}

	result, ok := <-child.Wait()
	if !ok {
		t.Fatal("Child.Wait() channel closed without an exit result")
	}

	if result.Err != nil {
		t.Fatalf("child exit error = %v", result.Err)
	}

	if result.Code != 0 {
		t.Fatalf("child exit code = %d, want 0", result.Code)
	}
}

func TestExecStarterPassesLaunchEnvironmentToCollector(t *testing.T) {
	if os.Getenv("GO_WANT_SUPERVISOR_ENV_HELPER") == "1" {
		if got, want := os.Getenv("OTEL_GATEWAY_ENDPOINT"), "gateway.example:4317"; got != want {
			os.Exit(1)
		}

		os.Exit(0)
	}

	t.Setenv("GO_WANT_SUPERVISOR_ENV_HELPER", "1")

	starter := &execStarter{
		newCommand: func(
			binaryPath string,
			arguments ...string,
		) *exec.Cmd {
			return exec.Command(
				os.Args[0],
				"-test.run=TestExecStarterPassesLaunchEnvironmentToCollector",
				"--",
			)
		},
	}

	child, err := starter.Start(LaunchOptions{
		BinaryPath: "C:/agent/bin/otelcol-contrib.exe",
		ConfigPath: "C:/agent/config/otel.yaml",
		Environment: []string{
			"OTEL_GATEWAY_ENDPOINT=gateway.example:4317",
		},
	})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	result, ok := <-child.Wait()
	if !ok {
		t.Fatal("Child.Wait() channel closed without an exit result")
	}

	if result.Err != nil {
		t.Fatalf("child exit error = %v", result.Err)
	}

	if result.Code != 0 {
		t.Fatalf("child exit code = %d, want 0", result.Code)
	}
}

func TestExecChildKillDelegatesToPlatformForceKill(t *testing.T) {
	process, err := os.FindProcess(os.Getpid())
	if err != nil {
		t.Fatalf("os.FindProcess() error = %v", err)
	}

	command := &exec.Cmd{
		Process: process,
	}

	called := false

	child := &execChild{
		command: command,
		forceKill: func(got *exec.Cmd) error {
			called = true

			if got != command {
				t.Fatal("forceKill() received an unexpected command")
			}

			return nil
		},
	}

	if err := child.Kill(); err != nil {
		t.Fatalf("Kill() error = %v", err)
	}

	if !called {
		t.Fatal("Kill() did not delegate to platform forceKill")
	}
}
