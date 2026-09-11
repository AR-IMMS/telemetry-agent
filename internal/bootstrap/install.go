package bootstrap

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
)

const installMetadataFileName = "install.json"

type installedMetadata struct {
	Version       string `json:"version"`
	OS            string `json:"os"`
	Architecture  string `json:"architecture"`
	ArchiveSHA256 string `json:"archive_sha256"`
	BinarySHA256  string `json:"binary_sha256"`
}

func findVerifiedInstallation(
	installDir string,
	artifact Artifact,
) (string, bool, error) {
	// Reuse is allowed only when metadata and the installed binary both match the
	// pinned artifact; malformed or incomplete state is treated as unusable.
	versionDir := versionDirectory(installDir, artifact)
	binaryPath := filepath.Join(versionDir, artifact.BinaryName)
	metadataPath := filepath.Join(versionDir, installMetadataFileName)

	metadataBytes, err := os.ReadFile(metadataPath)
	if errors.Is(err, os.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("read Collector installation metadata: %w", err)
	}

	var metadata installedMetadata
	if err := json.Unmarshal(metadataBytes, &metadata); err != nil {
		return "", false, nil
	}

	if metadata.Version != artifact.Version ||
		metadata.OS != artifact.OS ||
		metadata.Architecture != artifact.Architecture ||
		!strings.EqualFold(metadata.ArchiveSHA256, artifact.ArchiveSHA256) ||
		metadata.BinarySHA256 == "" {
		return "", false, nil
	}

	gotSHA256, err := sha256File(binaryPath)
	if err != nil {
		return "", false, nil
	}

	if !strings.EqualFold(gotSHA256, metadata.BinarySHA256) {
		return "", false, nil
	}

	return binaryPath, true, nil
}

func installArchive(
	archivePath string,
	installDir string,
	artifact Artifact,
) (string, error) {
	// Build a complete version directory before publishing it under versions/.
	versionsDir := filepath.Join(installDir, "versions")

	if err := os.MkdirAll(versionsDir, 0755); err != nil {
		return "", fmt.Errorf("create Collector versions directory: %w", err)
	}

	stagingDir, err := os.MkdirTemp(
		versionsDir,
		".otelcol-contrib-"+artifact.Version+"-",
	)
	if err != nil {
		return "", fmt.Errorf("create Collector installation staging directory: %w", err)
	}
	defer os.RemoveAll(stagingDir)

	binaryPath, err := extractCollectorBinary(
		archivePath,
		artifact,
		stagingDir,
	)
	if err != nil {
		return "", err
	}

	binarySHA256, err := sha256File(binaryPath)
	if err != nil {
		return "", fmt.Errorf("checksum extracted Collector binary: %w", err)
	}

	if err := writeInstallMetadata(stagingDir, installedMetadata{
		Version:       artifact.Version,
		OS:            artifact.OS,
		Architecture:  artifact.Architecture,
		ArchiveSHA256: artifact.ArchiveSHA256,
		BinarySHA256:  binarySHA256,
	}); err != nil {
		return "", err
	}

	targetDir := versionDirectory(installDir, artifact)

	if _, err := os.Stat(targetDir); err == nil {
		return "", fmt.Errorf(
			"Collector installation target %q already exists but is not verified",
			targetDir,
		)
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("inspect Collector installation target: %w", err)
	}

	if err := os.Rename(stagingDir, targetDir); err != nil {
		// Rename publishes the complete verified installation atomically.
		return "", fmt.Errorf("atomically install Collector: %w", err)
	}

	return filepath.Join(targetDir, artifact.BinaryName), nil
}

