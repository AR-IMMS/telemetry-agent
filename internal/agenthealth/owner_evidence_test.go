package agenthealth

import (
	"testing"
	"time"
)

func TestEvaluateInventoryEvidenceDoesNotClaimStaleSnapshotHealthy(t *testing.T) {
	now := time.Date(2026, time.November, 14, 6, 0, 0, 0, time.UTC)
	fresh := EvaluateInventoryEvidence(now, []InventoryProviderObservation{{
		Enabled: true, LastAttemptAt: now.Add(-30 * time.Second),
		LastSuccessValidUntil: now.Add(90 * time.Second),
	}}, false, true)
	if fresh.State != EvidenceHealthy || fresh.Freshness != FreshnessFresh || fresh.Completeness != CompletenessComplete {
		t.Fatalf("fresh inventory evidence=%+v, want complete healthy result", fresh)
	}

	evidence := EvaluateInventoryEvidence(now, []InventoryProviderObservation{{
		Enabled: true, LastAttemptAt: now.Add(-3 * time.Minute),
		LastSuccessValidUntil: now.Add(-time.Minute),
	}}, false, true)
	if evidence.State != EvidenceDegraded || evidence.Freshness != FreshnessStale || evidence.ReasonCode != "inventory_snapshot_stale" {
		t.Fatalf("inventory evidence=%+v, want degraded stale result", evidence)
	}
}

func TestEvaluateInventoryEvidencePreservesFailureAndTruncationSemantics(t *testing.T) {
	now := time.Date(2026, time.November, 14, 6, 0, 0, 0, time.UTC)
	failed := EvaluateInventoryEvidence(now, []InventoryProviderObservation{{
		Enabled: true, LastAttemptAt: now, LastSuccessValidUntil: now.Add(time.Minute), FailureObserved: true,
	}}, false, true)
	if failed.State != EvidenceDegraded || failed.Completeness != CompletenessPartial || failed.ReasonCode != "provider_observation_failed" {
		t.Fatalf("failed inventory evidence=%+v, want partial provider failure", failed)
	}
	truncated := EvaluateInventoryEvidence(now, nil, true, true)
	if truncated.State != EvidenceUnknown || truncated.Completeness != CompletenessPartial || truncated.ReasonCode != "inventory_provider_status_truncated" {
		t.Fatalf("truncated inventory evidence=%+v, want unknown partial result", truncated)
	}
	unavailable := EvaluateInventoryEvidence(now, nil, false, false)
	if unavailable.State != EvidenceDegraded || unavailable.ReasonCode != "inventory_snapshot_unavailable" {
		t.Fatalf("unavailable inventory evidence=%+v, want degraded unavailable result", unavailable)
	}
}

func TestEvaluateDependencyEvidenceKeepsUnavailableDistinctFromHealthy(t *testing.T) {
	now := time.Date(2026, time.November, 14, 6, 0, 0, 0, time.UTC)
	evidence := EvaluateDependencyEvidence(now, nil, false)
	if evidence.State != EvidenceUnknown || evidence.ReasonCode != "dependency_status_unavailable" {
		t.Fatalf("dependency evidence=%+v, want bounded unknown unavailable result", evidence)
	}
}

func TestEvaluateDependencyEvidenceUsesOnlyBoundedStatusFields(t *testing.T) {
	now := time.Date(2026, time.November, 14, 6, 0, 0, 0, time.UTC)
	healthy := EvaluateDependencyEvidence(now, []DependencyObservation{
		{Availability: DependencyAvailabilityEnabled, Health: DependencyHealthHealthy},
		{Availability: DependencyAvailabilityDisabled, Health: DependencyHealthUnknown},
	}, true)
	if healthy.State != EvidenceHealthy || healthy.Freshness != FreshnessFresh || healthy.Completeness != CompletenessComplete {
		t.Fatalf("dependency evidence=%+v, want fresh healthy enabled dependency", healthy)
	}

	failed := EvaluateDependencyEvidence(now, []DependencyObservation{{
		Availability: DependencyAvailabilityEnabled, Health: DependencyHealthUnhealthy,
	}}, true)
	if failed.State != EvidenceDegraded || failed.ReasonCode != "dependency_failure_observed" {
		t.Fatalf("dependency evidence=%+v, want degraded unhealthy dependency", failed)
	}

	unsupported := EvaluateDependencyEvidence(now, nil, true)
	if unsupported.State != EvidenceUnsupported || unsupported.ReasonCode != "no_supported_dependencies" {
		t.Fatalf("dependency evidence=%+v, want unsupported empty catalog", unsupported)
	}

	unknown := EvaluateDependencyEvidence(now, []DependencyObservation{{
		Availability: DependencyAvailabilityEnabled, Health: DependencyHealthUnknown, InspectionFailed: true,
	}}, true)
	if unknown.State != EvidenceDegraded || unknown.Completeness != CompletenessPartial {
		t.Fatalf("dependency evidence=%+v, want partial degraded inspection failure", unknown)
	}
}
