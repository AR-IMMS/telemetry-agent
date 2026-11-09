package integration

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
	"time"
)

func matchableDefinition(id, version, repository string) IntegrationDefinition {
	definition := testDefinition()
	definition.ID = id
	definition.Version = version
	definition.Matching = &MatchingDefinition{
		Selector: &MatchSelector{
			ResourceKinds:   []string{"container"},
			ExactAttributes: map[string]string{"container.image.repository": repository},
		},
		BuildCandidate: func(resource MatchResource) (CandidateBuild, error) {
			endpoint := ""
			for _, item := range resource.Endpoints {
				if item.Scope == "host" && item.Address == "127.0.0.1" && item.Protocol == "tcp" {
					endpoint = netJoinHostPort(item.Address, item.Port)
				}
			}
			if endpoint == "" {
				return CandidateBuild{RequiredInputs: []RequiredInput{{Name: "endpoint", Reason: "reachable endpoint is not unambiguous"}}}, nil
			}
			return CandidateBuild{Settings: map[string]any{"endpoint": endpoint}}, nil
		},
		RequiredCapabilities: []string{"otel.receiver.redis.metrics"},
	}
	return definition
}

func TestMatcherBuildsDeterministicImmutableCandidate(t *testing.T) {
	definition := matchableDefinition("redis", "1.0.0", "docker.io/library/redis")
	registry, err := NewRegistry(definition)
	if err != nil {
		t.Fatal(err)
	}
	matcher := NewMatcher(registry, fixedCapabilityAssessor{status: CapabilitySupported})
	resource := MatchResource{
		Provider: "docker", ProviderInstance: "local-engine", ResourceID: "container-id",
		Kind: "container", LifecycleState: "running", Presence: "present",
		Attributes: map[string]string{"container.image.repository": "docker.io/library/redis"},
		Endpoints:  []MatchEndpoint{{Address: "127.0.0.1", Port: 6380, Protocol: "tcp", Scope: "host", Provenance: "published_port"}},
		LastSeenAt: time.Date(2026, 11, 9, 9, 0, 0, 0, time.UTC),
	}
	input := MatchSnapshot{InventoryRevision: 42, AsOf: resource.LastSeenAt, StaleAfter: 2 * time.Minute, Resources: []MatchResource{resource}}
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
	if a.MatchStatus != MatchMatched || a.ResolutionStatus != ResolutionReady || a.CapabilityStatus != CapabilitySupported || a.Candidate == nil {
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
	definition := matchableDefinition("redis", "1.0.0", "docker.io/library/redis")
	registry, err := NewRegistry(definition)
	if err != nil {
		t.Fatal(err)
	}
	resource := testMatchResource()
	matcher := NewMatcher(registry, fixedCapabilityAssessor{status: CapabilitySupported})
	input := MatchSnapshot{AsOf: resource.LastSeenAt, StaleAfter: time.Minute, Resources: []MatchResource{resource}}
	report, err := matcher.Match(input)
	if err != nil {
		t.Fatal(err)
	}
	if report.Results[0].MatchStatus != MatchMatched || report.Results[0].ResolutionStatus != ResolutionReady {
		t.Fatalf("identification/resolution=%s/%s", report.Results[0].MatchStatus, report.Results[0].ResolutionStatus)
	}
	assessment := report.Results[0].Capability
	assessment.ArtifactVersion = ""
	matcher = NewMatcher(registry, fixedCapabilityAssessor{status: CapabilityStatus("unsupported-but-unverified")})
	matcher.capability = fixedCapabilityAssessorWithEvidence{assessment: assessment}
	report, err = matcher.Match(input)
	if err != nil {
		t.Fatal(err)
	}
	result := report.Results[0]
	if result.MatchStatus != MatchMatched || result.ResolutionStatus != ResolutionReady || result.CapabilityStatus != CapabilityUnverified {
		t.Fatalf("unverified capability changed or falsely supported independent statuses: %+v", result)
	}
	assessment.Status = CapabilityUnsupported
	assessment.ArtifactVersion = "0.160.0"
	assessment.ManifestVersion = "manifest-1"
	matcher.capability = fixedCapabilityAssessorWithEvidence{assessment: assessment}
	report, err = matcher.Match(input)
	if err != nil {
		t.Fatal(err)
	}
	if got := report.Results[0].CapabilityStatus; got != CapabilityUnverified {
		t.Fatalf("unsupported result without missing-capability evidence status=%q", got)
	}
}

func TestMatcherReportsStaleAndAmbiguousEvidenceWithoutCandidate(t *testing.T) {
	first := matchableDefinition("redis", "1.0.0", "docker.io/library/redis")
	second := matchableDefinition("redis-exporter", "1.0.0", "docker.io/library/redis")
	registry, err := NewRegistry(first, second)
	if err != nil {
		t.Fatal(err)
	}
	resource := testMatchResource()
	matcher := NewMatcher(registry, fixedCapabilityAssessor{status: CapabilitySupported})
	report, err := matcher.Match(MatchSnapshot{AsOf: resource.LastSeenAt, StaleAfter: 2 * time.Minute, Resources: []MatchResource{resource}})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Results) != 2 {
		t.Fatalf("ambiguous result count=%d", len(report.Results))
	}
	for _, result := range report.Results {
		if result.MatchStatus != MatchAmbiguous || result.Candidate != nil || result.ResolutionStatus != ResolutionNeedsInput {
			t.Fatalf("ambiguous result became actionable: %+v", result)
		}
	}
	resource.LastSeenAt = resource.LastSeenAt.Add(-3 * time.Minute)
	singleRegistry, err := NewRegistry(first)
	if err != nil {
		t.Fatal(err)
	}
	matcher = NewMatcher(singleRegistry, fixedCapabilityAssessor{status: CapabilitySupported})
	report, err = matcher.Match(MatchSnapshot{AsOf: resource.LastSeenAt.Add(3 * time.Minute), StaleAfter: 2 * time.Minute, Resources: []MatchResource{resource}})
	if err != nil {
		t.Fatal(err)
	}
	for _, result := range report.Results {
		if result.MatchStatus != MatchStale || result.Candidate != nil {
			t.Fatalf("stale evidence produced candidate: %+v", result)
		}
	}
}

