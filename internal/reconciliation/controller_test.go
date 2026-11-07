package reconciliation

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/ar-imms/telemetry-agent/internal/agentlifecycle"
	"github.com/ar-imms/telemetry-agent/internal/agentstate"
	"github.com/ar-imms/telemetry-agent/internal/bootstrap"
	"github.com/ar-imms/telemetry-agent/internal/collectorruntime"
	"github.com/ar-imms/telemetry-agent/internal/config"
	"github.com/ar-imms/telemetry-agent/internal/configuration"
	"github.com/ar-imms/telemetry-agent/internal/dependency"
	"github.com/ar-imms/telemetry-agent/internal/identity"
	"github.com/ar-imms/telemetry-agent/internal/integration"
	redisintegration "github.com/ar-imms/telemetry-agent/internal/integration/redis"
	"gopkg.in/yaml.v3"
)

func TestReconcileResolvesOnlyEnabledInstancesAndSubmitsCompleteSnapshot(t *testing.T) {
	desired := &snapshotReader{snapshot: configuration.DesiredSnapshot{Revision: 12, Instances: []integration.IntegrationInstance{
		{ID: "enabled", DefinitionID: "fixture", DefinitionVersion: "1.0.0", Configuration: map[string]any{"endpoint": "localhost:1"}, Enabled: true},
		{ID: "disabled", DefinitionID: "fixture", DefinitionVersion: "1.0.0", Configuration: map[string]any{"endpoint": "localhost:2"}},
	}}}
	resolver := &testResolver{}
	compiler := &testCompiler{}
	applier := &testApplier{}
	controller := Controller{Desired: desired, Baseline: staticBaseline{}, Resolver: resolver, Compiler: compiler, Validator: func(context.Context, collectorruntime.Candidate) error { return nil }, Applier: applier, Attempts: &testAttempts{}}

	result, err := controller.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if result.DesiredRevision != 12 || result.Status != StatusPending {
		t.Fatalf("result = %+v, want revision 12 pending until M2 terminal evidence", result)
	}
	if resolver.ids[0] != "enabled" || len(resolver.ids) != 1 {
		t.Fatalf("resolved IDs = %v, want only enabled instance", resolver.ids)
	}
	if compiler.count != 1 || applier.request.DesiredSetRevision != 12 || applier.request.ExpectedGeneration != 4 {
		t.Fatalf("compile/apply = %d / %+v", compiler.count, applier.request)
	}
}

func TestReconcileUnplugKeepsOtherEnabledIntegrationsInCandidate(t *testing.T) {
	desired := &snapshotReader{snapshot: configuration.DesiredSnapshot{Revision: 13, Instances: []integration.IntegrationInstance{
		{ID: "postgres-a", DefinitionID: "postgres", DefinitionVersion: "1.0.0", Configuration: map[string]any{}, Enabled: true},
		{ID: "redis-a", DefinitionID: "redis", DefinitionVersion: "1.0.0", Configuration: map[string]any{}, Enabled: false},
	}}}
	resolver, compiler := &testResolver{}, &testCompiler{}
	controller := Controller{Desired: desired, Baseline: staticBaseline{}, Resolver: resolver, Compiler: compiler, Validator: func(context.Context, collectorruntime.Candidate) error { return nil }, Applier: &testApplier{}, Attempts: &testAttempts{}}
	if _, err := controller.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(resolver.ids) != 1 || resolver.ids[0] != "postgres-a" || len(compiler.resolvedIDs) != 1 || compiler.resolvedIDs[0] != "postgres-a" {
		t.Fatalf("resolved=%v compiled=%v; disabling Redis must retain PostgreSQL only", resolver.ids, compiler.resolvedIDs)
	}
}

