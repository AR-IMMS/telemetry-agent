package agentlifecycle

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ar-imms/telemetry-agent/internal/agentstate"
	"github.com/ar-imms/telemetry-agent/internal/bootstrap"
	"github.com/ar-imms/telemetry-agent/internal/identity"
	"github.com/ar-imms/telemetry-agent/internal/supervisor"
)

func TestManagedSupervisorRuntimeDeliversOTLP(t *testing.T) {
	if os.Getenv("AR_IMMS_PR07C_RUNTIME_E2E") != "1" {
		t.Skip("set AR_IMMS_PR07C_RUNTIME_E2E=1 to run pinned Agent-managed Supervisor E2E")
	}
	if !canRunSupervisorRuntimeE2E() {
		t.Skip("protected Windows credential-store E2E requires an elevated process")
	}
	supervisorPath := strings.TrimSpace(os.Getenv("AR_IMMS_PR07A_SUPERVISOR_BINARY"))
	collectorPath := strings.TrimSpace(os.Getenv("AR_IMMS_PR05_COLLECTOR_BINARY"))
	if supervisorPath == "" || collectorPath == "" {
		t.Fatal("verified Supervisor and Collector binary paths are required")
	}
	platform := identity.PlatformInfo{OS: runtime.GOOS, Architecture: runtime.GOARCH}
	artifact, err := bootstrap.SelectSupervisorArtifact(platform)
	if err != nil {
		t.Fatal(err)
	}
	if got := fileSHA256(t, supervisorPath); got != artifact.SHA256 {
		t.Fatalf("Supervisor binary hash = %s, want pinned %s", got, artifact.SHA256)
	}
	for _, path := range []string{supervisorPath, collectorPath} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("inspect pinned binary %q: %v", path, err)
		}
	}
	collectorVersion := exec.Command(collectorPath, "--version")
	collectorVersionOutput, err := collectorVersion.CombinedOutput()
	if err != nil || !strings.Contains(string(collectorVersionOutput), "version 0.160.0") {
		t.Fatalf("Collector binary is not the pinned v0.160.0 build: err=%v output=%s", err, collectorVersionOutput)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	root := t.TempDir()
	sink, sinkEndpoint := startSupervisorRuntimeSink(t, ctx, collectorPath, root)

	healthEndpoint := reserveSupervisorRuntimeEndpoint(t)
	collectorConfig := filepath.Join(root, "local-collector.yaml")
	collectorYAML := fmt.Sprintf(`receivers:
  hostmetrics:
    collection_interval: 1s
    scrapers:
      cpu: {}
      memory: {}
exporters:
  otlp:
    endpoint: %q
    tls:
      insecure: true
extensions:
  health_check:
    endpoint: %q
service:
  extensions: [health_check]
  pipelines:
    metrics:
      receivers: [hostmetrics]
      exporters: [otlp]
`, sinkEndpoint, strings.TrimPrefix(healthEndpoint, "http://"))
	if err := os.WriteFile(collectorConfig, []byte(collectorYAML), 0o600); err != nil {
		t.Fatal(err)
	}
	validation := exec.CommandContext(ctx, collectorPath, "validate", "--config", collectorConfig)
	if result, err := validation.CombinedOutput(); err != nil {
		t.Fatalf("validate local Collector config: %v\n%s", err, result)
	}

	statePath := filepath.Join(root, "state.db")
	managed := agentstate.SupervisorContext{
		BinaryPath:        supervisorPath,
		ConfigurationPath: filepath.Join(root, "credentials", "opamp-supervisor.yaml"),
		StateDirectory:    filepath.Join(root, "credentials", "opamp-supervisor-state"),
		OpAMPServerURL:    "ws://127.0.0.1:4321/v1/opamp",
		Enabled:           true,
	}
	options := supervisor.Options{
		BinaryPath:      collectorPath,
		ConfigPath:      collectorConfig,
		HealthEndpoint:  healthEndpoint,
		GatewayEndpoint: sinkEndpoint,
		StartupTimeout:  30 * time.Second,
		ShutdownTimeout: 10 * time.Second,
	}
	runContext, stopRuntime := context.WithCancel(ctx)
	runtimeDone := make(chan error, 1)
	go func() { runtimeDone <- runManagedSupervisor(runContext, statePath, managed, options) }()
	waitForSupervisorRuntimeReadiness(t, ctx, runtimeDone, healthEndpoint)
	waitForSupervisorSinkTelemetry(t, ctx, runtimeDone, sink)
	stopRuntime()
	select {
	case err := <-runtimeDone:
		if err != nil {
			t.Fatalf("Agent-managed Supervisor shutdown: %v", err)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("Agent-managed Supervisor process tree did not stop")
	}
	writeSupervisorRuntimeEvidence(t, supervisorPath, collectorPath, healthEndpoint, sinkEndpoint, sink.String())
}

func startSupervisorRuntimeSink(t *testing.T, ctx context.Context, collectorPath, root string) (*supervisorRuntimeBuffer, string) {
	t.Helper()
	endpoint := reserveSupervisorRuntimeEndpoint(t)
	configPath := filepath.Join(root, "sink.yaml")
	content := fmt.Sprintf(`receivers:
  otlp:
    protocols:
      grpc:
        endpoint: %q
exporters:
  debug:
    verbosity: detailed
service:
  pipelines:
    metrics:
      receivers: [otlp]
      exporters: [debug]
`, strings.TrimPrefix(endpoint, "http://"))
	if err := os.WriteFile(configPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	validation := exec.CommandContext(ctx, collectorPath, "validate", "--config", configPath)
	if result, err := validation.CombinedOutput(); err != nil {
		t.Fatalf("validate isolated OTLP sink: %v\n%s", err, result)
	}
	output := &supervisorRuntimeBuffer{}
	command := exec.CommandContext(ctx, collectorPath, "--config", configPath)
	command.Stdout, command.Stderr = output, output
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
	return output, endpoint
}

func reserveSupervisorRuntimeEndpoint(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	endpoint := "http://" + listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	return endpoint
}

func waitForSupervisorRuntimeReadiness(t *testing.T, ctx context.Context, done <-chan error, endpoint string) {
	t.Helper()
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	client := &http.Client{Timeout: time.Second}
	for {
		request, _ := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		response, err := client.Do(request)
		if err == nil {
			response.Body.Close()
			if response.StatusCode >= 200 && response.StatusCode < 300 {
				return
			}
		}
		select {
		case err := <-done:
			t.Fatalf("Supervisor exited before Collector readiness: %v", err)
		case <-ctx.Done():
			t.Fatalf("timed out waiting for Collector readiness: %v", ctx.Err())
		case <-ticker.C:
		}
	}
}

func waitForSupervisorSinkTelemetry(t *testing.T, ctx context.Context, done <-chan error, output *supervisorRuntimeBuffer) {
	t.Helper()
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		if strings.Contains(output.String(), "system.cpu.time") || strings.Contains(output.String(), "system.memory.usage") {
			return
		}
		select {
		case err := <-done:
			t.Fatalf("Supervisor exited before OTLP telemetry arrived: %v", err)
		case <-ctx.Done():
			t.Fatalf("timed out waiting for OTLP telemetry: %v", ctx.Err())
		case <-ticker.C:
		}
	}
}

func writeSupervisorRuntimeEvidence(t *testing.T, supervisorPath, collectorPath, healthEndpoint, sinkEndpoint, output string) {
	t.Helper()
	evidenceDirectory := strings.TrimSpace(os.Getenv("AR_IMMS_PR07C_EVIDENCE_DIR"))
	if evidenceDirectory == "" {
		return
	}
	if err := os.MkdirAll(evidenceDirectory, 0o700); err != nil {
		t.Fatalf("create Supervisor runtime evidence directory: %v", err)
	}
	summary := fmt.Sprintf("result=PASS\nos=%s\narchitecture=%s\ncommit=%s\nsupervisor_sha256=%s\ncollector_sha256=%s\nhealth_endpoint=%s\notlp_sink_endpoint=%s\ntelemetry_marker_observed=%t\n", runtime.GOOS, runtime.GOARCH, os.Getenv("GITHUB_SHA"), fileSHA256(t, supervisorPath), fileSHA256(t, collectorPath), healthEndpoint, sinkEndpoint, strings.Contains(output, "system.cpu.time") || strings.Contains(output, "system.memory.usage"))
	if err := os.WriteFile(filepath.Join(evidenceDirectory, "managed-supervisor-runtime.txt"), []byte(summary), 0o600); err != nil {
		t.Fatalf("write Supervisor runtime evidence: %v", err)
	}
}

func fileSHA256(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

type supervisorRuntimeBuffer struct {
	mu      sync.Mutex
	content bytes.Buffer
}

func (b *supervisorRuntimeBuffer) Write(content []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.content.Write(content)
}

func (b *supervisorRuntimeBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.content.String()
}
