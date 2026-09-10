package bootstrap

import (
	"fmt"
	"runtime"

	"github.com/ar-imms/telemetry-agent/internal/identity"
)

func SelectArtifact(platform identity.PlatformInfo) (Artifact, error) {
	if platform.OS == "" {
		platform.OS = runtime.GOOS
	}
	if platform.Architecture == "" {
		platform.Architecture = runtime.GOARCH
	}
	if platform.Architecture != "amd64" {
		return Artifact{}, fmt.Errorf("unsupported Collector architecture %q", platform.Architecture)
	}
	base := "https://github.com/open-telemetry/opentelemetry-collector-releases/releases/download/v" + collectorVersion + "/"
	switch platform.OS {
	case "linux":
		name := "otelcol-contrib_0.160.0_linux_amd64.tar.gz"
		return Artifact{collectorVersion, "linux", "amd64", base + name, "7bb60c584c241c86261c2b8697cd3725dd8c56691f5ad5d98454eaa005b47b0c", name, "otelcol-contrib"}, nil
	case "windows":
		name := "otelcol-contrib_0.160.0_windows_amd64.tar.gz"
		return Artifact{collectorVersion, "windows", "amd64", base + name, "d8de67cf9dc3ffe928610d52b192cb029365bb42dc79a23e0586c040c83293cf", name, "otelcol-contrib.exe"}, nil
	default:
		return Artifact{}, fmt.Errorf("unsupported Collector operating system %q", platform.OS)
	}
}
