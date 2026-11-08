package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/ar-imms/telemetry-agent/internal/agentstate"
	"github.com/ar-imms/telemetry-agent/internal/discovery"
)

// writeInventoryHelp prints the read-only inventory command surface.
func writeInventoryHelp(output io.Writer) {
	fmt.Fprint(output, `Usage: agentctl inventory <list|inspect|status> [options]

Commands:
  list                           List persisted discovered resources.
  inspect <provider/instance/id> Show one persisted resource and its evidence.
  status                         Show provider and inventory status.
`)
}

// runInventory reads only committed SQLite inventory; it never calls a provider.
func runInventory(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "help" {
		writeInventoryHelp(stdout)
		return 0
	}
	command := args[0]
	flags := flag.NewFlagSet("inventory "+command, flag.ContinueOnError)
	flags.SetOutput(stderr)
	statePath := flags.String("state-path", agentstate.DefaultPath(), "persistent Agent state path")
	if err := flags.Parse(args[1:]); err != nil {
		return 2
	}
	if strings.TrimSpace(*statePath) == "" {
		fmt.Fprintln(stderr, "usage error: --state-path must not be empty")
		return 2
	}
	if err := ctx.Err(); err != nil {
		fmt.Fprintf(stderr, "inventory %s: %v\n", command, err)
		return 1
	}
	snapshot, err := agentstate.NewSQLiteStore(*statePath).InventoryRepository().Snapshot()
	if err != nil {
		fmt.Fprintf(stderr, "inventory %s: %v\n", command, err)
		return 1
	}
	switch command {
	case "list":
		if flags.NArg() != 0 {
			fmt.Fprintln(stderr, "usage error: inventory list does not accept positional arguments")
			return 2
		}
		fmt.Fprintln(stdout, "PROVIDER\tRESOURCE\tKIND\tNAME\tSTATE\tPRESENCE\tFRESHNESS")
		for _, resource := range snapshot.Resources {
			fmt.Fprintf(stdout, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", resource.Key.Provider, resource.Key.String(), resource.Kind, resource.DisplayName, resource.LifecycleState, resource.Presence, resource.FreshnessAt(time.Now().UTC(), discovery.DefaultPolicy().StaleAfter))
		}
		return 0
	case "inspect":
		if flags.NArg() != 1 {
			fmt.Fprintln(stderr, "usage error: inventory inspect requires provider/instance/resource-id")
			return 2
		}
		key := flags.Arg(0)
		for _, resource := range snapshot.Resources {
			if resource.Key.String() == key {
				fmt.Fprintf(stdout, "Resource: %s\nKind: %s\nName: %s\nLifecycle: %s\nPresence: %s\nFreshness: %s\nFirst seen: %s\nLast seen: %s\n",
					resource.Key.String(), resource.Kind, resource.DisplayName, resource.LifecycleState, resource.Presence, resource.FreshnessAt(time.Now().UTC(), discovery.DefaultPolicy().StaleAfter), resource.FirstSeenAt.Format(time.RFC3339Nano), resource.LastSeenAt.Format(time.RFC3339Nano))
				fmt.Fprintln(stdout, "Attributes:")
				attributeNames := make([]string, 0, len(resource.Attributes))
				for name := range resource.Attributes {
					attributeNames = append(attributeNames, name)
				}
				sort.Strings(attributeNames)
				for _, name := range attributeNames {
					fmt.Fprintf(stdout, "  %s=%s\\n", name, resource.Attributes[name])
				}
				fmt.Fprintln(stdout, "Endpoint evidence:")
				for _, endpoint := range resource.Endpoints {
					fmt.Fprintf(stdout, "  scope=%s address=%s port=%d protocol=%s provenance=%s\n", endpoint.Scope, endpoint.Address, endpoint.Port, endpoint.Protocol, endpoint.Provenance)
				}
				if resource.MissingSince != nil {
					fmt.Fprintf(stdout, "Missing since: %s (%d complete snapshots)\n", resource.MissingSince.Format(time.RFC3339Nano), resource.MissingSnapshots)
				}
				return 0
			}
		}
		fmt.Fprintf(stderr, "inventory inspect: resource %q not found\n", key)
		return 1
	case "status":
		if flags.NArg() != 0 {
			fmt.Fprintln(stderr, "usage error: inventory status does not accept positional arguments")
			return 2
		}
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
				provider.ProviderID, provider.ProviderInstanceID, state, provider.ResourceCount, formatInventoryTime(provider.LastAttemptAt), formatInventoryTime(provider.LastSuccessAt), emptyAsNone(provider.LastErrorCode))
		}
		return 0
	default:
		fmt.Fprintf(stderr, "usage error: unknown inventory command %q\n", command)
		writeInventoryHelp(stderr)
		return 2
	}
}

func formatInventoryTime(value time.Time) string {
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
