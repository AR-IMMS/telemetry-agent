package application

import (
	"errors"
	"testing"
	"time"

	matchingdomain "github.com/ar-imms/telemetry-agent/internal/monitoring/matching/domain"
	definitionapp "github.com/ar-imms/telemetry-agent/internal/monitoring/profiles/definitions/application"
	definitions "github.com/ar-imms/telemetry-agent/internal/monitoring/profiles/definitions/domain"
	templatesdomain "github.com/ar-imms/telemetry-agent/internal/monitoring/templates/domain"
)

func TestEvaluatorReturnsDeterministicConfigurationNeutralResults(t *testing.T) {
	evaluator := newTemplateEvaluator(t, matchingdomain.CapabilitySupported)
	template := evaluationTemplate("redis-default", true)
	snapshot := evaluationSnapshot()

	first, err := evaluator.MatchTemplates(snapshot, []templatesdomain.TemplateInput{template})
	if err != nil {
		t.Fatal(err)
	}
	second, err := evaluator.MatchTemplates(snapshot, []templatesdomain.TemplateInput{template})
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Results) != 1 || !first.Results[0].Materializable {
		t.Fatalf("template evaluation result=%+v", first.Results)
	}
	got, again := first.Results[0], second.Results[0]
	if got.Candidate == nil || got.Candidate.Digest() != again.Candidate.Digest() {
		t.Fatalf("candidate result is missing or unstable: first=%+v second=%+v", got, again)
	}
	if got.Candidate.Configuration() == nil || got.TemplateID != template.ID || got.TemplateRevision != template.Revision {
		t.Fatalf("configuration-neutral template result lost candidate/provenance: %+v", got)
	}
}

func TestEvaluatorMarksConflictsAndFailsClosedOnUnsupportedCapability(t *testing.T) {
	t.Run("conflict", func(t *testing.T) {
		evaluator := newTemplateEvaluator(t, matchingdomain.CapabilitySupported)
		report, err := evaluator.MatchTemplates(evaluationSnapshot(), []templatesdomain.TemplateInput{
			evaluationTemplate("redis-one", true),
			evaluationTemplate("redis-two", true),
		})
		if err != nil || len(report.Results) != 2 {
			t.Fatalf("results=%+v err=%v", report.Results, err)
		}
		for _, result := range report.Results {
			if !result.Conflict || result.Materializable || result.MatchStatus != matchingdomain.MatchAmbiguous {
				t.Fatalf("conflict was not failed closed: %+v", result)
			}
		}
	})

	t.Run("unsupported capability", func(t *testing.T) {
		evaluator := newTemplateEvaluator(t, matchingdomain.CapabilityUnsupported)
		report, err := evaluator.MatchTemplates(evaluationSnapshot(), []templatesdomain.TemplateInput{
			evaluationTemplate("redis-default", true),
		})
		if err != nil || len(report.Results) != 1 {
			t.Fatalf("results=%+v err=%v", report.Results, err)
		}
		if report.Results[0].Materializable || report.Results[0].CapabilityStatus != matchingdomain.CapabilityUnsupported {
			t.Fatalf("unsupported capability was materializable: %+v", report.Results[0])
		}
	})
}

func TestEvaluatorSkipsDisabledAndRejectsDuplicateTemplateIdentity(t *testing.T) {
	evaluator := newTemplateEvaluator(t, matchingdomain.CapabilitySupported)
	if report, err := evaluator.MatchTemplates(evaluationSnapshot(), []templatesdomain.TemplateInput{
		evaluationTemplate("redis-disabled", false),
	}); err != nil || len(report.Results) != 0 {
		t.Fatalf("disabled report=%+v err=%v", report, err)
	}
	first := evaluationTemplate("redis-default", true)
	second := evaluationTemplate("redis-default", false)
	if _, err := evaluator.MatchTemplates(evaluationSnapshot(), []templatesdomain.TemplateInput{first, second}); !errors.Is(err, definitions.ErrInvalidTemplate) {
		t.Fatalf("duplicate template error=%v, want %v", err, definitions.ErrInvalidTemplate)
	}
}

