package agentlifecycle

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/ar-imms/telemetry-agent/internal/agentstate"
	"github.com/ar-imms/telemetry-agent/internal/bootstrap"
	"github.com/ar-imms/telemetry-agent/internal/collectorruntime"
	"github.com/ar-imms/telemetry-agent/internal/config"
	"github.com/ar-imms/telemetry-agent/internal/dependency"
	"github.com/ar-imms/telemetry-agent/internal/identity"
	"github.com/ar-imms/telemetry-agent/internal/monitoringprofile"
	"github.com/ar-imms/telemetry-agent/internal/reconciliation/contracts"
	"github.com/ar-imms/telemetry-agent/internal/supervisor"
)

func TestApplyCollectorCandidateUsesExistingLifecycleAndWaitsForReadiness(t *testing.T) {
	store, request, options, activePath, _ := candidateFixture(t)
	outcome, err := ApplyCollectorCandidate(context.Background(), store, request, options)
	if err != nil {
		t.Fatalf("ApplyCollectorCandidate() error = %v", err)
	}
	if outcome.Result != "activated" || outcome.Generation != request.ExpectedGeneration+1 || outcome.EffectiveConfigDigest == "" {
		t.Fatalf("outcome = %#v", outcome)
	}
	state, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if state.AppliedGeneration != 0 || state.ActivatedGeneration != outcome.Generation || state.PendingActivation == nil || state.PendingActivation.Candidate == nil {
		t.Fatalf("state before Collector readiness = %#v", state)
	}
	active, err := os.ReadFile(activePath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := config.ReadinessTokenFromConfig(active); err != nil {
		t.Fatalf("active candidate readiness overlay invalid: %v", err)
	}
	if !strings.Contains(string(active), "${env:OTEL_GATEWAY_ENDPOINT}") || !strings.Contains(string(active), "prometheus/fixture_exporter") {
		t.Fatalf("active candidate did not preserve Gateway settings and managed receiver configuration:\n%s", active)
	}
	if err := store.MarkCandidateGenerationApplied(outcome.Generation, ""); err != nil {
		t.Fatalf("mark candidate applied after readiness: %v", err)
	}
	state, err = store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if state.AppliedGeneration != outcome.Generation || state.PendingActivation != nil {
		t.Fatalf("state after readiness = %#v", state)
	}
}

func TestCandidateReadinessCommitsM2MappingEvenWithGatewayRuntime(t *testing.T) {
	store, request, options, _, _ := candidateFixture(t)
	outcome, err := ApplyCollectorCandidate(context.Background(), store, request, options)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	marks := make(chan uint64, 1)
	gateway := &pendingGatewayRuntime{store: store, marks: marks}
	watcher := runtimeWatcher{store: store, gatewayRuntime: gateway}
	if err := watcher.onCollectorReady(context.Background(), snapshot, supervisor.Options{GatewayEndpoint: options.RenderValues["OTEL_GATEWAY_ENDPOINT"]}, outcome.Generation, options.ConfigPath, nil, func() {}); err != nil {
		t.Fatalf("onCollectorReady() error = %v", err)
	}
	mapping := snapshot.PendingActivation.Candidate
	got, generation, found, err := store.CandidateOutcome(mapping.DesiredSetRevision, mapping.BaselineFingerprint, mapping.CandidateDigest)
	if err != nil || !found || got.Outcome != "applied" || generation != outcome.Generation {
		t.Fatalf("candidate terminal evidence = %+v generation=%d found=%t error=%v", got, generation, found, err)
	}
	select {
	case generation := <-marks:
		t.Fatalf("candidate readiness used Gateway-only commit for generation %d", generation)
	default:
	}
}

func TestApplyCollectorCandidateRejectsChangedBaselineBeforeAdmission(t *testing.T) {
	store, request, options, _, _ := candidateFixture(t)
	if err := os.WriteFile(options.Layers[0].Path, []byte("receivers: {otlp: {protocols: {grpc: {endpoint: '127.0.0.1:4317'}}}}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	options.Runner = candidateRunnerFunc(func(context.Context, bootstrap.Command) bootstrap.CommandResult {
		t.Fatal("Collector validation ran for a stale baseline")
		return bootstrap.CommandResult{}
	})
	if _, err := ApplyCollectorCandidate(context.Background(), store, request, options); !errors.Is(err, agentstate.ErrStaleCandidate) {
		t.Fatalf("ApplyCollectorCandidate() error = %v, want stale candidate", err)
	}
	state, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if state.DesiredGeneration != request.ExpectedGeneration || state.PendingActivation != nil {
		t.Fatalf("stale admission mutated M2 state: %#v", state)
	}
}

func TestApplyCollectorCandidateFinishesAfterDesiredGenerationChangesPostAdmission(t *testing.T) {
	store, request, options, _, _ := candidateFixture(t)
	options.Runner = candidateRunnerFunc(func(context.Context, bootstrap.Command) bootstrap.CommandResult {
		if _, err := store.Update(func(state *agentstate.State) error {
			state.DesiredGeneration++
			return nil
		}); err != nil {
			t.Fatalf("advance desired generation during activation: %v", err)
		}
		return bootstrap.CommandResult{}
	})
	outcome, err := ApplyCollectorCandidate(context.Background(), store, request, options)
	if err != nil {
		t.Fatalf("ApplyCollectorCandidate() error = %v", err)
	}
	state, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Result != "activated" || state.DesiredGeneration != outcome.Generation+1 || state.ActivatedGeneration != outcome.Generation || state.PendingActivation == nil {
		t.Fatalf("outcome/state = %#v / %#v", outcome, state)
	}
	if err := store.MarkCandidateGenerationApplied(outcome.Generation, ""); err != nil {
		t.Fatalf("complete admitted generation after readiness: %v", err)
	}
	state, err = store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if state.AppliedGeneration != outcome.Generation || state.DesiredGeneration != outcome.Generation+1 {
		t.Fatalf("terminal state = %#v, want admitted generation applied and newer generation still desired", state)
	}
}

func TestApplyCollectorCandidateValidationFailurePreservesActiveConfigAndAppliedState(t *testing.T) {
	store, request, options, activePath, _ := candidateFixture(t)
	before, err := os.ReadFile(activePath)
	if err != nil {
		t.Fatal(err)
	}
	options.Runner = candidateRunnerFunc(func(context.Context, bootstrap.Command) bootstrap.CommandResult {
		return bootstrap.CommandResult{ExitCode: 1, Stderr: "invalid fixture"}
	})
	outcome, err := ApplyCollectorCandidate(context.Background(), store, request, options)
	if !errors.Is(err, bootstrap.ErrNativeConfigValidation) {
		t.Fatalf("ApplyCollectorCandidate() error = %v, want native validation error", err)
	}
	if outcome.Stage != "final_validation" || outcome.RecoveryResult != "succeeded" {
		t.Fatalf("failure outcome = %#v", outcome)
	}
	after, err := os.ReadFile(activePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatal("validation failure changed the active configuration")
	}
	state, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if state.AppliedGeneration != 0 || state.ActivatedGeneration != 0 || state.PendingActivation != nil || state.DesiredGeneration != request.ExpectedGeneration+1 {
		t.Fatalf("validation failure state = %#v", state)
	}
}

func TestPinnedInvalidNativeProfileCandidateRestoresPreviousConfiguration(t *testing.T) {
	binary := strings.TrimSpace(os.Getenv("AR_IMMS_PR05_COLLECTOR_BINARY"))
	if binary == "" {
		t.Skip("set AR_IMMS_PR05_COLLECTOR_BINARY to induce native profile validation failure with pinned Collector v0.160.0")
	}
	store, _, options, activePath, _ := candidateFixture(t)
	before, err := os.ReadFile(activePath)
	if err != nil {
		t.Fatal(err)
	}
	state, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	dockerEndpoint := "unix:///var/run/docker.sock"
	if options.Platform.OS == "windows" {
		dockerEndpoint = "npipe:////./pipe/docker_engine"
	}
	layer, receiverNames, err := monitoringprofile.DefaultRegistry().DockerCollectorLayer(options.Platform.OS, dockerEndpoint, "core-node-123")
	if err != nil {
		t.Fatal(err)
	}
	receivers := layer.Document["receivers"].(map[string]any)
	creator := receivers["receiver_creator"].(map[string]any)
	creator["watch_observers"] = "malformed"
	additional := func() ([]config.InlineLayer, []string, error) {
		return []config.InlineLayer{layer}, receiverNames, nil
	}
	snapshot, err := BuildCleanBaselineSnapshotWithAdditionalInputs(options.Platform, options.Catalog, options.Layers, state, options.RenderValues, []config.InlineLayer{layer}, receiverNames)
	if err != nil {
		t.Fatal(err)
	}
	capabilities, err := collectorruntime.StaticCatalog(options.Platform)
	if err != nil {
		t.Fatal(err)
	}
	candidate, err := collectorruntime.NewCompiler(capabilities).Compile(snapshot.Input, nil)
	if err != nil {
		t.Fatal(err)
	}
	request := contracts.CandidateApplyRequest{BaselineFingerprint: snapshot.Fingerprint, ExpectedGeneration: snapshot.DesiredGeneration, Candidate: candidate}
	options.AdditionalBaselineInputs = additional
	options.BinaryPath = binary
	options.Runner = bootstrap.OSCommandRunner{}
	outcome, err := ApplyCollectorCandidate(context.Background(), store, request, options)
	if !errors.Is(err, bootstrap.ErrNativeConfigValidation) || outcome.Stage != "final_validation" || outcome.RecoveryResult != "succeeded" {
		t.Fatalf("profile candidate failure outcome=%+v error=%v", outcome, err)
	}
	after, err := os.ReadFile(activePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) || strings.Contains(string(after), "redis/alternate_port") {
		t.Fatalf("validation failure did not restore the prior Agent-owned config:\n%s", after)
	}
	state, err = store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if state.AppliedGeneration != 0 || state.ActivatedGeneration != 0 || state.PendingActivation != nil {
		t.Fatalf("rejected profile candidate changed activation state: %+v", state)
	}
}

func TestPinnedNativeProfileActivationFailureRollsBackToLKG(t *testing.T) {
	binary := strings.TrimSpace(os.Getenv("AR_IMMS_PR05_COLLECTOR_BINARY"))
	if binary == "" {
		t.Skip("set AR_IMMS_PR05_COLLECTOR_BINARY to induce native profile activation failure with pinned Collector v0.160.0")
	}
	store, _, options, activePath, _ := candidateFixture(t)
	before, err := os.ReadFile(activePath)
	if err != nil {
		t.Fatal(err)
	}
	state, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	healthEndpoint := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	baselineBytes, err := os.ReadFile(options.Layers[0].Path)
	if err != nil {
		t.Fatal(err)
	}
	baselineBytes = []byte(strings.Replace(string(baselineBytes), "127.0.0.1:13133", healthEndpoint, 1))
	if err := os.WriteFile(options.Layers[0].Path, baselineBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	dockerEndpoint := "unix:///tmp/ar-imms-pr05-missing-docker.sock"
	if options.Platform.OS == "windows" {
		dockerEndpoint = "npipe:////./pipe/ar-imms-pr05-missing-docker"
	}
	layer, receiverNames, err := monitoringprofile.DefaultRegistry().DockerCollectorLayer(options.Platform.OS, dockerEndpoint, "core-node-123")
	if err != nil {
		t.Fatal(err)
	}
	additional := func() ([]config.InlineLayer, []string, error) {
		return []config.InlineLayer{layer}, receiverNames, nil
	}
	snapshot, err := BuildCleanBaselineSnapshotWithAdditionalInputs(options.Platform, options.Catalog, options.Layers, state, options.RenderValues, []config.InlineLayer{layer}, receiverNames)
	if err != nil {
		t.Fatal(err)
	}
	capabilities, err := collectorruntime.StaticCatalog(options.Platform)
	if err != nil {
		t.Fatal(err)
	}
	candidate, err := collectorruntime.NewCompiler(capabilities).Compile(snapshot.Input, nil)
	if err != nil {
		t.Fatal(err)
	}
	request := contracts.CandidateApplyRequest{BaselineFingerprint: snapshot.Fingerprint, ExpectedGeneration: snapshot.DesiredGeneration, Candidate: candidate}
	options.AdditionalBaselineInputs = additional
	options.BinaryPath = binary
	options.Runner = bootstrap.OSCommandRunner{}
	outcome, err := ApplyCollectorCandidate(context.Background(), store, request, options)
	if err != nil || outcome.Result != "activated" {
		t.Fatalf("apply valid but unavailable Docker profile candidate: outcome=%+v error=%v", outcome, err)
	}
	if !strings.Contains(string(candidate.Bytes()), "receiver_creator") || !strings.Contains(string(candidate.Bytes()), "ar-imms-pr05-missing-docker") {
		t.Fatal("activation failure candidate did not contain the valid native profile")
	}
	runtimeCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	watcher := runtimeWatcher{
		store: store, run: supervisor.Run,
		options:      supervisor.Options{BinaryPath: binary, ConfigPath: activePath, HealthEndpoint: "http://" + healthEndpoint, StartupTimeout: 5 * time.Second, ShutdownTimeout: 5 * time.Second},
		pollInterval: 50 * time.Millisecond, lockPath: activePath + ".lifecycle",
		commitConfig: bootstrap.CommitLastKnownGoodConfigFrom, snapshotConfig: bootstrap.SnapshotRuntimeConfig,
		rollback: func(ctx context.Context, path string, generation uint64) error {
			return rollbackUnappliedConfiguration(ctx, store, path, generation)
		},
	}
	if err := watcher.Run(runtimeCtx); err == nil {
		t.Fatal("Collector unexpectedly became ready with an unavailable Docker Observer endpoint")
	}
	after, err := os.ReadFile(activePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) || strings.Contains(string(after), "receiver_creator") {
		t.Fatalf("activation failure did not restore the prior config:\n%s", after)
	}
	state, err = store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if state.AppliedGeneration != 0 || state.ActivatedGeneration != 0 || state.PendingActivation != nil {
		t.Fatalf("activation failure left candidate lifecycle state: %+v", state)
	}
}

type candidateRunnerFunc func(context.Context, bootstrap.Command) bootstrap.CommandResult

func (f candidateRunnerFunc) Run(ctx context.Context, command bootstrap.Command) bootstrap.CommandResult {
	return f(ctx, command)
}

func candidateFixture(t *testing.T) (agentstate.SQLiteStore, contracts.CandidateApplyRequest, CollectorConfigurationOptions, string, string) {
	t.Helper()
	platform := identity.PlatformInfo{OS: runtime.GOOS, Architecture: runtime.GOARCH}
	layerPath := filepath.Join(t.TempDir(), "base.yaml")
	baseline := []byte(`receivers:
  otlp:
    protocols:
      grpc: { endpoint: 127.0.0.1:4317 }
  hostmetrics:
    scrapers:
      cpu: {}
      memory: {}
  prometheus/fixture_exporter:
    config:
      scrape_configs:
        - job_name: fixture_exporter
          static_configs:
            - targets: [127.0.0.1:19100]
processors:
  batch: {}
exporters:
  otlp:
    endpoint: ${env:OTEL_GATEWAY_ENDPOINT}
    tls: { insecure: true }
  debug: {}
extensions:
  health_check:
    endpoint: 127.0.0.1:13133
service:
  extensions: [health_check]
  pipelines:
    metrics:
      receivers: [otlp, hostmetrics]
      processors: [batch]
      exporters: [otlp]
    logs:
      receivers: [otlp]
      processors: [batch]
      exporters: [otlp]
    traces:
      receivers: [otlp]
      processors: [batch]
      exporters: [otlp]
`)
	if err := os.WriteFile(layerPath, baseline, 0o600); err != nil {
		t.Fatal(err)
	}
	activePath := filepath.Join(t.TempDir(), "otel.yaml")
	if err := os.WriteFile(activePath, baseline, 0o600); err != nil {
		t.Fatal(err)
	}
	statePath := filepath.Join(t.TempDir(), "state.db")
	store := agentstate.NewSQLiteStore(statePath)
	if _, err := store.Update(func(*agentstate.State) error { return nil }); err != nil {
		t.Fatal(err)
	}
	layers := []config.Layer{{Name: "base", Path: layerPath}}
	catalog, err := dependency.NewCatalog([]dependency.Integration{{
		Definition:        dependency.Definition{Name: "fixture-exporter", DisplayName: "Fixture Exporter", SupportedOS: []string{platform.OS}},
		CollectorReceiver: "prometheus/fixture_exporter",
		Install:           func(context.Context) (dependency.InstallResult, error) { return dependency.InstallResult{}, nil },
		Inspect:           func(context.Context) (dependency.Inspection, error) { return dependency.Inspection{}, nil },
	}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Update(func(state *agentstate.State) error {
		_, err := state.EnableDependency("fixture-exporter")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	state, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	gatewayEndpoint := "127.0.0.1:14317"
	values := map[string]string{"OTEL_GATEWAY_ENDPOINT": gatewayEndpoint}
	snapshot, err := BuildCleanBaselineSnapshot(platform, catalog, layers, state, values)
	if err != nil {
		t.Fatal(err)
	}
	capabilities, err := collectorruntime.StaticCatalog(platform)
	if err != nil {
		t.Fatal(err)
	}
	candidate, err := collectorruntime.NewCompiler(capabilities).Compile(snapshot.Input, nil)
	if err != nil {
		t.Fatal(err)
	}
	request := contracts.CandidateApplyRequest{BaselineFingerprint: snapshot.Fingerprint, ExpectedGeneration: snapshot.DesiredGeneration, Candidate: candidate}
	options := CollectorConfigurationOptions{
		Platform:     platform,
		Catalog:      catalog,
		Layers:       layers,
		BinaryPath:   "otelcol-contrib",
		ConfigPath:   activePath,
		RenderValues: values,
		ValidationEnvironment: []string{
			"OTEL_GATEWAY_ENDPOINT=" + gatewayEndpoint,
		},
		Runner: successfulCollectorRunner{},
	}
	return store, request, options, activePath, statePath
}
