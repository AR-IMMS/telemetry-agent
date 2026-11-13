package agentlifecycle

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
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

	"github.com/ar-imms/telemetry-agent/internal/agentstate"
	"github.com/ar-imms/telemetry-agent/internal/bootstrap"
	"github.com/ar-imms/telemetry-agent/internal/coreapi"
	"github.com/ar-imms/telemetry-agent/internal/credentialstore"
	"github.com/ar-imms/telemetry-agent/internal/enrollment"
	"github.com/ar-imms/telemetry-agent/internal/identity"
	"github.com/ar-imms/telemetry-agent/internal/mockcore"
	"github.com/ar-imms/telemetry-agent/internal/opampserver"
)

// TestManagedSupervisorEnrollmentAndRemoteGatewayE2E is the managed-runtime
// vertical slice: actual enrollment persists a protected Agent credential,
// the Agent starts Supervisor, authenticated WSS delivers Core policy, and
// Collector exports host telemetry to the remotely selected Gateway.
func TestManagedSupervisorEnrollmentAndRemoteGatewayE2E(t *testing.T) {
	if os.Getenv("AR_IMMS_PR07C_CONTROL_E2E") != "1" {
		t.Skip("set AR_IMMS_PR07C_CONTROL_E2E=1 to run the native enrollment-to-WSS managed-runtime E2E")
	}
	if !canRunSupervisorRuntimeE2E() {
		t.Fatal("protected credential-store E2E requires an elevated Windows process")
	}
	supervisorPath := strings.TrimSpace(os.Getenv("AR_IMMS_PR07A_SUPERVISOR_BINARY"))
	collectorPath := strings.TrimSpace(os.Getenv("AR_IMMS_PR05_COLLECTOR_BINARY"))
	if supervisorPath == "" || collectorPath == "" {
		t.Fatal("verified pinned Supervisor and Collector binary paths are required")
	}
	platform := identity.PlatformInfo{OS: runtime.GOOS, Architecture: runtime.GOARCH}
	artifact, err := bootstrap.SelectSupervisorArtifact(platform)
	if err != nil {
		t.Fatal(err)
	}
	if got := fileSHA256(t, supervisorPath); got != artifact.SHA256 {
		t.Fatalf("Supervisor hash %s does not match pinned %s", got, artifact.SHA256)
	}
	version, err := exec.Command(collectorPath, "--version").CombinedOutput()
	if err != nil || !strings.Contains(string(version), "version 0.160.0") {
		t.Fatalf("Collector binary is not pinned v0.160.0: err=%v output=%s", err, version)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "initial-sink"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "updated-sink"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "rotated-sink"), 0700); err != nil {
		t.Fatal(err)
	}
	cert, certPEM := newManagedRuntimeTestCertificate(t)
	serverCAFile := filepath.Join(root, "opamp-server-ca.pem")
	if err := os.WriteFile(serverCAFile, certPEM, 0600); err != nil {
		t.Fatal(err)
	}
	initialSink, initialSinkURL := startSupervisorRuntimeSink(t, ctx, collectorPath, filepath.Join(root, "initial-sink"))
	updatedSink, updatedSinkURL := startSupervisorRuntimeSink(t, ctx, collectorPath, filepath.Join(root, "updated-sink"))
	rotatedSink, rotatedSinkURL := startSupervisorRuntimeSink(t, ctx, collectorPath, filepath.Join(root, "rotated-sink"))
	initialEndpoint := strings.TrimPrefix(initialSinkURL, "http://")
	updatedEndpoint := strings.TrimPrefix(updatedSinkURL, "http://")
	rotatedEndpoint := strings.TrimPrefix(rotatedSinkURL, "http://")

	const (
		agentToken            = "pr07c-e2e-agent-token-never-use-in-production"
		replacementAgentToken = "pr07c-e2e-replacement-agent-token-never-use-in-production"
		bootstrapToken        = "pr07c-e2e-bootstrap-token-never-use-in-production"
		replacementBootstrap  = "pr07c-e2e-replacement-bootstrap-token-never-use-in-production"
	)
	const nodeID = "33333333-4444-4555-8666-777777777777"
	statePath := filepath.Join(root, "agent-state.db")
	installationID, err := identity.EnsureInstallationIdentity(agentstate.NewIdentityStore(statePath), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	core, err := mockcore.New(mockcore.ProvisionedInstallation{
		AccessToken: agentToken, InstallationID: installationID.String(),
		NodeID: nodeID, Policy: coreapi.DesiredGatewayPolicy{
			SchemaVersion: "ar-imms/gateway-policy/v1", Revision: 1, GatewayEndpoint: initialEndpoint,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	coreHandler := managedRuntimeEnrollmentHandler(t, core, agentToken, replacementAgentToken, bootstrapToken, replacementBootstrap, nodeID)
	coreHTTP := httptest.NewUnstartedServer(coreHandler)
	coreHTTP.TLS = &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12}
	coreHTTP.StartTLS()
	defer coreHTTP.Close()
	trustRoots := x509.NewCertPool()
	parsedCert, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	trustRoots.AddCert(parsedCert)
	previousTransport := http.DefaultTransport
	http.DefaultTransport = &http.Transport{TLSClientConfig: &tls.Config{RootCAs: trustRoots, MinVersion: tls.VersionTLS12}}
	t.Cleanup(func() {
		http.DefaultTransport.(*http.Transport).CloseIdleConnections()
		http.DefaultTransport = previousTransport
	})
	coreClient := func(token string) (coreapi.AgentManagementAPI, error) {
		return coreapi.NewAgentManagementClient(coreHTTP.URL, token, coreHTTP.Client())
	}
	opamp, err := opampserver.New(opampserver.Options{
		ListenEndpoint: "127.0.0.1:0", ListenPath: "/v1/opamp",
		TLSConfig:  &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12},
		CoreClient: coreClient,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := opamp.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer stopCancel()
		if err := opamp.Stop(stopCtx); err != nil {
			t.Errorf("stop OpAMP server: %v", err)
		}
	}()

	origin, err := enrollment.ParseCoreOrigin(coreHTTP.URL)
	if err != nil {
		t.Fatal(err)
	}
	service := enrollment.Service{
		Repository:  agentstate.NewEnrollmentStore(statePath),
		Credentials: credentialstore.New(statePath), Client: coreapi.NewEnrollmentClient(),
	}
	result, err := service.Start(ctx, origin, enrollment.EnrollmentPayload{
		Installation: enrollment.Installation{ID: installationID.String()},
		Agent:        enrollment.Agent{Version: "pr07c-e2e"},
		Environment:  enrollment.Environment{OS: runtime.GOOS, Architecture: runtime.GOARCH},
	}, bootstrapToken)
	if err != nil {
		t.Fatalf("actual Agent enrollment against Mock Core: %v", err)
	}
	if result.InstallationID != installationID.String() || result.Binding.NodeID != nodeID {
		t.Fatalf("enrollment identity result = %+v", result)
	}
	attempt, err := agentstate.NewEnrollmentStore(statePath).LoadAttempt()
	if err != nil {
		t.Fatal(err)
	}
	if attempt.Status != enrollment.AttemptRegistered {
		t.Fatalf("enrollment status = %s, want registered", attempt.Status)
	}
	storedToken, err := credentialstore.New(statePath).Get(attempt.AccessCredentialRef)
	if err != nil || storedToken != agentToken {
		t.Fatalf("resolve protected issued Agent credential: err=%v matches=%t", err, storedToken == agentToken)
	}
	encodedAttempt, err := json.Marshal(attempt)
	if err != nil || strings.Contains(string(encodedAttempt), agentToken) || strings.Contains(string(encodedAttempt), "pr07c-e2e-bootstrap-token") {
		t.Fatalf("SQLite enrollment state contains credential material: err=%v", err)
	}

	collectorConfig := filepath.Join(root, "collector.yaml")
	healthURL := reserveSupervisorRuntimeEndpoint(t)
	collectorYAML := fmt.Sprintf(`receivers:
  hostmetrics:
    collection_interval: 1s
    scrapers:
      cpu: {}
      memory: {}
exporters:
  otlp/gateway:
    endpoint: %q
    tls:
      insecure: true
extensions:
  health_check:
    endpoint: %q
service:
  telemetry:
    metrics:
      level: none
  extensions: [health_check]
  pipelines:
    metrics:
      receivers: [hostmetrics]
      exporters: [otlp/gateway]`, initialEndpoint, strings.TrimPrefix(healthURL, "http://"))
	if err := os.WriteFile(collectorConfig, []byte(collectorYAML), 0600); err != nil {
		t.Fatal(err)
	}
	healthEndpoint := healthURL
	collectorValidation := exec.CommandContext(ctx, collectorPath, "validate", "--config", collectorConfig)
	if output, err := collectorValidation.CombinedOutput(); err != nil {
		t.Fatalf("validate Collector config: %v\n%s\nconfig:\n%s", err, output, collectorYAML)
	}
	endpoint := "wss://" + opamp.Addr().String() + "/v1/opamp"
	managed := agentstate.SupervisorContext{
		BinaryPath:        supervisorPath,
		ConfigurationPath: filepath.Join(root, "credentials", "opamp-supervisor.yaml"),
		StateDirectory:    filepath.Join(root, "credentials", "opamp-supervisor-state"),
		OpAMPServerURL:    endpoint, ServerCAFile: serverCAFile, Enabled: true,
	}
	if _, err := agentstate.NewStore(statePath).Update(func(state *agentstate.State) error {
		state.Collector = agentstate.CollectorContext{
			BinaryPath: collectorPath, ConfigPath: collectorConfig,
			GatewayEndpoint: initialEndpoint, HealthEndpoint: healthEndpoint,
		}
		state.Supervisor = managed
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	runContext, stopRuntime := context.WithCancel(ctx)
	runtimeDone := make(chan error, 1)
	runtimeStopped := false
	t.Cleanup(func() {
		stopRuntime()
		if runtimeStopped {
			return
		}
		select {
		case err := <-runtimeDone:
			if err != nil {
				t.Errorf("Agent-managed runtime cleanup: %v", err)
			}
		case <-time.After(20 * time.Second):
			t.Errorf("Agent-managed runtime process tree did not stop during cleanup")
		}
	})
	noOpTeardown := func(context.Context, string, agentstate.TeardownAction, []agentstate.OwnedResource) error { return nil }
	go func() {
		runtimeDone <- RunCollectorRuntimeWithReconciliation(runContext, statePath, noOpTeardown, nil, nil, nil)
	}()
	waitForSupervisorRuntimeReadiness(t, ctx, runtimeDone, healthEndpoint)
	waitForSupervisorCorePolicy(t, ctx, runtimeDone, core, 1)
	waitForSupervisorSinkTelemetry(t, ctx, runtimeDone, initialSink)
	if err := core.SetPolicy(installationID.String(), coreapi.DesiredGatewayPolicy{
		SchemaVersion: "ar-imms/gateway-policy/v1", Revision: 2, GatewayEndpoint: updatedEndpoint,
	}); err != nil {
		t.Fatal(err)
	}
	waitForSupervisorCorePolicy(t, ctx, runtimeDone, core, 2)
	waitForSupervisorSinkTelemetry(t, ctx, runtimeDone, updatedSink)
	priorCredentialRef := attempt.AccessCredentialRef
	if _, err := service.ReEnroll(ctx, origin, enrollment.EnrollmentPayload{
		Installation: enrollment.Installation{ID: installationID.String()},
		Agent:        enrollment.Agent{Version: "pr07c-e2e"},
		Environment:  enrollment.Environment{OS: runtime.GOOS, Architecture: runtime.GOARCH},
	}, replacementBootstrap); err != nil {
		t.Fatalf("replace Agent credential through actual re-enrollment: %v", err)
	}
	replacementAttempt, err := agentstate.NewEnrollmentStore(statePath).LoadAttempt()
	if err != nil {
		t.Fatal(err)
	}
	if replacementAttempt.AccessCredentialRef == priorCredentialRef {
		t.Fatal("re-enrollment did not replace the protected credential reference")
	}
	if storedReplacement, err := credentialstore.New(statePath).Get(replacementAttempt.AccessCredentialRef); err != nil || storedReplacement != replacementAgentToken {
		t.Fatalf("resolve protected replacement Agent credential: err=%v matches=%t", err, storedReplacement == replacementAgentToken)
	}
	if _, err := credentialstore.New(statePath).Get(priorCredentialRef); !errors.Is(err, enrollment.ErrCredentialNotFound) {
		t.Fatalf("prior protected Agent credential remains available after re-enrollment: %v", err)
	}
	oldCredentialClient, err := coreapi.NewAgentManagementClient(coreHTTP.URL, agentToken, coreHTTP.Client())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := oldCredentialClient.GetSession(ctx); err == nil {
		t.Fatal("Mock Core still accepts the replaced Agent credential")
	} else {
		var apiErr *coreapi.ControlAPIError
		if !errors.As(err, &apiErr) || apiErr.Status != http.StatusUnauthorized {
			t.Fatalf("old Agent credential response = %v, want HTTP 401", err)
		}
	}
	replacementCredentialClient, err := coreapi.NewAgentManagementClient(coreHTTP.URL, replacementAgentToken, coreHTTP.Client())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := replacementCredentialClient.GetSession(ctx); err != nil {
		t.Fatalf("Mock Core rejected replacement Agent credential: %v", err)
	}
	if err := core.SetPolicy(installationID.String(), coreapi.DesiredGatewayPolicy{
		SchemaVersion: "ar-imms/gateway-policy/v1", Revision: 3, GatewayEndpoint: rotatedEndpoint,
	}); err != nil {
		t.Fatal(err)
	}
	waitForSupervisorCorePolicy(t, ctx, runtimeDone, core, 3)
	waitForSupervisorSinkTelemetry(t, ctx, runtimeDone, rotatedSink)
	stopRuntime()
	select {
	case err := <-runtimeDone:
		runtimeStopped = true
		if err != nil {
			t.Fatalf("Agent-managed Supervisor shutdown: %v", err)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("Agent-managed Supervisor process tree did not stop")
	}
	if initialSink.String() == "" || updatedSink.String() == "" || rotatedSink.String() == "" {
		t.Fatal("initial, updated, and post-rotation Gateway sinks must receive OTLP")
	}
	writeManagedControlE2EEvidence(t, result.InstallationID, result.Binding.NodeID)
	t.Logf("verified enrollment identity=%s Node=%s; policy revisions=1,2,3; replacement credential revoked the prior bearer and reconnected through authenticated WSS; initial and updated OTLP delivery", result.InstallationID, result.Binding.NodeID)
}

func managedRuntimeEnrollmentHandler(t *testing.T, core *mockcore.Server, accessToken, replacementToken, bootstrapToken, replacementBootstrap, nodeID string) http.Handler {
	t.Helper()
	enrollmentCount := 0
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/v1/agent-enrollments" || request.Method != http.MethodPost {
			core.ServeHTTP(writer, request)
			return
		}
		var input enrollment.EnrollmentRequest
		if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
			http.Error(writer, "invalid enrollment request", http.StatusUnauthorized)
			return
		}
		responseToken := accessToken
		switch enrollmentCount {
		case 0:
			if input.BootstrapToken != bootstrapToken {
				http.Error(writer, "invalid enrollment request", http.StatusUnauthorized)
				return
			}
		case 1:
			if input.BootstrapToken != replacementBootstrap {
				http.Error(writer, "invalid replacement enrollment request", http.StatusUnauthorized)
				return
			}
			responseToken = replacementToken
		default:
			http.Error(writer, "unexpected enrollment request", http.StatusConflict)
			return
		}
		if err := input.EnrollmentPayload.Validate(); err != nil {
			http.Error(writer, "invalid enrollment payload", http.StatusBadRequest)
			return
		}
		if enrollmentCount == 1 {
			if err := core.ReplaceCredential(accessToken, replacementToken); err != nil {
				http.Error(writer, "could not replace Mock Core credential", http.StatusInternalServerError)
				return
			}
		}
		response := enrollment.EnrollmentResponse{
			EnrollmentID:   "aaaaaaaa-1111-4222-8333-bbbbbbbbbbbb",
			InstallationID: input.Installation.ID,
			Binding:        enrollment.Binding{ID: "cccccccc-1111-4222-8333-dddddddddddd", NodeID: nodeID, Status: "active"},
			Credential:     enrollment.Credential{Type: "bearer", AccessToken: responseToken},
			ControlPlane:   enrollment.ControlPlane{BaseURL: "https://" + request.Host, PollIntervalSeconds: 30},
		}
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(writer).Encode(map[string]any{"data": response, "meta": map[string]string{"request_id": "pr07c-e2e-request"}})
		enrollmentCount++
	})
}

func newManagedRuntimeTestCertificate(t *testing.T) (tls.Certificate, []byte) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 120))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: "PR-07c local E2E"},
		NotBefore:    now.Add(-time.Minute), NotAfter: now.Add(2 * time.Hour),
		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment | x509.KeyUsageCertSign,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}, DNSNames: []string{"localhost"},
		BasicConstraintsValid: true, IsCA: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	privateKey, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	certificatePEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privateKey})
	certificate, err := tls.X509KeyPair(certificatePEM, keyPEM)
	if err != nil {
		t.Fatal(err)
	}
	return certificate, certificatePEM
}

