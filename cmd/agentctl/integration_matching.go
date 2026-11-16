package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"runtime"
	"sort"
	"time"

	"github.com/ar-imms/telemetry-agent/internal/agentstate"
	"github.com/ar-imms/telemetry-agent/internal/identity/host"
	"github.com/ar-imms/telemetry-agent/internal/integration"
	inventoryctx "github.com/ar-imms/telemetry-agent/internal/inventory"
	"github.com/ar-imms/telemetry-agent/internal/runtime/collector"
)

type integrationMatchOutput struct {
	InventoryRevision uint64                       `json:"inventory_revision"`
	EvaluatedAt       time.Time                    `json:"evaluated_at"`
	Results           []integrationMatchResultView `json:"results"`
	Diagnostics       []matchDiagnosticView        `json:"diagnostics"`
}

type integrationMatchResultView struct {
	Provider            string                       `json:"provider"`
	ProviderInstance    string                       `json:"provider_instance"`
	ResourceID          string                       `json:"resource_id"`
	DefinitionID        string                       `json:"definition_id,omitempty"`
	DefinitionVersion   string                       `json:"definition_version,omitempty"`
	MatchStatus         integration.MatchStatus      `json:"match_status"`
	ResolutionStatus    integration.ResolutionStatus `json:"resolution_status"`
	CapabilityStatus    integration.CapabilityStatus `json:"capability_status"`
	ResourceFingerprint string                       `json:"resource_fingerprint"`
	Evidence            []integration.MatchEvidence  `json:"evidence,omitempty"`
	RequiredInputs      []integration.RequiredInput  `json:"required_inputs,omitempty"`
	Capability          capabilityView               `json:"capability"`
	Candidate           *candidateView               `json:"candidate,omitempty"`
	Diagnostics         []matchDiagnosticView        `json:"diagnostics,omitempty"`
}

type capabilityView struct {
	Status          integration.CapabilityStatus `json:"status"`
	Platform        string                       `json:"platform,omitempty"`
	ArtifactVersion string                       `json:"artifact_version,omitempty"`
	ManifestVersion string                       `json:"manifest_version,omitempty"`
	Required        []string                     `json:"required,omitempty"`
	Unsupported     []string                     `json:"unsupported,omitempty"`
}

type candidateView struct {
	Configuration json.RawMessage `json:"configuration"`
	Digest        string          `json:"digest"`
}

type matchDiagnosticView struct {
	Code         string `json:"code"`
	DefinitionID string `json:"definition_id,omitempty"`
}

