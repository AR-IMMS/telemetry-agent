package main

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ar-imms/telemetry-agent/internal/agentstate"
	"github.com/ar-imms/telemetry-agent/internal/bootstrap"
	"github.com/ar-imms/telemetry-agent/internal/collectorruntime"
	"github.com/ar-imms/telemetry-agent/internal/discovery"
	discoverydocker "github.com/ar-imms/telemetry-agent/internal/discovery/docker"
	"github.com/ar-imms/telemetry-agent/internal/integration"
)

const composeLifecycleE2EEnv = "AR_IMMS_S4_COMPOSE_LIFECYCLE_E2E"

// TestLiveComposeRedisLifecycleConvergesWithoutGhostInstances verifies real Compose
// restart, down/up recreation, force recreation, cross-project identity, and convergence.
func TestLiveComposeRedisLifecycleConvergesWithoutGhostInstances(t *testing.T) {
	if os.Getenv(composeLifecycleE2EEnv) != "1" {
		t.Skip("set " + composeLifecycleE2EEnv + "=1 to run isolated Docker Compose lifecycle E2E")
	}
	binary := strings.TrimSpace(os.Getenv("OTELCOL_CONTRIB_BINARY"))
	if binary == "" {
		t.Fatal("OTELCOL_CONTRIB_BINARY must point to the verified pinned Collector binary")
	}
	if _, err := os.Stat(binary); err != nil {
		t.Fatalf("inspect pinned Collector binary: %v", err)
	}
	endpoint := strings.TrimSpace(os.Getenv(dockerDiscoveryEndpointEnv))
	if endpoint == "" {
		var err error
		endpoint, err = dockerContextEndpoint()
		if err != nil {
			t.Fatal(err)
		}
	}
	if _, err := discoverydocker.NewProvider(endpoint); err != nil {
		t.Fatalf("validate local Docker endpoint: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 9*time.Minute)
	defer cancel()
	root := t.TempDir()
	projectA := fmt.Sprintf("agentqa%d-a", time.Now().UnixNano())
	projectB := fmt.Sprintf("agentqa%d-b", time.Now().UnixNano())
	composeA := writeLifecycleComposeFile(t, root, "project-a", true, reserveComposeHostPort(t))
	composeB := writeLifecycleComposeFile(t, root, "project-b", false, reserveComposeHostPort(t))
	fixtureA := composeLifecycleFixture{project: projectA, file: composeA}
	fixtureB := composeLifecycleFixture{project: projectB, file: composeB}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cleanupCancel()
		fixtureA.down(cleanupCtx)
		fixtureB.down(cleanupCtx)
	})
	fixtureA.up(ctx, t)
	fixtureB.up(ctx, t)

	statePath := filepath.Join(root, "state.db")
	configRoot, err := filepath.Abs(filepath.Join("..", "..", "configs"))
	if err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(root, "agent", "otel.yaml")
	store := agentstate.NewSQLiteStore(statePath)
	initial := agentstate.State{
		Collector: agentstate.CollectorContext{
			ConfigRoot: configRoot, BinaryPath: binary, ConfigPath: configPath,
			GatewayEndpoint: "127.0.0.1:4317", HealthEndpoint: defaultHealthEndpoint,
		},
		DesiredGeneration: 1, ActivatedGeneration: 1, AppliedGeneration: 1,
		Dependencies: map[string]agentstate.DependencyState{},
	}
	if err := store.Save(initial); err != nil {
		t.Fatal(err)
	}
	baseline, err := newRuntimeBaseline(statePath)
	if err != nil {
		t.Fatal(err)
	}
	baselineSnapshot, err := baseline.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := collectorruntime.StaticCatalog(baseline.platform)
	if err != nil {
		t.Fatal(err)
	}
	cleanCandidate, err := collectorruntime.NewCompiler(catalog).Compile(baselineSnapshot.Input, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(configPath), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, cleanCandidate.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}

	registry, err := productionIntegrationRegistry()
	if err != nil {
		t.Fatal(err)
	}
	templateService := newRuntimeTemplateServiceForTest(t, statePath, registry)
	templateA := composeProjectRedisTemplate("redis-project-a", projectA)
	templateB := composeProjectRedisTemplate("redis-project-b", projectB)
	if _, _, err := templateService.Put(templateA, 0); err != nil {
		t.Fatal(err)
	}
	if _, _, err := templateService.Put(templateB, 0); err != nil {
		t.Fatal(err)
	}
	provider, err := discoverydocker.NewProvider(endpoint)
	if err != nil {
		t.Fatal(err)
	}
	coordinator, err := discovery.NewCoordinator(store.InventoryRepository(), []discovery.DiscoveryProvider{provider}, discovery.DefaultPolicy(), 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	materializer, err := newRuntimeTemplateMaterializer(statePath)
	if err != nil {
		t.Fatal(err)
	}
	runDiscoveryCycle := func() discovery.InventorySnapshot {
		t.Helper()
		cycle, err := coordinator.DiscoverOnce(ctx)
		if err != nil || !discoveryCycleSucceeded(cycle) {
			t.Fatalf("complete Docker discovery cycle failed: result=%+v err=%v", cycle, err)
		}
		if err := materializer.Refresh(ctx); err != nil {
			t.Fatal(err)
		}
		inventory, err := store.InventoryRepository().Snapshot()
		if err != nil {
			t.Fatal(err)
		}
		return inventory
	}

	firstInventory := runDiscoveryCycle()
	oldA := fixtureA.containerID(ctx, t)
	stableB := fixtureB.containerID(ctx, t)
	assertComposeInventory(t, firstInventory, projectA, oldA, true)
	assertComposeInventory(t, firstInventory, projectB, stableB, false)
	assertComposeDesired(t, store, []string{oldA, stableB}, true)
	assertEnabledComposeTemplateResources(t, store, []string{oldA, stableB})

	// Keep the production runtime's Discovery loop disabled in this E2E. The same
	// Coordinator and materializer are driven explicitly so the two-snapshot grace
	// window is deterministic; S3 and M2 still run through their production service path.
	t.Setenv(dockerDiscoveryEndpointEnv, "")
	t.Setenv("OTELCOL_CONTRIB_BINARY", binary)
	sinkOutput := &lockedTemplateE2EBuffer{}
	sinkEndpoint := startTemplateOTLPSink(t, ctx, binary, root, sinkOutput)
	state, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	state.Collector.GatewayEndpoint = sinkEndpoint
	if err := store.Save(state); err != nil {
		t.Fatal(err)
	}
	serviceCtx, stopService := context.WithCancel(ctx)
	serviceDone := make(chan error, 1)
	go func() {
		serviceDone <- runRuntimeWithReconciliation(serviceCtx, statePath, func(context.Context, string, agentstate.TeardownAction, []agentstate.OwnedResource) error {
			return nil
		}, nil, bootstrap.OSCommandRunner{})
	}()
	t.Cleanup(stopService)

	initialState, initialConfig := waitForTemplateActivation(t, ctx, store, configPath, initial.AppliedGeneration, true)
	initialEndpoints := composeTemplateEndpoints(t, store, []string{oldA, stableB})
	waitForComposeTelemetry(t, ctx, serviceDone, sinkOutput, initialEndpoints)
	if got := countRedisReceiverDefinitions(initialConfig); got != 2 {
		t.Fatalf("initial Collector config has %d Redis receivers, want one per Compose project: %s", got, initialConfig)
	}

	// A normal restart keeps Docker's container identity and the generated instance.
	beforeRestart, err := store.IntegrationRepository().Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	fixtureB.run(ctx, t, "restart", "redis")
	if got := fixtureB.containerID(ctx, t); got != stableB {
		t.Fatalf("ordinary restart changed container identity: before=%s after=%s", stableB, got)
	}
	restartInventory := runDiscoveryCycle()
	_ = restartInventory
	afterRestart, err := store.IntegrationRepository().Snapshot()
	if err != nil || afterRestart.Revision != beforeRestart.Revision {
		t.Fatalf("unchanged container restart changed generated desired state: before=%d after=%d before_instances=%+v after_instances=%+v err=%v", beforeRestart.Revision, afterRestart.Revision, beforeRestart.Instances, afterRestart.Instances, err)
	}
	if state, err := store.Load(); err != nil || state.AppliedGeneration != initialState.AppliedGeneration {
		t.Fatalf("unchanged restart caused an unnecessary Collector activation: before=%d after=%d err=%v", initialState.AppliedGeneration, state.AppliedGeneration, err)
	}

	// Compose down/up replaces every container. No Discovery snapshot is taken
	// while the project is down, so this deterministically measures the first miss.
	fixtureA.down(ctx)
	fixtureA.up(ctx, t)
	newA := fixtureA.containerID(ctx, t)
	if newA == oldA {
		t.Fatalf("Compose recreation reused Docker container ID %q", oldA)
	}
	firstMissInventory := runDiscoveryCycle()
	assertComposePresence(t, firstMissInventory, oldA, discovery.PresencePresent, 1)
	assertComposePresence(t, firstMissInventory, newA, discovery.PresencePresent, 0)
	assertComposeDesired(t, store, []string{oldA, newA, stableB}, true)
	assertEnabledComposeTemplateResources(t, store, []string{oldA, newA, stableB})
	afterRestartState, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	overlapState, overlapConfig := waitForRedisReceiverCount(t, ctx, serviceDone, store, configPath, afterRestartState.AppliedGeneration, 3)
	if !bytes.Contains(overlapConfig, []byte("hostmetrics")) || !bytes.Contains(overlapConfig, []byte("otlp/gateway")) {
		t.Fatalf("first-miss overlap candidate lost host/Gateway configuration: %s", overlapConfig)
	}
	t.Logf("measured permitted first-miss overlap: old=%s and new=%s both active in applied generation=%d", oldA, newA, overlapState.AppliedGeneration)

	secondMissInventory := runDiscoveryCycle()
	assertComposePresence(t, secondMissInventory, oldA, discovery.PresenceMissing, 2)
	assertComposeDesired(t, store, []string{newA, stableB}, true)
	assertEnabledComposeTemplateResources(t, store, []string{newA, stableB})
	finalState, finalConfig := waitForRedisReceiverCount(t, ctx, serviceDone, store, configPath, overlapState.AppliedGeneration, 2)
	assertComposeDesired(t, store, []string{oldA}, false)
	if !bytes.Contains(finalConfig, []byte("hostmetrics")) || !bytes.Contains(finalConfig, []byte("otlp/gateway")) {
		t.Fatalf("converged config retained old resource or lost baseline components: %s", finalConfig)
	}
	activeEndpoints := composeTemplateEndpoints(t, store, []string{newA, stableB})
	waitForComposeTelemetry(t, ctx, serviceDone, sinkOutput, activeEndpoints)

	// Force recreation independently exercises a new container identity while
	// the other Compose project's Redis remains monitored.
	fixtureB.run(ctx, t, "up", "--detach", "--force-recreate", "redis")
	newB := fixtureB.containerID(ctx, t)
	if newB == stableB {
		t.Fatalf("--force-recreate reused Docker container ID %q", stableB)
	}
	forceFirstMiss := runDiscoveryCycle()
	assertComposePresence(t, forceFirstMiss, stableB, discovery.PresencePresent, 1)
	assertComposePresence(t, forceFirstMiss, newB, discovery.PresencePresent, 0)
	assertComposeDesired(t, store, []string{newA, stableB, newB}, true)
	assertEnabledComposeTemplateResources(t, store, []string{newA, stableB, newB})
	forceOverlapState, forceOverlapConfig := waitForRedisReceiverCount(t, ctx, serviceDone, store, configPath, finalState.AppliedGeneration, 3)
	if !bytes.Contains(forceOverlapConfig, []byte("hostmetrics")) || !bytes.Contains(forceOverlapConfig, []byte("otlp/gateway")) {
		t.Fatalf("force-recreate overlap candidate lost host/Gateway configuration: %s", forceOverlapConfig)
	}
	t.Logf("measured force-recreate first-miss overlap: old=%s and new=%s active in applied generation=%d", stableB, newB, forceOverlapState.AppliedGeneration)

	forceSecondMiss := runDiscoveryCycle()
	assertComposePresence(t, forceSecondMiss, stableB, discovery.PresenceMissing, 2)
	assertComposeDesired(t, store, []string{stableB}, false)
	assertComposeDesired(t, store, []string{newA, newB}, true)
	assertEnabledComposeTemplateResources(t, store, []string{newA, newB})
	finalState, finalConfig = waitForRedisReceiverCount(t, ctx, serviceDone, store, configPath, forceOverlapState.AppliedGeneration, 2)
	if !bytes.Contains(finalConfig, []byte("hostmetrics")) || !bytes.Contains(finalConfig, []byte("otlp/gateway")) {
		t.Fatalf("force-recreate convergence lost host/Gateway configuration: %s", finalConfig)
	}
	activeEndpoints = composeTemplateEndpoints(t, store, []string{newA, newB})
	waitForComposeTelemetry(t, ctx, serviceDone, sinkOutput, activeEndpoints)

	var activeOut, activeErr bytes.Buffer
	if code := runInventory(context.Background(), []string{"list", "--active-only", "--state-path", statePath}, &activeOut, &activeErr); code != 0 {
		t.Fatalf("active inventory list code=%d stderr=%s", code, activeErr.String())
	}
	if strings.Contains(activeOut.String(), oldA) || strings.Contains(activeOut.String(), stableB) || !strings.Contains(activeOut.String(), newA) || !strings.Contains(activeOut.String(), newB) {
		t.Fatalf("active inventory view is inconsistent after convergence: %s", activeOut.String())
	}
	var historyOut, historyErr bytes.Buffer
	if code := runInventory(context.Background(), []string{"list", "--state-path", statePath}, &historyOut, &historyErr); code != 0 {
		t.Fatalf("inventory history list code=%d stderr=%s", code, historyErr.String())
	}
	if !strings.Contains(historyOut.String(), oldA) || !strings.Contains(historyOut.String(), stableB) || !strings.Contains(historyOut.String(), "missing") {
		t.Fatalf("inventory history did not retain the old missing resources: %s", historyOut.String())
	}

	// A second project's Redis survives project A recreation and remains independently monitored.
	stopService()
	select {
	case err := <-serviceDone:
		if err != nil {
			t.Fatalf("stop Agent runtime: %v", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("Agent runtime did not stop")
	}
	if finalState.AppliedGeneration <= initialState.AppliedGeneration {
		t.Fatalf("Compose recreation did not advance applied generation: initial=%d final=%d", initialState.AppliedGeneration, finalState.AppliedGeneration)
	}
}

type composeLifecycleFixture struct {
	project string
	file    string
}

func (f composeLifecycleFixture) run(ctx context.Context, t *testing.T, args ...string) string {
	t.Helper()
	commandArgs := []string{"compose", "--project-name", f.project, "--file", f.file}
	commandArgs = append(commandArgs, args...)
	output, err := exec.CommandContext(ctx, "docker", commandArgs...).CombinedOutput()
	if err != nil {
		t.Fatalf("docker %s failed: %v\n%s", strings.Join(commandArgs, " "), err, output)
	}
	return strings.TrimSpace(string(output))
}

func (f composeLifecycleFixture) up(ctx context.Context, t *testing.T) {
	f.run(ctx, t, "up", "--detach", "--remove-orphans")
}

func (f composeLifecycleFixture) down(ctx context.Context) {
	commandArgs := []string{"compose", "--project-name", f.project, "--file", f.file, "down", "--volumes", "--remove-orphans"}
	_ = exec.CommandContext(ctx, "docker", commandArgs...).Run()
}

func (f composeLifecycleFixture) containerID(ctx context.Context, t *testing.T) string {
	t.Helper()
	value := f.run(ctx, t, "ps", "--quiet", "redis")
	lines := strings.Fields(value)
	if len(lines) != 1 {
		t.Fatalf("Compose project %s has %d Redis containers, want one: %q", f.project, len(lines), value)
	}
	return lines[0]
}

func reserveComposeHostPort(t *testing.T) uint16 {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := uint16(listener.Addr().(*net.TCPAddr).Port)
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	return port
}

func writeLifecycleComposeFile(t *testing.T, root, name string, includeDependencies bool, redisHostPort uint16) string {
	t.Helper()
	directory := filepath.Join(root, name)
	if err := os.MkdirAll(directory, 0700); err != nil {
		t.Fatal(err)
	}
	content := fmt.Sprintf(`services:
  redis:
    image: redis:7-alpine
    ports:
      - "127.0.0.1:%d:6379"
`, redisHostPort)
	if includeDependencies {
		content += `  postgres:
    image: postgres:17-alpine
    environment:
      POSTGRES_PASSWORD: isolated-test-only
  python-app:
    image: python:3.12-alpine
    command: ["python", "-u", "-c", "import time; time.sleep(3600)"]
`
	}
	file := filepath.Join(directory, "compose.yaml")
	if err := os.WriteFile(file, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	return file
}

func dockerContextEndpoint() (string, error) {
	output, err := exec.Command("docker", "context", "inspect", "--format", "{{.Endpoints.docker.Host}}").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("inspect current Docker context: %w: %s", err, strings.TrimSpace(string(output)))
	}
	endpoint := strings.TrimSpace(string(output))
	if endpoint == "" {
		return "", fmt.Errorf("current Docker context has no endpoint")
	}
	return endpoint, nil
}

func composeProjectRedisTemplate(id, project string) integration.IntegrationTemplate {
	template := redisRuntimeTemplate()
	template.ID = id
	template.Selector.ExactAttributes["label.com.docker.compose.project"] = project
	return template
}

func assertComposeInventory(t *testing.T, snapshot discovery.InventorySnapshot, project, redisID string, dependencies bool) {
	t.Helper()
	foundRedis, foundPostgres, foundPython := false, false, false
	for _, resource := range snapshot.Resources {
		if resource.Attributes["label.com.docker.compose.project"] != project {
			continue
		}
		switch {
		case resource.Key.ProviderResourceID == redisID:
			foundRedis = resource.Presence == discovery.PresencePresent
		case resource.Attributes["container.image.repository"] == "docker.io/library/postgres":
			foundPostgres = resource.Presence == discovery.PresencePresent
		case resource.Attributes["container.image.repository"] == "docker.io/library/python":
			foundPython = resource.Presence == discovery.PresencePresent
		}
	}
	if !foundRedis || (dependencies && (!foundPostgres || !foundPython)) {
		t.Fatalf("Compose inventory missing expected project resources: project=%s redis=%t postgres=%t python=%t", project, foundRedis, foundPostgres, foundPython)
	}
}

func assertComposePresence(t *testing.T, snapshot discovery.InventorySnapshot, resourceID string, want discovery.Presence, missingSnapshots uint32) {
	t.Helper()
	for _, resource := range snapshot.Resources {
		if resource.Key.ProviderResourceID == resourceID {
			if resource.Presence != want || resource.MissingSnapshots != missingSnapshots {
				t.Fatalf("resource %s presence=%s missing_snapshots=%d, want %s/%d", resourceID, resource.Presence, resource.MissingSnapshots, want, missingSnapshots)
			}
			return
		}
	}
	t.Fatalf("inventory does not retain resource %s", resourceID)
}

func assertComposeDesired(t *testing.T, store agentstate.SQLiteStore, resourceIDs []string, enabled bool) {
	t.Helper()
	snapshot, err := store.IntegrationRepository().Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	wanted := make(map[string]struct{}, len(resourceIDs))
	for _, id := range resourceIDs {
		wanted[id] = struct{}{}
	}
	found := make(map[string]bool, len(resourceIDs))
	for _, instance := range snapshot.Instances {
		if instance.Source != integration.SourceTemplate || instance.Binding == nil {
			continue
		}
		if _, ok := wanted[instance.Binding.ResourceID]; ok {
			found[instance.Binding.ResourceID] = instance.Enabled
		}
	}
	for id := range wanted {
		if isEnabled, ok := found[id]; !ok || isEnabled != enabled {
			t.Fatalf("Template desired instance for %s found=%t enabled=%t want enabled=%t: %+v", id, ok, isEnabled, enabled, snapshot.Instances)
		}
	}
}

func assertEnabledComposeTemplateResources(t *testing.T, store agentstate.SQLiteStore, resourceIDs []string) {
	t.Helper()
	wanted := make(map[string]struct{}, len(resourceIDs))
	for _, id := range resourceIDs {
		wanted[id] = struct{}{}
	}
	snapshot, err := store.IntegrationRepository().Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	seen := make(map[string]struct{}, len(wanted))
	for _, instance := range snapshot.Instances {
		if instance.Source != integration.SourceTemplate || !instance.Enabled || instance.Binding == nil ||
			(instance.TemplateID != "redis-project-a" && instance.TemplateID != "redis-project-b") {
			continue
		}
		if _, expected := wanted[instance.Binding.ResourceID]; !expected {
			t.Fatalf("unexpected enabled Compose Template instance: %+v", instance)
		}
		seen[instance.Binding.ResourceID] = struct{}{}
	}
	if len(seen) != len(wanted) {
		t.Fatalf("enabled Compose Template resources=%v, want exactly %v", seen, resourceIDs)
	}
}

func composeTemplateEndpoints(t *testing.T, store agentstate.SQLiteStore, resourceIDs []string) map[string]string {
	t.Helper()
	snapshot, err := store.IntegrationRepository().Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	wanted := make(map[string]struct{}, len(resourceIDs))
	for _, id := range resourceIDs {
		wanted[id] = struct{}{}
	}
	endpoints := make(map[string]string, len(resourceIDs))
	for _, instance := range snapshot.Instances {
		if instance.Source != integration.SourceTemplate || !instance.Enabled || instance.Binding == nil {
			continue
		}
		if _, ok := wanted[instance.Binding.ResourceID]; !ok {
			continue
		}
		endpoint, ok := instance.Configuration["endpoint"].(string)
		if !ok || endpoint == "" {
			t.Fatalf("generated instance %s has no resolved endpoint", instance.ID)
		}
		endpoints[instance.Binding.ResourceID] = endpoint
	}
	if len(endpoints) != len(wanted) {
		t.Fatalf("resolved endpoints=%v, want one for each resource %v", endpoints, resourceIDs)
	}
	return endpoints
}

func waitForComposeTelemetry(t *testing.T, ctx context.Context, serviceDone <-chan error, sink *lockedTemplateE2EBuffer, endpoints map[string]string) {
	t.Helper()
	deadline := time.NewTimer(90 * time.Second)
	defer deadline.Stop()
	poll := time.NewTicker(500 * time.Millisecond)
	defer poll.Stop()
	for !sinkContainsTemplateEndpoints(sink.String(), endpoints) {
		select {
		case err := <-serviceDone:
			t.Fatalf("Agent runtime stopped before Redis telemetry delivery: %v", err)
		case <-ctx.Done():
			t.Fatalf("timed out waiting for telemetry from current Compose endpoints: %s", templateSinkDiagnostic(sink.String()))
		case <-deadline.C:
			t.Fatalf("timed out waiting for telemetry from current Compose endpoints: %s", templateSinkDiagnostic(sink.String()))
		case <-poll.C:
		}
	}
}

func waitForRedisReceiverCount(t *testing.T, ctx context.Context, serviceDone <-chan error, store agentstate.SQLiteStore, configPath string, afterGeneration uint64, want int) (agentstate.State, []byte) {
	t.Helper()
	deadline := time.NewTimer(90 * time.Second)
	defer deadline.Stop()
	poll := time.NewTicker(500 * time.Millisecond)
	defer poll.Stop()
	for {
		state, stateErr := store.Load()
		content, configErr := os.ReadFile(configPath)
		if stateErr == nil && configErr == nil && state.AppliedGeneration > afterGeneration && state.AppliedGeneration == state.DesiredGeneration && countRedisReceiverDefinitions(content) == want {
			return state, content
		}
		select {
		case err := <-serviceDone:
			t.Fatalf("Agent runtime stopped before Collector config converged: %v", err)
		case <-ctx.Done():
			t.Fatalf("timed out waiting for %d Redis receivers: state=%+v read_error=%v", want, state, configErr)
		case <-deadline.C:
			t.Fatalf("timed out waiting for %d Redis receivers: state=%+v read_error=%v config=%s", want, state, configErr, content)
		case <-poll.C:
		}
	}
}

func countRedisReceiverDefinitions(content []byte) int {
	count := 0
	for _, line := range strings.Split(string(content), "\n") {
		component := strings.TrimSpace(line)
		if strings.HasPrefix(component, "redis/i-") && strings.HasSuffix(component, ":") {
			count++
		}
	}
	return count
}
