package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/ar-imms/telemetry-agent/internal/agenthealth"
	"github.com/ar-imms/telemetry-agent/internal/agentstate"
	"github.com/ar-imms/telemetry-agent/internal/bootstrap"
	"github.com/ar-imms/telemetry-agent/internal/collectorruntime"
	"github.com/ar-imms/telemetry-agent/internal/identity"
)

const applicationProfilesE2EEnv = "AR_IMMS_PR06_APPLICATION_PROFILE_E2E"

// TestLiveDockerApplicationProfilesDeliverOTLP verifies native Redis,
// PostgreSQL, and Prometheus profile delivery through the local Collector.
func TestLiveDockerApplicationProfilesDeliverOTLP(t *testing.T) {
	if os.Getenv(applicationProfilesE2EEnv) != "1" {
		t.Skip("set " + applicationProfilesE2EEnv + "=1 to run live application profile E2E")
	}
	binary := strings.TrimSpace(os.Getenv("AR_IMMS_PR05_COLLECTOR_BINARY"))
	if binary == "" {
		t.Fatal("AR_IMMS_PR05_COLLECTOR_BINARY must point to pinned Collector v0.160.0")
	}
	dockerEndpoint := strings.TrimSpace(os.Getenv(dockerDiscoveryEndpointEnv))
	if dockerEndpoint == "" {
		var err error
		dockerEndpoint, err = dockerContextEndpoint()
		if err != nil {
			t.Fatal(err)
		}
	}
	if output, err := nativeDockerCommand(context.Background(), "version", "--format", "{{.Server.Version}}").CombinedOutput(); err != nil {
		t.Fatalf("Docker daemon is unavailable: %v (%s)", err, strings.TrimSpace(string(output)))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 7*time.Minute)
	defer cancel()
	root := t.TempDir()
	sinkOutput := &lockedTemplateE2EBuffer{}
	sinkEndpoint := startTemplateOTLPSink(t, ctx, binary, root, sinkOutput)
	configRoot := filepath.Join(root, "config")
	copyAgentConfigRoot(t, filepath.Join("..", "..", "configs"), configRoot)
	caFile := makePostgreSQLTestCertificate(t, root)
	writeApplicationProfileConfig(t, configRoot, caFile)
	postgresImage := buildPostgreSQLTestImage(t, ctx, root)
	for _, image := range []string{"redis:7-alpine", "postgres:17-alpine", postgresImage, "python:3.12-alpine"} {
		t.Logf("PR06_IMAGE tag=%s image_id=%s", image, nativeDockerImageID(t, ctx, image))
	}
	pgUser, pgPassword, pgAdminPassword := "pr06_"+randomProfileSecret(t), randomProfileSecret(t), randomProfileSecret(t)
	t.Setenv("AR_IMMS_PR06_PG_USER", pgUser)
	t.Setenv("AR_IMMS_PR06_PG_PASSWORD", pgPassword)
	t.Setenv(dockerDiscoveryEndpointEnv, dockerEndpoint)
	t.Setenv("OTELCOL_CONTRIB_BINARY", binary)

	name := fmt.Sprintf("arimms-pr06-%d", time.Now().UnixNano())
	redisName, postgresName, pythonName := name+"-redis", name+"-postgres", name+"-python"
	coolingName := name + "-cooling"
	labels := func(profile, workload string) []string {
		return []string{"--label", "ar-imms.monitoring.enabled=true", "--label", "ar-imms.monitoring.profile=" + profile, "--label", "ar-imms.workload.id=" + workload}
	}
	startProfileContainer(t, ctx, redisName, append(labels("redis", "pr06-redis"), "--publish", "127.0.0.1::6379", "redis:7-alpine")...)
	t.Cleanup(func() { stopNativeRedisContainer(redisName) })
	startProfileContainer(t, ctx, postgresName, append([]string{"--env", "POSTGRES_USER=postgres", "--env", "POSTGRES_PASSWORD=" + pgAdminPassword}, append(labels("postgres", "pr06-postgres"), "--publish", "127.0.0.1::5432", postgresImage)...)...)
	t.Cleanup(func() { stopNativeRedisContainer(postgresName) })
	waitProfileContainerReady(t, ctx, postgresName, pgAdminPassword)
	createRole := fmt.Sprintf("CREATE ROLE %s WITH LOGIN PASSWORD '%s'; GRANT pg_monitor TO %s", pgUser, pgPassword, pgUser)
	configurePostgreSQLMonitoringRole(t, ctx, postgresName, pgAdminPassword, createRole)
	pythonScript := func(metric, help string) string {
		body := fmt.Sprintf("# HELP %s %s\n# TYPE %s gauge\n%s 1\n", metric, help, metric, metric)
		return fmt.Sprintf("from http.server import BaseHTTPRequestHandler,HTTPServer\nclass H(BaseHTTPRequestHandler):\n def do_GET(self):\n  b=%q.encode('utf-8')\n  self.send_response(200); self.send_header('Content-Type','text/plain; version=0.0.4'); self.end_headers(); self.wfile.write(b)\nHTTPServer(('0.0.0.0',9108),H).serve_forever()", body)
	}
	startProfileContainer(t, ctx, pythonName, append(labels("python-metrics", "pr06-python"), "--publish", "127.0.0.1::9108", "python:3.12-alpine", "python", "-c", pythonScript("python_app_requests_total", "Python application request fixture"))...)
	t.Cleanup(func() { stopNativeRedisContainer(pythonName) })
	startProfileContainer(t, ctx, coolingName, append(labels("mock-cooling", "pr06-cooling"), "--publish", "127.0.0.1::9108", "python:3.12-alpine", "python", "-c", pythonScript("ar_imms_test_cooling_celsius", "Mock cooling exporter fixture"))...)
	t.Cleanup(func() { stopNativeRedisContainer(coolingName) })

	statePath := filepath.Join(root, "state.db")
	configPath := filepath.Join(root, "agent", "otel.yaml")
	store := agentstate.NewSQLiteStore(statePath)
	if err := store.Save(agentstate.State{
		Collector:         agentstate.CollectorContext{ConfigRoot: configRoot, BinaryPath: binary, ConfigPath: configPath, GatewayEndpoint: sinkEndpoint, HealthEndpoint: defaultHealthEndpoint},
		DesiredGeneration: 1, ActivatedGeneration: 1, AppliedGeneration: 1, Dependencies: map[string]agentstate.DependencyState{},
	}); err != nil {
		t.Fatal(err)
	}
	installationID, err := identity.EnsureInstallationIdentity(agentstate.NewIdentityStore(statePath), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	nodeID := "33333333-4444-4555-8666-777777777777"
	seedRegisteredEnrollment(t, statePath, installationID.String(), nodeID)
	baseline, err := newRuntimeBaseline(statePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(configPath), 0o700); err != nil {
		t.Fatal(err)
	}
	initial, err := baseline.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := collectorruntime.StaticCatalog(baseline.platform)
	if err != nil {
		t.Fatal(err)
	}
	candidate, err := collectorruntime.NewCompiler(catalog).Compile(initial.Input, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, candidate.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	collectorLogs := &nativeCollectorLogReporter{health: agenthealth.NewReporter(agenthealth.RuntimeObservation{CollectorState: agenthealth.CollectorStateStarting})}
	serviceCtx, stopService := context.WithCancel(ctx)
	serviceDone := make(chan error, 1)
	go func() {
		serviceDone <- runRuntimeWithReconciliation(serviceCtx, statePath, func(context.Context, string, agentstate.TeardownAction, []agentstate.OwnedResource) error { return nil }, collectorLogs, bootstrap.OSCommandRunner{})
	}()
	t.Cleanup(func() {
		stopService()
		select {
		case <-serviceDone:
		case <-time.After(10 * time.Second):
			t.Error("Agent runtime did not stop")
		}
	})
	resources := map[string]string{
		"redis.clients.connected":      nativeDockerContainerID(t, ctx, redisName),
		"postgresql.backends":          nativeDockerContainerID(t, ctx, postgresName),
		"python_app_requests_total":    nativeDockerContainerID(t, ctx, pythonName),
		"ar_imms_test_cooling_celsius": nativeDockerContainerID(t, ctx, coolingName),
	}
	for metric, resourceID := range resources {
		waitForApplicationMetric(t, ctx, serviceDone, sinkOutput, collectorLogs, nodeID, resourceID, metric)
		t.Logf("PR06_OTLP_DELIVERED metric=%s resource_id=%s node_id=%s", metric, resourceID, nodeID)
	}
	assertNativeReceiverUniqueness(t, collectorLogs.output())
	desired, err := store.IntegrationRepository().Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if len(desired.Instances) != 0 {
		t.Fatalf("native profiles created Agent-owned duplicate receiver instances: %+v", desired.Instances)
	}
	beforeChurn, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	configBeforeChurn, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	oldPythonID := resources["ar_imms_test_cooling_celsius"]
	stopNativeRedisContainer(coolingName)
	pythonReplacement := coolingName + "-replacement"
	startProfileContainer(t, ctx, pythonReplacement, append(labels("mock-cooling", "pr06-cooling"), "--publish", "127.0.0.1::9108", "python:3.12-alpine", "python", "-c", pythonScript("ar_imms_test_cooling_celsius", "Mock cooling exporter fixture"))...)
	t.Cleanup(func() { stopNativeRedisContainer(pythonReplacement) })
	newPythonID := nativeDockerContainerID(t, ctx, pythonReplacement)
	if newPythonID == oldPythonID {
		t.Fatalf("recreated Prometheus workload reused resource_id %s", oldPythonID)
	}
	waitForApplicationMetric(t, ctx, serviceDone, sinkOutput, collectorLogs, nodeID, newPythonID, "ar_imms_test_cooling_celsius")
	assertNativeReceiverUniqueness(t, collectorLogs.output())
	if duplicates := applicationDuplicateSamples(sinkOutput.String(), "ar_imms_test_cooling_celsius"); len(duplicates) > 0 {
		t.Fatalf("duplicate Prometheus OTLP samples after container recreation: %v", duplicates)
	}
	for metric, resourceID := range resources {
		if !applicationSinkContainsIdentity(sinkOutput.String(), metric, nodeID, resourceID) {
			t.Fatalf("isolated OTLP sink lacks metric=%s resource_id=%s", metric, resourceID)
		}
		if duplicates := applicationDuplicateSamples(sinkOutput.String(), metric); len(duplicates) > 0 {
			t.Fatalf("duplicate OTLP samples for %s: %v", metric, duplicates)
		}
	}
	stateAfter, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if stateAfter.AppliedGeneration != stateAfter.ActivatedGeneration {
		t.Fatalf("Collector did not apply its activated generation: applied=%d activated=%d", stateAfter.AppliedGeneration, stateAfter.ActivatedGeneration)
	}
	if stateAfter.AppliedGeneration != beforeChurn.AppliedGeneration {
		t.Fatalf("container churn changed Collector generation: %d -> %d", beforeChurn.AppliedGeneration, stateAfter.AppliedGeneration)
	}
	configAfterChurn, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(configBeforeChurn) != string(configAfterChurn) {
		t.Fatal("container churn changed generated Collector configuration")
	}
	if runtime.GOOS != "windows" && runtime.GOOS != "linux" {
		t.Fatalf("unsupported E2E host %s", runtime.GOOS)
	}
}

func copyAgentConfigRoot(t *testing.T, source, target string) {
	t.Helper()
	err := filepath.WalkDir(source, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		if strings.HasPrefix(filepath.ToSlash(relative), "monitoring-profiles/") {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		copyPath := filepath.Join(target, relative)
		if err := os.MkdirAll(filepath.Dir(copyPath), 0o700); err != nil {
			return err
		}
		return os.WriteFile(copyPath, content, 0o600)
	})
	if err != nil {
		t.Fatalf("copy base Collector configuration: %v", err)
	}
}

func writeApplicationProfileConfig(t *testing.T, root, caFile string) {
	t.Helper()
	directory := filepath.Join(root, "monitoring-profiles")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	content := `api_version: ar-imms/v1alpha1
profiles:
  - id: postgres
    provider: docker
    receiver_type: postgresql
    required_capabilities: [otel.receiver.postgresql.metrics]
    match_labels: {ar-imms.monitoring.enabled: "true", ar-imms.monitoring.profile: postgres}
    collection_interval: 10s
    target_port: 5432
    target_port_label: ar-imms.monitoring.port
    username_env: AR_IMMS_PR06_PG_USER
    password_env: AR_IMMS_PR06_PG_PASSWORD
    tls_ca_file: %q
  - id: python-metrics
    provider: docker
    receiver_type: prometheus
    required_capabilities: [otel.receiver.prometheus.metrics]
    match_labels: {ar-imms.monitoring.enabled: "true", ar-imms.monitoring.profile: python-metrics}
    collection_interval: 5s
    target_port: 9108
    target_port_label: ar-imms.monitoring.port
    metrics_path: /metrics
  - id: mock-cooling
    provider: docker
    receiver_type: prometheus
    required_capabilities: [otel.receiver.prometheus.metrics]
    match_labels: {ar-imms.monitoring.enabled: "true", ar-imms.monitoring.profile: mock-cooling}
    collection_interval: 5s
    target_port: 9108
    target_port_label: ar-imms.monitoring.port
    metrics_path: /metrics

`
	content = fmt.Sprintf(content, caFile)
	if err := os.WriteFile(filepath.Join(directory, "application.yaml"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func makePostgreSQLTestCertificate(t *testing.T, root string) string {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	template := x509.Certificate{
		SerialNumber: big.NewInt(now.UnixNano()), NotBefore: now.Add(-time.Minute), NotAfter: now.Add(24 * time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}, DNSNames: []string{"localhost"},
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	certificate := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	privateKey, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privateKey})
	contextDir := filepath.Join(root, "postgres-image")
	if err := os.MkdirAll(contextDir, 0o700); err != nil {
		t.Fatal(err)
	}
	certPath := filepath.Join(contextDir, "server.crt")
	if err := os.WriteFile(certPath, certificate, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(contextDir, "server.key"), keyPEM, 0o600); err != nil {
		t.Fatal(err)
	}
	dockerfile := "FROM postgres:17-alpine\nCOPY server.crt /etc/postgresql/server.crt\nCOPY server.key /etc/postgresql/server.key\nRUN chown postgres:postgres /etc/postgresql/server.crt /etc/postgresql/server.key && chmod 600 /etc/postgresql/server.key\nCMD [\"postgres\", \"-c\", \"ssl=on\", \"-c\", \"ssl_cert_file=/etc/postgresql/server.crt\", \"-c\", \"ssl_key_file=/etc/postgresql/server.key\"]\n"
	if err := os.WriteFile(filepath.Join(contextDir, "Dockerfile"), []byte(dockerfile), 0o600); err != nil {
		t.Fatal(err)
	}
	return certPath
}

func buildPostgreSQLTestImage(t *testing.T, ctx context.Context, root string) string {
	t.Helper()
	name := fmt.Sprintf("arimms-pr06-postgres-%d", time.Now().UnixNano())
	contextDir := filepath.Join(root, "postgres-image")
	if output, err := nativeDockerCommand(ctx, "build", "--tag", name, contextDir).CombinedOutput(); err != nil {
		t.Fatalf("build TLS-enabled PostgreSQL test image: %v (%s)", err, strings.TrimSpace(string(output)))
	}
	t.Cleanup(func() { _ = nativeDockerCommand(context.Background(), "image", "rm", "--force", name).Run() })
	return name
}

func randomProfileSecret(t *testing.T) string {
	t.Helper()
	content := make([]byte, 18)
	if _, err := rand.Read(content); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(content)
}

func startProfileContainer(t *testing.T, ctx context.Context, name string, args ...string) {
	t.Helper()
	command := append([]string{"run", "--detach", "--name", name}, args...)
	if output, err := nativeDockerCommand(ctx, command...).CombinedOutput(); err != nil {
		t.Fatalf("start isolated profile fixture %s: %v (%s)", name, err, strings.TrimSpace(string(output)))
	}
}

func nativeDockerImageID(t *testing.T, ctx context.Context, image string) string {
	t.Helper()
	output, err := nativeDockerCommand(ctx, "image", "inspect", "--format", "{{.Id}}", image).CombinedOutput()
	if err != nil {
		t.Fatalf("inspect test fixture image %s: %v (%s)", image, err, strings.TrimSpace(string(output)))
	}
	return strings.TrimSpace(string(output))
}

func waitProfileContainerReady(t *testing.T, ctx context.Context, name, adminPassword string) {
	t.Helper()
	deadline := time.Now().Add(90 * time.Second)
	for time.Now().Before(deadline) {
		command := nativeDockerCommand(ctx, "exec", "-e", "PGPASSWORD="+adminPassword, name, "pg_isready", "-U", "postgres")
		if command.Run() == nil {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatal("timed out waiting for PostgreSQL fixture")
		case <-time.After(time.Second):
		}
	}
	t.Fatal("PostgreSQL fixture did not become ready")
}

func configurePostgreSQLMonitoringRole(t *testing.T, ctx context.Context, container, adminPassword, statement string) {
	t.Helper()
	deadline := time.Now().Add(90 * time.Second)
	var lastOutput []byte
	var lastError error
	for time.Now().Before(deadline) {
		command := nativeDockerCommand(ctx, "exec", "-e", "PGPASSWORD="+adminPassword, container, "psql", "-U", "postgres", "-d", "postgres", "-c", statement)
		lastOutput, lastError = command.CombinedOutput()
		if lastError == nil {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatalf("timed out configuring restricted PostgreSQL role: %v", ctx.Err())
		case <-time.After(time.Second):
		}
	}
	t.Fatalf("configure restricted PostgreSQL monitoring role: %v (%s)", lastError, strings.TrimSpace(string(lastOutput)))
}

func waitForApplicationMetric(t *testing.T, ctx context.Context, serviceDone <-chan error, sink *lockedTemplateE2EBuffer, logs *nativeCollectorLogReporter, nodeID, resourceID, metric string) {
	t.Helper()
	deadline := time.NewTimer(100 * time.Second)
	defer deadline.Stop()
	poll := time.NewTicker(500 * time.Millisecond)
	defer poll.Stop()
	for {
		if applicationSinkContainsIdentity(sink.String(), metric, nodeID, resourceID) {
			return
		}
		select {
		case err := <-serviceDone:
			t.Fatalf("Agent runtime exited before %s delivery: %v; sink=%s; Collector=%s", metric, err, templateSinkDiagnostic(sink.String()), strings.Join(logs.output(), "\n"))
		case <-ctx.Done():
			t.Fatalf("timed out waiting for %s delivery; sink=%s; Collector=%s", metric, templateSinkDiagnostic(sink.String()), strings.Join(logs.output(), "\n"))
		case <-deadline.C:
			t.Fatalf("timed out waiting for %s delivery; sink=%s; Collector=%s", metric, templateSinkDiagnostic(sink.String()), strings.Join(logs.output(), "\n"))
		case <-poll.C:
		}
	}
}

func applicationSinkContainsIdentity(output, metric, nodeID, resourceID string) bool {
	for _, block := range strings.Split(output, "Resource attributes:")[1:] {
		attributes := strings.SplitN(block, "ScopeMetrics #", 2)[0]
		if strings.Contains(block, "Name: "+metric) && strings.Contains(attributes, "node_id: Str("+nodeID+")") && strings.Contains(attributes, "resource_id: Str("+resourceID+")") {
			return true
		}
	}
	return false
}

func applicationDuplicateSamples(output, metricName string) []string {
	counts := make(map[string]int)
	for _, resource := range strings.Split(output, "Resource attributes:")[1:] {
		attributes := strings.SplitN(resource, "ScopeMetrics #", 2)[0]
		resourceID := nativeStringAttribute(attributes, "resource_id")
		if resourceID == "" {
			continue
		}
		for _, metric := range strings.Split(resource, "Metric #")[1:] {
			if !strings.Contains(metric, "Name: "+metricName) {
				continue
			}
			if timestamp := nativeLineValue(metric, "Timestamp:"); timestamp != "" {
				counts[resourceID+"/"+metricName+"/"+timestamp]++
			}
		}
	}
	duplicates := make([]string, 0)
	for key, count := range counts {
		if count > 1 {
			duplicates = append(duplicates, key)
		}
	}
	return duplicates
}
