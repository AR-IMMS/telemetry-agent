package librehardwaremonitor

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"testing"
)

func TestPinnedArtifactIsVerifiedLibreHardwareMonitorRelease(t *testing.T) {
	artifact := PinnedArtifact()

	if artifact.Version != "0.9.6" {
		t.Fatalf("Version = %q, want 0.9.6", artifact.Version)
	}
	if artifact.FileName != "LibreHardwareMonitor.zip" {
		t.Fatalf(
			"FileName = %q, want LibreHardwareMonitor.zip",
			artifact.FileName,
		)
	}

	wantURL := "https://github.com/LibreHardwareMonitor/" +
		"LibreHardwareMonitor/releases/download/v0.9.6/" +
		"LibreHardwareMonitor.zip"

	if artifact.URL != wantURL {
		t.Fatalf("URL = %q, want %q", artifact.URL, wantURL)
	}

	wantSHA256 := "086d9f1b5a99e643edc2cfaaac160516" +
		"85b551e4c5ac0b32a57c58c0e529c001"

	if artifact.SHA256 != wantSHA256 {
		t.Fatalf(
			"SHA256 = %q, want release checksum %q",
			artifact.SHA256,
			wantSHA256,
		)
	}

	if !strings.HasSuffix(artifact.FileName, ".zip") {
		t.Fatalf("FileName = %q, want ZIP archive", artifact.FileName)
	}
}

type testDownloader func(
	context.Context,
	string,
	io.Writer,
) error

func (download testDownloader) Download(
	ctx context.Context,
	url string,
	destination io.Writer,
) error {
	return download(ctx, url, destination)
}

func TestDownloadAndVerifyRejectsChecksumMismatch(t *testing.T) {
	var destination bytes.Buffer

	err := DownloadAndVerify(
		context.Background(),
		PinnedArtifact(),
		testDownloader(func(
			ctx context.Context,
			url string,
			writer io.Writer,
		) error {
			if url == "" {
				return fmt.Errorf("URL must not be empty")
			}

			_, writeErr := writer.Write([]byte("tampered archive bytes"))

			return writeErr
		}),
		&destination,
	)

	if err == nil {
		t.Fatal("DownloadAndVerify() error = nil, want checksum mismatch")
	}
	if !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf(
			"DownloadAndVerify() error = %v, want checksum mismatch",
			err,
		)
	}
}

func TestDownloadAndVerifyRejectsOversizedArchive(t *testing.T) {
	oversizedContent := bytes.Repeat(
		[]byte("a"),
		(32<<20)+1,
	)

	err := DownloadAndVerify(
		context.Background(),
		Artifact{
			URL:    "https://example.test/LibreHardwareMonitor.zip",
			SHA256: strings.Repeat("0", 64),
		},
		testDownloader(func(
			ctx context.Context,
			url string,
			writer io.Writer,
		) error {
			_, writeErr := writer.Write(oversizedContent)

			return writeErr
		}),
		io.Discard,
	)

	if err == nil {
		t.Fatal("DownloadAndVerify() error = nil, want size limit error")
	}
	if !strings.Contains(err.Error(), "size limit") {
		t.Fatalf(
			"DownloadAndVerify() error = %v, want size limit error",
			err,
		)
	}
}
