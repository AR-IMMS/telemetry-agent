package main

import (
	"context"
	"errors"
	"fmt"
	"io"

	tea "github.com/charmbracelet/bubbletea"
)

var errDependencyConfigurationCancelled = errors.New(
	"dependency configuration cancelled",
)

type dependencyConfigureFunc func(
	context.Context,
	[]dependencyConfigureOption,
	io.Writer,
) (map[string]bool, error)

// dependencyConfigureRunner runs the TUI and returns its final state.
type dependencyConfigureRunner func(
	dependencyConfigureTUI,
	io.Writer,
) (dependencyConfigureTUI, error)

// newTerminalDependencyConfigurer adapts the configure TUI to the CLI
// dependency-configuration contract.
func newTerminalDependencyConfigurer(
	isTerminal func() bool,
	run dependencyConfigureRunner,
) dependencyConfigureFunc {
	return func(
		_ context.Context,
		options []dependencyConfigureOption,
		output io.Writer,
	) (map[string]bool, error) {
		if isTerminal == nil || !isTerminal() {
			return nil, fmt.Errorf(
				"interactive dependency configuration requires a terminal",
			)
		}
		if run == nil {
			return nil, fmt.Errorf(
				"interactive dependency configuration runner is not configured",
			)
		}

		model, err := run(newDependencyConfigureTUI(options), output)
		if err != nil {
			return nil, fmt.Errorf(
				"run interactive dependency configuration: %w",
				err,
			)
		}

		if model.cancelled {
			return nil, errDependencyConfigurationCancelled
		}
		if !model.complete {
			return nil, fmt.Errorf(
				"interactive dependency configuration did not complete",
			)
		}

		return model.DesiredStates(), nil
	}
}

// runDependencyConfigureProgram runs the Bubble Tea program with injected
// streams so the production CLI and headless tests use the same behavior.
func runDependencyConfigureProgram(
	input io.Reader,
) dependencyConfigureRunner {
	return func(
		model dependencyConfigureTUI,
		output io.Writer,
	) (dependencyConfigureTUI, error) {
		if input == nil {
			return dependencyConfigureTUI{}, fmt.Errorf(
				"interactive dependency configuration input is required",
			)
		}
		if output == nil {
			return dependencyConfigureTUI{}, fmt.Errorf(
				"interactive dependency configuration output is required",
			)
		}

		program := tea.NewProgram(
			model,
			tea.WithInput(input),
			tea.WithOutput(output),
		)

		finalModel, err := program.Run()
		if err != nil {
			return dependencyConfigureTUI{}, fmt.Errorf(
				"run dependency configure program: %w",
				err,
			)
		}

		result, ok := finalModel.(dependencyConfigureTUI)
		if !ok {
			return dependencyConfigureTUI{}, fmt.Errorf(
				"dependency configure returned unexpected model type %T",
				finalModel,
			)
		}

		return result, nil
	}
}
