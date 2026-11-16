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

	"github.com/ar-imms/telemetry-agent/internal/configuration/desiredstate/domain"
	templatedomain "github.com/ar-imms/telemetry-agent/internal/configuration/templates/domain"
	"github.com/ar-imms/telemetry-agent/internal/monitoring/profiles/definitions"
)

var (

	// ErrTemplateSettingsConflict identifies incompatible fixed settings sources.
	ErrTemplateSettingsConflict = errors.New("Integration Template settings conflict")
	// ErrTemplateValueUnavailable identifies a binding source that is absent or ambiguous.
	ErrTemplateValueUnavailable = errors.New("Integration Template binding value unavailable")
)

var templateIDPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)

// TemplateOwner aliases Configuration provenance during the model ownership migration.
type TemplateOwner = domain.TemplateOwner

const (
	// TemplateOwnerLocal identifies an operator-created standalone Template.
	TemplateOwnerLocal = domain.TemplateOwnerLocal
	// TemplateOwnerCore reserves the authority value for the existing Core contract.
	TemplateOwnerCore = domain.TemplateOwnerCore
)

// TemplateStatus preserves the historical name for Configuration's Template lifecycle state.
type TemplateStatus = templatedomain.Status

const (
	// TemplateEnabled authorizes automatic materialization for matching resources.
	TemplateEnabled = templatedomain.StatusEnabled
	// TemplateDisabled pauses materialization while preserving the template identity.
	TemplateDisabled = templatedomain.StatusDisabled
	// TemplateDeleted is a permanent tombstone that prevents ID reuse or reactivation.
	TemplateDeleted = templatedomain.StatusDeleted
)

// TemplateBindingSourceKind preserves the historical source-kind name.
type TemplateBindingSourceKind = templatedomain.BindingSourceKind

const (
	// TemplateSourceAttribute reads one exact allowlisted normalized attribute.
	TemplateSourceAttribute = templatedomain.SourceAttribute
	// TemplateSourceEndpoint selects an endpoint by exact scope, protocol and provenance.
	TemplateSourceEndpoint = templatedomain.SourceEndpoint
)

// TemplateBindingSource preserves the historical source declaration name.
type TemplateBindingSource = templatedomain.BindingSource

// TemplateSettingBinding preserves the historical settings-binding name.
type TemplateSettingBinding = templatedomain.SettingBinding

// IntegrationTemplate is retained as a source-compatible alias for Configuration's Template.
type IntegrationTemplate = templatedomain.Template

// TemplateSettingRule is retained as a compatibility alias for Monitoring's profile schema.
type TemplateSettingRule = definitions.TemplateSettingRule

// ValidateTemplateSettingValue forwards profile setting validation to Monitoring.
func ValidateTemplateSettingValue(name string, value any, rule TemplateSettingRule) error {
	return definitions.ValidateTemplateSettingValue(name, value, rule)
}

// ValidateIntegrationTemplate delegates validation to Configuration's Template domain.
func ValidateIntegrationTemplate(template IntegrationTemplate) error { return template.Validate() }

// CloneIntegrationTemplate returns a deep copy suitable for crossing module seams.
func CloneIntegrationTemplate(template IntegrationTemplate) (IntegrationTemplate, error) {
	return templatedomain.CloneTemplate(template)
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
			if err := ValidateTemplateSettingValue(key, value, rule); err != nil {
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
		value, exists := definition.TemplateSettingRules[key]
		if !exists {
			return nil, fmt.Errorf("%w: Definition does not declare setting %q for templates", ErrInvalidTemplate, key)
		}
		if err := ValidateTemplateSettingValue(key, template.Settings[key], value); err != nil {
			return nil, err
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
		if err := ValidateTemplateSettingValue(binding.Target, value, rule); err != nil {
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
	return source.Validate()
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
