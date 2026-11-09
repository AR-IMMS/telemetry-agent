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
	Enabled                             bool
	Revision                            uint64
}

// Contributor deterministically converts a validated instance into component intents.
type Contributor func(IntegrationInstance) (Contribution, error)

// IntegrationDefinition describes a shipped integration version.
type IntegrationDefinition struct {
	ID, Version string
	Validate    func(map[string]any) error
	Contribute  Contributor
	Matching    *MatchingDefinition
}

// ValidateDefinition checks identifiers and required definition behavior.
func ValidateDefinition(d IntegrationDefinition) error {
	if !definitionIDPattern.MatchString(d.ID) || !versionPattern.MatchString(d.Version) || d.Validate == nil || d.Contribute == nil {
		return fmt.Errorf("%w: malformed ID/version or missing behavior", ErrInvalidDefinition)
	}
	if err := validateMatchingDefinition(d.Matching); err != nil {
		return fmt.Errorf("%w: invalid matching metadata", ErrInvalidDefinition)
	}
	return nil
}

// ValidateInstance validates identity, exact definition reference, configuration shape and secrets.
func ValidateInstance(i IntegrationInstance) error {
	if !definitionIDPattern.MatchString(i.ID) || !definitionIDPattern.MatchString(i.DefinitionID) || !versionPattern.MatchString(i.DefinitionVersion) || i.Configuration == nil {
		return fmt.Errorf("%w: malformed identity, version, or configuration", ErrInvalidInstance)
	}
	b, err := json.Marshal(i.Configuration)
	if err != nil || containsSecret(json.RawMessage(b)) {
		return fmt.Errorf("%w: invalid or secret-bearing configuration", ErrInvalidInstance)
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
