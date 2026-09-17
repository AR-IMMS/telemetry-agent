package librehardwaremonitor

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ar-imms/telemetry-agent/internal/dependency"
)

// StageArchive downloads a verified ZIP into a private temporary directory.
// The caller must invoke the returned cleanup function.
func StageArchive(
	ctx context.Context,
	artifact Artifact,
	downloader dependency.ArtifactDownloader,
) (string, func(), error) {
	fileName := strings.TrimSpace(artifact.FileName)

	if fileName == "" {
		return "", nil, fmt.Errorf(
			"Libre Hardware Monitor artifact file name is required",
		)
	}
	if filepath.Base(fileName) != fileName {
		return "", nil, fmt.Errorf(
			"Libre Hardware Monitor artifact file name %q is unsafe",
			fileName,
		)
	}

	stagingDir, err := os.MkdirTemp("", ".ar-imms-lhm-")
	if err != nil {
		return "", nil, fmt.Errorf(
			"create Libre Hardware Monitor staging directory: %w",
			err,
		)
	}

	cleanup := func() {
		_ = os.RemoveAll(stagingDir)
	}

	archivePath := filepath.Join(stagingDir, fileName)

	file, err := os.OpenFile(
		archivePath,
		os.O_CREATE|os.O_EXCL|os.O_WRONLY,
		0o600,
	)
	if err != nil {
		cleanup()

		return "", nil, fmt.Errorf(
			"create Libre Hardware Monitor staging archive: %w",
			err,
		)
	}

	downloadErr := DownloadAndVerify(
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
			"close Libre Hardware Monitor staging archive: %w",
			closeErr,
		)
	}

	return archivePath, cleanup, nil
}
