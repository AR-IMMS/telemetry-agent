package agentlifecycle

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	credentialstore "github.com/ar-imms/telemetry-agent/internal/identity/credentials"
	fleetenrollment "github.com/ar-imms/telemetry-agent/internal/identity/fleetenrollment/application"
	fleethttp "github.com/ar-imms/telemetry-agent/internal/identity/fleetenrollment/delivery/http"
	identity "github.com/ar-imms/telemetry-agent/internal/identity/host"
	installationapp "github.com/ar-imms/telemetry-agent/internal/identity/installation/application"
	"github.com/ar-imms/telemetry-agent/internal/runtime/bootstrap"
	"github.com/ar-imms/telemetry-agent/internal/runtime/collector/supervisor"
	opampdomain "github.com/ar-imms/telemetry-agent/internal/runtime/opamp/domain"
	"github.com/ar-imms/telemetry-agent/internal/testsupport/sqlitefixture"
)

// TestSecureOTLPSupervisorUAT exercises a Core-issued invitation through real
// Fleet, the Agent protected store, the official Supervisor, the pinned
// Collector, the TLS ingress, the Gateway, and Core-driven revocation.
func TestSecureOTLPSupervisorUAT(t *testing.T) {
	if os.Getenv("AR_IMMS_SECURE_OTLP_SUPERVISOR_UAT") != "1" {
		t.Skip("set AR_IMMS_SECURE_OTLP_SUPERVISOR_UAT=1 for the live cross-service UAT")
	}
	if runtime.GOOS != "windows" && runtime.GOOS != "linux" {
		t.Fatalf("unsupported UAT runtime platform %q", runtime.GOOS)
	}
	root := strings.TrimSpace(os.Getenv("AR_IMMS_SECURE_OTLP_UAT_DIR"))
	invitationFile := strings.TrimSpace(os.Getenv("AR_IMMS_SECURE_OTLP_INVITATION_FILE"))
	if invitationFile == "" {
		t.Fatal("protected Core invitation file is required")
	}
	value, err := os.ReadFile(invitationFile)
	if err != nil || len(value) == 0 || len(value) > 4096 {
		t.Fatal("read bounded Core invitation from the protected UAT file")
	}
	invitation := strings.TrimSpace(string(value))
	clear(value)
	if err := os.Remove(invitationFile); err != nil {
		t.Fatal("remove temporary Core invitation input")
	}
	invitationID := os.Getenv("AR_IMMS_SECURE_OTLP_INVITATION_ID")
	fleetOrigin := os.Getenv("AR_IMMS_SECURE_OTLP_FLEET_ORIGIN")
	clientCertFile := os.Getenv("AR_IMMS_SECURE_OTLP_AGENT_CERT")
	clientKeyFile := os.Getenv("AR_IMMS_SECURE_OTLP_AGENT_KEY")
	caFile := os.Getenv("AR_IMMS_SECURE_OTLP_CA_FILE")
	supervisorPath := os.Getenv("AR_IMMS_PR07A_SUPERVISOR_BINARY")
	collectorPath := os.Getenv("AR_IMMS_PR05_COLLECTOR_BINARY")
	for name, value := range map[string]string{
		"UAT directory": root, "Core-issued invitation": invitation,
		"invitation ID": invitationID, "Fleet origin": fleetOrigin,
		"Agent client certificate": clientCertFile, "Agent client key": clientKeyFile,
		"TLS CA": caFile, "official Supervisor": supervisorPath, "pinned Collector": collectorPath,
	} {
		if strings.TrimSpace(value) == "" {
			t.Fatalf("%s is required", name)
		}
	}
	platform := identity.PlatformInfo{OS: runtime.GOOS, Architecture: runtime.GOARCH}
	supervisorArtifact, err := bootstrap.SelectSupervisorArtifact(platform)
	if err != nil {
		t.Fatal(err)
	}
	if got := fileSHA256(t, supervisorPath); got != supervisorArtifact.SHA256 {
		t.Fatalf("official Supervisor hash does not match the repository pin")
	}
	collectorVersion, err := exec.Command(collectorPath, "--version").CombinedOutput()
	if err != nil || !strings.Contains(string(collectorVersion), "version 0.160.0") {
		t.Fatalf("Collector binary is not the pinned v0.160.0 build")
	}

	stateRoot := envOr("AR_IMMS_SECURE_OTLP_AGENT_STATE_DIR", root)
	if err := os.MkdirAll(stateRoot, 0700); err != nil {
		t.Fatal("create temporary Agent state directory")
	}
	statePath := filepath.Join(stateRoot, "agent-state-"+runtime.GOOS+".db")
	installationID, err := installationapp.EnsureInstallationIdentity(sqlitefixture.NewIdentityStore(statePath), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	caPEM, err := os.ReadFile(caFile)
	if err != nil {
		t.Fatal("read UAT CA")
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(caPEM) {
		t.Fatal("parse UAT CA")
	}
	clientCertificate, err := tls.LoadX509KeyPair(clientCertFile, clientKeyFile)
	if err != nil {
		t.Fatal("load UAT Agent TLS identity")
	}
	transport := &http.Transport{TLSClientConfig: &tls.Config{
		MinVersion: tls.VersionTLS12, RootCAs: roots, Certificates: []tls.Certificate{clientCertificate},
	}}
	defer transport.CloseIdleConnections()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	fleetRepo := sqlitefixture.NewFleetEnrollmentStore(statePath)
	credentialStore := credentialstore.New(statePath)
	fleetClient := fleethttp.NewClient(&http.Client{Transport: transport, Timeout: 15 * time.Second})
	enrollmentService := fleetenrollment.Service{
		Repository: fleetRepo, Credentials: credentialStore, Client: fleetClient,
	}
	result, err := enrollmentService.Start(ctx, fleetOrigin, installationID.String(), invitation)
	if err != nil {
		t.Fatalf("Agent Fleet enrollment failed: %s", strings.ReplaceAll(err.Error(), invitation, "<redacted>"))
	}
	if result.OTLPCredentialID == "" || result.OTLPCredentialScope != "otlp.write" || result.OpAMPEndpoint == "" {
		t.Fatal("Fleet did not return the expected separate OTLP and OpAMP credentials")
	}
	attempt, err := fleetRepo.LoadAttempt()
	if err != nil {
		t.Fatal("load committed Fleet enrollment state")
	}
	if _, err := credentialStore.Get(attempt.OTLPCredentialRef); err != nil {
		t.Fatal("Fleet OTLP secret was not stored in the protected credential store")
	}
	if _, err := credentialStore.Get(attempt.CredentialRef); err != nil {
		t.Fatal("Fleet OpAMP secret was not stored in the protected credential store")
	}

	healthEndpoint := reserveSupervisorRuntimeEndpoint(t)
	configPath := filepath.Join(root, "secure-collector.yaml")
	gatewayEndpoint := envOr("AR_IMMS_SECURE_OTLP_GATEWAY_ENDPOINT", "127.0.0.1:18417")
	tokenFile, err := credentialStore.ProvisionOTLPToken(attempt.OTLPCredentialRef)
	if err != nil {
		t.Fatal("provision protected Collector token file")
	}
	config := fmt.Sprintf(`receivers:
  hostmetrics:
    collection_interval: 1s
    scrapers:
      cpu: {}
      memory: {}
processors:
  resource:
    attributes:
      - key: agent_id
        action: upsert
        value: client-supplied-spoof
  batch:
    timeout: 1s
extensions:
  health_check:
    endpoint: %q
  bearertokenauth/otlp:
    filename: '${env:AR_IMMS_OTLP_TOKEN_FILE}'
    retry_on_failure:
      enabled: false
exporters:
  otlphttp/gateway:
    endpoint: 'https://${env:OTEL_GATEWAY_ENDPOINT}'
    encoding: json
    compression: none
    auth:
      authenticator: bearertokenauth/otlp
    tls:
      ca_file: %q
      insecure: false
service:
  extensions: [health_check, bearertokenauth/otlp]
  pipelines:
    metrics:
      receivers: [hostmetrics]
      processors: [resource, batch]
      exporters: [otlphttp/gateway]
`, strings.TrimPrefix(healthEndpoint, "http://"), caFile)
	if err := os.WriteFile(configPath, []byte(config), 0600); err != nil {
		t.Fatal("write temporary secure Collector configuration")
	}
	validation := exec.CommandContext(ctx, collectorPath, "validate", "--config", configPath)
	validation.Env = append(os.Environ(),
		"AR_IMMS_OTLP_TOKEN_FILE="+tokenFile,
		"OTEL_GATEWAY_ENDPOINT="+gatewayEndpoint,
	)
	if output, err := validation.CombinedOutput(); err != nil {
		t.Fatalf("pinned Collector rejected secure UAT config: %v\n%s", err, output)
	}
	managed := opampdomain.SupervisorContext{
		BinaryPath:        supervisorPath,
		ConfigurationPath: filepath.Join(filepath.Dir(statePath), "credentials", "opamp-supervisor.yaml"),
		StateDirectory:    filepath.Join(filepath.Dir(statePath), "credentials", "opamp-supervisor-state"),
		OpAMPServerURL:    result.OpAMPEndpoint, CredentialSource: opampdomain.CredentialSourceFleet,
		ServerCAFile: caFile, Enabled: true,
	}
	if runtime.GOOS == "linux" {
		startSecureOTLPUATOpAMPRelay(t, result.OpAMPEndpoint)
	}
	options := supervisor.Options{
		BinaryPath: collectorPath, ConfigPath: configPath,
		HealthEndpoint: healthEndpoint, GatewayEndpoint: gatewayEndpoint,
		OTLPTokenFile: tokenFile, StartupTimeout: 30 * time.Second, ShutdownTimeout: 10 * time.Second,
	}
	runContext, stop := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() {
		done <- runManagedSupervisor(runContext, statePath, managed, options,
			sqlitefixture.NewEnrollmentStore(statePath), fleetRepo)
	}()
	defer func() {
		stop()
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("stop managed Supervisor: %v", err)
			}
		case <-time.After(20 * time.Second):
			t.Error("managed Supervisor process tree did not stop")
		}
	}()
	waitForSupervisorRuntimeReadiness(t, ctx, done, healthEndpoint)
	if err := waitForUATGatewayIdentity(ctx, installationID.String()); err != nil {
		t.Fatal(err)
	}
	enrolled := map[string]string{
		"result": "ENROLLED", "invitation_id": invitationID,
		"installation_id": installationID.String(), "agent_id": installationID.String(),
	}
	if err := writeUATRecord(filepath.Join(root, "enrolled.json"), enrolled); err != nil {
		t.Fatal("write sanitized enrollment checkpoint")
	}
	if err := waitForUATSignal(ctx, filepath.Join(root, "revoke.signal"), done); err != nil {
		t.Fatal(err)
	}
	if err := requireRevokedOTLP(ctx, roots, credentialStore, attempt.OTLPCredentialRef); err != nil {
		t.Fatal(err)
	}
	if err := writeUATRecord(filepath.Join(root, "revocation.json"), map[string]string{
		"result": "DENIED", "status": "401", "agent_id": installationID.String(),
	}); err != nil {
		t.Fatal("write sanitized revocation checkpoint")
	}
	t.Log("real Core/Fleet enrollment, official Supervisor, pinned Collector TLS export, Gateway attribution, and Core revocation passed")
}

