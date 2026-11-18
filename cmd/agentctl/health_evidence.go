package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/ar-imms/telemetry-agent/internal/app"
	agenthealth "github.com/ar-imms/telemetry-agent/internal/health"
	identityhost "github.com/ar-imms/telemetry-agent/internal/identity/host"
	installationid "github.com/ar-imms/telemetry-agent/internal/identity/installation/domain"
	identitysqlite "github.com/ar-imms/telemetry-agent/internal/identity/sqlite"
	inventoryapp "github.com/ar-imms/telemetry-agent/internal/inventory/observation/application"
	inventorydomain "github.com/ar-imms/telemetry-agent/internal/inventory/observation/domain"
	"github.com/ar-imms/telemetry-agent/internal/runtime/dependencies"
)

const dependencyEvidenceTimeout = 5 * time.Second

// newAgentEnvironmentEvidence reads the persisted installation ID and local
// host evidence without creating identity or asserting Core topology.
func newAgentEnvironmentEvidence(statePath string) *identityhost.EnvironmentEvidence {
	installationID := installationid.InstallationID{}
	var identityErr error
	if _, err := os.Stat(statePath); err != nil {
		identityErr = err
	} else {
		db, err := app.NewAgentSQLiteStore(statePath).OpenDatabase()
		if err != nil {
			identityErr = err
		} else {
			var initialized bool
			installationID, initialized, identityErr = identitysqlite.ReadInstallationID(db)
			closeErr := db.Close()
			if identityErr == nil && closeErr != nil {
				identityErr = closeErr
			}
			if identityErr == nil && !initialized {
				identityErr = fmt.Errorf("Agent installation identity is not initialized")
			}
		}
	}

	evidence := identityhost.CollectEnvironmentEvidence(installationID)
	if identityErr != nil {
		evidence.AgentID = ""
		evidence.Evidence = append(evidence.Evidence, identityhost.EnvironmentFact{
			Source: "agent-installation-id", Outcome: identityhost.EvidenceOutcomeUnavailable,
			Confidence: identityhost.EvidenceConfidenceUnknown, ObservedAt: evidence.ObservedAt,
			Reason: "persisted installation identity is unavailable",
		})
		return &evidence
	}
	evidence.Evidence = append(evidence.Evidence, identityhost.EnvironmentFact{
		Source: "agent-installation-id", Outcome: identityhost.EvidenceOutcomeObserved,
		Confidence: identityhost.EvidenceConfidenceVerified, ObservedAt: evidence.ObservedAt,
	})
	return &evidence
}

func newAgentHealthEvidenceSources(statePath string) []agenthealth.EvidenceSource {
	var inventory inventoryapp.ProviderStatusReader
	inventory, _ = app.NewInventoryRepository(statePath).(inventoryapp.ProviderStatusReader)

	return []agenthealth.EvidenceSource{
		func(ctx context.Context, now time.Time) agenthealth.ComponentEvidence {
			if inventory == nil {
				return agenthealth.EvaluateInventoryEvidence(now, nil, false, false)
			}
			statuses, truncated, err := inventory.ReadProviderStatuses(ctx, inventoryapp.MaxProviderStatusReadLimit)
			observations := make([]agenthealth.InventoryProviderObservation, 0, len(statuses))
			staleAfter := inventorydomain.DefaultPolicy().StaleAfter
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
