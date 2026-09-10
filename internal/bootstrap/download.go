package bootstrap

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
)

func downloadAndVerify(ctx context.Context, downloader Downloader, artifact Artifact, dst io.Writer) error {
	hash := sha256.New()
	limited := &boundedHashWriter{hash: hash, dst: dst}
	if err := downloader.Download(ctx, artifact.URL, limited); err != nil {
		return fmt.Errorf("download Collector %s: %w", artifact.Version, err)
	}
	if limited.size > maxArtifactBytes {
		return fmt.Errorf("download Collector %s: exceeds %d bytes", artifact.Version, maxArtifactBytes)
	}
	got := hex.EncodeToString(hash.Sum(nil))
	if got != artifact.SHA256 {
		return fmt.Errorf("Collector checksum mismatch: got %s, want %s", got, artifact.SHA256)
	}
	return nil
}

type boundedHashWriter struct {
	hash io.Writer
	dst  io.Writer
	size int64
}

func (w *boundedHashWriter) Write(p []byte) (int, error) {
	w.size += int64(len(p))
	if w.size > maxArtifactBytes {
		return 0, fmt.Errorf("artifact exceeds maximum size")
	}
	if _, err := w.hash.Write(p); err != nil {
		return 0, err
	}
	return w.dst.Write(p)
}
