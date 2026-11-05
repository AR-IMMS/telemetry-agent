package bootstrap

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ar-imms/telemetry-agent/internal/config"
)

const renderedConfigPermission = 0640

// ActivateRenderedConfig validates rendered config in a temporary directory
// before atomically replacing targetPath.
//
// A validation failure leaves the currently active target configuration
// unchanged.
func ActivateRenderedConfig(
	ctx context.Context,
	runner CommandRunner,
	binaryPath string,
	targetPath string,
	rendered []byte,
	environment []string,
) error {
	if runner == nil {
		return fmt.Errorf("Collector command runner is required")
	}

	if binaryPath == "" {
		return fmt.Errorf("Collector binary path is required")
	}

	if targetPath == "" {
		return fmt.Errorf("Collector configuration path is required")
	}

	configDir := filepath.Dir(targetPath)

	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf(
			"create Collector configuration directory %q: %w",
			configDir,
			err,
		)
	}

	// Create a temporary staging directory for the rendered config. This ensures
	// that the validation subprocess does not see any other files in the target
	// directory, which could affect validation results.
	stagingDir, err := os.MkdirTemp(configDir, ".bootstrap-config-")
	if err != nil {
		return fmt.Errorf(
			"create Collector configuration staging directory: %w",
			err,
		)
	}

	defer os.RemoveAll(stagingDir)

	// After this point we have another path
	// example:
	// 	- candidate: /path/to/config/.bootstrap-config-123456789/otelcol.yaml
	// 	- target: /path/to/config/otelcol.yaml
	stagedConfigPath := filepath.Join(
		stagingDir,
		filepath.Base(targetPath),
	)

	if err := config.WriteAtomic(
		stagedConfigPath,
		rendered,
		renderedConfigPermission,
	); err != nil {
		return fmt.Errorf(
			"write Collector configuration to staging path %q: %w",
			stagedConfigPath,
			err,
		)
	}

	if err := validateCollector(
		ctx,
		runner,
		binaryPath,
		stagedConfigPath,
		environment,
	); err != nil {
		return fmt.Errorf(
			"validate Collector configuration at staging path %q: %w",
			stagedConfigPath,
			err,
		)
	}

	if err := os.Rename(stagedConfigPath, targetPath); err != nil {
		return fmt.Errorf(
			"activate validated Collector configuration: %w",
			err,
		)
	}

	return nil
}

// ValidateRenderedConfig validates candidate bytes with the pinned Collector
// without touching the active configuration path or lifecycle state.
func ValidateRenderedConfig(ctx context.Context, runner CommandRunner, binaryPath string, rendered []byte, environment []string) error {
	if runner == nil {
		return fmt.Errorf("Collector command runner is required")
	}
	if binaryPath == "" {
		return fmt.Errorf("Collector binary path is required")
	}
	dir, err := os.MkdirTemp("", "otelcol-candidate-validation-")
	if err != nil {
		return fmt.Errorf("create Collector validation staging directory: %w", err)
	}
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "candidate.yaml")
	if err := config.WriteAtomic(path, rendered, renderedConfigPermission); err != nil {
		return fmt.Errorf("stage Collector validation candidate: %w", err)
	}
	if err := validateCollector(ctx, runner, binaryPath, path, append([]string(nil), environment...)); err != nil {
		return fmt.Errorf("validate Collector candidate: %w", err)
	}
	return nil
}
