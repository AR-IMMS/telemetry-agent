package agenthealth

import "time"

// Component identifies an independently assessed health dimension.
type Component string

const (
	// ComponentAgentRuntime reports whether a local Agent health snapshot was generated.
	ComponentAgentRuntime Component = "agent_runtime"
	// ComponentCoreConnectivity reports observed connectivity to AR-IMMS Core.
	ComponentCoreConnectivity Component = "core_connectivity"
	// ComponentSupervisor reports the managed OpAMP Supervisor process when configured.
	ComponentSupervisor Component = "supervisor"
	// ComponentCollector reports the supervised Collector lifecycle state.
	ComponentCollector Component = "collector"
	// ComponentConfiguration reports generation activation and application state.
	ComponentConfiguration Component = "configuration"
	// ComponentInventory reports persisted provider inventory evidence.
	ComponentInventory Component = "inventory"
	// ComponentMonitoring reports profile and receiver monitoring evidence.
	ComponentMonitoring Component = "monitoring"
	// ComponentDependencies reports installed dependency evidence.
	ComponentDependencies Component = "dependencies"
	// ComponentTelemetryCollection reports direct telemetry collection evidence.
	ComponentTelemetryCollection Component = "telemetry_collection"
	// ComponentTelemetryDelivery reports measured receipt at the configured backend.
	ComponentTelemetryDelivery Component = "telemetry_delivery"
)

// EvidenceState describes what the available evidence supports.
type EvidenceState string

const (
	// EvidenceHealthy indicates that current evidence supports the healthy claim.
	EvidenceHealthy EvidenceState = "healthy"
	// EvidenceDegraded indicates that evidence records a known failure or stale result.
	EvidenceDegraded EvidenceState = "degraded"
	// EvidenceUnknown indicates evidence is missing, incomplete, or not current enough.
	EvidenceUnknown EvidenceState = "unknown"
	// EvidenceUnsupported indicates the component is not configured or not applicable.
	EvidenceUnsupported EvidenceState = "unsupported"
)

// EvidenceFreshness describes whether evidence is current under its owner's validity contract.
type EvidenceFreshness string

const (
	// FreshnessFresh indicates the evidence has not reached its owner-provided validity deadline.
	FreshnessFresh EvidenceFreshness = "fresh"
	// FreshnessStale indicates evidence is at or beyond its owner-provided validity deadline.
	FreshnessStale EvidenceFreshness = "stale"
	// FreshnessUnknown indicates no usable observation/validity boundary is available.
	FreshnessUnknown EvidenceFreshness = "unknown"
)

// EvidenceCompleteness describes whether the source supplied the evidence required for its claim.
type EvidenceCompleteness string

const (
	// CompletenessComplete indicates the evidence owner supplied the evidence needed for its claim.
	CompletenessComplete EvidenceCompleteness = "complete"
	// CompletenessPartial indicates only part of the evidence required for its claim was supplied.
	CompletenessPartial EvidenceCompleteness = "partial"
	// CompletenessUnknown indicates no completeness information was supplied.
	CompletenessUnknown EvidenceCompleteness = "unknown"
)

// ComponentEvidence is a bounded, secret-free status projection for one component.
type ComponentEvidence struct {
	Component    Component            `json:"component"`
	State        EvidenceState        `json:"state"`
	Freshness    EvidenceFreshness    `json:"freshness"`
	Completeness EvidenceCompleteness `json:"completeness"`
	ObservedAt   *time.Time           `json:"observedAt,omitempty"`
	ValidUntil   *time.Time           `json:"validUntil,omitempty"`
	ReasonCode   string               `json:"reasonCode,omitempty"`
}

// Assessment summarizes independent component evidence without inferring collection or delivery.
type Assessment struct {
	State       EvidenceState       `json:"state"`
	GeneratedAt time.Time           `json:"generatedAt"`
	Components  []ComponentEvidence `json:"components"`
}

var assessmentComponents = []Component{
	ComponentAgentRuntime,
	ComponentCoreConnectivity,
	ComponentSupervisor,
	ComponentCollector,
	ComponentConfiguration,
	ComponentInventory,
	ComponentMonitoring,
	ComponentDependencies,
	ComponentTelemetryCollection,
	ComponentTelemetryDelivery,
}

