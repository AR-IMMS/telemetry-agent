package configuration

import (
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/ar-imms/telemetry-agent/internal/integration"
)

var (
	// ErrMaterializationConflict indicates one or more snapshot CAS tokens became stale.
	ErrMaterializationConflict = errors.New("template materialization snapshot conflict")
	// ErrInvalidMaterialization identifies an incomplete or inconsistent projection input.
	ErrInvalidMaterialization = errors.New("invalid template materialization")
)

// MaterializationInput binds matching results to the exact inventory, Template and desired snapshots.
type MaterializationInput struct {
	InventoryRevision      uint64
	InventoryWriteRevision uint64
	Templates              TemplateSnapshot
	Desired                DesiredSnapshot
	Resources              []integration.MatchResource
	Matches                integration.TemplateMatchReport
	StaleAfter             time.Duration
}

// MaterializationBatch contains only generated upserts and suspensions; it never deletes provenance.
type MaterializationBatch struct {
	ExpectedInventoryWriteRevision uint64
	ExpectedTemplateSetRevision    uint64
	ExpectedDesiredSetRevision     uint64
	Upsert                         []integration.IntegrationInstance
	Suspend                        []string
}

// MaterializationDiagnostic is a bounded, secret-free explanation for one skipped template result.
type MaterializationDiagnostic struct {
	TemplateID string
	Resource   integration.ResourceIdentity
	Code       string
}

// MaterializationPlan is a complete desired projection plus safe non-durable diagnostics.
type MaterializationPlan struct {
	Batch       MaterializationBatch
	Diagnostics []MaterializationDiagnostic
}

// MaterializationRepository atomically applies a revision-bound generated-instance projection.
type MaterializationRepository interface {
	ApplyTemplateProjection(MaterializationBatch) (uint64, error)
}

// MaterializationService persists a validated projection through Configuration ownership.
type MaterializationService struct{ repository MaterializationRepository }

// NewMaterializationService creates the Configuration-owned projection writer.
func NewMaterializationService(repository MaterializationRepository) MaterializationService {
	return MaterializationService{repository: repository}
}

// Apply commits all template-owned mutations with inventory, Template and desired-state CAS.
func (s MaterializationService) Apply(plan MaterializationPlan) (uint64, error) {
	if s.repository == nil {
		return 0, fmt.Errorf("materialization repository is required")
	}
	if plan.Batch.ExpectedInventoryWriteRevision == 0 || plan.Batch.ExpectedTemplateSetRevision > maxRevision || plan.Batch.ExpectedDesiredSetRevision > maxRevision {
		return 0, ErrInvalidMaterialization
	}
	seen := make(map[string]struct{}, len(plan.Batch.Upsert))
	for _, instance := range plan.Batch.Upsert {
		if instance.Source != integration.SourceTemplate || !instance.Enabled || instance.Revision != 0 {
			return 0, ErrInvalidMaterialization
		}
		if err := integration.ValidateInstance(instance); err != nil {
			return 0, fmt.Errorf("validate generated Integration instance: %w", err)
		}
		identity, err := integration.GeneratedInstanceID(instance.TemplateOwner, instance.TemplateID, integration.ResourceIdentity{
			Provider: instance.Binding.Provider, ProviderInstance: instance.Binding.ProviderInstance, ResourceID: instance.Binding.ResourceID,
		}, instance.DefinitionID)
		if err != nil || identity != instance.ID {
			return 0, ErrInvalidMaterialization
		}
		if _, exists := seen[instance.ID]; exists {
			return 0, ErrInvalidMaterialization
		}
		seen[instance.ID] = struct{}{}
	}
	for _, id := range plan.Batch.Suspend {
		if id == "" {
			return 0, ErrInvalidMaterialization
		}
		if _, exists := seen[id]; exists {
			return 0, ErrInvalidMaterialization
		}
		seen[id] = struct{}{}
	}
	revision, err := s.repository.ApplyTemplateProjection(plan.Batch)
	if err != nil {
		return 0, fmt.Errorf("apply template materialization: %w", err)
	}
	return revision, nil
}

const maxRevision = uint64(^uint64(0) >> 1)

