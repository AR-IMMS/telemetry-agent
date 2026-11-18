package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
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
	enrollment "github.com/ar-imms/telemetry-agent/internal/identity/enrollment/application"
	identity "github.com/ar-imms/telemetry-agent/internal/identity/host"
	installationid "github.com/ar-imms/telemetry-agent/internal/identity/installation/domain"
	"github.com/ar-imms/telemetry-agent/internal/runtime/bootstrap"
	opampsupervisorconfig "github.com/ar-imms/telemetry-agent/internal/runtime/opamp"
	processowner "github.com/ar-imms/telemetry-agent/internal/runtime/process"
	"github.com/ar-imms/telemetry-agent/internal/testsupport/mockfleet"
	"github.com/ar-imms/telemetry-agent/internal/testsupport/sqlitefixture"
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

	const agentID = "10000000-0000-4000-8000-000000000001"
	const nodeID = "10000000-0000-4000-8000-000000000002"
	const inviteToken = "mockfleet-supervisor-e2e-invitation"
	fleet := mockfleet.New()
	if err := fleet.SeedInvitation(inviteToken, nodeID); err != nil {
		t.Fatal(err)
	}
	remoteConfig := []byte(fmt.Sprintf("exporters:\n  otlp:\n    endpoint: %q\n    tls:\n      insecure: true\n", sinkB))
	enrollmentHTTP := httptest.NewServer(fleet)
	t.Cleanup(enrollmentHTTP.Close)
	statePath := filepath.Join(root, "agent-state.db")
	parsedAgentID, err := installationid.ParseInstallationID(agentID)
	if err != nil {
		t.Fatalf("parse persisted Agent installation identity: %v", err)
	}
	if _, err := sqlitefixture.NewIdentityStore(statePath).EnsureInstallationID(parsedAgentID); err != nil {
		t.Fatalf("persist Agent installation identity: %v", err)
	}
	enrollmentRepository := sqlitefixture.NewEnrollmentStore(statePath)
	credentialStore := credentials.New(statePath)
	coreOrigin, err := enrollment.ParseCoreOrigin("https://fleet.test.invalid")
	if err != nil {
		t.Fatalf("parse provisional enrollment origin: %v", err)
	}
	enrollmentClient := &provisionalMockFleetEnrollmentClient{endpoint: enrollmentHTTP.URL}
	enrollmentService := enrollment.Service{
		Repository:  enrollmentRepository,
		Credentials: credentialStore,
		Client:      enrollmentClient,
	}
	enrollmentResult, err := enrollmentService.Start(ctx, coreOrigin, enrollment.EnrollmentPayload{
		Installation: enrollment.Installation{ID: agentID},
		Agent:        enrollment.Agent{Version: "test"},
		Environment:  enrollment.Environment{OS: runtime.GOOS, Architecture: runtime.GOARCH},
		Metadata:     map[string]any{"host_id": "host-evidence"},
	}, inviteToken)
	if err != nil {
		t.Fatalf("run Agent enrollment application against provisional fixture: %v", err)
	}
	if enrollmentResult.InstallationID != agentID || enrollmentResult.Binding.NodeID != nodeID || enrollmentResult.Binding.Status != "active" {
		t.Fatalf("Agent enrollment result = %+v, want installation %s bound to fixture node %s", enrollmentResult, agentID, nodeID)
	}
	enrollmentAttempt, err := enrollmentRepository.LoadAttempt()
	if err != nil {
		t.Fatalf("load durable enrollment result: %v", err)
	}
	if enrollmentAttempt.Status != enrollment.AttemptRegistered || enrollmentAttempt.Result == nil {
		t.Fatalf("persisted enrollment attempt = %+v, want registered result", enrollmentAttempt)
	}
	if enrollmentAttempt.Payload.Installation.ID != agentID {
		t.Fatalf("persisted enrollment identity = %q, want %q", enrollmentAttempt.Payload.Installation.ID, agentID)
	}
	if enrollmentClient.credentialID == "" {
		t.Fatal("Agent enrollment did not retain the fixture credential ID for revocation verification")
	}
	storedEnrollmentCredential, err := credentialStore.Get(enrollmentAttempt.AccessCredentialRef)
	if err != nil {
		t.Fatalf("read Agent-enrolled credential: %v", err)
	}
	if storedEnrollmentCredential != enrollmentClient.credential {
		t.Fatal("Agent credential store does not contain the fixture-issued enrollment credential")
	}
	if _, err := credentialStore.Get(enrollmentAttempt.BootstrapCredentialRef); !errors.Is(err, enrollment.ErrCredentialNotFound) {
		t.Fatalf("bootstrap credential lookup error = %v, want deleted token after registration", err)
	}
	databaseFiles, err := filepath.Glob(statePath + "*")
	if err != nil {
		t.Fatalf("find Agent state files: %v", err)
	}
	for _, databaseFile := range databaseFiles {
		content, err := os.ReadFile(databaseFile)
		if err != nil {
			t.Fatalf("read Agent state file %s: %v", databaseFile, err)
		}
		containsSecret := bytes.Contains(content, []byte(inviteToken)) || bytes.Contains(content, []byte(enrollmentClient.credential))
		clear(content)
		if containsSecret {
			t.Fatalf("Agent state file %s contains enrollment secret material", databaseFile)
		}
	}
	tokenPath, err := credentialStore.ProvisionSupervisorToken(enrollmentAttempt.AccessCredentialRef)
	if err != nil {
		t.Fatalf("provision Supervisor bearer from Agent credential store: %v", err)
	}
	provisionedToken, err := os.ReadFile(tokenPath)
	if err != nil {
		t.Fatalf("read provisioned Supervisor token file: %v", err)
	}
	if !bytes.Equal(provisionedToken, []byte(enrollmentClient.credential)) {
		clear(provisionedToken)
		t.Fatal("Supervisor token file does not contain the Agent-enrolled credential")
	}
	clear(provisionedToken)
	opampListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve fixture OpAMP endpoint: %v", err)
	}
	opampEndpoint := opampListener.Addr().String()
	if err := opampListener.Close(); err != nil {
		t.Fatalf("release reserved fixture OpAMP endpoint: %v", err)
	}

	supervisorConfig, err := opampsupervisorconfig.Render(opampsupervisorconfig.Config{
		ServerEndpoint: "ws://" + opampEndpoint + "/v1/opamp",
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
	supervisorContext, stopSupervisor := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() {
		done <- processowner.Run(supervisorContext, processowner.Options{
			Executable: supervisorPath,
			Arguments:  []string{"--feature-gates=opampsupervisor.Extensions", "--config=" + supervisorConfigPath},
		})
	}()
	t.Cleanup(func() {
		stopSupervisor()
		select {
		case err := <-done:
			if err != nil && !errors.Is(err, context.Canceled) {
				t.Errorf("stop contained Supervisor process tree: %v", err)
			}
		case <-time.After(10 * time.Second):
			t.Errorf("Supervisor and managed Collector did not stop")
		}
		if runtime.GOOS == "windows" {
			stateDirectory := filepath.Join(root, "supervisor-state")
			if err := waitPR09MockFleetStateDirectoryRelease(stateDirectory, 10*time.Second); err != nil {
				t.Errorf("Supervisor state directory remained locked after process-tree cleanup: %v", err)
			}
		}
	})

	// Keep the OpAMP endpoint unavailable until Supervisor has started its local
	// Collector config. The pinned Supervisor can panic if its first OpAMP
	// session starts before initial config state is ready, so this exercises its
	// supported offline-start and reconnect path before remote config delivery.
	waitPR07ASinkContains(t, ctx, done, sinkAOutput, "hostmetrics", "Supervisor initial local-config OTLP")
	opampServer, err := fleet.StartOpAMPServerAt(opampEndpoint)
	if err != nil {
		t.Fatalf("start authenticated fixture OpAMP server after local startup: %v", err)
	}
	t.Cleanup(func() {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer stopCancel()
		if err := opampServer.Stop(stopCtx); err != nil {
			t.Errorf("stop fixture OpAMP server: %v", err)
		}
	})
	if err := fleet.SetDesiredConfig(agentID, remoteConfig); err != nil {
		t.Fatalf("set fixture remote config: %v", err)
	}
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
	if err := fleet.RevokeCredential(enrollmentClient.credentialID); err != nil {
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
	reconnectRequest.Header.Set("Authorization", "Bearer "+enrollmentClient.credential)
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
			"test-summary.txt": fmt.Sprintf("environment=%s/%s\nsupervisor=v0.160.0\ncollector=v0.160.0\nagent_enrollment_application=PASS\ndurable_binding=PASS\ncredential_store_to_supervisor_token=PASS\nsqlite_secret_absence=PASS\ncredential=Agent credential store; raw token omitted\nremote_config_sha256=%x\ninvalid_config_sha256=%x\nremote_config_otlp=PASS\nautomatic_rollback=PASS\nrollback_otlp=PASS\nactive_session_revocation=PASS\nsupervisor_reconnect_after_revocation=REJECTED\nrevoked_reconnect=HTTP_401\nproduction_fleet_core_interop=NOT_VERIFIED\n", runtime.GOOS, runtime.GOARCH, goodHash, invalidHash),
			"supervisor.log":   readPR09MockFleetSupervisorLog(t, filepath.Join(root, "supervisor-state", "agent.log")),
			"sink-a.log":       tailPR07ATestOutput(sinkAOutput.String(), 60),
			"sink-b.log":       tailPR07ATestOutput(sinkBOutput.String(), 100),
		})
	}
	t.Logf("remote_config_sha256=%x invalid_config_sha256=%x sinks=verified production_fleet_interop=NOT_VERIFIED", goodHash, invalidHash)
}

