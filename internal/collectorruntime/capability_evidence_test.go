package collectorruntime

import "testing"

func TestCatalogExposesSelectedArtifactAndManifestIdentity(t *testing.T) {
	catalog, err := NewCatalog("0.160.0", "manifest-v3", nil)
	if err != nil {
		t.Fatal(err)
	}
	if catalog.ArtifactVersion() != "0.160.0" || catalog.ManifestVersion() != "manifest-v3" {
		t.Fatalf("catalog evidence=%q/%q", catalog.ArtifactVersion(), catalog.ManifestVersion())
	}
}
