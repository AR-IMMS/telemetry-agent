package mockfleet

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	opampclient "github.com/open-telemetry/opamp-go/client"
	clienttypes "github.com/open-telemetry/opamp-go/client/types"
	"github.com/open-telemetry/opamp-go/protobufs"
)

func TestAuthenticatedOpAMPFixtureOffersAndRecordsDesiredConfig(t *testing.T) {
	const agentID = "agent-config-test"
	config := []byte("exporters:\n  otlp/gateway:\n    endpoint: 127.0.0.1:4317\n")
	fleet := New()
	if err := fleet.SeedInvitation("config-invitation", "node-config"); err != nil {
		t.Fatal(err)
	}
	if err := fleet.SetDesiredConfig(agentID, config); err != nil {
		t.Fatalf("set test desired config: %v", err)
	}
	enrollment := httptest.NewServer(fleet)
	t.Cleanup(enrollment.Close)
	request := []byte(`{"schema_version":1,"enrollment_attempt_id":"config-attempt","agent_id":"agent-config-test","invitation_token":"config-invitation"}`)
	status, body := exchange(t, enrollment.URL, request)
	if status != http.StatusCreated {
		t.Fatalf("enrollment status = %d; want %d", status, http.StatusCreated)
	}
	var credential struct {
		Credential string `json:"credential"`
	}
	if err := json.Unmarshal(body, &credential); err != nil {
		t.Fatalf("decode fixture credential: %v", err)
	}

	server, err := fleet.StartOpAMPServer()
	if err != nil {
		t.Fatalf("start provisional OpAMP server: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Stop(ctx); err != nil {
			t.Errorf("stop provisional OpAMP server: %v", err)
		}
	})
	instanceUID, err := uuid.NewRandom()
	if err != nil {
		t.Fatal(err)
	}
	remoteConfig := make(chan []byte, 1)
	client := opampclient.NewWebSocket(nil)
	if err := client.SetAgentDescription(&protobufs.AgentDescription{
		IdentifyingAttributes: []*protobufs.KeyValue{{
			Key: "service.name", Value: &protobufs.AnyValue{Value: &protobufs.AnyValue_StringValue{StringValue: agentID}},
		}},
	}); err != nil {
		t.Fatalf("set OpAMP AgentDescription: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := client.Start(ctx, clienttypes.StartSettings{
		OpAMPServerURL: server.Endpoint(),
		Header:         http.Header{"Authorization": []string{"Bearer " + credential.Credential}},
		InstanceUid:    clienttypes.InstanceUid(instanceUID),
		Capabilities:   protobufs.AgentCapabilities_AgentCapabilities_AcceptsRemoteConfig | protobufs.AgentCapabilities_AgentCapabilities_ReportsRemoteConfig,
		Callbacks: clienttypes.Callbacks{
			OnMessage: func(_ context.Context, message *clienttypes.MessageData) {
				if message.RemoteConfig == nil {
					return
				}
				file := message.RemoteConfig.GetConfig().GetConfigMap()[""]
				if file == nil {
					return
				}
				select {
				case remoteConfig <- append([]byte(nil), file.Body...):
				default:
				}
				_ = client.SetRemoteConfigStatus(&protobufs.RemoteConfigStatus{
					LastRemoteConfigHash: message.RemoteConfig.ConfigHash,
					Status:               protobufs.RemoteConfigStatuses_RemoteConfigStatuses_APPLIED,
				})
			},
		},
	}); err != nil {
		t.Fatalf("start authenticated OpAMP client: %v", err)
	}
	t.Cleanup(func() {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer stopCancel()
		if err := client.Stop(stopCtx); err != nil {
			t.Errorf("stop OpAMP client: %v", err)
		}
	})
	select {
	case got := <-remoteConfig:
		if !bytes.Equal(got, config) {
			t.Fatalf("remote config = %q; want %q", got, config)
		}
	case <-ctx.Done():
		t.Fatal("authenticated OpAMP session did not receive the configured remote payload")
	}
	wantHash := sha256.Sum256(config)
	deadline := time.NewTimer(10 * time.Second)
	defer deadline.Stop()
	for {
		for _, observed := range fleet.RemoteConfigStatuses(agentID) {
			if observed.GetStatus() == protobufs.RemoteConfigStatuses_RemoteConfigStatuses_APPLIED && bytes.Equal(observed.GetLastRemoteConfigHash(), wantHash[:]) {
				return
			}
		}
		select {
		case <-time.After(20 * time.Millisecond):
		case <-deadline.C:
			t.Fatalf("fixture did not record APPLIED status for hash %x: %+v", wantHash, fleet.RemoteConfigStatuses(agentID))
		}
	}
}
