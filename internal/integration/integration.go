package integration

import (
	"encoding/json"
	"regexp"
	"strings"

	"github.com/ar-imms/telemetry-agent/internal/configuration/desiredstate/domain"
	matchingdomain "github.com/ar-imms/telemetry-agent/internal/monitoring/matching/domain"
	"github.com/ar-imms/telemetry-agent/internal/monitoring/profiles/definitions"
)

var (
	definitionIDPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)
	versionPattern      = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`)
	providerIDPattern   = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)
)

var (
	ErrInvalidDefinition   = definitions.ErrInvalidDefinition
	ErrInvalidInstance     = domain.ErrInvalidInstance
	ErrUnknownDefinition   = definitions.ErrUnknownDefinition
	ErrInvalidContribution = definitions.ErrInvalidContribution
	ErrInvalidTemplate     = definitions.ErrInvalidTemplate
)

// ComponentIntent is a source-compatibility alias for Monitoring's runtime-neutral intent.
type ComponentIntent = definitions.ComponentIntent

// Contribution is a source-compatibility alias for Monitoring's immutable contribution.
type Contribution = definitions.Contribution

// Contributor is a source-compatibility alias for Monitoring's settings contributor.
type Contributor = definitions.Contributor

// IntegrationInstance is retained as a source-compatibility alias while callers migrate to Configuration.
type IntegrationInstance = domain.Instance

// DesiredSource is retained as a source-compatibility alias while callers migrate to Configuration.
type DesiredSource = domain.Source

const (
	SourceLocal     = domain.SourceLocal
	SourceDiscovery = domain.SourceDiscovery
	SourceTemplate  = domain.SourceTemplate
)

// ResourceBinding is retained as a source-compatibility alias while callers migrate to Configuration.
type ResourceBinding = domain.ResourceBinding

// IntegrationDefinition is retained as a source-compatibility alias for Monitoring's shipped definition.
type IntegrationDefinition = definitions.Definition

// NewComponentIntent forwards construction to Monitoring's immutable intent model.
func NewComponentIntent(capability string, config any) (ComponentIntent, error) {
	return definitions.NewComponentIntent(capability, config)
}

// NewContribution forwards immutable contribution construction to Monitoring.
func NewContribution(intents []ComponentIntent) Contribution {
	return definitions.NewContribution(intents)
}

// ValidateDefinition forwards shipped-definition validation to Monitoring.
func ValidateDefinition(definition IntegrationDefinition) error {
	return definitions.ValidateDefinition(definition)
}

// ValidateInstance forwards validation to Configuration's desired-state model.
func ValidateInstance(instance IntegrationInstance) error { return instance.Validate() }

func validResourceBinding(binding ResourceBinding) bool {
	return providerIDPattern.MatchString(binding.Provider) && validBoundedText(binding.ProviderInstance, 128) && validBoundedText(binding.ResourceID, 256)
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

func containsSecret(raw json.RawMessage) bool { return definitions.ContainsSecret(raw) }
func containsControlText(value string) bool   { return matchingdomain.ContainsControlText(value) }
func sensitiveMatchKey(value string) bool     { return matchingdomain.SensitiveMatchKey(value) }
func validateMatchingDefinition(value *MatchingDefinition) error {
	return matchingdomain.ValidateDefinition(value)
}