func TestResourceFingerprintExcludesPollTimesAndGlobalInventoryRevision(t *testing.T) {
	resource := testMatchResource()
	first := resourceFingerprint(resource)
	resource.LastSeenAt = resource.LastSeenAt.Add(30 * time.Second)
	second := resourceFingerprint(resource)
	if first != second {
		t.Fatalf("poll time changed resource fingerprint: %s != %s", first, second)
	}
	resource.Endpoints[0].Port++
	if got := resourceFingerprint(resource); got == first {
		t.Fatal("material endpoint change did not change resource fingerprint")
	}
}

func TestMatcherRejectsMalformedEndpointEvidence(t *testing.T) {
	resource := testMatchResource()
	resource.Endpoints[0].Scope = "unknown"
	matcher := NewMatcher(Registry{}, nil)
	if _, err := matcher.Match(MatchSnapshot{AsOf: resource.LastSeenAt, StaleAfter: time.Minute, Resources: []MatchResource{resource}}); err == nil {
		t.Fatal("unknown endpoint scope accepted")
	}
}

type fixedCapabilityAssessor struct{ status CapabilityStatus }

func (f fixedCapabilityAssessor) Assess(required []string) (CapabilityAssessment, error) {
	return CapabilityAssessment{Status: f.status, Required: append([]string(nil), required...), Platform: "test/amd64", ArtifactVersion: "test", ManifestVersion: "manifest-test"}, nil
}

type fixedCapabilityAssessorWithEvidence struct{ assessment CapabilityAssessment }

func (f fixedCapabilityAssessorWithEvidence) Assess([]string) (CapabilityAssessment, error) {
	return f.assessment, nil
}

func testMatchResource() MatchResource {
	return MatchResource{
		Provider: "docker", ProviderInstance: "local-engine", ResourceID: "container-id",
		Kind: "container", LifecycleState: "running", Presence: "present",
		Attributes: map[string]string{"container.image.repository": "docker.io/library/redis"},
		Endpoints:  []MatchEndpoint{{Address: "127.0.0.1", Port: 6380, Protocol: "tcp", Scope: "host", Provenance: "published_port"}},
		LastSeenAt: time.Date(2026, 11, 9, 9, 0, 0, 0, time.UTC),
	}
}

func netJoinHostPort(host string, port uint16) string {
	return fmt.Sprintf("%s:%d", host, port)
}
