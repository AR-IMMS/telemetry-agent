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
	"strconv"
	"strings"
	"sync/atomic"
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
	initialSinkDirectory := t.TempDir()
	updatedSinkDirectory := t.TempDir()
	rotatedSinkDirectory := t.TempDir()
	restartedSinkDirectory := t.TempDir()
	cert, certPEM := newManagedRuntimeTestCertificate(t)
	serverCAFile := filepath.Join(root, "opamp-server-ca.pem")
	if err := os.WriteFile(serverCAFile, certPEM, 0600); err != nil {
		t.Fatal(err)
	}
	initialSink, initialSinkURL := startSupervisorRuntimeSink(t, ctx, collectorPath, initialSinkDirectory)
	updatedSink, updatedSinkURL := startSupervisorRuntimeSink(t, ctx, collectorPath, updatedSinkDirectory)
	rotatedSink, rotatedSinkURL := startSupervisorRuntimeSink(t, ctx, collectorPath, rotatedSinkDirectory)
	restartedSink, restartedSinkURL := startSupervisorRuntimeSink(t, ctx, collectorPath, restartedSinkDirectory)
	initialEndpoint := strings.TrimPrefix(initialSinkURL, "http://")
	updatedEndpoint := strings.TrimPrefix(updatedSinkURL, "http://")
	rotatedEndpoint := strings.TrimPrefix(rotatedSinkURL, "http://")
	restartedEndpoint := strings.TrimPrefix(restartedSinkURL, "http://")

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
	coreStatePath := filepath.Join(root, "mock-core-state.json")
	seed := mockcore.ProvisionedInstallation{
		AccessToken: agentToken, InstallationID: installationID.String(),
		NodeID: nodeID, Policy: coreapi.DesiredGatewayPolicy{
			SchemaVersion: "ar-imms/gateway-policy/v1", Revision: 1, GatewayEndpoint: initialEndpoint,
		},
	}
	core, err := mockcore.NewPersistent(seed, coreStatePath)
	if err != nil {
		t.Fatal(err)
	}
	corePointer := &atomic.Pointer[mockcore.Server]{}
	corePointer.Store(core)
	coreHandler := managedRuntimeEnrollmentHandler(t, corePointer, agentToken, replacementAgentToken, bootstrapToken, replacementBootstrap, nodeID)
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
	opampReservation, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	opampListenEndpoint := opampReservation.Addr().String()
	if err := opampReservation.Close(); err != nil {
		t.Fatal(err)
	}
	startOpAMP := func() (*opampserver.Server, error) {
		server, err := opampserver.New(opampserver.Options{
			ListenEndpoint: opampListenEndpoint, ListenPath: "/v1/opamp",
			TLSConfig:  &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12},
			CoreClient: coreClient,
		})
		if err != nil {
			return nil, err
		}
		if err := server.Start(); err != nil {
			return nil, err
		}
		return server, nil
	}
	stopOpAMP := func(server *opampserver.Server) error {
		if server == nil {
			return nil
		}
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer stopCancel()
		return server.Stop(stopCtx)
	}
	var opamp *opampserver.Server
	defer func() {
		if err := stopOpAMP(opamp); err != nil {
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
	endpoint := "wss://" + opampListenEndpoint + "/v1/opamp"
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
	waitForSupervisorSinkTelemetry(t, ctx, runtimeDone, initialSink)
	opamp, err = startOpAMP()
	if err != nil {
		t.Fatalf("start OpAMP after offline Agent-managed fallback: %v", err)
	}
	waitForSupervisorCorePolicy(t, ctx, runtimeDone, core, 1)
	waitForSupervisorSinkTelemetry(t, ctx, runtimeDone, initialSink)
	localBaseline := strings.ReplaceAll(collectorYAML, initialEndpoint, "${env:OTEL_GATEWAY_ENDPOINT}")
	localBaseline = strings.Replace(localBaseline, "collection_interval: 1s", "collection_interval: 2s", 1)
	if localBaseline == strings.ReplaceAll(collectorYAML, initialEndpoint, "${env:OTEL_GATEWAY_ENDPOINT}") {
		t.Fatal("local configuration fixture did not change the hostmetrics interval")
	}
	localCandidate, localOptions := managedRuntimeCandidate(t, statePath, collectorPath, collectorConfig, managedCandidateBaselinePath(root), []byte(localBaseline))
	managedState := agentstate.NewSQLiteStore(statePath)
	localPriorPIDs := waitForManagedProcessCount(t, ctx, runtimeDone, collectorPath, root, 1)
	localSinkBytes := len(initialSink.String())
	localOutcome, err := ApplyCollectorCandidate(ctx, managedState, localCandidate, localOptions)
	if err != nil || localOutcome.Result != "activated" || localOutcome.AlreadyApplied {
		t.Fatalf("apply changed local configuration under managed Supervisor: outcome=%+v error=%v", localOutcome, err)
	}
	waitForManagedCandidateApplied(t, ctx, runtimeDone, managedState, localOutcome.Generation)
	localReplacementPIDs := waitForManagedProcessReplacement(t, ctx, runtimeDone, collectorPath, root, localPriorPIDs[0])
	if len(localReplacementPIDs) != 1 {
		t.Fatalf("local configuration update left multiple Collector owners: %v", localReplacementPIDs)
	}
	waitForSupervisorSinkGrowth(t, ctx, runtimeDone, initialSink, localSinkBytes)
	updatedLocalConfig, err := os.ReadFile(collectorConfig)
	if err != nil || !strings.Contains(string(updatedLocalConfig), "collection_interval: 2s") {
		t.Fatalf("Agent did not activate changed local configuration: error=%v config=%s", err, updatedLocalConfig)
	}
	noOpState, err := managedState.Load()
	if err != nil {
		t.Fatal(err)
	}
	noOpConfigBefore, err := os.ReadFile(collectorConfig)
	if err != nil {
		t.Fatal(err)
	}
	noOpPIDsBefore := waitForManagedProcessCount(t, ctx, runtimeDone, collectorPath, root, 1)
	noOpSinkBytes := len(initialSink.String())
	unchangedCandidate, unchangedOptions := managedRuntimeCandidate(t, statePath, collectorPath, collectorConfig, managedCandidateBaselinePath(root), []byte(localBaseline))
	noOpOutcome, err := ApplyCollectorCandidate(ctx, managedState, unchangedCandidate, unchangedOptions)
	if err != nil || noOpOutcome.Result != "applied" || !noOpOutcome.AlreadyApplied {
		t.Fatalf("unchanged local configuration was not an idempotent no-op: outcome=%+v error=%v", noOpOutcome, err)
	}
	time.Sleep(2 * time.Second)
	noOpStateAfter, err := managedState.Load()
	if err != nil {
		t.Fatal(err)
	}
	noOpConfigAfter, err := os.ReadFile(collectorConfig)
	if err != nil {
		t.Fatal(err)
	}
	noOpPIDsAfter, err := managedProcessIDs(collectorPath, root)
	if err != nil || len(noOpPIDsAfter) != 1 || noOpPIDsAfter[0] != noOpPIDsBefore[0] {
		t.Fatalf("unchanged local configuration restarted or duplicated Collector: before=%v after=%v error=%v", noOpPIDsBefore, noOpPIDsAfter, err)
	}
	if noOpStateAfter.DesiredGeneration != noOpState.DesiredGeneration || noOpStateAfter.ActivatedGeneration != noOpState.ActivatedGeneration || noOpStateAfter.AppliedGeneration != noOpState.AppliedGeneration || string(noOpConfigAfter) != string(noOpConfigBefore) {
		t.Fatalf("unchanged local configuration mutated Agent state or active bytes: before=%+v after=%+v", noOpState, noOpStateAfter)
	}
	waitForSupervisorSinkGrowth(t, ctx, runtimeDone, initialSink, noOpSinkBytes)
	collectorPIDs := waitForManagedProcessCount(t, ctx, runtimeDone, collectorPath, root, 1)
	collectorSinkBytes := len(initialSink.String())
	killManagedProcess(t, collectorPIDs[0])
	replacementCollectorPIDs := waitForManagedProcessReplacement(t, ctx, runtimeDone, collectorPath, root, collectorPIDs[0])
	waitForSupervisorSinkGrowth(t, ctx, runtimeDone, initialSink, collectorSinkBytes)
	if len(replacementCollectorPIDs) != 1 {
		t.Fatalf("Collector owners after managed crash recovery = %v, want exactly one process", replacementCollectorPIDs)
	}
	supervisorPIDs := waitForManagedProcessCount(t, ctx, runtimeDone, supervisorPath, managed.ConfigurationPath, 1)
	killManagedProcess(t, supervisorPIDs[0])
	select {
	case runtimeErr := <-runtimeDone:
		runtimeStopped = true
		if runtimeErr == nil {
			t.Fatal("Agent-managed runtime returned success after an unexpected Supervisor exit")
		}
	case <-ctx.Done():
		t.Fatalf("Agent runtime did not detect the Supervisor crash: %v", ctx.Err())
	}
	waitForManagedProcessCount(t, ctx, runtimeDone, supervisorPath, managed.ConfigurationPath, 0)
	waitForManagedProcessCount(t, ctx, runtimeDone, collectorPath, root, 0)
	crashSinkBytes := len(initialSink.String())
	time.Sleep(1500 * time.Millisecond)
	if got := len(initialSink.String()); got != crashSinkBytes {
		t.Fatalf("OTLP continued after Supervisor crash cleanup: sink bytes %d -> %d", crashSinkBytes, got)
	}
	runContext, stopRuntime = context.WithCancel(ctx)
	runtimeDone = make(chan error, 1)
	runtimeStopped = false
	go func() {
		runtimeDone <- RunCollectorRuntimeWithReconciliation(runContext, statePath, noOpTeardown, nil, nil, nil)
	}()
	waitForSupervisorRuntimeReadiness(t, ctx, runtimeDone, healthEndpoint)
	recoverySinkBytes := len(initialSink.String())
	waitForSupervisorSinkGrowth(t, ctx, runtimeDone, initialSink, recoverySinkBytes)
	waitForManagedProcessCount(t, ctx, runtimeDone, collectorPath, root, 1)
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
	var revisionThreeHash string
	var revisionThreeInstanceUID string
	for _, status := range core.Statuses() {
		if status.Report.Status == "APPLIED" && status.Report.AppliedRevision == 3 {
			revisionThreeHash = status.Report.ConfigHash
			revisionThreeInstanceUID = status.Report.InstanceUID
		}
	}
	if revisionThreeHash == "" || revisionThreeInstanceUID == "" {
		t.Fatal("Mock Core did not retain the applied revision-3 configuration hash and OpAMP instance UID")
	}
	resolvePublishedHash := func() (coreapi.ConfigurationPublication, error) {
		client, err := coreClient(replacementAgentToken)
		if err != nil {
			return coreapi.ConfigurationPublication{}, err
		}
		return client.ResolvePublication(ctx, revisionThreeInstanceUID, revisionThreeHash)
	}
	publication, err := resolvePublishedHash()
	if err != nil || publication.PolicyRevision != 3 || publication.ConfigHash != revisionThreeHash {
		t.Fatalf("resolve revision-3 publication before restart: publication=%+v err=%v", publication, err)
	}
	if err := stopOpAMP(opamp); err != nil {
		t.Fatalf("stop OpAMP before Mock Core restart: %v", err)
	}
	restartedCore, err := mockcore.NewPersistent(seed, coreStatePath)
	if err != nil {
		t.Fatalf("reload Mock Core durable state: %v", err)
	}
	corePointer.Store(restartedCore)
	core = restartedCore
	publication, err = resolvePublishedHash()
	if err != nil || publication.PolicyRevision != 3 || publication.ConfigHash != revisionThreeHash {
		t.Fatalf("resolve revision-3 publication after Mock Core restart: publication=%+v err=%v", publication, err)
	}
	if oldClient, err := coreapi.NewAgentManagementClient(coreHTTP.URL, agentToken, coreHTTP.Client()); err != nil {
		t.Fatal(err)
	} else if _, err := oldClient.GetSession(ctx); err == nil {
		t.Fatal("restarted Mock Core accepted the revoked bearer")
	} else {
		var apiErr *coreapi.ControlAPIError
		if !errors.As(err, &apiErr) || apiErr.Status != http.StatusUnauthorized {
			t.Fatalf("restarted Mock Core old bearer response = %v, want HTTP 401", err)
		}
	}
	if err := restartedCore.SetPolicy(installationID.String(), coreapi.DesiredGatewayPolicy{
		SchemaVersion: "ar-imms/gateway-policy/v1", Revision: 4, GatewayEndpoint: restartedEndpoint,
	}); err != nil {
		t.Fatal(err)
	}
	opamp, err = startOpAMP()
	if err != nil {
		t.Fatalf("restart OpAMP server: %v", err)
	}
	managed.OpAMPServerURL = "wss://" + opamp.Addr().String() + "/v1/opamp"
	if _, err := agentstate.NewStore(statePath).Update(func(state *agentstate.State) error {
		state.Supervisor = managed
		return nil
	}); err != nil {
		t.Fatalf("update OpAMP endpoint after server restart: %v", err)
	}
	runContext, stopRuntime = context.WithCancel(ctx)
	runtimeDone = make(chan error, 1)
	runtimeStopped = false
	go func() {
		runtimeDone <- RunCollectorRuntimeWithReconciliation(runContext, statePath, noOpTeardown, nil, nil, nil)
	}()
	waitForSupervisorRuntimeReadiness(t, ctx, runtimeDone, healthEndpoint)
	waitForSupervisorCorePolicy(t, ctx, runtimeDone, core, 4)
	waitForSupervisorSinkTelemetry(t, ctx, runtimeDone, restartedSink)
	rollbackSinkBytes := len(rotatedSink.String())
	if err := core.SetPolicy(installationID.String(), coreapi.DesiredGatewayPolicy{
		SchemaVersion: "ar-imms/gateway-policy/v1", Revision: 5, GatewayEndpoint: rotatedEndpoint,
	}); err != nil {
		t.Fatal(err)
	}
	waitForSupervisorCorePolicy(t, ctx, runtimeDone, core, 5)
	waitForSupervisorSinkGrowth(t, ctx, runtimeDone, rotatedSink, rollbackSinkBytes)
	rollbackPublication, err := resolvePublishedHash()
	if err != nil || rollbackPublication.PolicyRevision != 5 || rollbackPublication.ConfigHash != revisionThreeHash {
		t.Fatalf("resolve prior applied hash after revision-5 rollback: publication=%+v err=%v", rollbackPublication, err)
	}
	lkgConflictListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve an endpoint to induce Collector startup failure: %v", err)
	}
	lkgConflictEndpoint := lkgConflictListener.Addr().String()
	failedBaseline := strings.ReplaceAll(localBaseline, strings.TrimPrefix(healthEndpoint, "http://"), lkgConflictEndpoint)
	if failedBaseline == localBaseline {
		t.Fatal("LKG failure fixture did not replace the Collector health endpoint")
	}
	lastKnownGoodBefore, err := os.ReadFile(collectorConfig)
	if err != nil {
		t.Fatal(err)
	}
	lastKnownGoodState, err := managedState.Load()
	if err != nil {
		t.Fatal(err)
	}
	lkgPriorPIDs := waitForManagedProcessCount(t, ctx, runtimeDone, collectorPath, root, 1)
	failingCandidate, failingOptions := managedRuntimeCandidate(t, statePath, collectorPath, collectorConfig, managedCandidateBaselinePath(root), []byte(failedBaseline))
	failingOutcome, err := ApplyCollectorCandidate(ctx, managedState, failingCandidate, failingOptions)
	if err != nil || failingOutcome.Result != "activated" {
		t.Fatalf("admit startup-failing candidate for Agent LKG verification: outcome=%+v error=%v", failingOutcome, err)
	}
	select {
	case runtimeErr := <-runtimeDone:
		runtimeStopped = true
		if runtimeErr == nil {
			t.Fatal("Agent runtime returned success after managed Collector failed to reach readiness")
		}
	case <-ctx.Done():
		t.Fatalf("Agent runtime did not report failed managed configuration: %v", ctx.Err())
	}
	failedCollectorPIDs := waitForManagedProcessCount(t, ctx, runtimeDone, collectorPath, root, 0)
	if len(failedCollectorPIDs) != 0 {
		t.Fatalf("Collector survived failed configuration recovery: %v", failedCollectorPIDs)
	}
	failedSupervisorPIDs := waitForManagedProcessCount(t, ctx, runtimeDone, supervisorPath, managed.ConfigurationPath, 0)
	if len(failedSupervisorPIDs) != 0 {
		t.Fatalf("Supervisor survived failed configuration recovery: %v", failedSupervisorPIDs)
	}
	failedSinkBytes := len(rotatedSink.String())
	time.Sleep(1500 * time.Millisecond)
	if got := len(rotatedSink.String()); got != failedSinkBytes {
		t.Fatalf("OTLP continued after failed managed configuration cleanup: sink bytes %d -> %d", failedSinkBytes, got)
	}
	if err := lkgConflictListener.Close(); err != nil {
		t.Fatalf("release reserved LKG failure endpoint: %v", err)
	}
	lastKnownGoodAfter, err := os.ReadFile(collectorConfig)
	if err != nil {
		t.Fatal(err)
	}
	lastKnownGoodFile, err := os.ReadFile(collectorConfig + ".last-known-good")
	if err != nil || string(lastKnownGoodAfter) != string(lastKnownGoodBefore) || string(lastKnownGoodFile) != string(lastKnownGoodBefore) {
		t.Fatalf("Agent LKG did not restore the previously applied config: error=%v active_matches=%t lkg_matches=%t", err, string(lastKnownGoodAfter) == string(lastKnownGoodBefore), string(lastKnownGoodFile) == string(lastKnownGoodBefore))
	}
	lastKnownGoodStateAfter, err := managedState.Load()
	if err != nil {
		t.Fatal(err)
	}
	if lastKnownGoodStateAfter.ActivatedGeneration != lastKnownGoodState.AppliedGeneration || lastKnownGoodStateAfter.AppliedGeneration != lastKnownGoodState.AppliedGeneration || lastKnownGoodStateAfter.PendingActivation != nil {
		t.Fatalf("Agent state did not return to the previously applied generation: before=%+v after=%+v", lastKnownGoodState, lastKnownGoodStateAfter)
	}
	failureMapping, failureGeneration, foundFailure, err := managedState.CandidateOutcome(failingCandidate.DesiredSetRevision, failingCandidate.BaselineFingerprint, failingCandidate.Candidate.Digest())
	if err != nil || !foundFailure || failureGeneration != failingOutcome.Generation || failureMapping.Outcome != "failed" || failureMapping.Stage != "readiness" || failureMapping.RecoveryResult != "succeeded" {
		t.Fatalf("Agent did not durably record LKG recovery: mapping=%+v generation=%d found=%t error=%v", failureMapping, failureGeneration, foundFailure, err)
	}
	if len(lkgPriorPIDs) != 1 {
		t.Fatalf("LKG failure test began with non-unique Collector owners: %v", lkgPriorPIDs)
	}
	stopRuntime()
	runContext, stopRuntime = context.WithCancel(ctx)
	runtimeDone = make(chan error, 1)
	runtimeStopped = false
	go func() {
		runtimeDone <- RunCollectorRuntimeWithReconciliation(runContext, statePath, noOpTeardown, nil, nil, nil)
	}()
	waitForSupervisorRuntimeReadiness(t, ctx, runtimeDone, healthEndpoint)
	lkgSinkBytes := len(rotatedSink.String())
	waitForSupervisorSinkGrowth(t, ctx, runtimeDone, rotatedSink, lkgSinkBytes)
	stopRuntime()
	select {
	case err := <-runtimeDone:
		runtimeStopped = true
		if err != nil {
			t.Fatalf("Agent-managed runtime shutdown after OpAMP restart: %v", err)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("Agent-managed runtime process tree did not stop after OpAMP restart")
	}
	if err := stopOpAMP(opamp); err != nil {
		t.Fatalf("stop restarted OpAMP server: %v", err)
	}
	if initialSink.String() == "" || updatedSink.String() == "" || rotatedSink.String() == "" || restartedSink.String() == "" {
		t.Fatal("initial, updated, post-rotation, and post-restart Gateway sinks must receive OTLP")
	}
	writeManagedControlE2EEvidence(t, result.InstallationID, result.Binding.NodeID)
	t.Logf("verified enrollment identity=%s Node=%s; offline Agent-managed fallback delivered before OpAMP connected; authenticated reconnect applied policy revisions=1,2,3,4,5; replacement credential revoked the prior bearer; durable revision-3 publication resolved after Mock Core restart; restarted OpAMP and Agent runtime applied revision 4; revision-5 rollback re-attributed the prior applied hash and continued OTLP delivery", result.InstallationID, result.Binding.NodeID)
}

func managedRuntimeEnrollmentHandler(t *testing.T, corePointer *atomic.Pointer[mockcore.Server], accessToken, replacementToken, bootstrapToken, replacementBootstrap, nodeID string) http.Handler {
	t.Helper()
	enrollmentCount := 0
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		core := corePointer.Load()
		if core == nil {
			http.Error(writer, "Mock Core is unavailable", http.StatusServiceUnavailable)
			return
		}
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
	contents := fmt.Sprintf("result=PASS\nos=%s\narchitecture=%s\ncommit=%s\nenrollment=actual-agent-service\ncredential=protected-store-and-secret-free-sqlite\ncredential_replacement=previous_revoked_new_wss_session_verified\ntransport=authenticated-wss\noffline_startup_fallback_reconnect=verified\nlocal_configuration_update=verified_runtime_restart_and_otlp\nunchanged_configuration=noop_no_generation_change_no_collector_restart\nagent_lkg_equivalence=verified_active_and_lkg_bytes_state_and_journal\ncollector_crash_recovery=verified_single_replacement_and_otlp\nsupervisor_crash_process_tree_cleanup=verified\nagent_runtime_restart_after_supervisor_crash=verified\npolicy_revisions=1,2,3,4,5\nmock_core_restart=verified\nrevision3_publication_resolved_after_restart=verified\nopamp_server_restart=verified\nagent_runtime_restart=verified\nrollback_reuses_revision3_config_hash_at_revision5=verified\nrollback_gateway_otlp=verified\ninitial_gateway_otlp=verified\nupdated_gateway_otlp=verified\npost_rotation_gateway_otlp=verified\npost_restart_gateway_otlp=verified\ninstallation_identity=%s\nnode_identity=%s\n", runtime.GOOS, runtime.GOARCH, os.Getenv("GITHUB_SHA"), installationID, nodeID)
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

func waitForSupervisorSinkGrowth(t *testing.T, ctx context.Context, done <-chan error, output *supervisorRuntimeBuffer, previousSize int) {
	t.Helper()
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		if len(output.String()) > previousSize {
			return
		}
		select {
		case err := <-done:
			t.Fatalf("Agent-managed Supervisor exited before rollback Gateway OTLP arrived: %v", err)
		case <-ctx.Done():
			t.Fatalf("timed out waiting for rollback Gateway OTLP: %v", ctx.Err())
		case <-ticker.C:
		}
	}
}

func waitForManagedProcessCount(t *testing.T, ctx context.Context, runtimeDone <-chan error, executablePath, commandMarker string, expected int) []int {
	t.Helper()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		processIDs, err := managedProcessIDs(executablePath, commandMarker)
		if err != nil {
			t.Fatalf("inspect managed process %q: %v", executablePath, err)
		}
		if len(processIDs) == expected {
			return processIDs
		}
		select {
		case runtimeErr := <-runtimeDone:
			t.Fatalf("Agent runtime exited while waiting for %d process(es) %q: %v", expected, executablePath, runtimeErr)
		case <-ctx.Done():
			t.Fatalf("timed out waiting for %d process(es) %q; found %v: %v", expected, executablePath, processIDs, ctx.Err())
		case <-ticker.C:
		}
	}
}

