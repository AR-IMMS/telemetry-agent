package domain

import (
	"encoding/json"
	"fmt"
	"math"
)

// TemplateSettingRule declares which non-secret profile settings templates may set or bind.
type TemplateSettingRule struct {
	Type                     string          `json:"type,omitempty"`
	Description              string          `json:"description,omitempty"`
	Required                 bool            `json:"required,omitempty"`
	Constraints              []string        `json:"constraints,omitempty"`
	Validate                 func(any) error `json:"-"`
	Default                  any             `json:"default,omitempty"`
	DefaultOverridable       bool            `json:"default_overridable,omitempty"`
	AllowResourceBinding     bool            `json:"allow_resource_binding,omitempty"`
	BindingOverridesTemplate bool            `json:"binding_overrides_template,omitempty"`
}

// ValidateTemplateSettingValue checks a candidate value against its registered profile rule.
func ValidateTemplateSettingValue(name string, value any, rule TemplateSettingRule) error {
	if rule.Type == "" || !templateSettingValueMatchesType(value, rule.Type) {
		return fmt.Errorf("%w: setting %q must be %s", ErrInvalidTemplate, name, rule.Type)
	}
	if rule.Validate != nil {
		if err := rule.Validate(value); err != nil {
			return fmt.Errorf("%w: setting %q: %v", ErrInvalidTemplate, name, err)
		}
	}
	return nil
}

func templateSettingValueMatchesType(value any, valueType string) bool {
	encoded, err := json.Marshal(value)
	if err != nil {
		return false
	}
	var normalized any
	if err := json.Unmarshal(encoded, &normalized); err != nil {
		return false
	}
	switch valueType {
	case "string":
		_, ok := normalized.(string)
		return ok
	case "integer":
		number, ok := normalized.(float64)
		return ok && math.Trunc(number) == number
	case "number":
		_, ok := normalized.(float64)
		return ok
	case "boolean":
		_, ok := normalized.(bool)
		return ok
	case "object":
		_, ok := normalized.(map[string]any)
		return ok
	case "array":
		_, ok := normalized.([]any)
		return ok
	default:
		return false
	}
}
