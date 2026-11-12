package collectorruntime

import (
	"fmt"

	"github.com/ar-imms/telemetry-agent/internal/bootstrap"
	"github.com/ar-imms/telemetry-agent/internal/identity"
)

const manifestVersion = "otelcol-contrib-0.160.0-artifacts-v4"

var manifestArtifacts = map[string]string{
	"linux/amd64":   "7bb60c584c241c86261c2b8697cd3725dd8c56691f5ad5d98454eaa005b47b0c",
	"windows/amd64": "d8de67cf9dc3ffe928610d52b192cb029365bb42dc79a23e0586c040c83293cf",
}

// StaticCatalogEntries declares generic receiver mappings for the pinned
// v0.160.0 distribution. CI verifies each entry against each exact archive.
var staticCatalogEntries = []Capability{
	{ID: "otel.receiver.otlp.metrics", NativeName: "otlp", Kind: "receiver", OS: []string{"linux", "windows"}, Signals: []string{"metrics"}, Pipeline: "metrics"},
	{ID: "otel.receiver.otlp.logs", NativeName: "otlp", Kind: "receiver", OS: []string{"linux", "windows"}, Signals: []string{"logs"}, Pipeline: "logs"},
	{ID: "otel.receiver.otlp.traces", NativeName: "otlp", Kind: "receiver", OS: []string{"linux", "windows"}, Signals: []string{"traces"}, Pipeline: "traces"},
	{ID: "otel.receiver.prometheus.metrics", NativeName: "prometheus", Kind: "receiver", OS: []string{"linux", "windows"}, Signals: []string{"metrics"}, Pipeline: "metrics"},
	// Redis is enabled for Linux and Windows amd64 after artifact inventory and
	// native candidate validation against both exact shipped binaries.
	{ID: "otel.receiver.redis.metrics", NativeName: "redis", Kind: "receiver", OS: []string{"linux", "windows"}, Signals: []string{"metrics"}, Pipeline: "metrics"},
	// PostgreSQL and Prometheus are enabled after artifact inventory and native
	// candidate validation against both exact shipped binaries.
	{ID: "otel.receiver.postgresql.metrics", NativeName: "postgresql", Kind: "receiver", OS: []string{"linux", "windows"}, Signals: []string{"metrics"}, Pipeline: "metrics"},
}

// StaticCatalog returns the reviewed manifest for an exact platform artifact.
// No runtime component inventory is inspected. CI verifies every declared
// entry against the exact archive selected for each supported platform.
func StaticCatalog(platform identity.PlatformInfo) (Catalog, error) {
	artifact, err := bootstrap.SelectArtifact(platform)
	if err != nil {
		return Catalog{}, err
	}
	key := artifact.OS + "/" + artifact.Architecture
	if artifact.Version != "0.160.0" || manifestArtifacts[key] == "" || artifact.ArchiveSHA256 != manifestArtifacts[key] {
		return Catalog{}, fmt.Errorf("pinned Collector artifact does not match capability manifest")
	}
	return NewCatalog(artifact.Version, manifestVersion, staticCatalogEntriesForPlatform(artifact.OS))
}

func staticCatalogEntriesForPlatform(osName string) []Capability {
	entries := make([]Capability, 0, len(staticCatalogEntries))
	for _, entry := range staticCatalogEntries {
		for _, supportedOS := range entry.OS {
			if supportedOS == osName {
				entries = append(entries, entry)
				break
			}
		}
	}
	return entries
}
