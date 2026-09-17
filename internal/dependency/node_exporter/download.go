package nodeexporter

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/ar-imms/telemetry-agent/internal/dependency"
)

const maxArtifactBytes int64 = 32 << 20

// sizeLimitedWriter stops a download before it writes more than limit bytes.
type sizeLimitedWriter struct {
	destination io.Writer
	limit       int64
	written     int64
}

// DownloadAndVerify streams one Node Exporter archive and accepts it only when
// its SHA-256 checksum matches the pinned artifact.
func DownloadAndVerify(
	ctx context.Context,
	artifact Artifact,
	downloader dependency.ArtifactDownloader,
	destination io.Writer,
) error {
	if downloader == nil {
		return fmt.Errorf("Node Exporter artifact downloader is required")
	}
	if destination == nil {
		return fmt.Errorf("Node Exporter artifact destination is required")
	}
	if strings.TrimSpace(artifact.URL) == "" {
		return fmt.Errorf("Node Exporter artifact URL is required")
	}
	if strings.TrimSpace(artifact.SHA256) == "" {
		return fmt.Errorf("Node Exporter artifact SHA-256 is required")
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
		return fmt.Errorf("download Node Exporter artifact: %w", err)
	}

	gotSHA256 := hex.EncodeToString(hash.Sum(nil))

	if !strings.EqualFold(gotSHA256, artifact.SHA256) {
		return fmt.Errorf(
			"Node Exporter artifact checksum mismatch: got %s, want %s",
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
			"Node Exporter artifact exceeds size limit of %d bytes",
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
			"Node Exporter artifact exceeds size limit of %d bytes",
			w.limit,
		)
	}

	written, err := w.destination.Write(content)
	w.written += int64(written)

	return written, err
}

// StageArchive downloads a verified archive into a private temporary directory.
// The caller must invoke the returned cleanup function.
func StageArchive(
	ctx context.Context,
	artifact Artifact,
	downloader dependency.ArtifactDownloader,
) (string, func(), error) {
	archiveName := strings.TrimSpace(artifact.ArchiveName)

	if archiveName == "" {
		return "", nil, fmt.Errorf(
			"Node Exporter artifact archive name is required",
		)
	}
	if filepath.Base(archiveName) != archiveName {
		return "", nil, fmt.Errorf(
			"Node Exporter artifact archive name %q is unsafe",
			archiveName,
		)
	}

	stagingDir, err := os.MkdirTemp("", ".ar-imms-node-exporter-")
	if err != nil {
		return "", nil, fmt.Errorf(
			"create Node Exporter staging directory: %w",
			err,
		)
	}

	cleanup := func() {
		_ = os.RemoveAll(stagingDir)
	}

	archivePath := filepath.Join(stagingDir, archiveName)

	archive, err := os.OpenFile(
		archivePath,
		os.O_CREATE|os.O_EXCL|os.O_WRONLY,
		0o600,
	)
	if err != nil {
		cleanup()

		return "", nil, fmt.Errorf(
			"create Node Exporter archive staging file: %w",
			err,
		)
	}

	downloadErr := DownloadAndVerify(
		ctx,
		artifact,
		downloader,
		archive,
	)
	closeErr := archive.Close()

	if downloadErr != nil {
		cleanup()

		return "", nil, downloadErr
	}
	if closeErr != nil {
		cleanup()

		return "", nil, fmt.Errorf(
			"close Node Exporter archive staging file: %w",
			closeErr,
		)
	}

	return archivePath, cleanup, nil
}
