package integration

import (
	"github.com/ar-imms/telemetry-agent/internal/monitoring/matching/application"
)

// Matcher preserves the old call shape while delegating matching to Monitoring.
type Matcher struct {
	registry   Registry
	capability CapabilityAssessor
}

// NewMatcher creates a compatibility adapter over Monitoring's read-only matcher.
func NewMatcher(registry Registry, capability CapabilityAssessor) Matcher {
	return Matcher{registry: registry, capability: capability}
}

// Match evaluates one consistent Inventory projection through Monitoring's application capability.
func (m Matcher) Match(snapshot MatchSnapshot) (MatchReport, error) {
	return application.NewMatcher(m.registry, m.capability).Match(snapshot)
}

func validateAndCopyMatchSnapshot(snapshot MatchSnapshot) ([]MatchResource, error) {
	return application.ValidateAndCopyMatchSnapshot(snapshot)
}
func evaluateDefinition(definition IntegrationDefinition, resource MatchResource) (bool, []MatchEvidence, error) {
	return application.EvaluateDefinition(definition, resource)
}
func selectorMatches(selector MatchSelector, resource MatchResource) (bool, []MatchEvidence, error) {
	return application.SelectorMatches(selector, resource)
}
func cloneMatchResource(resource MatchResource) MatchResource {
	return application.CloneMatchResource(resource)
}
func resourceFingerprint(resource MatchResource) string {
	return application.ResourceFingerprint(resource)
}
func identityOf(resource MatchResource) ResourceIdentity { return application.IdentityOf(resource) }
func identitySortKey(identity ResourceIdentity) string   { return application.IdentitySortKey(identity) }
func normalizeRequiredInputs(inputs []RequiredInput) ([]RequiredInput, bool) {
	return application.NormalizeRequiredInputs(inputs)
}
func validEvidence(evidence []MatchEvidence) bool { return application.ValidEvidence(evidence) }
func sortEvidence(evidence []MatchEvidence)       { application.SortEvidence(evidence) }

func (m Matcher) assessCapabilities(required []string) CapabilityAssessment {
	return application.NewMatcher(m.registry, m.capability).AssessCapabilities(required)
}
