package domain

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	matchingdomain "github.com/ar-imms/telemetry-agent/internal/monitoring/matching/domain"
)

var templateAttributePattern = regexp.MustCompile(`^[a-z][a-z0-9_.-]{0,127}$`)

// TemplateInput is a narrow, read-only projection needed to evaluate one
// monitoring Template. Owner and revision are opaque provenance; Evaluate is a
// caller-supplied eligibility decision, not a Monitoring lifecycle policy.
type TemplateInput struct {
	ID                string
	Owner             string
	Revision          uint64
	Evaluate          bool
	DefinitionID      string
	DefinitionVersion string
	Selector          *matchingdomain.MatchSelector
	Settings          map[string]any
	Bindings          []TemplateSettingBinding
}

// Validate checks the structural invariants of a Monitoring Template input.
func (t TemplateInput) Validate() error {
	if strings.TrimSpace(t.ID) == "" || len(t.ID) > 128 || strings.TrimSpace(t.Owner) == "" || len(t.Owner) > 64 || t.Revision == 0 ||
		strings.TrimSpace(t.DefinitionID) == "" || strings.TrimSpace(t.DefinitionVersion) == "" {
		return fmt.Errorf("malformed Monitoring Template evaluation identity")
	}
	if t.Selector != nil {
		if err := matchingdomain.ValidateDefinition(&matchingdomain.MatchingDefinition{Selector: t.Selector}); err != nil {
			return fmt.Errorf("invalid Monitoring Template selector: %w", err)
		}
	}
	if len(t.Settings) > 128 || len(t.Bindings) > 64 {
		return fmt.Errorf("Monitoring Template settings or binding limits exceeded")
	}
	if _, err := json.Marshal(t.Settings); err != nil {
		return fmt.Errorf("encode Monitoring Template settings: %w", err)
	}
	seen := make(map[string]struct{}, len(t.Bindings))
	for _, binding := range t.Bindings {
		if !matchingdomain.ValidFieldName(binding.Target) {
			return fmt.Errorf("malformed Monitoring Template target setting")
		}
		if _, duplicate := seen[binding.Target]; duplicate {
			return fmt.Errorf("duplicate Monitoring Template binding target")
		}
		seen[binding.Target] = struct{}{}
		if err := binding.Source.Validate(); err != nil {
			return err
		}
	}
	return nil
}

// TemplateSettingBinding selects one normalized resource value for a profile setting.
type TemplateSettingBinding struct {
	Target string
	Source TemplateBindingSource
}

// TemplateBindingSourceKind identifies one supported normalized evidence source.
type TemplateBindingSourceKind string

const (
	// TemplateSourceAttribute selects one exact normalized resource attribute.
	TemplateSourceAttribute TemplateBindingSourceKind = "attribute"
	// TemplateSourceEndpoint selects endpoints by scope, protocol, and provenance.
	TemplateSourceEndpoint TemplateBindingSourceKind = "endpoint"
)

// TemplateBindingSource selects one bounded normalized resource field.
type TemplateBindingSource struct {
	Kind               TemplateBindingSourceKind
	Attribute          string
	EndpointScope      string
	EndpointProtocol   string
	EndpointProvenance string
}

// Validate checks that the source selects one supported non-sensitive evidence field.
func (s TemplateBindingSource) Validate() error {
	switch s.Kind {
	case TemplateSourceAttribute:
		if !templateAttributePattern.MatchString(s.Attribute) || matchingdomain.SensitiveMatchKey(s.Attribute) ||
			s.EndpointScope != "" || s.EndpointProtocol != "" || s.EndpointProvenance != "" {
			return fmt.Errorf("invalid Monitoring Template attribute source")
		}
	case TemplateSourceEndpoint:
		if s.Attribute != "" || (s.EndpointScope != "host" && s.EndpointScope != "container") ||
			s.EndpointProtocol != "tcp" || s.EndpointProvenance == "" || len(s.EndpointProvenance) > 64 {
			return fmt.Errorf("invalid Monitoring Template endpoint source")
		}
	default:
		return fmt.Errorf("unsupported Monitoring Template binding source")
	}
	return nil
}
