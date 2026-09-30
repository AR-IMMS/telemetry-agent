package agentlifecycle

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/ar-imms/telemetry-agent/internal/agenthealth"
	"github.com/ar-imms/telemetry-agent/internal/agentstate"
	"github.com/ar-imms/telemetry-agent/internal/supervisor"
)

func TestRuntimeWatcherMarksLaunchGenerationAppliedAfterCollectorReady(
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

	ready := make(chan struct{})

	watcher := runtimeWatcher{
		store: store,
		run: func(
			ctx context.Context,
			options supervisor.Options,
		) error {
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
			ConfigPath:      "otel.yaml",
			HealthEndpoint:  "http://127.0.0.1:13133",
			GatewayEndpoint: "gateway.example:4317",
			StartupTimeout:  time.Second,
			ShutdownTimeout: time.Second,
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

func TestRuntimeWatcherAcknowledgesLaunchGenerationWhenActivationAdvances(
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

	watcher := runtimeWatcher{
		store: store,
		run: func(
			ctx context.Context,
			options supervisor.Options,
		) error {
			_, err := store.Update(
				func(state *agentstate.State) error {
					state.DesiredGeneration = 4
					state.ActivatedGeneration = 4

					return nil
				},
			)
			if err != nil {
				return err
			}

			return options.OnReady()
		},
		options: supervisor.Options{
			BinaryPath:      "otelcol-contrib",
			ConfigPath:      "otel.yaml",
			HealthEndpoint:  "http://127.0.0.1:13133",
			GatewayEndpoint: "gateway.example:4317",
			StartupTimeout:  time.Second,
			ShutdownTimeout: time.Second,
		},

		// Keep the watcher from restarting; this test isolates OnReady's
		// launch-generation acknowledgement.
		pollInterval: time.Hour,
	}

	if err := watcher.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	state, err := store.Load()
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	if state.AppliedGeneration != 3 {
		t.Fatalf(
			"AppliedGeneration = %d, want launch generation 3",
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
