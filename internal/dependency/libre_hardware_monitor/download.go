package librehardwaremonitor

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"strings"

	"github.com/ar-imms/telemetry-agent/internal/dependency"
)

const maxArtifactBytes int64 = 32 << 20

// sizeLimitedWriter stops a download before it exceeds the allowed archive size.
type sizeLimitedWriter struct {
	destination io.Writer
	limit       int64
	written     int64
}

// DownloadAndVerify streams an LHM archive and verifies its pinned checksum.
func DownloadAndVerify(
	ctx context.Context,
	artifact Artifact,
	downloader dependency.ArtifactDownloader,
	destination io.Writer,
) error {
	if downloader == nil {
		return fmt.Errorf("Libre Hardware Monitor downloader is required")
	}
	if destination == nil {
		return fmt.Errorf("Libre Hardware Monitor destination is required")
	}
	if strings.TrimSpace(artifact.URL) == "" {
		return fmt.Errorf("Libre Hardware Monitor artifact URL is required")
	}
	if strings.TrimSpace(artifact.SHA256) == "" {
		return fmt.Errorf("Libre Hardware Monitor artifact SHA-256 is required")
	}

	hash := sha256.New()

	limitedDestination := &sizeLimitedWriter{
		destination: io.MultiWriter(destination, hash),
		limit:       maxArtifactBytes,
	}

	if err := downloader.Download(
		ctx,
		artifact.URL,
		limitedDestination,
	); err != nil {
		return fmt.Errorf("download Libre Hardware Monitor artifact: %w", err)
	}

	gotSHA256 := hex.EncodeToString(hash.Sum(nil))

	if !strings.EqualFold(gotSHA256, artifact.SHA256) {
		return fmt.Errorf(
			"Libre Hardware Monitor artifact checksum mismatch: got %s, want %s",
			gotSHA256,
			artifact.SHA256,
		)
	}

	return nil
}

// Write forwards bytes up to the configured limit, then stops the download.
func (w *sizeLimitedWriter) Write(content []byte) (int, error) {
	remaining := w.limit - w.written

	if remaining <= 0 {
		return 0, fmt.Errorf(
			"Libre Hardware Monitor artifact exceeds size limit of %d bytes",
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
			"Libre Hardware Monitor artifact exceeds size limit of %d bytes",
			w.limit,
		)
	}

	written, err := w.destination.Write(content)
	w.written += int64(written)

	return written, err
}
