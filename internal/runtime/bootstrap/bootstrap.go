package bootstrap

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/ar-imms/telemetry-agent/internal/config"
)

// Run renders and activates a validated Collector configuration, then installs
// or reuses its verified Collector binary.
func Run(
	ctx context.Context,
	options Options,
	downloader Downloader,
	runner CommandRunner,
) (Result, error) {
	if options.InstallDir == "" {
		return Result{}, fmt.Errorf("bootstrap install directory is required")
	}

	if options.ConfigPath == "" {
		return Result{}, fmt.Errorf("bootstrap configuration path is required")
	}

	if downloader == nil {
		return Result{}, fmt.Errorf("bootstrap downloader is required")
	}

	if runner == nil {
		return Result{}, fmt.Errorf("bootstrap command runner is required")
	}

	if configInfo, err := os.Stat(options.ConfigPath); err == nil {
		if configInfo.IsDir() {
			return Result{}, fmt.Errorf(
				"Collector configuration path %q is a directory",
				options.ConfigPath,
			)
		}
	} else if !os.IsNotExist(err) {
		return Result{}, fmt.Errorf(
			"inspect Collector configuration path: %w",
			err,
		)
	}

	// Render before downloading so invalid layers or invalid YAML fail without
	// changing the local Collector installation.
	renderedConfig, err := config.Render(options.ConfigInput)
	if err != nil {
		return Result{}, fmt.Errorf(
			"render Collector configuration: %w",
			err,
		)
	}

	if options.ValidationTimeout <= 0 {
		options.ValidationTimeout = 2 * time.Minute
	}

	artifact, err := SelectArtifact(options.Platform)
	if err != nil {
		return Result{}, err
	}

	if err := os.MkdirAll(options.InstallDir, 0755); err != nil {
		return Result{}, fmt.Errorf(
			"create Collector install directory: %w",
			err,
		)
	}

	binaryPath, reused, err := findVerifiedInstallation(
		options.InstallDir,
		artifact,
	)
	if err != nil {
		return Result{}, err
	}

	if !reused {
		binaryPath, err = downloadAndInstall(
			ctx,
			options.InstallDir,
			downloader,
			artifact,
		)
		if err != nil {
			return Result{}, err
		}
	}

	validationContext, cancel := context.WithTimeout(
		ctx,
		options.ValidationTimeout,
	)
	defer cancel()

	if err := ActivateRenderedConfig(
		validationContext,
		runner,
		binaryPath,
		options.ConfigPath,
		renderedConfig,
		options.ValidationEnvironment,
	); err != nil {
		return Result{}, err
	}

	return Result{
		Artifact:   artifact,
		BinaryPath: binaryPath,
		ConfigPath: options.ConfigPath,
		Reused:     reused,
	}, nil
}

func downloadAndInstall(
	ctx context.Context,
	installDir string,
	downloader Downloader,
	artifact Artifact,
) (string, error) {
	// Download and extraction happen below the install root so cleanup cannot
	// remove an existing version if either operation fails.
	stagingDir, err := os.MkdirTemp(installDir, ".bootstrap-")
	if err != nil {
		return "", fmt.Errorf(
			"create Collector download staging directory: %w",
			err,
		)
	}
	defer os.RemoveAll(stagingDir)

	archivePath := filepath.Join(stagingDir, artifact.ArchiveName)

	archive, err := os.OpenFile(
		archivePath,
		os.O_CREATE|os.O_EXCL|os.O_WRONLY,
		0600,
	)
	if err != nil {
		return "", fmt.Errorf(
			"create Collector archive staging file: %w",
			err,
		)
	}

	_, downloadErr := downloadAndVerify(
		ctx,
		downloader,
		artifact,
		archive,
	)
	closeErr := archive.Close()

	if downloadErr != nil {
		return "", downloadErr
	}

	if closeErr != nil {
		return "", fmt.Errorf(
			"close Collector archive staging file: %w",
			closeErr,
		)
	}

	binaryPath, err := installArchive(
		ctx,
		archivePath,
		installDir,
		artifact,
	)
	if err != nil {
		return "", err
	}

	return binaryPath, nil
}
