package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"

	"github.com/charmbracelet/huh"
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

type dependencyConfigureFormRunner func(*huh.Form) error

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

// runDependencyConfigureProgram runs the Huh form with injected streams. The
// caller gates this explicitly interactive flow on a real terminal.
func runDependencyConfigureProgram(
	input io.Reader,
) dependencyConfigureRunner {
	return runDependencyConfigureProgramWithFormRunner(input, func(form *huh.Form) error {
		return form.Run()
	})
}

func runDependencyConfigureProgramWithFormRunner(
	input io.Reader,
	runForm dependencyConfigureFormRunner,
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
		if runForm == nil {
			return dependencyConfigureTUI{}, fmt.Errorf(
				"dependency configure form runner is required",
			)
		}

		selected := make([]string, 0, len(model.options))
		choices := make([]huh.Option[string], 0, len(model.options))
		for _, option := range model.options {
			name := option.Definition.Name
			if option.Enabled {
				selected = append(selected, name)
			}
			displayName := option.Definition.DisplayName
			if displayName == "" {
				displayName = name
			}
			choices = append(choices, huh.NewOption(displayName, name))
		}

		field := huh.NewMultiSelect[string]().
			Title("Configure managed dependencies").
			Description("Select enabled dependencies, then confirm.").
			Options(choices...).
			Filterable(false).
			Value(&selected)
		form := huh.NewForm(huh.NewGroup(field)).
			WithInput(input).
			WithOutput(output)
		if err := runForm(form); errors.Is(err, huh.ErrUserAborted) {
			model.cancelled = true
			return model, nil
		} else if err != nil {
			return dependencyConfigureTUI{}, fmt.Errorf(
				"run dependency configure form: %w",
				err,
			)
		}

		for index := range model.options {
			model.selection.selected[index] = slices.Contains(selected, model.options[index].Definition.Name)
		}
		model.complete = true
		return model, nil
	}
}
