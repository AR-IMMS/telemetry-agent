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
		func(ctx context.Context, now time.Time) agenthealth.ComponentEvidence {
			if inventory == nil {
				return agenthealth.EvaluateInventoryEvidence(now, nil, false, false)
			}
			statuses, truncated, err := inventory.ReadProviderStatuses(ctx, discovery.MaxProviderStatusReadLimit)
			observations := make([]agenthealth.InventoryProviderObservation, 0, len(statuses))
			staleAfter := discovery.DefaultPolicy().StaleAfter
			for _, status := range statuses {
				validUntil := time.Time{}
				if !status.LastSuccessAt.IsZero() {
					validUntil = status.LastSuccessAt.Add(staleAfter)
				}
				observations = append(observations, agenthealth.InventoryProviderObservation{
					Enabled: status.Enabled, LastAttemptAt: status.LastAttemptAt,
					LastSuccessValidUntil: validUntil, FailureObserved: status.LastErrorCode != "",
				})
			}
			return agenthealth.EvaluateInventoryEvidence(now, observations, truncated, err == nil)
		},
		func(parent context.Context, now time.Time) agenthealth.ComponentEvidence {
			ctx, cancel := context.WithTimeout(parent, dependencyEvidenceTimeout)
			defer cancel()
			statuses, err := defaultListDependencyStatuses(ctx)
			observations := make([]agenthealth.DependencyObservation, 0, len(statuses))
			for _, result := range statuses {
				observations = append(observations, dependencyObservation(result))
			}
			return agenthealth.EvaluateDependencyEvidence(now, observations, err == nil)
		},
	}
}

func dependencyObservation(result dependency.StatusResult) agenthealth.DependencyObservation {
	availability := agenthealth.DependencyAvailabilityUnknown
	switch result.Status.Availability {
	case dependency.AvailabilityEnabled:
		availability = agenthealth.DependencyAvailabilityEnabled
	case dependency.AvailabilityDisabled:
		availability = agenthealth.DependencyAvailabilityDisabled
	}

	health := agenthealth.DependencyHealthUnknown
	switch result.Status.Health {
	case dependency.HealthHealthy:
		health = agenthealth.DependencyHealthHealthy
	case dependency.HealthUnhealthy:
		health = agenthealth.DependencyHealthUnhealthy
	}

	return agenthealth.DependencyObservation{
		Availability: availability, Health: health,
		Drifted: result.Status.Drifted, InspectionFailed: result.InspectionError != nil,
	}
}
