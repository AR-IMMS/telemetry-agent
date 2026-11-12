package sqlite_test

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/ar-imms/telemetry-agent/internal/agentstate"
	"github.com/ar-imms/telemetry-agent/internal/configuration"
	configurationsqlite "github.com/ar-imms/telemetry-agent/internal/configuration/sqlite"
	"github.com/ar-imms/telemetry-agent/internal/discovery"
	discoverysqlite "github.com/ar-imms/telemetry-agent/internal/discovery/sqlite"
	"github.com/ar-imms/telemetry-agent/internal/integration"
	"github.com/ar-imms/telemetry-agent/internal/platform/events"
)

type desiredEventPublisherFunc func(events.DesiredConfigurationChanged) error

func (f desiredEventPublisherFunc) Publish(event events.DesiredConfigurationChanged) error {
	return f(event)
}

func TestRepositoriesUseSharedStateDatabaseAndRetainCASContracts(t *testing.T) {
	store := agentstate.NewSQLiteStore(filepath.Join(t.TempDir(), "state.db"))
	locker := &sync.Mutex{}

	desired := configurationsqlite.NewIntegrationRepository(store.OpenDatabase, locker)
	created, revision, err := desired.Put(integrationInstance("cache", "localhost:6379"), 0)
	if err != nil || created.Revision != 1 || revision != 1 {
		t.Fatalf("create desired instance=%+v set revision=%d err=%v", created, revision, err)
	}
	unchanged, revision, err := desired.Put(integrationInstance("cache", "localhost:6379"), 1)
	if err != nil || unchanged.Revision != 1 || revision != 1 {
		t.Fatalf("no-op desired instance=%+v set revision=%d err=%v", unchanged, revision, err)
	}
	if _, _, err := desired.Put(integrationInstance("cache", "127.0.0.1:6379"), 0); !errors.Is(err, configuration.ErrRevisionConflict) {
		t.Fatalf("stale desired CAS error=%v", err)
	}

	templates := configurationsqlite.NewTemplateRepository(store.OpenDatabase, locker)
	template, revision, err := templates.Put(localTemplate("redis-main", integration.TemplateEnabled), 0)
	if err != nil || template.Revision != 1 || revision != 1 {
		t.Fatalf("create Template=%+v set revision=%d err=%v", template, revision, err)
	}
	unchangedTemplate, revision, err := templates.Put(localTemplate("redis-main", integration.TemplateEnabled), 1)
	if err != nil || unchangedTemplate.Revision != 1 || revision != 1 {
		t.Fatalf("no-op Template=%+v set revision=%d err=%v", unchangedTemplate, revision, err)
	}
	if _, _, err := templates.Put(localTemplate("redis-main", integration.TemplateDisabled), 0); !errors.Is(err, configuration.ErrRevisionConflict) {
		t.Fatalf("stale Template CAS error=%v", err)
	}

	if _, err := configurationsqlite.NewMaterializationRepository(store.OpenDatabase, locker).ApplyTemplateProjection(configuration.MaterializationBatch{}); !errors.Is(err, configuration.ErrInvalidMaterialization) {
		t.Fatalf("invalid materialization error=%v", err)
	}
	if _, _, err := configurationsqlite.NewApprovalRepository(store.OpenDatabase, locker).CommitApproval(configuration.ApprovalCommit{}); !errors.Is(err, discovery.ErrInvalidProposal) {
		t.Fatalf("invalid approval error=%v", err)
	}
}

