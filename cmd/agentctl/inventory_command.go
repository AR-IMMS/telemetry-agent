package main

import (
	"context"
	"io"

	inventorycommand "github.com/ar-imms/telemetry-agent/internal/cli/commands/inventory"
	agenthealth "github.com/ar-imms/telemetry-agent/internal/health"
	profilesapplication "github.com/ar-imms/telemetry-agent/internal/monitoring/profiles/application"
)

// runInventory preserves the legacy dispatcher entry while command parsing and
// presentation live in the CLI Inventory capability.
func runInventory(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	return inventorycommand.Execute(ctx, args, stdout, stderr, inventoryCommandDependencies())
}

func collectorScrapeEvidenceFromSnapshot(snapshot agenthealth.Snapshot) []profilesapplication.CollectorScrapeEvidence {
	return inventorycommand.CollectorScrapeEvidenceFromSnapshot(snapshot)
}
