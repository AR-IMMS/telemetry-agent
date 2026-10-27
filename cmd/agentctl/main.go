package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
)

func main() {
	// Convert termination signals into cancellation for graceful shutdown.
	deps := defaultDependencies()

	ctx, stop := signal.NotifyContext(
		context.Background(),
		terminationSignals()...,
	)

	exitCode := run(ctx, os.Args[1:], os.Stdout, os.Stderr, deps)

	stop()
	os.Exit(exitCode)
}

func writeRootHelp(output io.Writer) {
	fmt.Fprint(output, `Usage:
  agentctl <command> [options]

Commands:
  bootstrap   Bootstrap the OpenTelemetry Collector.
  dependency  Manage telemetry dependencies.
  run         Run the OpenTelemetry Collector under supervision.
  status      Show live Agent health.
  install     Install and start the Agent as a managed service.
  uninstall   Safely remove the Agent and its managed dependencies.
  help        Show this help.
`)
}

func run(
	ctx context.Context,
	args []string,
	stdout io.Writer,
	stderr io.Writer,
	deps dependencies,
) int {
	// Keep command dispatch separate from command-specific flag validation and effects.
	// usage: agentctl <bootstrap|run|dependency> [command options]
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage error: command is required")
		fmt.Fprintln(stderr)
		writeRootHelp(stderr)

		return 2
	}

	switch args[0] {
	case "bootstrap":
		return runBootstrap(ctx, args[1:], stdout, stderr, deps)

	case "run":
		return runCollector(ctx, args[1:], stdout, stderr, deps)

	case "dependency":
		return runDependency(ctx, args[1:], stdout, stderr, deps)

	case "status":
		return runStatus(ctx, args[1:], stdout, stderr, deps)

	case "service":
		return runService(ctx, args[1:], stdout, stderr, deps)

	case "install":
		return runInstall(ctx, args[1:], stdout, stderr, deps)

	case "uninstall":
		return runUninstall(ctx, args[1:], stdout, stderr, deps)

	case "help":
		writeRootHelp(stdout)
		return 0

	default:
		fmt.Fprintf(
			stderr,
			"usage error: unknown command %q\n\n",
			args[0],
		)
		writeRootHelp(stderr)

		return 2
	}
}

func usageError(stderr io.Writer, flags *flag.FlagSet, message string) int {
	fmt.Fprintln(stderr, "usage error: "+message)
	flags.Usage()
	return 2
}
