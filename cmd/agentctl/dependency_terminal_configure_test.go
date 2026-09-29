package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/ar-imms/telemetry-agent/internal/dependency"
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
			model dependencyConfigureTUI,
			output io.Writer,
		) (dependencyConfigureTUI, error) {
			runCalled = true
			model.selection.ToggleCurrent()
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
			model dependencyConfigureTUI,
			output io.Writer,
		) (dependencyConfigureTUI, error) {
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

func TestRunDependencyConfigureProgramConfirmsDesiredStates(
	t *testing.T,
) {
	var output bytes.Buffer

	run := runDependencyConfigureProgram(strings.NewReader(" \r"))

	model, err := run(
		newDependencyConfigureTUI(
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