// provisionalMockFleetEnrollmentClient adapts the current Core-shaped
// enrollment application port to test-only Mock Fleet exchange fields.
type provisionalMockFleetEnrollmentClient struct {
	endpoint     string
	credentialID string
	credential   string
}

func (c *provisionalMockFleetEnrollmentClient) Enroll(ctx context.Context, origin enrollment.CoreOrigin, request enrollment.EnrollmentRequest) (enrollment.EnrollmentResponse, error) {
	body, err := json.Marshal(struct {
		SchemaVersion int    `json:"schema_version"`
		AttemptID     string `json:"enrollment_attempt_id"`
		AgentID       string `json:"agent_id"`
		Token         string `json:"invitation_token"`
	}{SchemaVersion: 1, AttemptID: request.AttemptID, AgentID: request.Installation.ID, Token: request.BootstrapToken})
	if err != nil {
		return enrollment.EnrollmentResponse{}, fmt.Errorf("encode provisional Mock Fleet enrollment: %w", err)
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint+"/v1/enrollments/exchange", bytes.NewReader(body))
	if err != nil {
		return enrollment.EnrollmentResponse{}, fmt.Errorf("create provisional Mock Fleet enrollment request: %w", err)
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(httpRequest)
	if err != nil {
		return enrollment.EnrollmentResponse{}, fmt.Errorf("send provisional Mock Fleet enrollment: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		return enrollment.EnrollmentResponse{}, fmt.Errorf("provisional Mock Fleet enrollment returned HTTP %d", response.StatusCode)
	}
	var issued struct {
		AgentID      string `json:"agent_id"`
		NodeID       string `json:"node_id"`
		CredentialID string `json:"credential_id"`
		Credential   string `json:"credential"`
	}
	if err := json.NewDecoder(response.Body).Decode(&issued); err != nil {
		return enrollment.EnrollmentResponse{}, fmt.Errorf("decode provisional Mock Fleet enrollment response: %w", err)
	}
	if issued.AgentID != request.Installation.ID {
		return enrollment.EnrollmentResponse{}, fmt.Errorf("provisional Mock Fleet returned a mismatched Agent identity")
	}
	c.credentialID = issued.CredentialID
	c.credential = issued.Credential
	// These Core-shaped binding and control-plane values exercise the Agent's
	// existing enrollment persistence port only; they are not Fleet contracts.
	return enrollment.EnrollmentResponse{
		EnrollmentID:   request.AttemptID,
		InstallationID: request.Installation.ID,
		Binding:        enrollment.Binding{ID: "10000000-0000-4000-8000-000000000003", NodeID: issued.NodeID, Status: "active"},
		Credential:     enrollment.Credential{Type: "bearer", AccessToken: issued.Credential},
		ControlPlane:   enrollment.ControlPlane{BaseURL: origin.String(), PollIntervalSeconds: 30},
	}, nil
}

func readPR09MockFleetSupervisorLog(t *testing.T, path string) string {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		return fmt.Sprintf("Supervisor log unavailable: %v", err)
	}
	return tailPR07ATestOutput(string(contents), 160)
}

func waitPR09MockFleetStateDirectoryRelease(directory string, timeout time.Duration) error {
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	var lastErr error
	for {
		if lastErr = os.RemoveAll(directory); lastErr == nil {
			return nil
		}
		select {
		case <-deadline.C:
			return fmt.Errorf("remove %s after process cleanup: %w", directory, lastErr)
		case <-ticker.C:
		}
	}
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
