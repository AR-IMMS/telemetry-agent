package application

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sort"

	templatedomain "github.com/ar-imms/telemetry-agent/internal/configuration/templates/domain"
	matchingdomain "github.com/ar-imms/telemetry-agent/internal/monitoring/matching/domain"
	"github.com/ar-imms/telemetry-agent/internal/monitoring/profiles/definitions"
)

var (
	// ErrTemplateSettingsConflict reports incompatible fixed settings sources.
	ErrTemplateSettingsConflict = errors.New("Integration Template settings conflict")
	// ErrTemplateValueUnavailable reports a binding source that is absent or ambiguous.
	ErrTemplateValueUnavailable = errors.New("Integration Template binding value unavailable")
)

// ResolveSettings applies exact-version profile defaults, explicit values, and declared
// resource bindings. Returned settings are detached and secret-free.
func ResolveSettings(template templatedomain.Template, definition definitions.Definition, resource matchingdomain.MatchResource) (map[string]any, error) {
	if err := template.Validate(); err != nil {
		return nil, err
	}
	if definition.ID != template.DefinitionID || definition.Version != template.DefinitionVersion {
		return nil, fmt.Errorf("%w: Template requires exact Definition %s@%s", templatedomain.ErrInvalidTemplate, template.DefinitionID, template.DefinitionVersion)
	}
	if err := definitions.ValidateDefinition(definition); err != nil {
		return nil, err
	}
	settings := make(map[string]any, len(definition.TemplateSettingRules)+len(template.Settings))
	for key, rule := range definition.TemplateSettingRules {
		if rule.Default != nil {
			value, err := cloneJSONValue(rule.Default)
			if err != nil {
				return nil, fmt.Errorf("%w: copy Definition default for %q: %v", templatedomain.ErrInvalidTemplate, key, err)
			}
			if err := definitions.ValidateTemplateSettingValue(key, value, rule); err != nil {
				return nil, err
			}
			settings[key] = value
		}
	}
	keys := make([]string, 0, len(template.Settings))
	for key := range template.Settings {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		rule, exists := definition.TemplateSettingRules[key]
		if !exists {
			return nil, fmt.Errorf("%w: Definition does not declare setting %q for templates", templatedomain.ErrInvalidTemplate, key)
		}
		if err := definitions.ValidateTemplateSettingValue(key, template.Settings[key], rule); err != nil {
			return nil, err
		}
		if rule.Default != nil && !rule.DefaultOverridable {
			if !sameJSONValue(rule.Default, template.Settings[key]) {
				return nil, fmt.Errorf("%w: Definition setting %q is fixed", ErrTemplateSettingsConflict, key)
			}
			continue
		}
		cloned, err := cloneJSONValue(template.Settings[key])
		if err != nil {
			return nil, fmt.Errorf("%w: copy Template setting %q", templatedomain.ErrInvalidTemplate, key)
		}
		settings[key] = cloned
	}
	for _, binding := range template.Bindings {
		rule, exists := definition.TemplateSettingRules[binding.Target]
		if !exists || !rule.AllowResourceBinding {
			return nil, fmt.Errorf("%w: Definition does not allow binding setting %q", templatedomain.ErrInvalidTemplate, binding.Target)
		}
		value, err := resolveBinding(binding.Source, resource)
		if err != nil {
			return nil, err
		}
		if err := definitions.ValidateTemplateSettingValue(binding.Target, value, rule); err != nil {
			return nil, err
		}
		if current, exists := settings[binding.Target]; exists {
			if sameJSONValue(current, value) {
				continue
			}
			if !rule.BindingOverridesTemplate {
				return nil, fmt.Errorf("%w: resource binding cannot replace fixed setting %q", ErrTemplateSettingsConflict, binding.Target)
			}
		}
		settings[binding.Target] = value
	}
	encoded, err := json.Marshal(settings)
	if err != nil || definitions.ContainsSecret(encoded) {
		return nil, fmt.Errorf("%w: resolved settings are invalid or secret-bearing", templatedomain.ErrInvalidTemplate)
	}
	return settings, nil
}

func resolveBinding(source templatedomain.BindingSource, resource matchingdomain.MatchResource) (any, error) {
	if source.Kind == templatedomain.SourceAttribute {
		value, ok := resource.Attributes[source.Attribute]
		if !ok || value == "" {
			return nil, fmt.Errorf("%w: resource attribute %q is absent", ErrTemplateValueUnavailable, source.Attribute)
		}
		return value, nil
	}
	var selected string
	for _, endpoint := range resource.Endpoints {
		if endpoint.Scope != source.EndpointScope || endpoint.Protocol != source.EndpointProtocol || endpoint.Provenance != source.EndpointProvenance || endpoint.Port == 0 {
			continue
		}
		candidate := net.JoinHostPort(endpoint.Address, fmt.Sprint(endpoint.Port))
		if selected != "" && selected != candidate {
			return nil, fmt.Errorf("%w: multiple endpoints match the declared source", ErrTemplateValueUnavailable)
		}
		selected = candidate
	}
	if selected == "" {
		return nil, fmt.Errorf("%w: no endpoint matches the declared source", ErrTemplateValueUnavailable)
	}
	return selected, nil
}

func cloneJSONValue(value any) (any, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var cloned any
	if err := json.Unmarshal(encoded, &cloned); err != nil {
		return nil, err
	}
	return cloned, nil
}

func sameJSONValue(left, right any) bool {
	leftBytes, leftErr := json.Marshal(left)
	rightBytes, rightErr := json.Marshal(right)
	return leftErr == nil && rightErr == nil && string(leftBytes) == string(rightBytes)
}
