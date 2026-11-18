package application

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
	"time"

	matchingdomain "github.com/ar-imms/telemetry-agent/internal/monitoring/matching/domain"
	definitionsapp "github.com/ar-imms/telemetry-agent/internal/monitoring/profiles/definitions/application"
	definitionsdomain "github.com/ar-imms/telemetry-agent/internal/monitoring/profiles/definitions/domain"
)

func matchableDefinition(id, version, repository string) definitionsdomain.Definition {
	definition := definitionsdomain.Definition{ID: id, Version: version, Validate: func(map[string]any) error { return nil }}
	definition.Contribute = func(map[string]any) (definitionsdomain.Contribution, error) {
		return definitionsdomain.NewContribution(nil), nil
	}
	definition.Matching = &matchingdomain.MatchingDefinition{
		Selector: &matchingdomain.MatchSelector{
			ResourceKinds:   []string{"container"},
			ExactAttributes: map[string]string{"container.image.repository": repository},
		},
		BuildCandidate: func(resource matchingdomain.MatchResource) (matchingdomain.CandidateBuild, error) {
			for _, endpoint := range resource.Endpoints {
				if endpoint.Scope == "host" && endpoint.Address == "127.0.0.1" && endpoint.Protocol == "tcp" {
					return matchingdomain.CandidateBuild{Settings: map[string]any{"endpoint": fmt.Sprintf("%s:%d", endpoint.Address, endpoint.Port)}}, nil
				}
			}
			return matchingdomain.CandidateBuild{RequiredInputs: []matchingdomain.RequiredInput{{Name: "endpoint", Reason: "reachable endpoint is not unambiguous"}}}, nil
		},
		RequiredCapabilities: []string{"otel.receiver.redis.metrics"},
	}
	return definition
}

func TestMatcherBuildsDeterministicImmutableCandidate(t *testing.T) {
	registry, err := definitionsapp.NewRegistry(matchableDefinition("redis", "1.0.0", "docker.io/library/redis"))
	if err != nil {
		t.Fatal(err)
	}
	resource := testMatchResource()
	matcher := NewMatcher(registry, fixedCapabilityAssessor{status: matchingdomain.CapabilitySupported})
	input := matchingdomain.MatchSnapshot{InventoryRevision: 42, AsOf: resource.LastSeenAt, StaleAfter: 2 * time.Minute, Resources: []matchingdomain.MatchResource{resource}}
	first, err := matcher.Match(input)
	if err != nil {
		t.Fatal(err)
	}
	second, err := matcher.Match(input)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Results) != 1 || len(second.Results) != 1 {
		t.Fatalf("result counts=%d and %d", len(first.Results), len(second.Results))
	}
	a, b := first.Results[0], second.Results[0]
	if a.MatchStatus != matchingdomain.MatchMatched || a.ResolutionStatus != matchingdomain.ResolutionReady || a.CapabilityStatus != matchingdomain.CapabilitySupported || a.Candidate == nil {
		t.Fatalf("result statuses=%+v", a)
	}
	if a.ResourceFingerprint != b.ResourceFingerprint || a.Candidate.Digest() != b.Candidate.Digest() {
		t.Fatalf("unstable digests: %+v, %+v", a, b)
	}
	configuration := a.Candidate.Configuration()
	configuration[0] = 'x'
	var settings map[string]any
	if err := json.Unmarshal(a.Candidate.Configuration(), &settings); err != nil {
		t.Fatal(err)
	}
	if settings["endpoint"] != "127.0.0.1:6380" {
		t.Fatalf("candidate was not isolated from accessor mutation: %s", a.Candidate.Configuration())
	}
	if !reflect.DeepEqual(a.Evidence, b.Evidence) {
		t.Fatalf("evidence order is unstable: %+v / %+v", a.Evidence, b.Evidence)
	}
}