func TestEvaluatorRejectsSecretBearingSettingsFromInputContract(t *testing.T) {
	evaluator := newTemplateEvaluator(t, matchingdomain.CapabilitySupported)
	template := evaluationTemplate("redis-default", true)
	template.Settings["password"] = "must-not-enter-a-candidate"
	if _, err := evaluator.MatchTemplates(evaluationSnapshot(), []templatesdomain.TemplateInput{template}); !errors.Is(err, definitions.ErrInvalidTemplate) {
		t.Fatalf("secret-bearing input error = %v, want %v", err, definitions.ErrInvalidTemplate)
	}
}

func newTemplateEvaluator(t *testing.T, capability matchingdomain.CapabilityStatus) Evaluator {
	t.Helper()
	definition := definitions.Definition{
		ID: "redis", Version: "1.0.0",
		Validate: func(settings map[string]any) error {
			if settings["endpoint"] != "127.0.0.1:6379" {
				return errors.New("endpoint required")
			}
			return nil
		},
		Contribute: func(map[string]any) (definitions.Contribution, error) {
			return definitions.NewContribution(nil), nil
		},
		Matching: &matchingdomain.MatchingDefinition{
			Selector: &matchingdomain.MatchSelector{
				ResourceKinds: []string{"container"}, LifecycleStates: []string{"running"},
				ExactAttributes: map[string]string{"container.image.repository": "docker.io/library/redis"},
			},
			BuildCandidate: func(matchingdomain.MatchResource) (matchingdomain.CandidateBuild, error) {
				return matchingdomain.CandidateBuild{Settings: map[string]any{"endpoint": "127.0.0.1:6379"}}, nil
			},
			RequiredCapabilities: []string{"otel.receiver.redis.metrics"},
		},
		TemplateSettingRules: map[string]definitions.TemplateSettingRule{
			"endpoint": {Type: "string", Description: "Target endpoint", Required: true, AllowResourceBinding: true},
		},
	}
	registry, err := definitionapp.NewRegistry(definition)
	if err != nil {
		t.Fatal(err)
	}
	return NewEvaluator(registry, evaluationCapabilityAssessor{status: capability})
}

type evaluationCapabilityAssessor struct {
	status matchingdomain.CapabilityStatus
}

func (a evaluationCapabilityAssessor) Assess(required []string) (matchingdomain.CapabilityAssessment, error) {
	assessment := matchingdomain.CapabilityAssessment{
		Status: a.status, Platform: "linux/amd64", ArtifactVersion: "0.160.0", ManifestVersion: "1",
		Required: append([]string(nil), required...),
	}
	if a.status == matchingdomain.CapabilityUnsupported {
		assessment.Unsupported = append([]string(nil), required...)
	}
	return assessment, nil
}

func evaluationSnapshot() matchingdomain.MatchSnapshot {
	now := time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC)
	return matchingdomain.MatchSnapshot{
		InventoryRevision: 4, AsOf: now, StaleAfter: time.Minute,
		Resources: []matchingdomain.MatchResource{{
			Provider: "docker", ProviderInstance: "local", ResourceID: "container-a",
			Kind: "container", LifecycleState: "running", Presence: "present",
			Attributes: map[string]string{"container.image.repository": "docker.io/library/redis"},
			Endpoints: []matchingdomain.MatchEndpoint{{
				Address: "127.0.0.1", Protocol: "tcp", Scope: "host", Provenance: "published_port", Port: 6379,
			}},
			LastSeenAt: now,
		}},
	}
}

func evaluationTemplate(id string, enabled bool) templatesdomain.TemplateInput {
	return templatesdomain.TemplateInput{
		ID: id, DefinitionID: "redis", DefinitionVersion: "1.0.0", Owner: "local",
		Revision: 1, Evaluate: enabled, Settings: map[string]any{},
		Bindings: []templatesdomain.TemplateSettingBinding{{
			Target: "endpoint",
			Source: templatesdomain.TemplateBindingSource{
				Kind: templatesdomain.TemplateSourceEndpoint, EndpointScope: "host", EndpointProtocol: "tcp", EndpointProvenance: "published_port",
			},
		}},
	}
}