func waitForManagedProcessReplacement(t *testing.T, ctx context.Context, runtimeDone <-chan error, executablePath, commandMarker string, previousPID int) []int {
	t.Helper()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		processIDs, err := managedProcessIDs(executablePath, commandMarker)
		if err != nil {
			t.Fatalf("inspect replacement managed process %q: %v", executablePath, err)
		}
		if len(processIDs) == 1 && processIDs[0] != previousPID {
			return processIDs
		}
		select {
		case runtimeErr := <-runtimeDone:
			t.Fatalf("Agent runtime exited before managed process recovery: %v", runtimeErr)
		case <-ctx.Done():
			t.Fatalf("timed out waiting for replacement of process %d; current processes=%v: %v", previousPID, processIDs, ctx.Err())
		case <-ticker.C:
		}
	}
}

func killManagedProcess(t *testing.T, processID int) {
	t.Helper()
	process, err := os.FindProcess(processID)
	if err != nil {
		t.Fatalf("open managed process %d: %v", processID, err)
	}
	if err := process.Kill(); err != nil {
		t.Fatalf("kill managed process %d: %v", processID, err)
	}
}

func managedProcessIDs(executablePath, commandMarker string) ([]int, error) {
	if runtime.GOOS == "windows" {
		command := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", `$image = $env:AR_IMMS_TEST_PROCESS_IMAGE; $marker = $env:AR_IMMS_TEST_PROCESS_MARKER; Get-CimInstance Win32_Process | Where-Object { $_.ExecutablePath -ieq $image -and $_.CommandLine -like ("*" + $marker + "*") } | ForEach-Object { [Console]::WriteLine($_.ProcessId) }`)
		command.Env = append(os.Environ(), "AR_IMMS_TEST_PROCESS_IMAGE="+executablePath, "AR_IMMS_TEST_PROCESS_MARKER="+commandMarker)
		output, err := command.CombinedOutput()
		if err != nil {
			return nil, fmt.Errorf("query Windows process list: %w (%s)", err, strings.TrimSpace(string(output)))
		}
		return parseManagedProcessIDs(string(output))
	}
	if runtime.GOOS != "linux" {
		return nil, fmt.Errorf("managed process inspection is unsupported on %s", runtime.GOOS)
	}
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil, fmt.Errorf("read Linux process table: %w", err)
	}
	processIDs := make([]int, 0, 1)
	for _, entry := range entries {
		processID, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}
		processPath, err := os.Readlink(filepath.Join("/proc", entry.Name(), "exe"))
		if err != nil || !samePath(processPath, executablePath) {
			continue
		}
		commandLine, err := os.ReadFile(filepath.Join("/proc", entry.Name(), "cmdline"))
		if err == nil && strings.Contains(string(commandLine), commandMarker) {
			processIDs = append(processIDs, processID)
		}
	}
	return processIDs, nil
}

func parseManagedProcessIDs(output string) ([]int, error) {
	processIDs := make([]int, 0, 1)
	for _, line := range strings.Fields(output) {
		processID, err := strconv.Atoi(line)
		if err != nil {
			return nil, fmt.Errorf("parse process ID %q: %w", line, err)
		}
		processIDs = append(processIDs, processID)
	}
	return processIDs, nil
}
