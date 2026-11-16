package bootstrap

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/ar-imms/telemetry-agent/internal/config"
)

const lastKnownGoodSuffix = ".last-known-good"

// SnapshotRuntimeConfig creates a launch-specific config file so readiness
// records the exact bytes Collector read even if a later revision is promoted.
func SnapshotRuntimeConfig(configPath string, generation uint64) (string, func(), error) {
	content, err := os.ReadFile(configPath)
	if err != nil {
		return "", nil, fmt.Errorf("read Collector config for launch: %w", err)
	}
	directory, err := os.MkdirTemp(
		filepath.Dir(configPath),
		".collector-launch-"+strconv.FormatUint(generation, 10)+"-",
	)
	if err != nil {
		return "", nil, fmt.Errorf("create Collector launch config directory: %w", err)
	}
	path := filepath.Join(directory, filepath.Base(configPath))
	if err := config.WriteAtomic(path, content, renderedConfigPermission); err != nil {
		_ = os.RemoveAll(directory)
		return "", nil, fmt.Errorf("write Collector launch config: %w", err)
	}
	return path, func() { _ = os.RemoveAll(directory) }, nil
}

// PrepareConfigRecoveryArtifact snapshots the currently active config before
// a candidate is promoted. Its path is persisted as activation intent.
func PrepareConfigRecoveryArtifact(configPath string, generation uint64) (string, error) {
	if configPath == "" {
		return "", fmt.Errorf("Collector configuration path is required")
	}
	content, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("read active Collector configuration: %w", err)
	}
	path := configPath + ".recovery-" + strconv.FormatUint(generation, 10)
	if err := config.WriteAtomic(path, content, renderedConfigPermission); err != nil {
		return "", fmt.Errorf("persist Collector recovery artifact: %w", err)
	}
	return path, nil
}

// RestoreConfigArtifact restores a saved config. An empty artifact path means
// activation had no previously usable file, so the uncommitted target is removed.
func RestoreConfigArtifact(configPath, artifactPath string) error {
	if configPath == "" {
		return fmt.Errorf("Collector config path is required")
	}
	if artifactPath == "" {
		if err := os.Remove(configPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove uncommitted Collector configuration: %w", err)
		}
		return nil
	}
	content, err := os.ReadFile(artifactPath)
	if err != nil {
		return fmt.Errorf("read Collector recovery artifact: %w", err)
	}
	if err := config.WriteAtomic(configPath, content, renderedConfigPermission); err != nil {
		return fmt.Errorf("restore Collector configuration: %w", err)
	}
	return nil
}

// CommitLastKnownGoodConfig records the active Collector config after runtime
// readiness has confirmed it. The previous committed artifact is replaced
// atomically only at this point.
func CommitLastKnownGoodConfig(configPath string) error {
	return CommitLastKnownGoodConfigFrom(configPath, configPath)
}

// CommitLastKnownGoodConfigFrom records sourcePath bytes under the active
// config's stable last-known-good path.
func CommitLastKnownGoodConfigFrom(configPath, sourcePath string) error {
	if configPath == "" {
		return fmt.Errorf("Collector configuration path is required")
	}
	if sourcePath == "" {
		return fmt.Errorf("Collector source configuration path is required")
	}
	content, err := os.ReadFile(sourcePath)
	if err != nil {
		return fmt.Errorf("read active Collector configuration: %w", err)
	}
	if err := config.WriteAtomic(
		configPath+lastKnownGoodSuffix,
		content,
		renderedConfigPermission,
	); err != nil {
		return fmt.Errorf("persist last-known-good Collector configuration: %w", err)
	}
	return nil
}

// RestoreLastKnownGoodConfig atomically restores the most recently committed
// Collector configuration after a candidate fails runtime readiness.
func RestoreLastKnownGoodConfig(configPath string) error {
	if configPath == "" {
		return fmt.Errorf("Collector configuration path is required")
	}
	content, err := os.ReadFile(configPath + lastKnownGoodSuffix)
	if err != nil {
		return fmt.Errorf("read last-known-good Collector configuration: %w", err)
	}
	if err := config.WriteAtomic(configPath, content, renderedConfigPermission); err != nil {
		return fmt.Errorf("restore last-known-good Collector configuration: %w", err)
	}
	return nil
}
