package nodeexporter

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
)

const maxExpandedArchiveBytes int64 = 64 << 20

// ExtractBinary safely extracts only the pinned Node Exporter binary.
func ExtractBinary(
	archivePath string,
	artifact Artifact,
	destinationDir string,
) (string, error) {
	if artifact.ArchiveFormat != "tar.gz" {
		return "", fmt.Errorf(
			"unsupported Node Exporter archive format %q",
			artifact.ArchiveFormat,
		)
	}

	expectedPath, err := safeArchivePath(artifact.BinaryPath)
	if err != nil {
		return "", fmt.Errorf(
			"validate expected Node Exporter binary path: %w",
			err,
		)
	}

	if err := os.MkdirAll(destinationDir, 0o755); err != nil {
		return "", fmt.Errorf(
			"create Node Exporter installation directory: %w",
			err,
		)
	}

	archive, err := os.Open(archivePath)
	if err != nil {
		return "", fmt.Errorf("open Node Exporter archive: %w", err)
	}
	defer archive.Close()

	gzipReader, err := gzip.NewReader(archive)
	if err != nil {
		return "", fmt.Errorf("open Node Exporter gzip archive: %w", err)
	}
	defer gzipReader.Close()

	tarReader := tar.NewReader(gzipReader)

	binaryPath := filepath.Join(
		destinationDir,
		path.Base(expectedPath),
	)
	foundBinary := false
	var expandedSize int64

	for {
		header, err := tarReader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", fmt.Errorf("read Node Exporter archive: %w", err)
		}

		entryPath, err := safeArchivePath(header.Name)
		if err != nil {
			return "", err
		}

		switch header.Typeflag {
		case tar.TypeDir:
			continue

		case tar.TypeReg, tar.TypeRegA:
			if header.Size < 0 ||
				header.Size > maxExpandedArchiveBytes-expandedSize {
				return "", fmt.Errorf(
					"Node Exporter archive exceeds expanded-size limit of %d bytes",
					maxExpandedArchiveBytes,
				)
			}
			expandedSize += header.Size

		case tar.TypeSymlink, tar.TypeLink:
			return "", fmt.Errorf(
				"Node Exporter archive contains unsupported link entry %q",
				entryPath,
			)

		default:
			return "", fmt.Errorf(
				"Node Exporter archive contains unsupported entry %q",
				entryPath,
			)
		}

		if entryPath != expectedPath {
			continue
		}
		if foundBinary {
			return "", fmt.Errorf(
				"Node Exporter archive contains duplicate binary %q",
				expectedPath,
			)
		}

		binary, err := os.OpenFile(
			binaryPath,
			os.O_CREATE|os.O_EXCL|os.O_WRONLY,
			0o755,
		)
		if err != nil {
			return "", fmt.Errorf(
				"create extracted Node Exporter binary: %w",
				err,
			)
		}

		_, copyErr := io.Copy(binary, tarReader)
		closeErr := binary.Close()

		if copyErr != nil {
			return "", fmt.Errorf(
				"extract Node Exporter binary: %w",
				copyErr,
			)
		}
		if closeErr != nil {
			return "", fmt.Errorf(
				"close extracted Node Exporter binary: %w",
				closeErr,
			)
		}
		if err := os.Chmod(binaryPath, 0o755); err != nil {
			return "", fmt.Errorf(
				"set Node Exporter binary permissions: %w",
				err,
			)
		}

		foundBinary = true
	}

	if !foundBinary {
		return "", fmt.Errorf(
			"Node Exporter archive does not contain expected binary %q",
			expectedPath,
		)
	}

	return binaryPath, nil
}

// safeArchivePath normalizes an archive path and rejects path traversal.
func safeArchivePath(value string) (string, error) {
	cleaned := path.Clean(value)

	if value == "" ||
		cleaned == "." ||
		path.IsAbs(value) ||
		cleaned == ".." ||
		strings.HasPrefix(cleaned, "../") {
		return "", fmt.Errorf("unsafe Node Exporter archive path %q", value)
	}

	return cleaned, nil
}