func TestMaterializationRepositoryCommitsProjectionBeforePublishingWakeup(t *testing.T) {
	store := agentstate.NewSQLiteStore(filepath.Join(t.TempDir(), "state.db"))
	locker := &sync.Mutex{}
	inventory := store.InventoryRepository()
	at := time.Date(2026, 2, 3, 4, 5, 6, 0, time.UTC)
	providerSnapshot := discovery.ProviderSnapshot{
		ProviderID: "docker", ProviderInstanceID: "local", Complete: true, ObservedAt: at,
		Resources: []discovery.ResourceObservation{{ProviderResourceID: "container-a", Kind: discovery.ResourceContainer}},
	}
	resource := discovery.Resource{
		Key:  discovery.ResourceKey{Provider: "docker", ProviderInstance: "local", ProviderResourceID: "container-a"},
		Kind: discovery.ResourceContainer, Presence: discovery.PresencePresent, FirstSeenAt: at, LastSeenAt: at,
	}
	if err := inventory.CommitProviderSnapshot(0, providerSnapshot, []discovery.Resource{resource}, true); err != nil {
		t.Fatalf("commit inventory: %v", err)
	}
	template, _, err := configurationsqlite.NewTemplateRepository(store.OpenDatabase, locker).Put(localTemplate("redis-main", integration.TemplateEnabled), 0)
	if err != nil {
		t.Fatalf("persist Template: %v", err)
	}
	identity := integration.ResourceIdentity{Provider: "docker", ProviderInstance: "local", ResourceID: "container-a"}
	instanceID, err := integration.GeneratedInstanceID(integration.TemplateOwnerLocal, template.ID, identity, template.DefinitionID)
	if err != nil {
		t.Fatalf("derive generated instance ID: %v", err)
	}
	instance := integration.IntegrationInstance{
		ID: instanceID, DefinitionID: template.DefinitionID, DefinitionVersion: template.DefinitionVersion,
		Configuration: map[string]any{"endpoint": "127.0.0.1:6379"}, Enabled: true, Source: integration.SourceTemplate,
		TemplateOwner: template.Owner, TemplateID: template.ID, TemplateRevision: template.Revision,
		Binding: &integration.ResourceBinding{Provider: identity.Provider, ProviderInstance: identity.ProviderInstance, ResourceID: identity.ResourceID},
	}
	desired := configurationsqlite.NewIntegrationRepository(store.OpenDatabase, locker)
	var published uint64
	repository := configurationsqlite.NewMaterializationRepositoryWithEvents(store.OpenDatabase, locker, desiredEventPublisherFunc(func(event events.DesiredConfigurationChanged) error {
		snapshot, err := desired.Snapshot()
		if err != nil {
			return err
		}
		if snapshot.Revision != event.Revision || len(snapshot.Instances) != 1 {
			t.Fatalf("event revision=%d observed before desired snapshot=%+v", event.Revision, snapshot)
		}
		published = event.Revision
		return nil
	}))
	batch := configuration.MaterializationBatch{
		ExpectedInventoryWriteRevision: 1, ExpectedTemplateSetRevision: 1,
		ExpectedDesiredSetRevision: 0, Upsert: []integration.IntegrationInstance{instance},
	}
	revision, err := repository.ApplyTemplateProjection(batch)
	if err != nil || revision != 1 || published != 1 {
		t.Fatalf("materialization revision=%d published=%d err=%v", revision, published, err)
	}
}

