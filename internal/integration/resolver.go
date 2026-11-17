package integration

import (
	"encoding/json"
	"errors"
	"fmt"

	definitionapp "github.com/ar-imms/telemetry-agent/internal/monitoring/profiles/definitions/application"
	definitions "github.com/ar-imms/telemetry-agent/internal/monitoring/profiles/definitions/domain"
)

// ResolvedInstance preserves the compatibility result shape used by existing Runtime callers.
type ResolvedInstance struct {
	Instance     IntegrationInstance
	Contribution Contribution
}

// Resolver adapts the Configuration-owned instance model to Monitoring's settings projection.
type Resolver struct{ registry Registry }

// NewResolver creates a compatibility adapter over Monitoring's static registry.
func NewResolver(registry Registry) Resolver { return Resolver{registry: registry} }

// Resolve validates and projects one desired instance before delegating profile resolution to Monitoring.
func (r Resolver) Resolve(instance IntegrationInstance) (ResolvedInstance, error) {
	if instance.Source == "" {
		instance.Source = SourceLocal
	}
	if err := ValidateInstance(instance); err != nil {
		return ResolvedInstance{}, err
	}
	encoded, err := json.Marshal(instance.Configuration)
	if err != nil {
		return ResolvedInstance{}, fmt.Errorf("%w: copy settings", ErrInvalidInstance)
	}
	var copied map[string]any
	if err := json.Unmarshal(encoded, &copied); err != nil {
		return ResolvedInstance{}, fmt.Errorf("%w: copy settings", ErrInvalidInstance)
	}
	instance.Configuration = copied
	if instance.Binding != nil {
		binding := *instance.Binding
		instance.Binding = &binding
	}
	resolved, err := definitionapp.NewResolver(r.registry).Resolve(definitionapp.ResolveInput{
		DefinitionID: instance.DefinitionID, Version: instance.DefinitionVersion,
		Enabled: instance.Enabled, Settings: instance.Configuration,
	})
	if err != nil {
		if errors.Is(err, definitions.ErrUnknownDefinition) || errors.Is(err, definitions.ErrInvalidContribution) {
			return ResolvedInstance{}, err
		}
		if err != nil {
			return ResolvedInstance{}, fmt.Errorf("%w: %v", ErrInvalidInstance, err)
		}
		return ResolvedInstance{}, err
	}
	instance.Configuration = resolved.Settings
	return ResolvedInstance{Instance: instance, Contribution: resolved.Contribution}, nil
}
