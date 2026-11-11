package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/ar-imms/telemetry-agent/internal/agentstate"
	"github.com/ar-imms/telemetry-agent/internal/configuration"
	"github.com/ar-imms/telemetry-agent/internal/discovery"
	"github.com/ar-imms/telemetry-agent/internal/integration"
)

func runDiscovery(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "help" {
		writeDiscoveryHelp(stdout)
		return 0
	}
	if args[0] != "proposals" {
		return usageDiscovery(stderr, "expected proposals subcommand")
	}
	return runDiscoveryProposals(ctx, args[1:], stdout, stderr)
}

func writeDiscoveryHelp(w io.Writer) {
	fmt.Fprint(w, `Usage: agentctl discovery proposals <list|inspect|dismiss|approve> [options]

Proposal inspection is read-only. Approval updates desired state atomically; the Agent Service performs reconciliation.
`)
}

func runDiscoveryProposals(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "help" {
		writeDiscoveryHelp(stdout)
		return 0
	}
	command := args[0]
	flags := flag.NewFlagSet("discovery proposals "+command, flag.ContinueOnError)
	flags.SetOutput(stderr)
	statePath := flags.String("state-path", agentstate.DefaultPath(), "persistent Agent state path")
	id := flags.String("id", "", "stable proposal ID")
	revision := flags.Uint64("revision", 0, "expected proposal revision")
	desiredRevision := flags.Uint64("expected-desired-revision", 0, "expected desired Integration set revision")
	inputs := flags.String("input", "", "non-secret proposal inputs as comma-separated name=value pairs")
	if err := flags.Parse(args[1:]); err != nil {
		return 2
	}
	if flags.NArg() != 0 || strings.TrimSpace(*statePath) == "" {
		return usageDiscovery(stderr, "unexpected argument or empty state path")
	}
	if err := ctx.Err(); err != nil {
		return integrationCommandError(stderr, "discovery proposals "+command, err)
	}
	store := agentstate.NewSQLiteStore(*statePath)
	proposals := store.ProposalRepository()
	switch command {
	case "list":
		items, err := proposals.ProposalSnapshot()
		if err != nil {
			return integrationCommandError(stderr, command, err)
		}
		if err := json.NewEncoder(stdout).Encode(items); err != nil {
			return integrationCommandError(stderr, command, err)
		}
	case "inspect":
		if *id == "" {
			return usageDiscovery(stderr, "inspect requires --id")
		}
		items, err := proposals.ProposalSnapshot()
		if err != nil {
			return integrationCommandError(stderr, command, err)
		}
		for _, item := range items {
			if item.ID == *id {
				if err := json.NewEncoder(stdout).Encode(item); err != nil {
					return integrationCommandError(stderr, command, err)
				}
				return 0
			}
		}
		return integrationCommandError(stderr, command, discovery.ErrUnknownProposal)
	case "dismiss":
		if *id == "" || *revision == 0 {
			return usageDiscovery(stderr, "dismiss requires --id and --revision")
		}
		manager := discovery.NewProposalManager(proposals)
		if err := manager.Dismiss(*id, *revision, time.Now().UTC()); err != nil {
			return integrationCommandError(stderr, command, err)
		}
		fmt.Fprintln(stdout, "proposal dismissed")
	case "approve":
		if *id == "" || *revision == 0 {
			return usageDiscovery(stderr, "approve requires --id and --revision")
		}
		if !flagProvided(args[1:], "--expected-desired-revision") {
			return usageDiscovery(stderr, "approve requires --expected-desired-revision")
		}
		values, err := parseProposalInputs(*inputs)
		if err != nil {
			return usageDiscovery(stderr, err.Error())
		}
		service, err := newRuntimeApprovalService(*statePath)
		if err != nil {
			return integrationCommandError(stderr, command, err)
		}
		instance, nextRevision, err := service.Approve(ctx, configuration.ApprovalRequest{
			ProposalID: *id, ProposalRevision: *revision, ExpectedDesiredRevision: *desiredRevision, Inputs: values,
		})
		if err != nil {
			return integrationCommandError(stderr, command, err)
		}
		fmt.Fprintf(stdout, "proposal approved: integration_id=%s instance_revision=%d desired_set_revision=%d; Agent Service convergence is pending\n", instance.ID, instance.Revision, nextRevision)
	default:
		return usageDiscovery(stderr, "unknown proposal operation")
	}
	return 0
}

func usageDiscovery(stderr io.Writer, message string) int {
	fmt.Fprintln(stderr, "usage error: "+message)
	writeDiscoveryHelp(stderr)
	return 2
}

func parseProposalInputs(raw string) (map[string]any, error) {
	values := make(map[string]any)
	if strings.TrimSpace(raw) == "" {
		return values, nil
	}
	for _, pair := range strings.Split(raw, ",") {
		name, value, ok := strings.Cut(pair, "=")
		name = strings.TrimSpace(name)
		if !ok || name == "" || strings.TrimSpace(value) == "" {
			return nil, fmt.Errorf("--input must contain non-empty name=value pairs")
		}
		if _, exists := values[name]; exists {
			return nil, fmt.Errorf("--input contains a duplicate setting name")
		}
		values[name] = strings.TrimSpace(value)
	}
	if err := integration.ValidateInstance(integration.IntegrationInstance{
		ID: "proposal-input", DefinitionID: "redis", DefinitionVersion: "1.0.0", Configuration: values,
	}); err != nil {
		return nil, fmt.Errorf("--input must contain only non-secret settings")
	}
	return values, nil
}
