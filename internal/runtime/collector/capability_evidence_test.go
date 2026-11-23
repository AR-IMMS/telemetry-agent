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

func TestCatalogSupportsQueriesUsePinnedPlatformManifest(t *testing.T) {
	catalog, err := NewCatalog("0.160.0", "manifest-v3", []Capability{{
		ID: "redisreceiver", NativeName: "redis", Kind: "receiver", OS: []string{"linux"}, Signals: []string{"metrics"}, Pipeline: "metrics",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if !catalog.Supports("redisreceiver", "linux") || catalog.Supports("redisreceiver", "windows") || catalog.Supports("missing", "linux") {
		t.Fatal("capability query did not match the pinned platform manifest")
	}
}