// BuildMaterializationPlan derives generated updates and suspensions without performing I/O.
func BuildMaterializationPlan(input MaterializationInput) (MaterializationPlan, error) {
	if input.InventoryWriteRevision == 0 || input.Templates.Revision > maxRevision || input.Desired.Revision > maxRevision ||
		input.Matches.InventoryRevision != input.InventoryRevision {
		return MaterializationPlan{}, ErrInvalidMaterialization
	}
	plan := MaterializationPlan{Batch: MaterializationBatch{
		ExpectedInventoryWriteRevision: input.InventoryWriteRevision,
		ExpectedTemplateSetRevision:    input.Templates.Revision,
		ExpectedDesiredSetRevision:     input.Desired.Revision,
	}, Diagnostics: []MaterializationDiagnostic{}}
	templateByID := make(map[string]integration.IntegrationTemplate, len(input.Templates.Templates))
	for _, template := range input.Templates.Templates {
		if err := integration.ValidateIntegrationTemplate(template); err != nil || template.Owner != integration.TemplateOwnerLocal {
			return MaterializationPlan{}, ErrInvalidMaterialization
		}
		key := templateKey(template.Owner, template.ID)
		if _, exists := templateByID[key]; exists {
			return MaterializationPlan{}, ErrInvalidMaterialization
		}
		templateByID[key] = template
	}
	resources := make(map[string]integration.MatchResource, len(input.Resources))
	for _, resource := range input.Resources {
		key := resourceKey(integration.ResourceIdentity{Provider: resource.Provider, ProviderInstance: resource.ProviderInstance, ResourceID: resource.ResourceID})
		if _, exists := resources[key]; exists {
			return MaterializationPlan{}, ErrInvalidMaterialization
		}
		resources[key] = resource
	}
	results := make(map[string]integration.TemplateMatchResult, len(input.Matches.Results))
	for _, result := range input.Matches.Results {
		template, exists := templateByID[templateKey(result.TemplateOwner, result.TemplateID)]
		if !exists || template.Status != integration.TemplateEnabled || template.Revision != result.TemplateRevision || result.DefinitionID != template.DefinitionID {
			return MaterializationPlan{}, ErrInvalidMaterialization
		}
		key := resultKey(result.TemplateOwner, result.TemplateID, result.Resource, result.DefinitionID)
		if _, exists := results[key]; exists {
			return MaterializationPlan{}, ErrInvalidMaterialization
		}
		results[key] = result
		if !result.Materializable || result.Instance == nil {
			code := "template_candidate_not_materializable"
			if len(result.Diagnostics) > 0 {
				code = result.Diagnostics[0].Code
			}
			plan.Diagnostics = append(plan.Diagnostics, MaterializationDiagnostic{TemplateID: result.TemplateID, Resource: result.Resource, Code: code})
			continue
		}
		if result.Conflict || result.Instance.TemplateID != template.ID || result.Instance.TemplateRevision != template.Revision || result.Instance.TemplateOwner != template.Owner {
			return MaterializationPlan{}, ErrInvalidMaterialization
		}
		if bindingConflicts(input.Desired.Instances, *result.Instance) {
			plan.Diagnostics = append(plan.Diagnostics, MaterializationDiagnostic{TemplateID: result.TemplateID, Resource: result.Resource, Code: "desired_binding_conflict"})
			continue
		}
		plan.Batch.Upsert = append(plan.Batch.Upsert, *result.Instance)
	}
	upsertIDs := make(map[string]struct{}, len(plan.Batch.Upsert))
	for _, instance := range plan.Batch.Upsert {
		upsertIDs[instance.ID] = struct{}{}
	}
	suspend := make(map[string]struct{})
	for _, instance := range input.Desired.Instances {
		if instance.Source != integration.SourceTemplate || instance.Binding == nil {
			continue
		}
		template, exists := templateByID[templateKey(instance.TemplateOwner, instance.TemplateID)]
		if !exists || template.Status != integration.TemplateEnabled {
			suspend[instance.ID] = struct{}{}
			continue
		}
		if _, active := upsertIDs[instance.ID]; active {
			continue
		}
		resourceID := integration.ResourceIdentity{Provider: instance.Binding.Provider, ProviderInstance: instance.Binding.ProviderInstance, ResourceID: instance.Binding.ResourceID}
		resource, observed := resources[resourceKey(resourceID)]
		if !observed {
			continue
		}
		if resource.Presence == "missing" {
			suspend[instance.ID] = struct{}{}
			continue
		}
		if input.StaleAfter > 0 && (resource.LastSeenAt.IsZero() || resource.LastSeenAt.After(input.Matches.EvaluatedAt) || input.Matches.EvaluatedAt.Sub(resource.LastSeenAt) > input.StaleAfter) {
			continue
		}
		result, evaluated := results[resultKey(instance.TemplateOwner, instance.TemplateID, resourceID, instance.DefinitionID)]
		if evaluated && result.MatchStatus == integration.MatchStale {
			continue
		}
		if !evaluated || result.Conflict || !result.Materializable {
			suspend[instance.ID] = struct{}{}
		}
	}
	for id := range suspend {
		if _, active := upsertIDs[id]; !active {
			plan.Batch.Suspend = append(plan.Batch.Suspend, id)
		}
	}
	sort.Slice(plan.Batch.Upsert, func(i, j int) bool { return plan.Batch.Upsert[i].ID < plan.Batch.Upsert[j].ID })
	sort.Strings(plan.Batch.Suspend)
	sort.Slice(plan.Diagnostics, func(i, j int) bool {
		left, right := plan.Diagnostics[i], plan.Diagnostics[j]
		leftKey := left.TemplateID + "\x00" + resourceKey(left.Resource) + "\x00" + left.Code
		rightKey := right.TemplateID + "\x00" + resourceKey(right.Resource) + "\x00" + right.Code
		return leftKey < rightKey
	})
	return plan, nil
}

func templateKey(owner integration.TemplateOwner, id string) string {
	return string(owner) + "\x00" + id
}

func resourceKey(resource integration.ResourceIdentity) string {
	return resource.Provider + "\x00" + resource.ProviderInstance + "\x00" + resource.ResourceID
}

func resultKey(owner integration.TemplateOwner, templateID string, resource integration.ResourceIdentity, definitionID string) string {
	return templateKey(owner, templateID) + "\x00" + resourceKey(resource) + "\x00" + definitionID
}

func bindingConflicts(instances []integration.IntegrationInstance, candidate integration.IntegrationInstance) bool {
	if candidate.Binding == nil {
		return false
	}
	for _, current := range instances {
		if current.ID == candidate.ID || !current.Enabled || current.Binding == nil {
			continue
		}
		if *current.Binding == *candidate.Binding && (current.Source != integration.SourceTemplate || current.TemplateOwner != candidate.TemplateOwner || current.TemplateID != candidate.TemplateID) {
			return true
		}
	}
	return false
}
