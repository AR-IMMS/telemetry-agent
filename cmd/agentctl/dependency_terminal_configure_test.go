package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/charmbracelet/huh"

	"github.com/ar-imms/telemetry-agent/internal/runtime/dependencies"
)

func TestTerminalDependencyConfigurerReturnsConfirmedDesiredStates(
	t *testing.T,
) {
	runCalled := false

	configurer := newTerminalDependencyConfigurer(
		func() bool {
			return true
		},
		func(
			model dependencyConfigureSelection,
			output io.Writer,
		) (dependencyConfigureSelection, error) {
			runCalled = true
			model.desiredStates["windows-exporter"] = false
			model.complete = true

			return model, nil
		},
	)

	states, err := configurer(
		context.Background(),
		[]dependencyConfigureOption{
			{
				Definition: dependency.Definition{
					Name: "windows-exporter",
				},
				Enabled: true,
			},
		},
		io.Discard,
	)
	if err != nil {
		t.Fatalf("terminal configurer error = %v", err)
	}
	if !runCalled {
		t.Fatal("terminal configure runner was not called")
	}

	want := map[string]bool{
		"windows-exporter": false,
	}
	if !reflect.DeepEqual(states, want) {
		t.Fatalf("desired states = %#v, want %#v", states, want)
	}
}

func TestTerminalDependencyConfigurerReturnsCancellation(
	t *testing.T,
) {
	configurer := newTerminalDependencyConfigurer(
		func() bool {
			return true
		},
		func(
			model dependencyConfigureSelection,
			output io.Writer,
		) (dependencyConfigureSelection, error) {
			model.cancelled = true

			return model, nil
		},
	)

	_, err := configurer(
		context.Background(),
		[]dependencyConfigureOption{
			{
				Definition: dependency.Definition{
					Name: "windows-exporter",
				},
				Enabled: true,
			},
		},
		io.Discard,
	)

	if !errors.Is(err, errDependencyConfigurationCancelled) {
		t.Fatalf(
			"terminal configurer error = %v, want cancellation",
			err,
		)
	}
}

func TestTerminalDependencyConfigurerNeverRunsOutsideTTY(t *testing.T) {
	runCalled := false
	configurer := newTerminalDependencyConfigurer(
		func() bool { return false },
		func(model dependencyConfigureSelection, _ io.Writer) (dependencyConfigureSelection, error) {
			runCalled = true
			return model, nil
		},
	)
	_, err := configurer(
		context.Background(),
		[]dependencyConfigureOption{{Definition: dependency.Definition{Name: "redis"}}},
		io.Discard,
	)
	if err == nil || !strings.Contains(err.Error(), "requires a terminal") {
		t.Fatalf("non-TTY error = %v, want terminal requirement", err)
	}
	if runCalled {
		t.Fatal("interactive form ran without a terminal")
	}
}

func TestRunDependencyConfigureProgramConfirmsDesiredStates(
	t *testing.T,
) {
	var output bytes.Buffer

	run := runDependencyConfigureProgramWithFormRunner(
		strings.NewReader("1\n0\n"),
		func(form *huh.Form) error {
			return form.WithAccessible(true).Run()
		},
	)

	model, err := run(
		newDependencyConfigureSelection(
			[]dependencyConfigureOption{
				{
					Definition: dependency.Definition{
						Name:        "windows-exporter",
						DisplayName: "Windows Exporter",
					},
					Enabled: false,
				},
			},
		),
		&output,
	)
	if err != nil {
		t.Fatalf("runDependencyConfigureProgram() error = %v", err)
	}
	if !model.complete {
		t.Fatal("model.complete = false, want true")
	}

	want := map[string]bool{
		"windows-exporter": true,
	}
	if got := model.DesiredStates(); !reflect.DeepEqual(got, want) {
		t.Fatalf("desired states = %#v, want %#v", got, want)
	}
}
