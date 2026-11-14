package nats

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ar-imms/telemetry-agent/internal/agenthealth"
	"github.com/ar-imms/telemetry-agent/internal/agentstate"
	"github.com/ar-imms/telemetry-agent/internal/dependency"
	discoverydocker "github.com/ar-imms/telemetry-agent/internal/discovery/docker"
	"github.com/ar-imms/telemetry-agent/internal/identity"
	"github.com/ar-imms/telemetry-agent/internal/operations"
	"github.com/ar-imms/telemetry-agent/internal/operations/handlers"
	operationssqlite "github.com/ar-imms/telemetry-agent/internal/operations/sqlite"
	"github.com/ar-imms/telemetry-agent/internal/operations/testsupport/mockcore"
	"github.com/nats-io/nats.go/jetstream"
)

func TestLiveJetStreamReadOnlyOperationsE2E(t *testing.T) {
	if os.Getenv("AR_IMMS_PR09_NATS_E2E") != "1" {
		t.Skip("set AR_IMMS_PR09_NATS_E2E=1 with a live authenticated TLS JetStream server")
	}
	installationID, err := identity.ParseInstallationID("11111111-2222-4333-8444-555555555555")
	if err != nil {
		t.Fatal(err)
	}
	installation := installationID.String()
	binding := "binding-pr09-live"
	suffix := strings.ReplaceAll(installation, "-", "")
	requestStream := "ARIMMS_OP_REQUESTS_PR09"
	resultStream := "ARIMMS_OP_RESULTS_PR09"
	agentDurable := "AGENT_PR09_" + suffix
	coreDurable := "CORE_PR09_" + suffix
	fixture := startTestNATSServer(t, installation, requestStream, resultStream, agentDurable)

	core, err := Connect(Config{URL: fixture.url, Username: fixture.coreUser, Password: fixture.corePassword, CAFile: fixture.caFile,
		InstallationID: installation, BindingID: binding, Stream: requestStream, Durable: coreDurable})
	if err != nil {
		t.Fatal(err)
	}
	defer core.Close()
	if _, err := core.jetstream.CreateStream(context.Background(), jetstream.StreamConfig{Name: requestStream,
		Subjects: []string{"arimms.ops.v1.request." + installation}, Storage: jetstream.FileStorage,
		Retention: jetstream.LimitsPolicy, MaxAge: 24 * time.Hour, MaxMsgs: 10000, MaxBytes: 256 * 1024 * 1024}); err != nil {
		t.Fatal(err)
	}
	if _, err := core.jetstream.CreateStream(context.Background(), jetstream.StreamConfig{Name: resultStream,
		Subjects: []string{"arimms.ops.v1.result." + installation}, Storage: jetstream.FileStorage,
		Retention: jetstream.LimitsPolicy, MaxAge: 24 * time.Hour, MaxMsgs: 10000, MaxBytes: 256 * 1024 * 1024}); err != nil {
		t.Fatal(err)
	}
	consumerConfig := jetstream.ConsumerConfig{Name: agentDurable, Durable: agentDurable,
		FilterSubject: "arimms.ops.v1.request." + installation, AckPolicy: jetstream.AckExplicitPolicy,
		AckWait: 15 * time.Second, MaxDeliver: 5, MaxAckPending: 1, DeliverPolicy: jetstream.DeliverAllPolicy}
	if _, err := core.jetstream.CreateConsumer(context.Background(), requestStream, consumerConfig); err != nil {
		t.Fatal(err)
	}
	coreResultConfig := jetstream.ConsumerConfig{Name: coreDurable, Durable: coreDurable,
		FilterSubject: "arimms.ops.v1.result." + installation, AckPolicy: jetstream.AckExplicitPolicy,
		AckWait: 15 * time.Second, MaxDeliver: 5, MaxAckPending: 1, DeliverPolicy: jetstream.DeliverAllPolicy}
	if _, err := core.jetstream.CreateConsumer(context.Background(), resultStream, coreResultConfig); err != nil {
		t.Fatal(err)
	}
	mock, err := mockcore.New(core.jetstream, installation, requestStream, resultStream, coreDurable)
	if err != nil {
		t.Fatal(err)
	}
	coreContext, stopCore := context.WithCancel(context.Background())
	defer stopCore()
	if err := mock.Start(coreContext); err != nil {
		t.Fatal(err)
	}
	defer mock.Close()

	statePath := filepath.Join(t.TempDir(), "agent.db")
	store := agentstate.NewSQLiteStore(statePath)
	committed, err := agentstate.NewIdentityStore(statePath).EnsureInstallationID(installationID)
	if err != nil || committed.String() != installation {
		t.Fatalf("persist test identity=%s err=%v", committed.String(), err)
	}
	journal := operationssqlite.New(store)
	registry := operations.NewRegistry()
	health := &countedHealth{snapshot: agenthealth.Evaluate(agenthealth.RuntimeObservation{CollectorState: agenthealth.CollectorStateReady})}
	if err := operations.Register(registry, handlers.Diagnostics{Health: health}); err != nil {
		t.Fatal(err)
	}
	endpoint := strings.TrimSpace(os.Getenv("AR_IMMS_DOCKER_DISCOVERY_ENDPOINT"))
	if endpoint == "" {
		if runtime.GOOS == "windows" {
			endpoint = "npipe:////./pipe/docker_engine"
		} else {
			endpoint = "unix:///var/run/docker.sock"
		}
	}
	dockerProvider, err := discoverydocker.NewProvider(endpoint)
	if err != nil {
		t.Fatal(err)
	}
	if err := operations.Register(registry, handlers.ContainerInspect{Containers: liveContainerInspector{provider: dockerProvider}}); err != nil {
		t.Fatal(err)
	}
	dependencyInspector := &liveDependencyInspector{}
	if err := operations.Register(registry, handlers.DependencyInspect{Dependencies: dependencyInspector}); err != nil {
		t.Fatal(err)
	}
	executor := &operations.Executor{Registry: registry, Store: journal, InstallationID: installation, BindingID: binding}
	containerID := fixtureContainerID(t, fixture.container)
	requests := []operations.Request{
		{OperationID: "pr09-live-diagnostics-1", OperationType: operations.OperationAgentDiagnostics, Parameters: []byte(`{}`)},
		{OperationID: "pr09-live-container-inspect-1", OperationType: operations.OperationContainerInspect,
			Target:     operations.Target{Kind: "docker_container", ProviderID: "docker", ResourceID: containerID},
			Parameters: []byte(`{"container_id":"` + containerID + `"}`)},
		{OperationID: "pr09-live-dependency-inspect-1", OperationType: operations.OperationDependencyInspect,
			Target:     operations.Target{Kind: "managed_dependency", ProviderID: "agent", ResourceID: "node-exporter"},
			Parameters: []byte(`{"name":"node-exporter"}`)},
	}
	for index := range requests {
		requests[index].SchemaVersion = operations.RequestSchemaV1
		requests[index].InstallationID = installation
		requests[index].BindingID = binding
		requests[index].RequestedAt = time.Now().UTC().Add(-time.Second).Format(time.RFC3339Nano)
		requests[index].ExpiresAt = time.Now().UTC().Add(time.Minute).Format(time.RFC3339Nano)
	}
	// The durable request must remain available while the Agent is offline.
	if err := mock.PublishOperation(context.Background(), requests[0]); err != nil {
		t.Fatal(err)
	}
	// Persist the queued delivery across a real JetStream server restart while the Agent is offline.
	restartNATSFixture(t, fixture)
	if err := publishEventually(mock, requests[0]); err != nil {
		t.Fatalf("confirm queued command after JetStream restart: %v", err)
	}
	agent, err := Connect(Config{URL: fixture.url, Username: fixture.agentUser, Password: fixture.agentPassword, CAFile: fixture.caFile,
		InstallationID: installation, BindingID: binding, Stream: requestStream, Durable: agentDurable})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := agent.jetstream.Publish(context.Background(), "arimms.ops.v1.result.other-installation", []byte(`{}`)); err == nil {
		t.Fatal("Agent credential published a result for another installation")
	}
	if _, err := agent.jetstream.Consumer(context.Background(), requestStream, "OTHER_INSTALLATION_CONSUMER"); err == nil {
		t.Fatal("Agent credential accessed an unprovisioned consumer")
	}
	if _, err := agent.jetstream.Publish(context.Background(), "arimms.ops.v1.request."+installation, []byte(`{}`)); err == nil {
		t.Fatal("Agent credential published an operation command")
	}
	if _, err := agent.jetstream.CreateStream(context.Background(), jetstream.StreamConfig{Name: "AGENT_CREATED_STREAM", Subjects: []string{"agent.created.>"}}); err == nil {
		t.Fatal("Agent credential administered JetStream streams")
	}
	agentContext, stopAgent := context.WithCancel(context.Background())
	agentDone := make(chan error, 1)
	go func() { agentDone <- agent.Run(agentContext, executor, journal) }()
	defer func() {
		stopAgent()
		select {
		case <-agentDone:
		case <-time.After(8 * time.Second):
			t.Error("Agent Operations consumer did not stop within its shutdown bound")
		}
		agent.Close()
	}()

	for index, request := range requests {
		if index != 0 {
			if err := mock.PublishOperation(context.Background(), request); err != nil {
				t.Fatal(err)
			}
		}
		select {
		case result := <-mock.Results():
			if result.OperationID != request.OperationID || result.Status != operations.StatusSucceeded || result.InstallationID != installation {
				t.Fatalf("Mock Core received unexpected result: %+v", result)
			}
			if request.OperationType == operations.OperationContainerInspect && strings.Contains(string(result.Result), "PR09_SECRET_FIXTURE") {
				t.Fatalf("container inspection leaked the environment fixture: %s", result.Result)
			}
		case <-time.After(20 * time.Second):
			t.Fatalf("Mock Core did not receive result for %s", request.OperationType)
		}
	}
	if health.calls.Load() != 1 {
		t.Fatalf("diagnostics handler calls=%d, want 1", health.calls.Load())
	}
	if dependencyInspector.calls.Load() != 1 {
		t.Fatalf("managed dependency inspector calls=%d, want 1", dependencyInspector.calls.Load())
	}
	// A transport redelivery with the same operation ID must use the committed result.
	if err := mock.PublishOperationWithMessageID(context.Background(), requests[0], "pr09-live-diagnostics-redelivery"); err != nil {
		t.Fatal(err)
	}
	select {
	case duplicate := <-mock.Results():
		t.Fatalf("duplicate logical operation emitted another result: %+v", duplicate)
	case <-time.After(2 * time.Second):
	}
	if health.calls.Load() != 1 {
		t.Fatalf("redelivered operation re-executed diagnostics %d times", health.calls.Load())
	}
	restartNATSFixture(t, fixture)
	recoveryRequest := requests[0]
	recoveryRequest.OperationID = "pr09-live-diagnostics-after-reconnect"
	recoveryRequest.RequestedAt = time.Now().UTC().Add(-time.Second).Format(time.RFC3339Nano)
	recoveryRequest.ExpiresAt = time.Now().UTC().Add(time.Minute).Format(time.RFC3339Nano)
	if err := publishEventually(mock, recoveryRequest); err != nil {
		t.Fatal(err)
	}
	select {
	case result := <-mock.Results():
		if result.OperationID != recoveryRequest.OperationID || result.Status != operations.StatusSucceeded {
			t.Fatalf("post-reconnect result=%+v", result)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("Agent and Mock Core did not recover after the JetStream restart")
	}
	if health.calls.Load() != 2 {
		t.Fatalf("diagnostics calls after JetStream restart=%d, want 2", health.calls.Load())
	}
	var published sql.NullString
	var auditCount int
	db, err := store.OpenDatabase()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.QueryRow(`SELECT published_at FROM operation_result_outbox WHERE operation_id=?`, requests[0].OperationID).Scan(&published); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM operation_audit_event WHERE operation_id IN (?,?,?,?)`, requests[0].OperationID, requests[1].OperationID, requests[2].OperationID, recoveryRequest.OperationID).Scan(&auditCount); err != nil {
		t.Fatal(err)
	}
	if !published.Valid || auditCount != 12 {
		t.Fatalf("outbox published=%v audit events=%d", published.Valid, auditCount)
	}
}

func restartNATSFixture(t *testing.T, fixture natsFixture) {
	t.Helper()
	if output, err := dockerCommand("restart", "--time", "1", fixture.container).CombinedOutput(); err != nil {
		t.Fatalf("restart isolated JetStream fixture: %v: %s", err, strings.TrimSpace(string(output)))
	}
}

func publishEventually(mock *mockcore.Core, request operations.Request) error {
	deadline := time.Now().Add(30 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		lastErr = mock.PublishOperation(ctx, request)
		cancel()
		if lastErr == nil {
			return nil
		}
		time.Sleep(250 * time.Millisecond)
	}
	return fmt.Errorf("publish operation after JetStream reconnect: %w", lastErr)
}

type liveContainerInspector struct{ provider *discoverydocker.Provider }

func (i liveContainerInspector) Inspect(ctx context.Context, id string) (handlers.ContainerObservation, error) {
	inspection, err := i.provider.InspectContainer(ctx, id)
	if err != nil {
		return handlers.ContainerObservation{}, err
	}
	return handlers.ContainerObservation{ID: inspection.ID, Name: inspection.Name, Image: inspection.Image, State: inspection.State,
		Health: inspection.Health, RestartCount: inspection.RestartCount, StartedAt: inspection.StartedAt}, nil
}

type liveDependencyInspector struct{ calls atomic.Int32 }

func (i *liveDependencyInspector) Inspect(_ context.Context, name string) (handlers.DependencyObservation, error) {
	i.calls.Add(1)
	return handlers.DependencyObservation{Name: name, Availability: string(dependency.AvailabilityEnabled), Health: string(dependency.HealthHealthy)}, nil
}

func fixtureContainerID(t *testing.T, name string) string {
	t.Helper()
	output, err := dockerCommand("inspect", "--format", "{{.Id}}", name).CombinedOutput()
	if err != nil {
		t.Fatalf("read isolated NATS container identity: %v: %s", err, strings.TrimSpace(string(output)))
	}
	return strings.TrimSpace(string(output))
}

type countedHealth struct {
	snapshot agenthealth.Snapshot
	calls    atomic.Int32
}

func (h *countedHealth) SnapshotContext(context.Context) agenthealth.Snapshot {
	h.calls.Add(1)
	return h.snapshot
}
