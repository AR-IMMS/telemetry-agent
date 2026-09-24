package main

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/ar-imms/telemetry-agent/internal/dependency"
	tea "github.com/charmbracelet/bubbletea"
)

// dependencyMultiSelectRunner runs the TUI and returns its final state.
type dependencyMultiSelectRunner func(
	dependencyMultiSelectTUI,
	io.Writer,
) (dependencyMultiSelectTUI, error)

var errDependencySelectionCancelled = errors.New(
	"dependency selection cancelled",
)

// newTerminalDependencyMultiSelector adapts the multi-select TUI to the CLI
// dependency-selection contract.
func newTerminalDependencyMultiSelector(
	isTerminal func() bool,
	run dependencyMultiSelectRunner,
) dependencySelectionFunc {
	return func(
		_ context.Context,
		definitions []dependency.Definition,
		output io.Writer,
	) ([]string, error) {
		if isTerminal == nil || !isTerminal() {
			return nil, fmt.Errorf(
				"interactive dependency selection requires a terminal; specify a dependency name",
			)
		}
		if run == nil {
			return nil, fmt.Errorf(
				"interactive dependency selector runner is not configured",
			)
		}

		model, err := run(
			newDependencyMultiSelectTUI(definitions),
			output,
		)
		if err != nil {
			return nil, fmt.Errorf(
				"run interactive dependency selector: %w",
				err,
			)
		}

		if model.cancelled {
			return nil, errDependencySelectionCancelled
		}
		if !model.complete {
			return nil, fmt.Errorf(
				"interactive dependency selection did not complete",
			)
		}

		names, err := model.selection.Confirm()
		if err != nil {
			return nil, fmt.Errorf(
				"confirm dependency selection: %w",
				err,
			)
		}

		return names, nil
	}
}

// runDependencyMultiSelectProgram runs the Bubble Tea program with injected
// streams so the production CLI and headless tests use the same behavior.
func runDependencyMultiSelectProgram(
	input io.Reader,
) dependencyMultiSelectRunner {
	return func(
		model dependencyMultiSelectTUI,
		output io.Writer,
	) (dependencyMultiSelectTUI, error) {
		if input == nil {
			return dependencyMultiSelectTUI{}, fmt.Errorf(
				"interactive dependency selector input is required",
			)
		}
		if output == nil {
			return dependencyMultiSelectTUI{}, fmt.Errorf(
				"interactive dependency selector output is required",
			)
		}

		program := tea.NewProgram(
			model,
			tea.WithInput(input),
			tea.WithOutput(output),
		)

		finalModel, err := program.Run()
		if err != nil {
			return dependencyMultiSelectTUI{}, fmt.Errorf(
				"run dependency multi-select program: %w",
				err,
			)
		}

		result, ok := finalModel.(dependencyMultiSelectTUI)
		if !ok {
			return dependencyMultiSelectTUI{}, fmt.Errorf(
				"dependency multi-select returned unexpected model type %T",
				finalModel,
			)
		}

		return result, nil
	}
}
