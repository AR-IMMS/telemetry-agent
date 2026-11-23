package nats

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	agenthealth "github.com/ar-imms/telemetry-agent/internal/health"
	credentialstore "github.com/ar-imms/telemetry-agent/internal/identity/credentials"
	installationid "github.com/ar-imms/telemetry-agent/internal/identity/installation/domain"
	discoverydocker "github.com/ar-imms/telemetry-agent/internal/inventory/docker"
	operations "github.com/ar-imms/telemetry-agent/internal/operations/command"
	operationsdocker "github.com/ar-imms/telemetry-agent/internal/operations/dockeradapter"
	"github.com/ar-imms/telemetry-agent/internal/operations/handlers"
	operationssqlite "github.com/ar-imms/telemetry-agent/internal/operations/sqlite"
	"github.com/ar-imms/telemetry-agent/internal/operations/testsupport/mockcore"
	"github.com/ar-imms/telemetry-agent/internal/runtime/dependencies"
	"github.com/ar-imms/telemetry-agent/internal/testsupport/sqlitefixture"
	"github.com/nats-io/nats.go/jetstream"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestLiveJetStreamOperationsE2E(t *testing.T) {
	if os.Getenv("AR_IMMS_PR09_NATS_E2E") != "1" {
		t.Skip("set AR_IMMS_PR09_NATS_E2E=1 with a live authenticated TLS JetStream server")
	}
	installationID, err := installationid.ParseInstallationID("11111111-2222-4333-8444-555555555555")
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
	store := sqlitefixture.NewSQLiteStore(statePath)
	committed, err := sqlitefixture.NewIdentityStore(statePath).EnsureInstallationID(installationID)
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
	containerRestarter, err := operationsdocker.New(endpoint)
	if err != nil {
		t.Fatal(err)
	}
	if err := operations.Register(registry, handlers.ContainerRestart{Runtime: containerRestarter, Enabled: true, VerifyInterval: 100 * time.Millisecond}); err != nil {
		t.Fatal(err)
	}
	executor := &operations.Executor{Registry: registry, Store: journal, InstallationID: installation, BindingID: binding}
	containerID := fixtureContainerID(t, fixture.container)
	restartContainer := startOptedInRestartContainer(t)
	restartContainerID := fixtureContainerID(t, restartContainer)
	restartBefore := inspectRestartObservation(t, containerRestarter, restartContainerID)
	requests := []operations.Request{
		{OperationID: "pr09-live-diagnostics-1", OperationType: operations.OperationAgentDiagnostics, Parameters: []byte(`{}`)},
		{OperationID: "pr09-live-container-inspect-1", OperationType: operations.OperationContainerInspect,
			Target:     operations.Target{Kind: "docker_container", ProviderID: "docker", ResourceID: containerID},
			Parameters: []byte(`{"container_id":"` + containerID + `"}`)},
		{OperationID: "pr09-live-dependency-inspect-1", OperationType: operations.OperationDependencyInspect,
			Target:     operations.Target{Kind: "managed_dependency", ProviderID: "agent", ResourceID: "node-exporter"},
			Parameters: []byte(`{"name":"node-exporter"}`)},
		{OperationID: "pr09-live-container-restart-1", OperationType: operations.OperationContainerRestart,
			Target:     operations.Target{Kind: "docker_container", ProviderID: "docker", ResourceID: restartContainerID},
			Parameters: []byte(`{"container_id":"` + restartContainerID + `"}`)},
	}
	for index := range requests {
		requests[index].SchemaVersion = operations.RequestSchemaV1
		requests[index].InstallationID = installation
		requests[index].BindingID = binding
		requests[index].RequestedAt = time.Now().UTC().Add(-time.Second).Format(time.RFC3339Nano)
		requests[index].ExpiresAt = time.Now().UTC().Add(3 * time.Minute).Format(time.RFC3339Nano)
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
	outboxGate := &liveOutboxGate{delegate: journal}
	agentSession := startLiveAgentSession(agent, executor, outboxGate)
	defer agentSession.Stop(t)

	for index, request := range requests {
		if index != 0 {
			if err := mock.PublishOperation(context.Background(), request); err != nil {
				t.Fatal(err)
			}
		}
		resultWait := 20 * time.Second
		if request.OperationType == operations.OperationContainerRestart {
			// The Executor permits up to 30 seconds for a restart outcome to be verified.
			resultWait = 40 * time.Second
		}
		select {
		case result := <-mock.Results():
			if result.OperationID != request.OperationID || result.Status != operations.StatusSucceeded || result.InstallationID != installation {
				if request.OperationType == operations.OperationContainerRestart {
					restartAfter := inspectRestartObservation(t, containerRestarter, restartContainerID)
					t.Fatalf("Mock Core received unexpected restart result: %+v; before=%+v after=%+v", result, restartBefore, restartAfter)
				}
				t.Fatalf("Mock Core received unexpected result: %+v", result)
			}
			if request.OperationType == operations.OperationContainerInspect && strings.Contains(string(result.Result), "PR09_SECRET_FIXTURE") {
				t.Fatalf("container inspection leaked the environment fixture: %s", result.Result)
			}
			if request.OperationType == operations.OperationContainerRestart {
				var restartResult handlers.ContainerRestartResult
				if err := json.Unmarshal(result.Result, &restartResult); err != nil {
					t.Fatalf("decode restart verification: %v", err)
				}
				startedBefore, beforeErr := time.Parse(time.RFC3339Nano, restartResult.StartedAtBefore)
				startedAfter, afterErr := time.Parse(time.RFC3339Nano, restartResult.StartedAtAfter)
				if (restartResult.RestartCountAfter <= restartResult.RestartCountBefore && (beforeErr != nil || afterErr != nil || !startedAfter.After(startedBefore))) || restartResult.State != "running" || restartResult.RecoveryStatus != "RUNNING" {
					t.Fatalf("container restart was not verified: %+v", restartResult)
				}
			}
		case <-time.After(resultWait):
			t.Fatalf("Mock Core did not receive result for %s within %s; requested_at=%s expires_at=%s", request.OperationType, resultWait, request.RequestedAt, request.ExpiresAt)
		}
	}
	if health.calls.Load() != 1 {
		t.Fatalf("diagnostics handler calls=%d, want 1", health.calls.Load())
	}
	if dependencyInspector.calls.Load() != 1 {
		t.Fatalf("managed dependency inspector calls=%d, want 1", dependencyInspector.calls.Load())
	}
	verifiedRestart := inspectRestartObservation(t, containerRestarter, restartContainerID)
	if err := mock.PublishOperationWithMessageID(context.Background(), requests[3], "pr09-live-container-restart-redelivery"); err != nil {
		t.Fatal(err)
	}
	select {
	case duplicate := <-mock.Results():
		t.Fatalf("duplicate restart produced another result: %+v", duplicate)
	case <-time.After(2 * time.Second):
	}
	if got := inspectRestartObservation(t, containerRestarter, restartContainerID); got.RestartCount != verifiedRestart.RestartCount || got.StartedAt != verifiedRestart.StartedAt {
		t.Fatalf("duplicate operation restarted container again: before=%+v after=%+v", verifiedRestart, got)
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
	outboxRecoveryRequest := requests[0]
	outboxRecoveryRequest.OperationID = "pr09-live-outbox-after-agent-restart"
	outboxRecoveryRequest.RequestedAt = time.Now().UTC().Add(-time.Second).Format(time.RFC3339Nano)
	outboxRecoveryRequest.ExpiresAt = time.Now().UTC().Add(time.Minute).Format(time.RFC3339Nano)
	outboxGate.block.Store(true)
	if err := publishEventually(mock, outboxRecoveryRequest); err != nil {
		t.Fatalf("publish command for Agent restart/outbox recovery: %v", err)
	}
	committedOutbox := waitForPendingOperation(t, journal, outboxRecoveryRequest.OperationID)
	if committedOutbox.EventID == "" {
		t.Fatal("completed operation has no durable result event identity")
	}
	// Stop the Agent after its outcome is committed but before the outbox can publish.
	agentSession.Stop(t)
	if output, err := dockerCommand("stop", fixture.container).CombinedOutput(); err != nil {
		t.Fatalf("stop JetStream after the result commit: %v: %s", err, strings.TrimSpace(string(output)))
	}
	if output, err := dockerCommand("start", fixture.container).CombinedOutput(); err != nil {
		t.Fatalf("restart JetStream before Agent recovery: %v: %s", err, strings.TrimSpace(string(output)))
	}
	agentConfig := Config{URL: fixture.url, Username: fixture.agentUser, Password: fixture.agentPassword, CAFile: fixture.caFile,
		InstallationID: installation, BindingID: binding, Stream: requestStream, Durable: agentDurable}
	agent, err = connectTestNATSEventually(agentConfig)
	if err != nil {
		t.Fatalf("reconnect Agent after broker restart: %v", err)
	}
	agentSession = startLiveAgentSession(agent, executor, journal)
	defer agentSession.Stop(t)
	select {
	case result := <-mock.Results():
		if result.OperationID != outboxRecoveryRequest.OperationID || result.EventID != committedOutbox.EventID || result.Status != operations.StatusSucceeded {
			t.Fatalf("recovered outbox result=%+v, committed event=%s", result, committedOutbox.EventID)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("Agent restart did not publish its committed outbox result after broker recovery")
	}
	if health.calls.Load() != 3 {
		t.Fatalf("diagnostics calls after Agent/outbox recovery=%d, want 3", health.calls.Load())
	}
	consumer, err := agent.jetstream.Consumer(context.Background(), requestStream, agentDurable)
	if err != nil {
		t.Fatal(err)
	}
	consumerInfo, err := consumer.Info(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	rejectedBinding := requests[0]
	rejectedBinding.OperationID = "pr09-live-obsolete-binding"
	rejectedBinding.BindingID = "obsolete-binding"
	rejectedBinding.RequestedAt = time.Now().UTC().Add(-time.Minute).Format(time.RFC3339Nano)
	rejectedBinding.ExpiresAt = time.Now().UTC().Add(time.Minute).Format(time.RFC3339Nano)
	expiredRequest := requests[0]
	expiredRequest.OperationID = "pr09-live-expired-request"
	expiredRequest.RequestedAt = time.Now().UTC().Add(-2 * time.Minute).Format(time.RFC3339Nano)
	expiredRequest.ExpiresAt = time.Now().UTC().Add(-time.Minute).Format(time.RFC3339Nano)
	for _, rejected := range []operations.Request{rejectedBinding, expiredRequest} {
		if err := mock.PublishOperation(context.Background(), rejected); err != nil {
			t.Fatalf("publish rejected %s command: %v", rejected.OperationID, err)
		}
	}
	if _, err := core.jetstream.Publish(context.Background(), "arimms.ops.v1.request."+installation, []byte(`{"schema_version":`)); err != nil {
		t.Fatalf("publish malformed command: %v", err)
	}
	waitForConsumerAckFloor(t, consumer, consumerInfo.AckFloor.Consumer+3)
	if health.calls.Load() != 3 {
		t.Fatalf("rejected commands invoked diagnostics %d times, want 3", health.calls.Load())
	}
	select {
	case result := <-mock.Results():
		t.Fatalf("rejected command produced a result: %+v", result)
	default:
	}
	var published sql.NullString
	var auditCount int
	var rejectedClaims int
	db, err := store.OpenDatabase()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.QueryRow(`SELECT published_at FROM operation_result_outbox WHERE operation_id=?`, requests[0].OperationID).Scan(&published); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM operation_audit_event WHERE operation_id IN (?,?,?,?,?,?)`, requests[0].OperationID, requests[1].OperationID, requests[2].OperationID, requests[3].OperationID, recoveryRequest.OperationID, outboxRecoveryRequest.OperationID).Scan(&auditCount); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM operation_execution WHERE operation_id IN (?,?)`, rejectedBinding.OperationID, expiredRequest.OperationID).Scan(&rejectedClaims); err != nil {
		t.Fatal(err)
	}
	if !published.Valid || auditCount != 18 {
		t.Fatalf("outbox published=%v audit events=%d", published.Valid, auditCount)
	}
	if rejectedClaims != 0 {
		t.Fatalf("rejected commands created %d execution claims, want 0", rejectedClaims)
	}

	// Exercise versioned Core broker provisioning, protected-secret replacement,
	// and revocation against the live NATS server and Agent runner.
	agentSession.Stop(t)
	credentialRef := "operations-" + binding
	credentials := credentialstore.New(statePath)
	if err := credentials.Put(credentialRef, fixture.agentPassword); err != nil {
		t.Fatalf("persist initial protected Operations credential: %v", err)
	}
	brokerBinding := mockcore.BrokerBinding{SchemaVersion: mockcore.BrokerBindingSchemaV1,
		Installation: installation, BindingID: binding, Revision: 1, Status: "active", Endpoint: fixture.url,
		Username: fixture.agentUser, CredentialRef: credentialRef, CAFile: fixture.caFile,
		RequestStream: requestStream, Durable: agentDurable}
	if err := mock.ProvisionBrokerBinding(brokerBinding); err != nil {
		t.Fatalf("Mock Core provision initial binding: %v", err)
	}
	loadBinding := func(context.Context) (BindingState, error) {
		current, exists := mock.BrokerBinding(binding)
		if !exists {
			return BindingState{}, fmt.Errorf("Mock Core binding is not provisioned")
		}
		state := BindingState{SchemaVersion: BindingSchemaV1, Revision: fmt.Sprintf("%d", current.Revision), Active: current.Status == "active"}
		if !state.Active {
			return state, nil
		}
		password, err := credentials.Get(current.CredentialRef)
		if err != nil {
			return BindingState{}, err
		}
		state.Config = Config{URL: current.Endpoint, Username: current.Username, Password: password,
			CAFile: current.CAFile, InstallationID: current.Installation, BindingID: current.BindingID,
			Stream: current.RequestStream, Durable: current.Durable}
		return state, nil
	}
	runner := Runner{Binding: loadBinding, Executor: executor, Outbox: journal, PollInterval: 100 * time.Millisecond}
	runnerContext, stopRunner := context.WithCancel(context.Background())
	runnerDone := make(chan error, 1)
	go func() { runnerDone <- runner.Run(runnerContext) }()
	defer func() {
		stopRunner()
		select {
		case <-runnerDone:
		case <-time.After(8 * time.Second):
			t.Error("credential-aware Operations runner did not stop")
		}
	}()

	rotationRequest := makeLiveDiagnosticRequest(installation, binding, "pr09-live-binding-rotation")
	if err := publishEventually(mock, rotationRequest); err != nil {
		t.Fatalf("publish command after initial broker binding provision: %v", err)
	}
	awaitSuccessfulResult(t, mock, rotationRequest.OperationID)
	if err := waitForNATSUserConnections(fixture.monitorURL, fixture.agentUser, true); err != nil {
		t.Fatalf("initial Operations binding did not establish a live NATS session: %v", err)
	}

	rotatedPassword := randomFixtureSecret(t)
	fixture.provisionAgentCredential(t, rotatedPassword, true)
	waitForAgentCredential(t, fixture, rotatedPassword)
	if err := credentials.Put(credentialRef, rotatedPassword); err != nil {
		t.Fatalf("replace protected Operations credential: %v", err)
	}
	brokerBinding.Revision = 2
	if err := mock.ProvisionBrokerBinding(brokerBinding); err != nil {
		t.Fatalf("Mock Core publish rotated binding revision: %v", err)
	}
	rotationResultRequest := makeLiveDiagnosticRequest(installation, binding, "pr09-live-binding-rotated-result")
	if err := publishEventually(mock, rotationResultRequest); err != nil {
		t.Fatalf("publish command after credential rotation: %v", err)
	}
	awaitSuccessfulResult(t, mock, rotationResultRequest.OperationID)
	if err := waitForNATSUserConnections(fixture.monitorURL, fixture.agentUser, true); err != nil {
		t.Fatalf("rotated credential did not restore one active Operations session: %v", err)
	}
	if oldClient, err := Connect(Config{URL: fixture.url, Username: fixture.agentUser, Password: fixture.agentPassword, CAFile: fixture.caFile,
		InstallationID: installation, BindingID: binding, Stream: requestStream, Durable: agentDurable}); err == nil {
		oldClient.Invalidate()
		t.Fatal("old NATS password remained valid after credential replacement")
	}

	fixture.provisionAgentCredential(t, rotatedPassword, false)
	if err := credentials.Delete(credentialRef); err != nil {
		t.Fatalf("remove revoked protected Operations credential: %v", err)
	}
	brokerBinding = mockcore.BrokerBinding{SchemaVersion: mockcore.BrokerBindingSchemaV1,
		Installation: installation, BindingID: binding, Revision: 3, Status: "revoked"}
	if err := mock.ProvisionBrokerBinding(brokerBinding); err != nil {
		t.Fatalf("Mock Core revoke binding: %v", err)
	}
	if err := waitForNATSUserConnections(fixture.monitorURL, fixture.agentUser, false); err != nil {
		t.Fatalf("revoked binding retained an active NATS session: %v", err)
	}
	if oldClient, err := Connect(Config{URL: fixture.url, Username: fixture.agentUser, Password: rotatedPassword, CAFile: fixture.caFile,
		InstallationID: installation, BindingID: binding, Stream: requestStream, Durable: agentDurable}); err == nil {
		oldClient.Invalidate()
		t.Fatal("revoked NATS credential still authenticated")
	}
	revokedRequest := makeLiveDiagnosticRequest(installation, binding, "pr09-live-binding-revoked")
	if err := mock.PublishOperation(context.Background(), revokedRequest); err != nil {
		t.Fatalf("publish request while Operations binding revoked: %v", err)
	}
	time.Sleep(2 * time.Second)
	if health.calls.Load() != 5 {
		t.Fatalf("revoked binding executed %d diagnostics, want only the five pre-revocation commands", health.calls.Load())
	}
}

func makeLiveDiagnosticRequest(installation, binding, operationID string) operations.Request {
	now := time.Now().UTC()
	return operations.Request{SchemaVersion: operations.RequestSchemaV1, OperationID: operationID,
		OperationType: operations.OperationAgentDiagnostics, InstallationID: installation, BindingID: binding,
		RequestedAt: now.Add(-time.Second).Format(time.RFC3339Nano), ExpiresAt: now.Add(time.Minute).Format(time.RFC3339Nano),
		Parameters: []byte(`{}`)}
}

func awaitSuccessfulResult(t *testing.T, mock *mockcore.Core, operationID string) {
	t.Helper()
	select {
	case result := <-mock.Results():
		if result.OperationID != operationID || result.Status != operations.StatusSucceeded {
			t.Fatalf("unexpected result for %s: %+v", operationID, result)
		}
	case <-time.After(20 * time.Second):
		t.Fatalf("Mock Core did not receive result for %s", operationID)
	}
}

func waitForAgentCredential(t *testing.T, fixture natsFixture, password string) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		client, err := Connect(Config{URL: fixture.url, Username: fixture.agentUser, Password: password, CAFile: fixture.caFile,
			InstallationID: fixture.installation, BindingID: "credential-probe", Stream: fixture.requestStream, Durable: fixture.agentDurable})
		if err == nil {
			client.Invalidate()
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("NATS did not accept the newly provisioned binding credential")
}

func waitForNATSUserConnections(monitorURL, username string, wantConnected bool) error {
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		response, err := http.Get(monitorURL + "/connz?auth=1&user=" + username)
		if err == nil {
			var body struct {
				NumConnections int `json:"num_connections"`
			}
			decodeErr := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&body)
			_ = response.Body.Close()
			if decodeErr == nil {
				connected := body.NumConnections > 0
				if connected == wantConnected {
					return nil
				}
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("NATS connection monitor did not report user %q connected=%t", username, wantConnected)
}

type liveAgentSession struct {
	client  *Client
	cancel  context.CancelFunc
	done    chan error
	stopped atomic.Bool
}

func startLiveAgentSession(client *Client, executor *operations.Executor, outbox ResultOutbox) *liveAgentSession {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- client.Run(ctx, executor, outbox) }()
	return &liveAgentSession{client: client, cancel: cancel, done: done}
}

func (s *liveAgentSession) Stop(t *testing.T) {
	t.Helper()
	if s == nil || !s.stopped.CompareAndSwap(false, true) {
		return
	}
	s.cancel()
	select {
	case <-s.done:
	case <-time.After(8 * time.Second):
		t.Error("Agent Operations consumer did not stop within its shutdown bound")
	}
	s.client.Close()
}

type liveOutboxGate struct {
	delegate ResultOutbox
	block    atomic.Bool
}

func (g *liveOutboxGate) Pending(ctx context.Context, limit int) ([]operations.OutboxEvent, error) {
	if g.block.Load() {
		return nil, nil
	}
	return g.delegate.Pending(ctx, limit)
}

func (g *liveOutboxGate) MarkPublished(ctx context.Context, eventID string, at time.Time) error {
	return g.delegate.MarkPublished(ctx, eventID, at)
}

func (g *liveOutboxGate) MarkFailed(ctx context.Context, eventID, code string, next time.Time) error {
	return g.delegate.MarkFailed(ctx, eventID, code, next)
}

func waitForPendingOperation(t *testing.T, journal *operationssqlite.Repository, operationID string) operations.OutboxEvent {
	t.Helper()
	deadline := time.NewTimer(20 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		items, err := journal.Pending(context.Background(), 16)
		if err == nil {
			for _, item := range items {
				if item.OperationID == operationID {
					return item
				}
			}
		}
		select {
		case <-deadline.C:
			t.Fatalf("operation %s did not appear in the durable result outbox: %v", operationID, err)
		case <-ticker.C:
		}
	}
}

func connectTestNATSEventually(config Config) (*Client, error) {
	deadline := time.Now().Add(30 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		client, err := Connect(config)
		if err == nil {
			if _, err = client.jetstream.Consumer(context.Background(), config.Stream, config.Durable); err == nil {
				return client, nil
			}
			client.Close()
		}
		lastErr = err
		time.Sleep(250 * time.Millisecond)
	}
	return nil, fmt.Errorf("NATS did not accept the Agent connection before the deadline: %w", lastErr)
}

func waitForConsumerAckFloor(t *testing.T, consumer jetstream.Consumer, minimum uint64) {
	t.Helper()
	deadline := time.NewTimer(15 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		info, err := consumer.Info(context.Background())
		if err == nil && info.AckFloor.Consumer >= minimum {
			return
		}
		select {
		case <-deadline.C:
			t.Fatalf("JetStream consumer ack floor did not reach %d: info=%+v err=%v", minimum, info, err)
		case <-ticker.C:
		}
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

func startOptedInRestartContainer(t *testing.T) string {
	t.Helper()
	name := "arimms-pr09-restart-" + randomFixtureSecret(t)[:12]
	command := dockerCommand("run", "--detach", "--name", name, "--label",
		handlers.ContainerRestartOptInLabel+"=true", testNATSImage, "-js")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("start isolated opted-in Docker restart target: %v: %s", err, strings.TrimSpace(string(output)))
	}
	t.Cleanup(func() { _ = dockerCommand("rm", "--force", name).Run() })
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if strings.TrimSpace(string(mustCombinedOutput(t, dockerCommand("inspect", "--format", "{{.State.Status}}", name)))) == "running" {
			return name
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("opted-in restart target %s did not enter running state", name)
	return ""
}

func inspectRestartObservation(t *testing.T, runtime handlers.ContainerRestartRuntime, containerID string) handlers.ContainerRestartObservation {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	observation, err := runtime.InspectForRestart(ctx, containerID)
	if err != nil {
		t.Fatalf("inspect restart target %s: %v", containerID, err)
	}
	return observation
}

func mustCombinedOutput(t *testing.T, command *exec.Cmd) []byte {
	t.Helper()
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("run Docker fixture query: %v: %s", err, strings.TrimSpace(string(output)))
	}
	return output
}

type countedHealth struct {
	snapshot agenthealth.Snapshot
	calls    atomic.Int32
}

func (h *countedHealth) SnapshotContext(context.Context) agenthealth.Snapshot {
	h.calls.Add(1)
	return h.snapshot
}
