package application

import (
	"errors"
	"testing"
	"time"

	desiredapp "github.com/ar-imms/telemetry-agent/internal/configuration/desiredstate/application"
	desiredstate "github.com/ar-imms/telemetry-agent/internal/configuration/desiredstate/domain"
	materializationdomain "github.com/ar-imms/telemetry-agent/internal/configuration/materialization/domain"
	configurationtemplatesapp "github.com/ar-imms/telemetry-agent/internal/configuration/templates/application"
	templatedomain "github.com/ar-imms/telemetry-agent/internal/configuration/templates/domain"
	matching "github.com/ar-imms/telemetry-agent/internal/monitoring/matching/domain"
	templatesapp "github.com/ar-imms/telemetry-agent/internal/monitoring/templates/application"
)

func TestBuildPlanUpsertsMaterializableAndSuspendsDisabledTemplateInstances(t *testing.T) {
	active := testTemplate("redis-active", templatedomain.StatusEnabled)
	disabled := testTemplate("redis-disabled", templatedomain.StatusDisabled)
	resource := testResource("container-a", "present")
	activeInstance := testInstance(active, resource, true)
	disabledInstance := testInstance(disabled, resource, false)
	input := Input{
		InventoryRevision: 5, InventoryWriteRevision: 8,
		Templates: configurationtemplatesapp.Snapshot{Revision: 4, Templates: []templatedomain.Template{active, disabled}},
		Desired:   desiredapp.Snapshot{Revision: 12, Instances: []desiredstate.Instance{disabledInstance}},
		Resources: []matching.MatchResource{resource},
		Matches: templatesapp.TemplateMatchReport{InventoryRevision: 5, EvaluatedAt: time.Now().UTC(), Results: []templatesapp.TemplateMatchResult{{
			TemplateID: active.ID, TemplateOwner: string(active.Owner), TemplateRevision: active.Revision,
			Resource: identity(resource), DefinitionID: active.DefinitionID, DefinitionVersion: active.DefinitionVersion,
			MatchStatus: matching.MatchMatched, Materializable: true, Candidate: &matching.Candidate{}, Settings: activeInstance.Configuration,
		}}},
	}
	plan, err := BuildPlan(input)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Batch.Upsert) != 1 || plan.Batch.Upsert[0].ID != activeInstance.ID || len(plan.Batch.Suspend) != 1 || plan.Batch.Suspend[0] != disabledInstance.ID {
		t.Fatalf("batch=%+v", plan.Batch)
	}
	if plan.Batch.ExpectedInventoryWriteRevision != 8 || plan.Batch.ExpectedTemplateSetRevision != 4 || plan.Batch.ExpectedDesiredSetRevision != 12 {
		t.Fatalf("CAS tokens=%+v", plan.Batch)
	}
}

func TestBuildPlanPreservesStaleEvidenceAndSuspendsOnlyConfirmedAbsence(t *testing.T) {
	template := testTemplate("redis-main", templatedomain.StatusEnabled)
	staleResource := testResource("container-stale", "present")
	staleResource.LastSeenAt = time.Now().UTC().Add(-time.Hour)
	missingResource := testResource("container-missing", "missing")
	staleInstance := testInstance(template, staleResource, true)
	missingInstance := testInstance(template, missingResource, true)
	input := Input{
		InventoryRevision: 1, InventoryWriteRevision: 2,
		Templates: configurationtemplatesapp.Snapshot{Revision: 1, Templates: []templatedomain.Template{template}},
		Desired:   desiredapp.Snapshot{Revision: 3, Instances: []desiredstate.Instance{staleInstance, missingInstance}},
		Resources: []matching.MatchResource{staleResource, missingResource}, StaleAfter: time.Minute,
		Matches: templatesapp.TemplateMatchReport{InventoryRevision: 1, EvaluatedAt: time.Now().UTC(), Results: []templatesapp.TemplateMatchResult{{
			TemplateID: template.ID, TemplateOwner: string(template.Owner), TemplateRevision: template.Revision,
			Resource: identity(staleResource), DefinitionID: template.DefinitionID, MatchStatus: matching.MatchStale,
		}}},
	}
	plan, err := BuildPlan(input)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Batch.Suspend) != 1 || plan.Batch.Suspend[0] != missingInstance.ID {
		t.Fatalf("suspended IDs=%v, want only confirmed missing resource", plan.Batch.Suspend)
	}
}

func TestBuildPlanDoesNotMaterializeManualBindingConflict(t *testing.T) {
	template := testTemplate("redis-main", templatedomain.StatusEnabled)
	resource := testResource("container-a", "present")
	candidate := testInstance(template, resource, true)
	manual := candidate
	manual.ID, manual.Source = "manual-redis", desiredstate.SourceLocal
	manual.TemplateOwner, manual.TemplateID, manual.TemplateRevision = "", "", 0
	input := Input{
		InventoryRevision: 1, InventoryWriteRevision: 1,
		Templates: configurationtemplatesapp.Snapshot{Revision: 1, Templates: []templatedomain.Template{template}},
		Desired:   desiredapp.Snapshot{Revision: 1, Instances: []desiredstate.Instance{manual}},
		Resources: []matching.MatchResource{resource},
		Matches: templatesapp.TemplateMatchReport{InventoryRevision: 1, Results: []templatesapp.TemplateMatchResult{{
			TemplateID: template.ID, TemplateOwner: string(template.Owner), TemplateRevision: template.Revision,
			Resource: identity(resource), DefinitionID: template.DefinitionID, DefinitionVersion: template.DefinitionVersion, Materializable: true,
			Candidate: &matching.Candidate{}, Settings: candidate.Configuration,
		}}},
	}
	plan, err := BuildPlan(input)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Batch.Upsert) != 0 || len(plan.Diagnostics) != 1 || plan.Diagnostics[0].Code != "desired_binding_conflict" {
		t.Fatalf("plan=%+v", plan)
	}
}

