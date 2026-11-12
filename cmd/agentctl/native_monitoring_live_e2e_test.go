package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/ar-imms/telemetry-agent/internal/agentstate"
	"github.com/ar-imms/telemetry-agent/internal/bootstrap"
	"github.com/ar-imms/telemetry-agent/internal/collectorruntime"
	"github.com/ar-imms/telemetry-agent/internal/discovery"
	"github.com/ar-imms/telemetry-agent/internal/enrollment"
	"github.com/ar-imms/telemetry-agent/internal/identity"
	"github.com/ar-imms/telemetry-agent/internal/monitoringprofile"
)

const nativeMonitoringRuntimeE2EEnv = "AR_IMMS_PR05_NATIVE_RUNTIME_E2E"

// TestLiveNativeDockerRedisProfileDeliversOTLP verifies the OTel-native owner,
// multi-instance attribution, and container recreation against a real Docker
// Engine and isolated OTLP sink running the pinned Collector.
func TestLiveNativeDockerRedisProfileDeliversOTLP(t *testing.T) {
	if os.Getenv(nativeMonitoringRuntimeE2EEnv) != "1" {
		t.Skip("set " + nativeMonitoringRuntimeE2EEnv + "=1 to run Docker/Collector/OTLP parity E2E")
	}
	binary := strings.TrimSpace(os.Getenv("AR_IMMS_PR05_COLLECTOR_BINARY"))
	if binary == "" {
		t.Fatal("AR_IMMS_PR05_COLLECTOR_BINARY must point to the checksum-verified Collector v0.160.0 binary")
	}
	if _, err := os.Stat(binary); err != nil {
		t.Fatalf("inspect pinned Collector: %v", err)
	}
	dockerEndpoint, err := dockerContextEndpoint()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := exec.Command("docker", "version", "--format", "{{.Server.Version}}").CombinedOutput(); err != nil {
		t.Fatalf("Docker daemon is unavailable: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	root := t.TempDir()
	sinkOutput := &lockedTemplateE2EBuffer{}
	sinkEndpoint := startTemplateOTLPSink(t, ctx, binary, root, sinkOutput)
	statePath := filepath.Join(root, "state.db")
	configRoot, err := filepath.Abs(filepath.Join("..", "..", "configs"))
	if err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(root, "agent", "otel.yaml")
	store := agentstate.NewSQLiteStore(statePath)
	if err := store.Save(agentstate.State{
		Collector:         agentstate.CollectorContext{ConfigRoot: configRoot, BinaryPath: binary, ConfigPath: configPath, GatewayEndpoint: sinkEndpoint, HealthEndpoint: defaultHealthEndpoint},
		DesiredGeneration: 1, ActivatedGeneration: 1, AppliedGeneration: 1,
		Dependencies: map[string]agentstate.DependencyState{},
	}); err != nil {
		t.Fatal(err)
	}
	installationID, err := identity.EnsureInstallationIdentity(agentstate.NewIdentityStore(statePath), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	nodeID := "33333333-4444-4555-8666-777777777777"
	seedRegisteredEnrollment(t, statePath, installationID.String(), nodeID)
	t.Setenv(dockerDiscoveryEndpointEnv, dockerEndpoint)
	t.Setenv("OTELCOL_CONTRIB_BINARY", binary)
	baseline, err := newRuntimeBaseline(statePath)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := baseline.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := collectorruntime.StaticCatalog(baseline.platform)
	if err != nil {
		t.Fatal(err)
	}
	initialConfig, err := collectorruntime.NewCompiler(catalog).Compile(snapshot.Input, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(configPath), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, initialConfig.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	firstName := fmt.Sprintf("arimms-pr05-%d-a", time.Now().UnixNano())
	secondName := strings.ReplaceAll(firstName, "-a", "-b")
	thirdName := strings.ReplaceAll(firstName, "-a", "-unpublished")
	networkName := strings.ReplaceAll(firstName, "-a", "-extra-network")
	if output, err := exec.CommandContext(ctx, "docker", "network", "create", networkName).CombinedOutput(); err != nil {
		t.Fatalf("create isolated second Docker network: %v\n%s", err, output)
	}
	t.Cleanup(func() { _ = exec.Command("docker", "network", "rm", networkName).Run() })
	startNativeRedisContainer(t, ctx, firstName)
	t.Cleanup(func() { stopNativeRedisContainer(firstName) })
	startNativeRedisContainer(t, ctx, secondName)
	t.Cleanup(func() { stopNativeRedisContainer(secondName) })
	startNativeRedisContainerWithoutPublishedPort(t, ctx, thirdName)
	t.Cleanup(func() { stopNativeRedisContainer(thirdName) })
	if output, err := exec.CommandContext(ctx, "docker", "network", "connect", networkName, firstName).CombinedOutput(); err != nil {
		t.Fatalf("connect Redis fixture to second Docker network: %v\n%s", err, output)
	}
	publishedPort := nativeRedisPublishedPort(t, ctx, firstName)
	if publishedPort == 6379 {
		t.Fatalf("expected Docker to map Redis to a nondefault host port, got %d", publishedPort)
	}
	networkCount := nativeDockerNetworkCount(t, ctx, firstName)
	if networkCount < 2 {
		t.Fatalf("Redis fixture is attached to %d Docker networks, want at least 2", networkCount)
	}
	firstID := nativeDockerContainerID(t, ctx, firstName)
	secondID := nativeDockerContainerID(t, ctx, secondName)
	thirdID := nativeDockerContainerID(t, ctx, thirdName)
	t.Logf("PR05_SCENARIO mapped_host_port=%d container_networks=%d unpublished_container=%s", publishedPort, networkCount, thirdID)
	if firstID == secondID {
		t.Fatalf("Docker returned duplicate resource identities %q", firstID)
	}

	serviceCtx, stopService := context.WithCancel(ctx)
	serviceDone := make(chan error, 1)
	go func() {
		serviceDone <- runRuntimeWithReconciliation(serviceCtx, statePath, func(context.Context, string, agentstate.TeardownAction, []agentstate.OwnedResource) error { return nil }, nil, bootstrap.OSCommandRunner{})
	}()
	t.Cleanup(func() {
		stopService()
		select {
		case <-serviceDone:
		case <-time.After(10 * time.Second):
			t.Error("Agent runtime did not stop")
		}
	})
	waitForNativeDelivery(t, ctx, serviceDone, sinkOutput, nodeID, firstID, secondID)
	inventory, err := store.InventoryRepository().Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	for _, resourceID := range []string{firstID, secondID} {
		found := false
		for _, resource := range inventory.Resources {
			if resource.Key.ProviderResourceID == resourceID && resource.Presence == discovery.PresencePresent && resource.Attributes["label.ar-imms.monitoring.enabled"] == "true" {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("Agent inventory did not retain discovered opted-in Docker resource %q", resourceID)
		}
	}
	var unpublished discovery.Resource
	for _, resource := range inventory.Resources {
		if resource.Key.ProviderResourceID == thirdID && resource.Presence == discovery.PresencePresent {
			unpublished = resource
			break
		}
	}
	if unpublished.Key.ProviderResourceID == "" {
		t.Fatalf("unpublished Redis container %q was not retained in inventory", thirdID)
	}
	diagnostic := monitoringprofile.DiagnoseEndpoint(monitoringprofile.RedisProfile(), runtime.GOOS, unpublished.Endpoints, nil)
	if runtime.GOOS == "windows" && diagnostic.State != monitoringprofile.EndpointMissing {
		t.Fatalf("unpublished Windows endpoint diagnosis=%+v, want missing without host mapping", diagnostic)
	}
	stateBeforeChurn, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	configBeforeChurn, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	desiredBeforeChurn, err := store.IntegrationRepository().Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if len(desiredBeforeChurn.Instances) != 0 || countRedisReceiverDefinitions(configBeforeChurn) != 0 {
		t.Fatalf("native-owned targets created Agent receiver instances: desired=%+v config=%s", desiredBeforeChurn, configBeforeChurn)
	}
	stopNativeRedisContainer(firstName)
	replacementName := firstName + "-replacement"
	startNativeRedisContainer(t, ctx, replacementName)
	t.Cleanup(func() { stopNativeRedisContainer(replacementName) })
	replacementID := nativeDockerContainerID(t, ctx, replacementName)
	if replacementID == firstID || replacementID == secondID {
		t.Fatalf("recreated container reused an existing identity: old=%s other=%s new=%s", firstID, secondID, replacementID)
	}
	waitForNativeDelivery(t, ctx, serviceDone, sinkOutput, nodeID, replacementID, secondID)
	stateAfterChurn, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	configAfterChurn, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if stateAfterChurn.AppliedGeneration != stateBeforeChurn.AppliedGeneration || !bytes.Equal(configBeforeChurn, configAfterChurn) {
		t.Fatalf("container churn changed Collector activation: applied %d -> %d", stateBeforeChurn.AppliedGeneration, stateAfterChurn.AppliedGeneration)
	}
	if platformOS := runtime.GOOS; platformOS != "windows" && platformOS != "linux" {
		t.Fatalf("unexpected E2E host platform %s", platformOS)
	}
}

func startNativeRedisContainerWithoutPublishedPort(t *testing.T, ctx context.Context, name string) {
	t.Helper()
	args := []string{"run", "--detach", "--name", name, "--label", "ar-imms.monitoring.enabled=true", "--label", "ar-imms.monitoring.profile=redis", "--label", "ar-imms.workload.id=cache-service", "redis:7-alpine"}
	if output, err := exec.CommandContext(ctx, "docker", args...).CombinedOutput(); err != nil {
		t.Fatalf("start unpublished Redis fixture: %v\n%s", err, output)
	}
}

func nativeRedisPublishedPort(t *testing.T, ctx context.Context, name string) uint16 {
	t.Helper()
	output, err := exec.CommandContext(ctx, "docker", "port", name, "6379/tcp").CombinedOutput()
	if err != nil {
		t.Fatalf("inspect Redis host port: %v\n%s", err, output)
	}
	address := strings.TrimSpace(string(output))
	_, portText, ok := strings.Cut(address, ":")
	if !ok {
		t.Fatalf("unexpected Docker host port output %q", address)
	}
	var port uint16
	if _, err := fmt.Sscanf(portText, "%d", &port); err != nil || port == 0 {
		t.Fatalf("invalid Docker host port %q: %v", address, err)
	}
	return port
}

func nativeDockerNetworkCount(t *testing.T, ctx context.Context, name string) int {
	t.Helper()
	output, err := exec.CommandContext(ctx, "docker", "inspect", "--format", "{{len .NetworkSettings.Networks}}", name).CombinedOutput()
	if err != nil {
		t.Fatalf("inspect Redis Docker networks: %v\n%s", err, output)
	}
	var count int
	if _, err := fmt.Sscanf(strings.TrimSpace(string(output)), "%d", &count); err != nil {
		t.Fatalf("invalid Docker network count %q: %v", output, err)
	}
	return count
}

func seedRegisteredEnrollment(t *testing.T, statePath, installationID, nodeID string) {
	t.Helper()
	attemptID := "11111111-2222-4333-8444-555555555555"
	repository := agentstate.NewEnrollmentStore(statePath)
	attempt := enrollment.Attempt{
		Payload: enrollment.EnrollmentPayload{
			AttemptID: attemptID, Installation: enrollment.Installation{ID: installationID},
			Agent:       enrollment.Agent{Version: "test"},
			Environment: enrollment.Environment{OS: runtime.GOOS, Architecture: runtime.GOARCH},
		},
		CoreOrigin: "https://core.example.test", BootstrapCredentialRef: "opaque-bootstrap-ref",
		AccessCredentialRef: "opaque-access-ref", Status: enrollment.AttemptPending,
	}
	if err := repository.CreateAttempt(attempt); err != nil {
		t.Fatalf("seed pending enrollment: %v", err)
	}
	result := enrollment.EnrollmentResult{
		EnrollmentID: "22222222-3333-4444-8555-666666666666", InstallationID: installationID,
		Binding: enrollment.Binding{ID: "44444444-5555-4666-8777-888888888888", NodeID: nodeID, Status: "active"},
	}
	if err := repository.CompleteAttempt(attemptID, result, "opaque-access-ref"); err != nil {
		t.Fatalf("seed registered Core binding: %v", err)
	}
}

func startNativeRedisContainer(t *testing.T, ctx context.Context, name string) {
	t.Helper()
	args := []string{"run", "--detach", "--name", name, "--label", "ar-imms.monitoring.enabled=true", "--label", "ar-imms.monitoring.profile=redis", "--label", "ar-imms.workload.id=cache-service", "--publish", "127.0.0.1::6379", "redis:7-alpine"}
	if output, err := exec.CommandContext(ctx, "docker", args...).CombinedOutput(); err != nil {
		t.Fatalf("start Redis fixture: %v\n%s", err, output)
	}
}

func stopNativeRedisContainer(name string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_ = exec.CommandContext(ctx, "docker", "rm", "--force", name).Run()
}

func nativeDockerContainerID(t *testing.T, ctx context.Context, name string) string {
	t.Helper()
	output, err := exec.CommandContext(ctx, "docker", "inspect", "--format", "{{.Id}}", name).CombinedOutput()
	if err != nil {
		t.Fatalf("inspect Redis fixture identity: %v\n%s", err, output)
	}
	return strings.TrimSpace(string(output))
}

func waitForNativeDelivery(t *testing.T, ctx context.Context, serviceDone <-chan error, sink *lockedTemplateE2EBuffer, nodeID string, resourceIDs ...string) {
	t.Helper()
	deadline := time.NewTimer(100 * time.Second)
	defer deadline.Stop()
	poll := time.NewTicker(500 * time.Millisecond)
	defer poll.Stop()
	for {
		output := sink.String()
		ready := true
		for _, resourceID := range resourceIDs {
			if !nativeSinkContainsIdentity(output, nodeID, resourceID) {
				ready = false
				break
			}
		}
		if ready {
			return
		}
		select {
		case err := <-serviceDone:
			t.Fatalf("Agent runtime exited before native Redis telemetry delivery: %v; sink=%s", err, templateSinkDiagnostic(output))
		case <-ctx.Done():
			t.Fatalf("timed out waiting for native Redis telemetry; sink=%s", templateSinkDiagnostic(output))
		case <-deadline.C:
			t.Fatalf("timed out waiting for native Redis telemetry; sink=%s", templateSinkDiagnostic(output))
		case <-poll.C:
		}
	}
}

func nativeSinkContainsIdentity(output, nodeID, resourceID string) bool {
	for _, block := range strings.Split(output, "Resource attributes:")[1:] {
		attributes := strings.SplitN(block, "ScopeMetrics #", 2)[0]
		if strings.Contains(block, "redis.clients.connected") && strings.Contains(attributes, "node_id: Str("+nodeID+")") && strings.Contains(attributes, "resource_id: Str("+resourceID+")") && strings.Contains(attributes, "workload_id: Str(cache-service)") && strings.Contains(attributes, "container.name: Str(") {
			return true
		}
	}
	return false
}
