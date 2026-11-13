package bootstrap

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/ar-imms/telemetry-agent/internal/identity"
)

func TestSelectSupervisorArtifactPinsSupportedPlatform(t *testing.T) {
	for _, test := range []struct {
		platform identity.PlatformInfo
		name     string
		wantFile string
		wantHash string
	}{
		{platform: identity.PlatformInfo{OS: "windows", Architecture: "amd64"}, name: "windows", wantFile: "opampsupervisor_0.160.0_windows_amd64.exe", wantHash: "df2dfae96d6b711db31675d2deb8493d3dbada8561b25fe575fd90bd7cac8449"},
		{platform: identity.PlatformInfo{OS: "linux", Architecture: "amd64"}, name: "linux", wantFile: "opampsupervisor_0.160.0_linux_amd64", wantHash: "7967e3210e0252f25f41a96b15758dd2e03feb0aecbe85a945f1e2e6e71865c9"},
	} {
		t.Run(test.name, func(t *testing.T) {
			artifact, err := SelectSupervisorArtifact(test.platform)
			if err != nil {
				t.Fatal(err)
			}
			if artifact.Version != "0.160.0" || artifact.BinaryName != test.wantFile || artifact.SHA256 != test.wantHash {
				t.Fatalf("artifact = %+v", artifact)
			}
			if !strings.HasPrefix(artifact.URL, "https://github.com/open-telemetry/opentelemetry-collector-releases/releases/download/v0.160.0/") || !strings.HasSuffix(artifact.URL, test.wantFile) {
				t.Fatalf("artifact URL = %q", artifact.URL)
			}
		})
	}
}

func TestSelectSupervisorArtifactRejectsUnsupportedPlatform(t *testing.T) {
	if _, err := SelectSupervisorArtifact(identity.PlatformInfo{OS: "linux", Architecture: "arm64"}); err == nil {
		t.Fatal("unsupported Supervisor architecture was accepted")
	}
}

func TestInstallSupervisorBinaryVerifiesAndReusesPinnedBinary(t *testing.T) {
	content := []byte("verified supervisor fixture")
	sum := sha256.Sum256(content)
	artifact := SupervisorArtifact{Version: "test", OS: runtime.GOOS, Architecture: runtime.GOARCH, URL: "https://example.invalid/opampsupervisor", BinaryName: "opampsupervisor", SHA256: hex.EncodeToString(sum[:])}
	downloader := supervisorArtifactDownloader{content: content}
	root := t.TempDir()

	first, err := InstallSupervisorBinary(context.Background(), root, artifact, downloader)
	if err != nil {
		t.Fatal(err)
	}
	if first.Reused {
		t.Fatal("first install unexpectedly reused a binary")
	}
	if got, err := os.ReadFile(first.BinaryPath); err != nil || string(got) != string(content) {
		t.Fatalf("installed binary content = %q, err=%v", got, err)
	}
	second, err := InstallSupervisorBinary(context.Background(), root, artifact, supervisorArtifactDownloader{err: errors.New("unexpected download")})
	if err != nil {
		t.Fatal(err)
	}
	if !second.Reused || second.BinaryPath != first.BinaryPath {
		t.Fatalf("second install = %+v, want verified reuse at %s", second, first.BinaryPath)
	}
}

func TestInstallSupervisorBinaryRejectsChecksumMismatch(t *testing.T) {
	artifact := SupervisorArtifact{Version: "test", OS: runtime.GOOS, Architecture: runtime.GOARCH, URL: "https://example.invalid/opampsupervisor", BinaryName: "opampsupervisor", SHA256: strings.Repeat("0", 64)}
	if _, err := InstallSupervisorBinary(context.Background(), t.TempDir(), artifact, supervisorArtifactDownloader{content: []byte("bad")}); err == nil {
		t.Fatal("checksum mismatch was accepted")
	}
}

type supervisorArtifactDownloader struct {
	content []byte
	err     error
}

func (d supervisorArtifactDownloader) Download(_ context.Context, _ string, destination io.Writer) error {
	if d.err != nil {
		return d.err
	}
	_, err := destination.Write(d.content)
	return err
}

func TestInstallSupervisorBinaryDoesNotReplaceUnverifiedTarget(t *testing.T) {
	content := []byte("expected")
	sum := sha256.Sum256(content)
	artifact := SupervisorArtifact{Version: "test", OS: runtime.GOOS, Architecture: runtime.GOARCH, URL: "https://example.invalid/opampsupervisor", BinaryName: "opampsupervisor", SHA256: hex.EncodeToString(sum[:])}
	root := t.TempDir()
	path := filepath.Join(root, "versions", "opampsupervisor-test-"+runtime.GOOS+"-"+runtime.GOARCH, artifact.BinaryName)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("unverified"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := InstallSupervisorBinary(context.Background(), root, artifact, supervisorArtifactDownloader{content: content}); err == nil {
		t.Fatal("unverified existing installation was overwritten")
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != "unverified" {
		t.Fatalf("existing target changed: %q, err=%v", got, err)
	}
}
