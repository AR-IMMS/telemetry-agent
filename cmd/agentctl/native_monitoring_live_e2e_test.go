package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ar-imms/telemetry-agent/internal/agenthealth"
	"github.com/ar-imms/telemetry-agent/internal/agentstate"
	"github.com/ar-imms/telemetry-agent/internal/bootstrap"
	"github.com/ar-imms/telemetry-agent/internal/collectorruntime"
	"github.com/ar-imms/telemetry-agent/internal/discovery"
	"github.com/ar-imms/telemetry-agent/internal/enrollment"
	"github.com/ar-imms/telemetry-agent/internal/identity"
	"github.com/ar-imms/telemetry-agent/internal/monitoringprofile"
)

const nativeMonitoringRuntimeE2EEnv = "AR_IMMS_PR05_NATIVE_RUNTIME_E2E"
const nativeIsolatedDockerManagerEnv = "AR_IMMS_PR05_ISOLATED_DOCKER_MANAGER"
const nativeFixtureDockerHostEnv = "AR_IMMS_PR05_FIXTURE_DOCKER_HOST"

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
	dockerEndpoint := strings.TrimSpace(os.Getenv(dockerDiscoveryEndpointEnv))
	if dockerEndpoint == "" {
		var err error
		dockerEndpoint, err = dockerContextEndpoint()
		if err != nil {
			t.Fatal(err)
		}
	}
	if output, err := nativeDockerCommand(context.Background(), "version", "--format", "{{.Server.Version}}").CombinedOutput(); err != nil {
		t.Fatalf("Docker daemon is unavailable: %v\n%s", err, output)
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
	fourthName := strings.ReplaceAll(firstName, "-a", "-port-override")
	networkName := strings.ReplaceAll(firstName, "-a", "-extra-network")
	if output, err := nativeDockerCommand(ctx, "network", "create", networkName).CombinedOutput(); err != nil {
		t.Fatalf("create isolated second Docker network: %v\n%s", err, output)
	}
	t.Cleanup(func() { _ = nativeDockerCommand(context.Background(), "network", "rm", networkName).Run() })
	startNativeRedisContainer(t, ctx, firstName)
	t.Cleanup(func() { stopNativeRedisContainer(firstName) })
	startNativeRedisContainer(t, ctx, secondName)
	t.Cleanup(func() { stopNativeRedisContainer(secondName) })
	startNativeRedisContainerWithoutPublishedPort(t, ctx, thirdName)
	t.Cleanup(func() { stopNativeRedisContainer(thirdName) })
	startNativeRedisContainerAtPort(t, ctx, fourthName, 6380, "cache-service-6380", true)
	t.Cleanup(func() { stopNativeRedisContainer(fourthName) })
	if output, err := nativeDockerCommand(ctx, "network", "connect", networkName, thirdName).CombinedOutput(); err != nil {
		t.Fatalf("connect unpublished Redis fixture to second Docker network: %v\n%s", err, output)
	}
	publishedPort := nativeRedisPublishedPort(t, ctx, firstName, 6379)
	overridePublishedPort := nativeRedisPublishedPort(t, ctx, fourthName, 6380)
	if publishedPort == 6379 {
		t.Fatalf("expected Docker to map Redis to a nondefault host port, got %d", publishedPort)
	}
	networkCount := nativeDockerNetworkCount(t, ctx, thirdName)
	if networkCount < 2 {
		t.Fatalf("unpublished Redis fixture is attached to %d Docker networks, want at least 2", networkCount)
	}
	firstID := nativeDockerContainerID(t, ctx, firstName)
	secondID := nativeDockerContainerID(t, ctx, secondName)
	thirdID := nativeDockerContainerID(t, ctx, thirdName)
	fourthID := nativeDockerContainerID(t, ctx, fourthName)
	t.Logf("PR05_SCENARIO mapped_host_port=%d override_host_port=%d container_networks=%d unpublished_container=%s", publishedPort, overridePublishedPort, networkCount, thirdID)
	if firstID == secondID {
		t.Fatalf("Docker returned duplicate resource identities %q", firstID)
	}

	serviceCtx, stopService := context.WithCancel(ctx)
	serviceDone := make(chan error, 1)
	collectorLogs := &nativeCollectorLogReporter{health: agenthealth.NewReporter(agenthealth.RuntimeObservation{CollectorState: agenthealth.CollectorStateStarting})}
	go func() {
		serviceDone <- runRuntimeWithReconciliation(serviceCtx, statePath, func(context.Context, string, agentstate.TeardownAction, []agentstate.OwnedResource) error { return nil }, collectorLogs, bootstrap.OSCommandRunner{})
	}()
	t.Cleanup(func() {
		stopService()
		select {
		case <-serviceDone:
		case <-time.After(10 * time.Second):
			t.Error("Agent runtime did not stop")
		}
	})
	waitForNativeDelivery(t, ctx, serviceDone, sinkOutput, collectorLogs, nodeID, firstID, secondID, fourthID)
	assertNativeReceiverUniqueness(t, collectorLogs.output())
	if duplicates := nativeDuplicateMetricSamples(sinkOutput.String()); len(duplicates) > 0 {
		t.Fatalf("duplicate Redis OTLP points detected: %v", duplicates)
	}
	inventory, err := store.InventoryRepository().Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	for _, resourceID := range []string{firstID, secondID, fourthID} {
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
	if runtime.GOOS == "linux" {
		want := expectedNativeNetworkEndpoint(unpublished.Endpoints)
		if diagnostic.State != monitoringprofile.EndpointResolved || diagnostic.Endpoint.NetworkName != want.NetworkName || diagnostic.Endpoint.Address != want.Address {
			t.Fatalf("multi-network endpoint diagnosis=%+v, want deterministic priority/name selection %+v", diagnostic, want)
		}
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
	waitForNativeDelivery(t, ctx, serviceDone, sinkOutput, collectorLogs, nodeID, replacementID, secondID, fourthID)
	assertNativeReceiverUniqueness(t, collectorLogs.output())
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
	if manager := strings.TrimSpace(os.Getenv(nativeIsolatedDockerManagerEnv)); manager != "" {
		verifyNativeDockerRecovery(t, ctx, manager, store, sinkOutput, collectorLogs, serviceDone, replacementID, secondID, fourthID)
	}
	if runtime.GOOS == "linux" {
		secondPort := nativeRedisPublishedPort(t, ctx, secondName, 6379)
		if output, err := nativeDockerCommand(ctx, "pause", secondName).CombinedOutput(); err != nil {
			t.Fatalf("pause Redis to induce an isolated scrape failure: %v\n%s", err, output)
		}
		waitForNativeCollectorFailure(t, serviceDone, collectorLogs, "redis/alternate_port", fmt.Sprintf("127.0.0.1:%d", secondPort))
		inventory, err := store.InventoryRepository().Snapshot()
		if err != nil {
			t.Fatalf("read Agent inventory after induced scrape failure: %v", err)
		}
		foundDiagnostic := false
		for _, resource := range inventory.Resources {
			if resource.Key.ProviderResourceID != secondID {
				continue
			}
			diagnostic := monitoringprofile.DiagnoseEndpointWithAttributes(monitoringprofile.RedisProfile(), runtime.GOOS, resource.Endpoints, collectorScrapeEvidenceFromSnapshot(collectorLogs.health.Snapshot()), resource.Attributes)
			if diagnostic.State != monitoringprofile.EndpointUnreachable || diagnostic.EvidenceTime.IsZero() {
				t.Fatalf("live endpoint failure diagnosis=%+v, want unreachable with timestamped OTel evidence", diagnostic)
			}
			foundDiagnostic = true
			t.Logf("PR05_ENDPOINT_DIAGNOSTIC resource=%s state=%s observed_at=%s code=%s", secondID, diagnostic.State, diagnostic.EvidenceTime.UTC().Format(time.RFC3339Nano), diagnostic.EvidenceCode)
		}
		if !foundDiagnostic {
			t.Fatalf("Agent inventory lost target %s before endpoint diagnosis", secondID)
		}
		if output, err := nativeDockerCommand(ctx, "unpause", secondName).CombinedOutput(); err != nil {
			t.Fatalf("unpause Redis after scrape failure: %v\n%s", err, output)
		}
	}
}

func verifyNativeDockerRecovery(t *testing.T, ctx context.Context, manager string, store agentstate.SQLiteStore, sink *lockedTemplateE2EBuffer, logs *nativeCollectorLogReporter, serviceDone <-chan error, resourceIDs ...string) {
	t.Helper()
	waitForNativeInventoryPresent(t, ctx, store, resourceIDs...)
	stateBefore, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	configBefore, err := os.ReadFile(stateBefore.Collector.ConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	pointCounts := make(map[string]int, len(resourceIDs))
	for _, resourceID := range resourceIDs {
		pointCounts[resourceID] = nativeMetricPointCount(sink.String(), resourceID)
	}
	instancesBefore := nativeCollectorInstances(logs.output())
	if len(instancesBefore) == 0 {
		t.Fatal("Collector instance identity was not captured before Docker outage")
	}
	if output, err := exec.CommandContext(ctx, "docker", "stop", manager).CombinedOutput(); err != nil {
		t.Fatalf("stop isolated Docker Engine manager: %v\n%s", err, output)
	}
	outageAt := waitForNativeProviderOutage(t, ctx, store.InventoryRepository())
	if missing := nativeInventoryMissingResources(store, resourceIDs...); len(missing) != 0 {
		t.Fatalf("Agent inventory did not preserve present Redis resources during failed Docker observations: %v", missing)
	}
	if output, err := exec.CommandContext(ctx, "docker", "start", manager).CombinedOutput(); err != nil {
		t.Fatalf("restart isolated Docker Engine manager: %v\n%s", err, output)
	}
	waitForNativeDockerReady(t, ctx, manager)
	for _, resourceID := range resourceIDs {
		name := nativeContainerNameForID(t, ctx, resourceID)
		if output, err := nativeDockerCommand(ctx, "start", name).CombinedOutput(); err != nil {
			t.Fatalf("start monitored Redis after daemon recovery: %v\n%s", err, output)
		}
	}
	waitForNativeProviderRecovery(t, ctx, store.InventoryRepository(), outageAt)
	waitForNativeInventoryPresent(t, ctx, store, resourceIDs...)
	waitForNativeNewMetrics(t, ctx, serviceDone, sink, pointCounts)
	stateAfter, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	configAfter, err := os.ReadFile(stateAfter.Collector.ConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	if stateAfter.AppliedGeneration != stateBefore.AppliedGeneration || !bytes.Equal(configBefore, configAfter) {
		t.Fatalf("Docker recovery unnecessarily activated Collector config: generation %d -> %d", stateBefore.AppliedGeneration, stateAfter.AppliedGeneration)
	}
	if !equalStringSets(instancesBefore, nativeCollectorInstances(logs.output())) {
		t.Fatalf("Docker recovery restarted the Collector: instances before=%v after=%v", instancesBefore, nativeCollectorInstances(logs.output()))
	}
	assertNativeReceiverUniqueness(t, logs.output())
	t.Logf("PR05_DOCKER_RECOVERY preserved_inventory=true receiver_data_resumed=true applied_generation=%d collector_instances=%d", stateAfter.AppliedGeneration, len(instancesBefore))
}

func waitForNativeProviderOutage(t *testing.T, ctx context.Context, repository interface {
	Snapshot() (discovery.InventorySnapshot, error)
}) time.Time {
	t.Helper()
	deadline := time.NewTimer(70 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		snapshot, err := repository.Snapshot()
		if err != nil {
			t.Fatalf("read Agent inventory during Docker outage: %v", err)
		}
		for _, provider := range snapshot.Providers {
			if provider.Enabled && provider.LastErrorCode != "" {
				return provider.LastAttemptAt
			}
		}
		select {
		case <-ctx.Done():
			t.Fatalf("context ended while waiting for Docker provider outage: %v", ctx.Err())
		case <-deadline.C:
			t.Fatal("Agent did not record a Docker provider error during isolated daemon outage")
		case <-ticker.C:
		}
	}
}

func waitForNativeProviderRecovery(t *testing.T, ctx context.Context, repository interface {
	Snapshot() (discovery.InventorySnapshot, error)
}, outageAt time.Time) {
	t.Helper()
	deadline := time.NewTimer(70 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		snapshot, err := repository.Snapshot()
		if err != nil {
			t.Fatalf("read Agent inventory during Docker recovery: %v", err)
		}
		for _, provider := range snapshot.Providers {
			if provider.Enabled && provider.LastErrorCode == "" && provider.LastSuccessAt.After(outageAt) {
				return
			}
		}
		select {
		case <-ctx.Done():
			t.Fatalf("context ended while waiting for Docker provider recovery: %v", ctx.Err())
		case <-deadline.C:
			t.Fatal("Agent did not record a successful Docker observation after recovery")
		case <-ticker.C:
		}
	}
}

func waitForNativeDockerReady(t *testing.T, ctx context.Context, manager string) {
	t.Helper()
	deadline := time.NewTimer(45 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	var lastPermissionError error
	var lastDockerError error
	for {
		permission := exec.CommandContext(ctx, "docker", "exec", manager, "chmod", "666", "/run/host/docker.sock")
		if output, err := permission.CombinedOutput(); err != nil {
			lastPermissionError = fmt.Errorf("chmod isolated Docker API socket: %w: %s", err, output)
		} else if err := nativeDockerCommand(ctx, "info").Run(); err == nil {
			return
		} else {
			lastDockerError = err
		}
		select {
		case <-ctx.Done():
			t.Fatalf("context ended waiting for isolated Docker Engine: %v", ctx.Err())
		case <-deadline.C:
			t.Fatalf("isolated Docker Engine did not become ready after restart: socket permission error=%v; Docker API error=%v", lastPermissionError, lastDockerError)
		case <-ticker.C:
		}
	}
}

func waitForNativeInventoryPresent(t *testing.T, ctx context.Context, store agentstate.SQLiteStore, resourceIDs ...string) {
	t.Helper()
	deadline := time.NewTimer(70 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		if nativeInventoryContainsAll(store, resourceIDs...) {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatalf("context ended waiting for Docker inventory recovery: %v", ctx.Err())
		case <-deadline.C:
			t.Fatal("Agent inventory did not converge after Docker recovery")
		case <-ticker.C:
		}
	}
}

func nativeInventoryContainsAll(store agentstate.SQLiteStore, resourceIDs ...string) bool {
	return len(nativeInventoryMissingResources(store, resourceIDs...)) == 0
}

func nativeInventoryMissingResources(store agentstate.SQLiteStore, resourceIDs ...string) []string {
	snapshot, err := store.InventoryRepository().Snapshot()
	if err != nil {
		return []string{"inventory_snapshot_error=" + err.Error()}
	}
	presence := make(map[string]discovery.Presence, len(resourceIDs))
	for _, resourceID := range resourceIDs {
		presence[resourceID] = "absent"
	}
	for _, resource := range snapshot.Resources {
		if _, expected := presence[resource.Key.ProviderResourceID]; expected {
			presence[resource.Key.ProviderResourceID] = resource.Presence
		}
	}
	var missing []string
	for resourceID, state := range presence {
		if state != discovery.PresencePresent {
			missing = append(missing, fmt.Sprintf("%s=%s", resourceID, state))
		}
	}
	sort.Strings(missing)
	return missing
}

func waitForNativeNewMetrics(t *testing.T, ctx context.Context, serviceDone <-chan error, sink *lockedTemplateE2EBuffer, before map[string]int) {
	t.Helper()
	deadline := time.NewTimer(70 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		output := sink.String()
		allRecovered := true
		for resourceID, previous := range before {
			if nativeMetricPointCount(output, resourceID) <= previous {
				allRecovered = false
				break
			}
		}
		if allRecovered {
			return
		}
		select {
		case err := <-serviceDone:
			t.Fatalf("Agent runtime stopped before post-recovery OTLP delivery: %v", err)
		case <-ctx.Done():
			t.Fatalf("context ended waiting for post-recovery OTLP delivery: %s", templateSinkDiagnostic(output))
		case <-deadline.C:
			t.Fatalf("timed out waiting for post-recovery OTLP delivery: %s", templateSinkDiagnostic(output))
		case <-ticker.C:
		}
	}
}

func nativeContainerNameForID(t *testing.T, ctx context.Context, resourceID string) string {
	t.Helper()
	output, err := nativeDockerCommand(ctx, "inspect", "--format", "{{.Name}}", resourceID).CombinedOutput()
	if err != nil {
		t.Fatalf("resolve Docker container name for %s: %v\n%s", resourceID, err, output)
	}
	return strings.TrimPrefix(strings.TrimSpace(string(output)), "/")
}

func nativeMetricPointCount(output, resourceID string) int {
	count := 0
	for _, block := range strings.Split(output, "Resource attributes:")[1:] {
		attributes := strings.SplitN(block, "ScopeMetrics #", 2)[0]
		if nativeStringAttribute(attributes, "resource_id") == resourceID && strings.Contains(block, "Name: redis.clients.connected") {
			count++
		}
	}
	return count
}

func nativeCollectorInstances(lines []string) []string {
	instances := make(map[string]struct{})
	for _, line := range lines {
		start := strings.IndexByte(line, '{')
		if start < 0 {
			continue
		}
		var fields map[string]any
		if json.Unmarshal([]byte(line[start:]), &fields) != nil {
			continue
		}
		resource, _ := fields["resource"].(map[string]any)
		instance, _ := resource["service.instance.id"].(string)
		if instance != "" {
			instances[instance] = struct{}{}
		}
	}
	result := make([]string, 0, len(instances))
	for instance := range instances {
		result = append(result, instance)
	}
	sort.Strings(result)
	return result
}

func equalStringSets(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func startNativeRedisContainerWithoutPublishedPort(t *testing.T, ctx context.Context, name string) {
	t.Helper()
	args := []string{"run", "--detach", "--name", name, "--label", "ar-imms.monitoring.enabled=true", "--label", "ar-imms.monitoring.profile=redis", "--label", "ar-imms.workload.id=cache-service", "redis:7-alpine"}
	if output, err := nativeDockerCommand(ctx, args...).CombinedOutput(); err != nil {
		t.Fatalf("start unpublished Redis fixture: %v\n%s", err, output)
	}
}

func nativeRedisPublishedPort(t *testing.T, ctx context.Context, name string, privatePort uint16) uint16 {
	t.Helper()
	output, err := nativeDockerCommand(ctx, "port", name, fmt.Sprintf("%d/tcp", privatePort)).CombinedOutput()
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

func expectedNativeNetworkEndpoint(endpoints []discovery.Endpoint) discovery.Endpoint {
	candidates := make([]discovery.Endpoint, 0)
	for _, endpoint := range endpoints {
		if endpoint.Scope == discovery.EndpointContainer && endpoint.Provenance == "container_network" && endpoint.ContainerPort == 6379 {
			candidates = append(candidates, endpoint)
		}
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].NetworkPriority != candidates[j].NetworkPriority {
			return candidates[i].NetworkPriority > candidates[j].NetworkPriority
		}
		return candidates[i].NetworkName < candidates[j].NetworkName
	})
	if len(candidates) == 0 {
		return discovery.Endpoint{}
	}
	return candidates[0]
}

type nativeCollectorLogReporter struct {
	health *agenthealth.Reporter
	mu     sync.Mutex
	lines  []string
}

func (r *nativeCollectorLogReporter) Report(observation agenthealth.RuntimeObservation) {
	r.health.Report(observation)
}

func (r *nativeCollectorLogReporter) ReportCollectorOutput(line string) {
	r.mu.Lock()
	r.lines = append(r.lines, line)
	r.mu.Unlock()
	r.health.ReportCollectorOutput(line)
}

func (r *nativeCollectorLogReporter) output() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.lines...)
}

func waitForNativeCollectorFailure(t *testing.T, serviceDone <-chan error, reporter *nativeCollectorLogReporter, receiver, endpoint string) {
	t.Helper()
	deadline := time.NewTimer(35 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		for _, failure := range reporter.health.Snapshot().CollectorFailures {
			if failure.Receiver == receiver && failure.Endpoint == endpoint && failure.Code == "connection_failed" {
				t.Logf("PR05_ENDPOINT_FAILURE receiver=%s endpoint=%s code=%s", failure.Receiver, failure.Endpoint, failure.Code)
				return
			}
		}
		select {
		case err := <-serviceDone:
			t.Fatalf("Agent runtime exited before endpoint failure evidence: %v", err)
		case <-deadline.C:
			t.Fatalf("timed out waiting for endpoint failure receiver=%s endpoint=%s; collector output=%s", receiver, endpoint, strings.Join(reporter.output(), "\n"))
		case <-ticker.C:
		}
	}
}

func nativeDockerNetworkCount(t *testing.T, ctx context.Context, name string) int {
	t.Helper()
	output, err := nativeDockerCommand(ctx, "inspect", "--format", "{{len .NetworkSettings.Networks}}", name).CombinedOutput()
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
	startNativeRedisContainerAtPort(t, ctx, name, 6379, "cache-service", true)
}

func startNativeRedisContainerAtPort(t *testing.T, ctx context.Context, name string, privatePort uint16, workloadID string, publish bool) {
	t.Helper()
	args := []string{"run", "--detach", "--name", name, "--label", "ar-imms.monitoring.enabled=true", "--label", "ar-imms.monitoring.profile=redis", "--label", "ar-imms.workload.id=" + workloadID}
	if privatePort != 6379 {
		args = append(args, "--label", "ar-imms.monitoring.port="+fmt.Sprint(privatePort))
	}
	if publish {
		args = append(args, "--publish", fmt.Sprintf("127.0.0.1::%d", privatePort))
	}
	args = append(args, "redis:7-alpine", "redis-server", "--port", fmt.Sprint(privatePort))
	if output, err := nativeDockerCommand(ctx, args...).CombinedOutput(); err != nil {
		t.Fatalf("start Redis fixture: %v\n%s", err, output)
	}
}

func stopNativeRedisContainer(name string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_ = nativeDockerCommand(ctx, "rm", "--force", name).Run()
}

func nativeDockerContainerID(t *testing.T, ctx context.Context, name string) string {
	t.Helper()
	output, err := nativeDockerCommand(ctx, "inspect", "--format", "{{.Id}}", name).CombinedOutput()
	if err != nil {
		t.Fatalf("inspect Redis fixture identity: %v\n%s", err, output)
	}
	return strings.TrimSpace(string(output))
}

func nativeDockerCommand(ctx context.Context, args ...string) *exec.Cmd {
	commandArgs := make([]string, 0, len(args)+2)
	if host := strings.TrimSpace(os.Getenv(nativeFixtureDockerHostEnv)); host != "" {
		commandArgs = append(commandArgs, "--host", host)
	}
	commandArgs = append(commandArgs, args...)
	return exec.CommandContext(ctx, "docker", commandArgs...)
}

func waitForNativeDelivery(t *testing.T, ctx context.Context, serviceDone <-chan error, sink *lockedTemplateE2EBuffer, collectorLogs *nativeCollectorLogReporter, nodeID string, resourceIDs ...string) {
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
			t.Fatalf("Agent runtime exited before native Redis telemetry delivery: %v; sink=%s; collector=%s", err, templateSinkDiagnostic(output), strings.Join(collectorLogs.output(), "\n"))
		case <-ctx.Done():
			t.Fatalf("timed out waiting for native Redis telemetry; sink=%s; collector=%s", templateSinkDiagnostic(output), strings.Join(collectorLogs.output(), "\n"))
		case <-deadline.C:
			t.Fatalf("timed out waiting for native Redis telemetry; sink=%s; collector=%s", templateSinkDiagnostic(output), strings.Join(collectorLogs.output(), "\n"))
		case <-poll.C:
		}
	}
}

func nativeSinkContainsIdentity(output, nodeID, resourceID string) bool {
	for _, block := range strings.Split(output, "Resource attributes:")[1:] {
		attributes := strings.SplitN(block, "ScopeMetrics #", 2)[0]
		if strings.Contains(block, "redis.clients.connected") && strings.Contains(attributes, "node_id: Str("+nodeID+")") && strings.Contains(attributes, "resource_id: Str("+resourceID+")") && strings.Contains(attributes, "workload_id: Str(") && strings.Contains(attributes, "container.name: Str(") {
			return true
		}
	}
	return false
}

func assertNativeReceiverUniqueness(t *testing.T, lines []string) {
	t.Helper()
	active := make(map[string]struct{})
	for _, line := range lines {
		start := strings.IndexByte(line, '{')
		if start < 0 {
			continue
		}
		var fields map[string]any
		if json.Unmarshal([]byte(line[start:]), &fields) != nil {
			continue
		}
		component, _ := fields["otelcol.component.id"].(string)
		if component != "receiver_creator" {
			continue
		}
		endpointID, _ := fields["endpoint_id"].(string)
		if endpointID == "" {
			continue
		}
		resource, _ := fields["resource"].(map[string]any)
		instanceID, _ := resource["service.instance.id"].(string)
		key := instanceID + "/" + endpointID
		if strings.Contains(line, "starting receiver") {
			if _, exists := active[key]; exists {
				t.Fatalf("Receiver Creator started duplicate active receiver %s", key)
			}
			active[key] = struct{}{}
		} else if strings.Contains(line, "stopping receiver") {
			delete(active, key)
		}
	}
}

func nativeDuplicateMetricSamples(output string) []string {
	counts := make(map[string]int)
	for _, resource := range strings.Split(output, "Resource attributes:")[1:] {
		attributes := strings.SplitN(resource, "ScopeMetrics #", 2)[0]
		resourceID := nativeStringAttribute(attributes, "resource_id")
		if resourceID == "" {
			continue
		}
		for _, metric := range strings.Split(resource, "Metric #")[1:] {
			if !strings.Contains(metric, "Name: redis.clients.connected") {
				continue
			}
			timestamp := nativeLineValue(metric, "Timestamp:")
			if timestamp != "" {
				counts[resourceID+"/"+timestamp]++
			}
		}
	}
	duplicates := make([]string, 0)
	for key, count := range counts {
		if count > 1 {
			duplicates = append(duplicates, key)
		}
	}
	sort.Strings(duplicates)
	return duplicates
}

func nativeStringAttribute(attributes, name string) string {
	for _, line := range strings.Split(attributes, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "-> "+name+": Str(") && strings.HasSuffix(line, ")") {
			return strings.TrimSuffix(strings.TrimPrefix(line, "-> "+name+": Str("), ")")
		}
	}
	return ""
}

func nativeLineValue(block, prefix string) string {
	for _, line := range strings.Split(block, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(line, prefix))
		}
	}
	return ""
}
