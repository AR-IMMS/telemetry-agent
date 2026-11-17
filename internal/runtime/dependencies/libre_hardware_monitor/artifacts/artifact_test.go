package artifacts

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"testing"
)

func TestPinnedArtifactIdentifiesVerifiedRelease(t *testing.T) {
	artifact := PinnedArtifact()
	if artifact.Version != "0.9.6" || artifact.FileName != "LibreHardwareMonitor.zip" {
		t.Fatalf("pinned artifact = %#v, want Libre Hardware Monitor 0.9.6 ZIP", artifact)
	}
	if artifact.SHA256 != "086d9f1b5a99e643edc2cfaaac16051685b551e4c5ac0b32a57c58c0e529c001" {
		t.Fatalf("pinned artifact checksum = %q, want verified release checksum", artifact.SHA256)
	}
}

type testDownloader func(context.Context, string, io.Writer) error

func (download testDownloader) Download(ctx context.Context, url string, destination io.Writer) error {
	return download(ctx, url, destination)
}

func TestDownloadAndVerifyRejectsChecksumMismatch(t *testing.T) {
	var destination bytes.Buffer
	err := DownloadAndVerify(context.Background(), PinnedArtifact(), testDownloader(func(
		ctx context.Context,
		url string,
		writer io.Writer,
	) error {
		if url == "" {
			return fmt.Errorf("URL must not be empty")
		}
		_, writeErr := writer.Write([]byte("tampered archive bytes"))
		return writeErr
	}), &destination)
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("DownloadAndVerify() error = %v, want checksum mismatch", err)
	}
}

func TestDownloadAndVerifyRejectsOversizedArchive(t *testing.T) {
	oversizedContent := bytes.Repeat([]byte("a"), (32<<20)+1)
	err := DownloadAndVerify(context.Background(), Artifact{
		URL:    "https://example.test/LibreHardwareMonitor.zip",
		SHA256: strings.Repeat("0", 64),
	}, testDownloader(func(_ context.Context, _ string, writer io.Writer) error {
		_, writeErr := writer.Write(oversizedContent)
		return writeErr
	}), io.Discard)
	if err == nil || !strings.Contains(err.Error(), "size limit") {
		t.Fatalf("DownloadAndVerify() error = %v, want size limit error", err)
	}
}
