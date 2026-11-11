package configuration

import (
	"errors"
	"testing"
	"time"

	"github.com/ar-imms/telemetry-agent/internal/integration"
)

func TestBuildMaterializationPlanUpsertsValidAndSuspendsDisabledOwnership(t *testing.T) {
	active := materializationTemplate("redis-active", integration.TemplateEnabled)
	disabled := materializationTemplate("redis-disabled", integration.TemplateDisabled)
	resource := materializationResource()
	activeInstance := materializationInstance(active, resource, true)
	activeInstance.Revision = 0
	disabledInstance := materializationInstance(disabled, resource, false)
	input := MaterializationInput{
		InventoryRevision: 5, InventoryWriteRevision: 8,
		Templates: TemplateSnapshot{Revision: 4, Templates: []integration.IntegrationTemplate{active, disabled}},
		Desired:   DesiredSnapshot{Revision: 12, Instances: []integration.IntegrationInstance{disabledInstance}},
		Resources: []integration.MatchResource{resource},
		Matches: integration.TemplateMatchReport{InventoryRevision: 5, EvaluatedAt: time.Now().UTC(), Results: []integration.TemplateMatchResult{{
			TemplateID: active.ID, TemplateOwner: active.Owner, TemplateRevision: active.Revision, Resource: integration.ResourceIdentity{Provider: resource.Provider, ProviderInstance: resource.ProviderInstance, ResourceID: resource.ResourceID},
			DefinitionID: active.DefinitionID, DefinitionVersion: active.DefinitionVersion, MatchStatus: integration.MatchMatched,
			ResolutionStatus: integration.ResolutionReady, CapabilityStatus: integration.CapabilitySupported, Applicable: true, Materializable: true,
			Instance: &activeInstance,
		}}},
	}
	plan, err := BuildMaterializationPlan(input)
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

func TestBuildMaterializationPlanPreservesStaleEvidenceAndSuspendsConfirmedAbsence(t *testing.T) {
	template := materializationTemplate("redis-main", integration.TemplateEnabled)
	staleResource := materializationResource()
	staleResource.LastSeenAt = time.Now().UTC().Add(-time.Hour)
	missingResource := staleResource
	missingResource.ProviderInstance = "other"
	missingResource.ResourceID = "container-missing"
	missingResource.Presence = "missing"
	staleInstance := materializationInstance(template, staleResource, true)
	missingInstance := materializationInstance(template, missingResource, true)
	input := MaterializationInput{
		InventoryRevision: 1, InventoryWriteRevision: 2,
		Templates: TemplateSnapshot{Revision: 1, Templates: []integration.IntegrationTemplate{template}},
		Desired:   DesiredSnapshot{Revision: 3, Instances: []integration.IntegrationInstance{staleInstance, missingInstance}},
		Resources: []integration.MatchResource{staleResource, missingResource}, StaleAfter: time.Minute,
		Matches: integration.TemplateMatchReport{InventoryRevision: 1, EvaluatedAt: time.Now().UTC(), Results: []integration.TemplateMatchResult{{
			TemplateID: template.ID, TemplateOwner: template.Owner, TemplateRevision: template.Revision,
			Resource:     integration.ResourceIdentity{Provider: staleResource.Provider, ProviderInstance: staleResource.ProviderInstance, ResourceID: staleResource.ResourceID},
			DefinitionID: template.DefinitionID, MatchStatus: integration.MatchStale, ResolutionStatus: integration.ResolutionNeedsInput, Applicable: true,
		}}},
	}
	plan, err := BuildMaterializationPlan(input)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Batch.Suspend) != 1 || plan.Batch.Suspend[0] != missingInstance.ID {
		t.Fatalf("suspended IDs=%v, want only confirmed missing resource", plan.Batch.Suspend)
	}
}

func TestBuildMaterializationPlanDoesNotCreateTemplateInstanceOnManualBindingConflict(t *testing.T) {
	template := materializationTemplate("redis-main", integration.TemplateEnabled)
	resource := materializationResource()
	candidate := materializationInstance(template, resource, true)
	manual := candidate
	manual.ID = "manual-redis"
	manual.Source = integration.SourceLocal
	manual.TemplateOwner, manual.TemplateID, manual.TemplateRevision = "", "", 0
	input := MaterializationInput{
		InventoryRevision: 1, InventoryWriteRevision: 1,
		Templates: TemplateSnapshot{Revision: 1, Templates: []integration.IntegrationTemplate{template}},
		Desired:   DesiredSnapshot{Revision: 1, Instances: []integration.IntegrationInstance{manual}},
		Resources: []integration.MatchResource{resource},
		Matches: integration.TemplateMatchReport{InventoryRevision: 1, Results: []integration.TemplateMatchResult{{
			TemplateID: template.ID, TemplateOwner: template.Owner, TemplateRevision: template.Revision,
			Resource:     integration.ResourceIdentity{Provider: resource.Provider, ProviderInstance: resource.ProviderInstance, ResourceID: resource.ResourceID},
			DefinitionID: template.DefinitionID, Materializable: true, Instance: &candidate,
		}}},
	}
	plan, err := BuildMaterializationPlan(input)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Batch.Upsert) != 0 || len(plan.Diagnostics) != 1 || plan.Diagnostics[0].Code != "desired_binding_conflict" {
		t.Fatalf("plan=%+v", plan)
	}
}

func TestBuildMaterializationPlanRejectsMismatchedSnapshots(t *testing.T) {
	input := MaterializationInput{InventoryWriteRevision: 1, InventoryRevision: 2, Matches: integration.TemplateMatchReport{InventoryRevision: 1}}
	if _, err := BuildMaterializationPlan(input); !errors.Is(err, ErrInvalidMaterialization) {
		t.Fatalf("mismatched inventory revisions error=%v", err)
	}
}

func materializationTemplate(id string, status integration.TemplateStatus) integration.IntegrationTemplate {
	return integration.IntegrationTemplate{ID: id, DefinitionID: "redis", DefinitionVersion: "1.0.0", Owner: integration.TemplateOwnerLocal, Revision: 1, Status: status, Settings: map[string]any{}}
}

func materializationResource() integration.MatchResource {
	return integration.MatchResource{Provider: "docker", ProviderInstance: "local", ResourceID: "container-a", Kind: "container", LifecycleState: "running", Presence: "present", LastSeenAt: time.Now().UTC()}
}

func materializationInstance(template integration.IntegrationTemplate, resource integration.MatchResource, enabled bool) integration.IntegrationInstance {
	id, _ := integration.GeneratedInstanceID(template.Owner, template.ID, integration.ResourceIdentity{Provider: resource.Provider, ProviderInstance: resource.ProviderInstance, ResourceID: resource.ResourceID}, template.DefinitionID)
	return integration.IntegrationInstance{
		ID: id, DefinitionID: template.DefinitionID, DefinitionVersion: template.DefinitionVersion, Configuration: map[string]any{"endpoint": "127.0.0.1:6379"},
		Source: integration.SourceTemplate, Binding: &integration.ResourceBinding{Provider: resource.Provider, ProviderInstance: resource.ProviderInstance, ResourceID: resource.ResourceID},
		TemplateOwner: template.Owner, TemplateID: template.ID, TemplateRevision: template.Revision, Enabled: enabled, Revision: 1,
	}
}