func extractCollectorBinary(
	archivePath string,
	artifact Artifact,
	destinationDir string,
) (string, error) {
	// Extract only the expected regular file; the archive is untrusted input.
	if artifact.ArchiveFormat != "tar.gz" {
		return "", fmt.Errorf(
			"unsupported Collector archive format %q",
			artifact.ArchiveFormat,
		)
	}

	archive, err := os.Open(archivePath)
	if err != nil {
		return "", fmt.Errorf("open Collector archive: %w", err)
	}
	defer archive.Close()

	gzipReader, err := gzip.NewReader(archive)
	if err != nil {
		return "", fmt.Errorf("open Collector gzip archive: %w", err)
	}
	defer gzipReader.Close()

	tarReader := tar.NewReader(gzipReader)

	var expandedSize int64
	foundBinary := false
	binaryPath := filepath.Join(destinationDir, artifact.BinaryName)

	for {
		header, err := tarReader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", fmt.Errorf("read Collector archive: %w", err)
		}

		entryName, err := safeArchivePath(header.Name)
		if err != nil {
			return "", err
		}

		switch header.Typeflag {
		case tar.TypeDir:
			continue

		case tar.TypeReg, tar.TypeRegA:
			if header.Size < 0 ||
				header.Size > maxExpandedArchiveSize-expandedSize {
				return "", fmt.Errorf(
					"Collector archive exceeds expanded-size limit of %d bytes",
					maxExpandedArchiveSize,
				)
			}
			expandedSize += header.Size

		case tar.TypeSymlink, tar.TypeLink:
			// Links are rejected to prevent archives from escaping the install root.
			return "", fmt.Errorf(
				"Collector archive contains unsupported link entry %q",
				entryName,
			)

		default:
			return "", fmt.Errorf(
				"Collector archive contains unsupported entry %q",
				entryName,
			)
		}

		if entryName != artifact.BinaryPath {
			continue
		}

		if foundBinary {
			return "", fmt.Errorf(
				"Collector archive contains duplicate binary %q",
				artifact.BinaryPath,
			)
		}

		file, err := os.OpenFile(
			binaryPath,
			os.O_CREATE|os.O_EXCL|os.O_WRONLY,
			0755,
		)
		if err != nil {
			return "", fmt.Errorf("create extracted Collector binary: %w", err)
		}

		_, copyErr := io.Copy(file, tarReader)
		closeErr := file.Close()

		if copyErr != nil {
			return "", fmt.Errorf("extract Collector binary: %w", copyErr)
		}
		if closeErr != nil {
			return "", fmt.Errorf("close extracted Collector binary: %w", closeErr)
		}

		if err := os.Chmod(binaryPath, 0755); err != nil {
			return "", fmt.Errorf("set Collector binary permissions: %w", err)
		}

		foundBinary = true
	}

	if !foundBinary {
		return "", fmt.Errorf(
			"Collector archive does not contain expected binary %q",
			artifact.BinaryPath,
		)
	}

	return binaryPath, nil
}

func safeArchivePath(value string) (string, error) {
	// Normalize and reject traversal or absolute paths before joining with the
	// destination directory.
	cleaned := path.Clean(value)

	if value == "" ||
		cleaned == "." ||
		path.IsAbs(value) ||
		cleaned == ".." ||
		strings.HasPrefix(cleaned, "../") {
		return "", fmt.Errorf("unsafe Collector archive path %q", value)
	}

	return cleaned, nil
}

func writeInstallMetadata(
	directory string,
	metadata installedMetadata,
) error {
	// Restrict metadata to the installer account because it controls reuse decisions.
	encoded, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("encode Collector installation metadata: %w", err)
	}

	metadataPath := filepath.Join(directory, installMetadataFileName)

	if err := os.WriteFile(metadataPath, encoded, 0600); err != nil {
		return fmt.Errorf("write Collector installation metadata: %w", err)
	}

	return nil
}

func sha256File(filePath string) (string, error) {
	// Re-hash the installed binary so reuse is based on current bytes, not metadata alone.
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := sha256.New()

	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}

func versionDirectory(installDir string, artifact Artifact) string {
	// Include the pinned platform tuple to prevent incompatible versions sharing a path.
	name := fmt.Sprintf(
		"otelcol-contrib-%s-%s-%s",
		artifact.Version,
		artifact.OS,
		artifact.Architecture,
	)

	return filepath.Join(installDir, "versions", name)
}
