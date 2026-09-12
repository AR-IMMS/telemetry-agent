package bootstrap

import (
	"context"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ar-imms/telemetry-agent/internal/config"
	"github.com/ar-imms/telemetry-agent/internal/identity"
)

type recordingDownloader struct {
	calls int
}

func (d *recordingDownloader) Download(
	_ context.Context,
	_ string,
	_ io.Writer,
) error {
	d.calls++
	return nil
}

func TestRunDoesNotDownloadWhenConfigRenderingFails(t *testing.T) {
	downloader := &recordingDownloader{}

	_, err := Run(
		context.Background(),
		Options{
			InstallDir: t.TempDir(),
			ConfigPath: filepath.Join(
				t.TempDir(),
				"config",
				"otel.yaml",
			),
			ConfigInput: config.RenderInput{
				Layers: []config.Layer{
					{
						Name: "missing",
						Path: filepath.Join(
							t.TempDir(),
							"missing.yaml",
						),
					},
				},
			},
		},
		downloader,
		&activationRunner{},
	)

	if err == nil {
		t.Fatal("Run() error = nil, want configuration rendering error")
	}

	if downloader.calls != 0 {
		t.Fatalf(
			"downloader calls = %d, want 0",
			downloader.calls,
		)
	}
}

func TestSelectArtifactPinsOfficialRelease(t *testing.T) {
	artifact, err := SelectArtifact(identity.PlatformInfo{OS: "linux", Architecture: "amd64"})
	if err != nil {
		t.Fatal(err)
	}
	if artifact.Version != "0.160.0" ||
		!strings.HasSuffix(artifact.URL, "otelcol-contrib_0.160.0_linux_amd64.tar.gz") ||
		len(artifact.ArchiveSHA256) != 64 ||
		artifact.ArchiveFormat != "tar.gz" ||
		artifact.BinaryPath != "otelcol-contrib" {
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
	_, err := downloadAndVerify(context.Background(), fakeDownloader{[]byte("bad")}, Artifact{Version: "x", URL: "x", ArchiveSHA256: strings.Repeat("0", 64)}, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "checksum") {
		t.Fatalf("err = %v", err)
	}
}

type fakeRunner struct {
	result     CommandResult
	executable string
	args       []string
}

func (f *fakeRunner) Run(_ context.Context, command Command) CommandResult {
	f.executable, f.args = command.Executable, command.Args
	return f.result
}
func TestValidateCollectorUsesOfficialArguments(t *testing.T) {
	runner := &fakeRunner{result: CommandResult{ExitCode: 0}}
	if err := validateCollector(context.Background(), runner, "otelcol-contrib", "otel.yaml", nil); err != nil {
		t.Fatal(err)
	}
	if strings.Join(runner.args, " ") != "validate --config otel.yaml" {
		t.Fatalf("args = %v", runner.args)
	}
}
