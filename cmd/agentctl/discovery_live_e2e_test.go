package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ar-imms/telemetry-agent/internal/agentstate"
	"github.com/ar-imms/telemetry-agent/internal/discovery"
	discoverydocker "github.com/ar-imms/telemetry-agent/internal/discovery/docker"
	"github.com/ar-imms/telemetry-agent/internal/integration"
)

// TestLiveDockerInventoryWhenExplicitlyEnabled runs only in the documented local E2E.
func TestLiveDockerInventoryWhenExplicitlyEnabled(t *testing.T) {
	if os.Getenv("AR_IMMS_S4_E2E") != "1" {
		t.Skip("set AR_IMMS_S4_E2E=1 to run against an isolated live Docker fixture")
	}
	statePath := os.Getenv("AR_IMMS_S4_E2E_STATE")
	fixtureID := os.Getenv("AR_IMMS_S4_E2E_CONTAINER_ID")
	endpoint := os.Getenv(dockerDiscoveryEndpointEnv)
	expectedPresence := os.Getenv("AR_IMMS_S4_E2E_EXPECT_PRESENCE")
	expectedLifecycle := os.Getenv("AR_IMMS_S4_E2E_EXPECT_LIFECYCLE")
	if expectedPresence == "" {
		expectedPresence = string(discovery.PresencePresent)
	}
	if statePath == "" || fixtureID == "" || endpoint == "" {
		t.Fatal("E2E state path, fixture ID, and explicit Docker endpoint are required")
	}
	if filepath.Clean(statePath) == filepath.Clean(agentstate.DefaultPath()) {
		t.Fatal("E2E must not use the production Agent state path")
	}

	store := agentstate.NewSQLiteStore(statePath)
	if err := store.Save(agentstate.State{DesiredGeneration: 3, ActivatedGeneration: 2, AppliedGeneration: 2, Dependencies: map[string]agentstate.DependencyState{"host": {Enabled: true}}}); err != nil {
		t.Fatal(err)
	}
	initial, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	desiredBefore, err := store.IntegrationRepository().Snapshot()
	if err != nil {
		t.Fatal(err)
	}

	provider, err := discoverydocker.NewProvider(endpoint)
	if err != nil {
		t.Fatal(err)
	}
	repository := store.InventoryRepository()
	coordinator, err := discovery.NewCoordinator(repository, []discovery.DiscoveryProvider{provider}, discovery.DefaultPolicy(), 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	cycles := 1
	if expectedPresence == string(discovery.PresenceMissing) {
		cycles = 2
	}
	var result discovery.CycleResult
	for cycle := 0; cycle < cycles; cycle++ {
		result, err = coordinator.DiscoverOnce(context.Background())
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(result.ProviderResults) != 1 || result.ProviderResults[0].ErrorCode != "" {
		t.Fatalf("Docker discovery result=%+v", result.ProviderResults)
	}

	found := false
	for _, resource := range mustInventory(t, repository).Resources {
		if resource.Key.ProviderResourceID != fixtureID {
			continue
		}
		found = true
		if string(resource.Presence) != expectedPresence {
			t.Fatalf("fixture presence=%s, want %s", resource.Presence, expectedPresence)
		}
		if expectedLifecycle != "" && resource.LifecycleState != expectedLifecycle {
			t.Fatalf("fixture lifecycle=%s, want %s", resource.LifecycleState, expectedLifecycle)
		}
	}
	if !found {
		t.Fatalf("fixture %q not found in inventory; missing resources are retained", fixtureID)
	}

	var list, status, stderr bytes.Buffer
	if code := runInventory(context.Background(), []string{"list", "--state-path", statePath}, &list, &stderr); code != 0 {
		t.Fatalf("inventory list code=%d err=%s", code, stderr.String())
	}
	if !strings.Contains(list.String(), fixtureID) || !strings.Contains(list.String(), expectedPresence) {
		t.Fatalf("CLI output lacks fixture evidence: %s", list.String())
	}
	if code := runInventory(context.Background(), []string{"status", "--state-path", statePath}, &status, &stderr); code != 0 {
		t.Fatalf("inventory status code=%d err=%s", code, stderr.String())
	}
	if !strings.Contains(status.String(), "last error: none") {
		t.Fatalf("provider status=%s", status.String())
	}

	if expectedPresence == string(discovery.PresencePresent) {
		stable := false
		revision := result.InventoryRevision
		for attempt := 0; attempt < 6; attempt++ {
			time.Sleep(300 * time.Millisecond)
			next, err := coordinator.DiscoverOnce(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if next.InventoryRevision == revision {
				stable = true
				break
			}
			revision = next.InventoryRevision
		}
		if !stable {
			t.Fatal("inventory did not reach a stable material revision across repeated Docker snapshots")
		}
	}

	after, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	desiredAfter, err := store.IntegrationRepository().Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if after.DesiredGeneration != initial.DesiredGeneration || after.ActivatedGeneration != initial.ActivatedGeneration || after.AppliedGeneration != initial.AppliedGeneration || desiredAfter.Revision != desiredBefore.Revision {
		t.Fatalf("discovery changed S1-S3 lifecycle state: before=%+v after=%+v desired %d -> %d", initial, after, desiredBefore.Revision, desiredAfter.Revision)
	}
}

func TestLiveDockerUnavailableRetainsInventory(t *testing.T) {
	if os.Getenv("AR_IMMS_S4_E2E") != "1" {
		t.Skip("set AR_IMMS_S4_E2E=1 to run provider failure verification")
	}
	statePath := os.Getenv("AR_IMMS_S4_E2E_STATE")
	endpoint := os.Getenv("AR_IMMS_S4_E2E_UNAVAILABLE_ENDPOINT")
	if statePath == "" || endpoint == "" {
		t.Fatal("E2E state path and unreachable local Docker endpoint are required")
	}
	store := agentstate.NewSQLiteStore(statePath)
	repository := store.InventoryRepository()
	before, err := repository.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	provider, err := discoverydocker.NewProvider(endpoint)
	if err != nil {
		t.Fatal(err)
	}
	coordinator, err := discovery.NewCoordinator(repository, []discovery.DiscoveryProvider{provider}, discovery.DefaultPolicy(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	result, err := coordinator.DiscoverOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(result.ProviderResults) != 1 || result.ProviderResults[0].ErrorCode == "" {
		t.Fatalf("expected safe provider error, got %+v", result.ProviderResults)
	}
	after, err := repository.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if before.Revision != after.Revision || len(before.Resources) != len(after.Resources) {
		t.Fatalf("provider failure changed inventory: before=%+v after=%+v", before, after)
	}
	providerStatusFound := false
	for _, status := range after.Providers {
		if status.ProviderID == provider.ID() && status.ProviderInstanceID == provider.InstanceID() {
			providerStatusFound = status.LastErrorCode != ""
		}
	}
	if !providerStatusFound {
		t.Fatalf("provider failure was not recorded: %+v", after.Providers)
	}
	state, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if state.DesiredGeneration != 3 || state.ActivatedGeneration != 2 || state.AppliedGeneration != 2 {
		t.Fatalf("provider failure changed lifecycle generations: %+v", state)
	}
}

func TestLiveDockerProposalAndExplicitApproval(t *testing.T) {
	if os.Getenv("AR_IMMS_S4_E2E") != "1" {
		t.Skip("set AR_IMMS_S4_E2E=1 to run against an isolated live Docker fixture")
	}
	statePath := os.Getenv("AR_IMMS_S4_E2E_STATE")
	fixtureID := os.Getenv("AR_IMMS_S4_E2E_CONTAINER_ID")
	if statePath == "" || fixtureID == "" || filepath.Clean(statePath) == filepath.Clean(agentstate.DefaultPath()) {
		t.Fatal("isolated E2E state path and disposable fixture ID are required")
	}
	store := agentstate.NewSQLiteStore(statePath)
	before, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	desiredBefore, err := store.IntegrationRepository().Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	refresher, err := newRuntimeProposalRefresher(statePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := refresher.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	var matchOutput, matchStderr bytes.Buffer
	if code := runIntegrationMatch(context.Background(), []string{"--state-path", statePath}, &matchOutput, &matchStderr); code != 0 {
		t.Fatalf("read-only integration match code=%d stderr=%s", code, matchStderr.String())
	}
	if !strings.Contains(matchOutput.String(), `"definition_id": "redis"`) ||
		!strings.Contains(matchOutput.String(), `"match_status": "matched"`) ||
		!strings.Contains(matchOutput.String(), `"candidate": {`) {
		t.Fatalf("live read-only match omitted the Redis candidate: %s", matchOutput.String())
	}
	matchState, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	matchDesired, err := store.IntegrationRepository().Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if matchState.DesiredGeneration != before.DesiredGeneration || matchState.ActivatedGeneration != before.ActivatedGeneration ||
		matchState.AppliedGeneration != before.AppliedGeneration || matchDesired.Revision != desiredBefore.Revision {
		t.Fatalf("read-only match mutated lifecycle or desired state: before=%+v after=%+v desired %d -> %d", before, matchState, desiredBefore.Revision, matchDesired.Revision)
	}
	proposals, err := store.ProposalRepository().ProposalSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	var selected *discovery.Proposal
	for i := range proposals {
		proposal := &proposals[i]
		if proposal.Identity.Provider == "docker" && proposal.Identity.ResourceID == fixtureID && proposal.Identity.IntegrationID == "redis" {
			selected = proposal
			break
		}
	}
	if selected == nil || selected.Status != discovery.ProposalPending || selected.CandidateDigest == "" {
		t.Fatalf("Redis proposal=%+v; all proposals=%+v", selected, proposals)
	}
	var listed, approvalOutput, stderr bytes.Buffer
	if code := runDiscovery(context.Background(), []string{"proposals", "list", "--state-path", statePath}, &listed, &stderr); code != 0 {
		t.Fatalf("proposal list code=%d stderr=%s", code, stderr.String())
	}
	if !strings.Contains(listed.String(), selected.ID) || !strings.Contains(listed.String(), selected.CandidateDigest) {
		t.Fatalf("operator listing omitted proposal evidence: %s", listed.String())
	}
	if code := runDiscovery(context.Background(), []string{
		"proposals", "approve", "--state-path", statePath, "--id", selected.ID,
		"--revision", fmt.Sprint(selected.Revision), "--expected-desired-revision", fmt.Sprint(desiredBefore.Revision),
	}, &approvalOutput, &stderr); code != 0 {
		t.Fatalf("proposal approval code=%d stderr=%s", code, stderr.String())
	}
	desiredAfter, err := store.IntegrationRepository().Snapshot()
	if err != nil || len(desiredAfter.Instances) != len(desiredBefore.Instances)+1 || desiredAfter.Revision != desiredBefore.Revision+1 {
		t.Fatalf("desired state after approval=%+v err=%v", desiredAfter, err)
	}
	var approved *integration.IntegrationInstance
	for i := range desiredAfter.Instances {
		instance := &desiredAfter.Instances[i]
		if instance.Binding != nil && instance.Binding.ResourceID == fixtureID {
			approved = instance
			break
		}
	}
	if approved == nil || approved.Source != integration.SourceDiscovery || approved.Configuration["endpoint"] == nil {
		t.Fatalf("approved desired Integration=%+v", approved)
	}
	after, err := store.Load()
	if err != nil || after.DesiredGeneration != before.DesiredGeneration || after.ActivatedGeneration != before.ActivatedGeneration || after.AppliedGeneration != before.AppliedGeneration {
		t.Fatalf("approval changed Collector lifecycle state: before=%+v after=%+v err=%v", before, after, err)
	}
	if !strings.Contains(approvalOutput.String(), "convergence is pending") {
		t.Fatalf("approval output did not distinguish later convergence: %s", approvalOutput.String())
	}
}

func TestLiveDatabaseFixturesMatchIntoNeedsInputProposals(t *testing.T) {
	if os.Getenv("AR_IMMS_S4_E2E") != "1" {
		t.Skip("set AR_IMMS_S4_E2E=1 to run against isolated live Docker fixtures")
	}
	statePath := os.Getenv("AR_IMMS_S4_E2E_STATE")
	postgresID := os.Getenv("AR_IMMS_S4_E2E_POSTGRES_ID")
	mongoID := os.Getenv("AR_IMMS_S4_E2E_MONGO_ID")
	endpoint := os.Getenv(dockerDiscoveryEndpointEnv)
	if statePath == "" || postgresID == "" || mongoID == "" || endpoint == "" || filepath.Clean(statePath) == filepath.Clean(agentstate.DefaultPath()) {
		t.Fatal("isolated E2E state path, Docker endpoint, and PostgreSQL/MongoDB fixture IDs are required")
	}
	store := agentstate.NewSQLiteStore(statePath)
	provider, err := discoverydocker.NewProvider(endpoint)
	if err != nil {
		t.Fatal(err)
	}
	coordinator, err := discovery.NewCoordinator(store.InventoryRepository(), []discovery.DiscoveryProvider{provider}, discovery.DefaultPolicy(), 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := coordinator.DiscoverOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	inventory, err := store.InventoryRepository().Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	postgresDefinition := fixtureDatabaseDefinition("postgresql-fixture", "docker.io/library/postgres", []integration.RequiredInput{
		{Name: "username", Reason: "database username is required"}, {Name: "authentication_profile", Reason: "an approved authentication profile must be selected"},
	})
	mongoDefinition := fixtureDatabaseDefinition("mongodb-fixture", "docker.io/library/mongo", []integration.RequiredInput{
		{Name: "username", Reason: "database username is required"}, {Name: "authentication_profile", Reason: "an approved authentication profile must be selected"},
	})
	registry, err := integration.NewRegistry(postgresDefinition, mongoDefinition)
	if err != nil {
		t.Fatal(err)
	}
	matcher := integration.NewMatcher(registry, fixtureCapabilityAssessor{})
	report, err := matcher.Match(projectInventory(inventory, time.Now().UTC()))
	if err != nil {
		t.Fatal(err)
	}
	observations := make([]discovery.ProposalObservation, 0, 2)
	fixtureResults := make([]integration.MatchResult, 0, 2)
	for _, result := range report.Results {
		if result.Resource.ResourceID != postgresID && result.Resource.ResourceID != mongoID {
			continue
		}
		fixtureResults = append(fixtureResults, result)
		observation, err := proposalObservationFromMatch(result, report.InventoryRevision)
		if err != nil {
			t.Fatal(err)
		}
		observations = append(observations, observation)
	}
	if len(observations) != 2 {
		t.Fatalf("test-only matcher observations=%+v, want PostgreSQL and MongoDB fixtures", observations)
	}
	manager := discovery.NewProposalManager(store.ProposalRepository())
	if err := manager.Refresh(observations, report.EvaluatedAt); err != nil {
		t.Fatal(err)
	}
	proposals, err := store.ProposalRepository().ProposalSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	for _, fixture := range []struct{ id, definition string }{{postgresID, "postgresql-fixture"}, {mongoID, "mongodb-fixture"}} {
		found := false
		for _, proposal := range proposals {
			if proposal.Identity.ResourceID == fixture.id && proposal.Identity.IntegrationID == fixture.definition {
				found = proposal.Status == discovery.ProposalNeedsInput && len(proposal.RequiredInputs) == 2 && proposal.CandidateDigest == ""
			}
		}
		if !found {
			t.Fatalf("fixture %s did not produce a needs_input proposal; fixture_results=%+v proposals=%+v", fixture.id, fixtureResults, proposals)
		}
	}
}

func fixtureDatabaseDefinition(id, imageRepository string, inputs []integration.RequiredInput) integration.IntegrationDefinition {
	return integration.IntegrationDefinition{
		ID: id, Version: "1.0.0", Validate: func(settings map[string]any) error {
			endpoint, ok := settings["endpoint"].(string)
			if !ok || endpoint == "" {
				return errors.New("fixture endpoint is required")
			}
			return nil
		},
		Contribute: func(integration.IntegrationInstance) (integration.Contribution, error) {
			return integration.NewContribution(nil), nil
		},
		Matching: &integration.MatchingDefinition{
			Selector: &integration.MatchSelector{
				ResourceKinds: []string{"container"}, LifecycleStates: []string{"running"},
				ExactAttributes: map[string]string{"container.image.repository": imageRepository},
			},
			BuildCandidate: func(resource integration.MatchResource) (integration.CandidateBuild, error) {
				for _, endpoint := range resource.Endpoints {
					if endpoint.Scope == "host" && endpoint.Provenance == "published_port" && endpoint.Protocol == "tcp" && endpoint.Port > 0 {
						return integration.CandidateBuild{RequiredInputs: append([]integration.RequiredInput(nil), inputs...)}, nil
					}
				}
				missing := append([]integration.RequiredInput{{Name: "endpoint", Reason: "test fixture has no host-published endpoint"}}, inputs...)
				return integration.CandidateBuild{RequiredInputs: missing}, nil
			},
		},
	}
}

type fixtureCapabilityAssessor struct{}

func (fixtureCapabilityAssessor) Assess(required []string) (integration.CapabilityAssessment, error) {
	return integration.CapabilityAssessment{Status: integration.CapabilitySupported, Platform: "test-fixture", ArtifactVersion: "test-only", ManifestVersion: "test-only", Required: append([]string(nil), required...)}, nil
}

func mustInventory(t *testing.T, repository discovery.InventoryRepository) discovery.InventorySnapshot {
	t.Helper()
	snapshot, err := repository.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}
