package integration

import (
	"fmt"
	"sort"
)

type definitionKey struct{ id, version string }

// Registry performs exact ID and version lookup for shipped definitions.
type Registry struct {
	definitions map[definitionKey]IntegrationDefinition
}

// NewRegistry creates a static registry and rejects malformed or duplicate definitions.
func NewRegistry(definitions ...IntegrationDefinition) (Registry, error) {
	items := make(map[definitionKey]IntegrationDefinition, len(definitions))
	matchableVersions := make(map[string]string)
	for _, definition := range definitions {
		if err := ValidateDefinition(definition); err != nil {
			return Registry{}, err
		}
		key := definitionKey{definition.ID, definition.Version}
		if _, exists := items[key]; exists {
			return Registry{}, fmt.Errorf("%w: duplicate %s@%s", ErrInvalidDefinition, key.id, key.version)
		}
		if definition.Matching != nil {
			if version, exists := matchableVersions[definition.ID]; exists {
				return Registry{}, fmt.Errorf("%w: multiple matchable versions for %s (%s and %s)", ErrInvalidDefinition, definition.ID, version, definition.Version)
			}
			matchableVersions[definition.ID] = definition.Version
		}
		items[key] = cloneDefinition(definition)
	}
	return Registry{definitions: items}, nil
}

// ResolveDefinition returns only an exact registered ID/version pair.
func (r Registry) ResolveDefinition(id, version string) (IntegrationDefinition, error) {
	definition, ok := r.definitions[definitionKey{id, version}]
	if !ok {
		return IntegrationDefinition{}, fmt.Errorf("%w: %s@%s", ErrUnknownDefinition, id, version)
	}
	return cloneDefinition(definition), nil
}

// MatchableDefinitions returns a stable copy of definitions with discovery matching enabled.
func (r Registry) MatchableDefinitions() []IntegrationDefinition {
	definitions := make([]IntegrationDefinition, 0)
	for _, definition := range r.definitions {
		if definition.Matching != nil {
			definitions = append(definitions, cloneDefinition(definition))
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

func cloneDefinition(definition IntegrationDefinition) IntegrationDefinition {
	if definition.TemplateSettingRules != nil {
		rules := definition.TemplateSettingRules
		definition.TemplateSettingRules = make(map[string]TemplateSettingRule, len(rules))
		for key, rule := range rules {
			if rule.Default != nil {
				rule.Default, _ = cloneJSONValue(rule.Default)
			}
			definition.TemplateSettingRules[key] = rule
		}
	}
	if definition.Matching == nil {
		return definition
	}
	matching := *definition.Matching
	matching.RequiredCapabilities = append([]string(nil), matching.RequiredCapabilities...)
	if definition.Matching.Selector != nil {
		selector := *definition.Matching.Selector
		selector.ResourceKinds = append([]string(nil), selector.ResourceKinds...)
		selector.LifecycleStates = append([]string(nil), selector.LifecycleStates...)
		selector.ExactAttributes = make(map[string]string, len(definition.Matching.Selector.ExactAttributes))
		for key, value := range definition.Matching.Selector.ExactAttributes {
			selector.ExactAttributes[key] = value
		}
		matching.Selector = &selector
	}
	definition.Matching = &matching
	return definition
}
