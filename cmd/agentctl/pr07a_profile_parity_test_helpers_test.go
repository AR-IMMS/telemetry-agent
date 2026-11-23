package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func runPR07ASupervisorApplicationProfileParity(t *testing.T, ctx context.Context, root, supervisorBinary, collectorBinary, profileConfigPath, gatewayEndpoint string, sink *lockedTemplateE2EBuffer, nodeID string, resources map[string]string, replacementCoolingResourceID string) {
	t.Helper()
	evidenceDir := strings.TrimSpace(os.Getenv("AR_IMMS_PR07A_EVIDENCE_DIR"))
	if evidenceDir != "" {
		evidenceDir = filepath.Join(evidenceDir, "application-profiles")
	}
	output := &lockedTemplateE2EBuffer{}
	if evidenceDir != "" {
		t.Cleanup(func() {
			writePR07AEvidence(t, evidenceDir, map[string]string{
				"supervisor.log": tailPR07ATestOutput(output.String(), 250),
				"otlp-sink.log":  tailPR07ATestOutput(sink.String(), 120),
			})
		})
	}
	supervisorEndpoint := reservePR07ATCPAddress(t)
	storageDir := filepath.Join(root, "pr07a-supervisor-state")
	supervisorConfigPath := filepath.Join(root, "pr07a-supervisor.yaml")
	supervisorConfig := fmt.Sprintf(`server:
  endpoint: ws://%s/v1/opamp
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
  collector_crash_log_snippet_kib: 4
storage:
  directory: %q
`, supervisorEndpoint, collectorBinary, profileConfigPath, profileConfigPath, storageDir)
	if err := os.WriteFile(supervisorConfigPath, []byte(supervisorConfig), 0600); err != nil {
		t.Fatal(err)
	}
	command := exec.CommandContext(ctx, supervisorBinary, "--config="+supervisorConfigPath)
	command.Env = append(os.Environ(), "OTEL_GATEWAY_ENDPOINT="+gatewayEndpoint)
	command.Stdout, command.Stderr = output, output
	if err := command.Start(); err != nil {
		t.Fatalf("start Supervisor for application profile parity: %v", err)
	}
	done := make(chan error, 1)
	exited := make(chan struct{})
	go func() {
		done <- command.Wait()
		close(exited)
	}()
	t.Cleanup(func() {
		stopPR07ASupervisorProcess(command.Process)
		select {
		case <-exited:
		case <-time.After(10 * time.Second):
			t.Errorf("application-profile Supervisor did not stop")
			return
		}
		waitPR07ASupervisorStorageReleased(t, storageDir)
	})

	supervisorResources := make(map[string]string, len(resources))
	for metric, resourceID := range resources {
		supervisorResources[metric] = resourceID
	}
	supervisorResources["ar_imms_test_cooling_celsius"] = replacementCoolingResourceID
	profileMetrics := make([]string, 0, len(supervisorResources))
	for metric := range supervisorResources {
		profileMetrics = append(profileMetrics, metric)
	}
	beforeFallback := profileMetricOccurrences(sink.String(), profileMetrics)
	waitPR07AProfileTelemetry(t, ctx, done, sink, nodeID, supervisorResources, beforeFallback, "fallback profile telemetry")

	opamp := startPR07AOpAMPServer(t, supervisorEndpoint, nil)
	defer opamp.Stop()
	select {
	case <-opamp.connected:
	case err := <-done:
		t.Fatalf("Supervisor exited before connecting to test OpAMP server: %v", err)
	case <-ctx.Done():
		t.Fatalf("context ended waiting for Supervisor OpAMP connection")
	case <-time.After(30 * time.Second):
		t.Fatalf("Supervisor did not reconnect to test OpAMP server: %s", tailPR07ATestOutput(output.String(), 50))
	}
	beforeReconnect := profileMetricOccurrences(sink.String(), profileMetrics)
	waitPR07AProfileTelemetry(t, ctx, done, sink, nodeID, supervisorResources, beforeReconnect, "profile telemetry after fallback recovery")
	t.Logf("PR07A_SUPERVISOR_PROFILE_PARITY platform=%s profiles=redis,postgresql,prometheus fallback=PASS reconnect=PASS otlp=PASS", runtime.GOOS)
	if evidenceDir != "" {
		writePR07AEvidence(t, evidenceDir, map[string]string{
			"profile-parity-summary.txt": fmt.Sprintf("platform=%s\nsupervisor=v0.160.0\ncollector=otelcol-contrib-v0.160.0\nprofiles=redis,postgresql,prometheus\nstartup_fallback=PASS\nserver_reconnect=PASS\npost_reconnect_profile_otlp_delivery=PASS\nnode_id=%s\n", runtime.GOOS, nodeID),
		})
	}
}

func waitPR07ASupervisorStorageReleased(t *testing.T, storageDir string) {
	t.Helper()
	logPath := filepath.Join(storageDir, "agent.log")
	renamedPath := logPath + ".release-check"
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(logPath); os.IsNotExist(err) {
			return
		}
		if err := os.Rename(logPath, renamedPath); err == nil {
			if err := os.Rename(renamedPath, logPath); err != nil {
				t.Errorf("restore Supervisor log after release check: %v", err)
			}
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Errorf("Supervisor did not release its storage log %s", logPath)
}

func profileMetricOccurrences(output string, metrics []string) map[string]int {
	counts := make(map[string]int, len(metrics))
	for _, metric := range metrics {
		counts[metric] = strings.Count(output, "Name: "+metric)
	}
	return counts
}

func waitPR07AProfileTelemetry(t *testing.T, ctx context.Context, done <-chan error, sink *lockedTemplateE2EBuffer, nodeID string, resources map[string]string, previousCounts map[string]int, label string) {
	t.Helper()
	deadline := time.NewTimer(90 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		output := sink.String()
		ready := true
		for metric, resourceID := range resources {
			if strings.Count(output, "Name: "+metric) <= previousCounts[metric] || !applicationSinkContainsIdentity(output, metric, nodeID, resourceID) {
				ready = false
				break
			}
		}
		if ready {
			return
		}
		select {
		case err := <-done:
			t.Fatalf("Supervisor exited waiting for %s: %v", label, err)
		case <-ctx.Done():
			t.Fatalf("context ended waiting for %s", label)
		case <-deadline.C:
			t.Fatalf("timed out waiting for %s; sink evidence=%s", label, tailPR07ATestOutput(output, 80))
		case <-ticker.C:
		}
	}
}
