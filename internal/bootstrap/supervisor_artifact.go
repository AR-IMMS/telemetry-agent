package bootstrap

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/ar-imms/telemetry-agent/internal/identity"
)

const supervisorVersion = "0.160.0"

// SupervisorArtifact describes one pinned official OpAMP Supervisor binary.
type SupervisorArtifact struct {
	Version      string
	OS           string
	Architecture string
	URL          string
	BinaryName   string
	SHA256       string
}

// SupervisorInstallResult reports the verified binary installed for one platform.
type SupervisorInstallResult struct {
	Artifact   SupervisorArtifact
	BinaryPath string
	Reused     bool
}

type supervisorInstallMetadata struct {
	Version      string `json:"version"`
	OS           string `json:"os"`
	Architecture string `json:"architecture"`
	SHA256       string `json:"sha256"`
}

// SelectSupervisorArtifact returns the pinned official Supervisor binary for the platform.
func SelectSupervisorArtifact(platform identity.PlatformInfo) (SupervisorArtifact, error) {
	if platform.OS == "" {
		platform.OS = runtime.GOOS
	}
	if platform.Architecture == "" {
		platform.Architecture = runtime.GOARCH
	}
	if !strings.EqualFold(strings.TrimSpace(platform.Architecture), "amd64") {
		return SupervisorArtifact{}, fmt.Errorf("unsupported Supervisor architecture %q", platform.Architecture)
	}

	baseURL := "https://github.com/open-telemetry/opentelemetry-collector-releases/releases/download/v" + supervisorVersion + "/"
	switch strings.ToLower(strings.TrimSpace(platform.OS)) {
	case "windows":
		name := "opampsupervisor_0.160.0_windows_amd64.exe"
		return SupervisorArtifact{Version: supervisorVersion, OS: "windows", Architecture: "amd64", URL: baseURL + name, BinaryName: name, SHA256: "df2dfae96d6b711db31675d2deb8493d3dbada8561b25fe575fd90bd7cac8449"}, nil
	case "linux":
		name := "opampsupervisor_0.160.0_linux_amd64"
		return SupervisorArtifact{Version: supervisorVersion, OS: "linux", Architecture: "amd64", URL: baseURL + name, BinaryName: name, SHA256: "7967e3210e0252f25f41a96b15758dd2e03feb0aecbe85a945f1e2e6e71865c9"}, nil
	default:
		return SupervisorArtifact{}, fmt.Errorf("unsupported Supervisor operating system %q", platform.OS)
	}
}

