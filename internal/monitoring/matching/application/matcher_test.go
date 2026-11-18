package application

import (
	"testing"
	"time"

	matchingdomain "github.com/ar-imms/telemetry-agent/internal/monitoring/matching/domain"
	definitionapp "github.com/ar-imms/telemetry-agent/internal/monitoring/profiles/definitions/application"
	definitions "github.com/ar-imms/telemetry-agent/internal/monitoring/profiles/definitions/domain"
)

func TestMatcherReturnsSettingsCandidateFromMonitoringOwnedDefinition(t *testing.T) {
	registry, err := definitionapp.NewRegistry(definitions.Definition{
		ID: "sample", Version: "1.0.0",
		Validate: func(settings map[string]any) error {
			if settings["endpoint"] != "127.0.0.1:9000" {
				return errInvalidTestSettings
			}
			return nil
		},
		Contribute: func(settings map[string]any) (definitions.Contribution, error) {
			return definitions.NewContribution(nil), nil
		},
		Matching: &matchingdomain.MatchingDefinition{
			Selector: &matchingdomain.MatchSelector{ResourceKinds: []string{"container"}},
			BuildCandidate: func(matchingdomain.MatchResource) (matchingdomain.CandidateBuild, error) {
				return matchingdomain.CandidateBuild{Settings: map[string]any{"endpoint": "127.0.0.1:9000"}}, nil
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	resource := matchingdomain.MatchResource{
		Provider: "docker", ProviderInstance: "desktop", ResourceID: "container-1",
		Kind: "container", LifecycleState: "running", Presence: "present", LastSeenAt: time.Now().UTC(),
	}
	report, err := NewMatcher(registry, nil).Match(matchingdomain.MatchSnapshot{
		InventoryRevision: 4, AsOf: resource.LastSeenAt, StaleAfter: time.Minute, Resources: []matchingdomain.MatchResource{resource},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Results) != 1 {
		t.Fatalf("results = %d, want 1", len(report.Results))
	}
	result := report.Results[0]
	if result.MatchStatus != matchingdomain.MatchMatched || result.ResolutionStatus != matchingdomain.ResolutionReady || result.Candidate == nil {
		t.Fatalf("match result = %+v", result)
	}
}

var errInvalidTestSettings = &testSettingsError{}

type testSettingsError struct{}

func (*testSettingsError) Error() string { return "unexpected endpoint" }
