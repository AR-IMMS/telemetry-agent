package integration

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"

	"github.com/ar-imms/telemetry-agent/internal/configuration/desiredstate/domain"
	templatedomain "github.com/ar-imms/telemetry-agent/internal/configuration/templates/domain"
	"github.com/ar-imms/telemetry-agent/internal/monitoring/profiles/definitions"
	templatesapp "github.com/ar-imms/telemetry-agent/internal/monitoring/templates/application"
)

var (

	// ErrTemplateSettingsConflict preserves the historical settings-conflict error.
	ErrTemplateSettingsConflict = templatesapp.ErrTemplateSettingsConflict
	// ErrTemplateValueUnavailable preserves the historical binding-evidence error.
	ErrTemplateValueUnavailable = templatesapp.ErrTemplateValueUnavailable
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
	input, err := monitoringTemplateInput(template)
	if err != nil {
		return nil, err
	}
	return templatesapp.ResolveSettings(input, definition, resource)
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
