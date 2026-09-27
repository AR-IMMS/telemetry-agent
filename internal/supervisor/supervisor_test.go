package supervisor

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"
)

func TestSupervisorStopsChildAfterContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	child := &fakeChild{
		exited:        make(chan ExitResult, 1),
		stopRequested: make(chan struct{}),
	}

	supervisor := newSupervisor(
		fakeStarter{child: child},
		func(context.Context) error {
			return nil
		},
		time.Second,
	)

	runDone := make(chan error, 1)

	go func() {
		runDone <- supervisor.Run(ctx, LaunchOptions{
			BinaryPath: "C:/agent/bin/otelcol-contrib.exe",
			ConfigPath: "C:/agent/config/otel.yaml",
		})
	}()

	cancel()

	select {
	case <-child.stopRequested:
	case <-time.After(time.Second):
		t.Fatal("supervisor did not request graceful child shutdown")
	}

	child.exited <- ExitResult{
		Code: 0,
	}

	select {
	case err := <-runDone:
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}

	case <-time.After(time.Second):
		t.Fatal("supervisor did not finish after child exit")
	}
}

type fakeStarter struct {
	child Child
}

func (s fakeStarter) Start(LaunchOptions) (Child, error) {
	return s.child, nil
}

type fakeChild struct {
	exited        chan ExitResult
	stopRequested chan struct{}
	killRequested chan struct{}
}

func (c *fakeChild) Wait() <-chan ExitResult {
	return c.exited
}

func (c *fakeChild) RequestStop() error {
	close(c.stopRequested)
	return nil
}

func (c *fakeChild) Kill() error {
	if c.killRequested != nil {
		close(c.killRequested)
	}

	return nil
}

func TestSupervisorStopsChildWhenReadinessFails(t *testing.T) {
	readinessError := errors.New("Collector health endpoint is unavailable")

	child := &fakeChild{
		exited:        make(chan ExitResult, 1),
		stopRequested: make(chan struct{}),
	}

	supervisor := newSupervisor(
		fakeStarter{child: child},
		func(context.Context) error {
			return readinessError
		},
		time.Second,
	)

	runDone := make(chan error, 1)

	go func() {
		runDone <- supervisor.Run(
			context.Background(),
			LaunchOptions{
				BinaryPath: "C:/agent/bin/otelcol-contrib.exe",
				ConfigPath: "C:/agent/config/otel.yaml",
			},
		)
	}()

	select {
	case <-child.stopRequested:
		child.exited <- ExitResult{Code: 0}

	case <-time.After(time.Second):
		t.Fatal("supervisor did not stop Collector after readiness failure")
	}

	select {
	case err := <-runDone:
		if !errors.Is(err, readinessError) {
			t.Fatalf(
				"Run() error = %v, want wrapped readiness error %v",
				err,
				readinessError,
			)
		}

	case <-time.After(time.Second):
		t.Fatal("supervisor did not finish after Collector stopped")
	}
}

func TestSupervisorFailsWhenCollectorExitsBeforeReady(t *testing.T) {
	collectorExitError := errors.New("Collector exited before health became ready")

	child := &fakeChild{
		exited:        make(chan ExitResult, 1),
		stopRequested: make(chan struct{}),
	}

	readinessStarted := make(chan struct{})

	supervisor := newSupervisor(
		fakeStarter{child: child},
		func(ctx context.Context) error {
			close(readinessStarted)
			<-ctx.Done()

			return ctx.Err()
		},
		time.Second,
	)

	runDone := make(chan error, 1)

	go func() {
		runDone <- supervisor.Run(
			context.Background(),
			LaunchOptions{
				BinaryPath: "C:/agent/bin/otelcol-contrib.exe",
				ConfigPath: "C:/agent/config/otel.yaml",
			},
		)
	}()

	select {
	case <-readinessStarted:
	case <-time.After(time.Second):
		t.Fatal("supervisor did not begin readiness checking")
	}

	child.exited <- ExitResult{
		Code: 1,
		Err:  collectorExitError,
	}

	select {
	case err := <-runDone:
		if !errors.Is(err, collectorExitError) {
			t.Fatalf(
				"Run() error = %v, want wrapped Collector exit error %v",
				err,
				collectorExitError,
			)
		}

	case <-time.After(time.Second):
		t.Fatal("supervisor did not finish after Collector exit")
	}
}

