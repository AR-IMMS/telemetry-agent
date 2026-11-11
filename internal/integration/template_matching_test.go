package integration

import (
	"errors"
	"testing"
	"time"
)

func TestMatcherMaterializesSupportedTemplateCandidateDeterministically(t *testing.T) {
	matcher := templateTestMatcher(t, CapabilitySupported)
	template := integrationTestTemplate("redis-default", TemplateEnabled)
	snapshot := templateMatchSnapshot()
	first, err := matcher.MatchTemplates(snapshot, []IntegrationTemplate{template})
	if err != nil {
		t.Fatal(err)
	}
	second, err := matcher.MatchTemplates(snapshot, []IntegrationTemplate{template})
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Results) != 1 || !first.Results[0].Materializable || first.Results[0].Instance == nil {
		t.Fatalf("template result=%+v", first.Results)
	}
	got, again := first.Results[0], second.Results[0]
	if got.Instance.ID != again.Instance.ID || got.Candidate.Digest() != again.Candidate.Digest() {
		t.Fatalf("non-deterministic result: first=%+v second=%+v", got, again)
	}
	if got.Instance.Source != SourceTemplate || got.Instance.TemplateID != template.ID || got.Instance.TemplateRevision != template.Revision || got.Instance.Binding == nil {
		t.Fatalf("generated provenance incomplete: %+v", got.Instance)
	}
	if got.Instance.Configuration["endpoint"] != "127.0.0.1:6379" || got.Instance.Configuration["collection_interval"] != "10s" {
		t.Fatalf("resolved settings=%#v", got.Instance.Configuration)
	}
}

func TestMatcherDetectsGlobalTemplateConflictBeforeMaterialization(t *testing.T) {
	matcher := templateTestMatcher(t, CapabilitySupported)
	first := integrationTestTemplate("redis-one", TemplateEnabled)
	second := integrationTestTemplate("redis-two", TemplateEnabled)
	report, err := matcher.MatchTemplates(templateMatchSnapshot(), []IntegrationTemplate{second, first})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Results) != 2 {
		t.Fatalf("results=%d, want both conflicting templates", len(report.Results))
	}
	for _, result := range report.Results {
		if !result.Conflict || result.Materializable || result.Instance != nil || result.MatchStatus != MatchAmbiguous {
			t.Fatalf("conflict was not failed closed: %+v", result)
		}
	}
}

func TestMatcherSkipsDisabledTemplateAndFailsClosedOnUnsupportedCapability(t *testing.T) {
	t.Run("disabled", func(t *testing.T) {
		matcher := templateTestMatcher(t, CapabilitySupported)
		report, err := matcher.MatchTemplates(templateMatchSnapshot(), []IntegrationTemplate{integrationTestTemplate("redis-disabled", TemplateDisabled)})
		if err != nil || len(report.Results) != 0 {
			t.Fatalf("disabled results=%+v err=%v", report.Results, err)
		}
	})
	t.Run("unsupported", func(t *testing.T) {
		matcher := templateTestMatcher(t, CapabilityUnsupported)
		report, err := matcher.MatchTemplates(templateMatchSnapshot(), []IntegrationTemplate{integrationTestTemplate("redis-default", TemplateEnabled)})
		if err != nil || len(report.Results) != 1 {
			t.Fatalf("unsupported results=%+v err=%v", report.Results, err)
		}
		result := report.Results[0]
		if result.Materializable || result.Instance != nil || result.CapabilityStatus != CapabilityUnsupported {
			t.Fatalf("unsupported capability materialized: %+v", result)
		}
	})
}

func TestMatcherRejectsDuplicateTemplateIdentity(t *testing.T) {
	matcher := templateTestMatcher(t, CapabilitySupported)
	first := integrationTestTemplate("redis-default", TemplateEnabled)
	second := integrationTestTemplate("redis-default", TemplateDisabled)
	if _, err := matcher.MatchTemplates(templateMatchSnapshot(), []IntegrationTemplate{first, second}); !errors.Is(err, ErrInvalidTemplate) {
		t.Fatalf("duplicate template error=%v, want %v", err, ErrInvalidTemplate)
	}
}

func templateTestMatcher(t *testing.T, capability CapabilityStatus) Matcher {
	t.Helper()
	definition := IntegrationDefinition{
		ID: "redis", Version: "1.0.0", Validate: func(settings map[string]any) error {
			if settings["endpoint"] != "127.0.0.1:6379" {
				return errors.New("endpoint required")
			}
			return nil
		},
		Contribute: func(IntegrationInstance) (Contribution, error) { return NewContribution(nil), nil },
		Matching: &MatchingDefinition{
			Selector: &MatchSelector{ResourceKinds: []string{"container"}, LifecycleStates: []string{"running"}, ExactAttributes: map[string]string{"container.image.repository": "docker.io/library/redis"}},
			BuildCandidate: func(resource MatchResource) (CandidateBuild, error) {
				return CandidateBuild{Settings: map[string]any{"endpoint": "127.0.0.1:6379"}}, nil
			},
			RequiredCapabilities: []string{"otel.receiver.redis.metrics"},
		},
		TemplateSettingRules: map[string]TemplateSettingRule{
			"endpoint":            {Type: "string", Description: "Target endpoint", Required: true, AllowResourceBinding: true},
			"collection_interval": {Type: "string", Description: "Collection frequency", Default: "10s", DefaultOverridable: true},
		},
	}
	registry, err := NewRegistry(definition)
	if err != nil {
		t.Fatal(err)
	}
	return NewMatcher(registry, templateCapabilityAssessor{status: capability})
}

type templateCapabilityAssessor struct{ status CapabilityStatus }

func (a templateCapabilityAssessor) Assess(required []string) (CapabilityAssessment, error) {
	assessment := CapabilityAssessment{Status: a.status, Platform: "linux/amd64", ArtifactVersion: "0.160.0", ManifestVersion: "1", Required: append([]string(nil), required...)}
	if a.status == CapabilityUnsupported {
		assessment.Unsupported = append([]string(nil), required...)
	}
	return assessment, nil
}

func templateMatchSnapshot() MatchSnapshot {
	now := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	return MatchSnapshot{InventoryRevision: 4, AsOf: now, StaleAfter: time.Minute, Resources: []MatchResource{{
		Provider: "docker", ProviderInstance: "local", ResourceID: "container-a", Kind: "container", LifecycleState: "running", Presence: "present",
		Attributes: map[string]string{"container.image.repository": "docker.io/library/redis"},
		Endpoints:  []MatchEndpoint{{Address: "127.0.0.1", Protocol: "tcp", Scope: "host", Provenance: "published_port", Port: 6379}}, LastSeenAt: now,
	}}}
}

func integrationTestTemplate(id string, status TemplateStatus) IntegrationTemplate {
	return IntegrationTemplate{
		ID: id, DefinitionID: "redis", DefinitionVersion: "1.0.0", Owner: TemplateOwnerLocal,
		Revision: 1, Status: status, Settings: map[string]any{},
		Bindings: []TemplateSettingBinding{{Target: "endpoint", Source: TemplateBindingSource{
			Kind: TemplateSourceEndpoint, EndpointScope: "host", EndpointProtocol: "tcp", EndpointProvenance: "published_port",
		}}},
	}
}