func startSecureOTLPUATOpAMPRelay(t *testing.T, endpoint string) {
	t.Helper()
	parsed, err := url.Parse(endpoint)
	if err != nil {
		t.Fatal("parse the registered Fleet OpAMP endpoint")
	}
	_, port, err := net.SplitHostPort(parsed.Host)
	if err != nil {
		t.Fatal("parse the registered Fleet OpAMP port")
	}
	listener, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", port))
	if err != nil {
		t.Fatal("start the UAT-only transparent OpAMP relay")
	}
	closed := make(chan struct{})
	go func() {
		defer close(closed)
		for {
			client, acceptErr := listener.Accept()
			if acceptErr != nil {
				return
			}
			go func() {
				defer client.Close()
				upstream, dialErr := net.DialTimeout("tcp", net.JoinHostPort("host.docker.internal", port), 5*time.Second)
				if dialErr != nil {
					return
				}
				defer upstream.Close()
				finished := make(chan struct{}, 2)
				go func() { _, _ = io.Copy(upstream, client); finished <- struct{}{} }()
				go func() { _, _ = io.Copy(client, upstream); finished <- struct{}{} }()
				<-finished
			}()
		}
	}()
	t.Cleanup(func() {
		_ = listener.Close()
		select {
		case <-closed:
		case <-time.After(3 * time.Second):
			t.Error("UAT-only OpAMP relay did not stop")
		}
	})
}

