package application

// ValidateAndCopyMatchSnapshot returns a validated immutable copy of one Inventory projection.
func ValidateAndCopyMatchSnapshot(snapshot MatchSnapshot) ([]MatchResource, error) {
	return validateAndCopyMatchSnapshot(snapshot)
}

// EvaluateDefinition evaluates one definition's selector or pure matcher against a resource.
func EvaluateDefinition(definition IntegrationDefinition, resource MatchResource) (bool, []MatchEvidence, error) {
	return evaluateDefinition(definition, resource)
}

// SelectorMatches evaluates a declarative selector against one resource.
func SelectorMatches(selector MatchSelector, resource MatchResource) (bool, []MatchEvidence, error) {
	return selectorMatches(selector, resource)
}

// CloneMatchResource returns a defensive copy of matching input.
func CloneMatchResource(resource MatchResource) MatchResource { return cloneMatchResource(resource) }

// ResourceFingerprint returns the deterministic fingerprint for normalized resource evidence.
func ResourceFingerprint(resource MatchResource) string { return resourceFingerprint(resource) }

// IdentityOf returns provider-scoped identity for a match resource.
func IdentityOf(resource MatchResource) ResourceIdentity { return identityOf(resource) }

// IdentitySortKey returns the deterministic sort key for a provider-scoped identity.
func IdentitySortKey(identity ResourceIdentity) string { return identitySortKey(identity) }

// NormalizeRequiredInputs validates and orders non-secret required setting names.
func NormalizeRequiredInputs(inputs []RequiredInput) ([]RequiredInput, bool) {
	return normalizeRequiredInputs(inputs)
}

// ValidEvidence validates that matching evidence is bounded and free of secret fields.
func ValidEvidence(evidence []MatchEvidence) bool { return validEvidence(evidence) }

// SortEvidence orders matching evidence deterministically.
func SortEvidence(evidence []MatchEvidence) { sortEvidence(evidence) }

// AssessCapabilities evaluates required receiver IDs using the injected Runtime catalog query.
func (m Matcher) AssessCapabilities(required []string) CapabilityAssessment {
	return m.assessCapabilities(required)
}
