package integration

import (
	"encoding/json"
	"fmt"
)

// ResolvedInstance contains a validated instance and its deterministic contribution.
type ResolvedInstance struct {
	Instance     IntegrationInstance
	Contribution Contribution
}

// Resolver validates desired instances and invokes their registered contributor.
type Resolver struct{ registry Registry }

// NewResolver creates a resolver over a static definition registry.
func NewResolver(registry Registry) Resolver { return Resolver{registry: registry} }

// Resolve validates and resolves one instance without I/O or persistence.
func (r Resolver) Resolve(instance IntegrationInstance) (ResolvedInstance, error) {
	if err := ValidateInstance(instance); err != nil {
		return ResolvedInstance{}, err
	}
	definition, err := r.registry.ResolveDefinition(instance.DefinitionID, instance.DefinitionVersion)
	if err != nil {
		return ResolvedInstance{}, err
	}
	configuration, _ := json.Marshal(instance.Configuration)
	var copied map[string]any
	_ = json.Unmarshal(configuration, &copied)
	instance.Configuration = copied
	if err := definition.Validate(instance.Configuration); err != nil {
		return ResolvedInstance{}, fmt.Errorf("%w: %v", ErrInvalidInstance, err)
	}
	if !instance.Enabled {
		return ResolvedInstance{Instance: instance, Contribution: NewContribution(nil)}, nil
	}
	contribution, err := definition.Contribute(instance)
	if err != nil {
		return ResolvedInstance{}, fmt.Errorf("%w: %v", ErrInvalidContribution, err)
	}
	return ResolvedInstance{Instance: instance, Contribution: NewContribution(contribution.Intents())}, nil
}
