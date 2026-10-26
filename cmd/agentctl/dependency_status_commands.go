package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/ar-imms/telemetry-agent/internal/agentstate"
	"github.com/ar-imms/telemetry-agent/internal/dependency"
	"io"
	"os"
	"sort"
	"strings"
)

func runDependencyStatus(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer, deps dependencies) int {

	names, statePath, err := parseDependencyInstallArguments(args[1:])
	if err != nil {
		fmt.Fprintf(stderr, "usage error: %v\n\n", err)
		writeDependencyHelp(stderr)

		return 2
	}
	if len(names) != 0 {
		fmt.Fprintln(
			stderr,
			"usage error: dependency status does not accept a dependency name",
		)
		fmt.Fprintln(stderr)
		writeDependencyHelp(stderr)

		return 2
	}

	if deps.listDependencyStatuses == nil {
		fmt.Fprintln(
			stderr,
			"dependency status: lister is not configured",
		)

		return 1
	}

	state := agentstate.State{
		Dependencies: make(map[string]agentstate.DependencyState),
	}

	loadedState, err := agentstate.NewStore(statePath).Load()
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			fmt.Fprintf(
				stderr,
				"load dependency lifecycle state: %v\n",
				err,
			)

			return 1
		}
	} else {
		state = loadedState
	}

	results, err := deps.listDependencyStatuses(ctx)
	if err != nil {
		fmt.Fprintf(stderr, "dependency status: %v\n", err)

		return 1
	}

	fmt.Fprintln(stdout, "Dependency status:")

	for _, result := range results {
		if result.InspectionError != nil {
			fmt.Fprintf(
				stdout,
				"- %s: unknown (%v)\n",
				result.Definition.Name,
				result.InspectionError,
			)

			continue
		}

		name := strings.ToLower(
			strings.TrimSpace(result.Definition.Name),
		)
		_, managed := state.Dependencies[name]

		if result.Status.Availability == dependency.AvailabilityDisabled &&
			!managed {
			fmt.Fprintf(
				stdout,
				"- %s: not installed\n",
				result.Definition.Name,
			)

			continue
		}

		condition := string(result.Status.Health)
		if result.Status.Drifted {
			condition += ", drifted"
		}

		metricsEndpoint := strings.TrimSpace(
			result.Definition.MetricsEndpoint,
		)
		if metricsEndpoint == "" {
			fmt.Fprintf(
				stdout,
				"- %s: %s (%s)\n",
				result.Definition.Name,
				result.Status.Availability,
				condition,
			)

			continue
		}

		fmt.Fprintf(
			stdout,
			"- %s: %s (%s) — metrics: %s\n",
			result.Definition.Name,
			result.Status.Availability,
			condition,
			metricsEndpoint,
		)
	}

	return 0

}

func runDependencyPending(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer, deps dependencies) int {

	names, statePath, err := parseDependencyInstallArguments(args[1:])
	if err != nil {
		fmt.Fprintf(stderr, "usage error: %v\n\n", err)
		writeDependencyHelp(stderr)

		return 2
	}

	if len(names) != 0 {
		fmt.Fprintln(
			stderr,
			"usage error: dependency pending does not accept a dependency name",
		)
		fmt.Fprintln(stderr)
		writeDependencyHelp(stderr)

		return 2
	}

	store := agentstate.NewStore(statePath)

	state, err := store.Load()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			fmt.Fprintln(stdout, "No Agent lifecycle state found.")

			return 0
		}

		fmt.Fprintf(stderr, "load dependency lifecycle state: %v\n", err)

		return 1
	}

	names = make([]string, 0, len(state.Dependencies))
	for name, dependency := range state.Dependencies {
		if dependency.PendingTeardown != nil {
			names = append(names, name)
		}
	}

	if len(names) == 0 {
		fmt.Fprintln(stdout, "No pending dependency teardowns.")

		return 0
	}

	sort.Strings(names)

	fmt.Fprintln(stdout, "Pending dependency teardowns:")
	for _, name := range names {
		pending := state.Dependencies[name].PendingTeardown

		fmt.Fprintf(
			stdout,
			"- %s: %s, generation %d (applied generation: %d)\n",
			name,
			pending.Action,
			pending.Generation,
			state.AppliedGeneration,
		)
	}

	return 0

}
