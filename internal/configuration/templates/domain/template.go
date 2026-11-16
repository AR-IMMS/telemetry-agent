package domain

import (
	"encoding/json"
	"fmt"
	"regexp"

	configurationdomain "github.com/ar-imms/telemetry-agent/internal/configuration/desiredstate/domain"
	matchingdomain "github.com/ar-imms/telemetry-agent/internal/monitoring/matching/domain"
	"github.com/ar-imms/telemetry-agent/internal/monitoring/profiles/definitions"
)

var (
	// ErrInvalidTemplate identifies malformed or secret-bearing Template state.
	ErrInvalidTemplate = definitions.ErrInvalidTemplate
	templateIDPattern  = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)
	attributePattern   = regexp.MustCompile(`^[a-z][a-z0-9_.-]{0,127}$`)
)

// Status is the durable lifecycle state of one Configuration Template.
type Status string

const (
	// StatusEnabled permits Template matching and materialization.
	StatusEnabled Status = "enabled"
	// StatusDisabled pauses materialization without releasing Template identity.
	StatusDisabled Status = "disabled"
	// StatusDeleted permanently tombstones the Template ID.
	StatusDeleted Status = "deleted"
)

// BindingSourceKind selects one supported normalized resource-evidence shape.
type BindingSourceKind string

const (
	// SourceAttribute reads one exact allowlisted normalized attribute.
	SourceAttribute BindingSourceKind = "attribute"
	// SourceEndpoint selects an endpoint by exact scope, protocol and provenance.
	SourceEndpoint BindingSourceKind = "endpoint"
)

// BindingSource selects one bounded field from normalized resource evidence.
type BindingSource struct {
	Kind               BindingSourceKind `json:"kind"`
	Attribute          string            `json:"attribute,omitempty"`
	EndpointScope      string            `json:"endpoint_scope,omitempty"`
	EndpointProtocol   string            `json:"endpoint_protocol,omitempty"`
	EndpointProvenance string            `json:"endpoint_provenance,omitempty"`
}

// Validate checks that the binding reads only a supported, non-sensitive resource field.
func (s BindingSource) Validate() error {
	switch s.Kind {
	case SourceAttribute:
		if !attributePattern.MatchString(s.Attribute) || matchingdomain.SensitiveMatchKey(s.Attribute) ||
			s.EndpointScope != "" || s.EndpointProtocol != "" || s.EndpointProvenance != "" {
			return fmt.Errorf("%w: invalid attribute source", ErrInvalidTemplate)
		}
	case SourceEndpoint:
		if s.Attribute != "" || (s.EndpointScope != "host" && s.EndpointScope != "container") ||
			s.EndpointProtocol != "tcp" || s.EndpointProvenance == "" || len(s.EndpointProvenance) > 64 {
			return fmt.Errorf("%w: invalid endpoint source", ErrInvalidTemplate)
		}
	default:
		return fmt.Errorf("%w: unsupported binding source", ErrInvalidTemplate)
	}
	return nil
}

// SettingBinding assigns one selected resource value to a profile setting.
type SettingBinding struct {
	Target string        `json:"target"`
	Source BindingSource `json:"source"`
}

// Template is an operator-authorized reusable configuration for one exact profile version.
// Field names and JSON tags preserve the existing SQLite representation.
type Template struct {
	ID                string                            `json:"id"`
	DefinitionID      string                            `json:"definition_id"`
	DefinitionVersion string                            `json:"definition_version"`
	Owner             configurationdomain.TemplateOwner `json:"owner"`
	Revision          uint64                            `json:"revision"`
	Status            Status                            `json:"status"`
	Selector          *matchingdomain.MatchSelector     `json:"selector,omitempty"`
	Settings          map[string]any                    `json:"settings,omitempty"`
	Bindings          []SettingBinding                  `json:"bindings,omitempty"`
}

// Validate checks Template identity, lifecycle state, selector and secret-free settings.
func (t Template) Validate() error {
	identity := configurationdomain.Instance{
		ID: t.ID, DefinitionID: t.DefinitionID, DefinitionVersion: t.DefinitionVersion,
		Configuration: map[string]any{},
	}
	if err := identity.Validate(); err != nil || t.Revision == 0 || !validOwner(t.Owner) || !validStatus(t.Status) {
		return fmt.Errorf("%w: malformed identity, revision, owner or status", ErrInvalidTemplate)
	}
	if t.Selector != nil {
		if err := matchingdomain.ValidateDefinition(&matchingdomain.MatchingDefinition{Selector: t.Selector}); err != nil {
			return fmt.Errorf("%w: invalid selector: %v", ErrInvalidTemplate, err)
		}
	}
	if len(t.Settings) > 128 || len(t.Bindings) > 64 {
		return fmt.Errorf("%w: settings or binding limits exceeded", ErrInvalidTemplate)
	}
	encoded, err := json.Marshal(t.Settings)
	if err != nil || definitions.ContainsSecret(encoded) {
		return fmt.Errorf("%w: invalid or secret-bearing settings", ErrInvalidTemplate)
	}
	seen := make(map[string]struct{}, len(t.Bindings))
	for _, binding := range t.Bindings {
		if !matchingdomain.ValidFieldName(binding.Target) {
			return fmt.Errorf("%w: malformed target setting", ErrInvalidTemplate)
		}
		if _, duplicate := seen[binding.Target]; duplicate {
			return fmt.Errorf("%w: duplicate binding target", ErrInvalidTemplate)
		}
		seen[binding.Target] = struct{}{}
		if err := binding.Source.Validate(); err != nil {
			return err
		}
	}
	return nil
}

// CloneTemplate returns a deep copy suitable for crossing module seams.
func CloneTemplate(template Template) (Template, error) {
	encoded, err := json.Marshal(template)
	if err != nil {
		return Template{}, fmt.Errorf("copy Configuration Template: %w", err)
	}
	var cloned Template
	if err := json.Unmarshal(encoded, &cloned); err != nil {
		return Template{}, fmt.Errorf("decode copied Configuration Template: %w", err)
	}
	return cloned, nil
}

func validOwner(owner configurationdomain.TemplateOwner) bool {
	return owner == configurationdomain.TemplateOwnerLocal || owner == configurationdomain.TemplateOwnerCore
}

func validStatus(status Status) bool {
	return status == StatusEnabled || status == StatusDisabled || status == StatusDeleted
}
