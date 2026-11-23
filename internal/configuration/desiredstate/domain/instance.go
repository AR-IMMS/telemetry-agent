package domain

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

var (
	definitionIDPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)
	versionPattern      = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`)
	providerIDPattern   = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)
	templateIDPattern   = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)

	// ErrInvalidInstance identifies malformed or secret-bearing desired state.
	ErrInvalidInstance = errors.New("invalid integration instance")
)

// Source records how a desired Integration entered local state.
type Source string

const (
	// SourceLocal identifies an instance configured through a local Agent operation.
	SourceLocal Source = "local"
	// SourceDiscovery identifies an instance explicitly approved from resource evidence.
	SourceDiscovery Source = "discovery"
	// SourceTemplate identifies an instance generated under an enabled Template.
	SourceTemplate Source = "template"
)

// TemplateOwner records the authority scope for template-generated identity.
type TemplateOwner string

const (
	// TemplateOwnerLocal identifies an operator-created standalone Template.
	TemplateOwnerLocal TemplateOwner = "local"
	// TemplateOwnerCore reserves the authority value for the existing Core contract.
	TemplateOwnerCore TemplateOwner = "core"
)

// ResourceBinding links a desired instance to provider-scoped observed identity.
type ResourceBinding struct {
	Provider         string `json:"provider"`
	ProviderInstance string `json:"provider_instance"`
	ResourceID       string `json:"resource_id"`
}

// Instance is version-pinned desired configuration and its persisted lifecycle metadata.
// Field names and JSON tags intentionally preserve the existing SQLite representation.
type Instance struct {
	ID, DefinitionID, DefinitionVersion string
	Configuration                       map[string]any
	Source                              Source           `json:"source,omitempty"`
	Binding                             *ResourceBinding `json:"resource_binding,omitempty"`
	TemplateOwner                       TemplateOwner    `json:"template_owner,omitempty"`
	TemplateID                          string           `json:"template_id,omitempty"`
	TemplateRevision                    uint64           `json:"template_revision,omitempty"`
	Enabled                             bool
	Revision                            uint64
}

// Validate checks desired-state identity, provenance, binding and secret-free settings.
func (i Instance) Validate() error {
	if !definitionIDPattern.MatchString(i.ID) || !definitionIDPattern.MatchString(i.DefinitionID) ||
		!versionPattern.MatchString(i.DefinitionVersion) || i.Configuration == nil {
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
	encoded, err := json.Marshal(i.Configuration)
	if err != nil || containsSecret(json.RawMessage(encoded)) {
		return fmt.Errorf("%w: invalid or secret-bearing configuration", ErrInvalidInstance)
	}
	return nil
}

func validTemplateOwner(owner TemplateOwner) bool {
	return owner == TemplateOwnerLocal || owner == TemplateOwnerCore
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
			for key, child := range x {
				normalized := strings.ToLower(strings.NewReplacer("_", "", "-", "", " ", "").Replace(key))
				if strings.Contains(normalized, "password") || strings.Contains(normalized, "secret") ||
					strings.Contains(normalized, "token") || strings.Contains(normalized, "credential") ||
					strings.Contains(normalized, "apikey") || strings.Contains(normalized, "authorization") ||
					strings.Contains(normalized, "privatekey") || walk(child) {
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