// InstallSupervisorBinary downloads, verifies, and atomically installs one pinned Supervisor binary.
func InstallSupervisorBinary(ctx context.Context, installDir string, artifact SupervisorArtifact, downloader Downloader) (SupervisorInstallResult, error) {
	if ctx == nil || strings.TrimSpace(installDir) == "" || downloader == nil {
		return SupervisorInstallResult{}, fmt.Errorf("Supervisor install context, directory, and downloader are required")
	}
	if err := validateSupervisorArtifact(artifact); err != nil {
		return SupervisorInstallResult{}, err
	}
	targetDir := supervisorVersionDirectory(installDir, artifact)
	binaryPath := filepath.Join(targetDir, artifact.BinaryName)
	metadataPath := filepath.Join(targetDir, "install.json")
	if verified, err := findVerifiedSupervisorBinary(binaryPath, metadataPath, artifact); err != nil {
		return SupervisorInstallResult{}, err
	} else if verified {
		return SupervisorInstallResult{Artifact: artifact, BinaryPath: binaryPath, Reused: true}, nil
	}
	if _, err := os.Lstat(targetDir); err == nil {
		return SupervisorInstallResult{}, fmt.Errorf("Supervisor installation target %q exists but is not verified", targetDir)
	} else if !os.IsNotExist(err) {
		return SupervisorInstallResult{}, fmt.Errorf("inspect Supervisor installation target: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(targetDir), 0o755); err != nil {
		return SupervisorInstallResult{}, fmt.Errorf("create Supervisor versions directory: %w", err)
	}
	stagingDir, err := os.MkdirTemp(filepath.Dir(targetDir), ".opampsupervisor-"+artifact.Version+"-")
	if err != nil {
		return SupervisorInstallResult{}, fmt.Errorf("create Supervisor staging directory: %w", err)
	}
	defer os.RemoveAll(stagingDir)
	stagedPath := filepath.Join(stagingDir, artifact.BinaryName)
	file, err := os.OpenFile(stagedPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o700)
	if err != nil {
		return SupervisorInstallResult{}, fmt.Errorf("create staged Supervisor binary: %w", err)
	}
	writer := &supervisorHashWriter{dst: file, hash: sha256.New(), max: 128 << 20}
	downloadErr := downloader.Download(ctx, artifact.URL, writer)
	closeErr := file.Close()
	if downloadErr != nil || closeErr != nil {
		return SupervisorInstallResult{}, fmt.Errorf("download Supervisor v%s: %w", artifact.Version, errors.Join(downloadErr, closeErr))
	}
	if got := hex.EncodeToString(writer.hash.Sum(nil)); got != artifact.SHA256 {
		return SupervisorInstallResult{}, fmt.Errorf("Supervisor SHA-256 mismatch: got %s, want pinned digest", got)
	}
	metadata, err := json.Marshal(supervisorInstallMetadata{Version: artifact.Version, OS: artifact.OS, Architecture: artifact.Architecture, SHA256: artifact.SHA256})
	if err != nil {
		return SupervisorInstallResult{}, fmt.Errorf("encode Supervisor install metadata: %w", err)
	}
	if err := os.WriteFile(filepath.Join(stagingDir, "install.json"), metadata, 0o600); err != nil {
		return SupervisorInstallResult{}, fmt.Errorf("write Supervisor install metadata: %w", err)
	}
	if err := publishInstallation(ctx, stagingDir, targetDir); err != nil {
		return SupervisorInstallResult{}, fmt.Errorf("publish Supervisor installation: %w", err)
	}
	return SupervisorInstallResult{Artifact: artifact, BinaryPath: binaryPath}, nil
}

func validateSupervisorArtifact(artifact SupervisorArtifact) error {
	if artifact.Version == "" || artifact.OS == "" || artifact.Architecture != "amd64" || artifact.BinaryName == "" || !strings.HasPrefix(artifact.URL, "https://") {
		return fmt.Errorf("Supervisor artifact metadata is incomplete")
	}
	digest, err := hex.DecodeString(artifact.SHA256)
	if err != nil || len(digest) != sha256.Size {
		return fmt.Errorf("Supervisor artifact SHA-256 is invalid")
	}
	return nil
}

func supervisorVersionDirectory(installDir string, artifact SupervisorArtifact) string {
	return filepath.Join(installDir, "versions", fmt.Sprintf("opampsupervisor-%s-%s-%s", artifact.Version, artifact.OS, artifact.Architecture))
}

func findVerifiedSupervisorBinary(binaryPath, metadataPath string, artifact SupervisorArtifact) (bool, error) {
	data, err := os.ReadFile(metadataPath)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read Supervisor installation metadata: %w", err)
	}
	var metadata supervisorInstallMetadata
	if err := json.Unmarshal(data, &metadata); err != nil || metadata.Version != artifact.Version || metadata.OS != artifact.OS || metadata.Architecture != artifact.Architecture || metadata.SHA256 != artifact.SHA256 {
		return false, nil
	}
	file, err := os.Open(binaryPath)
	if err != nil {
		return false, nil
	}
	hash := sha256.New()
	_, copyErr := io.Copy(hash, file)
	closeErr := file.Close()
	if copyErr != nil || closeErr != nil {
		return false, nil
	}
	return hex.EncodeToString(hash.Sum(nil)) == artifact.SHA256, nil
}

type supervisorHashWriter struct {
	dst  io.Writer
	hash hash.Hash
	max  int64
	size int64
}

func (w *supervisorHashWriter) Write(value []byte) (int, error) {
	if int64(len(value)) > w.max-w.size {
		return 0, fmt.Errorf("Supervisor binary exceeds maximum size of %d bytes", w.max)
	}
	n, err := w.dst.Write(value)
	if n > 0 {
		_, _ = w.hash.Write(value[:n])
		w.size += int64(n)
	}
	return n, err
}
