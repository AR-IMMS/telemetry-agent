package agentlifecycle

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/ar-imms/telemetry-agent/internal/agentstate"
	"github.com/ar-imms/telemetry-agent/internal/bootstrap"
	"github.com/ar-imms/telemetry-agent/internal/collectorruntime"
	"github.com/ar-imms/telemetry-agent/internal/config"
	"github.com/ar-imms/telemetry-agent/internal/dependency"
	"github.com/ar-imms/telemetry-agent/internal/identity"
	"github.com/ar-imms/telemetry-agent/internal/reconciliation/contracts"
)

func managedRuntimeCandidate(
	t *testing.T,
	statePath string,
	collectorPath string,
	collectorConfigPath string,
	baselinePath string,
	baseline []byte,
) (contracts.CandidateApplyRequest, CollectorConfigurationOptions) {
	t.Helper()
	if err := os.WriteFile(baselinePath, baseline, 0o600); err != nil {
		t.Fatalf("write Agent local configuration baseline: %v", err)
	}
	platform := identity.PlatformInfo{OS: runtime.GOOS, Architecture: runtime.GOARCH}
	dependencies, err := dependency.NewCatalog(nil)
	if err != nil {
		t.Fatalf("create empty managed dependency catalog: %v", err)
	}
	store := agentstate.NewSQLiteStore(statePath)
	state, err := store.Load()
	if err != nil {
		t.Fatalf("load Agent state for local candidate: %v", err)
	}
	gatewayRepository := agentstate.NewCoreGatewayStore(statePath)
	gateway, err := gatewayRepository.EffectiveGateway()
	if err != nil {
		t.Fatalf("resolve current Gateway for local candidate: %v", err)
	}
	values := map[string]string{"OTEL_GATEWAY_ENDPOINT": gateway.Endpoint}
	layers := []config.Layer{{Name: "agent-local", Path: baselinePath}}
	clean, err := BuildCleanBaselineSnapshot(platform, dependencies, layers, state, values)
	if err != nil {
		t.Fatalf("build local candidate baseline: %v", err)
	}
	capabilities, err := collectorruntime.StaticCatalog(platform)
	if err != nil {
		t.Fatalf("load pinned Collector capabilities: %v", err)
	}
	candidate, err := collectorruntime.NewCompiler(capabilities).Compile(clean.Input, nil)
	if err != nil {
		t.Fatalf("compile local Collector candidate: %v", err)
	}
	request := contracts.CandidateApplyRequest{
		ExpectedGeneration:  clean.DesiredGeneration,
		BaselineFingerprint: clean.Fingerprint,
		Candidate:           candidate,
	}
	options := CollectorConfigurationOptions{
		Platform:          platform,
		Catalog:           dependencies,
		Layers:            layers,
		RenderValues:      values,
		BinaryPath:        collectorPath,
		ConfigPath:        collectorConfigPath,
		GatewayRepository: gatewayRepository,
		ValidationEnvironment: []string{
			fmt.Sprintf("OTEL_GATEWAY_ENDPOINT=%s", gateway.Endpoint),
		},
		Runner: bootstrap.OSCommandRunner{},
	}
	return request, options
}

func waitForManagedCandidateApplied(
	t *testing.T,
	ctx context.Context,
	done <-chan error,
	store agentstate.Repository,
	generation uint64,
) {
	t.Helper()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		state, err := store.Load()
		if err != nil {
			t.Fatalf("load Agent state while waiting for local candidate readiness: %v", err)
		}
		if state.AppliedGeneration >= generation {
			return
		}
		select {
		case runtimeErr := <-done:
			t.Fatalf("Agent-managed runtime exited before local candidate readiness: %v", runtimeErr)
		case <-ctx.Done():
			t.Fatalf("local candidate generation %d was not applied: %v", generation, ctx.Err())
		case <-ticker.C:
		}
	}
}

func managedCandidateBaselinePath(root string) string {
	return filepath.Join(root, "agent-managed-local-baseline.yaml")
}