func evaluateAssessment(now time.Time, observation RuntimeObservation) Assessment {
	components := make([]ComponentEvidence, 0, len(assessmentComponents))
	observedAt := timePointer(observation.ObservedAt)

	components = append(components, ComponentEvidence{
		Component: ComponentAgentRuntime, State: EvidenceHealthy,
		Freshness: FreshnessFresh, Completeness: CompletenessComplete,
		ObservedAt: timePointer(now), ReasonCode: "status_snapshot_generated",
	})
	components = append(components, unknownEvidence(ComponentCoreConnectivity, "core_connectivity_not_observed"))
	if !observation.SupervisorConfigured {
		components = append(components, ComponentEvidence{
			Component: ComponentSupervisor, State: EvidenceUnsupported,
			Freshness: FreshnessUnknown, Completeness: CompletenessComplete,
			ReasonCode: "legacy_runtime_owner",
		})
	} else {
		state, reason := supervisorEvidence(observation)
		components = append(components, ComponentEvidence{
			Component: ComponentSupervisor, State: state,
			Freshness:    evidenceFreshness(now, observation.ObservedAt, observation.ValidUntil),
			Completeness: evidenceCompleteness(observation.ObservedAt, observation.CollectorState != ""),
			ObservedAt:   observedAt, ValidUntil: timePointerFrom(observation.ValidUntil), ReasonCode: reason,
		})
	}

	collectorState, collectorReason := collectorEvidence(observation)
	components = append(components, ComponentEvidence{
		Component: ComponentCollector, State: collectorState,
		Freshness:    evidenceFreshness(now, observation.ObservedAt, observation.ValidUntil),
		Completeness: evidenceCompleteness(observation.ObservedAt, observation.CollectorState != ""),
		ObservedAt:   observedAt, ValidUntil: timePointerFrom(observation.ValidUntil), ReasonCode: collectorReason,
	})

	configurationState, configurationReason := configurationEvidence(observation)
	components = append(components, ComponentEvidence{
		Component: ComponentConfiguration, State: configurationState,
		Freshness:    evidenceFreshness(now, observation.ObservedAt, observation.ValidUntil),
		Completeness: evidenceCompleteness(observation.ObservedAt, configurationWasObserved(observation)),
		ObservedAt:   observedAt, ValidUntil: timePointerFrom(observation.ValidUntil), ReasonCode: configurationReason,
	})

	components = append(components,
		unknownEvidence(ComponentInventory, "inventory_not_aggregated"),
		unknownEvidence(ComponentMonitoring, "monitoring_success_not_observed"),
		unknownEvidence(ComponentDependencies, "dependency_status_not_aggregated"),
		unknownEvidence(ComponentTelemetryCollection, "successful_collection_not_observed"),
		unknownEvidence(ComponentTelemetryDelivery, "backend_receipt_not_measured"),
	)

	state := EvidenceHealthy
	for index := range components {
		item := &components[index]
		if item.ObservedAt != nil && item.ValidUntil != nil && !now.Before(*item.ValidUntil) {
			item.Freshness = FreshnessStale
		}
		if item.State == EvidenceDegraded || item.Freshness == FreshnessStale {
			state = EvidenceDegraded
		} else if item.State != EvidenceUnsupported &&
			(item.State == EvidenceUnknown || item.Freshness == FreshnessUnknown || item.Completeness == CompletenessUnknown) &&
			state != EvidenceDegraded {
			state = EvidenceUnknown
		}
	}

	return Assessment{State: state, GeneratedAt: now.UTC(), Components: components}
}

func unknownEvidence(component Component, reason string) ComponentEvidence {
	return ComponentEvidence{
		Component: component, State: EvidenceUnknown,
		Freshness: FreshnessUnknown, Completeness: CompletenessUnknown,
		ReasonCode: reason,
	}
}

func collectorEvidence(observation RuntimeObservation) (EvidenceState, string) {
	if observation.LastError != "" {
		return EvidenceDegraded, "runtime_error_observed"
	}
	switch observation.CollectorState {
	case CollectorStateReady:
		return EvidenceHealthy, "collector_ready"
	case CollectorStateFailed, CollectorStateStopped:
		return EvidenceDegraded, "collector_not_running"
	case CollectorStateStarting, CollectorStateStopping:
		return EvidenceUnknown, "collector_transitioning"
	default:
		return EvidenceUnknown, "collector_state_not_observed"
	}
}

func configurationEvidence(observation RuntimeObservation) (EvidenceState, string) {
	if observation.PendingTeardowns > 0 {
		return EvidenceDegraded, "dependency_teardown_pending"
	}
	if !observation.ConfigurationObserved {
		return EvidenceUnknown, "configuration_observation_missing"
	}
	if observation.DesiredGeneration == 0 && observation.ActivatedGeneration == 0 && observation.AppliedGeneration == 0 {
		return EvidenceUnknown, "configuration_not_initialized"
	}
	if observation.DesiredGeneration != observation.ActivatedGeneration ||
		observation.ActivatedGeneration != observation.AppliedGeneration {
		return EvidenceDegraded, "configuration_generation_not_applied"
	}
	return EvidenceHealthy, "configuration_generation_applied"
}

func supervisorEvidence(observation RuntimeObservation) (EvidenceState, string) {
	switch observation.CollectorState {
	case CollectorStateReady:
		return EvidenceHealthy, "managed_supervisor_collector_ready"
	case CollectorStateFailed, CollectorStateStopped:
		return EvidenceDegraded, "managed_supervisor_runtime_failed"
	default:
		return EvidenceUnknown, "managed_supervisor_startup_not_confirmed"
	}
}

func evidenceFreshness(now, observedAt time.Time, validUntil *time.Time) EvidenceFreshness {
	if observedAt.IsZero() || validUntil == nil || validUntil.IsZero() {
		return FreshnessUnknown
	}
	if !now.Before(*validUntil) {
		return FreshnessStale
	}
	return FreshnessFresh
}

func evidenceCompleteness(observedAt time.Time, claimEvidencePresent bool) EvidenceCompleteness {
	if observedAt.IsZero() {
		return CompletenessUnknown
	}
	if !claimEvidencePresent {
		return CompletenessPartial
	}
	return CompletenessComplete
}

func configurationWasObserved(observation RuntimeObservation) bool {
	return observation.ConfigurationObserved
}

func timePointer(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}
	result := value.UTC()
	return &result
}

func timePointerFrom(value *time.Time) *time.Time {
	if value == nil || value.IsZero() {
		return nil
	}
	result := value.UTC()
	return &result
}
