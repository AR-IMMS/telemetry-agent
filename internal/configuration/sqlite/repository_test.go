package sqlite_test

import (
	"errors"
	"path/filepath"
	"sync"
	"testing"

	"github.com/ar-imms/telemetry-agent/internal/agentstate"
	"github.com/ar-imms/telemetry-agent/internal/configuration"
	configurationsqlite "github.com/ar-imms/telemetry-agent/internal/configuration/sqlite"
	"github.com/ar-imms/telemetry-agent/internal/discovery"
	"github.com/ar-imms/telemetry-agent/internal/integration"
)

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