func TestM2GenerationChangeReconcilesSameEnabledIntegrationRevision(t *testing.T) {
	desired := &snapshotReader{snapshot: configuration.DesiredSnapshot{Revision: 12, Instances: []integration.IntegrationInstance{{ID: "postgres-a", DefinitionID: "postgres", DefinitionVersion: "1.0.0", Configuration: map[string]any{}, Enabled: true}}}}
	baseline := &changingBaseline{generation: 4}
	resolver, compiler, applier := &testResolver{}, &testCompiler{}, &revisionRecordingApplier{}
	controller := Controller{Desired: desired, Baseline: baseline, Resolver: resolver, Compiler: compiler, Validator: func(context.Context, collectorruntime.Candidate) error { return nil }, Applier: applier, Attempts: &testAttempts{}}
	if _, err := controller.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	baseline.generation = 5 // A managed-dependency mutation advances M2 without changing Integration revision.
	if _, err := controller.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(applier.generations) != 2 || applier.generations[0] != 4 || applier.generations[1] != 5 || len(resolver.ids) != 2 || compiler.count != 2 {
		t.Fatalf("generations=%v resolver=%v compiles=%d; expected both desired targets to include the enabled integration", applier.generations, resolver.ids, compiler.count)
	}
}

func TestStaleAdmissionImmediatelyReconcilesLatestDesiredRevision(t *testing.T) {
	desired := &sequenceReader{snapshots: []configuration.DesiredSnapshot{
		{Revision: 12},
		{Revision: 13},
	}}
	applier := &staleOnceApplier{}
	controller := Controller{Desired: desired, Baseline: staticBaseline{}, Resolver: &testResolver{}, Compiler: &testCompiler{}, Validator: func(context.Context, collectorruntime.Candidate) error { return nil }, Applier: applier, Attempts: &testAttempts{}}
	result, err := controller.reconcileLatest(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.DesiredRevision != 13 || len(applier.revisions) != 2 || applier.revisions[0] != 12 || applier.revisions[1] != 13 {
		t.Fatalf("result=%+v submitted revisions=%v; want latest revision 13 after stale admission", result, applier.revisions)
	}
}

func TestReconcileLatestRepeatsWhenBaselineFingerprintChangesAfterApply(t *testing.T) {
	desired := &snapshotReader{snapshot: configuration.DesiredSnapshot{Revision: 12}}
	baseline := &sequenceBaseline{snapshots: []agentlifecycle.CleanBaselineSnapshot{
		{DesiredGeneration: 4, Fingerprint: "before", Input: config.RenderInput{Platform: identity.PlatformInfo{OS: "windows", Architecture: "amd64"}}},
		{DesiredGeneration: 4, Fingerprint: "after", Input: config.RenderInput{Platform: identity.PlatformInfo{OS: "windows", Architecture: "amd64"}}},
		{DesiredGeneration: 4, Fingerprint: "after", Input: config.RenderInput{Platform: identity.PlatformInfo{OS: "windows", Architecture: "amd64"}}},
		{DesiredGeneration: 4, Fingerprint: "after", Input: config.RenderInput{Platform: identity.PlatformInfo{OS: "windows", Architecture: "amd64"}}},
	}}
	controller := Controller{
		Desired: desired, Baseline: baseline, Resolver: &testResolver{}, Compiler: &testCompiler{},
		Validator: func(context.Context, collectorruntime.Candidate) error { return nil },
		Applier:   &testApplier{}, Attempts: &testAttempts{},
		Outcomes: fixedOutcome{mapping: agentstate.CandidateMapping{Outcome: "applied"}, generation: 5, found: true},
	}
	result, err := controller.reconcileLatest(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != StatusApplied || result.BaselineFingerprint != "after" || baseline.calls != 4 {
		t.Fatalf("result=%+v baseline snapshots=%d; changed baseline must be recompiled before convergence is returned", result, baseline.calls)
	}
}

func TestReconcileAcknowledgesAppliedOnlyFromM2Journal(t *testing.T) {
	controller := Controller{Desired: &snapshotReader{snapshot: configuration.DesiredSnapshot{Revision: 12}}, Baseline: staticBaseline{}, Resolver: &testResolver{}, Compiler: &testCompiler{}, Validator: func(context.Context, collectorruntime.Candidate) error { return nil }, Applier: &testApplier{}, Attempts: &testAttempts{}, Outcomes: fixedOutcome{mapping: agentstate.CandidateMapping{Outcome: "applied"}, generation: 37, found: true}}
	result, err := controller.Reconcile(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != StatusApplied || result.Generation != 37 {
		t.Fatalf("result = %+v, want M2 applied generation 37", result)
	}
}

func TestPermanentCapabilityFailureBlocksWithoutRetry(t *testing.T) {
	failure := classify("compile", collectorruntime.ErrUnsupportedCapability)
	if failure.Class != FailurePermanent || attemptStatus(failure.Class, 1) != StatusBlocked {
		t.Fatalf("unsupported capability classification = %+v", failure)
	}
}

func TestPeriodicSQLitePollingDetectsCommittedCrossProcessDesiredChange(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	writer := agentstate.NewSQLiteStore(path)
	reader := agentstate.NewSQLiteStore(path)
	definition := integration.IntegrationDefinition{ID: "fixture", Version: "1.0.0", Validate: func(map[string]any) error { return nil }, Contribute: func(integration.IntegrationInstance) (integration.Contribution, error) {
		return integration.NewContribution(nil), nil
	}}
	registry, err := integration.NewRegistry(definition)
	if err != nil {
		t.Fatal(err)
	}
	observed := make(chan uint64, 8)
	controller := Controller{Desired: reader.IntegrationRepository(), Baseline: staticBaseline{}, Resolver: integration.NewResolver(registry), Compiler: &testCompiler{}, Validator: func(context.Context, collectorruntime.Candidate) error { return nil }, Applier: recordingApplier{revisions: observed}, Attempts: &testAttempts{}}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- controller.Run(ctx, 10*time.Millisecond) }()
	select {
	case revision := <-observed:
		if revision != 0 {
			t.Fatalf("startup revision=%d want 0", revision)
		}
	case <-time.After(time.Second):
		t.Fatal("startup reconciliation did not run")
	}
	service := configuration.NewIntegrationService(integration.NewResolver(registry), writer.IntegrationRepository())
	if _, _, err := service.Put(integration.IntegrationInstance{ID: "cross-process", DefinitionID: "fixture", DefinitionVersion: "1.0.0", Configuration: map[string]any{}, Enabled: true}, 0); err != nil {
		t.Fatal(err)
	}
	select {
	case revision := <-observed:
		if revision != 1 {
			t.Fatalf("polled revision=%d want 1", revision)
		}
	case <-time.After(time.Second):
		t.Fatal("periodic SQLite polling did not observe committed mutation")
	}
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() shutdown error=%v", err)
	}
}

func TestRedisAndGenericIntegrationPlugUnplugUsesCompleteM2Candidate(t *testing.T) {
	root := t.TempDir()
	statePath := filepath.Join(root, "state.db")
	configPath := filepath.Join(root, "otel.yaml")
	layerPath := filepath.Join(root, "base.yaml")
	baselineBytes := []byte(`receivers:
  otlp:
    protocols:
      grpc: { endpoint: 127.0.0.1:4317 }
  hostmetrics:
    collection_interval: 30s
    scrapers: { cpu: {}, memory: {} }
  prometheus/fixture_exporter:
    config:
      scrape_configs:
        - job_name: fixture_exporter
          static_configs:
            - targets: [127.0.0.1:19100]
processors:
  batch: {}
exporters:
  otlp/gateway:
    endpoint: ${env:OTEL_GATEWAY_ENDPOINT}
    tls: { insecure: true }
extensions:
  health_check:
    endpoint: 127.0.0.1:13133
service:
  extensions: [health_check]
  pipelines:
    metrics:
      receivers: [otlp, hostmetrics]
      processors: [batch]
      exporters: [otlp/gateway]
    logs:
      receivers: [otlp]
      processors: [batch]
      exporters: [otlp/gateway]
    traces:
      receivers: [otlp]
      processors: [batch]
      exporters: [otlp/gateway]
`)
	for _, path := range []string{layerPath, configPath} {
		if err := os.WriteFile(path, baselineBytes, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	platform := identity.PlatformInfo{OS: runtime.GOOS, Architecture: runtime.GOARCH}
	definitions, err := dependency.NewCatalog([]dependency.Integration{{
		Definition:        dependency.Definition{Name: "fixture-exporter", DisplayName: "Fixture Exporter", SupportedOS: []string{platform.OS}},
		CollectorReceiver: "prometheus/fixture_exporter",
		Install:           func(context.Context) (dependency.InstallResult, error) { return dependency.InstallResult{}, nil },
		Inspect:           func(context.Context) (dependency.Inspection, error) { return dependency.Inspection{}, nil },
	}})
	if err != nil {
		t.Fatal(err)
	}
	layers := []config.Layer{{Name: "base", Path: layerPath}}
	values := map[string]string{"OTEL_GATEWAY_ENDPOINT": "127.0.0.1:4317"}
	store := agentstate.NewSQLiteStore(statePath)
	if _, err := store.Update(func(*agentstate.State) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Update(func(state *agentstate.State) error {
		_, err := state.EnableDependency("fixture-exporter")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	definition := integration.IntegrationDefinition{
		ID: "fixture-postgres", Version: "1.0.0",
		Validate: func(settings map[string]any) error {
			if _, ok := settings["endpoint"].(string); !ok {
				return errors.New("endpoint is required")
			}
			return nil
		},
		Contribute: func(instance integration.IntegrationInstance) (integration.Contribution, error) {
			intent, err := integration.NewComponentIntent("fixture-receiver-metrics", map[string]any{"endpoint": instance.Configuration["endpoint"]})
			if err != nil {
				return integration.Contribution{}, err
			}
			return integration.NewContribution([]integration.ComponentIntent{intent}), nil
		},
	}
	redisDefinition := redisintegration.Definition()
	registry, err := integration.NewRegistry(definition, redisDefinition)
	if err != nil {
		t.Fatal(err)
	}
	resolver := integration.NewResolver(registry)
	repository := store.IntegrationRepository()
	service := configuration.NewIntegrationService(resolver, repository)
	_, _, err = service.Put(integration.IntegrationInstance{ID: "postgres-a", DefinitionID: definition.ID, DefinitionVersion: definition.Version, Configuration: map[string]any{"endpoint": "127.0.0.1:5432"}, Enabled: true}, 0)
	if err != nil {
		t.Fatal(err)
	}
	redisInstance, _, err := service.Put(integration.IntegrationInstance{ID: "redis-a", DefinitionID: redisDefinition.ID, DefinitionVersion: redisDefinition.Version, Configuration: map[string]any{"endpoint": "127.0.0.1:6379"}, Enabled: true}, 0)
	if err != nil {
		t.Fatal(err)
	}
	capabilities, err := collectorruntime.NewCatalog("0.160.0", "fixture-manifest", []collectorruntime.Capability{
		{ID: "fixture-receiver-metrics", NativeName: "prometheus", Kind: "receiver", OS: []string{platform.OS}, Signals: []string{"metrics"}, Pipeline: "metrics"},
		{ID: "otel.receiver.redis.metrics", NativeName: "redis", Kind: "receiver", OS: []string{platform.OS}, Signals: []string{"metrics"}, Pipeline: "metrics"},
	})
	if err != nil {
		t.Fatal(err)
	}
	compiler := collectorruntime.NewCompiler(capabilities)
	options := agentlifecycle.CollectorConfigurationOptions{Platform: platform, Catalog: definitions, Layers: layers, BinaryPath: "otelcol-contrib", ConfigPath: configPath, RenderValues: values, ValidationEnvironment: []string{"OTEL_GATEWAY_ENDPOINT=127.0.0.1:4317"}, Runner: successfulCandidateRunner{}}
	controller := Controller{Desired: repository, Baseline: testCleanBaseline{store: store, platform: platform, dependencies: definitions, layers: layers, values: values}, Resolver: resolver, Compiler: compiler, Validator: func(context.Context, collectorruntime.Candidate) error { return nil }, Applier: testM2Applier{store: store, options: options}, Attempts: NewSQLiteAttemptsFromStore(store), Outcomes: CandidateOutcomes{Store: store}}
	first, err := controller.Reconcile(context.Background())
	if err != nil || first.Status != StatusApplied {
		t.Fatalf("plug reconciliation=%+v err=%v", first, err)
	}
	active, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(active), "prometheus/i-") || !strings.Contains(string(active), "redis/i-") || !strings.Contains(string(active), "hostmetrics") || !strings.Contains(string(active), "127.0.0.1:4317") || !pipelineHasReceiver(t, active, "prometheus/fixture_exporter") {
		t.Fatalf("plug candidate omitted Redis/PostgreSQL-like integrations, managed dependency, host telemetry, or Gateway:\n%s", active)
	}
	if _, err := store.Update(func(state *agentstate.State) error {
		_, err := state.DisableDependency("fixture-exporter")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	dependencyDisabled, err := controller.Reconcile(context.Background())
	if err != nil || dependencyDisabled.Status != StatusApplied || dependencyDisabled.DesiredRevision != first.DesiredRevision {
		t.Fatalf("managed dependency disable reconciliation=%+v err=%v", dependencyDisabled, err)
	}
	active, err = os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(active), "prometheus/i-") || !strings.Contains(string(active), "redis/i-") || !strings.Contains(string(active), "hostmetrics") || !strings.Contains(string(active), "127.0.0.1:4317") || pipelineHasReceiver(t, active, "prometheus/fixture_exporter") {
		t.Fatalf("dependency disable disturbed either Integration, host telemetry, Gateway, or retained dependency pipeline:\n%s", active)
	}
	if _, err := store.Update(func(state *agentstate.State) error {
		_, err := state.EnableDependency("fixture-exporter")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	dependencyEnabled, err := controller.Reconcile(context.Background())
	if err != nil || dependencyEnabled.Status != StatusApplied || dependencyEnabled.DesiredRevision != first.DesiredRevision {
		t.Fatalf("managed dependency enable reconciliation=%+v err=%v", dependencyEnabled, err)
	}
	redisInstance.Enabled = false
	if _, _, err := service.Put(redisInstance, redisInstance.Revision); err != nil {
		t.Fatal(err)
	}
	second, err := controller.Reconcile(context.Background())
	if err != nil || second.Status != StatusApplied || second.DesiredRevision != first.DesiredRevision+1 {
		t.Fatalf("unplug reconciliation=%+v err=%v", second, err)
	}
	active, err = os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(active), "redis/i-") || !strings.Contains(string(active), "prometheus/i-") || !strings.Contains(string(active), "hostmetrics") || !strings.Contains(string(active), "127.0.0.1:4317") || !pipelineHasReceiver(t, active, "prometheus/fixture_exporter") {
		t.Fatalf("Redis unplug removed the PostgreSQL-like Integration, managed dependency, host telemetry, or Gateway, or retained Redis:\n%s", active)
	}
}

func pipelineHasReceiver(t *testing.T, candidate []byte, expected string) bool {
	t.Helper()
	var document map[string]any
	if err := yaml.Unmarshal(candidate, &document); err != nil {
		t.Fatalf("decode candidate YAML: %v", err)
	}
	service, ok := document["service"].(map[string]any)
	if !ok {
		t.Fatal("candidate service section is missing")
	}
	pipelines, ok := service["pipelines"].(map[string]any)
	if !ok {
		t.Fatal("candidate pipelines section is missing")
	}
	metrics, ok := pipelines["metrics"].(map[string]any)
	if !ok {
		t.Fatal("candidate metrics pipeline is missing")
	}
	receivers, ok := metrics["receivers"].([]any)
	if !ok {
		t.Fatal("candidate metrics receiver list is missing")
	}
	for _, receiver := range receivers {
		if receiver == expected {
			return true
		}
	}
	return false
}

func TestReconcileDoesNotApplyWhenResolverFails(t *testing.T) {
	controller := Controller{Desired: &snapshotReader{snapshot: configuration.DesiredSnapshot{Revision: 1, Instances: []integration.IntegrationInstance{{ID: "one", DefinitionID: "fixture", DefinitionVersion: "1.0.0", Configuration: map[string]any{}, Enabled: true}}}}, Baseline: staticBaseline{}, Resolver: &testResolver{err: errors.New("invalid")}, Compiler: &testCompiler{}, Validator: func(context.Context, collectorruntime.Candidate) error { return nil }, Applier: &testApplier{}, Attempts: &testAttempts{}}
	if _, err := controller.Reconcile(context.Background()); err == nil {
		t.Fatal("Reconcile() error = nil, want resolver error")
	}
}

type snapshotReader struct{ snapshot configuration.DesiredSnapshot }

func (s *snapshotReader) Snapshot() (configuration.DesiredSnapshot, error) { return s.snapshot, nil }

type staticBaseline struct{}

func (staticBaseline) Snapshot() (agentlifecycle.CleanBaselineSnapshot, error) {
	return agentlifecycle.CleanBaselineSnapshot{DesiredGeneration: 4, Fingerprint: "fingerprint", Input: config.RenderInput{Platform: identity.PlatformInfo{OS: "windows", Architecture: "amd64"}}}, nil
}

type testResolver struct {
	ids []string
	err error
}

func (r *testResolver) Resolve(i integration.IntegrationInstance) (integration.ResolvedInstance, error) {
	r.ids = append(r.ids, i.ID)
	if r.err != nil {
		return integration.ResolvedInstance{}, r.err
	}
	return integration.ResolvedInstance{Instance: i, Contribution: integration.NewContribution(nil)}, nil
}

type testCompiler struct {
	count       int
	resolvedIDs []string
}

func (c *testCompiler) Compile(_ config.RenderInput, resolved []integration.ResolvedInstance) (collectorruntime.Candidate, error) {
	c.count++
	for _, item := range resolved {
		c.resolvedIDs = append(c.resolvedIDs, item.Instance.ID)
	}
	return collectorruntime.Candidate{}, nil
}

type testApplier struct {
	request agentlifecycle.CandidateApplyRequest
}

func (a *testApplier) ApplyCandidate(_ context.Context, request agentlifecycle.CandidateApplyRequest) (agentlifecycle.CandidateActivationOutcome, error) {
	a.request = request
	return agentlifecycle.CandidateActivationOutcome{Generation: 5, Result: "activated"}, nil
}

type testAttempts struct{}

func (testAttempts) BeginAttempt(Target) (Attempt, error)  { return Attempt{Number: 1}, nil }
func (testAttempts) RecordCandidate(Attempt, string) error { return nil }
func (testAttempts) FinishAttempt(Attempt, Outcome) error  { return nil }

type fixedOutcome struct {
	mapping    agentstate.CandidateMapping
	generation uint64
	found      bool
}

type recordingApplier struct{ revisions chan<- uint64 }

func (a recordingApplier) ApplyCandidate(_ context.Context, request agentlifecycle.CandidateApplyRequest) (agentlifecycle.CandidateActivationOutcome, error) {
	a.revisions <- request.DesiredSetRevision
	return agentlifecycle.CandidateActivationOutcome{Generation: request.ExpectedGeneration + 1, Result: "activated"}, nil
}

type revisionRecordingApplier struct{ generations []uint64 }

func (a *revisionRecordingApplier) ApplyCandidate(_ context.Context, request agentlifecycle.CandidateApplyRequest) (agentlifecycle.CandidateActivationOutcome, error) {
	a.generations = append(a.generations, request.ExpectedGeneration)
	return agentlifecycle.CandidateActivationOutcome{Generation: request.ExpectedGeneration + 1, Result: "activated"}, nil
}

type staleOnceApplier struct {
	revisions []uint64
	stale     bool
}

func (a *staleOnceApplier) ApplyCandidate(_ context.Context, request agentlifecycle.CandidateApplyRequest) (agentlifecycle.CandidateActivationOutcome, error) {
	a.revisions = append(a.revisions, request.DesiredSetRevision)
	if !a.stale {
		a.stale = true
		return agentlifecycle.CandidateActivationOutcome{}, agentstate.ErrStaleCandidate
	}
	return agentlifecycle.CandidateActivationOutcome{Generation: request.ExpectedGeneration + 1, Result: "activated"}, nil
}

type sequenceReader struct {
	snapshots []configuration.DesiredSnapshot
	index     int
}

func (s *sequenceReader) Snapshot() (configuration.DesiredSnapshot, error) {
	index := s.index
	if index >= len(s.snapshots) {
		index = len(s.snapshots) - 1
	}
	s.index++
	return s.snapshots[index], nil
}

type changingBaseline struct{ generation uint64 }

func (b *changingBaseline) Snapshot() (agentlifecycle.CleanBaselineSnapshot, error) {
	return agentlifecycle.CleanBaselineSnapshot{DesiredGeneration: b.generation, Fingerprint: "baseline", Input: config.RenderInput{Platform: identity.PlatformInfo{OS: "windows", Architecture: "amd64"}}}, nil
}

type sequenceBaseline struct {
	snapshots []agentlifecycle.CleanBaselineSnapshot
	calls     int
}

func (b *sequenceBaseline) Snapshot() (agentlifecycle.CleanBaselineSnapshot, error) {
	index := b.calls
	if index >= len(b.snapshots) {
		index = len(b.snapshots) - 1
	}
	b.calls++
	return b.snapshots[index], nil
}

type testCleanBaseline struct {
	store        agentstate.Repository
	platform     identity.PlatformInfo
	dependencies dependency.Catalog
	layers       []config.Layer
	values       map[string]string
}

func (b testCleanBaseline) Snapshot() (agentlifecycle.CleanBaselineSnapshot, error) {
	state, err := b.store.Load()
	if err != nil {
		return agentlifecycle.CleanBaselineSnapshot{}, err
	}
	return agentlifecycle.BuildCleanBaselineSnapshot(b.platform, b.dependencies, b.layers, state, b.values)
}

type successfulCandidateRunner struct{}

func (successfulCandidateRunner) Run(context.Context, bootstrap.Command) bootstrap.CommandResult {
	return bootstrap.CommandResult{}
}

type testM2Applier struct {
	store   agentstate.SQLiteStore
	options agentlifecycle.CollectorConfigurationOptions
}

func (a testM2Applier) ApplyCandidate(ctx context.Context, request agentlifecycle.CandidateApplyRequest) (agentlifecycle.CandidateActivationOutcome, error) {
	outcome, err := agentlifecycle.ApplyCollectorCandidate(ctx, a.store, request, a.options)
	if err != nil || outcome.AlreadyApplied {
		return outcome, err
	}
	state, err := a.store.Load()
	if err != nil {
		return outcome, err
	}
	if state.PendingActivation == nil {
		return outcome, errors.New("candidate admission did not retain a pending readiness mapping")
	}
	if err := a.store.MarkCandidateGenerationApplied(state.PendingActivation.Generation, a.options.RenderValues["OTEL_GATEWAY_ENDPOINT"]); err != nil {
		return outcome, err
	}
	return outcome, nil
}

func (o fixedOutcome) Lookup(uint64, string, string) (agentstate.CandidateMapping, uint64, bool, error) {
	return o.mapping, o.generation, o.found, nil
}
