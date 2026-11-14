package main

import (
	"context"
	"time"

	"github.com/ar-imms/telemetry-agent/internal/agenthealth"
	"github.com/ar-imms/telemetry-agent/internal/agentstate"
	"github.com/ar-imms/telemetry-agent/internal/dependency"
	"github.com/ar-imms/telemetry-agent/internal/discovery"
)

const dependencyEvidenceTimeout = 5 * time.Second

type inventoryRepositoryProvider interface {
	InventoryRepository() discovery.InventoryRepository
}

func newAgentHealthEvidenceSources(statePath string) []agenthealth.EvidenceSource {
	store, ok := agentstate.NewStore(statePath).(inventoryRepositoryProvider)
	var inventory discovery.ProviderStatusReader
	if ok {
		inventory, _ = store.InventoryRepository().(discovery.ProviderStatusReader)
	}

	return []agenthealth.EvidenceSource{
		func(now time.Time) agenthealth.ComponentEvidence {
			if inventory == nil {
				return agenthealth.ComponentEvidence{
					Component: agenthealth.ComponentInventory, State: agenthealth.EvidenceUnknown,
					Freshness: agenthealth.FreshnessUnknown, Completeness: agenthealth.CompletenessUnknown,
					ReasonCode: "inventory_repository_unavailable",
				}
			}
			statuses, truncated, err := inventory.ReadProviderStatuses(discovery.MaxProviderStatusReadLimit)
			return evaluateInventoryEvidence(now, statuses, truncated, err)
		},
		func(now time.Time) agenthealth.ComponentEvidence {
			ctx, cancel := context.WithTimeout(context.Background(), dependencyEvidenceTimeout)
			defer cancel()
			statuses, err := defaultListDependencyStatuses(ctx)
			return evaluateDependencyEvidence(now, statuses, err)
		},
	}
}

func evaluateInventoryEvidence(now time.Time, providers []discovery.ProviderStatus, truncated bool, sourceErr error) agenthealth.ComponentEvidence {
	evidence := agenthealth.ComponentEvidence{
		Component:    agenthealth.ComponentInventory,
		State:        agenthealth.EvidenceUnknown,
		Freshness:    agenthealth.FreshnessUnknown,
		Completeness: agenthealth.CompletenessUnknown,
		ReasonCode:   "inventory_not_observed",
	}
	if sourceErr != nil {
		evidence.State = agenthealth.EvidenceDegraded
		evidence.ReasonCode = "inventory_snapshot_unavailable"
		return evidence
	}
	if truncated {
		evidence.Completeness = agenthealth.CompletenessPartial
		evidence.ReasonCode = "inventory_provider_status_truncated"
		return evidence
	}
	if len(providers) == 0 {
		return evidence
	}

	policy := discovery.DefaultPolicy()
	enabled := 0
	allSuccessful := true
	hasFailure := false
	var lastAttempt time.Time
	var validUntil time.Time
	for _, provider := range providers {
		if !provider.Enabled {
			continue
		}
		enabled++
		if provider.LastAttemptAt.After(lastAttempt) {
			lastAttempt = provider.LastAttemptAt
		}
		if provider.LastErrorCode != "" {
			hasFailure = true
			allSuccessful = false
		}
		if provider.LastSuccessAt.IsZero() {
			allSuccessful = false
			continue
		}
		providerValidUntil := provider.LastSuccessAt.Add(policy.StaleAfter)
		if validUntil.IsZero() || providerValidUntil.Before(validUntil) {
			validUntil = providerValidUntil
		}
	}
	if enabled == 0 {
		evidence.State = agenthealth.EvidenceUnsupported
		evidence.Completeness = agenthealth.CompletenessComplete
		evidence.ReasonCode = "no_enabled_inventory_providers"
		return evidence
	}
	if !lastAttempt.IsZero() {
		observedAt := lastAttempt.UTC()
		evidence.ObservedAt = &observedAt
	}
	if !validUntil.IsZero() {
		validUntil = validUntil.UTC()
		evidence.ValidUntil = &validUntil
		evidence.Freshness = agenthealth.FreshnessFresh
		if !now.Before(validUntil) {
			evidence.Freshness = agenthealth.FreshnessStale
		}
	}
	if hasFailure {
		evidence.State = agenthealth.EvidenceDegraded
		evidence.Completeness = agenthealth.CompletenessPartial
		evidence.ReasonCode = "provider_observation_failed"
		return evidence
	}
	if allSuccessful {
		evidence.State = agenthealth.EvidenceHealthy
		evidence.Completeness = agenthealth.CompletenessComplete
		evidence.ReasonCode = "provider_snapshot_complete"
		return evidence
	}
	evidence.Completeness = agenthealth.CompletenessPartial
	evidence.ReasonCode = "provider_success_not_observed"
	return evidence
}

func evaluateDependencyEvidence(now time.Time, results []dependency.StatusResult, sourceErr error) agenthealth.ComponentEvidence {
	evidence := agenthealth.ComponentEvidence{
		Component:    agenthealth.ComponentDependencies,
		State:        agenthealth.EvidenceUnknown,
		Freshness:    agenthealth.FreshnessUnknown,
		Completeness: agenthealth.CompletenessUnknown,
		ReasonCode:   "dependency_status_not_observed",
	}
	if sourceErr != nil {
		evidence.State = agenthealth.EvidenceUnknown
		evidence.ReasonCode = "dependency_status_unavailable"
		return evidence
	}
	observedAt := now.UTC()
	evidence.ObservedAt = &observedAt
	evidence.Freshness = agenthealth.FreshnessFresh
	evidence.Completeness = agenthealth.CompletenessComplete
	if len(results) == 0 {
		evidence.State = agenthealth.EvidenceUnsupported
		evidence.ReasonCode = "no_supported_dependencies"
		return evidence
	}

	enabled, hasUnknown, hasDegraded := 0, false, false
	for _, result := range results {
		if result.InspectionError != nil {
			hasDegraded = true
			evidence.Completeness = agenthealth.CompletenessPartial
			continue
		}
		switch result.Status.Availability {
		case dependency.AvailabilityDisabled:
			continue
		case dependency.AvailabilityEnabled:
			enabled++
			if result.Status.Health == dependency.HealthUnhealthy || result.Status.Drifted {
				hasDegraded = true
			} else if result.Status.Health != dependency.HealthHealthy {
				hasUnknown = true
			}
		default:
			hasUnknown = true
		}
	}
	if hasDegraded {
		evidence.State = agenthealth.EvidenceDegraded
		evidence.ReasonCode = "dependency_failure_observed"
		return evidence
	}
	if hasUnknown {
		evidence.State = agenthealth.EvidenceUnknown
		evidence.ReasonCode = "dependency_status_incomplete"
		return evidence
	}
	if enabled == 0 {
		evidence.State = agenthealth.EvidenceUnsupported
		evidence.ReasonCode = "no_enabled_dependencies"
		return evidence
	}
	evidence.State = agenthealth.EvidenceHealthy
	evidence.ReasonCode = "enabled_dependencies_healthy"
	return evidence
}
