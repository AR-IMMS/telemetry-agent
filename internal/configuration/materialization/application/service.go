package application

import (
	"fmt"
	"sort"
	"time"

	desiredapp "github.com/ar-imms/telemetry-agent/internal/configuration/desiredstate/application"
	desiredstate "github.com/ar-imms/telemetry-agent/internal/configuration/desiredstate/domain"
	"github.com/ar-imms/telemetry-agent/internal/configuration/materialization/domain"
	templatesapp "github.com/ar-imms/telemetry-agent/internal/configuration/templates/application"
	templatedomain "github.com/ar-imms/telemetry-agent/internal/configuration/templates/domain"
	matching "github.com/ar-imms/telemetry-agent/internal/monitoring/matching/domain"
	templatesmonitoring "github.com/ar-imms/telemetry-agent/internal/monitoring/templates/application"
)

var (
	// ErrInvalidMaterialization identifies an incomplete or inconsistent projection input.
	ErrInvalidMaterialization = domain.ErrInvalidMaterialization
	// ErrMaterializationConflict indicates that a snapshot CAS token became stale.
	ErrMaterializationConflict = domain.ErrMaterializationConflict
)

// Input binds matching results to the exact Inventory, Template and desired snapshots.
type Input struct {
	InventoryRevision      uint64
	InventoryWriteRevision uint64
	Templates              templatesapp.Snapshot
	Desired                desiredapp.Snapshot
	Resources              []matching.MatchResource
	Matches                templatesmonitoring.TemplateMatchReport
	StaleAfter             time.Duration
}

// Repository atomically applies a revision-bound generated-instance projection.
type Repository interface {
	ApplyTemplateProjection(domain.Batch) (uint64, error)
}

// Service persists a validated projection through the owning Repository.
type Service struct{ repository Repository }

// NewService creates a Configuration-owned projection writer.
func NewService(repository Repository) Service { return Service{repository: repository} }

// Apply commits all template-owned mutations with Inventory, Template and desired-state CAS.
func (s Service) Apply(plan domain.Plan) (uint64, error) {
	if s.repository == nil {
		return 0, fmt.Errorf("materialization repository is required")
	}
	if err := domain.ValidateBatch(plan.Batch); err != nil {
		return 0, err
	}
	revision, err := s.repository.ApplyTemplateProjection(plan.Batch)
	if err != nil {
		return 0, fmt.Errorf("apply template materialization: %w", err)
	}
	return revision, nil
}

