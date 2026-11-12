package main

import (
	"context"
	"fmt"
	"net"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ar-imms/telemetry-agent/internal/coreapi"
	"github.com/ar-imms/telemetry-agent/internal/mockcore"
	"github.com/ar-imms/telemetry-agent/internal/opampserver"
)

func TestPR07BSupervisorAuthenticatesAndAppliesMockCorePolicy(t *testing.T) {
	if os.Getenv("AR_IMMS_PR07B_SUPERVISOR_E2E") != "1" {
		t.Skip("set AR_IMMS_PR07B_SUPERVISOR_E2E=1 to run pinned Supervisor/Core E2E")
	}
	supervisorBinary := strings.TrimSpace(os.Getenv("AR_IMMS_PR07A_SUPERVISOR_BINARY"))
	collectorBinary := strings.TrimSpace(os.Getenv("AR_IMMS_PR05_COLLECTOR_BINARY"))
	if supervisorBinary == "" || collectorBinary == "" {
		t.Fatal("verified Supervisor and Collector binary paths are required")
	}
	for _, binary := range []string{supervisorBinary, collectorBinary} {
		if _, err := os.Stat(binary); err != nil {
			t.Fatalf("inspect verified binary %q: %v", binary, err)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	root, err := os.MkdirTemp("", "pr07b-supervisor-core-e2e-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })

	const token = "pr07b-local-test-bearer-never-use-in-production"
	core, err := mockcore.New(mockcore.ProvisionedInstallation{
		AccessToken: token, InstallationID: "11111111-2222-4333-8444-555555555555",
		NodeID: "33333333-4444-4555-8666-777777777777",
		Policy: coreapi.DesiredGatewayPolicy{SchemaVersion: "ar-imms/gateway-policy/v1", Revision: 1, GatewayEndpoint: "127.0.0.1:4317"},
	})
	if err != nil {
		t.Fatal(err)
	}
	coreHTTP := httptest.NewTLSServer(core)
	defer coreHTTP.Close()
	coreFactory := func(bearer string) (coreapi.AgentManagementAPI, error) {
		return coreapi.NewAgentManagementClient(coreHTTP.URL, bearer, coreHTTP.Client())
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	opampEndpoint := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	adapter, err := opampserver.New(opampserver.Options{ListenEndpoint: opampEndpoint, ListenPath: "/v1/opamp", CoreClient: coreFactory})
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer stopCancel()
		if err := adapter.Stop(stopCtx); err != nil {
			t.Errorf("stop OpAMP adapter: %v", err)
		}
	}()

	tokenPath := filepath.Join(root, "supervisor-opamp.token")
	if err := os.WriteFile(tokenPath, []byte(token), 0600); err != nil {
		t.Fatal(err)
	}
	collectorConfig := filepath.Join(root, "collector.yaml")
	collectorFallback := filepath.Join(root, "fallback.yaml")
	writePR07ACollectorConfig(t, collectorConfig, "127.0.0.1:4317")
	writePR07ACollectorConfig(t, collectorFallback, "127.0.0.1:4317")
	supervisorConfig := fmt.Sprintf(`server:
  endpoint: ws://%s/v1/opamp
  auth: bearertokenauth
capabilities:
  accepts_remote_config: true
  reports_remote_config: true
agent:
  executable: %q
  config_files:
    - %q
    - $REMOTE_CONFIG
  startup_fallback_configs:
    - %q
  automatic_config_rollback: true
storage:
  directory: %q
extensions:
  bearertokenauth:
    filename: %q
telemetry:
  logs:
    level: info
`, opampEndpoint, collectorBinary, collectorConfig, collectorFallback, filepath.Join(root, "supervisor-state"), tokenPath)
	configPath := filepath.Join(root, "supervisor.yaml")
	if err := os.WriteFile(configPath, []byte(supervisorConfig), 0600); err != nil {
		t.Fatal(err)
	}
	command := exec.CommandContext(ctx, supervisorBinary, "--feature-gates=opampsupervisor.Extensions", "--config="+configPath)
	output := &lockedTemplateE2EBuffer{}
	command.Stdout, command.Stderr = output, output
	if err := command.Start(); err != nil {
		t.Fatalf("start pinned Supervisor: %v", err)
	}
	done := make(chan struct{})
	var processErr error
	go func() {
		processErr = command.Wait()
		close(done)
	}()
	t.Cleanup(func() {
		select {
		case <-done:
			return
		default:
			stopPR07ASupervisorProcess(command.Process)
		}
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			t.Errorf("Supervisor did not stop")
		}
	})

	deadline := time.NewTimer(90 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		statuses := core.Statuses()
		for _, observation := range statuses {
			if observation.Report.Status == "APPLIED" && observation.Report.AppliedRevision == 1 {
				if observation.Report.InstanceUID == "" {
					t.Fatal("status was not bound to an OpAMP instance UID")
				}
				writePR07BSupervisorEvidence(t, strings.TrimSpace(os.Getenv("AR_IMMS_PR07B_EVIDENCE_DIR")), output.String(), observation.Report)
				return
			}
		}
		select {
		case <-done:
			t.Fatalf("Supervisor exited before applying authenticated policy (exit=%v); logs: %s", processErr, redactPR07BToken(output.String(), token))
		case <-deadline.C:
			t.Fatalf("timed out waiting for authenticated Supervisor policy status; logs: %s", redactPR07BToken(output.String(), token))
		case <-ticker.C:
		}
	}
}

func writePR07BSupervisorEvidence(t *testing.T, directory, supervisorLog string, report coreapi.SupervisorStatusReport) {
	t.Helper()
	if directory == "" {
		return
	}
	if err := os.MkdirAll(directory, 0700); err != nil {
		t.Fatalf("create PR-07b evidence directory: %v", err)
	}
	files := map[string]string{
		"supervisor.log": redactPR07BToken(supervisorLog, "pr07b-local-test-bearer-never-use-in-production"),
		"status-summary.txt": fmt.Sprintf("status=%s\ndesired_revision=%d\napplied_revision=%d\nconfig_hash_sha256=%s\ninstance_identity_bound=%t\n",
			report.Status, report.DesiredRevision, report.AppliedRevision, report.ConfigHash, report.InstanceUID != ""),
		"run-summary.txt": "supervisor=v0.160.0\ncollector=otelcol-contrib-v0.160.0\ntransport=local-opamp-websocket\ncore=mockcore-over-https\nauth=supervisor-bearertokenauth-from-file\n",
	}
	for name, contents := range files {
		if err := os.WriteFile(filepath.Join(directory, name), []byte(contents), 0600); err != nil {
			t.Fatalf("write PR-07b evidence %s: %v", name, err)
		}
	}
}

func redactPR07BToken(value, token string) string {
	return strings.ReplaceAll(value, token, "[REDACTED_TEST_TOKEN]")
}
