package app

import (
	"errors"
	"fmt"

	desiredstatedomain "github.com/ar-imms/telemetry-agent/internal/configuration/desiredstate/domain"
	definitionsapp "github.com/ar-imms/telemetry-agent/internal/monitoring/profiles/definitions/application"
	definitionsdomain "github.com/ar-imms/telemetry-agent/internal/monitoring/profiles/definitions/domain"
)

// MonitoringDesiredStateResolver adapts Monitoring profile resolution to the Configuration desired-state port.
type MonitoringDesiredStateResolver struct {
	resolver definitionsapp.Resolver
}

// NewMonitoringDesiredStateResolver composes one immutable Monitoring resolver for Configuration use cases.
func NewMonitoringDesiredStateResolver(resolver definitionsapp.Resolver) MonitoringDesiredStateResolver {
	return MonitoringDesiredStateResolver{resolver: resolver}
}

// Resolve validates a desired Configuration instance and applies its Monitoring profile rules.
func (r MonitoringDesiredStateResolver) Resolve(instance desiredstatedomain.Instance) (desiredstatedomain.Instance, error) {
	resolved, _, err := r.ResolveWithContribution(instance)
	return resolved, err
}

// ResolveWithContribution resolves one instance and returns the runtime-neutral Monitoring contribution.
func (r MonitoringDesiredStateResolver) ResolveWithContribution(instance desiredstatedomain.Instance) (desiredstatedomain.Instance, definitionsdomain.Contribution, error) {
	if instance.Source == "" {
		instance.Source = desiredstatedomain.SourceLocal
	}
	if err := instance.Validate(); err != nil {
		return desiredstatedomain.Instance{}, definitionsdomain.Contribution{}, err
	}
	resolved, err := r.resolver.Resolve(definitionsapp.ResolveInput{
		DefinitionID: instance.DefinitionID, Version: instance.DefinitionVersion,
		Enabled: instance.Enabled, Settings: instance.Configuration,
	})
	if err != nil {
		if errors.Is(err, definitionsdomain.ErrUnknownDefinition) || errors.Is(err, definitionsdomain.ErrInvalidContribution) {
			return desiredstatedomain.Instance{}, definitionsdomain.Contribution{}, err
		}
		return desiredstatedomain.Instance{}, definitionsdomain.Contribution{}, fmt.Errorf("%w: %v", desiredstatedomain.ErrInvalidInstance, err)
	}
	instance.Configuration = resolved.Settings
	if instance.Binding != nil {
		binding := *instance.Binding
		instance.Binding = &binding
	}
	return instance, resolved.Contribution, nil
}