// BuildPlan derives generated updates and suspensions without performing I/O.
func BuildPlan(input Input) (domain.Plan, error) {
	if input.InventoryWriteRevision == 0 || input.Templates.Revision > maxRevision || input.Desired.Revision > maxRevision ||
		input.Matches.InventoryRevision != input.InventoryRevision {
		return domain.Plan{}, ErrInvalidMaterialization
	}
	plan := domain.Plan{Batch: domain.Batch{
		ExpectedInventoryWriteRevision: input.InventoryWriteRevision,
		ExpectedTemplateSetRevision:    input.Templates.Revision,
		ExpectedDesiredSetRevision:     input.Desired.Revision,
	}, Diagnostics: []domain.Diagnostic{}}
	templateByID := make(map[string]templatedomain.Template, len(input.Templates.Templates))
	for _, template := range input.Templates.Templates {
		if err := template.Validate(); err != nil || template.Owner != desiredstate.TemplateOwnerLocal {
			return domain.Plan{}, ErrInvalidMaterialization
		}
		key := templateKey(template.Owner, template.ID)
		if _, exists := templateByID[key]; exists {
			return domain.Plan{}, ErrInvalidMaterialization
		}
		templateByID[key] = template
	}
	resources := make(map[string]matching.MatchResource, len(input.Resources))
	for _, resource := range input.Resources {
		key := resourceKey(desiredstate.ResourceBinding{Provider: resource.Provider, ProviderInstance: resource.ProviderInstance, ResourceID: resource.ResourceID})
		if _, exists := resources[key]; exists {
			return domain.Plan{}, ErrInvalidMaterialization
		}
		resources[key] = resource
	}
	results := make(map[string]templatesmonitoring.TemplateMatchResult, len(input.Matches.Results))
	for _, result := range input.Matches.Results {
		owner := desiredstate.TemplateOwner(result.TemplateOwner)
		template, exists := templateByID[templateKey(owner, result.TemplateID)]
		if !exists || template.Status != templatedomain.StatusEnabled || template.Revision != result.TemplateRevision || template.DefinitionID != result.DefinitionID {
			return domain.Plan{}, ErrInvalidMaterialization
		}
		identity := desiredstate.ResourceBinding{Provider: result.Resource.Provider, ProviderInstance: result.Resource.ProviderInstance, ResourceID: result.Resource.ResourceID}
		key := resultKey(owner, result.TemplateID, identity, result.DefinitionID)
		if _, exists := results[key]; exists {
			return domain.Plan{}, ErrInvalidMaterialization
		}
		results[key] = result
		if !result.Materializable || result.Candidate == nil || result.Settings == nil {
			code := "template_candidate_not_materializable"
			if len(result.Diagnostics) > 0 {
				code = result.Diagnostics[0].Code
			}
			plan.Diagnostics = append(plan.Diagnostics, domain.Diagnostic{TemplateID: result.TemplateID, Resource: identity, Code: code})
			continue
		}
		if result.Conflict || result.DefinitionVersion != template.DefinitionVersion {
			return domain.Plan{}, ErrInvalidMaterialization
		}
		instance, err := generatedInstance(template, result)
		if err != nil {
			return domain.Plan{}, ErrInvalidMaterialization
		}
		if bindingConflicts(input.Desired.Instances, instance) {
			plan.Diagnostics = append(plan.Diagnostics, domain.Diagnostic{TemplateID: result.TemplateID, Resource: identity, Code: "desired_binding_conflict"})
			continue
		}
		plan.Batch.Upsert = append(plan.Batch.Upsert, instance)
	}
	upsertIDs := make(map[string]struct{}, len(plan.Batch.Upsert))
	for _, instance := range plan.Batch.Upsert {
		upsertIDs[instance.ID] = struct{}{}
	}
	suspend := make(map[string]struct{})
	for _, instance := range input.Desired.Instances {
		if instance.Source != desiredstate.SourceTemplate || instance.Binding == nil {
			continue
		}
		template, exists := templateByID[templateKey(instance.TemplateOwner, instance.TemplateID)]
		if !exists || template.Status != templatedomain.StatusEnabled {
			suspend[instance.ID] = struct{}{}
			continue
		}
		if _, active := upsertIDs[instance.ID]; active {
			continue
		}
		identity := desiredstate.ResourceBinding{Provider: instance.Binding.Provider, ProviderInstance: instance.Binding.ProviderInstance, ResourceID: instance.Binding.ResourceID}
		resource, observed := resources[resourceKey(identity)]
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
		result, evaluated := results[resultKey(instance.TemplateOwner, instance.TemplateID, identity, instance.DefinitionID)]
		if evaluated && result.MatchStatus == matching.MatchStale {
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

const maxRevision = uint64(^uint64(0) >> 1)

func generatedInstance(template templatedomain.Template, result templatesmonitoring.TemplateMatchResult) (desiredstate.Instance, error) {
	identity := desiredstate.ResourceBinding{Provider: result.Resource.Provider, ProviderInstance: result.Resource.ProviderInstance, ResourceID: result.Resource.ResourceID}
	id, err := domain.GeneratedInstanceID(template.Owner, template.ID, identity, template.DefinitionID)
	if err != nil {
		return desiredstate.Instance{}, err
	}
	instance := desiredstate.Instance{
		ID: id, DefinitionID: result.DefinitionID, DefinitionVersion: result.DefinitionVersion,
		Configuration: result.Settings, Source: desiredstate.SourceTemplate,
		Binding:       &desiredstate.ResourceBinding{Provider: identity.Provider, ProviderInstance: identity.ProviderInstance, ResourceID: identity.ResourceID},
		TemplateOwner: template.Owner, TemplateID: template.ID, TemplateRevision: template.Revision, Enabled: true,
	}
	return instance, instance.Validate()
}

func templateKey(owner desiredstate.TemplateOwner, id string) string {
	return string(owner) + "\x00" + id
}

func resourceKey(resource desiredstate.ResourceBinding) string {
	return resource.Provider + "\x00" + resource.ProviderInstance + "\x00" + resource.ResourceID
}

func resultKey(owner desiredstate.TemplateOwner, templateID string, resource desiredstate.ResourceBinding, definitionID string) string {
	return templateKey(owner, templateID) + "\x00" + resourceKey(resource) + "\x00" + definitionID
}

func bindingConflicts(instances []desiredstate.Instance, candidate desiredstate.Instance) bool {
	if candidate.Binding == nil {
		return false
	}
	for _, current := range instances {
		if current.ID == candidate.ID || !current.Enabled || current.Binding == nil {
			continue
		}
		if *current.Binding == *candidate.Binding && (current.Source != desiredstate.SourceTemplate || current.TemplateOwner != candidate.TemplateOwner || current.TemplateID != candidate.TemplateID) {
			return true
		}
	}
	return false
}
