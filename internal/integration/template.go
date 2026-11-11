package integration

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"regexp"
	"sort"
)

var (
	// ErrInvalidTemplate identifies malformed or secret-bearing template input.
	ErrInvalidTemplate = errors.New("invalid Integration Configuration Template")
	// ErrTemplateSettingsConflict identifies incompatible fixed settings sources.
	ErrTemplateSettingsConflict = errors.New("Integration Template settings conflict")
	// ErrTemplateValueUnavailable identifies a binding source that is absent or ambiguous.
	ErrTemplateValueUnavailable = errors.New("Integration Template binding value unavailable")
)

var templateIDPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)
var templateAttributePattern = regexp.MustCompile(`^[a-z][a-z0-9_.-]{0,127}$`)

// TemplateOwner records the authority scope used for template identity.
type TemplateOwner string

const (
	// TemplateOwnerLocal identifies an operator-created standalone template.
	TemplateOwnerLocal TemplateOwner = "local"
	// TemplateOwnerCore reserves the authority value for a future Core contract.
	TemplateOwnerCore TemplateOwner = "core"
)

// TemplateStatus is the durable lifecycle state of one configuration template.
type TemplateStatus string

const (
	// TemplateEnabled authorizes automatic materialization for matching resources.
	TemplateEnabled TemplateStatus = "enabled"
	// TemplateDisabled pauses materialization while preserving the template identity.
	TemplateDisabled TemplateStatus = "disabled"
	// TemplateDeleted is a permanent tombstone that prevents ID reuse or reactivation.
	TemplateDeleted TemplateStatus = "deleted"
)

// TemplateSettingRule declares which values a template may override or bind.
type TemplateSettingRule struct {
	Default                  any  `json:"default,omitempty"`
	DefaultOverridable       bool `json:"default_overridable,omitempty"`
	AllowResourceBinding     bool `json:"allow_resource_binding,omitempty"`
	BindingOverridesTemplate bool `json:"binding_overrides_template,omitempty"`
}

// IntegrationTemplate is an operator-authorized reusable configuration for one Integration.
// Revision is mutable metadata and never contributes to generated instance identity.
type IntegrationTemplate struct {
	ID                string                   `json:"id"`
	DefinitionID      string                   `json:"definition_id"`
	DefinitionVersion string                   `json:"definition_version"`
	Owner             TemplateOwner            `json:"owner"`
	Revision          uint64                   `json:"revision"`
	Status            TemplateStatus           `json:"status"`
	Selector          *MatchSelector           `json:"selector,omitempty"`
	Settings          map[string]any           `json:"settings,omitempty"`
	Bindings          []TemplateSettingBinding `json:"bindings,omitempty"`
}

// TemplateBindingSourceKind identifies one supported resource evidence shape.
type TemplateBindingSourceKind string

const (
	// TemplateSourceAttribute reads one exact allowlisted normalized attribute.
	TemplateSourceAttribute TemplateBindingSourceKind = "attribute"
	// TemplateSourceEndpoint selects one endpoint by exact scope, protocol and provenance.
	TemplateSourceEndpoint TemplateBindingSourceKind = "endpoint"
)

// TemplateBindingSource selects one bounded field from a normalized resource.
type TemplateBindingSource struct {
	Kind               TemplateBindingSourceKind `json:"kind"`
	Attribute          string                    `json:"attribute,omitempty"`
	EndpointScope      string                    `json:"endpoint_scope,omitempty"`
	EndpointProtocol   string                    `json:"endpoint_protocol,omitempty"`
	EndpointProvenance string                    `json:"endpoint_provenance,omitempty"`
}

// TemplateSettingBinding assigns one selected resource value to a Definition setting.
type TemplateSettingBinding struct {
	Target string                `json:"target"`
	Source TemplateBindingSource `json:"source"`
}

// ValidateIntegrationTemplate checks template identity, state and secret-free settings.
func ValidateIntegrationTemplate(template IntegrationTemplate) error {
	if !templateIDPattern.MatchString(template.ID) || !definitionIDPattern.MatchString(template.DefinitionID) ||
		!versionPattern.MatchString(template.DefinitionVersion) || template.Revision == 0 ||
		!validTemplateOwner(template.Owner) || !validTemplateStatus(template.Status) {
		return fmt.Errorf("%w: malformed identity, revision, owner or status", ErrInvalidTemplate)
	}
	if template.Selector != nil {
		if err := validateMatchingDefinition(&MatchingDefinition{Selector: template.Selector}); err != nil {
			return fmt.Errorf("%w: invalid selector: %v", ErrInvalidTemplate, err)
		}
	}
	if len(template.Settings) > 128 || len(template.Bindings) > 64 {
		return fmt.Errorf("%w: settings or binding limits exceeded", ErrInvalidTemplate)
	}
	encoded, err := json.Marshal(template.Settings)
	if err != nil || containsSecret(encoded) {
		return fmt.Errorf("%w: invalid or secret-bearing settings", ErrInvalidTemplate)
	}
	seen := make(map[string]struct{}, len(template.Bindings))
	for _, binding := range template.Bindings {
		if !matchFieldPattern.MatchString(binding.Target) {
			return fmt.Errorf("%w: malformed target setting", ErrInvalidTemplate)
		}
		if _, duplicate := seen[binding.Target]; duplicate {
			return fmt.Errorf("%w: duplicate binding target", ErrInvalidTemplate)
		}
		seen[binding.Target] = struct{}{}
		if err := validateTemplateSource(binding.Source); err != nil {
			return err
		}
	}
	return nil
}

