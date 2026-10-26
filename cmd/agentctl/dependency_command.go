package main

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/ar-imms/telemetry-agent/internal/agentstate"
)

func writeDependencyHelp(output io.Writer) {
	fmt.Fprint(output, `Usage:
  agentctl dependency <command>

Commands:
  list                               List dependencies available on this operating system.
  install [dependency-name]          Install by name or select in a terminal.
  status [--state-path <path>]       Show lifecycle status of managed dependencies.
  pending [--state-path <path>]      List scheduled dependency teardowns.
  configure [--state-path <path>]     Configure enabled managed dependencies in a terminal.
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

func runDependency(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer, deps dependencies) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage error: dependency command is required")
		fmt.Fprintln(stderr)
		writeDependencyHelp(stderr)
		return 2
	}
	if len(args) == 1 && args[0] == "help" {
		writeDependencyHelp(stdout)
		return 0
	}
	switch args[0] {
	case "list":
		return runDependencyList(ctx, args, stdout, stderr, deps)
	case "status":
		return runDependencyStatus(ctx, args, stdout, stderr, deps)
	case "pending":
		return runDependencyPending(ctx, args, stdout, stderr, deps)
	case "configure":
		return runDependencyConfigure(ctx, args, stdout, stderr, deps)
	case "disable", "uninstall":
		return runDependencyLifecycle(ctx, args, stdout, stderr, deps)
	case "install":
		return runDependencyInstall(ctx, args, stdout, stderr, deps)
	default:
		fmt.Fprintf(stderr, "usage error: unknown dependency command %q\n\n", args[0])
		writeDependencyHelp(stderr)
		return 2
	}
}
