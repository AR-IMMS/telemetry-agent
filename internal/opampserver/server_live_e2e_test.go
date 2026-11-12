package opampserver_test

import (
	"context"
	"encoding/hex"
	"net"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/ar-imms/telemetry-agent/internal/coreapi"
	"github.com/ar-imms/telemetry-agent/internal/mockcore"
	"github.com/ar-imms/telemetry-agent/internal/opampserver"
	"github.com/google/uuid"
	opampclient "github.com/open-telemetry/opamp-go/client"
	clienttypes "github.com/open-telemetry/opamp-go/client/types"
	"github.com/open-telemetry/opamp-go/protobufs"
)

func TestLiveAuthenticatedSupervisorToMockCorePolicyAndStatus(t *testing.T) {
	if os.Getenv("AR_IMMS_PR07B_LIVE_E2E") != "1" {
		t.Skip("set AR_IMMS_PR07B_LIVE_E2E=1 to run authenticated OpAMP/Core live E2E")
	}
	const token = "pr07b-test-bearer"
	const instanceUID = "aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee"
	mock, err := mockcore.New(mockcore.ProvisionedInstallation{
		AccessToken: token, InstallationID: "11111111-2222-4333-8444-555555555555",
		NodeID: "33333333-4444-4555-8666-777777777777",
		Policy: coreapi.DesiredGatewayPolicy{SchemaVersion: "ar-imms/gateway-policy/v1", Revision: 1, GatewayEndpoint: "127.0.0.1:4317"},
	})
	if err != nil {
		t.Fatal(err)
	}
	coreServer := httptest.NewTLSServer(mock)
	defer coreServer.Close()
	coreFactory := func(bearer string) (coreapi.AgentManagementAPI, error) {
		return coreapi.NewAgentManagementClient(coreServer.URL, bearer, coreServer.Client())
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	listenEndpoint := listener.Addr().String()
	_ = listener.Close()
	adapter, err := opampserver.New(opampserver.Options{
		ListenEndpoint: listenEndpoint, ListenPath: "/v1/opamp", CoreClient: coreFactory,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := adapter.Stop(ctx); err != nil {
			t.Errorf("stop OpAMP adapter: %v", err)
		}
	}()

	uid, err := uuid.Parse(instanceUID)
	if err != nil {
		t.Fatal(err)
	}
	remoteConfig := make(chan *protobufs.AgentRemoteConfig, 4)
	statusReported := make(chan error, 1)
	connected := make(chan struct{}, 1)
	connectFailed := make(chan error, 1)
	protocolClient := opampclient.NewWebSocket(nil)
	if err := protocolClient.SetAgentDescription(&protobufs.AgentDescription{
		IdentifyingAttributes: []*protobufs.KeyValue{{
			Key:   "service.name",
			Value: &protobufs.AnyValue{Value: &protobufs.AnyValue_StringValue{StringValue: "ar-imms.telemetry-agent.test"}},
		}},
	}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := protocolClient.Start(ctx, clienttypes.StartSettings{
		OpAMPServerURL: "ws://" + listenEndpoint + "/v1/opamp",
		Header:         map[string][]string{"Authorization": {"Bearer " + token}},
		InstanceUid:    clienttypes.InstanceUid(uid),
		Capabilities: protobufs.AgentCapabilities_AgentCapabilities_AcceptsRemoteConfig |
			protobufs.AgentCapabilities_AgentCapabilities_ReportsRemoteConfig,
		Callbacks: clienttypes.Callbacks{
			OnConnect: func(context.Context) { connected <- struct{}{} },
			OnConnectFailed: func(_ context.Context, err error) {
				select {
				case connectFailed <- err:
				default:
				}
			},
			OnMessage: func(ctx context.Context, message *clienttypes.MessageData) {
				if message.RemoteConfig == nil {
					return
				}
				select {
				case remoteConfig <- message.RemoteConfig:
				default:
				}
				statusReported <- protocolClient.SetRemoteConfigStatus(&protobufs.RemoteConfigStatus{
					LastRemoteConfigHash: message.RemoteConfig.ConfigHash,
					Status:               protobufs.RemoteConfigStatuses_RemoteConfigStatuses_APPLIED,
				})
			},
		},
	}); err != nil {
		t.Fatal(err)
	}
	defer func() {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer stopCancel()
		if err := protocolClient.Stop(stopCtx); err != nil {
			t.Errorf("stop OpAMP client: %v", err)
		}
	}()

	select {
	case config := <-remoteConfig:
		file := config.GetConfig().GetConfigMap()[""]
		if file == nil || string(file.Body) != "exporters:\n  otlp/gateway:\n    endpoint: \"127.0.0.1:4317\"\n" {
			t.Fatalf("remote config = %+v", config)
		}
	case <-ctx.Done():
		select {
		case err := <-connectFailed:
			t.Fatalf("authenticated Supervisor client could not connect: %v", err)
		default:
		}
		select {
		case <-connected:
			t.Fatal("authenticated Supervisor client connected but received no remote policy")
		default:
			t.Fatal("authenticated Supervisor client neither connected nor received remote policy")
		}
	}
	select {
	case err := <-statusReported:
		if err != nil {
			t.Fatalf("report OpAMP status to client: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("Supervisor client did not report remote policy status")
	}
	deadline := time.Now().Add(5 * time.Second)
	for len(mock.Statuses()) == 0 && time.Now().Before(deadline) {
		time.Sleep(25 * time.Millisecond)
	}
	observed := mock.Statuses()
	if len(observed) == 0 || observed[0].Report.AppliedRevision != 1 || observed[0].Report.Status != "APPLIED" {
		t.Fatalf("Mock Core status evidence = %+v", observed)
	}

	// Advancing Core's policy revision without changing the rendered endpoint
	// keeps the OpAMP config hash stable. The same applied hash must now map to
	// revision 2 rather than remaining attributed to revision 1.
	if err := mock.SetPolicy("11111111-2222-4333-8444-555555555555", coreapi.DesiredGatewayPolicy{
		SchemaVersion: "ar-imms/gateway-policy/v1", Revision: 2, GatewayEndpoint: "127.0.0.1:4317",
	}); err != nil {
		t.Fatalf("advance Core policy revision without changing endpoint: %v", err)
	}
	appliedHash, err := hex.DecodeString(observed[0].Report.ConfigHash)
	if err != nil {
		t.Fatalf("decode applied config hash: %v", err)
	}
	if err := protocolClient.SetRemoteConfigStatus(&protobufs.RemoteConfigStatus{
		LastRemoteConfigHash: appliedHash,
		Status:               protobufs.RemoteConfigStatuses_RemoteConfigStatuses_APPLYING,
	}); err != nil {
		t.Fatalf("report unchanged config applying for new policy revision: %v", err)
	}
	deadline = time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		for _, status := range mock.Statuses() {
			if status.Report.Status == "APPLYING" && status.Report.DesiredRevision == 2 {
				goto applyingObserved
			}
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("unchanged config APPLYING status was not attributed to policy revision 2; statuses=%+v", mock.Statuses())

applyingObserved:
	if err := protocolClient.SetRemoteConfigStatus(&protobufs.RemoteConfigStatus{
		LastRemoteConfigHash: appliedHash,
		Status:               protobufs.RemoteConfigStatuses_RemoteConfigStatuses_APPLIED,
	}); err != nil {
		t.Fatalf("report unchanged applied config for new policy revision: %v", err)
	}
	deadline = time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		for _, status := range mock.Statuses() {
			if status.Report.Status == "APPLIED" && status.Report.AppliedRevision == 2 {
				return
			}
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("unchanged applied config hash was not attributed to policy revision 2; statuses=%+v", mock.Statuses())

}
