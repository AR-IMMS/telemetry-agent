package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/ar-imms/telemetry-agent/internal/agentstate"
	desiredstateapp "github.com/ar-imms/telemetry-agent/internal/configuration/desiredstate/application"
	desiredstatedomain "github.com/ar-imms/telemetry-agent/internal/configuration/desiredstate/domain"
	"github.com/ar-imms/telemetry-agent/internal/configuration/reconciliation"
	reconciliationcontracts "github.com/ar-imms/telemetry-agent/internal/configuration/reconciliation/contracts"
	definitionapp "github.com/ar-imms/telemetry-agent/internal/monitoring/profiles/definitions/application"
	definitionsdomain "github.com/ar-imms/telemetry-agent/internal/monitoring/profiles/definitions/domain"
)

func writeIntegrationHelp(w io.Writer) {
	fmt.Fprint(w, `Usage: agentctl integration <configure|update|enable|disable|delete|list|inspect|status|history|retry|match|catalog|template> [options]

Desired mutations commit locally. The Agent Service detects them through SQLite polling and applies configuration asynchronously.
The match command reads persisted inventory and prints candidate diagnostics without changing desired state or contacting providers.
Use "agentctl integration catalog help" to list shipped Integration Definitions and inspect their supported settings and Collector capability evidence.
Use "agentctl integration template help" to manage local automatic monitoring Templates. Templates change desired state; the Agent Service owns discovery projection and Collector convergence.
`)
}

