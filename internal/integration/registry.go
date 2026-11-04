package integration

import "fmt"

type definitionKey struct{ id, version string }

// Registry performs exact ID and version lookup for shipped definitions.
type Registry struct {
	definitions map[definitionKey]IntegrationDefinition
}

// NewRegistry creates a static registry and rejects malformed or duplicate definitions.
func NewRegistry(definitions ...IntegrationDefinition) (Registry, error) {
	items := make(map[definitionKey]IntegrationDefinition, len(definitions))
	for _, definition := range definitions {
		if err := ValidateDefinition(definition); err != nil {
			return Registry{}, err
		}
		key := definitionKey{definition.ID, definition.Version}
		if _, exists := items[key]; exists {
			return Registry{}, fmt.Errorf("%w: duplicate %s@%s", ErrInvalidDefinition, key.id, key.version)
		}
		items[key] = definition
	}
	return Registry{definitions: items}, nil
}

// ResolveDefinition returns only an exact registered ID/version pair.
func (r Registry) ResolveDefinition(id, version string) (IntegrationDefinition, error) {
	definition, ok := r.definitions[definitionKey{id, version}]
	if !ok {
		return IntegrationDefinition{}, fmt.Errorf("%w: %s@%s", ErrUnknownDefinition, id, version)
	}
	return definition, nil
}