func TestSupervisorForceKillsChildAfterShutdownTimeout(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	child := &fakeChild{
		exited:        make(chan ExitResult),
		stopRequested: make(chan struct{}),
		killRequested: make(chan struct{}),
	}

	supervisor := newSupervisor(
		fakeStarter{child: child},
		func(context.Context) error {
			return nil
		},
		10*time.Millisecond,
	)

	runDone := make(chan error, 1)

	go func() {
		runDone <- supervisor.Run(
			ctx,
			LaunchOptions{
				BinaryPath: "C:/agent/bin/otelcol-contrib.exe",
				ConfigPath: "C:/agent/config/otel.yaml",
			},
		)
	}()

	cancel()

	select {
	case <-child.stopRequested:
	case <-time.After(time.Second):
		t.Fatal("supervisor did not request graceful child shutdown")
	}

	select {
	case <-child.killRequested:
	case <-time.After(time.Second):
		t.Fatal("supervisor did not force-kill child after shutdown timeout")
	}

	select {
	case err := <-runDone:
		if err == nil {
			t.Fatal("Run() error = nil, want forced-shutdown error")
		}

	case <-time.After(time.Second):
		t.Fatal("supervisor did not finish after force-killing child")
	}
}

func TestRunRejectsMissingHealthEndpoint(t *testing.T) {
	err := Run(
		context.Background(),
		Options{
			BinaryPath:      "C:/agent/bin/otelcol-contrib.exe",
			ConfigPath:      "C:/agent/config/otel.yaml",
			StartupTimeout:  30 * time.Second,
			ShutdownTimeout: 10 * time.Second,
		},
	)

	if err == nil {
		t.Fatal("Run() error = nil, want an error for missing health endpoint")
	}
}

func TestRunWithStopsChildWhenStartupTimeoutExpires(t *testing.T) {
	server := httptest.NewServer(
		http.HandlerFunc(func(
			writer http.ResponseWriter,
			request *http.Request,
		) {
			writer.WriteHeader(http.StatusServiceUnavailable)
		}),
	)
	defer server.Close()

	child := &fakeChild{
		exited:        make(chan ExitResult, 1),
		stopRequested: make(chan struct{}),
	}

	go func() {
		<-child.stopRequested
		child.exited <- ExitResult{Code: 0}
	}()

	err := runWith(
		context.Background(),
		Options{
			BinaryPath:      "C:/agent/bin/otelcol-contrib.exe",
			ConfigPath:      "C:/agent/config/otel.yaml",
			HealthEndpoint:  server.URL,
			GatewayEndpoint: "gateway.example:4317",
			StartupTimeout:  10 * time.Millisecond,
			ShutdownTimeout: time.Second,
		},
		fakeStarter{child: child},
		server.Client(),
	)

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf(
			"runWith() error = %v, want wrapped context deadline exceeded",
			err,
		)
	}
}

func TestOptionsValidateRejectsMissingGatewayEndpoint(t *testing.T) {
	options := Options{
		BinaryPath:      "C:/agent/bin/otelcol-contrib.exe",
		ConfigPath:      "C:/agent/config/otel.yaml",
		HealthEndpoint:  "http://127.0.0.1:13133",
		StartupTimeout:  30 * time.Second,
		ShutdownTimeout: 10 * time.Second,
	}

	if err := options.validate(); err == nil {
		t.Fatal("Options.validate() error = nil, want an error")
	}
}

func TestRunWithPassesGatewayEndpointToChild(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	child := &fakeChild{
		exited:        make(chan ExitResult, 1),
		stopRequested: make(chan struct{}),
	}

	startedWith := make(chan LaunchOptions, 1)

	starter := capturingStarter{
		child:       child,
		startedWith: startedWith,
	}

	runDone := make(chan error, 1)

	go func() {
		runDone <- runWith(
			ctx,
			Options{
				BinaryPath:      "C:/agent/bin/otelcol-contrib.exe",
				ConfigPath:      "C:/agent/config/otel.yaml",
				HealthEndpoint:  "http://127.0.0.1:13133",
				GatewayEndpoint: "gateway.example:4317",
				StartupTimeout:  30 * time.Second,
				ShutdownTimeout: time.Second,
			},
			starter,
			http.DefaultClient,
		)
	}()

	select {
	case options := <-startedWith:
		want := []string{
			"OTEL_GATEWAY_ENDPOINT=gateway.example:4317",
		}

		if !reflect.DeepEqual(options.Environment, want) {
			t.Fatalf(
				"LaunchOptions.Environment = %q, want %q",
				options.Environment,
				want,
			)
		}

	case <-time.After(time.Second):
		t.Fatal("supervisor did not start Collector")
	}

	cancel()

	select {
	case <-child.stopRequested:
		child.exited <- ExitResult{Code: 0}

	case <-time.After(time.Second):
		t.Fatal("supervisor did not request Collector shutdown")
	}

	if err := <-runDone; err != nil {
		t.Fatalf("runWith() error = %v", err)
	}
}

