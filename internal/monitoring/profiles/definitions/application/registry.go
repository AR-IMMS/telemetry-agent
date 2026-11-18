package application

import (
	"fmt"
	"sort"

	"github.com/ar-imms/telemetry-agent/internal/monitoring/profiles/definitions/domain"
)

type definitionKey struct{ id, version string }

// Registry performs exact ID and version lookup for shipped Monitoring definitions.
type Registry struct {
	definitions map[definitionKey]domain.Definition
}

// NewRegistry creates a static registry and rejects malformed or duplicate definitions.
func NewRegistry(definitions ...domain.Definition) (Registry, error) {
	items := make(map[definitionKey]domain.Definition, len(definitions))
	matchableVersions := make(map[string]string)
	for _, definition := range definitions {
		if err := domain.ValidateDefinition(definition); err != nil {
			return Registry{}, err
		}
		key := definitionKey{definition.ID, definition.Version}
		if _, exists := items[key]; exists {
			return Registry{}, fmt.Errorf("%w: duplicate %s@%s", domain.ErrInvalidDefinition, key.id, key.version)
		}
		if definition.Matching != nil {
			if version, exists := matchableVersions[definition.ID]; exists {
				return Registry{}, fmt.Errorf("%w: multiple matchable versions for %s (%s and %s)", domain.ErrInvalidDefinition, definition.ID, version, definition.Version)
			}
			matchableVersions[definition.ID] = definition.Version
		}
		items[key] = definition.Clone()
	}
	return Registry{definitions: items}, nil
}

// ResolveDefinition returns only an exact registered ID/version pair.
func (r Registry) ResolveDefinition(id, version string) (domain.Definition, error) {
	definition, ok := r.definitions[definitionKey{id, version}]
	if !ok {
		return domain.Definition{}, fmt.Errorf("%w: %s@%s", domain.ErrUnknownDefinition, id, version)
	}
	return definition.Clone(), nil
}

// MatchableDefinitions returns a stable copy of definitions with discovery matching enabled.
func (r Registry) MatchableDefinitions() []domain.Definition {
	definitions := make([]domain.Definition, 0)
	for _, definition := range r.definitions {
		if definition.Matching != nil {
			definitions = append(definitions, definition.Clone())
		}
	}
	sort.Slice(definitions, func(i, j int) bool {
		if definitions[i].ID == definitions[j].ID {
			return definitions[i].Version < definitions[j].Version
		}
		return definitions[i].ID < definitions[j].ID
	})
	return definitions
}

// Definitions returns every registered definition as a defensive copy in stable ID/version order.
func (r Registry) Definitions() []domain.Definition {
	definitions := make([]domain.Definition, 0, len(r.definitions))
	for _, definition := range r.definitions {
		definitions = append(definitions, definition.Clone())
	}
	sort.Slice(definitions, func(i, j int) bool {
		if definitions[i].ID == definitions[j].ID {
			return definitions[i].Version < definitions[j].Version
		}
		return definitions[i].ID < definitions[j].ID
	})
	return definitions
}
