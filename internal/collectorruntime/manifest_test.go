package collectorruntime

import (
	"errors"
	"os"
	"os/exec"
	"testing"

	"github.com/ar-imms/telemetry-agent/internal/bootstrap"
	"github.com/ar-imms/telemetry-agent/internal/identity"
	"gopkg.in/yaml.v3"
)

func TestStaticManifestIsBoundToPinnedPlatformArtifacts(t *testing.T) {
	for _, platform := range []identity.PlatformInfo{{OS: "linux", Architecture: "amd64"}, {OS: "windows", Architecture: "amd64"}} {
		artifact, err := bootstrap.SelectArtifact(platform)
		if err != nil {
			t.Fatal(err)
		}
		want, ok := manifestArtifacts[artifact.OS+"/"+artifact.Architecture]
		if !ok || artifact.Version != "0.160.0" || want != artifact.ArchiveSHA256 {
			t.Fatalf("manifest does not match %s artifact %s", artifact.OS, artifact.ArchiveSHA256)
		}
		catalog, err := StaticCatalog(platform)
		if err != nil {
			t.Fatal(err)
		}
		if catalog.version != artifact.Version || catalog.manifestVersion != manifestVersion {
			t.Fatalf("catalog pin = %s/%s", catalog.version, catalog.manifestVersion)
		}
		if len(catalog.Manifest()) == 0 {
			t.Fatalf("catalog for %s has no declared capabilities", artifact.OS)
		}
	}
}

func TestStaticManifestRejectsDuplicateAndMalformedEntries(t *testing.T) {
	entry := Capability{ID: "duplicate", NativeName: "otlp", Kind: "receiver", OS: []string{"linux"}, Signals: []string{"metrics"}, Pipeline: "metrics"}
	if _, err := NewCatalog("0.160.0", "test", []Capability{entry, entry}); !errors.Is(err, ErrInvalidCatalog) {
		t.Fatalf("duplicate entry error=%v", err)
	}
	entry.ID = "Bad ID"
	if _, err := NewCatalog("0.160.0", "test", []Capability{entry}); !errors.Is(err, ErrInvalidCatalog) {
		t.Fatalf("malformed entry error=%v", err)
	}
}

func TestStaticManifestMatchesActualCollectorBinaryWhenProvided(t *testing.T) {
	binary := os.Getenv("OTELCOL_CONTRIB_BINARY")
	if binary == "" {
		t.Skip("exact platform Collector artifact is not available in this test environment")
	}
	output, err := exec.Command(binary, "components").Output()
	if err != nil {
		t.Fatalf("Collector components probe: %v", err)
	}
	var inventory struct {
		BuildInfo struct {
			Version string `yaml:"version"`
		} `yaml:"buildinfo"`
		Receivers []struct {
			Name string `yaml:"name"`
		} `yaml:"receivers"`
		Processors []struct {
			Name string `yaml:"name"`
		} `yaml:"processors"`
		Exporters []struct {
			Name string `yaml:"name"`
		} `yaml:"exporters"`
		Connectors []struct {
			Name string `yaml:"name"`
		} `yaml:"connectors"`
	}
	if err := yaml.Unmarshal(output, &inventory); err != nil {
		t.Fatalf("decode Collector components output: %v", err)
	}
	if inventory.BuildInfo.Version != "0.160.0" {
		t.Fatalf("Collector version=%q, want 0.160.0", inventory.BuildInfo.Version)
	}
	available := map[string]bool{}
	for _, c := range inventory.Receivers {
		available["receiver/"+c.Name] = true
	}
	for _, c := range inventory.Processors {
		available["processor/"+c.Name] = true
	}
	for _, c := range inventory.Exporters {
		available["exporter/"+c.Name] = true
	}
	for _, c := range inventory.Connectors {
		available["connector/"+c.Name] = true
	}
	for _, entry := range staticCatalogEntries {
		if !available[entry.Kind+"/"+entry.NativeName] {
			t.Errorf("manifest component %s/%s absent from pinned binary", entry.Kind, entry.NativeName)
		}
	}
}
