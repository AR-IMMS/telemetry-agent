package integration

import (
	"reflect"
	"testing"
)

type matchingTestCatalog struct {
	artifact, manifest string
	supported          map[string]bool
}

func (c matchingTestCatalog) Supports(id, _ string) bool { return c.supported[id] }
func (c matchingTestCatalog) ArtifactVersion() string    { return c.artifact }
func (c matchingTestCatalog) ManifestVersion() string    { return c.manifest }

func TestCatalogCapabilityAssessorReportsPinnedCatalogEvidence(t *testing.T) {
	assessor := NewCatalogCapabilityAssessor(matchingTestCatalog{
		artifact: "0.160.0", manifest: "manifest-v3",
		supported: map[string]bool{"redisreceiver": true},
	}, "windows", "windows/amd64")
	got, err := assessor.Assess([]string{"redisreceiver", "hostmetrics", "otherreceiver"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != CapabilityUnsupported || got.Platform != "windows/amd64" || got.ArtifactVersion != "0.160.0" || got.ManifestVersion != "manifest-v3" {
		t.Fatalf("assessment=%+v", got)
	}
	if !reflect.DeepEqual(got.Required, []string{"redisreceiver", "hostmetrics", "otherreceiver"}) || !reflect.DeepEqual(got.Unsupported, []string{"hostmetrics", "otherreceiver"}) {
		t.Fatalf("capability lists required=%v unsupported=%v", got.Required, got.Unsupported)
	}
}
