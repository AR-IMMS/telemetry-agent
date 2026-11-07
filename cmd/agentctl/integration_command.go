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
	"time"

	"github.com/ar-imms/telemetry-agent/internal/agentlifecycle"
	"github.com/ar-imms/telemetry-agent/internal/agentstate"
	"github.com/ar-imms/telemetry-agent/internal/collectorruntime"
	"github.com/ar-imms/telemetry-agent/internal/configuration"
	"github.com/ar-imms/telemetry-agent/internal/identity"
	"github.com/ar-imms/telemetry-agent/internal/integration"
	"github.com/ar-imms/telemetry-agent/internal/reconciliation"
)

func writeIntegrationHelp(w io.Writer) {
	fmt.Fprint(w, `Usage: agentctl integration <configure|update|enable|disable|delete|list|inspect|status|history|retry> [options]

Desired mutations commit locally. The Agent Service detects them through SQLite polling and applies configuration asynchronously.
`)
}

func runIntegration(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "help" {
		writeIntegrationHelp(stdout)
		return 0
	}
	command := args[0]
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
	if err := flags.Parse(args[1:]); err != nil {
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
	stateStore := agentstate.NewStore(*statePath)
	store, ok := stateStore.(agentstate.SQLiteStore)
	if !ok {
		return integrationCommandError(stderr, command, fmt.Errorf("SQLite state store is required"))
	}
	repository := store.IntegrationRepository()
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
		if command == "update" && !flagProvided(args[1:], "--enabled") {
			snapshot, err := repository.Snapshot()
			if err != nil {
				return integrationCommandError(stderr, command, err)
			}
			found := false
			for _, instance := range snapshot.Instances {
				if instance.ID == *id {
					*enabled = instance.Enabled
					found = true
					break
				}
			}
			if !found {
				return integrationCommandError(stderr, command, sql.ErrNoRows)
			}
		}
		var values map[string]any
		if err := json.Unmarshal([]byte(*settings), &values); err != nil || values == nil {
			fmt.Fprintln(stderr, "usage error: --settings must be a JSON object")
			return 2
		}
		registry, err := productionIntegrationRegistryForHost()
		if err != nil {
			return integrationCommandError(stderr, command, err)
		}
		service := configuration.NewIntegrationService(integration.NewResolver(registry), repository)
		stored, revision, err := service.Put(integration.IntegrationInstance{ID: *id, DefinitionID: *definition, DefinitionVersion: *version, Configuration: values, Enabled: *enabled}, *expected)
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
		snapshot, err := repository.Snapshot()
		if err != nil {
			return integrationCommandError(stderr, command, err)
		}
		var instance *integration.IntegrationInstance
		for i := range snapshot.Instances {
			if snapshot.Instances[i].ID == *id {
				instance = &snapshot.Instances[i]
				break
			}
		}
		if instance == nil {
			return integrationCommandError(stderr, command, sql.ErrNoRows)
		}
		instance.Enabled = command == "enable"
		registry, err := productionIntegrationRegistryForHost()
		if err != nil {
			return integrationCommandError(stderr, command, err)
		}
		service := configuration.NewIntegrationService(integration.NewResolver(registry), repository)
		stored, revision, err := service.Put(*instance, *expected)
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
		evidence, err := store.ReconciliationEvidence()
		status := "Desired"
		if err == nil {
			currentBaseline, baselineErr := newRuntimeBaseline(*statePath)
			if baselineErr != nil {
				return integrationCommandError(stderr, command, baselineErr)
			}
			baseline, baselineErr := currentBaseline.Snapshot()
			if baselineErr != nil {
				return integrationCommandError(stderr, command, baselineErr)
			}
			status = integrationConvergenceStatus(evidence, desired.Revision, baseline)
		}
		fmt.Fprintf(stdout, "Desired revision: %d\nConvergence: %s\n", desired.Revision, status)
		if err == nil {
			fmt.Fprintf(stdout, "stage=%s code=%s attempts=%d/5 generation=%d candidate=%s\n", evidence.Stage, evidence.Code, evidence.Attempts, evidence.LifecycleGeneration, evidence.CandidateDigest)
		} else if !errors.Is(err, sql.ErrNoRows) && !errors.Is(err, os.ErrNotExist) {
			return integrationCommandError(stderr, command, err)
		}
		fmt.Fprintln(stdout, "Integration health and telemetry delivery are not inferred from configuration application.")
	case "history":
		rows, err := store.ReconciliationHistory(*limit)
		if err != nil {
			return integrationCommandError(stderr, command, err)
		}
		for _, row := range rows {
			fmt.Fprintf(stdout, "attempt=%d epoch=%d status=%s stage=%s code=%s generation=%d candidate=%s at=%s detail=%s\n", row.Number, row.Epoch, row.Status, row.Stage, row.Code, row.LifecycleGeneration, row.CandidateDigest, row.UpdatedAt, row.SafeDetail)
		}
	case "retry":
		evidence, err := store.ReconciliationEvidence()
		if err != nil {
			return integrationCommandError(stderr, command, err)
		}
		desired, err := repository.Snapshot()
		if err != nil {
			return integrationCommandError(stderr, command, err)
		}
		currentBaseline, err := newRuntimeBaseline(*statePath)
		if err != nil {
			return integrationCommandError(stderr, command, err)
		}
		baseline, err := currentBaseline.Snapshot()
		if err != nil {
			return integrationCommandError(stderr, command, err)
		}
		if !integrationTargetMatches(evidence, desired.Revision, baseline) {
			return integrationCommandError(stderr, command, fmt.Errorf("latest desired or baseline target changed; the new target has its own retry budget"))
		}
		if err := store.RetryReconciliationTarget(evidence.Target.Key, time.Now().UTC()); err != nil {
			return integrationCommandError(stderr, command, err)
		}
		fmt.Fprintf(stdout, "Explicit retry recorded for desired revision %d; Agent Service polling will resume convergence\n", evidence.Target.DesiredRevision)
	default:
		return usageIntegration(stderr, "unknown integration command "+command)
	}
	return 0
}

func productionIntegrationRegistryForHost() (integration.Registry, error) {
	platform, err := identity.CollectPlatformInfo()
	if err != nil {
		return integration.Registry{}, err
	}
	catalog, err := collectorruntime.StaticCatalog(platform)
	if err != nil {
		return integration.Registry{}, err
	}
	return productionIntegrationRegistry(catalog)
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

func integrationConvergenceStatus(evidence agentstate.ReconciliationEvidence, desiredRevision uint64, baseline agentlifecycle.CleanBaselineSnapshot) string {
	if !integrationTargetMatches(evidence, desiredRevision, baseline) {
		return string(reconciliation.StatusPending)
	}
	return evidence.Status
}

func integrationTargetMatches(evidence agentstate.ReconciliationEvidence, desiredRevision uint64, baseline agentlifecycle.CleanBaselineSnapshot) bool {
	return evidence.Target.DesiredRevision == desiredRevision &&
		evidence.Target.DesiredGeneration == baseline.DesiredGeneration &&
		evidence.Target.BaselineFingerprint == baseline.Fingerprint
}