func waitForUATGatewayIdentity(ctx context.Context, agentID string) error {
	client := &http.Client{Timeout: 2 * time.Second}
	metricsURL := envOr("AR_IMMS_SECURE_OTLP_GATEWAY_METRICS_URL", "http://127.0.0.1:14388/metrics")
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		request, _ := http.NewRequestWithContext(ctx, http.MethodGet, metricsURL, nil)
		response, err := client.Do(request)
		if err == nil {
			body, readErr := readBoundedBody(response.Body, 4<<20)
			response.Body.Close()
			if readErr == nil && response.StatusCode == http.StatusOK && strings.Contains(string(body), agentID) && !strings.Contains(string(body), "client-supplied-spoof") {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("Gateway did not expose the authenticated Agent ID within the UAT deadline")
		case <-ticker.C:
		}
	}
}

func waitForUATSignal(ctx context.Context, path string, done <-chan error) error {
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		if _, err := os.Stat(path); err == nil {
			return nil
		}
		select {
		case err := <-done:
			return fmt.Errorf("managed Supervisor exited before Core revocation signal: %v", err)
		case <-ctx.Done():
			return fmt.Errorf("timed out waiting for Core revocation signal")
		case <-ticker.C:
		}
	}
}

func requireRevokedOTLP(ctx context.Context, roots *x509.CertPool, store credentialstore.Store, reference string) error {
	token, err := store.Get(reference)
	if err != nil {
		return fmt.Errorf("load protected token for revocation probe")
	}
	transport := &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots}}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 10 * time.Second}
	ingressURL := envOr("AR_IMMS_SECURE_OTLP_INGRESS_URL", "https://127.0.0.1:18417/v1/metrics")
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, ingressURL, strings.NewReader(`{"resourceMetrics":[]}`))
	if err != nil {
		return fmt.Errorf("create revocation probe")
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("send revocation probe")
	}
	response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		return fmt.Errorf("Fleet-revoked OTLP token returned HTTP %d, want 401", response.StatusCode)
	}
	return nil
}

func envOr(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func writeUATRecord(path string, value map[string]string) error {
	content, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return os.WriteFile(path, content, 0600)
}

func readBoundedBody(body io.Reader, limit int64) ([]byte, error) {
	return io.ReadAll(io.LimitReader(body, limit+1))
}