func runIntegration(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "help" {
		writeIntegrationHelp(stdout)
		return 0
	}
	if args[0] == "template" {
		return runIntegrationTemplate(ctx, args[1:], stdout, stderr)
	}
	if args[0] == "catalog" {
		return runIntegrationCatalog(ctx, args[1:], stdout, stderr)
	}
	command := args[0]
	if command == "match" {
		return runIntegrationMatch(ctx, args[1:], stdout, stderr)
	}
	flags := flag.NewFlagSet("integration "+command, flag.ContinueOnError)
	flags.SetOutput(stderr)
	statePath := flags.String("state-path", agentstate.DefaultPath(), "persistent Agent state path")
	id := flags.String("id", "", "Integration instance ID")
	definition := flags.String("definition", "", "exact definition ID")
	version := flags.String("version", "", "exact definition version")
	settings := flags.String("settings", "{}", "JSON settings object")
	enabled := flags.Bool("enabled", true, "enable created or updated instance")
	expected := flags.Uint64("expected-revision", 0, "expected instance revision")
	limit := flags.Int("limit", 20, "history limit (1-100)")
	if err := parseCompatibilityFlags(flags, args[1:]); err != nil {
		return 2
	}
	if strings.TrimSpace(*statePath) == "" {
		fmt.Fprintln(stderr, "usage error: --state-path must not be empty")
		return 2
	}
	if command != "inspect" && flags.NArg() != 0 {
		return usageIntegration(stderr, command+" does not accept positional arguments")
	}
	if err := ctx.Err(); err != nil {
		return integrationCommandError(stderr, command, err)
	}
	stateStore := newAgentStateStore(*statePath)
	store, ok := stateStore.(agentstate.SQLiteStore)
	if !ok {
		return integrationCommandError(stderr, command, fmt.Errorf("SQLite state store is required"))
	}
	repository := store.IntegrationRepository()
	diagnostics := store.ReconciliationRepository()
	switch command {
	case "configure", "update":
		if *id == "" || *definition == "" || *version == "" {
			fmt.Fprintln(stderr, "usage error: --id, --definition, and --version are required")
			return 2
		}
		if command == "configure" && *expected != 0 {
			fmt.Fprintln(stderr, "usage error: configure creates with --expected-revision 0")
			return 2
		}
		if command == "update" && !flagProvided(args[1:], "--expected-revision") {
			fmt.Fprintln(stderr, "usage error: update requires --expected-revision")
			return 2
		}
		var values map[string]any
		if err := json.Unmarshal([]byte(*settings), &values); err != nil || values == nil {
			fmt.Fprintln(stderr, "usage error: --settings must be a JSON object")
			return 2
		}
		registry, err := productionIntegrationRegistry()
		if err != nil {
			return integrationCommandError(stderr, command, err)
		}
		service := desiredstateapp.NewService(desiredStateResolverAdapter{resolver: definitionapp.NewResolver(registry)}, repository)
		instance := desiredstatedomain.Instance{ID: *id, DefinitionID: *definition, DefinitionVersion: *version, Configuration: values, Enabled: *enabled}
		var stored desiredstatedomain.Instance
		var revision uint64
		if command == "update" && !flagProvided(args[1:], "--enabled") {
			stored, revision, err = service.PutPreservingEnabled(instance, *expected)
		} else {
			stored, revision, err = service.Put(instance, *expected)
		}
		if errors.Is(err, desiredstateapp.ErrInstanceNotFound) {
			err = sql.ErrNoRows
		}
		if err != nil {
			return integrationCommandError(stderr, command, err)
		}
		fmt.Fprintf(stdout, "Desired Integration committed: id=%s instance_revision=%d desired_set_revision=%d; Agent Service convergence is pending\n", stored.ID, stored.Revision, revision)
	case "enable", "disable", "delete":
		if *id == "" || !flagProvided(args[1:], "--expected-revision") {
			fmt.Fprintln(stderr, "usage error: --id and --expected-revision are required")
			return 2
		}
		if command == "delete" {
			revision, err := repository.Delete(*id, *expected)
			if err != nil {
				return integrationCommandError(stderr, command, err)
			}
			fmt.Fprintf(stdout, "Desired Integration deleted: id=%s desired_set_revision=%d; Agent Service convergence is pending\n", *id, revision)
			return 0
		}
		registry, err := productionIntegrationRegistry()
		if err != nil {
			return integrationCommandError(stderr, command, err)
		}
		service := desiredstateapp.NewService(desiredStateResolverAdapter{resolver: definitionapp.NewResolver(registry)}, repository)
		stored, revision, err := service.SetEnabled(*id, command == "enable", *expected)
		if errors.Is(err, desiredstateapp.ErrInstanceNotFound) {
			err = sql.ErrNoRows
		}
		if err != nil {
			return integrationCommandError(stderr, command, err)
		}
		fmt.Fprintf(stdout, "Desired Integration %s: id=%s instance_revision=%d desired_set_revision=%d; Agent Service convergence is pending\n", command, stored.ID, stored.Revision, revision)
	case "list":
		snapshot, err := repository.Snapshot()
		if err != nil {
			return integrationCommandError(stderr, command, err)
		}
		fmt.Fprintf(stdout, "Desired set revision: %d\n", snapshot.Revision)
		for _, item := range snapshot.Instances {
			state := "disabled"
			if item.Enabled {
				state = "enabled"
			}
			fmt.Fprintf(stdout, "%s %s@%s revision=%d %s\n", item.ID, item.DefinitionID, item.DefinitionVersion, item.Revision, state)
		}
	case "inspect":
		if flags.NArg() != 1 {
			return usageIntegration(stderr, "inspect requires one instance ID")
		}
		snapshot, err := repository.Snapshot()
		if err != nil {
			return integrationCommandError(stderr, command, err)
		}
		for _, item := range snapshot.Instances {
			if item.ID == flags.Arg(0) {
				fmt.Fprintf(stdout, "id=%s definition=%s@%s revision=%d enabled=%t settings_keys=%v\n", item.ID, item.DefinitionID, item.DefinitionVersion, item.Revision, item.Enabled, sortedIntegrationKeys(item.Configuration))
				return 0
			}
		}
		return integrationCommandError(stderr, command, sql.ErrNoRows)
	case "status":
		desired, err := repository.Snapshot()
		if err != nil {
			return integrationCommandError(stderr, command, err)
		}
		evidence, err := diagnostics.Evidence()
		status := "Desired"
		if err == nil {
			currentBaseline, baselineErr := newRuntimeBaseline(*statePath)
			if baselineErr != nil {
				return integrationCommandError(stderr, command, baselineErr)
			}
			baseline, baselineErr := currentBaseline.Snapshot(ctx)
			if baselineErr != nil {
				return integrationCommandError(stderr, command, baselineErr)
			}
			status = integrationConvergenceStatus(evidence, desired.Revision, baseline)
		}
		fmt.Fprintf(stdout, "Desired revision: %d\nConvergence: %s\n", desired.Revision, status)
		if err == nil {
			fmt.Fprintf(stdout, "stage=%s code=%s attempts=%d/5 generation=%d candidate=%s\n", evidence.Stage, evidence.Code, evidence.Attempts, evidence.Generation, evidence.CandidateDigest)
		} else if !errors.Is(err, sql.ErrNoRows) && !errors.Is(err, os.ErrNotExist) {
			return integrationCommandError(stderr, command, err)
		}
		fmt.Fprintln(stdout, "Integration health and telemetry delivery are not inferred from configuration application.")
	case "history":
		rows, err := diagnostics.History(*limit)
		if err != nil {
			return integrationCommandError(stderr, command, err)
		}
		for _, row := range rows {
			fmt.Fprintf(stdout, "attempt=%d epoch=%d status=%s stage=%s code=%s generation=%d candidate=%s at=%s detail=%s\n", row.Number, row.Epoch, row.Status, row.Stage, row.Code, row.Generation, row.CandidateDigest, row.UpdatedAt, row.SafeDetail)
		}
	case "retry":
		currentBaseline, err := newRuntimeBaseline(*statePath)
		if err != nil {
			return integrationCommandError(stderr, command, err)
		}
		revision, err := reconciliation.NewRetryService(repository, currentBaseline, diagnostics).RetryCurrentTarget(ctx)
		if err != nil {
			return integrationCommandError(stderr, command, err)
		}
		fmt.Fprintf(stdout, "Explicit retry recorded for desired revision %d; Agent Service polling will resume convergence\n", revision)
	default:
		return usageIntegration(stderr, "unknown integration command "+command)
	}
	return 0
}

