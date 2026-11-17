package inventory

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"sort"
	"strings"
	"time"

	clisettings "github.com/ar-imms/telemetry-agent/internal/cli/settings"
	agenthealth "github.com/ar-imms/telemetry-agent/internal/health"
	domain "github.com/ar-imms/telemetry-agent/internal/inventory/observation/domain"
	profilesapplication "github.com/ar-imms/telemetry-agent/internal/monitoring/profiles/application"
	profilesdomain "github.com/ar-imms/telemetry-agent/internal/monitoring/profiles/domain"
)

// Dependencies provide the Inventory read and cross-context evidence required
// by the command. Implementations are supplied at executable composition.
type Dependencies struct {
	DefaultStatePath             func() string
	LoadSnapshot                 func(string) (domain.InventorySnapshot, error)
	LoadProfiles                 func(string) (profilesdomain.Registry, error)
	FetchCollectorScrapeEvidence func(context.Context) []profilesapplication.CollectorScrapeEvidence
	ProfileCredentialsConfigured func(string, string, string) bool
	DiagnoseEndpoint             func(profilesdomain.Profile, string, []domain.Endpoint, []profilesapplication.CollectorScrapeEvidence, map[string]string) profilesapplication.EndpointDiagnostic
	Platform                     string
}

// Execute parses and runs one existing Inventory command using injected
// application readers while preserving its established text and exit contract.
func Execute(ctx context.Context, args []string, stdout, stderr io.Writer, deps Dependencies) int {
	if len(args) == 0 || args[0] == "help" {
		writeHelp(stdout)
		return 0
	}
	command := args[0]
	flags := flag.NewFlagSet("inventory "+command, flag.ContinueOnError)
	flags.SetOutput(stderr)
	defaultPath := ""
	if deps.DefaultStatePath != nil {
		defaultPath = deps.DefaultStatePath()
	}
	statePath := flags.String("state-path", defaultPath, "persistent Agent state path")
	activeOnly := flags.Bool("active-only", false, "list only resources currently present in a complete provider snapshot")
	if err := clisettings.ParseCompatibilityFlags(flags, args[1:]); err != nil {
		return 2
	}
	if strings.TrimSpace(*statePath) == "" {
		fmt.Fprintln(stderr, "usage error: --state-path must not be empty")
		return 2
	}
	switch command {
	case "list", "status":
		if flags.NArg() != 0 {
			fmt.Fprintf(stderr, "usage error: inventory %s does not accept positional arguments\n", command)
			return 2
		}
	case "inspect":
		if flags.NArg() != 1 {
			fmt.Fprintln(stderr, "usage error: inventory inspect requires provider/instance/resource-id")
			return 2
		}
	default:
		fmt.Fprintf(stderr, "usage error: unknown inventory command %q\n", command)
		writeHelp(stderr)
		return 2
	}
	if *activeOnly && command != "list" {
		fmt.Fprintln(stderr, "usage error: --active-only is supported only by inventory list")
		return 2
	}
	if err := ctx.Err(); err != nil {
		fmt.Fprintf(stderr, "inventory %s: %v\n", command, err)
		return 1
	}
	if deps.LoadSnapshot == nil {
		fmt.Fprintf(stderr, "inventory %s: inventory snapshot reader is unavailable\n", command)
		return 1
	}
	snapshot, err := deps.LoadSnapshot(*statePath)
	if err != nil {
		fmt.Fprintf(stderr, "inventory %s: %v\n", command, err)
		return 1
	}
	profileRegistry := profilesdomain.DefaultRegistry()
	if deps.LoadProfiles != nil {
		profileRegistry, err = deps.LoadProfiles(*statePath)
		if err != nil {
			fmt.Fprintf(stderr, "load local monitoring profiles: %v\n", err)
			return 1
		}
	}
	switch command {
	case "list":
		fmt.Fprintln(stdout, "PROVIDER\tRESOURCE\tKIND\tNAME\tSTATE\tPRESENCE\tFRESHNESS")
		for _, resource := range snapshot.Resources {
			if *activeOnly && resource.Presence != domain.PresencePresent {
				continue
			}
			fmt.Fprintf(stdout, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", resource.Key.Provider, resource.Key.String(), resource.Kind, resource.DisplayName, resource.LifecycleState, resource.Presence, resource.FreshnessAt(time.Now().UTC(), domain.DefaultPolicy().StaleAfter))
		}
		return 0
	case "inspect":
		key := flags.Arg(0)
		for _, resource := range snapshot.Resources {
			if resource.Key.String() != key {
				continue
			}
			writeResource(stdout, resource, profileRegistry, ctx, deps)
			return 0
		}
		fmt.Fprintf(stderr, "inventory inspect: resource %q not found\n", key)
		return 1
	case "status":
		fmt.Fprintf(stdout, "Inventory revision: %d\nResources: %d\n", snapshot.Revision, len(snapshot.Resources))
		if len(snapshot.Providers) == 0 {
			fmt.Fprintln(stdout, "Docker discovery: disabled (no explicit endpoint configured)")
			return 0
		}
		for _, provider := range snapshot.Providers {
			state := "disabled"
			if provider.Enabled {
				state = "enabled"
			}
			fmt.Fprintf(stdout, "Provider: %s/%s\n  state: %s\n  resources: %d\n  last attempt: %s\n  last success: %s\n  last error: %s\n",
				provider.ProviderID, provider.ProviderInstanceID, state, provider.ResourceCount, formatTime(provider.LastAttemptAt), formatTime(provider.LastSuccessAt), emptyAsNone(provider.LastErrorCode))
		}
		return 0
	}
	return 2
}

func writeHelp(output io.Writer) {
	fmt.Fprint(output, `Usage: agentctl inventory <list|inspect|status> [options]

Commands:
  list [--active-only]           List persisted resources; optionally hide missing history.
  inspect <provider/instance/id> Show one persisted resource and its evidence.
  status                         Show provider and inventory status.
`)
}

func writeResource(output io.Writer, resource domain.Resource, profileRegistry profilesdomain.Registry, ctx context.Context, deps Dependencies) {
	fmt.Fprintf(output, "Resource: %s\nKind: %s\nName: %s\nLifecycle: %s\nPresence: %s\nFreshness: %s\nFirst seen: %s\nLast seen: %s\n",
		resource.Key.String(), resource.Kind, resource.DisplayName, resource.LifecycleState, resource.Presence, resource.FreshnessAt(time.Now().UTC(), domain.DefaultPolicy().StaleAfter), resource.FirstSeenAt.Format(time.RFC3339Nano), resource.LastSeenAt.Format(time.RFC3339Nano))
	fmt.Fprintln(output, "Attributes:")
	attributeNames := make([]string, 0, len(resource.Attributes))
	for name := range resource.Attributes {
		attributeNames = append(attributeNames, name)
	}
	sort.Strings(attributeNames)
	for _, name := range attributeNames {
		fmt.Fprintf(output, "  %s=%s\n", name, resource.Attributes[name])
	}
	fmt.Fprintln(output, "Endpoint evidence:")
	for _, endpoint := range resource.Endpoints {
		fmt.Fprintf(output, "  scope=%s address=%s port=%d protocol=%s provenance=%s\n", endpoint.Scope, endpoint.Address, endpoint.Port, endpoint.Protocol, endpoint.Provenance)
	}
	if profile, matched := profileRegistry.Match(string(resource.Key.Provider), resource.Attributes); matched {
		if deps.ProfileCredentialsConfigured != nil && !deps.ProfileCredentialsConfigured(profile.ReceiverType, profile.Settings.UsernameEnv, profile.Settings.PasswordEnv) {
			fmt.Fprintln(output, "Monitoring configuration: requires_configuration (PostgreSQL credential values are unavailable to the Agent service)")
		}
		var evidence []profilesapplication.CollectorScrapeEvidence
		if deps.FetchCollectorScrapeEvidence != nil {
			evidence = deps.FetchCollectorScrapeEvidence(ctx)
		}
		platform := deps.Platform
		if platform == "" {
			platform = runtime.GOOS
		}
		var diagnostic profilesapplication.EndpointDiagnostic
		if deps.DiagnoseEndpoint != nil {
			diagnostic = deps.DiagnoseEndpoint(profile, platform, resource.Endpoints, evidence, resource.Attributes)
		} else {
			diagnostic = profilesapplication.DiagnoseEndpointWithAttributes(profile, platform, nil, evidence, resource.Attributes)
		}
		fmt.Fprintf(output, "Monitoring profile: %s\nProfile endpoint: %s (candidates=%d, evidence=%s)\n", profile.ID, diagnostic.State, diagnostic.CandidateCount, diagnostic.EvidenceSource)
		if diagnostic.State == profilesapplication.EndpointResolved {
			fmt.Fprintln(output, "Endpoint reachability: unverified; only a matching OTel receiver error can establish unreachable")
		} else if diagnostic.State == profilesapplication.EndpointUnreachable {
			fmt.Fprintf(output, "Endpoint reachability: unreachable (last OTel evidence code=%s at %s)\n", diagnostic.EvidenceCode, diagnostic.EvidenceTime.UTC().Format(time.RFC3339Nano))
		}
	}
	if resource.MissingSince != nil {
		fmt.Fprintf(output, "Missing since: %s (%d complete snapshots)\n", resource.MissingSince.Format(time.RFC3339Nano), resource.MissingSnapshots)
	}
}

// CollectorScrapeEvidenceFromSnapshot projects Health's sanitized Collector
// failure records into the Monitoring endpoint-diagnostic contract.
func CollectorScrapeEvidenceFromSnapshot(snapshot agenthealth.Snapshot) []profilesapplication.CollectorScrapeEvidence {
	evidence := make([]profilesapplication.CollectorScrapeEvidence, 0, len(snapshot.CollectorFailures))
	for _, failure := range snapshot.CollectorFailures {
		observedAt, _ := time.Parse(time.RFC3339Nano, failure.ObservedAt)
		evidence = append(evidence, profilesapplication.CollectorScrapeEvidence{ReceiverID: failure.Receiver, Endpoint: failure.Endpoint, FailureCode: failure.Code, ObservedAt: observedAt})
	}
	return evidence
}

func formatTime(value time.Time) string {
	if value.IsZero() {
		return "never"
	}
	return value.UTC().Format(time.RFC3339Nano)
}

func emptyAsNone(value string) string {
	if value == "" {
		return "none"
	}
	return value
}

// FetchCollectorScrapeEvidence retrieves supported local health evidence with
// bounded timeouts. A failed health request yields no inference of reachability.
func FetchCollectorScrapeEvidence(ctx context.Context, client *http.Client, endpoint string) []profilesapplication.CollectorScrapeEvidence {
	statusContext, cancel := context.WithTimeout(ctx, 400*time.Millisecond)
	defer cancel()
	snapshot, err := agenthealth.FetchStatus(statusContext, client, endpoint)
	if err != nil {
		return nil
	}
	return CollectorScrapeEvidenceFromSnapshot(snapshot)
}
