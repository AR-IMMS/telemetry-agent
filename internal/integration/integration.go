package integration

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

var (
	definitionIDPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)
	capabilityIDPattern = regexp.MustCompile(`^[a-z][a-z0-9._-]{0,127}$`)
	versionPattern      = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`)
	providerIDPattern   = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)
)

var (
	ErrInvalidDefinition   = errors.New("invalid integration definition")
	ErrInvalidInstance     = errors.New("invalid integration instance")
	ErrUnknownDefinition   = errors.New("unknown integration definition")
	ErrInvalidContribution = errors.New("invalid integration contribution")
)

// ComponentIntent describes one required runtime capability without prescribing
// how a Collector implements or activates it.
type ComponentIntent struct {
	capability string
	config     []byte
}

// NewComponentIntent validates and copies JSON configuration into an immutable intent.
func NewComponentIntent(capability string, config any) (ComponentIntent, error) {
	capability = strings.TrimSpace(capability)
	if !capabilityIDPattern.MatchString(capability) {
		return ComponentIntent{}, fmt.Errorf("%w: malformed capability ID", ErrInvalidContribution)
	}
	b, err := json.Marshal(config)
	if err != nil {
		return ComponentIntent{}, fmt.Errorf("%w: encode configuration: %v", ErrInvalidContribution, err)
	}
	if containsSecret(json.RawMessage(b)) {
		return ComponentIntent{}, fmt.Errorf("%w: secret-bearing configuration", ErrInvalidContribution)
	}
	return ComponentIntent{capability: capability, config: bytes.Clone(b)}, nil
}

// Capability returns the runtime-neutral capability identifier.
func (i ComponentIntent) Capability() string { return i.capability }

// Configuration returns a copy of the intent configuration JSON.
func (i ComponentIntent) Configuration() json.RawMessage { return bytes.Clone(i.config) }

// Contribution is an immutable set of component intents.
type Contribution struct{ intents []ComponentIntent }

// NewContribution copies component intents into a contribution.
func NewContribution(intents []ComponentIntent) Contribution {
	return Contribution{intents: append([]ComponentIntent(nil), intents...)}
}

// Intents returns copied intents and configuration bytes.
func (c Contribution) Intents() []ComponentIntent {
	out := make([]ComponentIntent, len(c.intents))
	for i, intent := range c.intents {
		out[i] = ComponentIntent{capability: intent.capability, config: bytes.Clone(intent.config)}
	}
	return out
}

// IntegrationInstance is desired configuration pinned to an exact definition version.
type IntegrationInstance struct {
	ID, DefinitionID, DefinitionVersion string
	Configuration                       map[string]any
	Source                              DesiredSource    `json:"source,omitempty"`
	Binding                             *ResourceBinding `json:"resource_binding,omitempty"`
	TemplateOwner                       TemplateOwner    `json:"template_owner,omitempty"`
	TemplateID                          string           `json:"template_id,omitempty"`
	TemplateRevision                    uint64           `json:"template_revision,omitempty"`
	Enabled                             bool
	Revision                            uint64
}

// DesiredSource records how an Integration instance entered local desired state.
type DesiredSource string

const (
	// SourceLocal identifies an instance configured through a local Agent operation.
	SourceLocal DesiredSource = "local"
	// SourceDiscovery identifies an instance explicitly approved from resource evidence.
	SourceDiscovery DesiredSource = "discovery"
	// SourceTemplate identifies an instance generated under an enabled Integration Template.
	SourceTemplate DesiredSource = "template"
)

// ResourceBinding links an Integration instance to provider-scoped observed resource identity.
type ResourceBinding struct {
	Provider         string `json:"provider"`
	ProviderInstance string `json:"provider_instance"`
	ResourceID       string `json:"resource_id"`
}

// Contributor deterministically converts a validated instance into component intents.
type Contributor func(IntegrationInstance) (Contribution, error)

// IntegrationDefinition describes a shipped integration version.
type IntegrationDefinition struct {
	ID, Version          string
	Validate             func(map[string]any) error
	Contribute           Contributor
	Matching             *MatchingDefinition
	TemplateSettingRules map[string]TemplateSettingRule
}

// ValidateDefinition checks identifiers and required definition behavior.
func ValidateDefinition(d IntegrationDefinition) error {
	if !definitionIDPattern.MatchString(d.ID) || !versionPattern.MatchString(d.Version) || d.Validate == nil || d.Contribute == nil {
		return fmt.Errorf("%w: malformed ID/version or missing behavior", ErrInvalidDefinition)
	}
	if err := validateMatchingDefinition(d.Matching); err != nil {
		return fmt.Errorf("%w: invalid matching metadata", ErrInvalidDefinition)
	}
	if err := validateTemplateSettingRules(d.TemplateSettingRules); err != nil {
		return fmt.Errorf("%w: invalid Template setting rules", ErrInvalidDefinition)
	}
	return nil
}

// ValidateInstance validates identity, exact definition reference, configuration shape and secrets.
func ValidateInstance(i IntegrationInstance) error {
	if !definitionIDPattern.MatchString(i.ID) || !definitionIDPattern.MatchString(i.DefinitionID) || !versionPattern.MatchString(i.DefinitionVersion) || i.Configuration == nil {
		return fmt.Errorf("%w: malformed identity, version, or configuration", ErrInvalidInstance)
	}
	if i.Source != "" && i.Source != SourceLocal && i.Source != SourceDiscovery && i.Source != SourceTemplate {
		return fmt.Errorf("%w: unknown desired source", ErrInvalidInstance)
	}
	if i.Source == SourceDiscovery && i.Binding == nil {
		return fmt.Errorf("%w: discovery source requires a resource binding", ErrInvalidInstance)
	}
	if i.Source == SourceTemplate {
		if i.Binding == nil || !validTemplateOwner(i.TemplateOwner) || !templateIDPattern.MatchString(i.TemplateID) || i.TemplateRevision == 0 {
			return fmt.Errorf("%w: template source requires owner, template revision and resource binding", ErrInvalidInstance)
		}
	} else if i.TemplateOwner != "" || i.TemplateID != "" || i.TemplateRevision != 0 {
		return fmt.Errorf("%w: non-template instance cannot carry template provenance", ErrInvalidInstance)
	}
	if i.Binding != nil && !validResourceBinding(*i.Binding) {
		return fmt.Errorf("%w: malformed resource binding", ErrInvalidInstance)
	}
	b, err := json.Marshal(i.Configuration)
	if err != nil || containsSecret(json.RawMessage(b)) {
		return fmt.Errorf("%w: invalid or secret-bearing configuration", ErrInvalidInstance)
	}
	return nil
}

func validateTemplateSettingRules(rules map[string]TemplateSettingRule) error {
	if len(rules) > 128 {
		return fmt.Errorf("too many Template setting rules")
	}
	for key, rule := range rules {
		if !matchFieldPattern.MatchString(key) || sensitiveMatchKey(key) {
			return fmt.Errorf("invalid or sensitive Template setting name")
		}
		if rule.Type != "string" && rule.Type != "integer" && rule.Type != "number" && rule.Type != "boolean" && rule.Type != "object" && rule.Type != "array" {
			return fmt.Errorf("invalid Template setting type for %q", key)
		}
		if strings.TrimSpace(rule.Description) == "" || len(rule.Description) > 512 || containsControlText(rule.Description) || len(rule.Constraints) > 16 {
			return fmt.Errorf("invalid Template setting metadata for %q", key)
		}
		for _, constraint := range rule.Constraints {
			if strings.TrimSpace(constraint) == "" || len(constraint) > 512 || containsControlText(constraint) {
				return fmt.Errorf("invalid Template setting constraint for %q", key)
			}
		}
		if rule.DefaultOverridable && rule.Default == nil || rule.BindingOverridesTemplate && !rule.AllowResourceBinding {
			return fmt.Errorf("inconsistent Template setting precedence for %q", key)
		}
		if rule.Default != nil {
			encoded, err := json.Marshal(rule.Default)
			if err != nil || containsSecret(json.RawMessage(encoded)) {
				return fmt.Errorf("invalid or secret-bearing default for %q", key)
			}
			if err := ValidateTemplateSettingValue(key, rule.Default, rule); err != nil {
				return fmt.Errorf("invalid Template setting default for %q: %w", key, err)
			}
		}
	}
	return nil
}

func validResourceBinding(binding ResourceBinding) bool {
	return providerIDPattern.MatchString(binding.Provider) &&
		validBoundedText(binding.ProviderInstance, 128) && validBoundedText(binding.ResourceID, 256)
}

func validBoundedText(value string, max int) bool {
	if value == "" || len(value) > max || strings.TrimSpace(value) != value {
		return false
	}
	for _, r := range value {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	return true
}

func containsSecret(raw json.RawMessage) bool {
	var value any
	if json.Unmarshal(raw, &value) != nil {
		return true
	}
	var walk func(any) bool
	walk = func(v any) bool {
		switch x := v.(type) {
		case map[string]any:
			for k, child := range x {
				key := strings.ToLower(strings.NewReplacer("_", "", "-", "", " ", "").Replace(k))
				if strings.Contains(key, "password") || strings.Contains(key, "secret") ||
					strings.Contains(key, "token") || strings.Contains(key, "credential") ||
					strings.Contains(key, "apikey") || strings.Contains(key, "authorization") ||
					strings.Contains(key, "privatekey") || walk(child) {
					return true
				}
			}
		case []any:
			for _, child := range x {
				if walk(child) {
					return true
				}
			}
		}
		return false
	}
	return walk(value)
}