type desiredStateResolverAdapter struct{ resolver definitionapp.Resolver }

func (a desiredStateResolverAdapter) Resolve(instance desiredstatedomain.Instance) (desiredstatedomain.Instance, error) {
	if instance.Source == "" {
		instance.Source = desiredstatedomain.SourceLocal
	}
	if err := instance.Validate(); err != nil {
		return desiredstatedomain.Instance{}, err
	}
	resolved, err := a.resolver.Resolve(definitionapp.ResolveInput{
		DefinitionID: instance.DefinitionID, Version: instance.DefinitionVersion,
		Enabled: instance.Enabled, Settings: instance.Configuration,
	})
	if err != nil {
		if errors.Is(err, definitionsdomain.ErrUnknownDefinition) || errors.Is(err, definitionsdomain.ErrInvalidContribution) {
			return desiredstatedomain.Instance{}, err
		}
		return desiredstatedomain.Instance{}, fmt.Errorf("%w: %v", desiredstatedomain.ErrInvalidInstance, err)
	}
	instance.Configuration = resolved.Settings
	if instance.Binding != nil {
		binding := *instance.Binding
		instance.Binding = &binding
	}
	return instance, nil
}

func usageIntegration(w io.Writer, message string) int {
	fmt.Fprintln(w, "usage error: "+message)
	writeIntegrationHelp(w)
	return 2
}
func integrationCommandError(w io.Writer, command string, err error) int {
	fmt.Fprintf(w, "integration %s: %v\n", command, err)
	return 1
}
func sortedIntegrationKeys(values map[string]any) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func flagProvided(args []string, name string) bool {
	for _, arg := range args {
		if arg == name || strings.HasPrefix(arg, name+"=") {
			return true
		}
	}
	return false
}

func integrationConvergenceStatus(evidence reconciliationcontracts.TargetEvidence, desiredRevision uint64, baseline reconciliation.BaselineSnapshot) string {
	if !reconciliation.TargetIsCurrent(evidence, desiredRevision, baseline) {
		return string(reconciliation.StatusPending)
	}
	return string(evidence.Status)
}
