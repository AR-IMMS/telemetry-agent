package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"strings"
	"syscall"
	"time"

	"github.com/ar-imms/telemetry-agent/internal/agenthealth"
)

func runStatus(
	ctx context.Context,
	args []string,
	stdout io.Writer,
	stderr io.Writer,
	deps dependencies,
) int {
	return runHealthReport(ctx, args, stdout, stderr, deps, false)
}

func runDoctor(
	ctx context.Context,
	args []string,
	stdout io.Writer,
	stderr io.Writer,
	deps dependencies,
) int {
	return runHealthReport(ctx, args, stdout, stderr, deps, true)
}

func runHealthReport(
	ctx context.Context,
	args []string,
	stdout io.Writer,
	stderr io.Writer,
	deps dependencies,
	detailed bool,
) int {
	command := "status"
	if detailed {
		command = "doctor"
	}
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.Usage = func() {
		fmt.Fprintln(
			stderr,
			"usage: agentctl "+command+" [--endpoint <url>]",
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
			"usage error: "+command+" does not accept positional arguments",
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
		fmt.Fprintf(stderr, "agentctl %s: status endpoint is unavailable (%s)\n", command, statusErrorCategory(err))

		return 1
	}

	if detailed {
		writeDoctorSnapshot(stdout, snapshot)
	} else {
		writeStatusSummary(stdout, snapshot)
	}

	return 0
}

func statusErrorCategory(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	var networkError net.Error
	if errors.As(err, &networkError) && networkError.Timeout() {
		return "timeout"
	}
	var dnsError *net.DNSError
	if errors.As(err, &dnsError) {
		return "dns_resolution_failed"
	}
	if errors.Is(err, syscall.ECONNREFUSED) {
		return "connection_refused"
	}
	return "request_failed"
}

func writeStatusSummary(output io.Writer, snapshot agenthealth.Snapshot) {
	fmt.Fprintf(output, "Agent health: %s\n", snapshot.Status)
	fmt.Fprintf(output, "Collector state: %s\n", snapshot.CollectorState)
	fmt.Fprintf(output, "Configuration generations: desired=%d activated=%d applied=%d\n", snapshot.DesiredGeneration, snapshot.ActivatedGeneration, snapshot.AppliedGeneration)
	fmt.Fprintf(output, "Pending dependency teardowns: %d\n", snapshot.PendingTeardowns)
	fmt.Fprintln(output, "Status scope: Collector/configuration runtime readiness (legacy projection)")
	writeSummaryComponent(output, snapshot.Assessment, agenthealth.ComponentAgentRuntime)
	assessmentState := snapshot.Assessment.State
	if assessmentState == "" {
		assessmentState = agenthealth.EvidenceUnknown
	}
	fmt.Fprintf(output, "Evidence assessment: %s\n", assessmentState)
	writeSummaryComponent(output, snapshot.Assessment, agenthealth.ComponentCollector)
	writeSummaryComponent(output, snapshot.Assessment, agenthealth.ComponentConfiguration)
	writeSummaryComponent(output, snapshot.Assessment, agenthealth.ComponentTelemetryDelivery)
	if snapshot.LastError != "" {
		fmt.Fprintf(output, "Last error: %s\n", snapshot.LastError)
	}
	for _, failure := range snapshot.CollectorFailures {
		fmt.Fprintf(output, "Collector endpoint failure: receiver=%s endpoint=%s code=%s\n", failure.Receiver, failure.Endpoint, failure.Code)
	}
}

func writeSummaryComponent(output io.Writer, assessment agenthealth.Assessment, component agenthealth.Component) {
	for _, evidence := range assessment.Components {
		if evidence.Component != component {
			continue
		}
		fmt.Fprintf(output, "%s: %s (freshness=%s)\n", component, evidence.State, evidence.Freshness)
		return
	}
	fmt.Fprintf(output, "%s: %s (freshness=%s)\n", component, agenthealth.EvidenceUnknown, agenthealth.FreshnessUnknown)
}

func writeDoctorSnapshot(output io.Writer, snapshot agenthealth.Snapshot) {
	assessmentState := snapshot.Assessment.State
	if assessmentState == "" {
		assessmentState = agenthealth.EvidenceUnknown
	}
	fmt.Fprintf(output, "Agent health: %s\n", snapshot.Status)
	fmt.Fprintln(output, "Status scope: Collector/configuration runtime readiness (legacy projection)")
	writeSummaryComponent(output, snapshot.Assessment, agenthealth.ComponentAgentRuntime)
	fmt.Fprintf(output, "Evidence assessment: %s\n", assessmentState)
	if snapshot.Assessment.GeneratedAt.IsZero() {
		fmt.Fprintln(output, "Assessment generated: unknown")
	} else {
		fmt.Fprintf(output, "Assessment generated: %s\n", snapshot.Assessment.GeneratedAt.UTC().Format(time.RFC3339Nano))
	}
	fmt.Fprintf(output, "Collector runtime state: %s\n", snapshot.CollectorState)
	fmt.Fprintf(output, "Configuration generations: desired=%d activated=%d applied=%d\n", snapshot.DesiredGeneration, snapshot.ActivatedGeneration, snapshot.AppliedGeneration)
	fmt.Fprintf(output, "Pending dependency teardowns: %d\n", snapshot.PendingTeardowns)
	fmt.Fprintln(output, "Component evidence:")
	for _, evidence := range snapshot.Assessment.Components {
		fmt.Fprintf(output, "  %s: state=%s freshness=%s completeness=%s", evidence.Component, evidence.State, evidence.Freshness, evidence.Completeness)
		if evidence.ObservedAt != nil {
			fmt.Fprintf(output, " observed_at=%s", evidence.ObservedAt.UTC().Format(time.RFC3339Nano))
		}
		if evidence.ValidUntil != nil {
			fmt.Fprintf(output, " valid_until=%s", evidence.ValidUntil.UTC().Format(time.RFC3339Nano))
		}
		if evidence.ReasonCode != "" {
			fmt.Fprintf(output, " reason=%s", evidence.ReasonCode)
		}
		fmt.Fprintln(output)
	}
	if snapshot.LastError != "" {
		fmt.Fprintf(output, "Last error: %s\n", snapshot.LastError)
	}
	for _, failure := range snapshot.CollectorFailures {
		fmt.Fprintf(output, "Collector endpoint failure: receiver=%s endpoint=%s code=%s observed_at=%s\n", failure.Receiver, failure.Endpoint, failure.Code, failure.ObservedAt)
	}
}
