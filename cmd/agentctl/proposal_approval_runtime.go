package main

import (
	"fmt"
	"runtime"

	"github.com/ar-imms/telemetry-agent/internal/app"
	"github.com/ar-imms/telemetry-agent/internal/configuration/proposals/application"
	"github.com/ar-imms/telemetry-agent/internal/identity/host"
	matchingapp "github.com/ar-imms/telemetry-agent/internal/monitoring/matching/application"
	matchingdomain "github.com/ar-imms/telemetry-agent/internal/monitoring/matching/domain"
	definitionapp "github.com/ar-imms/telemetry-agent/internal/monitoring/profiles/definitions/application"
	monitoringtemplatesapp "github.com/ar-imms/telemetry-agent/internal/monitoring/templates/application"
	collectorruntime "github.com/ar-imms/telemetry-agent/internal/runtime/collector"
)

// newRuntimeProposalRefresher wires the Configuration workflow to Inventory and Monitoring adapters.
func newRuntimeProposalRefresher(statePath string) (application.ProposalRefreshService, error) {
	registry, err := productionIntegrationRegistry()
	if err != nil {
		return application.ProposalRefreshService{}, err
	}
	platform := identity.PlatformInfo{OS: runtime.GOOS, Architecture: runtime.GOARCH}
	var assessor matchingdomain.CapabilityAssessor
	if catalog, catalogErr := collectorruntime.StaticCatalog(platform); catalogErr == nil {
		assessor = matchingdomain.NewCatalogCapabilityAssessor(catalog, platform.OS, platform.OS+"/"+platform.Architecture)
	}
	inventoryRepository := app.NewInventoryRepository(statePath)
	inventory, ok := inventoryRepository.(application.InventorySnapshotReader)
	if !ok {
		return application.ProposalRefreshService{}, fmt.Errorf("Inventory repository does not support cancellable snapshots")
	}
	templateRepository := app.NewTemplateRepository(statePath)
	templates, ok := templateRepository.(application.TemplateSnapshotReader)
	if !ok {
		return application.ProposalRefreshService{}, fmt.Errorf("Template repository does not support cancellable snapshots")
	}
	return application.NewProposalRefreshService(
		inventory, templates, application.NewProposalManager(app.NewProposalRepository(statePath)),
		matchingapp.NewMatcher(registry, assessor), monitoringtemplatesapp.NewEvaluator(registry, assessor),
	), nil
}

// newRuntimeApprovalService wires Configuration approval to fresh Inventory and Monitoring evidence.
func newRuntimeApprovalService(statePath string) (application.ApprovalService, error) {
	registry, err := productionIntegrationRegistry()
	if err != nil {
		return application.ApprovalService{}, err
	}
	platform := identity.PlatformInfo{OS: runtime.GOOS, Architecture: runtime.GOARCH}
	var assessor matchingdomain.CapabilityAssessor
	if catalog, catalogErr := collectorruntime.StaticCatalog(platform); catalogErr == nil {
		assessor = matchingdomain.NewCatalogCapabilityAssessor(catalog, platform.OS, platform.OS+"/"+platform.Architecture)
	}
	inventoryRepository := app.NewInventoryRepository(statePath)
	inventory, ok := inventoryRepository.(application.InventorySnapshotReader)
	if !ok {
		return application.ApprovalService{}, fmt.Errorf("Inventory repository does not support cancellable snapshots")
	}
	verifier := application.NewCandidateVerificationService(
		inventory, matchingapp.NewMatcher(registry, assessor), definitionapp.NewResolver(registry),
	)
	return application.NewApprovalService(app.NewProposalRepository(statePath), verifier, app.NewApprovalRepository(statePath)), nil
}