func TestBuildPlanRejectsMismatchedInventorySnapshots(t *testing.T) {
	input := Input{InventoryWriteRevision: 1, InventoryRevision: 2, Matches: templatesapp.TemplateMatchReport{InventoryRevision: 1}}
	if _, err := BuildPlan(input); !errors.Is(err, ErrInvalidMaterialization) {
		t.Fatalf("mismatched inventory revisions error=%v", err)
	}
}

func TestBuildPlanRejectsResultForDifferentProfileVersion(t *testing.T) {
	template := testTemplate("redis-main", templatedomain.StatusEnabled)
	resource := testResource("container-a", "present")
	input := Input{
		InventoryRevision: 1, InventoryWriteRevision: 1,
		Templates: configurationtemplatesapp.Snapshot{Revision: 1, Templates: []templatedomain.Template{template}},
		Resources: []matching.MatchResource{resource},
		Matches: templatesapp.TemplateMatchReport{InventoryRevision: 1, Results: []templatesapp.TemplateMatchResult{{
			TemplateID: template.ID, TemplateOwner: string(template.Owner), TemplateRevision: template.Revision,
			Resource: identity(resource), DefinitionID: template.DefinitionID, DefinitionVersion: "2.0.0",
			Materializable: true, Candidate: &matching.Candidate{}, Settings: map[string]any{},
		}}},
	}
	if _, err := BuildPlan(input); !errors.Is(err, ErrInvalidMaterialization) {
		t.Fatalf("mismatched profile version error=%v", err)
	}
}

func TestServiceRejectsInvalidGeneratedIdentityBeforePersistence(t *testing.T) {
	repository := &recordingRepository{}
	service := NewService(repository)
	plan := materializationdomain.Plan{Batch: materializationdomain.Batch{
		ExpectedInventoryWriteRevision: 1,
		Upsert: []desiredstate.Instance{{ID: "wrong", DefinitionID: "redis", DefinitionVersion: "1.0.0", Configuration: map[string]any{}, Source: desiredstate.SourceTemplate,
			Binding:       &desiredstate.ResourceBinding{Provider: "docker", ProviderInstance: "local", ResourceID: "container-a"},
			TemplateOwner: desiredstate.TemplateOwnerLocal, TemplateID: "redis-main", TemplateRevision: 1, Enabled: true}},
	}}
	if _, err := service.Apply(plan); !errors.Is(err, ErrInvalidMaterialization) {
		t.Fatalf("Apply error=%v", err)
	}
	if repository.calls != 0 {
		t.Fatalf("repository calls=%d, want 0", repository.calls)
	}
}

type recordingRepository struct{ calls int }

func (r *recordingRepository) ApplyTemplateProjection(materializationdomain.Batch) (uint64, error) {
	r.calls++
	return 1, nil
}

func testTemplate(id string, status templatedomain.Status) templatedomain.Template {
	return templatedomain.Template{ID: id, DefinitionID: "redis", DefinitionVersion: "1.0.0", Owner: desiredstate.TemplateOwnerLocal, Revision: 1, Status: status, Settings: map[string]any{}}
}

func testResource(id, presence string) matching.MatchResource {
	return matching.MatchResource{Provider: "docker", ProviderInstance: "local", ResourceID: id, Kind: "container", LifecycleState: "running", Presence: presence, LastSeenAt: time.Now().UTC()}
}

func identity(resource matching.MatchResource) matching.ResourceIdentity {
	return matching.ResourceIdentity{Provider: resource.Provider, ProviderInstance: resource.ProviderInstance, ResourceID: resource.ResourceID}
}

func testInstance(template templatedomain.Template, resource matching.MatchResource, enabled bool) desiredstate.Instance {
	resourceBinding := desiredstate.ResourceBinding{Provider: resource.Provider, ProviderInstance: resource.ProviderInstance, ResourceID: resource.ResourceID}
	id, _ := materializationdomain.GeneratedInstanceID(template.Owner, template.ID, resourceBinding, template.DefinitionID)
	return desiredstate.Instance{
		ID: id, DefinitionID: template.DefinitionID, DefinitionVersion: template.DefinitionVersion, Configuration: map[string]any{"endpoint": "127.0.0.1:6379"},
		Source: desiredstate.SourceTemplate, Binding: &desiredstate.ResourceBinding{Provider: resource.Provider, ProviderInstance: resource.ProviderInstance, ResourceID: resource.ResourceID},
		TemplateOwner: template.Owner, TemplateID: template.ID, TemplateRevision: template.Revision, Enabled: enabled, Revision: 1,
	}
}
