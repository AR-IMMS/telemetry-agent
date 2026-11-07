package collectorruntime

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"

	"github.com/ar-imms/telemetry-agent/internal/bootstrap"
	"github.com/ar-imms/telemetry-agent/internal/identity"
	"github.com/ar-imms/telemetry-agent/internal/integration"
	redisintegration "github.com/ar-imms/telemetry-agent/internal/integration/redis"
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
	expectedArtifact, supportedPlatform := manifestArtifacts[runtime.GOOS+"/"+runtime.GOARCH]
	if !supportedPlatform {
		t.Fatalf("test runner platform %s/%s has no pinned artifact manifest", runtime.GOOS, runtime.GOARCH)
	}
	if got := os.Getenv("OTELCOL_ARTIFACT_SHA256"); got != expectedArtifact {
		t.Fatalf("Collector archive checksum evidence=%q, want %q", got, expectedArtifact)
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
			Name   string `yaml:"name"`
			Module string `yaml:"module"`
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
	modules := map[string]string{}
	for _, c := range inventory.Receivers {
		available["receiver/"+c.Name] = true
		modules["receiver/"+c.Name] = c.Module
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
	for _, entry := range staticCatalogEntriesForPlatform(runtime.GOOS) {
		if !available[entry.Kind+"/"+entry.NativeName] {
			t.Errorf("manifest component %s/%s absent from pinned binary", entry.Kind, entry.NativeName)
		}
	}
	if catalogHasCapability(staticCatalogEntriesForPlatform(runtime.GOOS), "otel.receiver.redis.metrics") && !strings.Contains(modules["receiver/redis"], "receiver/redisreceiver v0.160.0") {
		t.Fatalf("Redis receiver module identity=%q, want receiver/redisreceiver v0.160.0", modules["receiver/redis"])
	}
}

func TestRedisCapabilityIsEnabledOnlyForVerifiedArtifactPlatforms(t *testing.T) {
	windows, err := StaticCatalog(identity.PlatformInfo{OS: "windows", Architecture: "amd64"})
	if err != nil {
		t.Fatal(err)
	}
	if !catalogHasCapability(windows.Manifest(), "otel.receiver.redis.metrics") {
		t.Fatal("verified Windows catalog is missing the Redis metrics receiver")
	}

	linux, err := StaticCatalog(identity.PlatformInfo{OS: "linux", Architecture: "amd64"})
	if err != nil {
		t.Fatal(err)
	}
	if !catalogHasCapability(linux.Manifest(), "otel.receiver.redis.metrics") {
		t.Fatal("verified Linux catalog is missing the Redis metrics receiver")
	}
}

func TestStaticCatalogRejectsUnsupportedArchitecture(t *testing.T) {
	if _, err := StaticCatalog(identity.PlatformInfo{OS: "linux", Architecture: "arm64"}); err == nil {
		t.Fatal("StaticCatalog() error = nil for an artifact without pinned evidence")
	}
}

func catalogHasCapability(entries []Capability, id string) bool {
	for _, entry := range entries {
		if entry.ID == id {
			return true
		}
	}
	return false
}

func TestRedisCandidateNativeValidationWhenPinnedArtifactProvided(t *testing.T) {
	binary := os.Getenv("OTELCOL_CONTRIB_BINARY")
	if binary == "" {
		t.Skip("exact pinned Collector binary is not available in this test environment")
	}
	platformKey := runtime.GOOS + "/" + runtime.GOARCH
	expectedChecksum, supportedPlatform := manifestArtifacts[platformKey]
	if !supportedPlatform {
		t.Skipf("no verified Collector artifact for %s", platformKey)
	}
	if got := os.Getenv("OTELCOL_ARTIFACT_SHA256"); got != expectedChecksum {
		t.Fatalf("Collector archive checksum evidence=%q, want %q", got, expectedChecksum)
	}
	definition := redisintegration.Definition()
	registry, err := integration.NewRegistry(definition)
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := integration.NewResolver(registry).Resolve(integration.IntegrationInstance{
		ID: "redis-e2e", DefinitionID: definition.ID, DefinitionVersion: definition.Version,
		Enabled: true, Configuration: map[string]any{"endpoint": "127.0.0.1:6379"},
	})
	if err != nil {
		t.Fatal(err)
	}
	input := testInput(t)
	input.Platform = identity.PlatformInfo{OS: runtime.GOOS, Architecture: runtime.GOARCH}
	catalog, err := StaticCatalog(input.Platform)
	if err != nil {
		t.Fatal(err)
	}
	candidate, err := NewCompiler(catalog).Compile(input, []integration.ResolvedInstance{resolved})
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateCandidate(context.Background(), bootstrap.OSCommandRunner{}, binary, candidate, nil); err != nil {
		t.Fatalf("pinned %s Collector rejected Redis candidate: %v", platformKey, err)
	}
}
