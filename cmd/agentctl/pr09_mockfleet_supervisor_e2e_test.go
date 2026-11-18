package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/ar-imms/telemetry-agent/internal/identity/credentials"
	identity "github.com/ar-imms/telemetry-agent/internal/identity/host"
	"github.com/ar-imms/telemetry-agent/internal/runtime/bootstrap"
	opampsupervisorconfig "github.com/ar-imms/telemetry-agent/internal/runtime/opamp"
	"github.com/ar-imms/telemetry-agent/internal/testsupport/mockfleet"
	"github.com/open-telemetry/opamp-go/protobufs"
)

const (
	pr09MockFleetLiveE2EEnv  = "AR_IMMS_PR09_MOCKFLEET_LIVE_E2E"
	pr09MockFleetEvidenceEnv = "AR_IMMS_PR09_MOCKFLEET_EVIDENCE_DIR"
)

// TestPR09MockFleetSupervisorConfigAndRollbackE2E verifies authenticated
// standard OpAMP config delivery, real Supervisor rollback, and Collector OTLP
// delivery using only test infrastructure and pinned upstream binaries.
func TestPR09MockFleetSupervisorConfigAndRollbackE2E(t *testing.T) {
	if os.Getenv(pr09MockFleetLiveE2EEnv) != "1" {
		t.Skip("set " + pr09MockFleetLiveE2EEnv + "=1 to run pinned Mock Fleet/Supervisor/Collector E2E")
	}
	supervisorPath := strings.TrimSpace(os.Getenv("AR_IMMS_PR07A_SUPERVISOR_BINARY"))
	collectorPath := strings.TrimSpace(os.Getenv("AR_IMMS_PR05_COLLECTOR_BINARY"))
	if supervisorPath == "" || collectorPath == "" {
		t.Fatal("verified pinned Supervisor and Collector binary paths are required")
	}
	supervisorArtifact, err := bootstrap.SelectSupervisorArtifact(identity.PlatformInfo{OS: runtime.GOOS, Architecture: runtime.GOARCH})
	if err != nil {
		t.Fatal(err)
	}
	if err := verifyPR07AArtifact(supervisorPath, supervisorArtifact.SHA256); err != nil {
		t.Fatalf("verify pinned Supervisor artifact: %v", err)
	}
	versionCtx, versionCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer versionCancel()
	collectorVersion, err := exec.CommandContext(versionCtx, collectorPath, "--version").CombinedOutput()
	if err != nil || !strings.Contains(string(collectorVersion), "version 0.160.0") {
		t.Fatalf("Collector is not pinned v0.160.0: err=%v output=%s", err, collectorVersion)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	root := t.TempDir()
	collectorOutput := &lockedTemplateE2EBuffer{}
	sinkAOutput, sinkBOutput := &lockedTemplateE2EBuffer{}, &lockedTemplateE2EBuffer{}
	sinkADir, sinkBDir := filepath.Join(root, "sink-a"), filepath.Join(root, "sink-b")
	for _, dir := range []string{sinkADir, sinkBDir} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatalf("create isolated OTLP sink directory: %v", err)
		}
	}
	sinkA := startTemplateOTLPSink(t, ctx, collectorPath, sinkADir, sinkAOutput)
	sinkB := startTemplateOTLPSink(t, ctx, collectorPath, sinkBDir, sinkBOutput)
	localConfigPath := filepath.Join(root, "local.yaml")
	fallbackConfigPath := filepath.Join(root, "fallback.yaml")
	writePR07ACollectorConfig(t, localConfigPath, sinkA)
	writePR07ACollectorConfig(t, fallbackConfigPath, sinkA)

	const agentID = "mockfleet-supervisor-e2e-agent"
	const inviteToken = "mockfleet-supervisor-e2e-invitation"
	const attemptID = "mockfleet-supervisor-e2e-attempt"
	fleet := mockfleet.New()
	if err := fleet.SeedInvitation(inviteToken, "mockfleet-supervisor-e2e-node"); err != nil {
		t.Fatal(err)
	}
	remoteConfig := []byte(fmt.Sprintf("exporters:\n  otlp:\n    endpoint: %q\n    tls:\n      insecure: true\n", sinkB))
	if err := fleet.SetDesiredConfig(agentID, remoteConfig); err != nil {
		t.Fatalf("set fixture remote config: %v", err)
	}
	enrollmentHTTP := httptest.NewServer(fleet)
	t.Cleanup(enrollmentHTTP.Close)
	exchangeBody, err := json.Marshal(struct {
		SchemaVersion int    `json:"schema_version"`
		AttemptID     string `json:"enrollment_attempt_id"`
		AgentID       string `json:"agent_id"`
		Token         string `json:"invitation_token"`
	}{SchemaVersion: 1, AttemptID: attemptID, AgentID: agentID, Token: inviteToken})
	if err != nil {
		t.Fatal(err)
	}
	exchangeRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, enrollmentHTTP.URL+"/v1/enrollments/exchange", bytes.NewReader(exchangeBody))
	if err != nil {
		t.Fatal(err)
	}
	exchangeRequest.Header.Set("Content-Type", "application/json")
	exchangeResponse, err := http.DefaultClient.Do(exchangeRequest)
	if err != nil {
		t.Fatalf("exchange provisional enrollment invitation: %v", err)
	}
	var issued struct {
		CredentialID string `json:"credential_id"`
		Credential   string `json:"credential"`
	}
	if exchangeResponse.StatusCode != http.StatusCreated {
		_ = exchangeResponse.Body.Close()
		t.Fatalf("provisional enrollment status = %d; want %d", exchangeResponse.StatusCode, http.StatusCreated)
	}
	if err := json.NewDecoder(exchangeResponse.Body).Decode(&issued); err != nil {
		_ = exchangeResponse.Body.Close()
		t.Fatalf("decode provisional enrollment response: %v", err)
	}
	_ = exchangeResponse.Body.Close()

	statePath := filepath.Join(root, "agent-state.db")
	credentialStore := credentials.New(statePath)
	if err := credentialStore.Put("mockfleet-opamp", issued.Credential); err != nil {
		t.Fatalf("store fixture credential with Agent credential store: %v", err)
	}
	tokenPath, err := credentialStore.ProvisionSupervisorToken("mockfleet-opamp")
	if err != nil {
		t.Fatalf("provision Supervisor bearer from Agent credential store: %v", err)
	}
	opampServer, err := fleet.StartOpAMPServer()
	if err != nil {
		t.Fatalf("start authenticated fixture OpAMP server: %v", err)
	}
	t.Cleanup(func() {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer stopCancel()
		if err := opampServer.Stop(stopCtx); err != nil {
			t.Errorf("stop fixture OpAMP server: %v", err)
		}
	})

	supervisorConfig, err := opampsupervisorconfig.Render(opampsupervisorconfig.Config{
		ServerEndpoint: opampServer.Endpoint(),
		TokenFile:      tokenPath,
		CollectorPath:  collectorPath,
		LocalConfig:    localConfigPath,
		FallbackConfig: fallbackConfigPath,
		StateDirectory: filepath.Join(root, "supervisor-state"),
	})
	if err != nil {
		t.Fatalf("render Agent-owned Supervisor configuration: %v", err)
	}
	supervisorConfigPath := filepath.Join(root, "supervisor.yaml")
	if err := os.WriteFile(supervisorConfigPath, supervisorConfig, 0600); err != nil {
		t.Fatalf("write Supervisor test configuration: %v", err)
	}
	command := exec.CommandContext(ctx, supervisorPath, "--feature-gates=opampsupervisor.Extensions", "--config="+supervisorConfigPath)
	command.Stdout, command.Stderr = collectorOutput, collectorOutput
	if err := command.Start(); err != nil {
		t.Fatalf("start pinned Supervisor: %v", err)
	}
	done := make(chan error, 1)
	go func() { done <- command.Wait() }()
	t.Cleanup(func() {
		if command.Process != nil {
			stopPR07ASupervisorProcess(command.Process)
		}
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			t.Errorf("Supervisor and managed Collector did not stop")
		}
	})

	waitPR07ASinkContains(t, ctx, done, sinkBOutput, "hostmetrics", "Mock Fleet remote-config OTLP")
	goodHash := sha256.Sum256(remoteConfig)
	waitPR09MockFleetStatus(t, ctx, done, fleet, agentID, protobufs.RemoteConfigStatuses_RemoteConfigStatuses_APPLIED, goodHash[:])

	occupiedReceiver, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve failure receiver endpoint: %v", err)
	}
	defer occupiedReceiver.Close()
	invalidConfig := []byte(fmt.Sprintf("receivers:\n  otlp:\n    protocols:\n      grpc:\n        endpoint: %q\nservice:\n  pipelines:\n    metrics:\n      receivers: [otlp]\n", occupiedReceiver.Addr().String()))
	invalidHash := sha256.Sum256(invalidConfig)
	bytesBeforeFailure := len(sinkBOutput.String())
	if err := fleet.SetDesiredConfig(agentID, invalidConfig); err != nil {
		t.Fatalf("set invalid test config: %v", err)
	}
	waitPR09MockFleetStatus(t, ctx, done, fleet, agentID, protobufs.RemoteConfigStatuses_RemoteConfigStatuses_FAILED, invalidHash[:])
	waitPR09MockFleetStatus(t, ctx, done, fleet, agentID, protobufs.RemoteConfigStatuses_RemoteConfigStatuses_APPLIED, goodHash[:])
	waitPR07ASinkGrowth(t, ctx, done, sinkBOutput, bytesBeforeFailure, "OTLP after Supervisor automatic rollback")

	rejectedBeforeRevocation := opampServer.RejectedConnectionCount()
	if err := fleet.RevokeCredential(issued.CredentialID); err != nil {
		t.Fatalf("revoke fixture-issued Supervisor credential: %v", err)
	}
	waitPR09MockFleetSessionsClosed(t, ctx, fleet, agentID)
	waitPR09MockFleetRejectedReconnect(t, ctx, done, opampServer, rejectedBeforeRevocation)
	t.Log("supervisor_reconnect_after_revocation=REJECTED")
	reconnectURL := "http" + strings.TrimPrefix(opampServer.Endpoint(), "ws")
	reconnectRequest, err := http.NewRequestWithContext(ctx, http.MethodGet, reconnectURL, nil)
	if err != nil {
		t.Fatalf("create revoked OpAMP handshake request: %v", err)
	}
	reconnectRequest.Header.Set("Authorization", "Bearer "+issued.Credential)
	reconnectResponse, err := http.DefaultClient.Do(reconnectRequest)
	if err != nil {
		t.Fatalf("check reconnect with revoked Supervisor credential: %v", err)
	}
	_ = reconnectResponse.Body.Close()
	if reconnectResponse.StatusCode != http.StatusUnauthorized {
		t.Fatalf("revoked Supervisor credential response = %d; want %d", reconnectResponse.StatusCode, http.StatusUnauthorized)
	}

	if evidenceDir := strings.TrimSpace(os.Getenv(pr09MockFleetEvidenceEnv)); evidenceDir != "" {
		writePR07AEvidence(t, evidenceDir, map[string]string{
			"test-summary.txt": fmt.Sprintf("environment=%s/%s\nsupervisor=v0.160.0\ncollector=v0.160.0\ncredential=Agent credential store; raw token omitted\nremote_config_sha256=%x\ninvalid_config_sha256=%x\nremote_config_otlp=PASS\nautomatic_rollback=PASS\nrollback_otlp=PASS\nactive_session_revocation=PASS\nsupervisor_reconnect_after_revocation=REJECTED\nrevoked_reconnect=HTTP_401\nproduction_fleet_core_interop=NOT_VERIFIED\n", runtime.GOOS, runtime.GOARCH, goodHash, invalidHash),
			"supervisor.log":   tailPR07ATestOutput(collectorOutput.String(), 160),
			"sink-a.log":       tailPR07ATestOutput(sinkAOutput.String(), 60),
			"sink-b.log":       tailPR07ATestOutput(sinkBOutput.String(), 100),
		})
	}
	t.Logf("remote_config_sha256=%x invalid_config_sha256=%x sinks=verified production_fleet_interop=NOT_VERIFIED", goodHash, invalidHash)
}

