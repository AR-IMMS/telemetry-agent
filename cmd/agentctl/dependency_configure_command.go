package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/ar-imms/telemetry-agent/internal/agentstate"
	"io"
	"os"
	"strings"
)

func runDependencyConfigure(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer, deps dependencies) int {

	names, statePath, err := parseDependencyInstallArguments(args[1:])
	if err != nil {
		fmt.Fprintf(stderr, "usage error: %v\n\n", err)
		writeDependencyHelp(stderr)

		return 2
	}
	if len(names) != 0 {
		fmt.Fprintln(
			stderr,
			"usage error: dependency configure does not accept a dependency name",
		)
		fmt.Fprintln(stderr)
		writeDependencyHelp(stderr)

		return 2
	}

	if deps.listDependencies == nil {
		fmt.Fprintln(
			stderr,
			"configure dependency: lister is not configured",
		)

		return 1
	}
	if deps.configureDependencies == nil {
		fmt.Fprintln(
			stderr,
			"configure dependency: selector is not configured",
		)

		return 1
	}
	if deps.manageDependencyLifecycle == nil {
		fmt.Fprintln(
			stderr,
			"configure dependency: lifecycle manager is not configured",
		)

		return 1
	}

	state, err := agentstate.NewStore(statePath).Load()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			fmt.Fprintln(
				stderr,
				"configure dependency: no Agent lifecycle state found",
			)

			return 1
		}

		fmt.Fprintf(
			stderr,
			"load dependency lifecycle state: %v\n",
			err,
		)

		return 1
	}

	definitions, err := deps.listDependencies(ctx)
	if err != nil {
		fmt.Fprintf(stderr, "configure dependency: %v\n", err)

		return 1
	}

	options := make(
		[]dependencyConfigureOption,
		0,
		len(definitions),
	)
	for _, definition := range definitions {
		name := strings.ToLower(strings.TrimSpace(definition.Name))
		dependencyState, managed := state.Dependencies[name]
		if !managed {
			continue
		}

		options = append(options, dependencyConfigureOption{
			Definition: definition,
			Enabled:    dependencyState.Enabled,
		})
	}

	if len(options) == 0 {
		fmt.Fprintln(
			stdout,
			"No managed dependencies are available to configure.",
		)

		return 0
	}

	desiredStates, err := deps.configureDependencies(
		ctx,
		options,
		stdout,
	)
	if errors.Is(err, errDependencyConfigurationCancelled) {
		fmt.Fprintln(stdout, "Dependency configuration cancelled.")

		return 0
	}
	if err != nil {
		fmt.Fprintf(stderr, "configure dependency: %v\n", err)

		return 1
	}

	if len(desiredStates) != len(options) {
		fmt.Fprintln(
			stderr,
			"configure dependency: selector returned incomplete desired state",
		)

		return 1
	}

	for _, option := range options {
		desiredEnabled, present := desiredStates[option.Definition.Name]
		if !present {
			fmt.Fprintf(
				stderr,
				"configure dependency: selector omitted %q\n",
				option.Definition.Name,
			)

			return 1
		}
		if desiredEnabled == option.Enabled {
			continue
		}

		action := dependencyLifecycleEnable
		if !desiredEnabled {
			action = dependencyLifecycleDisable
		}

		if err := deps.manageDependencyLifecycle(
			ctx,
			option.Definition.Name,
			statePath,
			action,
		); err != nil {
			fmt.Fprintf(stderr, "configure dependency: %v\n", err)

			return 1
		}

		if action == dependencyLifecycleEnable {
			fmt.Fprintf(
				stdout,
				"Dependency enabled: %s\n",
				option.Definition.Name,
			)

			continue
		}

		fmt.Fprintf(
			stdout,
			"Dependency disable scheduled: %s\n",
			option.Definition.Name,
		)
	}

	return 0

}