func TestMatcherKeepsStatusAxesSeparateAndFailsClosedOnUnverifiedCapability(t *testing.T) {
	registry, err := definitionsapp.NewRegistry(matchableDefinition("redis", "1.0.0", "docker.io/library/redis"))
	if err != nil {
		t.Fatal(err)
	}
	resource := testMatchResource()
	input := matchingdomain.MatchSnapshot{AsOf: resource.LastSeenAt, StaleAfter: time.Minute, Resources: []matchingdomain.MatchResource{resource}}
	matcher := NewMatcher(registry, fixedCapabilityAssessor{status: matchingdomain.CapabilitySupported})
	report, err := matcher.Match(input)
	if err != nil {
		t.Fatal(err)
	}
	if report.Results[0].MatchStatus != matchingdomain.MatchMatched || report.Results[0].ResolutionStatus != matchingdomain.ResolutionReady {
		t.Fatalf("identification/resolution=%s/%s", report.Results[0].MatchStatus, report.Results[0].ResolutionStatus)
	}
	assessment := report.Results[0].Capability
	assessment.ArtifactVersion = ""
	matcher = NewMatcher(registry, fixedCapabilityAssessorWithEvidence{assessment: assessment})
	report, err = matcher.Match(input)
	if err != nil {
		t.Fatal(err)
	}
	result := report.Results[0]
	if result.MatchStatus != matchingdomain.MatchMatched || result.ResolutionStatus != matchingdomain.ResolutionReady || result.CapabilityStatus != matchingdomain.CapabilityUnverified {
		t.Fatalf("unverified capability changed or falsely supported independent statuses: %+v", result)
	}
	assessment.Status = matchingdomain.CapabilityUnsupported
	assessment.ArtifactVersion = "0.160.0"
	assessment.ManifestVersion = "manifest-1"
	matcher = NewMatcher(registry, fixedCapabilityAssessorWithEvidence{assessment: assessment})
	report, err = matcher.Match(input)
	if err != nil {
		t.Fatal(err)
	}
	if got := report.Results[0].CapabilityStatus; got != matchingdomain.CapabilityUnverified {
		t.Fatalf("unsupported result without missing-capability evidence status=%q", got)
	}
}

func TestMatcherReportsStaleAndAmbiguousEvidenceWithoutCandidate(t *testing.T) {
	first := matchableDefinition("redis", "1.0.0", "docker.io/library/redis")
	second := matchableDefinition("redis-exporter", "1.0.0", "docker.io/library/redis")
	registry, err := definitionsapp.NewRegistry(first, second)
	if err != nil {
		t.Fatal(err)
	}
	resource := testMatchResource()
	matcher := NewMatcher(registry, fixedCapabilityAssessor{status: matchingdomain.CapabilitySupported})
	report, err := matcher.Match(matchingdomain.MatchSnapshot{AsOf: resource.LastSeenAt, StaleAfter: 2 * time.Minute, Resources: []matchingdomain.MatchResource{resource}})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Results) != 2 {
		t.Fatalf("ambiguous result count=%d", len(report.Results))
	}
	for _, result := range report.Results {
		if result.MatchStatus != matchingdomain.MatchAmbiguous || result.Candidate != nil || result.ResolutionStatus != matchingdomain.ResolutionNeedsInput {
			t.Fatalf("ambiguous result became actionable: %+v", result)
		}
	}
	resource.LastSeenAt = resource.LastSeenAt.Add(-3 * time.Minute)
	single, err := definitionsapp.NewRegistry(first)
	if err != nil {
		t.Fatal(err)
	}
	matcher = NewMatcher(single, fixedCapabilityAssessor{status: matchingdomain.CapabilitySupported})
	report, err = matcher.Match(matchingdomain.MatchSnapshot{AsOf: resource.LastSeenAt.Add(3 * time.Minute), StaleAfter: 2 * time.Minute, Resources: []matchingdomain.MatchResource{resource}})
	if err != nil {
		t.Fatal(err)
	}
	for _, result := range report.Results {
		if result.MatchStatus != matchingdomain.MatchStale || result.Candidate != nil {
			t.Fatalf("stale evidence produced candidate: %+v", result)
		}
	}
}

