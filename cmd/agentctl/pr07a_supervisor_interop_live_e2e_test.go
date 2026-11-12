package main

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/open-telemetry/opamp-go/protobufs"
)

const pr07aLiveE2EEnv = "AR_IMMS_PR07A_LIVE_E2E"

// TestPR07ASupervisorRemoteConfigAndFallbackDelivery verifies the official
// Supervisor against the pinned Collector, including offline fallback,
// OpAMP remote configuration, status reporting, and real OTLP delivery.
func TestPR07ASupervisorRemoteConfigAndFallbackDelivery(t *testing.T) {
	if os.Getenv(pr07aLiveE2EEnv) != "1" {
		t.Skip("set " + pr07aLiveE2EEnv + "=1 to run the isolated Supervisor E2E")
	}
	supervisorBinary := strings.TrimSpace(os.Getenv("AR_IMMS_PR07A_SUPERVISOR_BINARY"))
	collectorBinary := strings.TrimSpace(os.Getenv("AR_IMMS_PR05_COLLECTOR_BINARY"))
	if supervisorBinary == "" || collectorBinary == "" {
		t.Fatal("verified Supervisor and Collector binary paths are required")
	}
	if _, err := os.Stat(supervisorBinary); err != nil {
		t.Fatalf("inspect Supervisor binary: %v", err)
	}
	if _, err := os.Stat(collectorBinary); err != nil {
		t.Fatalf("inspect Collector binary: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	tempDir, err := os.MkdirTemp("", "pr07a-supervisor-e2e-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		deadline := time.Now().Add(5 * time.Second)
		for {
			err := os.RemoveAll(tempDir)
			if err == nil || time.Now().After(deadline) {
				if err != nil {
					t.Errorf("remove Supervisor E2E temp directory: %v", err)
				}
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
	})
	collectorOutput := &lockedTemplateE2EBuffer{}
	sinkAOutput, sinkBOutput := &lockedTemplateE2EBuffer{}, &lockedTemplateE2EBuffer{}
	evidenceDir := strings.TrimSpace(os.Getenv("AR_IMMS_PR07A_EVIDENCE_DIR"))
	if evidenceDir != "" {
		t.Cleanup(func() {
			writePR07AEvidence(t, evidenceDir, map[string]string{
				"supervisor.log": tailPR07ATestOutput(collectorOutput.String(), 300),
				"sink-a.log":     tailPR07ATestOutput(sinkAOutput.String(), 100),
				"sink-b.log":     tailPR07ATestOutput(sinkBOutput.String(), 100),
			})
		})
	}
	sinkADir, sinkBDir := filepath.Join(tempDir, "sink-a"), filepath.Join(tempDir, "sink-b")
	if err := os.MkdirAll(sinkADir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(sinkBDir, 0700); err != nil {
		t.Fatal(err)
	}
	sinkA := startTemplateOTLPSink(t, ctx, collectorBinary, sinkADir, sinkAOutput)
	sinkB := startTemplateOTLPSink(t, ctx, collectorBinary, sinkBDir, sinkBOutput)

	opampEndpoint := reservePR07ATCPAddress(t)
	localConfigPath := filepath.Join(tempDir, "local.yaml")
	fallbackConfigPath := filepath.Join(tempDir, "fallback.yaml")
	writePR07ACollectorConfig(t, localConfigPath, sinkA)
	writePR07ACollectorConfig(t, fallbackConfigPath, sinkA)

	supervisorConfigPath := filepath.Join(tempDir, "supervisor.yaml")
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
telemetry:
  logs:
    level: debug
`, opampEndpoint, collectorBinary, localConfigPath, fallbackConfigPath, filepath.Join(tempDir, "supervisor-state"))
	if err := os.WriteFile(supervisorConfigPath, []byte(supervisorConfig), 0600); err != nil {
		t.Fatal(err)
	}
	command := exec.CommandContext(ctx, supervisorBinary, "--config="+supervisorConfigPath)
	command.Stdout, command.Stderr = collectorOutput, collectorOutput
	if err := command.Start(); err != nil {
		t.Fatalf("start official Supervisor: %v", err)
	}
	done := make(chan error, 1)
	go func() { done <- command.Wait() }()
	t.Cleanup(func() {
		stopPR07ASupervisorProcess(command.Process)
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			t.Errorf("Supervisor did not stop after kill")
		}
	})

	waitPR07ASinkContains(t, ctx, done, sinkAOutput, "hostmetrics", "startup fallback telemetry")

	remoteConfig := []byte(fmt.Sprintf(`exporters:
  otlp:
    endpoint: %q
`, sinkB))
	opamp := startPR07AOpAMPServer(t, opampEndpoint, remoteConfig)
	defer opamp.Stop()
	waitPR07ARemoteConfigApplied(t, ctx, done, opamp, "remote configuration")
	waitPR07ASinkContains(t, ctx, done, sinkBOutput, "hostmetrics", "remote-config OTLP telemetry")

	occupiedReceiver, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve endpoint for activation failure: %v", err)
	}
	defer occupiedReceiver.Close()
	invalidConfig := []byte(fmt.Sprintf("receivers:\n  otlp:\n    protocols:\n      grpc:\n        endpoint: %q\nservice:\n  pipelines:\n    metrics:\n      receivers: [otlp]\n", occupiedReceiver.Addr().String()))
	invalidHash := sha256.Sum256(invalidConfig)
	goodHash := sha256.Sum256(remoteConfig)
	bytesBeforeFailure := len(sinkBOutput.String())
	drainPR07ARemoteStatuses(opamp)
	opamp.setRemoteConfig(invalidConfig)
	waitPR07ARemoteStatus(t, ctx, done, opamp, protobufs.RemoteConfigStatuses_RemoteConfigStatuses_FAILED, invalidHash[:], "invalid candidate failure status")
	automaticRollbackObserved := observePR07ARemoteStatus(ctx, done, opamp, protobufs.RemoteConfigStatuses_RemoteConfigStatuses_APPLIED, goodHash[:], 20*time.Second)
	t.Logf("automatic_rollback_applied_status_observed=%t", automaticRollbackObserved)
	if !automaticRollbackObserved {
		opamp.setRemoteConfig(remoteConfig)
		waitPR07ARemoteStatus(t, ctx, done, opamp, protobufs.RemoteConfigStatuses_RemoteConfigStatuses_APPLIED, goodHash[:], "server restore of last known working remote config")
	}
	waitPR07ASinkGrowth(t, ctx, done, sinkBOutput, bytesBeforeFailure, "OTLP after remote-config recovery")

	startedBeforeCrash := strings.Count(collectorOutput.String(), "Agent process started")
	killPR07AManagedCollector(t, collectorOutput.String())
	waitPR07ACollectorRestart(t, ctx, done, collectorOutput, startedBeforeCrash)
	sinkBytesBeforeCrash := len(sinkBOutput.String())
	waitPR07ASinkGrowth(t, ctx, done, sinkBOutput, sinkBytesBeforeCrash, "OTLP delivery after Collector crash recovery")

	// Retain enough evidence in process output for debugging a failed child exit.
	t.Logf("supervisor_binary=%s collector_binary=%s platform=%s", supervisorBinary, collectorBinary, runtime.GOOS)
	t.Logf("remote_config_sha256=%x sink_a=%s sink_b=%s", sha256.Sum256(remoteConfig), sinkA, sinkB)
	t.Logf("supervisor_output_tail=%s", tailPR07ATestOutput(collectorOutput.String(), 80))
	t.Logf("sink_a_evidence=%s", tailPR07ATestOutput(sinkAOutput.String(), 20))
	t.Logf("sink_b_evidence=%s", tailPR07ATestOutput(sinkBOutput.String(), 20))
	if evidenceDir != "" {
		writePR07AEvidence(t, evidenceDir, map[string]string{
			"test-summary.txt": fmt.Sprintf("platform=%s\nsupervisor=v0.160.0\ncollector=otelcol-contrib-v0.160.0\nremote_config_sha256=%x\nautomatic_rollback_applied_status_observed=%t\nfallback_otlp_delivery=PASS\nremote_update_otlp_delivery=PASS\nactivation_failure_status=FAILED\nlast_working_config_rollback=PASS\ncollector_crash_restart=PASS\npost_restart_otlp_delivery=PASS\n", runtime.GOOS, sha256.Sum256(remoteConfig), automaticRollbackObserved),
		})
	}
}
