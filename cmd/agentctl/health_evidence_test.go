package main

import (
	"errors"
	"testing"
	"time"

	"github.com/ar-imms/telemetry-agent/internal/agenthealth"
	"github.com/ar-imms/telemetry-agent/internal/dependency"
	"github.com/ar-imms/telemetry-agent/internal/discovery"
)

func TestEvaluateInventoryEvidenceUsesProviderStatusAndOwnerStaleness(t *testing.T) {
	now := time.Date(2026, time.November, 14, 6, 0, 0, 0, time.UTC)
	lastSuccess := now.Add(-30 * time.Second)
	providers := []discovery.ProviderStatus{{
		ProviderID: "docker", ProviderInstanceID: "local", Enabled: true,
		LastAttemptAt: lastSuccess, LastSuccessAt: lastSuccess, ResourceCount: 2,
	}}
	evidence := evaluateInventoryEvidence(now, providers, false, nil)
	if evidence.State != agenthealth.EvidenceHealthy || evidence.Freshness != agenthealth.FreshnessFresh || evidence.Completeness != agenthealth.CompletenessComplete {
		t.Fatalf("inventory evidence=%+v, want recent complete provider snapshot", evidence)
	}

	providers[0].LastSuccessAt = now.Add(-3 * time.Minute)
	providers[0].LastAttemptAt = now.Add(-3 * time.Minute)
	evidence = evaluateInventoryEvidence(now, providers, false, nil)
	if evidence.Freshness != agenthealth.FreshnessStale || evidence.State != agenthealth.EvidenceHealthy {
		t.Fatalf("stale inventory evidence=%+v, want healthy last result with stale freshness", evidence)
	}
}

func TestEvaluateInventoryEvidenceReportsProviderFailureWithoutErasingLastSuccess(t *testing.T) {
	now := time.Date(2026, time.November, 14, 6, 0, 0, 0, time.UTC)
	providers := []discovery.ProviderStatus{{
		ProviderID: "docker", ProviderInstanceID: "local", Enabled: true,
		LastAttemptAt: now, LastSuccessAt: now.Add(-time.Minute), LastErrorCode: "daemon_unavailable", ResourceCount: 2,
	}}
	evidence := evaluateInventoryEvidence(now, providers, false, nil)
	if evidence.State != agenthealth.EvidenceDegraded || evidence.Completeness != agenthealth.CompletenessPartial || evidence.ReasonCode != "provider_observation_failed" {
		t.Fatalf("inventory evidence=%+v, want partial failure while preserving prior snapshot", evidence)
	}
}

func TestEvaluateInventoryEvidenceDoesNotClaimCompletenessWhenProviderStatusesAreTruncated(t *testing.T) {
	now := time.Date(2026, time.November, 14, 6, 0, 0, 0, time.UTC)
	evidence := evaluateInventoryEvidence(now, nil, true, nil)
	if evidence.State != agenthealth.EvidenceUnknown || evidence.Completeness != agenthealth.CompletenessPartial || evidence.ReasonCode != "inventory_provider_status_truncated" {
		t.Fatalf("inventory evidence=%+v, want unknown partial evidence after bounded read truncation", evidence)
	}
}

func TestEvaluateDependencyEvidenceUsesReadOnlyStatuses(t *testing.T) {
	now := time.Date(2026, time.November, 14, 6, 0, 0, 0, time.UTC)
	results := []dependency.StatusResult{
		{Status: dependency.Status{Availability: dependency.AvailabilityEnabled, Health: dependency.HealthHealthy}},
		{Status: dependency.Status{Availability: dependency.AvailabilityDisabled, Health: dependency.HealthUnknown}},
	}
	evidence := evaluateDependencyEvidence(now, results, nil)
	if evidence.State != agenthealth.EvidenceHealthy || evidence.Freshness != agenthealth.FreshnessFresh || evidence.Completeness != agenthealth.CompletenessComplete {
		t.Fatalf("dependency evidence=%+v, want fresh healthy enabled dependencies", evidence)
	}

	results[0].Status.Health = dependency.HealthUnhealthy
	evidence = evaluateDependencyEvidence(now, results, nil)
	if evidence.State != agenthealth.EvidenceDegraded || evidence.ReasonCode != "dependency_failure_observed" {
		t.Fatalf("dependency evidence=%+v, want degraded unhealthy dependency", evidence)
	}
}

func TestEvaluateDependencyEvidenceKeepsMissingAndUnavailableEvidenceDistinct(t *testing.T) {
	now := time.Date(2026, time.November, 14, 6, 0, 0, 0, time.UTC)
	evidence := evaluateDependencyEvidence(now, nil, nil)
	if evidence.State != agenthealth.EvidenceUnsupported || evidence.ReasonCode != "no_supported_dependencies" {
		t.Fatalf("empty dependency evidence=%+v, want unsupported empty catalog", evidence)
	}
	evidence = evaluateDependencyEvidence(now, nil, errors.New("secret-bearing source detail"))
	if evidence.State != agenthealth.EvidenceUnknown || evidence.ReasonCode != "dependency_status_unavailable" {
		t.Fatalf("unavailable dependency evidence=%+v, want bounded unknown reason", evidence)
	}
}
