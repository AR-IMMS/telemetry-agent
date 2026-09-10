package bootstrap

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/ar-imms/telemetry-agent/internal/identity"
)

func TestSelectArtifactPinsOfficialRelease(t *testing.T) {
	artifact, err := SelectArtifact(identity.PlatformInfo{OS: "linux", Architecture: "amd64"})
	if err != nil {
		t.Fatal(err)
	}
	if artifact.Version != "0.160.0" || !strings.HasSuffix(artifact.URL, "otelcol-contrib_0.160.0_linux_amd64.tar.gz") || len(artifact.SHA256) != 64 {
		t.Fatalf("unexpected artifact: %+v", artifact)
	}
}
func TestSelectArtifactRejectsUnsupportedPlatform(t *testing.T) {
	if _, err := SelectArtifact(identity.PlatformInfo{OS: "darwin", Architecture: "amd64"}); err == nil {
		t.Fatal("expected unsupported platform error")
	}
}

type fakeDownloader struct{ data []byte }

func (f fakeDownloader) Download(_ context.Context, _ string, dst io.Writer) error {
	_, err := dst.Write(f.data)
	return err
}
func TestDownloadAndVerifyRejectsMismatch(t *testing.T) {
	err := downloadAndVerify(context.Background(), fakeDownloader{[]byte("bad")}, Artifact{Version: "x", URL: "x", SHA256: strings.Repeat("0", 64)}, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "checksum") {
		t.Fatalf("err = %v", err)
	}
}

type fakeRunner struct {
	result     CommandResult
	executable string
	args       []string
}

func (f *fakeRunner) Run(_ context.Context, executable string, args ...string) CommandResult {
	f.executable, f.args = executable, args
	return f.result
}
func TestValidateCollectorUsesOfficialArguments(t *testing.T) {
	runner := &fakeRunner{result: CommandResult{ExitCode: 0}}
	if err := validateCollector(context.Background(), runner, "otelcol-contrib", "otel.yaml"); err != nil {
		t.Fatal(err)
	}
	if strings.Join(runner.args, " ") != "validate --config otel.yaml" {
		t.Fatalf("args = %v", runner.args)
	}
}
