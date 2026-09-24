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

func TestTerminalDependencyMultiSelectorReturnsConfirmedNames(
	t *testing.T,
) {
	runCalled := false

	selector := newTerminalDependencyMultiSelector(
		func() bool {
			return true
		},
		func(
			model dependencyMultiSelectTUI,
			output io.Writer,
		) (dependencyMultiSelectTUI, error) {
			runCalled = true
			model.selection.ToggleCurrent()
			model.complete = true

			return model, nil
		},
	)

	names, err := selector(
		context.Background(),
		[]dependency.Definition{
			{Name: "windows-exporter"},
		},
		io.Discard,
	)
	if err != nil {
		t.Fatalf("terminal multi-selector error = %v", err)
	}

	if !runCalled {
		t.Fatal("terminal multi-select runner was not called")
	}

	if got, want := names,
		[]string{"windows-exporter"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("selected names = %v, want %v", got, want)
	}
}

func TestTerminalDependencyMultiSelectorReturnsCancellation(
	t *testing.T,
) {
	selector := newTerminalDependencyMultiSelector(
		func() bool {
			return true
		},
		func(
			model dependencyMultiSelectTUI,
			output io.Writer,
		) (dependencyMultiSelectTUI, error) {
			model.cancelled = true

			return model, nil
		},
	)

	_, err := selector(
		context.Background(),
		[]dependency.Definition{
			{Name: "windows-exporter"},
		},
		io.Discard,
	)

	if !errors.Is(err, errDependencySelectionCancelled) {
		t.Fatalf(
			"terminal multi-selector error = %v, want cancellation",
			err,
		)
	}
}

func TestRunDependencyMultiSelectProgramConfirmsSelection(
	t *testing.T,
) {
	var output bytes.Buffer

	run := runDependencyMultiSelectProgram(
		strings.NewReader(" \r"),
	)

	model, err := run(
		newDependencyMultiSelectTUI(
			[]dependency.Definition{
				{
					Name:        "windows-exporter",
					DisplayName: "Windows Exporter",
				},
			},
		),
		&output,
	)
	if err != nil {
		t.Fatalf("runDependencyMultiSelectProgram() error = %v", err)
	}

	if !model.complete {
		t.Fatal("model.complete = false, want true")
	}

	names, err := model.selection.Confirm()
	if err != nil {
		t.Fatalf("Confirm() error = %v", err)
	}

	want := []string{"windows-exporter"}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("selected names = %v, want %v", names, want)
	}
}