type capturingStarter struct {
	child       Child
	startedWith chan<- LaunchOptions
}

func (s capturingStarter) Start(options LaunchOptions) (Child, error) {
	s.startedWith <- options
	return s.child, nil
}

func TestSupervisorForceKillsChildWhenReadinessStopRequestFails(t *testing.T) {
	readinessErr := errors.New("Collector is not ready")
	child := newStopRequestFailureChild(
		errors.New("graceful shutdown request failed"),
	)

	supervisor := newSupervisor(
		stopRequestFailureStarter{child: child},
		func(context.Context) error {
			return readinessErr
		},
		time.Second,
	)

	err := supervisor.Run(context.Background(), LaunchOptions{
		BinaryPath: "collector",
		ConfigPath: "otel.yaml",
	})

	if !errors.Is(err, readinessErr) {
		t.Fatalf("Run() error = %v, want wrapped readiness error", err)
	}

	if child.killCalls != 1 {
		t.Fatalf("Kill() calls = %d, want 1", child.killCalls)
	}
}

func TestSupervisorShutdownForceKillsChildWhenStopRequestFails(t *testing.T) {
	child := newStopRequestFailureChild(
		errors.New("graceful shutdown request failed"),
	)

	supervisor := newSupervisor(nil, nil, time.Second)

	err := supervisor.shutdown(newLifecycle(), child)
	if err == nil {
		t.Fatal("shutdown() error = nil, want stop-request error")
	}

	if child.killCalls != 1 {
		t.Fatalf("Kill() calls = %d, want 1", child.killCalls)
	}
}

type stopRequestFailureStarter struct {
	child Child
}

func (s stopRequestFailureStarter) Start(LaunchOptions) (Child, error) {
	return s.child, nil
}

type stopRequestFailureChild struct {
	exited    chan ExitResult
	stopErr   error
	killCalls int
}

func newStopRequestFailureChild(stopErr error) *stopRequestFailureChild {
	return &stopRequestFailureChild{
		exited:  make(chan ExitResult, 1),
		stopErr: stopErr,
	}
}

func (c *stopRequestFailureChild) Wait() <-chan ExitResult {
	return c.exited
}

func (c *stopRequestFailureChild) RequestStop() error {
	return c.stopErr
}

func (c *stopRequestFailureChild) Kill() error {
	c.killCalls++

	c.exited <- ExitResult{}
	close(c.exited)

	return nil
}

func TestRunWithCallsOnReadyAfterCollectorPassesReadiness(
	t *testing.T,
) {
	server := httptest.NewServer(
		http.HandlerFunc(func(
			writer http.ResponseWriter,
			request *http.Request,
		) {
			writer.WriteHeader(http.StatusOK)
		}),
	)
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	child := &fakeChild{
		exited:        make(chan ExitResult, 1),
		stopRequested: make(chan struct{}),
	}

	onReadyCalls := 0
	readyCalled := make(chan struct{})

	runDone := make(chan error, 1)

	go func() {
		runDone <- runWith(
			ctx,
			Options{
				BinaryPath:      "C:/agent/bin/otelcol-contrib.exe",
				ConfigPath:      "C:/agent/config/otel.yaml",
				HealthEndpoint:  server.URL,
				GatewayEndpoint: "gateway.example:4317",
				StartupTimeout:  time.Second,
				ShutdownTimeout: time.Second,
				OnReady: func() error {
					onReadyCalls++
					close(readyCalled)

					return nil
				},
			},
			fakeStarter{child: child},
			server.Client(),
		)
	}()

	select {
	case <-readyCalled:
	case <-time.After(time.Second):
		t.Fatal("OnReady was not called after Collector became ready")
	}

	if onReadyCalls != 1 {
		t.Fatalf("OnReady calls = %d, want 1", onReadyCalls)
	}

	cancel()

	select {
	case <-child.stopRequested:
		child.exited <- ExitResult{Code: 0}

	case <-time.After(time.Second):
		t.Fatal("supervisor did not request Collector shutdown")
	}

	select {
	case err := <-runDone:
		if err != nil {
			t.Fatalf("runWith() error = %v", err)
		}

	case <-time.After(time.Second):
		t.Fatal("supervisor did not finish after cancellation")
	}
}