func waitPR09MockFleetStatus(t *testing.T, ctx context.Context, done <-chan error, fleet *mockfleet.Server, agentID string, status protobufs.RemoteConfigStatuses, hash []byte) {
	t.Helper()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		for _, observed := range fleet.RemoteConfigStatuses(agentID) {
			if observed.GetStatus() == status && bytes.Equal(observed.GetLastRemoteConfigHash(), hash) {
				return
			}
		}
		select {
		case err := <-done:
			t.Fatalf("Supervisor exited waiting for Mock Fleet status %s/%x: %v", status, hash, err)
		case <-ctx.Done():
			t.Fatalf("timed out waiting for Mock Fleet status %s/%x: %+v", status, hash, fleet.RemoteConfigStatuses(agentID))
		case <-ticker.C:
		}
	}
}

func waitPR09MockFleetSessionsClosed(t *testing.T, ctx context.Context, fleet *mockfleet.Server, agentID string) {
	t.Helper()
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for fleet.ActiveOpAMPSessions(agentID) != 0 {
		select {
		case <-ctx.Done():
			t.Fatalf("fixture credential revocation left %d active OpAMP sessions", fleet.ActiveOpAMPSessions(agentID))
		case <-ticker.C:
		}
	}
}

func waitPR09MockFleetRejectedReconnect(t *testing.T, ctx context.Context, done <-chan error, server *mockfleet.OpAMPServer, rejectedBeforeRevocation int) {
	t.Helper()
	deadline := time.NewTicker(100 * time.Millisecond)
	defer deadline.Stop()
	for server.RejectedConnectionCount() <= rejectedBeforeRevocation {
		select {
		case err := <-done:
			t.Fatalf("Supervisor exited before retrying with its revoked credential: %v", err)
		case <-ctx.Done():
			t.Fatal("Supervisor did not attempt an OpAMP reconnect after credential revocation")
		case <-deadline.C:
		}
	}
}
