package agentinstallation

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/ar-imms/telemetry-agent/internal/agentstate"
	"github.com/ar-imms/telemetry-agent/internal/identity"
)

func TestPR07NativeServiceUpgradeMigrationRollbackUninstall(t *testing.T) {
	if os.Getenv("AR_IMMS_PR07C_NATIVE_SERVICE_E2E") != "1" {
		t.Skip("set AR_IMMS_PR07C_NATIVE_SERVICE_E2E=1 on a clean privileged VM")
	}
	legacyBinary := requiredNativeServiceE2EValue(t, "AR_IMMS_PR07C_LEGACY_AGENTCTL_BINARY")
	currentBinary := requiredNativeServiceE2EValue(t, "AR_IMMS_PR07C_AGENTCTL_BINARY")
	configRoot := requiredNativeServiceE2EValue(t, "AR_IMMS_PR07C_CONFIG_ROOT")
	evidenceDirectory := strings.TrimSpace(os.Getenv("AR_IMMS_PR07C_SERVICE_EVIDENCE_DIR"))
	var evidence []string
	record := func(format string, values ...any) { evidence = append(evidence, fmt.Sprintf(format, values...)) }
	installed := false
	var layout Layout
	t.Cleanup(func() {
		if installed {
			cleanupContext, cleanupCancel := context.WithTimeout(context.Background(), 3*time.Minute)
			output, cleanupErr := nativeServiceE2ERun(cleanupContext, currentBinary, "uninstall", "--state-path", layout.StatePath, "--timeout", "2m")
			cleanupCancel()
			if cleanupErr != nil {
				record("cleanup_uninstall_error=%q output=%q", cleanupErr.Error(), output)
				t.Errorf("cleanup uninstall failed: %v; output=%s", cleanupErr, output)
			} else {
				record("cleanup_uninstall=success")
			}
		}
		if evidenceDirectory != "" {
			if err := os.MkdirAll(evidenceDirectory, 0o755); err != nil {
				t.Errorf("create native service evidence directory: %v", err)
				return
			}
			path := filepath.Join(evidenceDirectory, "native-service-e2e.txt")
			if writeErr := os.WriteFile(path, []byte(strings.Join(evidence, "\n")+"\n"), 0o600); writeErr != nil {
				t.Errorf("write native service evidence: %v", writeErr)
			}
		}
	})
	if runtime.GOARCH != "amd64" {
		t.Fatalf("native service E2E requires amd64, got %s", runtime.GOARCH)
	}
	platform, err := identity.CollectPlatformInfo()
	if err != nil {
		t.Fatalf("collect platform: %v", err)
	}
	layout, err = DefaultLayout(platform)
	if err != nil {
		t.Fatal(err)
	}
	admin, err := nativeServiceE2EIsAdministrator()
	if err != nil {
		t.Fatalf("check service-install privilege: %v", err)
	}
	if !admin {
		t.Fatal("native service E2E requires root/system-administrator privileges")
	}
	if err := nativeServiceE2ERequireCleanHost(layout); err != nil {
		t.Fatalf("unsafe native service test host: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	legacyDigest, err := nativeServiceE2EDigest(legacyBinary)
	if err != nil {
		t.Fatal(err)
	}
	currentDigest, err := nativeServiceE2EDigest(currentBinary)
	if err != nil {
		t.Fatal(err)
	}
	record("platform=%s/%s", platform.OS, platform.Architecture)
	record("legacy_agentctl_sha256=%s", legacyDigest)
	record("current_agentctl_sha256=%s", currentDigest)
	record("service=%s", layout.ServiceName)
	installLegacy := []string{"install", "--config-root", configRoot, "--gateway-endpoint", "127.0.0.1:4317", "--validation-endpoint", "127.0.0.1:4317", "--timeout", "2m"}
	installed = true
	if output, err := nativeServiceE2ERun(ctx, legacyBinary, installLegacy...); err != nil {
		t.Fatalf("install PR-06 legacy Agent: %v; output=%s", err, output)
	}
	if err := waitForNativeServiceE2ERuntime(ctx, currentBinary, layout, false); err != nil {
		t.Fatalf("legacy service did not become healthy: %v", err)
	}
	assertNativeServiceE2EOwner(t, layout, false)
	record("legacy_install=active healthy collector_processes=1 supervisor_processes=0")

	installManaged := append(append([]string(nil), installLegacy...), "--opamp-server-url", "ws://127.0.0.1:1/v1/opamp")
	if output, err := nativeServiceE2ERun(ctx, currentBinary, installManaged...); err != nil {
		t.Fatalf("upgrade and migrate to managed Supervisor: %v; output=%s", err, output)
	}
	if err := waitForNativeServiceE2ERuntime(ctx, currentBinary, layout, true); err != nil {
		t.Fatalf("managed service did not become healthy after migration: %v", err)
	}
	assertNativeServiceE2EOwner(t, layout, true)
	record("upgrade_migration=active healthy collector_processes=1 supervisor_processes=1")

	if err := nativeServiceE2ERestart(ctx, layout.ServiceName); err != nil {
		t.Fatalf("restart migrated native service: %v", err)
	}
	if err := waitForNativeServiceE2ERuntime(ctx, currentBinary, layout, true); err != nil {
		t.Fatalf("managed service did not recover after native restart: %v", err)
	}
	record("managed_service_restart=healthy collector_processes=1 supervisor_processes=1")

	rollback := append(append([]string(nil), installLegacy...), "--disable-opamp-supervisor")
	if output, err := nativeServiceE2ERun(ctx, currentBinary, rollback...); err != nil {
		t.Fatalf("rollback to legacy Agent runtime: %v; output=%s", err, output)
	}
	if err := waitForNativeServiceE2ERuntime(ctx, currentBinary, layout, false); err != nil {
		t.Fatalf("legacy service did not become healthy after rollback: %v", err)
	}
	assertNativeServiceE2EOwner(t, layout, false)
	record("rollback=active healthy collector_processes=1 supervisor_processes=0")

	if err := nativeServiceE2ERestart(ctx, layout.ServiceName); err != nil {
		t.Fatalf("restart rolled-back native service: %v", err)
	}
	if err := waitForNativeServiceE2ERuntime(ctx, currentBinary, layout, false); err != nil {
		t.Fatalf("legacy service did not recover after restart: %v", err)
	}
	record("legacy_service_restart=healthy collector_processes=1 supervisor_processes=0")

	if output, err := nativeServiceE2ERun(ctx, currentBinary, "uninstall", "--state-path", layout.StatePath, "--timeout", "2m"); err != nil {
		t.Fatalf("uninstall migrated Agent: %v; output=%s", err, output)
	}
	installed = false
	if err := nativeServiceE2ERequireRemoved(layout); err != nil {
		t.Fatalf("uninstall left owned service resources: %v", err)
	}
	if err := waitForNativeServiceE2EProcesses(ctx, 0, 0); err != nil {
		t.Fatalf("uninstall left Agent-owned runtime processes: %v", err)
	}
	record("uninstall=service_paths_and_runtime_processes_removed")
}

func requiredNativeServiceE2EValue(t *testing.T, key string) string {
	t.Helper()
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		t.Fatalf("%s is required when native service E2E is enabled", key)
	}
	return value
}

func nativeServiceE2EDigest(path string) (string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read test Agent binary: %w", err)
	}
	return fmt.Sprintf("%x", sha256.Sum256(content)), nil
}

