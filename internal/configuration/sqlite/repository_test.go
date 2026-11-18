package sqlite_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	desiredstateapp "github.com/ar-imms/telemetry-agent/internal/configuration/desiredstate/application"
	desiredstate "github.com/ar-imms/telemetry-agent/internal/configuration/desiredstate/domain"
	materializationdomain "github.com/ar-imms/telemetry-agent/internal/configuration/materialization/domain"
	proposals "github.com/ar-imms/telemetry-agent/internal/configuration/proposals/application"
	configurationsqlite "github.com/ar-imms/telemetry-agent/internal/configuration/sqlite"
	templatesapp "github.com/ar-imms/telemetry-agent/internal/configuration/templates/application"
	templatesdomain "github.com/ar-imms/telemetry-agent/internal/configuration/templates/domain"
	inventorydomain "github.com/ar-imms/telemetry-agent/internal/inventory/observation/domain"
	"github.com/ar-imms/telemetry-agent/internal/platform/events"
	"github.com/ar-imms/telemetry-agent/internal/testsupport/sqlitefixture"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestTemplateSnapshotContextHonorsCancellationBeforeOpeningDatabase(t *testing.T) {
	opened := false
	repository := configurationsqlite.NewTemplateRepository(func() (*sql.DB, error) {
		opened = true
		return nil, errors.New("database should not be opened")
	}, &sync.Mutex{})
	reader, ok := repository.(interface {
		SnapshotContext(context.Context) (templatesapp.Snapshot, error)
	})
	if !ok {
		t.Fatal("Template SQLite repository does not expose cancellable snapshots")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := reader.SnapshotContext(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("SnapshotContext() error = %v, want context.Canceled", err)
	}
	if opened {
		t.Fatal("canceled Template snapshot opened its database")
	}
}

type desiredEventPublisherFunc func(events.DesiredConfigurationChanged) error

func (f desiredEventPublisherFunc) Publish(event events.DesiredConfigurationChanged) error {
	return f(event)
}

func TestRepositoriesUseSharedStateDatabaseAndRetainCASContracts(t *testing.T) {
	store := sqlitefixture.NewSQLiteStore(filepath.Join(t.TempDir(), "state.db"))
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
	if _, _, err := desired.Put(integrationInstance("cache", "127.0.0.1:6379"), 0); !errors.Is(err, desiredstateapp.ErrRevisionConflict) {
		t.Fatalf("stale desired CAS error=%v", err)
	}

	templates := configurationsqlite.NewTemplateRepository(store.OpenDatabase, locker)
	template, revision, err := templates.Put(localTemplate("redis-main", templatesdomain.StatusEnabled), 0)
	if err != nil || template.Revision != 1 || revision != 1 {
		t.Fatalf("create Template=%+v set revision=%d err=%v", template, revision, err)
	}
	unchangedTemplate, revision, err := templates.Put(localTemplate("redis-main", templatesdomain.StatusEnabled), 1)
	if err != nil || unchangedTemplate.Revision != 1 || revision != 1 {
		t.Fatalf("no-op Template=%+v set revision=%d err=%v", unchangedTemplate, revision, err)
	}
	if _, _, err := templates.Put(localTemplate("redis-main", templatesdomain.StatusDisabled), 0); !errors.Is(err, templatesapp.ErrRevisionConflict) {
		t.Fatalf("stale Template CAS error=%v", err)
	}

	if _, err := configurationsqlite.NewMaterializationRepository(store.OpenDatabase, locker).ApplyTemplateProjection(materializationdomain.Batch{}); !errors.Is(err, materializationdomain.ErrInvalidMaterialization) {
		t.Fatalf("invalid materialization error=%v", err)
	}
	if _, _, err := configurationsqlite.NewApprovalRepository(store.OpenDatabase, locker).CommitApproval(proposals.ApprovalCommit{}); !errors.Is(err, proposals.ErrInvalidProposal) {
		t.Fatalf("invalid approval error=%v", err)
	}
}

func TestMaterializationRepositoryCommitsProjectionBeforePublishingWakeup(t *testing.T) {
	store := sqlitefixture.NewSQLiteStore(filepath.Join(t.TempDir(), "state.db"))
	locker := &sync.Mutex{}
	inventory := sqlitefixture.NewInventoryRepositoryFromStore(store)
	at := time.Date(2026, 2, 3, 4, 5, 6, 0, time.UTC)
	providerSnapshot := inventorydomain.ProviderSnapshot{
		ProviderID: "docker", ProviderInstanceID: "local", Complete: true, ObservedAt: at,
		Resources: []inventorydomain.ResourceObservation{{ProviderResourceID: "container-a", Kind: inventorydomain.ResourceContainer}},
	}
	resource := inventorydomain.Resource{
		Key:  inventorydomain.ResourceKey{Provider: "docker", ProviderInstance: "local", ProviderResourceID: "container-a"},
		Kind: inventorydomain.ResourceContainer, Presence: inventorydomain.PresencePresent, FirstSeenAt: at, LastSeenAt: at,
	}
	if err := inventory.CommitProviderSnapshot(0, providerSnapshot, []inventorydomain.Resource{resource}, true); err != nil {
		t.Fatalf("commit inventory: %v", err)
	}
	template, _, err := configurationsqlite.NewTemplateRepository(store.OpenDatabase, locker).Put(localTemplate("redis-main", templatesdomain.StatusEnabled), 0)
	if err != nil {
		t.Fatalf("persist Template: %v", err)
	}
	identity := desiredstate.ResourceBinding{Provider: "docker", ProviderInstance: "local", ResourceID: "container-a"}
	instanceID, err := materializationdomain.GeneratedInstanceID(desiredstate.TemplateOwnerLocal, template.ID, identity, template.DefinitionID)
	if err != nil {
		t.Fatalf("derive generated instance ID: %v", err)
	}
	instance := desiredstate.Instance{
		ID: instanceID, DefinitionID: template.DefinitionID, DefinitionVersion: template.DefinitionVersion,
		Configuration: map[string]any{"endpoint": "127.0.0.1:6379"}, Enabled: true, Source: desiredstate.SourceTemplate,
		TemplateOwner: template.Owner, TemplateID: template.ID, TemplateRevision: template.Revision,
		Binding: &desiredstate.ResourceBinding{Provider: identity.Provider, ProviderInstance: identity.ProviderInstance, ResourceID: identity.ResourceID},
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
	batch := materializationdomain.Batch{
		ExpectedInventoryWriteRevision: 1, ExpectedTemplateSetRevision: 1,
		ExpectedDesiredSetRevision: 0, Upsert: []desiredstate.Instance{instance},
	}
	revision, err := repository.ApplyTemplateProjection(batch)
	if err != nil || revision != 1 || published != 1 {
		t.Fatalf("materialization revision=%d published=%d err=%v", revision, published, err)
	}
}

func TestApprovalRepositoryCommitsProposalAndDesiredStateTogether(t *testing.T) {
	store := sqlitefixture.NewSQLiteStore(filepath.Join(t.TempDir(), "state.db"))
	locker := &sync.Mutex{}
	at := time.Date(2026, 2, 3, 4, 5, 6, 0, time.UTC)
	providerSnapshot := inventorydomain.ProviderSnapshot{
		ProviderID: "docker", ProviderInstanceID: "local", Complete: true, ObservedAt: at,
		Resources: []inventorydomain.ResourceObservation{{ProviderResourceID: "container-a", Kind: inventorydomain.ResourceContainer}},
	}
	resource := inventorydomain.Resource{
		Key:  inventorydomain.ResourceKey{Provider: "docker", ProviderInstance: "local", ProviderResourceID: "container-a"},
		Kind: inventorydomain.ResourceContainer, Presence: inventorydomain.PresencePresent, FirstSeenAt: at, LastSeenAt: at,
	}
	if err := sqlitefixture.NewInventoryRepositoryFromStore(store).CommitProviderSnapshot(0, providerSnapshot, []inventorydomain.Resource{resource}, true); err != nil {
		t.Fatalf("commit inventory: %v", err)
	}
	proposal := pendingProposal(t, at)
	proposalRepo := configurationsqlite.NewProposalRepository(store.OpenDatabase, locker)
	if err := proposalRepo.PutProposal(proposal, 0); err != nil {
		t.Fatalf("persist proposal: %v", err)
	}
	instance := desiredstate.Instance{
		ID: "approved-redis", DefinitionID: proposal.Identity.IntegrationID, DefinitionVersion: proposal.DefinitionVersion,
		Configuration: map[string]any{"endpoint": "127.0.0.1:6379"}, Enabled: true, Source: desiredstate.SourceDiscovery,
		Binding: &desiredstate.ResourceBinding{Provider: proposal.Identity.Provider, ProviderInstance: proposal.Identity.ProviderInstance, ResourceID: proposal.Identity.ResourceID},
	}
	commit := proposals.ApprovalCommit{
		ProposalID: proposal.ID, ProposalRevision: proposal.Revision, ExpectedDesiredRevision: 0,
		ExpectedCandidateDigest: proposal.CandidateDigest, VerifiedCandidateDigest: proposal.CandidateDigest,
		InventoryWriteRevision: 1, Instance: instance,
	}
	approved, revision, err := configurationsqlite.NewApprovalRepository(store.OpenDatabase, locker).CommitApproval(commit)
	if err != nil || revision != 1 || approved.Revision != 1 {
		t.Fatalf("approved instance=%+v revision=%d err=%v", approved, revision, err)
	}
	currentProposals, err := proposalRepo.ProposalSnapshot()
	if err != nil || len(currentProposals) != 1 || currentProposals[0].Status != proposals.ProposalApproved || currentProposals[0].ApprovedInstanceID != instance.ID {
		t.Fatalf("approved proposals=%+v err=%v", currentProposals, err)
	}
	desired, err := configurationsqlite.NewIntegrationRepository(store.OpenDatabase, locker).Snapshot()
	if err != nil || desired.Revision != 1 || len(desired.Instances) != 1 || desired.Instances[0].ID != instance.ID {
		t.Fatalf("desired state after approval=%+v err=%v", desired, err)
	}
}

func integrationInstance(id, endpoint string) desiredstate.Instance {
	return desiredstate.Instance{
		ID: id, DefinitionID: "redis", DefinitionVersion: "1.0.0", Enabled: true,
		Configuration: map[string]any{"endpoint": endpoint},
	}
}

func localTemplate(id string, status templatesdomain.Status) templatesdomain.Template {
	return templatesdomain.Template{
		ID: id, DefinitionID: "redis", DefinitionVersion: "1.0.0", Owner: desiredstate.TemplateOwnerLocal,
		Revision: 1, Status: status, Settings: map[string]any{},
		Bindings: []templatesdomain.SettingBinding{{Target: "endpoint", Source: templatesdomain.BindingSource{
			Kind: templatesdomain.SourceEndpoint, EndpointScope: "host", EndpointProtocol: "tcp", EndpointProvenance: "published_port",
		}}},
	}
}

func pendingProposal(t *testing.T, at time.Time) proposals.Proposal {
	t.Helper()
	identity := proposals.ProposalIdentity{Provider: "docker", ProviderInstance: "local", ResourceID: "container-a", IntegrationID: "redis"}
	id, err := proposals.StableProposalID(identity)
	if err != nil {
		t.Fatal(err)
	}
	proposal := proposals.Proposal{
		ID: id, Identity: identity, Revision: 1, Status: proposals.ProposalPending, DefinitionVersion: "1.0.0",
		ResourceFingerprint: "sha256:" + repeatedHex('a'), CandidateDigest: "sha256:" + repeatedHex('b'),
		CandidateConfiguration: json.RawMessage(`{"endpoint":"127.0.0.1:6379"}`), CapabilityEvidence: json.RawMessage(`{"status":"supported"}`),
		InventoryRevision: 1, CreatedAt: at, UpdatedAt: at,
	}
	if err := proposals.ValidateProposal(proposal); err != nil {
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
