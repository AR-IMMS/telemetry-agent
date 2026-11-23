package bootstrap

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/ar-imms/telemetry-agent/internal/identity/host"
)

func TestInspectInstalledRuntimeVersionsRequiresVerifiedPinnedBinaries(t *testing.T) {
	platform := identity.PlatformInfo{OS: runtime.GOOS, Architecture: runtime.GOARCH}
	collector, err := SelectArtifact(platform)
	if err != nil {
		t.Fatal(err)
	}
	supervisor, err := SelectSupervisorArtifact(platform)
	if err != nil {
		t.Fatal(err)
	}

	root := t.TempDir()
	collectorPath := writeCollectorVersionFixture(t, filepath.Join(root, "collector"), collector)
	supervisorPath := writeSupervisorVersionFixture(t, filepath.Join(root, "supervisor"), supervisor)

	got := InspectInstalledRuntimeVersions(collectorPath, supervisorPath)
	if got.Collector != collector.Version || got.Supervisor != "" {
		t.Fatalf("verified versions = %+v; want Collector %q and an unverified Supervisor version to be empty", got, collector.Version)
	}

	if err := os.WriteFile(collectorPath, []byte("tampered Collector"), 0o600); err != nil {
		t.Fatal(err)
	}
	got = InspectInstalledRuntimeVersions(collectorPath, supervisorPath)
	if got.Collector != "" || got.Supervisor != "" {
		t.Fatalf("versions after Collector tamper = %+v; want empty values for unverified binaries", got)
	}
}

func TestVerifiedSupervisorVersionAtRequiresPinnedPathAndChecksum(t *testing.T) {
	content := []byte("verified Supervisor fixture")
	digest := sha256.Sum256(content)
	artifact := SupervisorArtifact{
		Version: "test", OS: runtime.GOOS, Architecture: runtime.GOARCH,
		URL: "https://example.invalid/opampsupervisor", BinaryName: "opampsupervisor",
		SHA256: hex.EncodeToString(digest[:]),
	}
	versionDir := supervisorVersionDirectory(t.TempDir(), artifact)
	if err := os.MkdirAll(versionDir, 0o700); err != nil {
		t.Fatal(err)
	}
	binaryPath := filepath.Join(versionDir, artifact.BinaryName)
	if err := os.WriteFile(binaryPath, content, 0o600); err != nil {
		t.Fatal(err)
	}
	metadata, err := json.Marshal(supervisorInstallMetadata{
		Version: artifact.Version, OS: artifact.OS, Architecture: artifact.Architecture, SHA256: artifact.SHA256,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(versionDir, "install.json"), metadata, 0o600); err != nil {
		t.Fatal(err)
	}
	if got := verifiedSupervisorVersionAt(binaryPath, artifact); got != artifact.Version {
		t.Fatalf("verified Supervisor version = %q; want %q", got, artifact.Version)
	}
	if err := os.WriteFile(binaryPath, []byte("tampered Supervisor"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := verifiedSupervisorVersionAt(binaryPath, artifact); got != "" {
		t.Fatalf("Supervisor version after binary tamper = %q; want empty", got)
	}
}

func writeCollectorVersionFixture(t *testing.T, installDir string, artifact Artifact) string {
	t.Helper()
	versionDir := versionDirectory(installDir, artifact)
	if err := os.MkdirAll(versionDir, 0o700); err != nil {
		t.Fatal(err)
	}
	binaryPath := filepath.Join(versionDir, artifact.BinaryName)
	content := []byte("pinned Collector fixture")
	if err := os.WriteFile(binaryPath, content, 0o600); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(content)
	metadata, err := json.Marshal(installedMetadata{
		Version: artifact.Version, OS: artifact.OS, Architecture: artifact.Architecture,
		ArchiveSHA256: artifact.ArchiveSHA256, BinarySHA256: hex.EncodeToString(digest[:]),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(versionDir, installMetadataFileName), metadata, 0o600); err != nil {
		t.Fatal(err)
	}
	return binaryPath
}

func writeSupervisorVersionFixture(t *testing.T, installDir string, artifact SupervisorArtifact) string {
	t.Helper()
	versionDir := supervisorVersionDirectory(installDir, artifact)
	if err := os.MkdirAll(versionDir, 0o700); err != nil {
		t.Fatal(err)
	}
	binaryPath := filepath.Join(versionDir, artifact.BinaryName)
	if err := os.WriteFile(binaryPath, []byte("pinned Supervisor fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	metadata, err := json.Marshal(supervisorInstallMetadata{
		Version: artifact.Version, OS: artifact.OS, Architecture: artifact.Architecture, SHA256: artifact.SHA256,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(versionDir, "install.json"), metadata, 0o600); err != nil {
		t.Fatal(err)
	}
	return binaryPath
}
