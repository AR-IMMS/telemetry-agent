package application

import (
	"encoding/json"
	"fmt"

	"github.com/ar-imms/telemetry-agent/internal/monitoring/profiles/definitions/domain"
)

// ResolveInput is the narrow settings projection required to resolve a profile definition.
type ResolveInput struct {
	DefinitionID string
	Version      string
	Enabled      bool
	Settings     map[string]any
}

// Resolved contains validated settings and the profile's runtime-neutral contribution.
type Resolved struct {
	DefinitionID string
	Version      string
	Settings     map[string]any
	Contribution domain.Contribution
}

// Intents returns a defensive copy of the resolved component intents.
func (r Resolved) Intents() []domain.ComponentIntent { return r.Contribution.Intents() }

// Resolver validates a narrow settings projection and invokes the exact registered definition.
type Resolver struct{ registry Registry }

// NewResolver binds deterministic profile resolution to one immutable registry.
func NewResolver(registry Registry) Resolver { return Resolver{registry: registry} }

// Resolve validates one settings projection without Configuration models, I/O, persistence or activation.
func (r Resolver) Resolve(input ResolveInput) (Resolved, error) {
	definition, err := r.registry.ResolveDefinition(input.DefinitionID, input.Version)
	if err != nil {
		return Resolved{}, err
	}
	settings, err := cloneSettings(input.Settings)
	if err != nil {
		return Resolved{}, fmt.Errorf("%w: copy settings: %v", domain.ErrInvalidDefinition, err)
	}
	if err := definition.Validate(settings); err != nil {
		return Resolved{}, fmt.Errorf("%w: %v", domain.ErrInvalidDefinition, err)
	}
	resolved := Resolved{DefinitionID: definition.ID, Version: definition.Version, Settings: settings, Contribution: domain.NewContribution(nil)}
	if !input.Enabled {
		return resolved, nil
	}
	contribution, err := definition.Contribute(settings)
	if err != nil {
		return Resolved{}, fmt.Errorf("%w: %v", domain.ErrInvalidContribution, err)
	}
	resolved.Contribution = domain.NewContribution(contribution.Intents())
	return resolved, nil
}

func cloneSettings(settings map[string]any) (map[string]any, error) {
	encoded, err := json.Marshal(settings)
	if err != nil {
		return nil, err
	}
	var copied map[string]any
	if err := json.Unmarshal(encoded, &copied); err != nil {
		return nil, err
	}
	return copied, nil
}
