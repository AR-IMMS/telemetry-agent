package bootstrap

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

type activationRunner struct {
	command Command
	result  CommandResult
}

func (r *activationRunner) Run(
	_ context.Context,
	command Command,
) CommandResult {
	r.command = command

	return r.result
}

func TestActivateRenderedConfigPreservesExistingConfigWhenValidationFails(
	t *testing.T,
) {
	targetPath := filepath.Join(
		t.TempDir(),
		"config",
		"otel.yaml",
	)

	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	if err := os.WriteFile(
		targetPath,
		[]byte("old configuration"),
		0o640,
	); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	runner := &activationRunner{
		result: CommandResult{
			ExitCode: 1,
			Stderr:   "invalid Collector configuration",
			Err:      errors.New("validation failed"),
		},
	}

	err := ActivateRenderedConfig(
		context.Background(),
		runner,
		"/fake/otelcol-contrib",
		targetPath,
		[]byte("new configuration"),
		nil,
	)
	if err == nil {
		t.Fatal("ActivateRenderedConfig() error = nil, want error")
	}

	got, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	if string(got) != "old configuration" {
		t.Fatalf(
			"active configuration = %q, want %q",
			got,
			"old configuration",
		)
	}
}

func TestActivateRenderedConfigReplacesExistingConfigAfterValidation(
	t *testing.T,
) {
	targetPath := filepath.Join(
		t.TempDir(),
		"config",
		"otel.yaml",
	)

	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	if err := os.WriteFile(
		targetPath,
		[]byte("old configuration"),
		0o640,
	); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	runner := &activationRunner{
		result: CommandResult{
			ExitCode: 0,
		},
	}

	if err := ActivateRenderedConfig(
		context.Background(),
		runner,
		"/fake/otelcol-contrib",
		targetPath,
		[]byte("new configuration"),
		[]string{"OTEL_GATEWAY_ENDPOINT=127.0.0.1:4317"},
	); err != nil {
		t.Fatalf("ActivateRenderedConfig() error = %v", err)
	}

	got, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	if string(got) != "new configuration" {
		t.Fatalf(
			"active configuration = %q, want %q",
			got,
			"new configuration",
		)
	}

	if runner.command.Executable != "/fake/otelcol-contrib" {
		t.Fatalf(
			"command executable = %q, want %q",
			runner.command.Executable,
			"/fake/otelcol-contrib",
		)
	}

	if len(runner.command.Args) != 3 {
		t.Fatalf(
			"command args = %v, want validate command arguments",
			runner.command.Args,
		)
	}

	if runner.command.Args[0] != "validate" ||
		runner.command.Args[1] != "--config" {
		t.Fatalf(
			"command args prefix = %v, want [validate --config]",
			runner.command.Args,
		)
	}

	stagedPath := runner.command.Args[2]

	if stagedPath == targetPath {
		t.Fatal("Collector validated active config instead of staged config")
	}

	if filepath.Base(stagedPath) != filepath.Base(targetPath) {
		t.Fatalf(
			"staged config filename = %q, want %q",
			filepath.Base(stagedPath),
			filepath.Base(targetPath),
		)
	}

	if len(runner.command.Environment) != 1 ||
		runner.command.Environment[0] !=
			"OTEL_GATEWAY_ENDPOINT=127.0.0.1:4317" {
		t.Fatalf(
			"command environment = %v, want validation environment",
			runner.command.Environment,
		)
	}
}