func TestApprovalRepositoryCommitsProposalAndDesiredStateTogether(t *testing.T) {
	store := agentstate.NewSQLiteStore(filepath.Join(t.TempDir(), "state.db"))
	locker := &sync.Mutex{}
	at := time.Date(2026, 2, 3, 4, 5, 6, 0, time.UTC)
	providerSnapshot := discovery.ProviderSnapshot{
		ProviderID: "docker", ProviderInstanceID: "local", Complete: true, ObservedAt: at,
		Resources: []discovery.ResourceObservation{{ProviderResourceID: "container-a", Kind: discovery.ResourceContainer}},
	}
	resource := discovery.Resource{
		Key:  discovery.ResourceKey{Provider: "docker", ProviderInstance: "local", ProviderResourceID: "container-a"},
		Kind: discovery.ResourceContainer, Presence: discovery.PresencePresent, FirstSeenAt: at, LastSeenAt: at,
	}
	if err := store.InventoryRepository().CommitProviderSnapshot(0, providerSnapshot, []discovery.Resource{resource}, true); err != nil {
		t.Fatalf("commit inventory: %v", err)
	}
	proposal := pendingProposal(t, at)
	proposals := discoverysqlite.NewProposalRepository(store.OpenDatabase, locker)
	if err := proposals.PutProposal(proposal, 0); err != nil {
		t.Fatalf("persist proposal: %v", err)
	}
	instance := integration.IntegrationInstance{
		ID: "approved-redis", DefinitionID: proposal.Identity.IntegrationID, DefinitionVersion: proposal.DefinitionVersion,
		Configuration: map[string]any{"endpoint": "127.0.0.1:6379"}, Enabled: true, Source: integration.SourceDiscovery,
		Binding: &integration.ResourceBinding{Provider: proposal.Identity.Provider, ProviderInstance: proposal.Identity.ProviderInstance, ResourceID: proposal.Identity.ResourceID},
	}
	commit := configuration.ApprovalCommit{
		ProposalID: proposal.ID, ProposalRevision: proposal.Revision, ExpectedDesiredRevision: 0,
		ExpectedCandidateDigest: proposal.CandidateDigest, VerifiedCandidateDigest: proposal.CandidateDigest,
		InventoryWriteRevision: 1, Instance: instance,
	}
	approved, revision, err := configurationsqlite.NewApprovalRepository(store.OpenDatabase, locker).CommitApproval(commit)
	if err != nil || revision != 1 || approved.Revision != 1 {
		t.Fatalf("approved instance=%+v revision=%d err=%v", approved, revision, err)
	}
	currentProposals, err := proposals.ProposalSnapshot()
	if err != nil || len(currentProposals) != 1 || currentProposals[0].Status != discovery.ProposalApproved || currentProposals[0].ApprovedInstanceID != instance.ID {
		t.Fatalf("approved proposals=%+v err=%v", currentProposals, err)
	}
	desired, err := configurationsqlite.NewIntegrationRepository(store.OpenDatabase, locker).Snapshot()
	if err != nil || desired.Revision != 1 || len(desired.Instances) != 1 || desired.Instances[0].ID != instance.ID {
		t.Fatalf("desired state after approval=%+v err=%v", desired, err)
	}
}

func integrationInstance(id, endpoint string) integration.IntegrationInstance {
	return integration.IntegrationInstance{
		ID: id, DefinitionID: "redis", DefinitionVersion: "1.0.0", Enabled: true,
		Configuration: map[string]any{"endpoint": endpoint},
	}
}

func localTemplate(id string, status integration.TemplateStatus) integration.IntegrationTemplate {
	return integration.IntegrationTemplate{
		ID: id, DefinitionID: "redis", DefinitionVersion: "1.0.0", Owner: integration.TemplateOwnerLocal,
		Revision: 1, Status: status, Settings: map[string]any{},
		Bindings: []integration.TemplateSettingBinding{{Target: "endpoint", Source: integration.TemplateBindingSource{
			Kind: integration.TemplateSourceEndpoint, EndpointScope: "host", EndpointProtocol: "tcp", EndpointProvenance: "published_port",
		}}},
	}
}

func pendingProposal(t *testing.T, at time.Time) discovery.Proposal {
	t.Helper()
	identity := discovery.ProposalIdentity{Provider: "docker", ProviderInstance: "local", ResourceID: "container-a", IntegrationID: "redis"}
	id, err := discovery.StableProposalID(identity)
	if err != nil {
		t.Fatal(err)
	}
	proposal := discovery.Proposal{
		ID: id, Identity: identity, Revision: 1, Status: discovery.ProposalPending, DefinitionVersion: "1.0.0",
		ResourceFingerprint: "sha256:" + repeatedHex('a'), CandidateDigest: "sha256:" + repeatedHex('b'),
		CandidateConfiguration: json.RawMessage(`{"endpoint":"127.0.0.1:6379"}`), CapabilityEvidence: json.RawMessage(`{"status":"supported"}`),
		InventoryRevision: 1, CreatedAt: at, UpdatedAt: at,
	}
	if err := discovery.ValidateProposal(proposal); err != nil {
		t.Fatal(err)
	}
	return proposal
}

func repeatedHex(value byte) string {
	bytes := make([]byte, 64)
	for i := range bytes {
		bytes[i] = value
	}
	return string(bytes)
}
