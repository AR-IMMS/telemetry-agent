package main

import (
	"context"
	"errors"
	"fmt"
	desiredstateapp "github.com/ar-imms/telemetry-agent/internal/configuration/desiredstate/application"
	materializationapp "github.com/ar-imms/telemetry-agent/internal/configuration/materialization/application"
	materializationdomain "github.com/ar-imms/telemetry-agent/internal/configuration/materialization/domain"
	templatesconfigapp "github.com/ar-imms/telemetry-agent/internal/configuration/templates/application"
	"github.com/ar-imms/telemetry-agent/internal/identity/host"
	inventoryctx "github.com/ar-imms/telemetry-agent/internal/inventory"
	matchingdomain "github.com/ar-imms/telemetry-agent/internal/monitoring/matching/domain"
	monitoringdomain "github.com/ar-imms/telemetry-agent/internal/monitoring/profiles/domain"
	templatesmonitoringapp "github.com/ar-imms/telemetry-agent/internal/monitoring/templates/application"
	"github.com/ar-imms/telemetry-agent/internal/platform/events"
	"github.com/ar-imms/telemetry-agent/internal/runtime/collector"
	"log"
	"runtime"
	"sort"
	"time"
)

const materializationCASAttempts = 3

type runtimeTemplateMaterializer struct {
	inventory        inventoryctx.InventoryRepository
	templates        templatesconfigapp.Repository
	desired          desiredstateapp.Repository
	service          materializationapp.Service
	matcher          templatesmonitoringapp.Evaluator
	nativeMonitoring bool
	nativeProfiles   monitoringdomain.Registry
}

func newRuntimeTemplateMaterializer(statePath string, publisher ...events.Publisher) (runtimeTemplateMaterializer, error) {
	store := newAgentSQLiteStore(statePath)
	state, err := newAgentStateStore(statePath).Load()
	if err != nil {
		return runtimeTemplateMaterializer{}, fmt.Errorf("load Agent config root for template materialization: %w", err)
	}
	profiles, err := loadMonitoringProfiles(state.Collector.ConfigRoot)
	if err != nil {
		return runtimeTemplateMaterializer{}, fmt.Errorf("load local monitoring profiles: %w", err)
	}
	registry, err := productionIntegrationRegistry()
	if err != nil {
		return runtimeTemplateMaterializer{}, err
	}
	platform := identity.PlatformInfo{OS: runtime.GOOS, Architecture: runtime.GOARCH}
	var assessor matchingdomain.CapabilityAssessor
	if catalog, catalogErr := collectorruntime.StaticCatalog(platform); catalogErr == nil {
		assessor = matchingdomain.NewCatalogCapabilityAssessor(catalog, platform.OS, platform.OS+"/"+platform.Architecture)
	}
	materializationRepository := store.MaterializationRepository()
	if len(publisher) > 0 && publisher[0] != nil {
		materializationRepository = store.MaterializationRepositoryWithEvents(publisher[0])
	}
	nativeLayers, _, err := nativeDockerMonitoringInputs(statePath, platform.OS)
	if err != nil {
		return runtimeTemplateMaterializer{}, fmt.Errorf("resolve native monitoring for template materialization: %w", err)
	}
	return runtimeTemplateMaterializer{
		inventory: store.InventoryRepository(), templates: store.TemplateRepository(), desired: store.IntegrationRepository(),
		service:          materializationapp.NewService(materializationRepository),
		matcher:          templatesmonitoringapp.NewEvaluator(registry, assessor),
		nativeMonitoring: len(nativeLayers) > 0, nativeProfiles: profiles,
	}, nil
}

func (r runtimeTemplateMaterializer) Refresh(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	for attempt := 0; attempt < materializationCASAttempts; attempt++ {
		inventory, err := r.inventory.Snapshot()
		if err != nil {
			return fmt.Errorf("read inventory for template materialization: %w", err)
		}
		templates, err := r.templates.Snapshot()
		if err != nil {
			return fmt.Errorf("read Templates for materialization: %w", err)
		}
		desired, err := r.desired.Snapshot()
		if err != nil {
			return fmt.Errorf("read desired Integrations for materialization: %w", err)
		}
		asOf := time.Now().UTC()
		matchSnapshot := projectInventory(inventory, asOf)
		if r.nativeMonitoring {
			filtered := matchSnapshot.Resources[:0]
			for _, resource := range matchSnapshot.Resources {
				if _, native := r.nativeProfiles.Match(resource.Provider, resource.Attributes); native {
					continue
				}
				filtered = append(filtered, resource)
			}
			matchSnapshot.Resources = filtered
		}
		monitoringTemplates, err := monitoringTemplateInputs(templates.Templates)
		if err != nil {
			return fmt.Errorf("project Configuration Templates for Monitoring evaluation: %w", err)
		}
		matches, err := r.matcher.MatchTemplates(matchSnapshot, monitoringTemplates)
		if err != nil {
			return fmt.Errorf("match Templates against inventory: %w", err)
		}
		plan, err := materializationapp.BuildPlan(materializationapp.Input{
			InventoryRevision: inventory.Revision, InventoryWriteRevision: inventory.WriteRevision,
			Templates: templates, Desired: desired, Resources: matchSnapshot.Resources, Matches: matches,
			StaleAfter: inventoryctx.DefaultPolicy().StaleAfter,
		})
		if err != nil {
			return fmt.Errorf("build Integration materialization plan: %w", err)
		}
		if _, err := r.service.Apply(plan); err != nil {
			if errors.Is(err, materializationdomain.ErrMaterializationConflict) {
				continue
			}
			return err
		}
		logMaterializationDiagnostics(plan.Diagnostics)
		return nil
	}
	return materializationdomain.ErrMaterializationConflict
}

func logMaterializationDiagnostics(diagnostics []materializationdomain.Diagnostic) {
	if len(diagnostics) == 0 {
		return
	}
	counts := make(map[string]int)
	for _, diagnostic := range diagnostics {
		counts[diagnostic.Code]++
	}
	codes := make([]string, 0, len(counts))
	for code := range counts {
		codes = append(codes, code)
	}
	sort.Strings(codes)
	for _, code := range codes {
		log.Printf("Template materialization skipped candidates code=%s count=%d", code, counts[code])
	}
}
