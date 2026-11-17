package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/ar-imms/telemetry-agent/internal/agentstate"
	dependencycommands "github.com/ar-imms/telemetry-agent/internal/cli/commands/dependency"
	dependencydomain "github.com/ar-imms/telemetry-agent/internal/runtime/dependencies/domain"
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
		return dependencycommands.Status(ctx, stdout, stderr, nil, nil)
	}

	state := agentstate.State{
		Dependencies: make(map[string]dependencydomain.DependencyState),
	}

	loadedState, err := newAgentStateStore(statePath).Load()
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

	managedNames := make(map[string]struct{}, len(state.Dependencies))
	for name := range state.Dependencies {
		managedNames[strings.ToLower(strings.TrimSpace(name))] = struct{}{}
	}

	return dependencycommands.Status(ctx, stdout, stderr, deps.listDependencyStatuses, managedNames)

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

	store := newAgentStateStore(statePath)

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
