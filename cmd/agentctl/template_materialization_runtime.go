package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"runtime"
	"sort"
	"time"

	"github.com/ar-imms/telemetry-agent/internal/agentstate"
	"github.com/ar-imms/telemetry-agent/internal/collectorruntime"
	"github.com/ar-imms/telemetry-agent/internal/configuration"
	"github.com/ar-imms/telemetry-agent/internal/discovery"
	"github.com/ar-imms/telemetry-agent/internal/identity"
	"github.com/ar-imms/telemetry-agent/internal/integration"
	"github.com/ar-imms/telemetry-agent/internal/monitoringprofile"
	"github.com/ar-imms/telemetry-agent/internal/platform/events"
)

const materializationCASAttempts = 3

type runtimeTemplateMaterializer struct {
	inventory        discovery.InventoryRepository
	templates        configuration.TemplateRepository
	desired          configuration.IntegrationRepository
	service          configuration.MaterializationService
	matcher          integration.Matcher
	nativeMonitoring bool
	nativeProfiles   monitoringprofile.Registry
}

func newRuntimeTemplateMaterializer(statePath string, publisher ...events.Publisher) (runtimeTemplateMaterializer, error) {
	store := agentstate.NewSQLiteStore(statePath)
	state, err := agentstate.NewStore(statePath).Load()
	if err != nil {
		return runtimeTemplateMaterializer{}, fmt.Errorf("load Agent config root for template materialization: %w", err)
	}
	profiles, err := monitoringprofile.LoadDirectory(state.Collector.ConfigRoot)
	if err != nil {
		return runtimeTemplateMaterializer{}, fmt.Errorf("load local monitoring profiles: %w", err)
	}
	registry, err := productionIntegrationRegistry()
	if err != nil {
		return runtimeTemplateMaterializer{}, err
	}
	platform := identity.PlatformInfo{OS: runtime.GOOS, Architecture: runtime.GOARCH}
	var assessor integration.CapabilityAssessor
	if catalog, catalogErr := collectorruntime.StaticCatalog(platform); catalogErr == nil {
		assessor = catalogCapabilityAssessor{catalog: catalog, platform: platform.OS + "/" + platform.Architecture}
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
		service:          configuration.NewMaterializationService(materializationRepository),
		matcher:          integration.NewMatcher(registry, assessor),
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
		matches, err := r.matcher.MatchTemplates(matchSnapshot, templates.Templates)
		if err != nil {
			return fmt.Errorf("match Templates against inventory: %w", err)
		}
		plan, err := configuration.BuildMaterializationPlan(configuration.MaterializationInput{
			InventoryRevision: inventory.Revision, InventoryWriteRevision: inventory.WriteRevision,
			Templates: templates, Desired: desired, Resources: matchSnapshot.Resources, Matches: matches,
			StaleAfter: discovery.DefaultPolicy().StaleAfter,
		})
		if err != nil {
			return fmt.Errorf("build Integration materialization plan: %w", err)
		}
		if _, err := r.service.Apply(plan); err != nil {
			if errors.Is(err, configuration.ErrMaterializationConflict) {
				continue
			}
			return err
		}
		logMaterializationDiagnostics(plan.Diagnostics)
		return nil
	}
	return configuration.ErrMaterializationConflict
}

func logMaterializationDiagnostics(diagnostics []configuration.MaterializationDiagnostic) {
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
