package bootstrap

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

func Run(ctx context.Context, options Options, downloader Downloader, runner CommandRunner) (Result, error) {
	artifact, err := SelectArtifact(options.Platform)
	if err != nil {
		return Result{}, err
	}
	if options.InstallDir == "" {
		return Result{}, fmt.Errorf("bootstrap install directory is required")
	}
	if options.ValidationTimeout <= 0 {
		options.ValidationTimeout = 2 * time.Minute
	}
	if err := os.MkdirAll(options.InstallDir, 0755); err != nil {
		return Result{}, fmt.Errorf("create Collector install directory: %w", err)
	}
	archivePath := filepath.Join(options.InstallDir, "."+artifact.ArchiveName+".download")
	archive, err := os.OpenFile(archivePath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		return Result{}, fmt.Errorf("create Collector staging file: %w", err)
	}
	err = downloadAndVerify(ctx, downloader, artifact, archive)
	closeErr := archive.Close()
	if err != nil {
		os.Remove(archivePath)
		return Result{}, err
	}
	if closeErr != nil {
		return Result{}, fmt.Errorf("close Collector staging file: %w", closeErr)
	}
	// Archive extraction and config rendering are intentionally explicit follow-up seams.
	binaryPath := filepath.Join(options.InstallDir, artifact.BinaryName)
	validationContext, cancel := context.WithTimeout(ctx, options.ValidationTimeout)
	defer cancel()
	if options.ConfigPath == "" {
		return Result{}, fmt.Errorf("bootstrap configuration path is required")
	}
	if err := validateCollector(validationContext, runner, binaryPath, options.ConfigPath); err != nil {
		return Result{}, err
	}
	return Result{Artifact: artifact, BinaryPath: binaryPath, ConfigPath: options.ConfigPath}, nil
}
