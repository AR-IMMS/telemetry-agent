package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"
)

func runStatus(
	ctx context.Context,
	args []string,
	stdout io.Writer,
	stderr io.Writer,
	deps dependencies,
) int {
	flags := flag.NewFlagSet("status", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.Usage = func() {
		fmt.Fprintln(
			stderr,
			"usage: agentctl status [--endpoint <url>]",
		)
	}

	endpoint := flags.String(
		"endpoint",
		defaultStatusEndpoint,
		"local Agent status endpoint",
	)

	if err := flags.Parse(args); err != nil {
		flags.Usage()

		return 2
	}
	if flags.NArg() != 0 {
		fmt.Fprintln(
			stderr,
			"usage error: status does not accept positional arguments",
		)
		flags.Usage()

		return 2
	}
	if strings.TrimSpace(*endpoint) == "" {
		return usageError(
			stderr,
			flags,
			"--endpoint must not be empty",
		)
	}

	fetchStatus := deps.fetchHealthStatus
	if fetchStatus == nil {
		fetchStatus = defaultFetchHealthStatus
	}

	snapshot, err := fetchStatus(ctx, *endpoint)
	if err != nil {
		fmt.Fprintf(
			stderr,
			"Agent is not running or status endpoint is unreachable: %v\n",
			err,
		)

		return 1
	}

	fmt.Fprintf(stdout, "Agent health: %s\n", snapshot.Status)
	fmt.Fprintf(
		stdout,
		"Collector state: %s\n",
		snapshot.CollectorState,
	)
	fmt.Fprintf(
		stdout,
		"Configuration generations: desired=%d activated=%d applied=%d\n",
		snapshot.DesiredGeneration,
		snapshot.ActivatedGeneration,
		snapshot.AppliedGeneration,
	)
	fmt.Fprintf(
		stdout,
		"Pending dependency teardowns: %d\n",
		snapshot.PendingTeardowns,
	)

	if snapshot.LastError != "" {
		fmt.Fprintf(stdout, "Last error: %s\n", snapshot.LastError)
	}

	return 0
}
