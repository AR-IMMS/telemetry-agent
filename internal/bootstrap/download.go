package bootstrap

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Download retrieves a Collector artifact into dst using the caller's context.
func (HTTPDownloader) Download(
	ctx context.Context,
	url string,
	dst io.Writer,
) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("create Collector download request: %w", err)
	}

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return fmt.Errorf("request Collector artifact: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return fmt.Errorf(
			"download Collector artifact: unexpected HTTP status %s",
			response.Status,
		)
	}

	if _, err := io.Copy(dst, response.Body); err != nil {
		return fmt.Errorf("write Collector artifact: %w", err)
	}

	return nil
}

func downloadAndVerify(
	ctx context.Context,
	downloader Downloader,
	artifact Artifact,
	dst io.Writer,
) (int64, error) {
	// Hash and size checks happen while writing, bounding disk consumption before
	// the archive can reach extraction.
	if downloader == nil {
		return 0, fmt.Errorf("Collector downloader is required")
	}

	if err := validateExpectedSHA256(artifact.ArchiveSHA256); err != nil {
		return 0, fmt.Errorf("invalid pinned Collector checksum: %w", err)
	}

	hash := sha256.New()
	writer := &boundedHashWriter{
		hash: hash,
		dst:  dst,
		max:  maxArtifactBytes,
	}

	if err := downloader.Download(ctx, artifact.URL, writer); err != nil {
		return 0, fmt.Errorf(
			"download Collector %s: %w",
			artifact.Version,
			err,
		)
	}

	if artifact.ArchiveSizeBytes > 0 &&
		writer.size != artifact.ArchiveSizeBytes {
		return 0, fmt.Errorf(
			"Collector artifact size mismatch: got %d bytes, want %d bytes",
			writer.size,
			artifact.ArchiveSizeBytes,
		)
	}

	got := hex.EncodeToString(hash.Sum(nil))
	if !strings.EqualFold(got, artifact.ArchiveSHA256) {
		return 0, fmt.Errorf(
			"Collector checksum mismatch: got %s, want %s",
			got,
			artifact.ArchiveSHA256,
		)
	}

	return writer.size, nil
}

func validateExpectedSHA256(value string) error {
	// Validate the pinned value before downloading so malformed metadata fails closed.
	decoded, err := hex.DecodeString(value)
	if err != nil {
		return err
	}

	if len(decoded) != sha256.Size {
		return fmt.Errorf(
			"SHA-256 checksum must be %d bytes, got %d",
			sha256.Size,
			len(decoded),
		)
	}

	return nil
}

type boundedHashWriter struct {
	hash io.Writer
	dst  io.Writer
	max  int64
	size int64
}

func (w *boundedHashWriter) Write(p []byte) (int, error) {
	// Refuse oversized writes before hashing or persisting any bytes beyond the cap.
	if int64(len(p)) > w.max-w.size {
		return 0, fmt.Errorf(
			"artifact exceeds maximum size of %d bytes",
			w.max,
		)
	}

	if _, err := w.hash.Write(p); err != nil {
		return 0, fmt.Errorf("hash artifact: %w", err)
	}

	n, err := w.dst.Write(p)
	w.size += int64(n)

	return n, err
}
