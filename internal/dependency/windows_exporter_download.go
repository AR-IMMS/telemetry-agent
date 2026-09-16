package dependency

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const maxWindowsExporterArtifactBytes int64 = 32 << 20

// ArtifactDownloader retrieves one artifact URL into the supplied destination.
type ArtifactDownloader interface {
	Download(context.Context, string, io.Writer) error
}

// sizeLimitedWriter stops a download before it writes more than limit bytes.
type sizeLimitedWriter struct {
	destination io.Writer
	limit       int64
	written     int64
}

// DownloadAndVerifyWindowsExporterArtifact streams the pinned MSI to
// destination and rejects it unless its checksum matches the pinned artifact.
func DownloadAndVerifyWindowsExporterArtifact(
	ctx context.Context,
	artifact WindowsExporterArtifact,
	downloader ArtifactDownloader,
	destination io.Writer,
) error {
	if downloader == nil {
		return fmt.Errorf("Windows Exporter artifact downloader is required")
	}
	if destination == nil {
		return fmt.Errorf("Windows Exporter artifact destination is required")
	}
	if strings.TrimSpace(artifact.URL) == "" {
		return fmt.Errorf("Windows Exporter artifact URL is required")
	}
	if strings.TrimSpace(artifact.SHA256) == "" {
		return fmt.Errorf("Windows Exporter artifact SHA-256 is required")
	}

	hash := sha256.New()

	limitedDestination := &sizeLimitedWriter{
		destination: io.MultiWriter(destination, hash),
		limit:       maxWindowsExporterArtifactBytes,
	}

	if err := downloader.Download(
		ctx,
		artifact.URL,
		limitedDestination,
	); err != nil {
		return fmt.Errorf("download Windows Exporter artifact: %w", err)
	}

	gotSHA256 := hex.EncodeToString(hash.Sum(nil))

	if !strings.EqualFold(gotSHA256, artifact.SHA256) {
		return fmt.Errorf(
			"Windows Exporter artifact checksum mismatch: got %s, want %s",
			gotSHA256,
			artifact.SHA256,
		)
	}

	return nil
}

// Write forwards bytes until the configured limit is reached.
func (w *sizeLimitedWriter) Write(content []byte) (int, error) {
	remaining := w.limit - w.written

	if remaining <= 0 {
		return 0, fmt.Errorf(
			"Windows Exporter artifact exceeds size limit of %d bytes",
			w.limit,
		)
	}

	if int64(len(content)) > remaining {
		allowed := int(remaining)

		written, err := w.destination.Write(content[:allowed])
		w.written += int64(written)

		if err != nil {
			return written, err
		}

		return written, fmt.Errorf(
			"Windows Exporter artifact exceeds size limit of %d bytes",
			w.limit,
		)
	}

	written, err := w.destination.Write(content)
	w.written += int64(written)

	return written, err
}

// stageWindowsExporterMSI downloads a verified MSI into a private temporary
// directory. The caller must invoke the returned cleanup function.
func stageWindowsExporterMSI(
	ctx context.Context,
	artifact WindowsExporterArtifact,
	downloader ArtifactDownloader,
) (string, func(), error) {
	fileName := strings.TrimSpace(artifact.FileName)

	if fileName == "" {
		return "", nil, fmt.Errorf(
			"Windows Exporter artifact file name is required",
		)
	}
	if filepath.Base(fileName) != fileName {
		return "", nil, fmt.Errorf(
			"Windows Exporter artifact file name %q is unsafe",
			fileName,
		)
	}

	stagingDir, err := os.MkdirTemp(
		"",
		".ar-imms-windows-exporter-",
	)
	if err != nil {
		return "", nil, fmt.Errorf(
			"create Windows Exporter staging directory: %w",
			err,
		)
	}

	cleanup := func() {
		_ = os.RemoveAll(stagingDir)
	}

	msiPath := filepath.Join(stagingDir, fileName)

	file, err := os.OpenFile(
		msiPath,
		os.O_CREATE|os.O_EXCL|os.O_WRONLY,
		0600,
	)
	if err != nil {
		cleanup()

		return "", nil, fmt.Errorf(
			"create Windows Exporter MSI staging file: %w",
			err,
		)
	}

	downloadErr := DownloadAndVerifyWindowsExporterArtifact(
		ctx,
		artifact,
		downloader,
		file,
	)
	closeErr := file.Close()

	if downloadErr != nil {
		cleanup()

		return "", nil, downloadErr
	}
	if closeErr != nil {
		cleanup()

		return "", nil, fmt.Errorf(
			"close Windows Exporter MSI staging file: %w",
			closeErr,
		)
	}

	return msiPath, cleanup, nil
}
