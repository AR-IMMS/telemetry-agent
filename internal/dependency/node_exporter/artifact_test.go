package nodeexporter

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
)

func TestPinnedArtifactIsLinuxAMD64TarGZ(t *testing.T) {
	artifact := PinnedArtifact()

	if artifact.Version != "1.12.1" {
		t.Fatalf(
			"artifact version = %q, want 1.12.1",
			artifact.Version,
		)
	}

	if artifact.ArchiveName !=
		"node_exporter-1.12.1.linux-amd64.tar.gz" {
		t.Fatalf(
			"archive name = %q, want Linux amd64 archive",
			artifact.ArchiveName,
		)
	}

	if artifact.ArchiveFormat != "tar.gz" {
		t.Fatalf(
			"archive format = %q, want tar.gz",
			artifact.ArchiveFormat,
		)
	}

	if artifact.URL !=
		"https://github.com/prometheus/node_exporter/releases/download/v1.12.1/"+ //nolint:lll
			"node_exporter-1.12.1.linux-amd64.tar.gz" {
		t.Fatalf("artifact URL = %q, want official release URL", artifact.URL)
	}

	if artifact.SHA256 !=
		"b51d8a76aa2a9156a55d501aca6276fae09e262259a5e4e831d2c2222f084e63" {
		t.Fatalf(
			"artifact SHA-256 = %q, want pinned official checksum",
			artifact.SHA256,
		)
	}

	if artifact.BinaryPath !=
		"node_exporter-1.12.1.linux-amd64/node_exporter" {
		t.Fatalf(
			"binary path = %q, want expected archive path",
			artifact.BinaryPath,
		)
	}
}

type downloaderFunc func(
	context.Context,
	string,
	io.Writer,
) error

func (f downloaderFunc) Download(
	ctx context.Context,
	url string,
	destination io.Writer,
) error {
	return f(ctx, url, destination)
}

func TestDownloadAndVerifyRejectsChecksumMismatch(t *testing.T) {
	artifact := PinnedArtifact()
	artifact.SHA256 = strings.Repeat("0", 64)

	var destination bytes.Buffer

	err := DownloadAndVerify(
		context.Background(),
		artifact,
		downloaderFunc(func(
			_ context.Context,
			_ string,
			writer io.Writer,
		) error {
			_, err := writer.Write([]byte("untrusted Node Exporter archive"))

			return err
		}),
		&destination,
	)
	if err == nil {
		t.Fatal("DownloadAndVerify() error = nil, want checksum mismatch")
	}
	if !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf(
			"DownloadAndVerify() error = %q, want checksum mismatch",
			err,
		)
	}
}

func TestDownloadAndVerifyRejectsOversizedArchive(t *testing.T) {
	artifact := PinnedArtifact()
	artifact.SHA256 = strings.Repeat("0", 64)

	oversized := bytes.Repeat(
		[]byte("x"),
		(32<<20)+1,
	)

	var destination bytes.Buffer

	err := DownloadAndVerify(
		context.Background(),
		artifact,
		downloaderFunc(func(
			_ context.Context,
			_ string,
			writer io.Writer,
		) error {
			_, err := writer.Write(oversized)

			return err
		}),
		&destination,
	)
	if err == nil {
		t.Fatal("DownloadAndVerify() error = nil, want size limit error")
	}
	if !strings.Contains(err.Error(), "exceeds size limit") {
		t.Fatalf(
			"DownloadAndVerify() error = %q, want size limit error",
			err,
		)
	}
}
