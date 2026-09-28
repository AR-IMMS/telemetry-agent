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
	"github.com/ar-imms/telemetry-agent/internal/dependency"
)

func writeDependencyHelp(output io.Writer) {
	fmt.Fprint(output, `Usage:
  agentctl dependency <command>

Commands:
  list                               List dependencies available on this operating system.
  install [dependency-name]          Install by name or select in a terminal.
  status [--state-path <path>]       Show lifecycle status of managed dependencies.
  pending [--state-path <path>]      List scheduled dependency teardowns.
  disable <dependency-name> [--state-path <path>]
                                    Disable safely after Collector readiness.
  uninstall <dependency-name> [--state-path <path>]
                                    Uninstall safely after Collector readiness.
  help                              Show this help.
`)
}

func parseDependencyInstallArguments(
	args []string,
) ([]string, string, error) {
	statePath := agentstate.DefaultPath()
	names := make([]string, 0, 1)

	for index := 0; index < len(args); index++ {
		argument := args[index]

		switch {
		case argument == "--state-path":
			if index+1 == len(args) {
				return nil, "", fmt.Errorf(
					"--state-path requires a value",
				)
			}

			index++
			statePath = args[index]

		case strings.HasPrefix(argument, "--state-path="):
			statePath = strings.TrimPrefix(
				argument,
				"--state-path=",
			)

		case strings.HasPrefix(argument, "-"):
			return nil, "", fmt.Errorf(
				"unknown dependency install option %q",
				argument,
			)

		default:
			names = append(names, argument)
		}
	}

	if strings.TrimSpace(statePath) == "" {
		return nil, "", fmt.Errorf(
			"--state-path must not be empty",
		)
	}

	return names, statePath, nil
}

func runDependency(
	ctx context.Context,
	args []string,
	stdout io.Writer,
	stderr io.Writer,
	deps dependencies,
) int {
	if len(args) == 0 {
		fmt.Fprintln(
			stderr,
			"usage error: dependency command is required",
		)
		fmt.Fprintln(stderr)
		writeDependencyHelp(stderr)

		return 2
	}

	if len(args) == 1 && args[0] == "help" {
		writeDependencyHelp(stdout)

		return 0
	}

	if args[0] == "list" {
		if len(args) != 1 {
			fmt.Fprintln(
				stderr,
				"usage error: dependency list does not accept arguments",
			)
			fmt.Fprintln(stderr)
			writeDependencyHelp(stderr)

			return 2
		}

		if deps.listDependencies == nil {
			fmt.Fprintln(
				stderr,
				"list dependencies: lister is not configured",
			)

			return 1
		}

		definitions, err := deps.listDependencies(ctx)
		if err != nil {
			fmt.Fprintf(stderr, "list dependencies: %v\n", err)

			return 1
		}

		if len(definitions) == 0 {
			fmt.Fprintln(
				stdout,
				"No dependencies are available for this operating system.",
			)

			return 0
		}

		fmt.Fprintln(stdout, "Available dependencies:")
		for _, definition := range definitions {
			fmt.Fprintf(
				stdout,
				"- %s: %s — %s\n",
				definition.Name,
				definition.DisplayName,
				definition.Description,
			)
		}

		return 0
	}

	if args[0] == "status" {
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

		loadedState, err := agentstate.NewFileStore(statePath).Load()
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

	if args[0] == "pending" {
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

		store := agentstate.NewFileStore(statePath)

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

	if args[0] == "disable" || args[0] == "uninstall" {
		command := args[0]
		action := agentstate.TeardownActionDisable

		if command == "uninstall" {
			action = agentstate.TeardownActionUninstall
		}

		names, statePath, err := parseDependencyInstallArguments(args[1:])
		if err != nil {
			fmt.Fprintf(stderr, "usage error: %v\n\n", err)
			writeDependencyHelp(stderr)

			return 2
		}

		if len(names) != 1 {
			fmt.Fprintf(
				stderr,
				"usage error: dependency %s accepts exactly one dependency name\n",
				command,
			)
			fmt.Fprintln(stderr)
			writeDependencyHelp(stderr)

			return 2
		}

		if deps.manageDependencyLifecycle == nil {
			fmt.Fprintf(
				stderr,
				"%s dependency: lifecycle manager is not configured\n",
				command,
			)

			return 1
		}

		if err := deps.manageDependencyLifecycle(
			ctx,
			names[0],
			statePath,
			action,
		); err != nil {
			fmt.Fprintf(stderr, "%s dependency: %v\n", command, err)

			return 1
		}

		fmt.Fprintf(
			stdout,
			"Dependency %s scheduled: %s\n",
			command,
			names[0],
		)

		return 0
	}

	if args[0] != "install" {
		fmt.Fprintf(
			stderr,
			"usage error: unknown dependency command %q\n\n",
			args[0],
		)
		writeDependencyHelp(stderr)

		return 2
	}

	names, statePath, err := parseDependencyInstallArguments(
		args[1:],
	)
	if err != nil {
		fmt.Fprintf(stderr, "usage error: %v\n\n", err)
		writeDependencyHelp(stderr)

		return 2
	}

	if len(names) == 0 {
		if deps.listDependencies == nil {
			fmt.Fprintln(
				stderr,
				"install dependency: lister is not configured",
			)

			return 1
		}

		definitions, err := deps.listDependencies(ctx)
		if err != nil {
			fmt.Fprintf(stderr, "install dependency: %v\n", err)

			return 1
		}

		if deps.selectDependencies == nil {
			fmt.Fprintln(
				stderr,
				"install dependency: selector is not configured",
			)

			return 1
		}

		names, err = deps.selectDependencies(
			ctx,
			definitions,
			stdout,
		)
		if errors.Is(err, errDependencySelectionCancelled) {
			fmt.Fprintln(stdout, "Dependency installation cancelled.")

			return 0
		}
		if err != nil {
			fmt.Fprintf(stderr, "install dependency: %v\n", err)

			return 1
		}
	} else if len(names) != 1 {
		fmt.Fprintln(
			stderr,
			"usage error: dependency install accepts exactly one dependency name",
		)
		fmt.Fprintln(stderr)
		writeDependencyHelp(stderr)

		return 2
	}

	if deps.manageDependency == nil &&
		deps.installDependency == nil {
		fmt.Fprintln(
			stderr,
			"install dependency: installer is not configured",
		)

		return 1
	}

	for _, name := range names {
		if deps.manageDependency != nil {
			result, err := deps.manageDependency(
				ctx,
				name,
				statePath,
			)
			if err != nil {
				fmt.Fprintf(stderr, "install dependency: %v\n", err)

				return 1
			}

			fmt.Fprintf(
				stdout,
				"Dependency installed: %s\n",
				result.Name,
			)

			continue
		}

		result, err := deps.installDependency(ctx, name)
		if err != nil {
			fmt.Fprintf(stderr, "install dependency: %v\n", err)

			return 1
		}

		fmt.Fprintf(stdout, "Dependency installed: %s\n", result.Name)
	}

	return 0
}
