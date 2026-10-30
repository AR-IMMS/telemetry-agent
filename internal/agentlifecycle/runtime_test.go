package agentlifecycle

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ar-imms/telemetry-agent/internal/agenthealth"
	"github.com/ar-imms/telemetry-agent/internal/agentstate"
	"github.com/ar-imms/telemetry-agent/internal/config"
	"github.com/ar-imms/telemetry-agent/internal/coreconfig"
	"github.com/ar-imms/telemetry-agent/internal/supervisor"
)

func TestRuntimeWatcherMarksLaunchGenerationAppliedAfterCollectorReady(
	t *testing.T,
) {
	configRoot := t.TempDir()
	configPath := filepath.Join(configRoot, "otel.yaml")
	launchConfigPath := filepath.Join(configRoot, "launch.yaml")
	if err := os.WriteFile(launchConfigPath, []byte("extensions:\n  health_check:\n    response_body:\n      healthy: generation-token\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	store := agentstate.NewFileStore(
		filepath.Join(t.TempDir(), "state.json"),
	)

	_, err := store.Update(func(state *agentstate.State) error {
		state.DesiredGeneration = 3
		state.ActivatedGeneration = 3
		state.AppliedGeneration = 2

		return nil
	})
	if err != nil {
		t.Fatalf("initialize state: %v", err)
	}

	ready := make(chan struct{})
	var committedConfig, committedSource string

	watcher := runtimeWatcher{
		store: store,
		run: func(
			ctx context.Context,
			options supervisor.Options,
		) error {
			if options.ReadinessBody != "generation-token" {
				t.Errorf("ReadinessBody = %q, want generation-token", options.ReadinessBody)
			}
			if options.OnReady == nil {
				t.Error("supervisor options OnReady is nil")
				return nil
			}

			if err := options.OnReady(); err != nil {
				return err
			}

			close(ready)

			<-ctx.Done()

			return nil
		},
		options: supervisor.Options{
			BinaryPath:      "otelcol-contrib",
			ConfigPath:      configPath,
			HealthEndpoint:  "http://127.0.0.1:13133",
			GatewayEndpoint: "gateway.example:4317",
			StartupTimeout:  time.Second,
			ShutdownTimeout: time.Second,
		},
		snapshotConfig: func(string, uint64) (string, func(), error) {
			return launchConfigPath, func() {}, nil
		},
		commitConfig: func(configPath, sourcePath string) error {
			committedConfig, committedSource = configPath, sourcePath
			return nil
		},
		pollInterval: time.Millisecond,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runDone := make(chan error, 1)

	go func() {
		runDone <- watcher.Run(ctx)
	}()

	select {
	case <-ready:
	case err := <-runDone:
		t.Fatalf(
			"runtime watcher stopped before readiness: %v",
			err,
		)
	case <-time.After(time.Second):
		t.Fatal("Collector readiness callback was not called")
	}

	state, err := store.Load()
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	if state.AppliedGeneration != 3 {
		t.Fatalf(
			"AppliedGeneration = %d, want 3",
			state.AppliedGeneration,
		)
	}
	if committedConfig != configPath || committedSource != launchConfigPath {
		t.Fatalf("last-known-good commit paths = %q <- %q, want stable path <- launch snapshot", committedConfig, committedSource)
	}

	cancel()

	select {
	case err := <-runDone:
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}

	case <-time.After(time.Second):
		t.Fatal("runtime watcher did not stop after cancellation")
	}
}

func TestRuntimeLaunchSnapshotWaitsForConfigActivationLock(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, "otel.yaml")
	statePath := filepath.Join(root, "state.json")
	store := agentstate.NewFileStore(statePath)
	if err := os.WriteFile(configPath, []byte("extensions:\n  health_check:\n    response_body:\n      healthy: revision-one\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(agentstate.State{
		Collector:           agentstate.CollectorContext{ConfigPath: configPath},
		ActivatedGeneration: 1,
		Dependencies:        map[string]agentstate.DependencyState{},
	}); err != nil {
		t.Fatal(err)
	}
	lock, err := agentstate.AcquireLifecycleLock(configPath + ".lifecycle")
	if err != nil {
		t.Fatal(err)
	}
	watcher := runtimeWatcher{
		store:    store,
		options:  supervisor.Options{ConfigPath: configPath},
		lockPath: configPath + ".lifecycle",
		snapshotConfig: func(path string, _ uint64) (string, func(), error) {
			content, err := os.ReadFile(path)
			if err != nil {
				return "", nil, err
			}
			snapshotPath := filepath.Join(root, "launch.yaml")
			if err := os.WriteFile(snapshotPath, content, 0o600); err != nil {
				return "", nil, err
			}
			return snapshotPath, func() {}, nil
		},
	}
	type launchResult struct {
		state          agentstate.State
		path           string
		readinessToken string
		err            error
	}
	done := make(chan launchResult, 1)
	go func() {
		state, options, cleanup, err := watcher.prepareLaunch()
		cleanup()
		done <- launchResult{state: state, path: options.ConfigPath, readinessToken: options.ReadinessBody, err: err}
	}()
	select {
	case <-done:
		t.Fatal("launch snapshot passed an activation that still held the lock")
	case <-time.After(40 * time.Millisecond):
	}
	if _, err := store.Update(func(state *agentstate.State) error {
		state.ActivatedGeneration = 2
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, []byte("extensions:\n  health_check:\n    response_body:\n      healthy: revision-two\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := lock.Close(); err != nil {
		t.Fatal(err)
	}
	result := <-done
	if result.err != nil {
		t.Fatal(result.err)
	}
	if result.state.ActivatedGeneration != 2 {
		t.Fatalf("launch revision = %d, want 2", result.state.ActivatedGeneration)
	}
	if result.readinessToken != "revision-two" {
		t.Fatalf("launch readiness token = %q, want token from revision-two config", result.readinessToken)
	}
	if got := result.path; got == "" {
		t.Fatal("launch snapshot path is empty")
	}
	content, err := os.ReadFile(result.path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "healthy: revision-two") {
		t.Fatalf("launch config = %q, want revision-two token", content)
	}
}

func TestRuntimeLaunchUsesGatewayForActivatedGeneration(t *testing.T) {
	store := agentstate.NewFileStore(filepath.Join(t.TempDir(), "state.json"))
	if err := store.Save(agentstate.State{ActivatedGeneration: 8, Dependencies: map[string]agentstate.DependencyState{}}); err != nil {
		t.Fatal(err)
	}
	watcher := runtimeWatcher{
		store: store,
		gatewayRuntime: fakeGatewayRuntimeRepository{resolved: coreconfig.RuntimeGateway{
			Endpoint: "core.example:4317", Revision: 12, Generation: 8,
		}},
		options: supervisor.Options{GatewayEndpoint: "bootstrap.example:4317"},
	}
	_, options, cleanup, err := watcher.prepareLaunch()
	defer cleanup()
	if err != nil {
		t.Fatal(err)
	}
	if options.GatewayEndpoint != "core.example:4317" {
		t.Fatalf("Collector launch endpoint = %q, want Core endpoint", options.GatewayEndpoint)
	}
}

func TestRuntimeLaunchAddsGenerationReadinessToLegacyConfigSnapshot(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, "otel.yaml")
	launchPath := filepath.Join(root, "launch.yaml")
	legacy := []byte("extensions:\n  health_check:\n    endpoint: 127.0.0.1:13133\nservice:\n  extensions: [health_check]\n")
	if err := os.WriteFile(configPath, legacy, 0o600); err != nil {
		t.Fatal(err)
	}
	store := agentstate.NewFileStore(filepath.Join(root, "state.json"))
	if err := store.Save(agentstate.State{ActivatedGeneration: 2, Dependencies: map[string]agentstate.DependencyState{}}); err != nil {
		t.Fatal(err)
	}
	watcher := runtimeWatcher{
		store:   store,
		options: supervisor.Options{ConfigPath: configPath},
		snapshotConfig: func(source string, _ uint64) (string, func(), error) {
			content, err := os.ReadFile(source)
			if err != nil {
				return "", nil, err
			}
			if err := os.WriteFile(launchPath, content, 0o600); err != nil {
				return "", nil, err
			}
			return launchPath, func() {}, nil
		},
	}
	_, options, cleanup, err := watcher.prepareLaunch()
	defer cleanup()
	if err != nil {
		t.Fatal(err)
	}
	if options.ReadinessBody == "" {
		t.Fatal("legacy launch config did not receive a generation token")
	}
	active, err := os.ReadFile(configPath)
	if err != nil || string(active) != string(legacy) {
		t.Fatalf("stable legacy config changed: %q, %v", active, err)
	}
	launch, err := os.ReadFile(options.ConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	token, err := config.ReadinessTokenFromConfig(launch)
	if err != nil || token != options.ReadinessBody {
		t.Fatalf("launch readiness token = %q, %v; options token = %q", token, err, options.ReadinessBody)
	}
}

func TestRuntimeWatcherConfirmsUnchangedGatewayRevisionWithoutRestart(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, "otel.yaml")
	launchPath := filepath.Join(root, "launch.yaml")
	if err := os.WriteFile(configPath, []byte("extensions:\n  health_check:\n    endpoint: 127.0.0.1:13133\n    response_body:\n      healthy: generation-one\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	store := agentstate.NewFileStore(filepath.Join(root, "state.json"))
	if err := store.Save(agentstate.State{
		Collector:           agentstate.CollectorContext{ConfigPath: configPath},
		DesiredGeneration:   1,
		ActivatedGeneration: 1,
		Dependencies:        map[string]agentstate.DependencyState{},
	}); err != nil {
		t.Fatal(err)
	}
	repository := &pendingGatewayRuntime{
		store: store,
		gateway: coreconfig.RuntimeGateway{
			Endpoint: "core.example:4317", Revision: 10, Generation: 1,
		},
		marks: make(chan uint64, 3),
	}
	ready := make(chan struct{})
	verifyCalls := make(chan string, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	watcher := runtimeWatcher{
		store:          store,
		gatewayRuntime: repository,
		run: func(_ context.Context, options supervisor.Options) error {
			if err := options.OnReady(); err != nil {
				return err
			}
			close(ready)
			<-ctx.Done()
			return nil
		},
		options: supervisor.Options{
			ConfigPath: configPath, HealthEndpoint: "http://127.0.0.1:13133",
			GatewayEndpoint: "core.example:4317",
		},
		pollInterval: time.Millisecond,
		snapshotConfig: func(string, uint64) (string, func(), error) {
			content, err := os.ReadFile(configPath)
			if err != nil {
				return "", nil, err
			}
			if err := os.WriteFile(launchPath, content, 0o600); err != nil {
				return "", nil, err
			}
			return launchPath, func() {}, nil
		},
		verifyGatewayReadiness: func(_ context.Context, _, token string) error {
			verifyCalls <- token
			return nil
		},
	}
	runDone := make(chan error, 1)
	go func() { runDone <- watcher.Run(ctx) }()
	select {
	case <-ready:
	case <-time.After(time.Second):
		t.Fatal("Collector did not reach initial readiness")
	}
	<-repository.marks
	repository.setPendingRevision(11)
	select {
	case token := <-verifyCalls:
		if token != "generation-one" {
			t.Fatalf("readiness token = %q, want active generation token", token)
		}
	case <-time.After(time.Second):
		t.Fatal("runtime did not verify readiness for unchanged Core revision")
	}
	select {
	case generation := <-repository.marks:
		if generation != 1 {
			t.Fatalf("applied generation = %d, want unchanged local generation 1", generation)
		}
	case <-time.After(time.Second):
		t.Fatal("runtime did not mark the unchanged Core revision applied")
	}
	cancel()
	if err := <-runDone; err != nil {
		t.Fatalf("runtime watcher returned error: %v", err)
	}
	state, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if state.DesiredGeneration != 1 || state.ActivatedGeneration != 1 || state.AppliedGeneration != 1 {
		t.Fatalf("local generations changed while mapping Core revision: %+v", state)
	}
}

type pendingGatewayRuntime struct {
	mu      sync.Mutex
	store   agentstate.Repository
	gateway coreconfig.RuntimeGateway
	marks   chan uint64
}

func (r *pendingGatewayRuntime) AcceptGatewaySnapshot(coreconfig.GatewaySnapshot) (coreconfig.Acceptance, error) {
	return coreconfig.Acceptance{}, nil
}

func (r *pendingGatewayRuntime) EffectiveGateway() (coreconfig.EffectiveGateway, error) {
	return coreconfig.EffectiveGateway{}, nil
}

func (r *pendingGatewayRuntime) GatewayForGeneration(uint64) (coreconfig.RuntimeGateway, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.gateway, nil
}

func (r *pendingGatewayRuntime) MarkGatewayActivated(uint64) error { return nil }

func (r *pendingGatewayRuntime) MarkGatewayApplied(generation uint64, _ string) error {
	_, err := r.store.Update(func(state *agentstate.State) error {
		return state.MarkGenerationApplied(generation)
	})
	if err != nil {
		return err
	}
	r.mu.Lock()
	r.gateway.AppliedRevision = r.gateway.Revision
	r.gateway.NeedsReadiness = false
	r.mu.Unlock()
	r.marks <- generation
	return nil
}

func (r *pendingGatewayRuntime) setPendingRevision(revision uint64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.gateway.Revision = revision
	r.gateway.NeedsReadiness = true
}

type fakeGatewayRuntimeRepository struct {
	resolved coreconfig.RuntimeGateway
	applied  []any
}

func (r fakeGatewayRuntimeRepository) AcceptGatewaySnapshot(coreconfig.GatewaySnapshot) (coreconfig.Acceptance, error) {
	return coreconfig.Acceptance{}, nil
}

func (r fakeGatewayRuntimeRepository) EffectiveGateway() (coreconfig.EffectiveGateway, error) {
	return coreconfig.EffectiveGateway{}, nil
}

func (r fakeGatewayRuntimeRepository) GatewayForGeneration(uint64) (coreconfig.RuntimeGateway, error) {
	return r.resolved, nil
}

func (r fakeGatewayRuntimeRepository) MarkGatewayActivated(uint64) error { return nil }

func (r fakeGatewayRuntimeRepository) MarkGatewayApplied(uint64, string) error { return nil }

func TestRuntimeWatcherRestartsWhenActivationAdvancesBeforeReadinessAcknowledgement(
	t *testing.T,
) {
	store := agentstate.NewFileStore(
		filepath.Join(t.TempDir(), "state.json"),
	)

	_, err := store.Update(func(state *agentstate.State) error {
		state.DesiredGeneration = 3
		state.ActivatedGeneration = 3
		state.AppliedGeneration = 2

		return nil
	})
	if err != nil {
		t.Fatalf("initialize state: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	secondReady := make(chan struct{})
	launches := 0

	watcher := runtimeWatcher{
		store: store,
		run: func(
			ctx context.Context,
			options supervisor.Options,
		) error {
			launches++

			switch launches {
			case 1:
				_, err := store.Update(func(state *agentstate.State) error {
					state.DesiredGeneration = 4
					state.ActivatedGeneration = 4

					return nil
				})
				if err != nil {
					return err
				}

				// Generation 4 arrives before generation 3 acknowledges readiness.
				// The watcher must still restart and apply generation 4.
				return options.OnReady()

			case 2:
				if err := options.OnReady(); err != nil {
					return err
				}

				close(secondReady)
				<-ctx.Done()

				return nil

			default:
				return fmt.Errorf(
					"Collector launches = %d, want at most 2",
					launches,
				)
			}
		},
		options: supervisor.Options{
			BinaryPath:      "otelcol-contrib",
			ConfigPath:      "otel.yaml",
			HealthEndpoint:  "http://127.0.0.1:13133",
			GatewayEndpoint: "gateway.example:4317",
			StartupTimeout:  time.Second,
			ShutdownTimeout: time.Second,
		},
		pollInterval: time.Hour,
	}

	runDone := make(chan error, 1)
	go func() {
		runDone <- watcher.Run(ctx)
	}()

	select {
	case <-secondReady:
	case err := <-runDone:
		t.Fatalf(
			"runtime watcher stopped before launching generation 4: %v",
			err,
		)
	case <-time.After(time.Second):
		t.Fatal("generation 4 Collector launch did not become ready")
	}

	cancel()

	select {
	case err := <-runDone:
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("runtime watcher did not stop after cancellation")
	}

	state, err := store.Load()
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	if state.AppliedGeneration != 4 {
		t.Fatalf(
			"AppliedGeneration = %d, want 4",
			state.AppliedGeneration,
		)
	}
}

func TestRuntimeWatcherRestartsCollectorForNewActivatedGeneration(
	t *testing.T,
) {
	statePath := filepath.Join(t.TempDir(), "state.json")
	store := agentstate.NewFileStore(statePath)

	if err := store.Save(agentstate.State{
		DesiredGeneration:   1,
		ActivatedGeneration: 1,
		Dependencies:        map[string]agentstate.DependencyState{},
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	firstReady := make(chan struct{})
	firstStopped := make(chan struct{})
	secondReady := make(chan struct{})

	launches := 0

	watcher := runtimeWatcher{
		store:        store,
		pollInterval: time.Millisecond,
		options: supervisor.Options{
			BinaryPath:     "otelcol-contrib",
			ConfigPath:     "otel.yaml",
			HealthEndpoint: "http://127.0.0.1:13133",
		},
		run: func(
			ctx context.Context,
			options supervisor.Options,
		) error {
			launches++

			if err := options.OnReady(); err != nil {
				return err
			}

			switch launches {
			case 1:
				close(firstReady)

			case 2:
				close(secondReady)

			default:
				t.Fatalf("Collector launches = %d, want at most 2", launches)
			}

			<-ctx.Done()

			if launches == 1 {
				close(firstStopped)
			}

			return nil
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runDone := make(chan error, 1)

	go func() {
		runDone <- watcher.Run(ctx)
	}()

	select {
	case <-firstReady:
	case <-time.After(time.Second):
		t.Fatal("first Collector launch did not become ready")
	}

	_, err := store.Update(func(state *agentstate.State) error {
		state.DesiredGeneration = 2
		state.ActivatedGeneration = 2

		return nil
	})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	select {
	case <-firstStopped:
	case <-time.After(time.Second):
		t.Fatal("first Collector launch was not stopped")
	}

	select {
	case <-secondReady:
	case err := <-runDone:
		t.Fatalf(
			"runtime watcher stopped before replacement Collector became ready: %v",
			err,
		)
	case <-time.After(time.Second):
		t.Fatal("replacement Collector launch did not become ready")
	}

	current, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if current.AppliedGeneration != 2 {
		t.Fatalf(
			"AppliedGeneration = %d, want 2",
			current.AppliedGeneration,
		)
	}

	cancel()

	select {
	case err := <-runDone:
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("runtime watcher did not stop after cancellation")
	}
}

func TestRuntimeWatcherAppliesTeardownsAfterLaunchGenerationIsApplied(
	t *testing.T,
) {
	store := agentstate.NewFileStore(
		filepath.Join(t.TempDir(), "state.json"),
	)

	if err := store.Save(agentstate.State{
		DesiredGeneration:   3,
		ActivatedGeneration: 3,
		AppliedGeneration:   2,
		Dependencies:        map[string]agentstate.DependencyState{},
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	teardownCalls := 0

	watcher := runtimeWatcher{
		store: store,
		run: func(
			ctx context.Context,
			options supervisor.Options,
		) error {
			return options.OnReady()
		},
		options: supervisor.Options{
			BinaryPath:      "otelcol-contrib",
			ConfigPath:      "otel.yaml",
			GatewayEndpoint: "gateway.example:4317",
			HealthEndpoint:  "http://127.0.0.1:13133",
			StartupTimeout:  time.Second,
			ShutdownTimeout: time.Second,
		},
		pollInterval: time.Hour,
		applyTeardowns: func(ctx context.Context) error {
			teardownCalls++

			state, err := store.Load()
			if err != nil {
				return err
			}
			if state.AppliedGeneration != 3 {
				return fmt.Errorf(
					"AppliedGeneration = %d, want 3 before teardown",
					state.AppliedGeneration,
				)
			}

			return nil
		},
	}

	if err := watcher.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if teardownCalls != 1 {
		t.Fatalf(
			"teardown calls = %d, want 1",
			teardownCalls,
		)
	}
}

func TestRuntimeWatcherCancelsLaunchBeforeReturningReadinessError(
	t *testing.T,
) {
	store := agentstate.NewFileStore(
		filepath.Join(t.TempDir(), "state.json"),
	)
	if err := store.Save(agentstate.State{
		DesiredGeneration:   1,
		ActivatedGeneration: 1,
		Dependencies:        map[string]agentstate.DependencyState{},
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	readinessErr := errors.New("readiness failed")

	watcher := runtimeWatcher{
		store:        store,
		pollInterval: time.Millisecond,
		options: supervisor.Options{
			OnReady: func() error {
				return readinessErr
			},
		},
		run: func(ctx context.Context, options supervisor.Options) error {
			if options.OnReady == nil {
				return errors.New("Collector readiness callback is nil")
			}

			err := options.OnReady()
			if !errors.Is(err, readinessErr) {
				return fmt.Errorf(
					"OnReady() error = %v, want %v",
					err,
					readinessErr,
				)
			}

			select {
			case <-ctx.Done():
				return err
			default:
				return errors.New(
					"launch context was not cancelled after readiness failure",
				)
			}
		},
	}

	err := watcher.Run(context.Background())
	if !errors.Is(err, readinessErr) {
		t.Fatalf("Run() error = %v, want readiness error", err)
	}
}

func TestRuntimeWatcherCompletesPendingUninstallAfterCollectorReady(
	t *testing.T,
) {
	store := agentstate.NewFileStore(
		filepath.Join(t.TempDir(), "state.json"),
	)

	if err := store.Save(agentstate.State{
		DesiredGeneration:   3,
		ActivatedGeneration: 3,
		AppliedGeneration:   2,
		Dependencies: map[string]agentstate.DependencyState{
			"windows-exporter": {
				Enabled: false,
				Ownership: &agentstate.OwnershipRecord{
					Resources: []agentstate.OwnedResource{
						{
							Kind:       "windows-service",
							Identifier: "windows_exporter",
						},
					},
				},
				PendingTeardown: &agentstate.PendingTeardown{
					Action:     agentstate.TeardownActionUninstall,
					Generation: 3,
				},
			},
		},
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	teardownCalls := 0

	coordinator := teardownCoordinator{
		store: store,
		teardown: func(
			ctx context.Context,
			name string,
			action agentstate.TeardownAction,
			resources []agentstate.OwnedResource,
		) error {
			teardownCalls++

			if name != "windows-exporter" {
				t.Fatalf("teardown name = %q, want windows-exporter", name)
			}
			if action != agentstate.TeardownActionUninstall {
				t.Fatalf(
					"teardown action = %q, want %q",
					action,
					agentstate.TeardownActionUninstall,
				)
			}
			if len(resources) != 1 ||
				resources[0].Kind != "windows-service" ||
				resources[0].Identifier != "windows_exporter" {
				t.Fatalf("teardown resources = %#v", resources)
			}

			return nil
		},
	}

	watcher := runtimeWatcher{
		store: store,
		run: func(
			ctx context.Context,
			options supervisor.Options,
		) error {
			return options.OnReady()
		},
		options: supervisor.Options{
			BinaryPath:      "otelcol-contrib",
			ConfigPath:      "otel.yaml",
			GatewayEndpoint: "gateway.example:4317",
			HealthEndpoint:  "http://127.0.0.1:13133",
			StartupTimeout:  time.Second,
			ShutdownTimeout: time.Second,
		},
		pollInterval:   time.Hour,
		applyTeardowns: coordinator.Apply,
	}

	if err := watcher.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if teardownCalls != 1 {
		t.Fatalf("teardown calls = %d, want 1", teardownCalls)
	}

	state, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if state.AppliedGeneration != 3 {
		t.Fatalf(
			"AppliedGeneration = %d, want 3",
			state.AppliedGeneration,
		)
	}
	if _, exists := state.Dependencies["windows-exporter"]; exists {
		t.Fatal("windows-exporter still exists after uninstall teardown")
	}
}

func TestRuntimeWatcherReportsHealthySnapshotAfterCollectorReady(
	t *testing.T,
) {
	store := agentstate.NewFileStore(
		filepath.Join(t.TempDir(), "state.json"),
	)

	if err := store.Save(agentstate.State{
		DesiredGeneration:   3,
		ActivatedGeneration: 3,
		AppliedGeneration:   2,
		Dependencies:        map[string]agentstate.DependencyState{},
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	reporter := agenthealth.NewReporter(
		agenthealth.RuntimeObservation{
			CollectorState:      agenthealth.CollectorStateStarting,
			DesiredGeneration:   3,
			ActivatedGeneration: 3,
			AppliedGeneration:   2,
		},
	)

	watcher := runtimeWatcher{
		store: store,
		run: func(
			ctx context.Context,
			options supervisor.Options,
		) error {
			if options.OnReady == nil {
				t.Fatal("OnReady is nil")
			}

			return options.OnReady()
		},
		options: supervisor.Options{
			OnReady: func() error {
				return nil
			},
		},
		pollInterval:   time.Hour,
		healthReporter: reporter,
	}

	if err := watcher.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	snapshot := reporter.Snapshot()
	t.Logf("health snapshot = %#v", snapshot)

	if snapshot.Status != agenthealth.StatusHealthy {
		t.Fatalf(
			"health status = %q, want %q",
			snapshot.Status,
			agenthealth.StatusHealthy,
		)
	}

	if snapshot.AppliedGeneration != 3 {
		t.Fatalf(
			"applied generation = %d, want 3",
			snapshot.AppliedGeneration,
		)
	}
}

func TestRuntimeWatcherReportsFailedSnapshotWhenCollectorExitsWithError(
	t *testing.T,
) {
	store := agentstate.NewFileStore(
		filepath.Join(t.TempDir(), "state.json"),
	)

	if err := store.Save(agentstate.State{
		DesiredGeneration:   3,
		ActivatedGeneration: 3,
		AppliedGeneration:   3,
		Dependencies:        map[string]agentstate.DependencyState{},
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	reporter := agenthealth.NewReporter(
		agenthealth.RuntimeObservation{
			CollectorState:      agenthealth.CollectorStateStarting,
			DesiredGeneration:   3,
			ActivatedGeneration: 3,
			AppliedGeneration:   3,
		},
	)
	wantError := errors.New("Collector exited unexpectedly")

	watcher := runtimeWatcher{
		store: store,
		run: func(
			context.Context,
			supervisor.Options,
		) error {
			return wantError
		},
		pollInterval:   time.Hour,
		healthReporter: reporter,
	}

	err := watcher.Run(context.Background())
	if !errors.Is(err, wantError) {
		t.Fatalf("Run() error = %v, want %v", err, wantError)
	}

	snapshot := reporter.Snapshot()

	if snapshot.Status != agenthealth.StatusDegraded {
		t.Fatalf(
			"health status = %q, want %q",
			snapshot.Status,
			agenthealth.StatusDegraded,
		)
	}
	if snapshot.CollectorState != agenthealth.CollectorStateFailed {
		t.Fatalf(
			"Collector state = %q, want %q",
			snapshot.CollectorState,
			agenthealth.CollectorStateFailed,
		)
	}
	if snapshot.LastError != wantError.Error() {
		t.Fatalf(
			"last error = %q, want %q",
			snapshot.LastError,
			wantError.Error(),
		)
	}
}

func TestRuntimeWatcherRestartsWhenCollectorStopsCleanlyAfterNewGenerationActivates(
	t *testing.T,
) {
	store := agentstate.NewFileStore(
		filepath.Join(t.TempDir(), "state.json"),
	)

	if err := store.Save(agentstate.State{
		DesiredGeneration:   1,
		ActivatedGeneration: 1,
		Dependencies:        map[string]agentstate.DependencyState{},
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	firstReady := make(chan struct{})
	releaseFirst := make(chan struct{})
	secondReady := make(chan struct{})
	launches := 0

	watcher := runtimeWatcher{
		store:        store,
		pollInterval: time.Hour, // force the clean-exit path before polling
		run: func(
			ctx context.Context,
			options supervisor.Options,
		) error {
			launches++

			if err := options.OnReady(); err != nil {
				return err
			}

			switch launches {
			case 1:
				close(firstReady)
				<-releaseFirst

				// Reproduces a Collector that exits cleanly just after a
				// newer generation has been activated.
				return nil

			case 2:
				close(secondReady)
				<-ctx.Done()

				return nil

			default:
				t.Fatalf("Collector launches = %d, want at most 2", launches)

				return nil
			}
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runDone := make(chan error, 1)
	go func() {
		runDone <- watcher.Run(ctx)
	}()

	select {
	case <-firstReady:
	case <-time.After(time.Second):
		t.Fatal("first Collector launch did not become ready")
	}

	_, err := store.Update(func(state *agentstate.State) error {
		state.DesiredGeneration = 2
		state.ActivatedGeneration = 2

		return nil
	})
	if err != nil {
		t.Fatalf("activate replacement generation: %v", err)
	}

	close(releaseFirst)

	select {
	case err := <-runDone:
		t.Fatalf(
			"runtime watcher stopped after clean Collector exit: %v",
			err,
		)

	case <-secondReady:
	case <-time.After(time.Second):
		t.Fatal("replacement Collector launch did not become ready")
	}

	state, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if state.AppliedGeneration != 2 {
		t.Fatalf(
			"AppliedGeneration = %d, want 2",
			state.AppliedGeneration,
		)
	}

	cancel()

	select {
	case err := <-runDone:
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}

	case <-time.After(time.Second):
		t.Fatal("runtime watcher did not stop after cancellation")
	}
}
