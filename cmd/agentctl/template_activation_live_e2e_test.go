package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ar-imms/telemetry-agent/internal/agentstate"
	"github.com/ar-imms/telemetry-agent/internal/app"
	"github.com/ar-imms/telemetry-agent/internal/bootstrap"
	"github.com/ar-imms/telemetry-agent/internal/collectorruntime"
	"github.com/ar-imms/telemetry-agent/internal/configuration"
	"github.com/ar-imms/telemetry-agent/internal/discovery"
	discoverydocker "github.com/ar-imms/telemetry-agent/internal/discovery/docker"
	"github.com/ar-imms/telemetry-agent/internal/integration"
)

const templateRuntimeE2EEnv = "AR_IMMS_S4_TEMPLATE_RUNTIME_E2E"

// TestLiveDockerTemplateConvergesThroughAgentRuntime proves a generated
// Template instance reaches the existing S3/M2 runtime and an isolated OTLP sink.
func TestLiveDockerTemplateConvergesThroughAgentRuntime(t *testing.T) {
	if os.Getenv(templateRuntimeE2EEnv) != "1" {
		t.Skip("set " + templateRuntimeE2EEnv + "=1 to run the isolated Collector E2E")
	}
	binary := strings.TrimSpace(os.Getenv("OTELCOL_CONTRIB_BINARY"))
	dockerEndpoint := strings.TrimSpace(os.Getenv(dockerDiscoveryEndpointEnv))
	containerIDs := templateRuntimeFixtureIDs()
	if binary == "" || dockerEndpoint == "" || len(containerIDs) == 0 {
		t.Fatal("verified Collector binary, explicit Docker endpoint, and at least one Redis container ID are required")
	}
	if _, err := os.Stat(binary); err != nil {
		t.Fatalf("inspect pinned Collector binary: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	tempDir := t.TempDir()
	sinkOutput := &lockedTemplateE2EBuffer{}
	sinkEndpoint := startTemplateOTLPSink(t, ctx, binary, tempDir, sinkOutput)
	statePath := filepath.Join(tempDir, "state.db")
	configRoot, err := filepath.Abs(filepath.Join("..", "..", "configs"))
	if err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(tempDir, "agent", "otel.yaml")
	store := agentstate.NewSQLiteStore(statePath)
	initial := agentstate.State{
		Collector: agentstate.CollectorContext{
			ConfigRoot: configRoot, BinaryPath: binary, ConfigPath: configPath,
			GatewayEndpoint: sinkEndpoint, HealthEndpoint: defaultHealthEndpoint,
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
	templateFile := filepath.Join(tempDir, "redis-monitoring.json")
	writeRuntimeTemplateFile(t, templateFile, redisRuntimeTemplate())
	runTemplateCLI(t, statePath, "create", "--file", templateFile, "--expected-revision", "0")
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if code := runIntegration(context.Background(), []string{"template", "inspect", "--state-path", statePath, "--id", "redis-auto", "--output", "json"}, stdout, stderr); code != 0 || !bytes.Contains(stdout.Bytes(), []byte(`"bindings"`)) {
		t.Fatalf("inspect persisted Template exit=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	templateSnapshot, err := templateService.Snapshot()
	if err != nil || len(templateSnapshot.Templates) != 1 {
		t.Fatalf("read created Template snapshot=%+v err=%v", templateSnapshot, err)
	}
	storedTemplate := templateSnapshot.Templates[0]
	provider, err := discoverydocker.NewProvider(dockerEndpoint)
	if err != nil {
		t.Fatal(err)
	}
	discoveryCoordinator, err := discovery.NewCoordinator(store.InventoryRepository(), []discovery.DiscoveryProvider{provider}, discovery.DefaultPolicy(), 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	cycle, err := discoveryCoordinator.DiscoverOnce(ctx)
	if err != nil || !app.DiscoveryCycleSucceeded(cycle) {
		t.Fatalf("discover live Docker resources: result=%+v err=%v", cycle, err)
	}
	materializer, err := newRuntimeTemplateMaterializer(statePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := materializer.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	desired, err := store.IntegrationRepository().Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	generated := make(map[string]string, len(containerIDs))
	for _, instance := range desired.Instances {
		if instance.Source == integration.SourceTemplate && instance.Binding != nil && instance.Enabled {
			for _, containerID := range containerIDs {
				if instance.Binding.ResourceID == containerID {
					endpoint, ok := instance.Configuration["endpoint"].(string)
					if !ok || endpoint == "" {
						t.Fatalf("generated instance for %q has no resolved endpoint: %+v", containerID, instance)
					}
					generated[containerID] = endpoint
				}
			}
		}
	}
	if len(generated) != len(containerIDs) {
		t.Fatalf("Template generated %d enabled instances for %d Redis containers: generated=%v", len(generated), len(containerIDs), generated)
	}

	t.Setenv("OTELCOL_CONTRIB_BINARY", binary)
	serviceCtx, stopService := context.WithCancel(ctx)
	serviceDone := make(chan error, 1)
	go func() {
		serviceDone <- runRuntimeWithReconciliation(serviceCtx, statePath, func(context.Context, string, agentstate.TeardownAction, []agentstate.OwnedResource) error {
			return nil
		}, nil, bootstrap.OSCommandRunner{})
	}()

	deadline := time.NewTimer(2 * time.Minute)
	defer deadline.Stop()
	poll := time.NewTicker(500 * time.Millisecond)
	defer poll.Stop()
	for {
		state, loadErr := store.Load()
		if loadErr != nil {
			stopService()
			t.Fatalf("read Agent runtime state: %v", loadErr)
		}
		configBytes, readErr := os.ReadFile(configPath)
		if readErr == nil && state.AppliedGeneration > initial.AppliedGeneration && strings.Count(string(configBytes), "redis/i-") >= len(containerIDs) {
			if !bytes.Contains(configBytes, []byte("hostmetrics")) || !bytes.Contains(configBytes, []byte("otlp/gateway")) {
				stopService()
				t.Fatalf("Template candidate did not preserve baseline host/Gateway components: %s", configBytes)
			}
			if sinkContainsTemplateEndpoints(sinkOutput.String(), generated) {
				break
			}
		}
		select {
		case runtimeErr := <-serviceDone:
			stopService()
			t.Fatalf("Agent runtime stopped before Template telemetry was delivered for every Redis endpoint: err=%v sink_evidence=%s", runtimeErr, templateSinkDiagnostic(sinkOutput.String()))
		case <-ctx.Done():
			stopService()
			t.Fatalf("timed out waiting for applied Template config and per-endpoint Redis metrics: sink_evidence=%s", templateSinkDiagnostic(sinkOutput.String()))
		case <-deadline.C:
			stopService()
			t.Fatalf("timed out waiting for applied Template config and per-endpoint Redis metrics: sink_evidence=%s", templateSinkDiagnostic(sinkOutput.String()))
		case <-poll.C:
		}
	}

	beforeUpdate, err := store.Load()
	if err != nil {
		stopService()
		t.Fatalf("read state before Template update: %v", err)
	}
	updatedTemplate := storedTemplate
	updatedTemplate.Settings = map[string]any{"collection_interval": "15s"}
	writeRuntimeTemplateFile(t, templateFile, updatedTemplate)
	runTemplateCLI(t, statePath, "update", "--file", templateFile, "--expected-revision", fmt.Sprint(storedTemplate.Revision))
	templateSnapshot, err = templateService.Snapshot()
	if err != nil || len(templateSnapshot.Templates) != 1 {
		stopService()
		t.Fatalf("read updated Template snapshot=%+v err=%v", templateSnapshot, err)
	}
	updatedTemplate = templateSnapshot.Templates[0]
	if err := materializer.Refresh(ctx); err != nil {
		stopService()
		t.Fatalf("materialize updated Redis Template: %v", err)
	}
	updateDeadline := time.NewTimer(60 * time.Second)
	defer updateDeadline.Stop()
	updatePoll := time.NewTicker(500 * time.Millisecond)
	defer updatePoll.Stop()
	var updatedConfig []byte
	for {
		state, loadErr := store.Load()
		configBytes, readErr := os.ReadFile(configPath)
		if loadErr == nil && readErr == nil && state.AppliedGeneration > beforeUpdate.AppliedGeneration &&
			bytes.Contains(configBytes, []byte("collection_interval: 15s")) {
			updatedConfig = configBytes
			break
		}
		select {
		case runtimeErr := <-serviceDone:
			stopService()
			t.Fatalf("Agent runtime stopped before updated Template was applied: err=%v", runtimeErr)
		case <-ctx.Done():
			stopService()
			t.Fatalf("timed out applying updated Template: state=%+v config_read_error=%v", state, readErr)
		case <-updateDeadline.C:
			stopService()
			t.Fatalf("timed out applying updated Template: state=%+v config_read_error=%v", state, readErr)
		case <-updatePoll.C:
		}
	}
	if !bytes.Contains(updatedConfig, []byte("hostmetrics")) || !bytes.Contains(updatedConfig, []byte("otlp/gateway")) {
		stopService()
		t.Fatalf("updated Template candidate lost host or Gateway components: %s", updatedConfig)
	}
	desiredBeforeNoOp, err := store.IntegrationRepository().Snapshot()
	if err != nil {
		stopService()
		t.Fatalf("read desired snapshot before no-op materialization: %v", err)
	}
	if err := materializer.Refresh(ctx); err != nil {
		stopService()
		t.Fatalf("repeat unchanged Template materialization: %v", err)
	}
	desiredAfterNoOp, err := store.IntegrationRepository().Snapshot()
	if err != nil || desiredAfterNoOp.Revision != desiredBeforeNoOp.Revision {
		stopService()
		t.Fatalf("unchanged materialization advanced desired revision: before=%d after=%d err=%v", desiredBeforeNoOp.Revision, desiredAfterNoOp.Revision, err)
	}
	steadyState, err := store.Load()
	if err != nil {
		stopService()
		t.Fatalf("read state after updated Template activation: %v", err)
	}
	steadyGeneration := steadyState.AppliedGeneration
	select {
	case runtimeErr := <-serviceDone:
		stopService()
		t.Fatalf("Agent runtime stopped during unchanged Template observation: err=%v", runtimeErr)
	case <-ctx.Done():
		stopService()
		t.Fatalf("timed out observing unchanged Template runtime")
	case <-time.After(app.DiscoveryPollInterval + app.ReconciliationPollInterval + time.Second):
	}
	finalSteadyState, err := store.Load()
	finalConfig, configErr := os.ReadFile(configPath)
	if err != nil || configErr != nil || finalSteadyState.AppliedGeneration != steadyGeneration || !bytes.Equal(finalConfig, updatedConfig) {
		stopService()
		t.Fatalf("unchanged Template caused another lifecycle change: generation=%d want=%d config_changed=%t state_err=%v config_err=%v", finalSteadyState.AppliedGeneration, steadyGeneration, !bytes.Equal(finalConfig, updatedConfig), err, configErr)
	}

	runTemplateCLI(t, statePath, "disable", "--id", updatedTemplate.ID, "--expected-revision", fmt.Sprint(updatedTemplate.Revision))
	templateSnapshot, err = templateService.Snapshot()
	if err != nil || len(templateSnapshot.Templates) != 1 {
		stopService()
		t.Fatalf("read disabled Template snapshot=%+v err=%v", templateSnapshot, err)
	}
	disabledTemplate := templateSnapshot.Templates[0]
	if err := materializer.Refresh(ctx); err != nil {
		stopService()
		t.Fatalf("materialize disabled Redis Template: %v", err)
	}
	disabledState, disabledConfig := waitForTemplateActivation(t, ctx, store, configPath, steadyGeneration, false)
	if !bytes.Contains(disabledConfig, []byte("hostmetrics")) || !bytes.Contains(disabledConfig, []byte("otlp/gateway")) {
		stopService()
		t.Fatalf("disabling Template removed baseline telemetry: %s", disabledConfig)
	}
	assertTemplateInstancesEnabled(t, store, false, containerIDs)

	runTemplateCLI(t, statePath, "enable", "--id", disabledTemplate.ID, "--expected-revision", fmt.Sprint(disabledTemplate.Revision))
	templateSnapshot, err = templateService.Snapshot()
	if err != nil || len(templateSnapshot.Templates) != 1 {
		stopService()
		t.Fatalf("read enabled Template snapshot=%+v err=%v", templateSnapshot, err)
	}
	enabledTemplate := templateSnapshot.Templates[0]
	if err := materializer.Refresh(ctx); err != nil {
		stopService()
		t.Fatalf("materialize re-enabled Redis Template: %v", err)
	}
	enabledState, enabledConfig := waitForTemplateActivation(t, ctx, store, configPath, disabledState.AppliedGeneration, true)
	if !bytes.Contains(enabledConfig, []byte("hostmetrics")) || !bytes.Contains(enabledConfig, []byte("otlp/gateway")) {
		stopService()
		t.Fatalf("re-enabling Template removed baseline telemetry: %s", enabledConfig)
	}
	assertTemplateInstancesEnabled(t, store, true, containerIDs)
	outputLengthBeforeRestart := len(sinkOutput.String())
	stopService()
	select {
	case runtimeErr := <-serviceDone:
		if runtimeErr != nil {
			t.Fatalf("stop Agent runtime before restart: %v", runtimeErr)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("Agent runtime did not stop before restart")
	}
	serviceCtx, stopService = context.WithCancel(ctx)
	serviceDone = make(chan error, 1)
	go func() {
		serviceDone <- runRuntimeWithReconciliation(serviceCtx, statePath, func(context.Context, string, agentstate.TeardownAction, []agentstate.OwnedResource) error {
			return nil
		}, nil, bootstrap.OSCommandRunner{})
	}()
	if err := waitForRestoredTemplateRuntime(ctx, store, configPath, sinkOutput, outputLengthBeforeRestart, true, serviceDone); err != nil {
		stopService()
		t.Fatal(err)
	}
	assertTemplateInstancesEnabled(t, store, true, containerIDs)

	runTemplateCLI(t, statePath, "delete", "--id", enabledTemplate.ID, "--expected-revision", fmt.Sprint(enabledTemplate.Revision))
	if err := materializer.Refresh(ctx); err != nil {
		stopService()
		t.Fatalf("materialize deleted Redis Template: %v", err)
	}
	_, deletedConfig := waitForTemplateActivation(t, ctx, store, configPath, enabledState.AppliedGeneration, false)
	if !bytes.Contains(deletedConfig, []byte("hostmetrics")) || !bytes.Contains(deletedConfig, []byte("otlp/gateway")) {
		stopService()
		t.Fatalf("deleting Template removed baseline telemetry: %s", deletedConfig)
	}
	assertTemplateInstancesEnabled(t, store, false, containerIDs)

	stopService()
	select {
	case err := <-serviceDone:
		if err != nil {
			t.Fatalf("stop Agent runtime cleanly: %v", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("Agent runtime did not stop after cancellation")
	}
	finalState, err := store.Load()
	if err != nil || finalState.AppliedGeneration <= initial.AppliedGeneration {
		t.Fatalf("M2 did not persist an applied generation: state=%+v err=%v", finalState, err)
	}
}

func waitForTemplateActivation(t *testing.T, ctx context.Context, store agentstate.SQLiteStore, configPath string, afterGeneration uint64, redisExpected bool) (agentstate.State, []byte) {
	t.Helper()
	deadline := time.NewTimer(60 * time.Second)
	defer deadline.Stop()
	poll := time.NewTicker(500 * time.Millisecond)
	defer poll.Stop()
	for {
		state, stateErr := store.Load()
		configBytes, configErr := os.ReadFile(configPath)
		containsRedis := configErr == nil && bytes.Contains(configBytes, []byte("redis/i-"))
		if stateErr == nil && configErr == nil && state.AppliedGeneration > afterGeneration && containsRedis == redisExpected {
			return state, configBytes
		}
		select {
		case <-ctx.Done():
			t.Fatalf("timed out waiting for Redis Template runtime state: redis_expected=%t state=%+v state_err=%v config_err=%v", redisExpected, state, stateErr, configErr)
		case <-deadline.C:
			t.Fatalf("timed out waiting for Redis Template runtime state: redis_expected=%t state=%+v state_err=%v config_err=%v", redisExpected, state, stateErr, configErr)
		case <-poll.C:
		}
	}
}

func assertTemplateInstancesEnabled(t *testing.T, store agentstate.SQLiteStore, enabled bool, resourceIDs []string) {
	t.Helper()
	snapshot, err := store.IntegrationRepository().Snapshot()
	if err != nil {
		t.Fatalf("read generated Integration instances: %v", err)
	}
	expected := make(map[string]struct{}, len(resourceIDs))
	for _, resourceID := range resourceIDs {
		expected[resourceID] = struct{}{}
	}
	seen := make(map[string]struct{}, len(resourceIDs))
	for _, instance := range snapshot.Instances {
		if instance.Source != integration.SourceTemplate || instance.Binding == nil {
			continue
		}
		if _, isFixture := expected[instance.Binding.ResourceID]; !isFixture {
			continue
		}
		seen[instance.Binding.ResourceID] = struct{}{}
		if instance.Enabled != enabled {
			t.Errorf("Template instance %q enabled=%t, want %t", instance.ID, instance.Enabled, enabled)
		}
	}
	if len(seen) != len(expected) {
		t.Errorf("Template fixture instance count=%d, want %d for resources %v", len(seen), len(expected), resourceIDs)
	}
}

func waitForRestoredTemplateRuntime(ctx context.Context, store agentstate.SQLiteStore, configPath string, sink *lockedTemplateE2EBuffer, previousOutputLength int, redisExpected bool, serviceDone <-chan error) error {
	deadline := time.NewTimer(60 * time.Second)
	defer deadline.Stop()
	poll := time.NewTicker(500 * time.Millisecond)
	defer poll.Stop()
	for {
		state, stateErr := store.Load()
		configBytes, configErr := os.ReadFile(configPath)
		output := sink.String()
		containsRedis := configErr == nil && bytes.Contains(configBytes, []byte("redis/i-"))
		if stateErr == nil && configErr == nil && state.DesiredGeneration == state.AppliedGeneration &&
			containsRedis == redisExpected && bytes.Contains(configBytes, []byte("hostmetrics")) &&
			bytes.Contains(configBytes, []byte("otlp/gateway")) && len(output) > previousOutputLength {
			return nil
		}
		select {
		case runtimeErr := <-serviceDone:
			return fmt.Errorf("Agent runtime stopped before post-restart telemetry: %v", runtimeErr)
		case <-ctx.Done():
			return fmt.Errorf("timed out restoring Agent Template runtime after restart: state=%+v state_err=%v config_err=%v", state, stateErr, configErr)
		case <-deadline.C:
			return fmt.Errorf("timed out restoring Agent Template runtime after restart: state=%+v state_err=%v config_err=%v", state, stateErr, configErr)
		case <-poll.C:
		}
	}
}

func templateRuntimeFixtureIDs() []string {
	value := strings.TrimSpace(os.Getenv("AR_IMMS_S4_TEMPLATE_E2E_CONTAINER_IDS"))
	if value == "" {
		value = strings.TrimSpace(os.Getenv("AR_IMMS_S4_TEMPLATE_E2E_CONTAINER_ID"))
	}
	if value == "" {
		return nil
	}
	seen := make(map[string]struct{})
	var ids []string
	for _, id := range strings.Split(value, ",") {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	return ids
}

func sinkContainsTemplateEndpoints(output string, generated map[string]string) bool {
	if len(generated) == 0 {
		return false
	}
	resourceBlocks := strings.Split(output, "Resource attributes:")
	for _, endpoint := range generated {
		_, port, err := net.SplitHostPort(endpoint)
		if err != nil {
			return false
		}
		attributed := false
		for _, block := range resourceBlocks[1:] {
			attributes := strings.SplitN(block, "ScopeMetrics #", 2)[0]
			if !strings.Contains(attributes, "server.port: Str("+port+")") {
				continue
			}
			if strings.Contains(block, "redis.clients.connected") {
				attributed = true
				break
			}
		}
		if !attributed {
			return false
		}
	}
	return true
}

func templateSinkDiagnostic(output string) string {
	lines := strings.Split(output, "\n")
	var evidence []string
	for index, line := range lines {
		if !strings.Contains(line, "redis.clients.connected") {
			continue
		}
		start := index - 30
		if start < 0 {
			start = 0
		}
		end := index + 10
		if end > len(lines) {
			end = len(lines)
		}
		evidence = append(evidence, strings.Join(lines[start:end], "\n"))
		if len(evidence) == 3 {
			break
		}
	}
	result := strings.Join(evidence, "\n---\n")
	if len(result) > 6000 {
		result = result[:6000] + "..."
	}
	return result
}

func startTemplateOTLPSink(t *testing.T, ctx context.Context, binary, tempDir string, output *lockedTemplateE2EBuffer) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	endpoint := listener.Addr().String()
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()
	telemetryListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	telemetryPort := telemetryListener.Addr().(*net.TCPAddr).Port
	_ = telemetryListener.Close()
	configPath := filepath.Join(tempDir, "sink.yaml")
	content := fmt.Sprintf(`receivers:
  otlp:
    protocols:
      grpc:
        endpoint: %q
exporters:
  debug:
    verbosity: detailed
service:
  telemetry:
    metrics:
      readers:
        - pull:
            exporter:
              prometheus:
                host: 127.0.0.1
                port: %d
  pipelines:
    metrics:
      receivers: [otlp]
      exporters: [debug]
`, endpoint, telemetryPort)
	if err := os.WriteFile(configPath, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	validation := (bootstrap.OSCommandRunner{}).Run(ctx, bootstrap.Command{Executable: binary, Args: []string{"validate", "--config", configPath}})
	if validation.ExitCode != 0 {
		t.Fatalf("validate isolated OTLP sink config: exit=%d err=%v stderr=%s", validation.ExitCode, validation.Err, validation.Stderr)
	}
	command := exec.CommandContext(ctx, binary, "--config", configPath)
	command.Stdout = output
	command.Stderr = output
	if err := command.Start(); err != nil {
		t.Fatalf("start isolated OTLP sink: %v", err)
	}
	done := make(chan error, 1)
	go func() { done <- command.Wait() }()
	t.Cleanup(func() {
		if command.Process != nil {
			_ = command.Process.Kill()
		}
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Errorf("isolated OTLP sink did not stop")
		}
	})
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		connection, dialErr := net.DialTimeout("tcp", endpoint, 200*time.Millisecond)
		if dialErr == nil {
			_ = connection.Close()
			return fmt.Sprintf("127.0.0.1:%d", port)
		}
		select {
		case err := <-done:
			t.Fatalf("isolated OTLP sink exited during startup: %v output=%s", err, output.String())
		case <-ctx.Done():
			t.Fatal("timed out starting isolated OTLP sink")
		case <-time.After(100 * time.Millisecond):
		}
	}
	t.Fatalf("isolated OTLP sink did not listen on %s", endpoint)
	return ""
}

func newRuntimeTemplateServiceForTest(t *testing.T, statePath string, registry integration.Registry) configuration.TemplateService {
	t.Helper()
	return configuration.NewTemplateService(registry, agentstate.NewSQLiteStore(statePath).TemplateRepository())
}

func writeRuntimeTemplateFile(t *testing.T, path string, template integration.IntegrationTemplate) {
	t.Helper()
	content, err := json.MarshalIndent(template, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, content, 0600); err != nil {
		t.Fatal(err)
	}
}

func runTemplateCLI(t *testing.T, statePath string, args ...string) {
	t.Helper()
	commandArgs := append([]string{"template"}, args...)
	commandArgs = append(commandArgs, "--state-path", statePath)
	var stdout, stderr bytes.Buffer
	if code := runIntegration(context.Background(), commandArgs, &stdout, &stderr); code != 0 {
		t.Fatalf("agentctl integration %s exit=%d stdout=%q stderr=%q", strings.Join(commandArgs, " "), code, stdout.String(), stderr.String())
	}
}

type lockedTemplateE2EBuffer struct {
	mu sync.Mutex
	bytes.Buffer
}

func (b *lockedTemplateE2EBuffer) Write(value []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.Buffer.Write(value)
}

func (b *lockedTemplateE2EBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.Buffer.String()
}
