package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	desiredstate "github.com/ar-imms/telemetry-agent/internal/configuration/desiredstate/domain"
)

var (
	// ErrMaterializationConflict indicates that a snapshot CAS token became stale.
	ErrMaterializationConflict = errors.New("template materialization snapshot conflict")
	// ErrInvalidMaterialization identifies an incomplete or inconsistent projection.
	ErrInvalidMaterialization = errors.New("invalid template materialization")
	templateIDPattern         = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)
	definitionIDPattern       = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)
	providerIDPattern         = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)
)

// Batch contains only generated upserts and suspensions; it never deletes provenance.
type Batch struct {
	ExpectedInventoryWriteRevision uint64
	ExpectedTemplateSetRevision    uint64
	ExpectedDesiredSetRevision     uint64
	Upsert                         []desiredstate.Instance
	Suspend                        []string
}

// Diagnostic is a bounded, secret-free explanation for one skipped candidate.
type Diagnostic struct {
	TemplateID string
	Resource   desiredstate.ResourceBinding
	Code       string
}

// Plan combines a revision-bound desired projection with non-durable diagnostics.
type Plan struct {
	Batch       Batch
	Diagnostics []Diagnostic
}

// GeneratedInstanceID returns the stable ID used by existing persisted template instances.
func GeneratedInstanceID(owner desiredstate.TemplateOwner, templateID string, resource desiredstate.ResourceBinding, definitionID string) (string, error) {
	if (owner != desiredstate.TemplateOwnerLocal && owner != desiredstate.TemplateOwnerCore) ||
		!templateIDPattern.MatchString(templateID) || !definitionIDPattern.MatchString(definitionID) || !validResourceIdentity(resource) {
		return "", fmt.Errorf("%w: malformed generated instance identity", ErrInvalidMaterialization)
	}
	canonical, err := json.Marshal(struct {
		Owner         desiredstate.TemplateOwner                              `json:"owner"`
		TemplateID    string                                                  `json:"template_id"`
		Resource      struct{ Provider, ProviderInstance, ResourceID string } `json:"resource"`
		IntegrationID string                                                  `json:"integration_id"`
	}{owner, templateID, struct{ Provider, ProviderInstance, ResourceID string }{
		Provider: resource.Provider, ProviderInstance: resource.ProviderInstance, ResourceID: resource.ResourceID,
	}, definitionID})
	if err != nil {
		return "", fmt.Errorf("%w: encode generated instance identity: %v", ErrInvalidMaterialization, err)
	}
	digest := sha256.Sum256(canonical)
	return "template-" + hex.EncodeToString(digest[:12]), nil
}

// ValidateBatch checks revisions, generated ownership, deterministic identity and uniqueness.
func ValidateBatch(batch Batch) error {
	maxRevision := uint64(^uint64(0) >> 1)
	if batch.ExpectedInventoryWriteRevision == 0 || batch.ExpectedInventoryWriteRevision > maxRevision ||
		batch.ExpectedTemplateSetRevision > maxRevision || batch.ExpectedDesiredSetRevision > maxRevision {
		return ErrInvalidMaterialization
	}
	seen := make(map[string]struct{}, len(batch.Upsert)+len(batch.Suspend))
	for _, instance := range batch.Upsert {
		if instance.Source != desiredstate.SourceTemplate || !instance.Enabled || instance.Revision != 0 || instance.Validate() != nil {
			return ErrInvalidMaterialization
		}
		identity, err := GeneratedInstanceID(instance.TemplateOwner, instance.TemplateID, *instance.Binding, instance.DefinitionID)
		if err != nil || identity != instance.ID {
			return ErrInvalidMaterialization
		}
		if _, duplicate := seen[instance.ID]; duplicate {
			return ErrInvalidMaterialization
		}
		seen[instance.ID] = struct{}{}
	}
	for _, id := range batch.Suspend {
		if id == "" {
			return ErrInvalidMaterialization
		}
		if _, duplicate := seen[id]; duplicate {
			return ErrInvalidMaterialization
		}
		seen[id] = struct{}{}
	}
	return nil
}

func validResourceIdentity(resource desiredstate.ResourceBinding) bool {
	return providerIDPattern.MatchString(resource.Provider) && validBoundedText(resource.ProviderInstance, 128) && validBoundedText(resource.ResourceID, 256)
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
