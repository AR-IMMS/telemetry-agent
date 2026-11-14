package agenthealth

import "time"

// InventoryProviderObservation is the Health-safe projection of one provider's persisted status.
type InventoryProviderObservation struct {
	Enabled               bool
	LastAttemptAt         time.Time
	LastSuccessValidUntil time.Time
	FailureObserved       bool
}

// EvaluateInventoryEvidence classifies bounded provider observations without owning their persistence or freshness policy.
func EvaluateInventoryEvidence(now time.Time, providers []InventoryProviderObservation, truncated, sourceAvailable bool) ComponentEvidence {
	evidence := ComponentEvidence{
		Component: ComponentInventory, State: EvidenceUnknown,
		Freshness: FreshnessUnknown, Completeness: CompletenessUnknown,
		ReasonCode: "inventory_not_observed",
	}
	if !sourceAvailable {
		evidence.State = EvidenceDegraded
		evidence.ReasonCode = "inventory_snapshot_unavailable"
		return evidence
	}
	if truncated {
		evidence.Completeness = CompletenessPartial
		evidence.ReasonCode = "inventory_provider_status_truncated"
		return evidence
	}
	if len(providers) == 0 {
		return evidence
	}

	enabled, allSuccessful, hasFailure := 0, true, false
	var lastAttempt, validUntil time.Time
	for _, provider := range providers {
		if !provider.Enabled {
			continue
		}
		enabled++
		if provider.LastAttemptAt.After(lastAttempt) {
			lastAttempt = provider.LastAttemptAt
		}
		if provider.FailureObserved {
			hasFailure = true
			allSuccessful = false
		}
		if provider.LastSuccessValidUntil.IsZero() {
			allSuccessful = false
			continue
		}
		if validUntil.IsZero() || provider.LastSuccessValidUntil.Before(validUntil) {
			validUntil = provider.LastSuccessValidUntil
		}
	}
	if enabled == 0 {
		evidence.State = EvidenceUnsupported
		evidence.Completeness = CompletenessComplete
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
		evidence.Freshness = FreshnessFresh
		if !now.Before(validUntil) {
			evidence.Freshness = FreshnessStale
		}
	}
	if hasFailure {
		evidence.State = EvidenceDegraded
		evidence.Completeness = CompletenessPartial
		evidence.ReasonCode = "provider_observation_failed"
		return evidence
	}
	if evidence.Freshness == FreshnessStale {
		evidence.State = EvidenceDegraded
		evidence.Completeness = CompletenessComplete
		evidence.ReasonCode = "inventory_snapshot_stale"
		return evidence
	}
	if allSuccessful {
		evidence.State = EvidenceHealthy
		evidence.Completeness = CompletenessComplete
		evidence.ReasonCode = "provider_snapshot_complete"
		return evidence
	}
	evidence.Completeness = CompletenessPartial
	evidence.ReasonCode = "provider_success_not_observed"
	return evidence
}

// DependencyAvailability describes whether a dependency is enabled for this Agent.
type DependencyAvailability string

const (
	// DependencyAvailabilityUnknown indicates that dependency enablement was not observed.
	DependencyAvailabilityUnknown DependencyAvailability = "unknown"
	// DependencyAvailabilityDisabled indicates that the Agent does not manage the dependency as enabled.
	DependencyAvailabilityDisabled DependencyAvailability = "disabled"
	// DependencyAvailabilityEnabled indicates that the dependency is enabled for the Agent.
	DependencyAvailabilityEnabled DependencyAvailability = "enabled"
)

// DependencyHealth describes the observed runtime health of an enabled dependency.
type DependencyHealth string

const (
	// DependencyHealthUnknown indicates that enabled dependency health was not observed.
	DependencyHealthUnknown DependencyHealth = "unknown"
	// DependencyHealthHealthy indicates that the dependency inspection reported healthy.
	DependencyHealthHealthy DependencyHealth = "healthy"
	// DependencyHealthUnhealthy indicates that the dependency inspection reported unhealthy.
	DependencyHealthUnhealthy DependencyHealth = "unhealthy"
)

// DependencyObservation is a bounded status projection that contains no source errors or credentials.
type DependencyObservation struct {
	Availability     DependencyAvailability
	Health           DependencyHealth
	Drifted          bool
	InspectionFailed bool
}

// EvaluateDependencyEvidence classifies read-only dependency observations and does not inspect or mutate dependencies.
func EvaluateDependencyEvidence(now time.Time, results []DependencyObservation, sourceAvailable bool) ComponentEvidence {
	evidence := ComponentEvidence{
		Component: ComponentDependencies, State: EvidenceUnknown,
		Freshness: FreshnessUnknown, Completeness: CompletenessUnknown,
		ReasonCode: "dependency_status_not_observed",
	}
	if !sourceAvailable {
		evidence.ReasonCode = "dependency_status_unavailable"
		return evidence
	}
	observedAt := now.UTC()
	evidence.ObservedAt = &observedAt
	evidence.Freshness = FreshnessFresh
	evidence.Completeness = CompletenessComplete
	if len(results) == 0 {
		evidence.State = EvidenceUnsupported
		evidence.ReasonCode = "no_supported_dependencies"
		return evidence
	}

	enabled, hasUnknown, hasDegraded := 0, false, false
	for _, result := range results {
		if result.InspectionFailed {
			hasDegraded = true
			evidence.Completeness = CompletenessPartial
			continue
		}
		switch result.Availability {
		case DependencyAvailabilityDisabled:
			continue
		case DependencyAvailabilityEnabled:
			enabled++
			if result.Health == DependencyHealthUnhealthy || result.Drifted {
				hasDegraded = true
			} else if result.Health != DependencyHealthHealthy {
				hasUnknown = true
			}
		default:
			hasUnknown = true
		}
	}
	if hasDegraded {
		evidence.State = EvidenceDegraded
		evidence.ReasonCode = "dependency_failure_observed"
		return evidence
	}
	if hasUnknown {
		evidence.State = EvidenceUnknown
		evidence.ReasonCode = "dependency_status_incomplete"
		return evidence
	}
	if enabled == 0 {
		evidence.State = EvidenceUnsupported
		evidence.ReasonCode = "no_enabled_dependencies"
		return evidence
	}
	evidence.State = EvidenceHealthy
	evidence.ReasonCode = "enabled_dependencies_healthy"
	return evidence
}