// CloneIntegrationTemplate returns a deep copy suitable for crossing module seams.
func CloneIntegrationTemplate(template IntegrationTemplate) (IntegrationTemplate, error) {
	encoded, err := json.Marshal(template)
	if err != nil {
		return IntegrationTemplate{}, fmt.Errorf("copy Integration Template: %w", err)
	}
	var cloned IntegrationTemplate
	if err := json.Unmarshal(encoded, &cloned); err != nil {
		return IntegrationTemplate{}, fmt.Errorf("decode copied Integration Template: %w", err)
	}
	return cloned, nil
}

// GeneratedInstanceID returns a deterministic ID for one owner/template/resource/Integration pairing.
func GeneratedInstanceID(owner TemplateOwner, templateID string, resource ResourceIdentity, integrationID string) (string, error) {
	if !validTemplateOwner(owner) || !templateIDPattern.MatchString(templateID) ||
		!definitionIDPattern.MatchString(integrationID) || validateResourceIdentity(resource) != nil {
		return "", fmt.Errorf("%w: malformed generated instance identity", ErrInvalidTemplate)
	}
	canonical, err := json.Marshal(struct {
		Owner         TemplateOwner    `json:"owner"`
		TemplateID    string           `json:"template_id"`
		Resource      ResourceIdentity `json:"resource"`
		IntegrationID string           `json:"integration_id"`
	}{owner, templateID, resource, integrationID})
	if err != nil {
		return "", fmt.Errorf("%w: encode generated instance identity: %v", ErrInvalidTemplate, err)
	}
	digest := sha256.Sum256(canonical)
	return "template-" + hex.EncodeToString(digest[:12]), nil
}

// ResolveTemplateSettings applies Definition defaults, explicit settings and declared resource bindings.
func ResolveTemplateSettings(template IntegrationTemplate, definition IntegrationDefinition, resource MatchResource) (map[string]any, error) {
	if err := ValidateIntegrationTemplate(template); err != nil {
		return nil, err
	}
	if definition.ID != template.DefinitionID || definition.Version != template.DefinitionVersion {
		return nil, fmt.Errorf("%w: Template requires exact Definition %s@%s", ErrInvalidTemplate, template.DefinitionID, template.DefinitionVersion)
	}
	if err := ValidateDefinition(definition); err != nil {
		return nil, err
	}
	settings := make(map[string]any, len(definition.TemplateSettingRules)+len(template.Settings))
	for key, rule := range definition.TemplateSettingRules {
		if rule.Default != nil {
			value, err := cloneJSONValue(rule.Default)
			if err != nil {
				return nil, fmt.Errorf("%w: copy Definition default for %q: %v", ErrInvalidTemplate, key, err)
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
		value, exists := definition.TemplateSettingRules[key]
		if !exists {
			return nil, fmt.Errorf("%w: Definition does not declare setting %q for templates", ErrInvalidTemplate, key)
		}
		if value.Default != nil && !value.DefaultOverridable {
			if !sameJSONValue(value.Default, template.Settings[key]) {
				return nil, fmt.Errorf("%w: Definition setting %q is fixed", ErrTemplateSettingsConflict, key)
			}
			continue
		}
		cloned, err := cloneJSONValue(template.Settings[key])
		if err != nil {
			return nil, fmt.Errorf("%w: copy Template setting %q", ErrInvalidTemplate, key)
		}
		settings[key] = cloned
	}
	for _, binding := range template.Bindings {
		rule, exists := definition.TemplateSettingRules[binding.Target]
		if !exists || !rule.AllowResourceBinding {
			return nil, fmt.Errorf("%w: Definition does not allow binding setting %q", ErrInvalidTemplate, binding.Target)
		}
		value, err := resolveTemplateSource(binding.Source, resource)
		if err != nil {
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
	if err != nil || containsSecret(encoded) {
		return nil, fmt.Errorf("%w: resolved settings are invalid or secret-bearing", ErrInvalidTemplate)
	}
	return settings, nil
}

func validateTemplateSource(source TemplateBindingSource) error {
	switch source.Kind {
	case TemplateSourceAttribute:
		if !templateAttributePattern.MatchString(source.Attribute) || sensitiveMatchKey(source.Attribute) ||
			source.EndpointScope != "" || source.EndpointProtocol != "" || source.EndpointProvenance != "" {
			return fmt.Errorf("%w: invalid attribute source", ErrInvalidTemplate)
		}
	case TemplateSourceEndpoint:
		if source.Attribute != "" || (source.EndpointScope != "host" && source.EndpointScope != "container") ||
			source.EndpointProtocol != "tcp" || source.EndpointProvenance == "" || len(source.EndpointProvenance) > 64 {
			return fmt.Errorf("%w: invalid endpoint source", ErrInvalidTemplate)
		}
	default:
		return fmt.Errorf("%w: unsupported binding source", ErrInvalidTemplate)
	}
	return nil
}

func resolveTemplateSource(source TemplateBindingSource, resource MatchResource) (any, error) {
	if err := validateTemplateSource(source); err != nil {
		return nil, err
	}
	if source.Kind == TemplateSourceAttribute {
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

func validTemplateOwner(owner TemplateOwner) bool {
	return owner == TemplateOwnerLocal || owner == TemplateOwnerCore
}

func validTemplateStatus(status TemplateStatus) bool {
	return status == TemplateEnabled || status == TemplateDisabled || status == TemplateDeleted
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

func validateResourceIdentity(identity ResourceIdentity) error {
	if !providerIDPattern.MatchString(identity.Provider) || !validBoundedText(identity.ProviderInstance, 128) ||
		!validBoundedText(identity.ResourceID, 256) {
		return fmt.Errorf("malformed provider-scoped resource identity")
	}
	return nil
}