func nativeServiceE2ERun(ctx context.Context, executable string, args ...string) (string, error) {
	command := exec.CommandContext(ctx, executable, args...)
	output, err := command.CombinedOutput()
	return strings.TrimSpace(string(output)), err
}

func waitForNativeServiceE2ERuntime(ctx context.Context, binary string, layout Layout, supervisorEnabled bool) error {
	for {
		active, serviceErr := nativeServiceE2EActive(layout.ServiceName)
		statusOutput, statusErr := nativeServiceE2ERun(ctx, binary, "status")
		state, stateErr := agentstate.NewStore(layout.StatePath).Load()
		collectorCount, supervisorCount, processErr := nativeServiceE2EProcessCounts()
		if serviceErr == nil && active && statusErr == nil && strings.Contains(statusOutput, "Agent health: healthy") && strings.Contains(statusOutput, "Collector state: ready") && stateErr == nil && state.Supervisor.Enabled == supervisorEnabled && collectorCount == 1 && boolToCount(supervisorEnabled) == supervisorCount {
			return nil
		}
		if ctx.Err() != nil {
			return fmt.Errorf("service_active=%t service_error=%v status=%q status_error=%v owner_enabled=%t state_error=%v collector_processes=%d supervisor_processes=%d process_error=%v: %w", active, serviceErr, statusOutput, statusErr, state.Supervisor.Enabled, stateErr, collectorCount, supervisorCount, processErr, ctx.Err())
		}
		select {
		case <-ctx.Done():
		case <-time.After(time.Second):
		}
	}
}

func waitForNativeServiceE2EProcesses(ctx context.Context, collectorWant, supervisorWant int) error {
	for {
		collectorCount, supervisorCount, err := nativeServiceE2EProcessCounts()
		if err == nil && collectorCount == collectorWant && supervisorCount == supervisorWant {
			return nil
		}
		if ctx.Err() != nil {
			return fmt.Errorf("collector_processes=%d supervisor_processes=%d process_error=%v: %w", collectorCount, supervisorCount, err, ctx.Err())
		}
		select {
		case <-ctx.Done():
		case <-time.After(time.Second):
		}
	}
}

func assertNativeServiceE2EOwner(t *testing.T, layout Layout, enabled bool) {
	t.Helper()
	state, err := agentstate.NewStore(layout.StatePath).Load()
	if err != nil {
		t.Fatalf("load persisted ownership state: %v", err)
	}
	if state.Installation == nil || state.Supervisor.Enabled != enabled {
		t.Fatalf("persisted owner state = installation:%t supervisor:%t, want installation present and supervisor:%t", state.Installation != nil, state.Supervisor.Enabled, enabled)
	}
}

func boolToCount(value bool) int {
	if value {
		return 1
	}
	return 0
}