func runIntegrationMatch(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("integration match", flag.ContinueOnError)
	flags.SetOutput(stderr)
	statePath := flags.String("state-path", agentstate.DefaultPath(), "persistent Agent state path")
	if err := parseCompatibilityFlags(flags, args); err != nil {
		return 2
	}
	if flags.NArg() != 0 || *statePath == "" {
		fmt.Fprintln(stderr, "usage error: integration match accepts only --state-path")
		return 2
	}
	if err := ctx.Err(); err != nil {
		return integrationCommandError(stderr, "match", err)
	}

	snapshot, err := newAgentSQLiteStore(*statePath).InventoryRepository().Snapshot()
	if err != nil {
		return integrationCommandError(stderr, "match", err)
	}
	registry, err := productionIntegrationRegistry()
	if err != nil {
		return integrationCommandError(stderr, "match", err)
	}
	var assessor integration.CapabilityAssessor
	platform := identity.PlatformInfo{OS: runtime.GOOS, Architecture: runtime.GOARCH}
	if catalog, catalogErr := collectorruntime.StaticCatalog(platform); catalogErr == nil {
		assessor = integration.NewCatalogCapabilityAssessor(catalog, platform.OS, platform.OS+"/"+platform.Architecture)
	}
	report, err := integration.NewMatcher(registry, assessor).Match(projectInventory(snapshot, time.Now().UTC()))
	if err != nil {
		return integrationCommandError(stderr, "match", err)
	}
	output := matchReportView(report)
	encoder := json.NewEncoder(stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(output); err != nil {
		return integrationCommandError(stderr, "match", err)
	}
	return 0
}

func projectInventory(snapshot inventoryctx.InventorySnapshot, asOf time.Time) integration.MatchSnapshot {
	resources := make([]integration.MatchResource, 0, len(snapshot.Resources))
	for _, resource := range snapshot.Resources {
		attributes := make(map[string]string, len(resource.Attributes))
		for key, value := range resource.Attributes {
			attributes[key] = value
		}
		endpoints := make([]integration.MatchEndpoint, 0, len(resource.Endpoints))
		for _, endpoint := range resource.Endpoints {
			endpoints = append(endpoints, integration.MatchEndpoint{
				Address: endpoint.Address, Port: endpoint.Port, Protocol: endpoint.Protocol,
				Scope: string(endpoint.Scope), Provenance: endpoint.Provenance,
			})
		}
		resources = append(resources, integration.MatchResource{
			Provider: string(resource.Key.Provider), ProviderInstance: string(resource.Key.ProviderInstance),
			ResourceID: resource.Key.ProviderResourceID, Kind: string(resource.Kind),
			LifecycleState: resource.LifecycleState, Presence: string(resource.Presence),
			Attributes: attributes, Endpoints: endpoints, LastSeenAt: resource.LastSeenAt,
		})
	}
	sort.Slice(resources, func(i, j int) bool {
		if resources[i].Provider != resources[j].Provider {
			return resources[i].Provider < resources[j].Provider
		}
		if resources[i].ProviderInstance != resources[j].ProviderInstance {
			return resources[i].ProviderInstance < resources[j].ProviderInstance
		}
		return resources[i].ResourceID < resources[j].ResourceID
	})
	return integration.MatchSnapshot{
		InventoryRevision: snapshot.Revision, AsOf: asOf,
		StaleAfter: inventoryctx.DefaultPolicy().StaleAfter, Resources: resources,
	}
}

func matchReportView(report integration.MatchReport) integrationMatchOutput {
	output := integrationMatchOutput{
		InventoryRevision: report.InventoryRevision, EvaluatedAt: report.EvaluatedAt,
		Results:     make([]integrationMatchResultView, 0, len(report.Results)),
		Diagnostics: matchDiagnosticsView(report.Diagnostics),
	}
	for _, result := range report.Results {
		view := integrationMatchResultView{
			Provider: result.Resource.Provider, ProviderInstance: result.Resource.ProviderInstance,
			ResourceID: result.Resource.ResourceID, DefinitionID: result.DefinitionID,
			DefinitionVersion: result.DefinitionVersion, MatchStatus: result.MatchStatus,
			ResolutionStatus: result.ResolutionStatus, CapabilityStatus: result.CapabilityStatus,
			ResourceFingerprint: result.ResourceFingerprint,
			Evidence:            append([]integration.MatchEvidence(nil), result.Evidence...),
			RequiredInputs:      append([]integration.RequiredInput(nil), result.RequiredInputs...),
			Capability: capabilityView{
				Status: result.Capability.Status, Platform: result.Capability.Platform,
				ArtifactVersion: result.Capability.ArtifactVersion, ManifestVersion: result.Capability.ManifestVersion,
				Required:    append([]string(nil), result.Capability.Required...),
				Unsupported: append([]string(nil), result.Capability.Unsupported...),
			}, Diagnostics: matchDiagnosticsView(result.Diagnostics),
		}
		if result.Candidate != nil {
			view.Candidate = &candidateView{Configuration: result.Candidate.Configuration(), Digest: result.Candidate.Digest()}
		}
		output.Results = append(output.Results, view)
	}
	return output
}

func matchDiagnosticsView(diagnostics []integration.MatchDiagnostic) []matchDiagnosticView {
	result := make([]matchDiagnosticView, 0, len(diagnostics))
	for _, diagnostic := range diagnostics {
		result = append(result, matchDiagnosticView{Code: diagnostic.Code, DefinitionID: diagnostic.DefinitionID})
	}
	return result
}