func writeManagedControlE2EEvidence(t *testing.T, installationID, nodeID string) {
	t.Helper()
	directory := strings.TrimSpace(os.Getenv("AR_IMMS_PR07C_EVIDENCE_DIR"))
	if directory == "" {
		return
	}
	if err := os.MkdirAll(directory, 0700); err != nil {
		t.Fatalf("create managed control-plane evidence directory: %v", err)
	}
	contents := fmt.Sprintf("result=PASS\nos=%s\narchitecture=%s\ncommit=%s\nenrollment=actual-agent-service\ncredential=protected-store-and-secret-free-sqlite\ncredential_replacement=previous_revoked_new_wss_session_verified\ntransport=authenticated-wss\npolicy_revisions=1,2,3\ninitial_gateway_otlp=verified\nupdated_gateway_otlp=verified\npost_rotation_gateway_otlp=verified\ninstallation_identity=%s\nnode_identity=%s\n", runtime.GOOS, runtime.GOARCH, os.Getenv("GITHUB_SHA"), installationID, nodeID)
	if err := os.WriteFile(filepath.Join(directory, "managed-enrollment-opamp-e2e.txt"), []byte(contents), 0600); err != nil {
		t.Fatalf("write sanitized managed control-plane evidence: %v", err)
	}
}

func waitForSupervisorCorePolicy(t *testing.T, ctx context.Context, done <-chan error, core *mockcore.Server, revision int64) {
	t.Helper()
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		for _, status := range core.Statuses() {
			if status.Report.Status == "FAILED" && status.Report.DesiredRevision == revision {
				t.Fatalf("Supervisor failed desired policy revision %d: %+v", revision, status.Report)
			}
			if status.Report.Status == "APPLIED" && status.Report.AppliedRevision == revision {
				return
			}
		}
		select {
		case err := <-done:
			t.Fatalf("Agent-managed Supervisor exited before policy revision %d: %v", revision, err)
		case <-ctx.Done():
			t.Fatalf("timed out waiting for applied policy revision %d", revision)
		case <-ticker.C:
		}
	}
}
