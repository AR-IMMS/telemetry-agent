package bootstrap

import (
	"path/filepath"
	"runtime"
	"strings"

	"github.com/ar-imms/telemetry-agent/internal/identity/host"
)

// InspectInstalledRuntimeVersions returns versions only when the installed
// Collector and Supervisor binaries still match their verified install records.
func InspectInstalledRuntimeVersions(collectorBinaryPath, supervisorBinaryPath string) identity.ManagedRuntimeVersions {
	platform := identity.PlatformInfo{OS: runtime.GOOS, Architecture: runtime.GOARCH}
	versions := identity.ManagedRuntimeVersions{}
	if artifact, err := SelectArtifact(platform); err == nil {
		versions.Collector = verifiedCollectorVersionAt(collectorBinaryPath, artifact)
	}
	if artifact, err := SelectSupervisorArtifact(platform); err == nil {
		versions.Supervisor = verifiedSupervisorVersionAt(supervisorBinaryPath, artifact)
	}
	return versions
}

func verifiedCollectorVersionAt(binaryPath string, artifact Artifact) string {
	installDir, ok := installationRootForBinary(binaryPath, artifact.BinaryName)
	if !ok || !sameManagedPath(binaryPath, filepath.Join(versionDirectory(installDir, artifact), artifact.BinaryName)) {
		return ""
	}
	verifiedPath, verified, err := findVerifiedInstallation(installDir, artifact)
	if err != nil || !verified || !sameManagedPath(binaryPath, verifiedPath) {
		return ""
	}
	return artifact.Version
}

func verifiedSupervisorVersionAt(binaryPath string, artifact SupervisorArtifact) string {
	installDir, ok := installationRootForBinary(binaryPath, artifact.BinaryName)
	if !ok || !sameManagedPath(binaryPath, filepath.Join(supervisorVersionDirectory(installDir, artifact), artifact.BinaryName)) {
		return ""
	}
	metadataPath := filepath.Join(filepath.Dir(binaryPath), "install.json")
	verified, err := findVerifiedSupervisorBinary(binaryPath, metadataPath, artifact)
	if err != nil || !verified {
		return ""
	}
	return artifact.Version
}

func installationRootForBinary(binaryPath, binaryName string) (string, bool) {
	binaryPath = strings.TrimSpace(binaryPath)
	if binaryPath == "" || binaryName == "" {
		return "", false
	}
	cleaned := filepath.Clean(binaryPath)
	if !sameManagedPath(filepath.Base(cleaned), binaryName) {
		return "", false
	}
	versionDirectory := filepath.Dir(cleaned)
	versionsDirectory := filepath.Dir(versionDirectory)
	if !sameManagedPath(filepath.Base(versionsDirectory), "versions") {
		return "", false
	}
	return filepath.Dir(versionsDirectory), true
}

func sameManagedPath(left, right string) bool {
	left = filepath.Clean(left)
	right = filepath.Clean(right)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(left, right)
	}
	return left == right
}
