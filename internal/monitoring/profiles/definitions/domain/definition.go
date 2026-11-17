package domain

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	matchingdomain "github.com/ar-imms/telemetry-agent/internal/monitoring/matching/domain"
)

var (
	definitionIDPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)
	capabilityIDPattern = regexp.MustCompile(`^[a-z][a-z0-9._-]{0,127}$`)
	versionPattern      = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`)
	providerIDPattern   = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)
)

var (
	// ErrInvalidDefinition identifies a malformed shipped Monitoring definition.
	ErrInvalidDefinition = errors.New("invalid integration definition")
	// ErrUnknownDefinition identifies a definition version absent from the registry.
	ErrUnknownDefinition = errors.New("unknown integration definition")
	// ErrInvalidTemplate identifies an invalid template setting or binding.
	ErrInvalidTemplate = errors.New("invalid Integration Configuration Template")
	// ErrInvalidContribution identifies invalid runtime-neutral component intent.
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

// Contributor deterministically converts a validated instance into component intents.
type Contributor func(map[string]any) (Contribution, error)

// Definition describes one shipped monitoring definition version.
type Definition struct {
	ID, Version          string
	Validate             func(map[string]any) error
	Contribute           Contributor
	Matching             *matchingdomain.MatchingDefinition
	TemplateSettingRules map[string]TemplateSettingRule
}

// ValidateDefinition checks identifiers and required definition behavior.
func ValidateDefinition(d Definition) error {
	if !definitionIDPattern.MatchString(d.ID) || !versionPattern.MatchString(d.Version) || d.Validate == nil || d.Contribute == nil {
		return fmt.Errorf("%w: malformed ID/version or missing behavior", ErrInvalidDefinition)
	}
	if err := matchingdomain.ValidateDefinition(d.Matching); err != nil {
		return fmt.Errorf("%w: invalid matching metadata", ErrInvalidDefinition)
	}
	if err := ValidateTemplateSettingRules(d.TemplateSettingRules); err != nil {
		return fmt.Errorf("%w: invalid Template setting rules", ErrInvalidDefinition)
	}
	return nil
}

// ValidateTemplateSettingRules checks declared non-secret template settings and precedence.
func ValidateTemplateSettingRules(rules map[string]TemplateSettingRule) error {
	if len(rules) > 128 {
		return fmt.Errorf("too many Template setting rules")
	}
	for key, rule := range rules {
		if !matchingdomain.ValidFieldName(key) || matchingdomain.SensitiveMatchKey(key) {
			return fmt.Errorf("invalid or sensitive Template setting name")
		}
		if rule.Type != "string" && rule.Type != "integer" && rule.Type != "number" && rule.Type != "boolean" && rule.Type != "object" && rule.Type != "array" {
			return fmt.Errorf("invalid Template setting type for %q", key)
		}
		if strings.TrimSpace(rule.Description) == "" || len(rule.Description) > 512 || matchingdomain.ContainsControlText(rule.Description) || len(rule.Constraints) > 16 {
			return fmt.Errorf("invalid Template setting metadata for %q", key)
		}
		for _, constraint := range rule.Constraints {
			if strings.TrimSpace(constraint) == "" || len(constraint) > 512 || matchingdomain.ContainsControlText(constraint) {
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

// ContainsSecret reports whether JSON contains a key conventionally used for secret material.
func ContainsSecret(raw json.RawMessage) bool { return containsSecret(raw) }