func TestMatcherRejectsSecretBearingRequiredInputNames(t *testing.T) {
	definition := matchableDefinition("redis", "1.0.0", "docker.io/library/redis")
	definition.Matching.BuildCandidate = func(matchingdomain.MatchResource) (matchingdomain.CandidateBuild, error) {
		return matchingdomain.CandidateBuild{RequiredInputs: []matchingdomain.RequiredInput{{Name: "password", Reason: "authentication is required"}}}, nil
	}
	registry, err := definitionsapp.NewRegistry(definition)
	if err != nil {
		t.Fatal(err)
	}
	resource := testMatchResource()
	report, err := NewMatcher(registry, fixedCapabilityAssessor{status: matchingdomain.CapabilitySupported}).Match(matchingdomain.MatchSnapshot{AsOf: resource.LastSeenAt, StaleAfter: 2 * time.Minute, Resources: []matchingdomain.MatchResource{resource}})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Results) != 1 {
		t.Fatalf("results=%d, want one", len(report.Results))
	}
	result := report.Results[0]
	if result.ResolutionStatus != matchingdomain.ResolutionInvalid || len(result.RequiredInputs) != 0 || len(result.Diagnostics) != 1 || result.Diagnostics[0].Code != "candidate_input_invalid" {
		t.Fatalf("secret-bearing required input was not rejected safely: %+v", result)
	}
}

func TestResourceFingerprintExcludesPollTimesAndGlobalInventoryRevision(t *testing.T) {
	resource := testMatchResource()
	first := ResourceFingerprint(resource)
	resource.LastSeenAt = resource.LastSeenAt.Add(30 * time.Second)
	if got := ResourceFingerprint(resource); got != first {
		t.Fatalf("poll time changed resource fingerprint: %s != %s", first, got)
	}
	resource.Endpoints[0].Port++
	if got := ResourceFingerprint(resource); got == first {
		t.Fatal("material endpoint change did not change resource fingerprint")
	}
}

func TestMatcherRejectsMalformedEndpointEvidence(t *testing.T) {
	resource := testMatchResource()
	resource.Endpoints[0].Scope = "unknown"
	if _, err := NewMatcher(definitionsapp.Registry{}, nil).Match(matchingdomain.MatchSnapshot{AsOf: resource.LastSeenAt, StaleAfter: time.Minute, Resources: []matchingdomain.MatchResource{resource}}); err == nil {
		t.Fatal("unknown endpoint scope accepted")
	}
}

type fixedCapabilityAssessor struct {
	status matchingdomain.CapabilityStatus
}

func (f fixedCapabilityAssessor) Assess(required []string) (matchingdomain.CapabilityAssessment, error) {
	return matchingdomain.CapabilityAssessment{Status: f.status, Required: append([]string(nil), required...), Platform: "test/amd64", ArtifactVersion: "test", ManifestVersion: "manifest-test"}, nil
}

type fixedCapabilityAssessorWithEvidence struct {
	assessment matchingdomain.CapabilityAssessment
}

func (f fixedCapabilityAssessorWithEvidence) Assess([]string) (matchingdomain.CapabilityAssessment, error) {
	return f.assessment, nil
}

func testMatchResource() matchingdomain.MatchResource {
	return matchingdomain.MatchResource{Provider: "docker", ProviderInstance: "local-engine", ResourceID: "container-id", Kind: "container", LifecycleState: "running", Presence: "present", Attributes: map[string]string{"container.image.repository": "docker.io/library/redis"}, Endpoints: []matchingdomain.MatchEndpoint{{Address: "127.0.0.1", Port: 6380, Protocol: "tcp", Scope: "host", Provenance: "published_port"}}, LastSeenAt: time.Date(2026, 11, 9, 9, 0, 0, 0, time.UTC)}
}
